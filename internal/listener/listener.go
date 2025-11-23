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
	// Estructura esperada: Firma (64) + Nonce (24) + JSON overhead (~100) + Params (<500).
	// Cualquier cosa superior a 1KB es descartada para prevenir ataques de asignación de memoria.
	MaxPayloadSize = 1024

	// SnapLen: Longitud de captura (Snapshot Length).
	// Usamos 1518 (MTU estándar de Ethernet + headers) en lugar de un valor menor.
	// Esto garantiza que si llega un paquete grande, lo leemos entero para poder
	// medir su tamaño real y descartarlo, en lugar de leer una versión truncada.
	SnapLen = 1518
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
	
	// MODIFICACIÓN 1: Usamos SnapLen (1518) para captura completa de frames
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

			// 1. Extraer Capa de Red (IP)
			// Usamos guard clause: si no hay capa de red, saltamos al siguiente.
			netLayer := packet.NetworkLayer()
			if netLayer == nil {
				continue
			}
			srcIP := netLayer.NetworkFlow().Src().Raw()

			// 2. Extraer Capa de Aplicación (Payload UDP)
			appLayer := packet.ApplicationLayer()
			if appLayer == nil {
				continue
			}

			payload := appLayer.Payload()

			// MODIFICACIÓN 2: Validación estricta de tamaño (Hardening)
			// Descartamos paquetes gigantes antes de enviarlos a la lógica de negocio.
			if len(payload) > MaxPayloadSize {
				// CORRECCIÓN: Convertimos srcIP a net.IP explícitamente para poder usar .String()
				slog.Debug("Paquete descartado por exceso de tamaño", 
					"source_ip", net.IP(srcIP).String(), 
					"size", len(payload), 
					"limit", MaxPayloadSize)
				continue
			}

			// Si llegamos aquí, el paquete es válido estructuralmente (tiene IP, tiene datos y mide < 1KB)
			packetInfo := PacketInfo{
				Payload:  payload,
				SourceIP: srcIP, // Go acepta esto porque net.IP es un alias de []byte
			}
			select {
			case packetsCh <- packetInfo:
			case <-ctx.Done():
				return
			}
		}
	}
}
