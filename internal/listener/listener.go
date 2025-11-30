// El paquete listener se encarga de la captura de paquetes de bajo nivel.
package listener

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
	"github.com/your-org/ghostknock/internal/config"
)

// Constantes de Seguridad para la captura
const (
	// MaxPayloadSize: Límite estricto para el payload UDP.
	// Evita allocations gigantescas en memoria.
	MaxPayloadSize = 1024

	// SnapLen: Longitud de captura (Snapshot Length).
	SnapLen = 1518
)

// PacketInfo contiene el payload de un paquete y metadatos relevantes.
type PacketInfo struct {
	Payload  []byte
	SourceIP net.IP
}

// Start inicia la captura de tráfico.
// onDrop se ejecuta cuando el canal de procesamiento está lleno (Saturación).
func Start(ctx context.Context, listenerCfg config.Listener, packetsCh chan<- PacketInfo, onDrop func()) {
	defer close(packetsCh)

	slog.Info("Iniciando escucha pasiva (Non-Blocking Mode)", "interface", listenerCfg.Interface, "udp_port", listenerCfg.Port)

	const pcapTimeout = 300 * time.Millisecond

	// SEGURIDAD: Modo Promiscuo = false
	// Solo procesamos paquetes destinados a nuestra MAC/IP. Reduce superficie de ataque y CPU.
	handle, err := pcap.OpenLive(listenerCfg.Interface, SnapLen, false, pcapTimeout)
	if err != nil {
		slog.Error("Error al abrir la interfaz de captura", "interface", listenerCfg.Interface, "error", err)
		os.Exit(1)
	}
	defer handle.Close()

	var bpfFilter string
	if listenerCfg.ListenIP != "" {
		bpfFilter = fmt.Sprintf("dst host %s and udp and port %d", listenerCfg.ListenIP, listenerCfg.Port)
	} else {
		bpfFilter = fmt.Sprintf("udp and port %d", listenerCfg.Port)
	}

	if err := handle.SetBPFFilter(bpfFilter); err != nil {
		slog.Error("Error al establecer el filtro BPF", "filter", bpfFilter, "error", err)
		os.Exit(1)
	}
	slog.Info("Filtro BPF aplicado con éxito", "filter", bpfFilter)

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())

	slog.Info("Esperando paquetes...")
	for {
		select {
		case <-ctx.Done():
			slog.Info("Contexto cancelado, deteniendo el listener de paquetes.")
			return
		case packet := <-packetSource.Packets():
			if packet == nil {
				continue
			}

			// Validamos y extraemos la información
			if info, ok := extractPacketInfo(packet); ok {
				// SEGURIDAD: Envío NO BLOQUEANTE.
				// Si el canal está lleno (Workers saturados), descartamos el paquete inmediatamente.
				// Esto evita que el listener se cuelgue y provoque un fallo en cascada en la red.
				select {
				case packetsCh <- info:
					// Paquete encolado correctamente
				default:
					// Canal lleno. Invocamos callback para métricas y continuamos.
					// NO LOGUEAMOS aquí para evitar DoS por saturación de I/O en disco.
					if onDrop != nil {
						onDrop()
					}
				}
			}
		}
	}
}

// extractPacketInfo contiene la lógica pura de validación.
func extractPacketInfo(packet gopacket.Packet) (PacketInfo, bool) {
	// 1. Extraer Capa de Red (IP)
	netLayer := packet.NetworkLayer()
	if netLayer == nil {
		return PacketInfo{}, false
	}
	srcIP := netLayer.NetworkFlow().Src().Raw()

	// 2. Extraer Capa de Aplicación (Payload UDP)
	appLayer := packet.ApplicationLayer()
	if appLayer == nil {
		return PacketInfo{}, false
	}

	payload := appLayer.Payload()

	// 3. Validación estricta de tamaño (Hardening)
	if len(payload) > MaxPayloadSize {
		return PacketInfo{}, false
	}

	return PacketInfo{
		Payload:  payload,
		SourceIP: srcIP, // Casting implícito a net.IP
	}, true
}
