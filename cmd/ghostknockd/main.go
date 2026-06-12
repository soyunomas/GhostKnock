// ghostknockd es el demonio del servidor blindado que escucha pasivamente los knocks.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/soyunomas/ghostknock/internal/config"
	"github.com/soyunomas/ghostknock/internal/executor"
	"github.com/soyunomas/ghostknock/internal/listener"
	"github.com/soyunomas/ghostknock/internal/protocol"
	"golang.org/x/time/rate"
)

// version se establece en tiempo de compilación.
var version = "dev"

// Default fallback si no hay config
const defaultLogFilePath = "/var/log/ghostknockd.log"

const replayCacheGuard = time.Second

// ipLimiter almacena el estado de rate limit por IP
type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type Server struct {
	// config ahora está protegido por configMutex para permitir Hot-Reload
	config      *config.Config
	configMutex sync.RWMutex
	configFile  string // Guardamos la ruta para recargar

	serverPrivateKey ed25519.PrivateKey

	// Mapas de estado
	actionCooldowns map[string]time.Time
	signaturesCache map[string]time.Time

	// Mutex RW para optimización de lectura en caché (Anti-Replay rápido)
	cacheMutex sync.RWMutex

	// Gestión de Rate Limit
	ipLimiters    map[string]*ipLimiter
	limitersMutex sync.Mutex

	// SEGURIDAD: Semáforo para limitar procesos concurrentes (Anti-ForkBomb)
	executionSem chan struct{}
	// SEGURIDAD: WaitGroup para asegurar que los scripts terminen al apagar (Integridad)
	executionWg sync.WaitGroup

	// VISIBILIDAD: Métricas atómicas (High Performance Counters)
	droppedPackets   uint64
	processedPackets uint64
}

func main() {
	// 1. Parseo de Flags
	showVersion := flag.Bool("version", false, "Muestra la versión de la aplicación y sale.")
	configFile := flag.String("config", "config.yaml", "Ruta al archivo de configuración YAML")
	testConfig := flag.Bool("t", false, "Prueba la sintaxis del archivo de configuración y sale.")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ghostknockd version %s\n", version)
		os.Exit(0)
	}

	if *testConfig {
		fmt.Printf("Probando la configuración desde: %s\n", *configFile)
		_, err := config.LoadConfig(*configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: La configuración es INVÁLIDA.\nDetalles: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("La sintaxis del archivo de configuración es correcta.")
		os.Exit(0)
	}

	// 2. Carga Inicial de Configuración
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		// Logger básico de emergencia a stderr si falla la carga inicial
		slog.Error("Error crítico al cargar la configuración inicial", "file", *configFile, "error", err)
		os.Exit(1)
	}

	// 3. Configuración de Logging (Dinámica)
	setupLogging(cfg.Logging)

	slog.Info("Iniciando demonio GhostKnockd (v2.1 Hot-Reload Ready)...")

	// 4. Carga de Claves
	serverPrivKeyBytes, err := os.ReadFile(cfg.ServerPrivateKeyPath)
	if err != nil {
		slog.Error("Error crítico al leer la clave privada del servidor", "path", cfg.ServerPrivateKeyPath, "error", err)
		os.Exit(1)
	}
	if len(serverPrivKeyBytes) != ed25519.PrivateKeySize {
		slog.Error("El archivo de clave privada del servidor tiene un tamaño incorrecto", "path", cfg.ServerPrivateKeyPath)
		os.Exit(1)
	}

	// 5. Gestión del PID File
	if cfg.Daemon.PIDFile != "" {
		pid := os.Getpid()
		pidStr := strconv.Itoa(pid)
		if err := os.WriteFile(cfg.Daemon.PIDFile, []byte(pidStr), 0644); err != nil {
			slog.Error("No se pudo escribir el archivo PID", "path", cfg.Daemon.PIDFile, "error", err)
			os.Exit(1)
		}
		defer os.Remove(cfg.Daemon.PIDFile)
	}

	// 6. Inicialización del Servidor
	server := &Server{
		config:           cfg,
		configFile:       *configFile,
		serverPrivateKey: ed25519.PrivateKey(serverPrivKeyBytes),
		actionCooldowns:  make(map[string]time.Time),
		signaturesCache:  make(map[string]time.Time),
		ipLimiters:       make(map[string]*ipLimiter),
		executionSem:     make(chan struct{}, 10), // Hard-cap de 10 procesos concurrentes por seguridad
	}

	ctx, cancel := context.WithCancel(context.Background())

	// --- GESTIÓN DE SEÑALES (Reload + Shutdown) ---
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Tareas de limpieza en segundo plano (Ahora dinámicas)
	go server.startCacheCleaner(ctx)
	go server.startLimiterCleaner(ctx)

	// --- HEARTBEAT DE MÉTRICAS ---
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				dropped := atomic.SwapUint64(&server.droppedPackets, 0)
				processed := atomic.SwapUint64(&server.processedPackets, 0)
				if dropped > 0 || processed > 0 {
					slog.Info("Estado del Servidor (10s)",
						"procesados", processed,
						"descartados_saturacion", dropped)
				}
			}
		}
	}()

	// --- INICIO DEL LISTENER ---
	// Usamos el buffer definido en Tuning
	packetsCh := make(chan listener.PacketInfo, cfg.Tuning.PacketChannelBuffer)

	// Callback de saturación
	onDrop := func() {
		atomic.AddUint64(&server.droppedPackets, 1)
	}

	// Listener asíncrono
	go listener.Start(ctx, cfg.Listener, cfg.Tuning.PcapTimeoutMs, packetsCh, onDrop)

	// --- WORKER POOL (Crypto) ---
	numWorkers := runtime.NumCPU()
	slog.Info("Iniciando Worker Pool", "workers", numWorkers, "buffer_size", cfg.Tuning.PacketChannelBuffer)

	var workerWg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		workerWg.Add(1)
		go func(workerID int) {
			defer workerWg.Done()
			for packet := range packetsCh {
				server.processKnock(packet)
			}
		}(i)
	}

	slog.Info("Servidor listo y blindado.")

	// 7. BUCLE PRINCIPAL DE SEÑALES
	for {
		sig := <-signalChan
		if sig == syscall.SIGHUP {
			slog.Info("Señal SIGHUP recibida: Recargando configuración...")
			server.reloadConfig()
		} else {
			// SIGINT o SIGTERM -> Apagado
			slog.Info("Señal recibida, iniciando secuencia de apagado...", "signal", sig.String())

			// Paso A: Detener entrada de nuevos paquetes
			cancel()

			// Paso B: Esperar a que los workers procesen lo que hay en el buffer
			slog.Info("Esperando drenaje del buffer de red...")
			workerWg.Wait()

			// Paso C: Esperar a que los scripts de ejecución terminen
			slog.Info("Esperando finalización de procesos activos...")
			server.executionWg.Wait()

			slog.Info("Apagado seguro completado.")
			break // Salir del bucle y terminar programa
		}
	}
}

// setupLogging configura el logger global basándose en cfg.Logging
func setupLogging(logCfg config.Logging) {
	var output io.Writer

	switch logCfg.LogFile {
	case "stdout":
		output = os.Stdout
	case "/dev/null":
		output = io.Discard
	default:
		// Default a archivo. Si viene vacío, usamos el default hardcoded por seguridad.
		path := logCfg.LogFile
		if path == "" {
			path = defaultLogFilePath
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			// Fallback a stdout si no se puede escribir en archivo
			fmt.Fprintf(os.Stderr, "ERROR: No se pudo abrir log file '%s': %v. Usando Stdout.\n", path, err)
			output = os.Stdout
		} else {
			output = f
		}
	}

	var logLevel slog.Level
	switch logCfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: logLevel}
	var handler slog.Handler

	if logCfg.LogFormat == "json" {
		handler = slog.NewJSONHandler(output, opts)
	} else {
		handler = slog.NewTextHandler(output, opts)
	}

	slog.SetDefault(slog.New(handler))
}

// reloadConfig maneja la lógica de recarga en caliente
func (s *Server) reloadConfig() {
	newCfg, err := config.LoadConfig(s.configFile)
	if err != nil {
		slog.Error("RELOAD FALLIDO: El archivo de configuración contiene errores. Manteniendo configuración anterior.", "error", err)
		return
	}

	s.configMutex.Lock()
	defer s.configMutex.Unlock()

	// Detectar cambios críticos que requieren reinicio
	needsRestart := false
	if newCfg.Listener != s.config.Listener {
		needsRestart = true
	}
	if newCfg.Tuning.PacketChannelBuffer != s.config.Tuning.PacketChannelBuffer {
		needsRestart = true
	}
	if newCfg.Tuning.PcapTimeoutMs != s.config.Tuning.PcapTimeoutMs {
		needsRestart = true
	}
	if preserveReplayWindowOnReload(s.config, newCfg) {
		needsRestart = true
	}

	if needsRestart {
		slog.Warn("RELOAD PARCIAL: Se han detectado cambios en Network, Tuning (Buffer/Timeout) o Replay Window. Estos cambios NO se aplican en caliente. Reinicie el servicio (systemctl restart) para aplicarlos.")
	}

	// Aplicar nueva configuración lógica
	s.config = newCfg

	// Actualizar logging en caliente
	setupLogging(newCfg.Logging)

	slog.Info("Configuración recargada exitosamente.",
		"users_count", len(newCfg.Users),
		"actions_count", len(newCfg.Actions))
}

func preserveReplayWindowOnReload(current, next *config.Config) bool {
	if next.Security.ReplayWindowSeconds == current.Security.ReplayWindowSeconds {
		return false
	}
	next.Security.ReplayWindowSeconds = current.Security.ReplayWindowSeconds
	return true
}

// getConfig obtiene una instantánea segura de la configuración actual
func (s *Server) getConfig() *config.Config {
	s.configMutex.RLock()
	defer s.configMutex.RUnlock()
	return s.config
}

// getLimiter implementa Rate Limiting usando parámetros dinámicos de Tuning
func (s *Server) getLimiter(ip net.IP, limit float64, burst int, tuning config.Tuning) *rate.Limiter {
	s.limitersMutex.Lock()
	defer s.limitersMutex.Unlock()

	ipStr := ip.String()

	if entry, exists := s.ipLimiters[ipStr]; exists {
		entry.lastSeen = time.Now()
		return entry.limiter
	}

	// Uso de parámetros dinámicos para protección de memoria
	if len(s.ipLimiters) >= tuning.MaxTrackedIPs {
		deleted := 0
		// Eviction policy: Random eviction (por iteración de mapa)
		// Mejoras futuras: LRU real (más costoso)
		for k := range s.ipLimiters {
			delete(s.ipLimiters, k)
			deleted++
			if deleted >= tuning.EvictionBatchSize {
				break
			}
		}
		slog.Warn("Tabla de IPs llena: purga parcial ejecutada",
			"purgados", deleted,
			"limite_actual", tuning.MaxTrackedIPs)
	}

	newLimiter := rate.NewLimiter(rate.Limit(limit), burst)
	s.ipLimiters[ipStr] = &ipLimiter{limiter: newLimiter, lastSeen: time.Now()}
	return newLimiter
}

func validatePayloadFreshness(now time.Time, payloadTS int64, pastWindow, futureSkew time.Duration) error {
	if pastWindow <= 0 {
		return fmt.Errorf("past window must be positive")
	}
	if futureSkew < 0 {
		return fmt.Errorf("future skew cannot be negative")
	}

	timestamp := time.Unix(0, payloadTS)
	if timestamp.Before(now.Add(-pastWindow)) {
		return fmt.Errorf("payload timestamp is older than the accepted window")
	}
	if timestamp.After(now.Add(futureSkew)) {
		return fmt.Errorf("payload timestamp is newer than the accepted clock skew")
	}
	return nil
}

func replayWindowDuration(seconds int) (time.Duration, error) {
	if seconds <= 0 || seconds > config.MaxReplayWindowSeconds {
		return 0, fmt.Errorf(
			"replay window must be between 1 and %d seconds",
			config.MaxReplayWindowSeconds,
		)
	}
	return time.Duration(seconds) * time.Second, nil
}

func replayCacheExpiration(now time.Time, payloadTS int64, pastWindow, guard time.Duration) (time.Time, error) {
	if pastWindow <= 0 {
		return time.Time{}, fmt.Errorf("past window must be positive")
	}
	if guard <= 0 {
		return time.Time{}, fmt.Errorf("replay cache guard must be positive")
	}

	timestamp := time.Unix(0, payloadTS)
	expiration := timestamp.Add(pastWindow).Add(guard)
	minExpiration := now.Add(guard)
	if expiration.Before(minExpiration) {
		expiration = minExpiration
	}
	return expiration, nil
}

func (s *Server) isSignatureKnown(signature []byte) bool {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	_, known := s.signaturesCache[string(signature)]
	return known
}

func (s *Server) storeSignatureIfNew(signature []byte, expiration time.Time) bool {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	key := string(signature)
	if _, exists := s.signaturesCache[key]; exists {
		return false
	}
	if s.signaturesCache == nil {
		s.signaturesCache = make(map[string]time.Time)
	}
	s.signaturesCache[key] = expiration
	return true
}

func purgeExpiredSignatures(signatures map[string]time.Time, now time.Time) int {
	purged := 0
	for signature, expiration := range signatures {
		if !now.Before(expiration) {
			delete(signatures, signature)
			purged++
		}
	}
	return purged
}

// processKnock maneja la lógica de negocio
func (s *Server) processKnock(packetInfo listener.PacketInfo) {
	atomic.AddUint64(&s.processedPackets, 1)

	// 1. Obtener configuración actual de forma segura (Thread-Safe)
	currentConfig := s.getConfig()

	// 1.5. LISTA NEGRA: Verificación Ultrarrápida (Short-Circuit)
	if currentConfig.IsIPDenied(packetInfo.SourceIP) {
		return
	}

	// 2. Rate Limit IP (Anti-DoS ligero)
	limiter := s.getLimiter(packetInfo.SourceIP,
		currentConfig.Security.RateLimitPerSecond,
		currentConfig.Security.RateLimitBurst,
		currentConfig.Tuning)

	if !limiter.Allow() {
		return // Descarte silencioso
	}

	// 3. Validación estructural básica
	if len(packetInfo.Payload) < ed25519.SignatureSize {
		return
	}

	sigBytes := packetInfo.Payload[:ed25519.SignatureSize]

	// 4. CACHÉ CHECK 1 (Read Lock - Barato)
	if s.isSignatureKnown(sigBytes) {
		slog.Debug("Replay detectado (Fast-Path)", "src", packetInfo.SourceIP)
		return
	}

	// 5. VALIDACIÓN CRIPTOGRÁFICA (CPU Intensivo)
	var authorizedUser *config.User
	var payload *protocol.Payload
	var err error

	for i := range currentConfig.Users {
		user := &currentConfig.Users[i]
		payload, err = protocol.VerifyAndDecrypt(packetInfo.Payload, user.DecodedPublicKey, s.serverPrivateKey)
		if err == nil {
			authorizedUser = user
			break
		}
	}

	if authorizedUser == nil {
		return // Firma inválida o basura
	}

	// Validar claves, valores y colisiones antes de extraer TOTP. De este modo
	// `otp` y `OTP` no pueden converger después en la misma variable de entorno.
	if err := executor.ValidateParams(payload.Params); err != nil {
		slog.Warn("Parámetros inválidos; paquete rechazado", "user", authorizedUser.Name)
		return
	}

	// 6. VALIDACIÓN 2FA (TOTP) - FASE 3
	if authorizedUser.TotpSecret != "" {
		otpCode, ok := payload.Params["otp"]
		if !ok || otpCode == "" {
			slog.Warn("Autenticación fallida: Se requiere código OTP (2FA habilitado)", "user", authorizedUser.Name, "src", packetInfo.SourceIP)
			return
		}

		if !validateTOTP(authorizedUser.TotpSecret, otpCode) {
			slog.Warn("Autenticación fallida: Código OTP inválido", "user", authorizedUser.Name, "src", packetInfo.SourceIP)
			return
		}

		// Eliminar OTP de los parámetros para que no se pase al comando/script
		delete(payload.Params, "otp")
		slog.Debug("2FA (TOTP) verificado correctamente", "user", authorizedUser.Name)
	}
	if containsParamFold(payload.Params, "otp") {
		slog.Warn("Parámetro reservado presente; paquete rechazado", "user", authorizedUser.Name)
		return
	}

	// 7. Validación explícita del Timestamp del Payload
	window, err := replayWindowDuration(currentConfig.Security.ReplayWindowSeconds)
	if err != nil {
		slog.Error("Configuración temporal inválida; paquete rechazado", "error", err)
		return
	}
	futureSkew := window
	now := time.Now()
	if err := validatePayloadFreshness(now, payload.Timestamp, window, futureSkew); err != nil {
		slog.Warn("Paquete fuera de ventana temporal", "user", authorizedUser.Name)
		return
	}

	// 8. CACHÉ CHECK 2 + STORE (Write Lock - Atómico)
	expiration, err := replayCacheExpiration(now, payload.Timestamp, window, replayCacheGuard)
	if err != nil {
		slog.Error("Configuración temporal inválida; paquete rechazado", "error", err)
		return
	}
	if !s.storeSignatureIfNew(sigBytes, expiration) {
		slog.Warn("Replay detectado (Race-Win)", "user", authorizedUser.Name)
		return
	}

	// 9. Autorización de Acción y Cooldowns
	if !s.checkActionAuthAndCooldown(authorizedUser, payload, packetInfo.SourceIP, currentConfig) {
		return
	}

	slog.Info("Knock autorizado, solicitando slot de ejecución", "user", authorizedUser.Name, "action", payload.ActionID)

	// 10. EJECUCIÓN SEGURA
	select {
	case s.executionSem <- struct{}{}:
		s.executionWg.Add(1)

		go func() {
			defer func() {
				<-s.executionSem
				s.executionWg.Done()
				if r := recover(); r != nil {
					slog.Error("Panic recuperado en executor", "err", r)
				}
			}()

			// Buscamos la acción en la config actual
			actionDef := currentConfig.Actions[payload.ActionID]

			if actionDef.TimeoutSeconds <= 0 {
				actionDef.TimeoutSeconds = 30
			}

			// ACTUALIZADO: Pasamos config.Daemon para el shell configurable
			if err := executor.Execute(actionDef, authorizedUser.Name, packetInfo.SourceIP, payload.Params, currentConfig.GlobalHooks, currentConfig.Daemon); err != nil {
				slog.Error("Error en ejecución", "action", payload.ActionID, "error", err)
			}
		}()
	default:
		slog.Error("Ejecución rechazada: Límite de procesos concurrentes alcanzado", "user", authorizedUser.Name)
	}
}

func containsParamFold(params map[string]string, target string) bool {
	for key := range params {
		if strings.EqualFold(key, target) {
			return true
		}
	}
	return false
}

// checkActionAuthAndCooldown usa el snapshot de configuración pasado
func (s *Server) checkActionAuthAndCooldown(user *config.User, payload *protocol.Payload, sourceIP net.IP, cfg *config.Config) bool {
	// Verificar permisos
	allowed := false
	for _, a := range user.AllowedActions {
		if a == payload.ActionID {
			allowed = true
			break
		}
	}
	if !allowed {
		slog.Warn("Acción no autorizada para este usuario", "user", user.Name, "action", payload.ActionID)
		return false
	}

	// Verificar IP source (CIDR)
	if len(user.SourceCIDRs) > 0 {
		ipAllowed := false
		for _, subnet := range user.SourceCIDRs {
			if subnet.Contains(sourceIP) {
				ipAllowed = true
				break
			}
		}
		if !ipAllowed {
			slog.Warn("IP de origen no autorizada para este usuario", "user", user.Name, "ip", sourceIP.String())
			return false
		}
	}

	// Verificar Cooldowns
	actionDef, ok := cfg.Actions[payload.ActionID]
	if !ok {
		return false
	}

	effectiveCooldown := time.Duration(cfg.Security.DefaultActionCooldownSeconds) * time.Second
	if actionDef.CooldownSeconds != nil && *actionDef.CooldownSeconds >= 0 {
		effectiveCooldown = time.Duration(*actionDef.CooldownSeconds) * time.Second
	}

	cooldownKey := fmt.Sprintf("%s:%s", user.PublicKeyB64, payload.ActionID)

	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	if last, exists := s.actionCooldowns[cooldownKey]; exists {
		if time.Since(last) < effectiveCooldown {
			slog.Debug("Cooldown activo", "user", user.Name, "action", payload.ActionID)
			return false
		}
	}
	s.actionCooldowns[cooldownKey] = time.Now()
	return true
}

// startLimiterCleaner soporta recarga dinámica de intervalos mediante polling
func (s *Server) startLimiterCleaner(ctx context.Context) {
	for {
		// Leemos la config actual para saber cuánto dormir
		cfg := s.getConfig()
		sleepTime := time.Duration(cfg.Tuning.LimiterCleanupSeconds) * time.Second
		if sleepTime <= 0 {
			sleepTime = 3 * time.Minute
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(sleepTime):
			// Ejecutar limpieza
			cfg := s.getConfig() // Recargar config por si cambió eviction_age
			evictionAge := time.Duration(cfg.Tuning.LimiterEvictionAgeSeconds) * time.Second
			if evictionAge <= 0 {
				evictionAge = 5 * time.Minute
			}

			s.limitersMutex.Lock()
			purged := 0
			for ip, info := range s.ipLimiters {
				if time.Since(info.lastSeen) > evictionAge {
					delete(s.ipLimiters, ip)
					purged++
				}
			}
			s.limitersMutex.Unlock()
			if purged > 0 {
				slog.Debug("Limpieza rutinaria de IPs inactivas", "count", purged)
			}
		}
	}
}

// startCacheCleaner soporta recarga dinámica de intervalos
func (s *Server) startCacheCleaner(ctx context.Context) {
	for {
		cfg := s.getConfig()
		sleepTime := time.Duration(cfg.Tuning.CacheCleanupSeconds) * time.Second
		if sleepTime <= 0 {
			sleepTime = 60 * time.Second
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(sleepTime):
			s.cacheMutex.Lock()
			now := time.Now()
			// Limpiar firmas (Replay attack cache)
			purgeExpiredSignatures(s.signaturesCache, now)
			// Limpiar cooldowns antiguos (si > 24h, hardcoded safe limit)
			for key, t := range s.actionCooldowns {
				if time.Since(t) > 24*time.Hour {
					delete(s.actionCooldowns, key)
				}
			}
			s.cacheMutex.Unlock()
		}
	}
}

// validateTOTP verifica un código TOTP basado en el secreto Base32 dado.
// Implementación mínima de RFC 6238 con ventana de +/- 1 paso (30s).
func validateTOTP(secret, passcode string) bool {
	// Limpieza del secreto (eliminar espacios y convertir a mayúsculas)
	secret = strings.ToUpper(strings.ReplaceAll(secret, " ", ""))

	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		// Intentar con padding estándar si falla
		key, err = base32.StdEncoding.DecodeString(secret)
		if err != nil {
			slog.Error("Error decoding TOTP secret", "err", err)
			return false
		}
	}

	// Verificar ventana actual y adyacentes (+/- 1 intervalo de 30s)
	// Esto ayuda con relojes ligeramente desincronizados.
	currentInterval := time.Now().Unix() / 30
	for i := -1; i <= 1; i++ {
		if generateTOTP(key, currentInterval+int64(i)) == passcode {
			return true
		}
	}
	return false
}

// generateTOTP calcula el código HMAC-SHA1 para un intervalo dado.
func generateTOTP(key []byte, interval int64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(interval))

	h := hmac.New(sha1.New, key)
	h.Write(buf)
	sum := h.Sum(nil)

	offset := sum[len(sum)-1] & 0xf
	binCode := int64(((int(sum[offset]) & 0x7f) << 24) |
		((int(sum[offset+1]) & 0xff) << 16) |
		((int(sum[offset+2]) & 0xff) << 8) |
		(int(sum[offset+3]) & 0xff))

	code := int(binCode % 1000000)
	// Formatear a 6 dígitos con ceros a la izquierda
	return fmt.Sprintf("%06d", code)
}
