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
func Start(ctx context.Context, listenerCfg config.Listener, packetsCh chan<- PacketInfo) {
	defer close(packetsCh)

	slog.Info("Iniciando escucha pasiva", "interface", listenerCfg.Interface, "udp_port", listenerCfg.Port)

	const pcapTimeout = 300 * time.Millisecond

	handle, err := pcap.OpenLive(listenerCfg.Interface, SnapLen, true, pcapTimeout)
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

			// Llamamos a la función pura que valida y extrae
			if info, ok := extractPacketInfo(packet); ok {
				select {
				case packetsCh <- info:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// extractPacketInfo contiene la lógica pura de validación.
// Es pública (o accesible internamente) para facilitar el Fuzzing.
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
		// En contexto de Fuzzing o High Load, quizás no queramos loguear cada fallo
		// para no saturar I/O, pero en producción normal es útil en debug.
		// Para el Fuzzing, si esto no hace panic, es un éxito.
		return PacketInfo{}, false
	}

	return PacketInfo{
		Payload:  payload,
		SourceIP: srcIP, // Casting implícito a net.IP
	}, true
}
