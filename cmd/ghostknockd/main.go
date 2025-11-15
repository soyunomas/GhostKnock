// ghostknockd es el demonio del servidor que escucha pasivamente los knocks.
package main

import (
	"crypto/ed25519"
//	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/your-org/ghostknock/internal/config"
	"github.com/your-org/ghostknock/internal/executor"
	"github.com/your-org/ghostknock/internal/listener"
	"github.com/your-org/ghostknock/internal/protocol"
)

const (
	replayWindowSeconds   = 5
	// CAMBIO: La caché ahora es para el cooldown de acciones.
	actionCooldownSeconds  = 15                // Cooldown de 15 segundos por usuario/acción.
	cacheCleanupInterval   = 1 * time.Minute   // Frecuencia de limpieza de la caché.
)

// Server encapsula todo el estado del servidor.
type Server struct {
	config          *config.Config
	// RENOMBRADO: De seenSignatures a actionCooldowns para reflejar el nuevo propósito.
	actionCooldowns map[string]time.Time
	cacheMutex      sync.RWMutex
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	configFile := flag.String("config", "config.yaml", "Ruta al archivo de configuración YAML")
	flag.Parse()

	log.Println("Iniciando demonio GhostKnockd...")

	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("FATAL: Error al cargar la configuración: %v", err)
	}
	log.Printf("Configuración cargada con éxito. %d usuario(s) y %d accione(s) definidas.", len(cfg.Users), len(cfg.Actions))

	server := &Server{
		config:          cfg,
		actionCooldowns: make(map[string]time.Time), // CAMBIO: Nombre del mapa actualizado
	}

	go server.startCacheCleaner()

	packetsCh := make(chan listener.PacketInfo)
	go listener.Start(cfg.Listener.Interface, cfg.Listener.Port, packetsCh)

	log.Println("El listener está activo. Procesando knocks recibidos...")
	for packetInfo := range packetsCh {
		server.processKnock(packetInfo)
	}
}

// startCacheCleaner ahora limpia el mapa de cooldowns. La lógica es la misma.
func (s *Server) startCacheCleaner() {
	ticker := time.NewTicker(cacheCleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		s.cacheMutex.Lock()
		
		purgedCount := 0
		// La duración de expiración ahora es el cooldown.
		expirationDuration := time.Duration(actionCooldownSeconds) * time.Second
		for key, lastSeen := range s.actionCooldowns {
			if time.Since(lastSeen) > expirationDuration {
				delete(s.actionCooldowns, key)
				purgedCount++
			}
		}
		
		s.cacheMutex.Unlock()
		if purgedCount > 0 {
			log.Printf("[CacheCleaner] Limpiadas %d entradas de cooldown antiguas.", purgedCount)
		}
	}
}

func (s *Server) processKnock(packetInfo listener.PacketInfo) {
	rawPayload := packetInfo.Payload
	if len(rawPayload) <= ed25519.SignatureSize {
		// ... (código sin cambios)
		return
	}

	signature := rawPayload[:ed25519.SignatureSize]
	serializedPayload := rawPayload[ed25519.SignatureSize:]

	payload, err := protocol.DeserializePayload(serializedPayload)
	if err != nil {
		// ... (código sin cambios)
		return
	}

	timestamp := time.Unix(0, payload.Timestamp)
	age := time.Since(timestamp)
	if age < 0 || age > (replayWindowSeconds*time.Second) {
		log.Printf("Paquete descartado: fuera de la ventana de tiempo anti-replay (edad: %s).", age)
		return
	}

	// --- LÓGICA DE AUTENTICACIÓN Y AUTORIZACIÓN (PRIMERO) ---
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
		log.Printf("Paquete descartado de %s: firma inválida.", packetInfo.SourceIP)
		return
	}
	if authorizedUser == nil {
		log.Printf("Paquete descartado de %s: firma válida, pero la acción '%s' no está autorizada.", packetInfo.SourceIP, payload.ActionID)
		return
	}

	// --- NUEVA LÓGICA DE COOLDOWN (DESPUÉS DE AUTORIZAR) ---
	// 1. Crear una clave única para la combinación usuario+acción.
	// La clave pública del usuario es su identificador único.
	cooldownKey := fmt.Sprintf("%s:%s", authorizedUser.PublicKeyB64, payload.ActionID)

	// 2. Comprobar si esta acción está actualmente en cooldown para este usuario.
	s.cacheMutex.RLock()
	lastExecutionTime, onCooldown := s.actionCooldowns[cooldownKey]
	s.cacheMutex.RUnlock()

	if onCooldown {
		// Calcular cuánto tiempo queda de cooldown.
		elapsed := time.Since(lastExecutionTime)
		if elapsed < (time.Duration(actionCooldownSeconds) * time.Second) {
			remaining := (time.Duration(actionCooldownSeconds) * time.Second) - elapsed
			log.Printf("[COOLDOWN ACTIVO] Acción '%s' para '%s' descartada. Inténtelo de nuevo en %s.",
				payload.ActionID, authorizedUser.Name, remaining.Round(time.Second))
			return // ¡Acción repetida bloqueada!
		}
	}

	// 3. Registrar la ejecución ANTES de ejecutar la acción.
	s.cacheMutex.Lock()
	s.actionCooldowns[cooldownKey] = time.Now()
	s.cacheMutex.Unlock()
	// ---------------------------------------------------------

	log.Printf("✅ [ÉXITO] Knock válido de '%s' desde %s. Acción autorizada: '%s'", authorizedUser.Name, packetInfo.SourceIP, payload.ActionID)

	actionDef, ok := s.config.Actions[payload.ActionID]
	if !ok {
		// ... (código sin cambios)
		return
	}
	if err := executor.Execute(actionDef, packetInfo.SourceIP); err != nil {
		// ... (código sin cambios)
	}
}

// isActionAllowed (sin cambios)
func isActionAllowed(action string, allowedActions []string) bool {
	for _, a := range allowedActions {
		if a == action {
			return true
		}
	}
	return false
}
