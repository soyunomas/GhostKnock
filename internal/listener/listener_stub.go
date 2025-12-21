//go:build !linux

package listener

import (
	"context"
	"log/slog"
	"os"

	"github.com/soyunomas/ghostknock/internal/config"
)

// Start es un stub para sistemas no-Linux.
func Start(ctx context.Context, listenerCfg config.Listener, pcapTimeoutMs int, packetsCh chan<- PacketInfo, onDrop func()) {
	// Cerramos el canal inmediatamente para no bloquear a nadie que espere leer.
	defer close(packetsCh)

	msg := "FATAL: El modo servidor de GhostKnock solo está soportado nativamente en Linux (requiere AF_PACKET/libpcap)."
	slog.Error(msg)

	// En Windows/Mac, esto debería detener la aplicación inmediatamente si intentan correr el servidor.
	// El cliente (ghostknock-cli) sí funcionará porque no usa este paquete 'listener'.
	os.Exit(1)
}

// Stub para pruebas si fuera necesario, devuelve siempre falso.
func extractPacketInfo(packet interface{}) (PacketInfo, bool) {
	return PacketInfo{}, false
}
