package listener

import "net"

// Constantes de Seguridad para la captura
const (
	// MaxPayloadSize: Límite estricto para el payload UDP.
	MaxPayloadSize = 1024

	// SnapLen: Longitud de captura (Snapshot Length).
	SnapLen = 1518
)

// PacketInfo contiene el payload de un paquete y metadatos relevantes.
// Esta estructura es compartida entre todas las plataformas.
type PacketInfo struct {
	Payload  []byte
	SourceIP net.IP
}
