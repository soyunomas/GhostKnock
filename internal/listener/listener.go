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
	"github.com/your-org/ghostknock/internal/config" // <<-- NUEVA IMPORTACIÓN
)

// PacketInfo contiene el payload de un paquete y metadatos relevantes.
type PacketInfo struct {
	Payload  []byte
	SourceIP net.IP
}

// Start ahora acepta una struct config.Listener para mayor flexibilidad.
func Start(ctx context.Context, listenerCfg config.Listener, packetsCh chan<- PacketInfo) {
	defer close(packetsCh)

	slog.Info("Iniciando escucha pasiva", "interface", listenerCfg.Interface, "udp_port", listenerCfg.Port)

	const pcapTimeout = 300 * time.Millisecond
	handle, err := pcap.OpenLive(listenerCfg.Interface, 1024, true, pcapTimeout)
	if err != nil {
		slog.Error("Error al abrir la interfaz de captura", "interface", listenerCfg.Interface, "error", err)
		os.Exit(1)
	}
	defer handle.Close()

	// <<-- LÓGICA DE FILTRADO DINÁMICO
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

			var srcIP net.IP
			if netLayer := packet.NetworkLayer(); netLayer != nil {
				srcIP = netLayer.NetworkFlow().Src().Raw()
			}

			if appLayer := packet.ApplicationLayer(); appLayer != nil && srcIP != nil {
				packetInfo := PacketInfo{
					Payload:  appLayer.Payload(),
					SourceIP: srcIP,
				}
				select {
				case packetsCh <- packetInfo:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}
