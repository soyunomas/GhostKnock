// ghostknockd es el demonio del servidor blindado que escucha pasivamente los knocks.
package main

import (
	"context"
	"crypto/ed25519"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/your-org/ghostknock/internal/config"
	"github.com/your-org/ghostknock/internal/executor"
	"github.com/your-org/ghostknock/internal/listener"
	"github.com/your-org/ghostknock/internal/protocol"
	"golang.org/x/time/rate"
)

// version se establece en tiempo de compilación.
var version = "dev"

const (
	// Intervalos de limpieza
	cacheCleanupInterval   = 1 * time.Minute
	limiterCleanupInterval = 3 * time.Minute
	limiterEvictionAge     = 5 * time.Minute
	logFilePath            = "/var/log/ghostknockd.log"

	// SEGURIDAD: Límite estricto de memoria para rastreo de IPs (Anti-OOM)
	maxLimitersEntries = 20000
	// SEGURIDAD: Cantidad de entradas a purgar si nos llenamos (10%)
	evictionBatchSize = 2000

	// SEGURIDAD: Buffer pequeño para forzar "Fail Fast" y evitar latencia (Bufferbloat)
	packetChannelBuffer = 100
)

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

	// 2. Configuración de Logging
	setupLogging("info") // Inicio básico
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		slog.Error("Error crítico al cargar la configuración inicial", "file", *configFile, "error", err)
		os.Exit(1)
	}
	// Reconfigurar logging con el nivel del archivo
	setupLogging(cfg.Logging.LogLevel)

	slog.Info("Iniciando demonio GhostKnockd (v2.1 Hot-Reload Ready)...")

	// 3. Carga de Claves
	serverPrivKeyBytes, err := os.ReadFile(cfg.ServerPrivateKeyPath)
	if err != nil {
		slog.Error("Error crítico al leer la clave privada del servidor", "path", cfg.ServerPrivateKeyPath, "error", err)
		os.Exit(1)
	}
	if len(serverPrivKeyBytes) != ed25519.PrivateKeySize {
		slog.Error("El archivo de clave privada del servidor tiene un tamaño incorrecto", "path", cfg.ServerPrivateKeyPath)
		os.Exit(1)
	}

	// 4. Gestión del PID File
	if cfg.Daemon.PIDFile != "" {
		pid := os.Getpid()
		pidStr := strconv.Itoa(pid)
		if err := os.WriteFile(cfg.Daemon.PIDFile, []byte(pidStr), 0644); err != nil {
			slog.Error("No se pudo escribir el archivo PID", "path", cfg.Daemon.PIDFile, "error", err)
			os.Exit(1)
		}
		defer os.Remove(cfg.Daemon.PIDFile)
	}

	// 5. Inicialización del Servidor
	server := &Server{
		config:           cfg,
		configFile:       *configFile,
		serverPrivateKey: ed25519.PrivateKey(serverPrivKeyBytes),
		actionCooldowns:  make(map[string]time.Time),
		signaturesCache:  make(map[string]time.Time),
		ipLimiters:       make(map[string]*ipLimiter),
		executionSem:     make(chan struct{}, 10),
	}

	ctx, cancel := context.WithCancel(context.Background())

	// --- GESTIÓN DE SEÑALES (Reload + Shutdown) ---
	signalChan := make(chan os.Signal, 1)
	// Añadimos SIGHUP a las señales escuchadas
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Tareas de limpieza en segundo plano
	go server.startCacheCleaner()
	go server.startLimiterCleaner()

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
	packetsCh := make(chan listener.PacketInfo, packetChannelBuffer)

	// Callback de saturación
	onDrop := func() {
		atomic.AddUint64(&server.droppedPackets, 1)
	}

	// Listener asíncrono
	go listener.Start(ctx, cfg.Listener, packetsCh, onDrop)

	// --- WORKER POOL (Crypto) ---
	numWorkers := runtime.NumCPU()
	slog.Info("Iniciando Worker Pool", "workers", numWorkers)

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

	// 6. BUCLE PRINCIPAL DE SEÑALES
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

// setupLogging configura el logger global
func setupLogging(levelStr string) {
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// Fallback a Stdout si falla el archivo
		log.Printf("ERROR: No se pudo abrir log file: %v", err)
		return
	}

	var logLevel slog.Level
	switch levelStr {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	handlerOpts := &slog.HandlerOptions{Level: logLevel}
	logger := slog.New(slog.NewTextHandler(logFile, handlerOpts))
	slog.SetDefault(logger)
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
	if newCfg.Listener.Port != s.config.Listener.Port || newCfg.Listener.Interface != s.config.Listener.Interface {
		slog.Warn("RELOAD PARCIAL: Se han detectado cambios en Interface/Puerto. Estos cambios NO se aplican en caliente. Por favor, reinicie el servicio (systemctl restart) para aplicar cambios de red.")
	}

	// Aplicar nueva configuración
	s.config = newCfg
	
	// Actualizar nivel de log en caliente
	setupLogging(newCfg.Logging.LogLevel)
	
	slog.Info("Configuración recargada exitosamente.", 
		"users_count", len(newCfg.Users), 
		"actions_count", len(newCfg.Actions))
}

// getConfig obtiene una instantánea segura de la configuración actual
func (s *Server) getConfig() *config.Config {
	s.configMutex.RLock()
	defer s.configMutex.RUnlock()
	return s.config
}

// getLimiter implementa Rate Limiting con "Purga Parcial" (Anti-OOM + Anti-Evasión)
func (s *Server) getLimiter(ip net.IP, limit float64, burst int) *rate.Limiter {
	s.limitersMutex.Lock()
	defer s.limitersMutex.Unlock()

	ipStr := ip.String()

	if entry, exists := s.ipLimiters[ipStr]; exists {
		entry.lastSeen = time.Now()
		// Opcional: Podríamos actualizar el límite aquí si cambió en config, 
		// pero rate.Limiter no facilita el cambio dinámico barato.
		return entry.limiter
	}

	if len(s.ipLimiters) >= maxLimitersEntries {
		deleted := 0
		for k := range s.ipLimiters {
			delete(s.ipLimiters, k)
			deleted++
			if deleted >= evictionBatchSize {
				break
			}
		}
		slog.Warn("Tabla de IPs llena: purga parcial ejecutada", "purgados", deleted)
	}

	newLimiter := rate.NewLimiter(rate.Limit(limit), burst)
	s.ipLimiters[ipStr] = &ipLimiter{limiter: newLimiter, lastSeen: time.Now()}
	return newLimiter
}

// processKnock maneja la lógica de negocio
func (s *Server) processKnock(packetInfo listener.PacketInfo) {
	atomic.AddUint64(&s.processedPackets, 1)

	// 1. Obtener configuración actual de forma segura (Thread-Safe)
	currentConfig := s.getConfig()

	// 1.5. LISTA NEGRA: Verificación Ultrarrápida (Short-Circuit)
	// Comprobamos esto ANTES del Rate Limit, la caché y la criptografía.
	if currentConfig.IsIPDenied(packetInfo.SourceIP) {
		// Drop silencioso inmediato. Ahorra CPU.
		return
	}

	// 2. Rate Limit IP (Anti-DoS ligero)
	// Usamos los valores de la config actual
	limiter := s.getLimiter(packetInfo.SourceIP, currentConfig.Security.RateLimitPerSecond, currentConfig.Security.RateLimitBurst)
	if !limiter.Allow() {
		return // Descarte silencioso
	}

	// 3. Validación estructural básica
	if len(packetInfo.Payload) < ed25519.SignatureSize {
		return
	}

	sigBytes := packetInfo.Payload[:ed25519.SignatureSize]

	// 4. CACHÉ CHECK 1 (Read Lock - Barato)
	s.cacheMutex.RLock()
	_, known := s.signaturesCache[string(sigBytes)]
	s.cacheMutex.RUnlock()

	if known {
		slog.Debug("Replay detectado (Fast-Path)", "src", packetInfo.SourceIP)
		return
	}

	// 5. VALIDACIÓN CRIPTOGRÁFICA (CPU Intensivo)
	var authorizedUser *config.User
	var payload *protocol.Payload
	var err error

	// Iteramos sobre los usuarios de la configuración (snapshot seguro)
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

	// 6. CACHÉ CHECK 2 + STORE (Write Lock - Atómico)
	ttl := time.Duration(currentConfig.Security.ReplayWindowSeconds+1) * time.Second
	expiration := time.Now().Add(ttl)

	s.cacheMutex.Lock()
	if _, exists := s.signaturesCache[string(sigBytes)]; exists {
		s.cacheMutex.Unlock()
		slog.Warn("Replay detectado (Race-Win)", "user", authorizedUser.Name)
		return
	}
	s.signaturesCache[string(sigBytes)] = expiration
	s.cacheMutex.Unlock()

	// 7. Validación Timestamp del Payload
	ts := time.Unix(0, payload.Timestamp)
	if time.Since(ts) > time.Duration(currentConfig.Security.ReplayWindowSeconds)*time.Second {
		slog.Warn("Paquete expirado (Timestamp fuera de ventana)", "user", authorizedUser.Name)
		return
	}

	// 8. Autorización de Acción y Cooldowns
	// Pasamos la configuración actual para asegurar consistencia
	if !s.checkActionAuthAndCooldown(authorizedUser, payload, packetInfo.SourceIP, currentConfig) {
		return
	}

	slog.Info("Knock autorizado, solicitando slot de ejecución", "user", authorizedUser.Name, "action", payload.ActionID)

	// 9. EJECUCIÓN SEGURA
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

			// Buscamos la acción en la config que teníamos cuando validamos
			actionDef := currentConfig.Actions[payload.ActionID]

			if actionDef.TimeoutSeconds <= 0 {
				actionDef.TimeoutSeconds = 30
			}

			if err := executor.Execute(actionDef, packetInfo.SourceIP, payload.Params); err != nil {
				slog.Error("Error en ejecución", "action", payload.ActionID, "error", err)
			}
		}()
	default:
		slog.Error("Ejecución rechazada: Límite de procesos concurrentes alcanzado", "user", authorizedUser.Name)
	}
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

func (s *Server) startLimiterCleaner() {
	ticker := time.NewTicker(limiterCleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.limitersMutex.Lock()
		purged := 0
		for ip, info := range s.ipLimiters {
			if time.Since(info.lastSeen) > limiterEvictionAge {
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

func (s *Server) startCacheCleaner() {
	ticker := time.NewTicker(cacheCleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.cacheMutex.Lock()
		now := time.Now()
		for sig, exp := range s.signaturesCache {
			if now.After(exp) {
				delete(s.signaturesCache, sig)
			}
		}
		for key, t := range s.actionCooldowns {
			if time.Since(t) > 24*time.Hour {
				delete(s.actionCooldowns, key)
			}
		}
		s.cacheMutex.Unlock()
	}
}
