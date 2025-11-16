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
	"strconv" // <<-- NUEVA IMPORTACIÓN
	"sync"
	"syscall"
	"time"

	"github.com/your-org/ghostknock/internal/config"
	"github.com/your-org/ghostknock/internal/executor"
	"github.com/your-org/ghostknock/internal/listener"
	"github.com/your-org/ghostknock/internal/protocol"
	"golang.org/x/time/rate"
)

const (
	replayWindowSeconds      = 5
	actionCooldownSeconds    = 15
	cacheCleanupInterval     = 1 * time.Minute
	rateLimitEventsPerSecond = 1.0
	rateLimitBurst           = 3
	limiterCleanupInterval   = 3 * time.Minute
	limiterEvictionAge       = 5 * time.Minute
	logFilePath              = "/var/log/ghostknockd.log"
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
	configFile := flag.String("config", "config.yaml", "Ruta al archivo de configuración YAML")
	flag.Parse()

	// Cargar la configuración ANTES de inicializar el logger final.
	tempLogger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		tempLogger.Error("Error crítico al cargar la configuración", "file", *configFile, "error", err)
		os.Exit(1)
	}

	// --- INICIALIZACIÓN DEL LOGGER ESTRUCTURADO A ARCHIVO ---
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

	// --- GESTIÓN DEL ARCHIVO PID ---
	if cfg.Daemon.PIDFile != "" {
		pid := os.Getpid()
		pidStr := strconv.Itoa(pid)
		if err := os.WriteFile(cfg.Daemon.PIDFile, []byte(pidStr), 0644); err != nil {
			slog.Error("No se pudo escribir el archivo PID", "path", cfg.Daemon.PIDFile, "error", err)
			os.Exit(1)
		}
		slog.Info("Archivo PID creado", "path", cfg.Daemon.PIDFile, "pid", pid)

		// Usamos defer para asegurar que el archivo PID se elimina al salir de main.
		defer func() {
			if err := os.Remove(cfg.Daemon.PIDFile); err != nil {
				slog.Warn("No se pudo eliminar el archivo PID al salir", "path", cfg.Daemon.PIDFile, "error", err)
			} else {
				slog.Info("Archivo PID eliminado", "path", cfg.Daemon.PIDFile)
			}
		}()
	}
	// ------------------------------------

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

	// --- CONFIGURACIÓN DEL GRACEFUL SHUTDOWN ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	// ---------------------------------------------

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
		newLimiter := rate.NewLimiter(rateLimitEventsPerSecond, rateLimitBurst)
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
		expirationDuration := time.Duration(actionCooldownSeconds) * time.Second
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
	limiter := s.getLimiter(packetInfo.SourceIP)
	if !limiter.Allow() {
		slog.Warn("Paquete descartado", "reason", "rate_limit_exceeded", "source_ip", packetInfo.SourceIP.String())
		return
	}

	rawPayload := packetInfo.Payload
	if len(rawPayload) <= ed25519.SignatureSize {
		return
	}

	signature := rawPayload[:ed25519.SignatureSize]
	serializedPayload := rawPayload[ed25519.SignatureSize:]

	payload, err := protocol.DeserializePayload(serializedPayload)
	if err != nil {
		slog.Warn("Paquete descartado", "reason", "payload_deserialization_failed", "source_ip", packetInfo.SourceIP.String(), "error", err)
		return
	}

	timestamp := time.Unix(0, payload.Timestamp)
	age := time.Since(timestamp)
	if age < 0 || age > (replayWindowSeconds*time.Second) {
		slog.Warn("Paquete descartado", "reason", "outside_replay_window", "source_ip", packetInfo.SourceIP.String(), "age_seconds", age.Seconds())
		return
	}

	var authorizedUser *config.User
	var isSignatureValid bool
	for _, user := range s.config.Users {
		if ed25519.Verify(user.DecodedPublicKey, serializedPayload, signature) {
			isSignatureValid = true
			if isActionAllowed(payload.ActionID, user.AllowedActions) {
				u := user
				authorizedUser = &u
				break
			}
		}
	}

	if !isSignatureValid {
		slog.Warn("Paquete descartado", "reason", "invalid_signature", "source_ip", packetInfo.SourceIP.String())
		return
	}
	if authorizedUser == nil {
		slog.Warn("Paquete descartado", "reason", "unauthorized_action", "source_ip", packetInfo.SourceIP.String(), "action_id", payload.ActionID)
		return
	}

	cooldownKey := fmt.Sprintf("%s:%s", authorizedUser.PublicKeyB64, payload.ActionID)
	s.cacheMutex.RLock()
	lastExecutionTime, onCooldown := s.actionCooldowns[cooldownKey]
	s.cacheMutex.RUnlock()

	if onCooldown {
		elapsed := time.Since(lastExecutionTime)
		if elapsed < (time.Duration(actionCooldownSeconds) * time.Second) {
			remaining := (time.Duration(actionCooldownSeconds) * time.Second) - elapsed
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

	s.cacheMutex.Lock()
	s.actionCooldowns[cooldownKey] = time.Now()
	s.cacheMutex.Unlock()

	slog.Info("Knock válido recibido y autorizado",
		"user", authorizedUser.Name,
		"source_ip", packetInfo.SourceIP.String(),
		"action_id", payload.ActionID,
	)

	actionDef, ok := s.config.Actions[payload.ActionID]
	if !ok {
		slog.Error("Inconsistencia de configuración: la acción autorizada no existe", "action_id", payload.ActionID)
		return
	}
	if err := executor.Execute(actionDef, packetInfo.SourceIP); err != nil {
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
