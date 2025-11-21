// ghostknockd es el demonio del servidor que escucha pasivamente los knocks.
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
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/your-org/ghostknock/internal/config"
	"github.com/your-org/ghostknock/internal/executor"
	"github.com/your-org/ghostknock/internal/listener"
	"github.com/your-org/ghostknock/internal/protocol"
	"golang.org/x/time/rate"
)

// version se establece en tiempo de compilación usando ldflags.
var version = "dev"

const (
	// Constantes que no se exponen en config.yaml por ser de ajuste interno
	cacheCleanupInterval   = 1 * time.Minute
	limiterCleanupInterval = 3 * time.Minute
	limiterEvictionAge     = 5 * time.Minute
	logFilePath            = "/var/log/ghostknockd.log"
)

type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type Server struct {
	config          *config.Config
	actionCooldowns map[string]time.Time
	cacheMutex      sync.RWMutex
	ipLimiters      map[string]*ipLimiter
	limitersMutex   sync.Mutex
}

func main() {
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

	tempLogger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		tempLogger.Error("Error crítico al cargar la configuración", "file", *configFile, "error", err)
		os.Exit(1)
	}

	// <<-- LÍNEA CORREGIDA -->>
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("FATAL: No se pudo abrir el archivo de log en %s: %v. ¿Ejecutaste con sudo?", logFilePath, err)
	}
	defer logFile.Close()

	var logLevel slog.Level
	switch cfg.Logging.LogLevel {
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

	slog.Info("Iniciando demonio GhostKnockd...")

	if cfg.Daemon.PIDFile != "" {
		pid := os.Getpid()
		pidStr := strconv.Itoa(pid)
		if err := os.WriteFile(cfg.Daemon.PIDFile, []byte(pidStr), 0644); err != nil {
			slog.Error("No se pudo escribir el archivo PID", "path", cfg.Daemon.PIDFile, "error", err)
			os.Exit(1)
		}
		slog.Info("Archivo PID creado", "path", cfg.Daemon.PIDFile, "pid", pid)

		defer func() {
			if err := os.Remove(cfg.Daemon.PIDFile); err != nil {
				slog.Warn("No se pudo eliminar el archivo PID al salir", "path", cfg.Daemon.PIDFile, "error", err)
			} else {
				slog.Info("Archivo PID eliminado", "path", cfg.Daemon.PIDFile)
			}
		}()
	}

	slog.Info(
		"Configuración cargada con éxito",
		"users_count", len(cfg.Users),
		"actions_count", len(cfg.Actions),
		"log_level", cfg.Logging.LogLevel,
	)

	server := &Server{
		config:          cfg,
		actionCooldowns: make(map[string]time.Time),
		ipLimiters:      make(map[string]*ipLimiter),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go server.startCacheCleaner()
	go server.startLimiterCleaner()

	packetsCh := make(chan listener.PacketInfo)
	go listener.Start(ctx, cfg.Listener, packetsCh)

	slog.Info("El listener está activo, procesando knocks y esperando señales...")

mainLoop:
	for {
		select {
		case packetInfo, ok := <-packetsCh:
			if !ok {
				slog.Info("El canal del listener se ha cerrado, finalizando.")
				break mainLoop
			}
			server.processKnock(packetInfo)
		case sig := <-signalChan:
			slog.Info("Señal de apagado recibida", "signal", sig.String())
			slog.Info("Iniciando cierre controlado...")
			cancel()
		}
	}

	slog.Info("Demonio GhostKnockd detenido limpiamente.")
}

func (s *Server) getLimiter(ip net.IP) *rate.Limiter {
	s.limitersMutex.Lock()
	defer s.limitersMutex.Unlock()
	ipStr := ip.String()
	limiter, exists := s.ipLimiters[ipStr]
	if !exists {
		// Usamos los valores de la configuración en lugar de constantes harcodeadas
		newLimiter := rate.NewLimiter(rate.Limit(s.config.Security.RateLimitPerSecond), s.config.Security.RateLimitBurst)
		s.ipLimiters[ipStr] = &ipLimiter{limiter: newLimiter, lastSeen: time.Now()}
		return newLimiter
	}
	limiter.lastSeen = time.Now()
	return limiter.limiter
}

func (s *Server) startLimiterCleaner() {
	ticker := time.NewTicker(limiterCleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.limitersMutex.Lock()
		purgedCount := 0
		for ip, limiterInfo := range s.ipLimiters {
			if time.Since(limiterInfo.lastSeen) > limiterEvictionAge {
				delete(s.ipLimiters, ip)
				purgedCount++
			}
		}
		s.limitersMutex.Unlock()
		if purgedCount > 0 {
			slog.Debug("Limpiados limitadores de IP inactivos", "count", purgedCount)
		}
	}
}

func (s *Server) startCacheCleaner() {
	ticker := time.NewTicker(cacheCleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.cacheMutex.Lock()
		purgedCount := 0
		// Usamos el valor de la configuración
		expirationDuration := time.Duration(s.config.Security.DefaultActionCooldownSeconds*2) * time.Second
		for key, lastSeen := range s.actionCooldowns {
			if time.Since(lastSeen) > expirationDuration {
				delete(s.actionCooldowns, key)
				purgedCount++
			}
		}
		s.cacheMutex.Unlock()
		if purgedCount > 0 {
			slog.Debug("Limpiadas entradas de cooldown antiguas", "count", purgedCount)
		}
	}
}

func (s *Server) processKnock(packetInfo listener.PacketInfo) {
	// 1. RATE LIMITING
	limiter := s.getLimiter(packetInfo.SourceIP)
	if !limiter.Allow() {
		slog.Warn("Paquete descartado", "reason", "rate_limit_exceeded", "source_ip", packetInfo.SourceIP.String())
		return
	}

	// 2. VALIDACIÓN DE ESTRUCTURA BÁSICA
	rawPayload := packetInfo.Payload
	if len(rawPayload) <= ed25519.SignatureSize {
		return
	}

	signature := rawPayload[:ed25519.SignatureSize]
	serializedPayload := rawPayload[ed25519.SignatureSize:]

	// 3. VERIFICACIÓN CRIPTOGRÁFICA TEMPRANA
	var authorizedUser *config.User
	for i := range s.config.Users {
		user := &s.config.Users[i]
		if ed25519.Verify(user.DecodedPublicKey, serializedPayload, signature) {
			authorizedUser = user
			break
		}
	}

	if authorizedUser == nil {
		slog.Warn("Paquete descartado", "reason", "invalid_signature", "source_ip", packetInfo.SourceIP.String())
		return
	}

	// 4. DESERIALIZACIÓN SEGURA (Solo si la firma es válida)
	payload, err := protocol.DeserializePayload(serializedPayload)
	if err != nil {
		slog.Warn("Paquete descartado", "reason", "payload_deserialization_failed", "source_ip", packetInfo.SourceIP.String(), "user", authorizedUser.Name, "error", err)
		return
	}

	// 5. VALIDACIONES DE NEGOCIO
	timestamp := time.Unix(0, payload.Timestamp)
	age := time.Since(timestamp)
	// Usamos el valor de la configuración
	replayWindow := time.Duration(s.config.Security.ReplayWindowSeconds) * time.Second
	if age < 0 || age > replayWindow {
		slog.Warn("Paquete descartado", "reason", "outside_replay_window", "source_ip", packetInfo.SourceIP.String(), "user", authorizedUser.Name, "age_seconds", age.Seconds())
		return
	}

	if !isActionAllowed(payload.ActionID, authorizedUser.AllowedActions) {
		slog.Warn("Paquete descartado", "reason", "unauthorized_action", "source_ip", packetInfo.SourceIP.String(), "user", authorizedUser.Name, "action_id", payload.ActionID)
		return
	}

	if len(authorizedUser.SourceCIDRs) > 0 {
		isIPAllowed := false
		for _, cidr := range authorizedUser.SourceCIDRs {
			if cidr.Contains(packetInfo.SourceIP) {
				isIPAllowed = true
				break
			}
		}
		if !isIPAllowed {
			slog.Warn("Paquete descartado",
				"reason", "unauthorized_source_ip",
				"user", authorizedUser.Name,
				"action_id", payload.ActionID,
				"source_ip", packetInfo.SourceIP.String(),
			)
			return
		}
	}

	// 6. LÓGICA DE COOLDOWN
	actionDef, ok := s.config.Actions[payload.ActionID]
	if !ok {
		slog.Error("Inconsistencia de configuración: la acción autorizada no existe", "action_id", payload.ActionID)
		return
	}

	// Usamos el valor de la configuración como valor por defecto
	effectiveCooldown := time.Duration(s.config.Security.DefaultActionCooldownSeconds) * time.Second
	if actionDef.CooldownSeconds >= 0 { // -1 significa usar el global, 0 significa sin cooldown
		effectiveCooldown = time.Duration(actionDef.CooldownSeconds) * time.Second
	}

	cooldownKey := fmt.Sprintf("%s:%s", authorizedUser.PublicKeyB64, payload.ActionID)

	if effectiveCooldown > 0 {
		s.cacheMutex.RLock()
		lastExecutionTime, onCooldown := s.actionCooldowns[cooldownKey]
		s.cacheMutex.RUnlock()

		if onCooldown {
			elapsed := time.Since(lastExecutionTime)
			if elapsed < effectiveCooldown {
				remaining := effectiveCooldown - elapsed
				slog.Warn(
					"Acción descartada",
					"reason", "cooldown_active",
					"user", authorizedUser.Name,
					"action_id", payload.ActionID,
					"remaining_seconds", remaining.Seconds(),
				)
				return
			}
		}
	}

	s.cacheMutex.Lock()
	s.actionCooldowns[cooldownKey] = time.Now()
	s.cacheMutex.Unlock()

	slog.Info("Knock válido recibido y autorizado",
		"user", authorizedUser.Name,
		"source_ip", packetInfo.SourceIP.String(),
		"action_id", payload.ActionID,
	)

	// 7. EJECUCIÓN CON PARÁMETROS
	if err := executor.Execute(actionDef, packetInfo.SourceIP, payload.Params); err != nil {
		slog.Error("Falló la ejecución de la acción", "action_id", payload.ActionID, "user", authorizedUser.Name, "error", err)
	}
}

func isActionAllowed(action string, allowedActions []string) bool {
	for _, a := range allowedActions {
		if a == action {
			return true
		}
	}
	return false
}
