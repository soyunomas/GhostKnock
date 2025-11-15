// El paquete listener se encarga de la captura de paquetes de bajo nivel.
package listener

import (
	"fmt"
	"log"
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
)

// PacketInfo contiene el payload de un paquete y metadatos relevantes.
type PacketInfo struct {
	Payload  []byte
	SourceIP net.IP
}

// Start captura paquetes en la interfaz y puerto especificados. Envía la información
// del paquete (PacketInfo) a través del canal 'packetsCh'.
func Start(ifaceName string, port int, packetsCh chan<- PacketInfo) {
	log.Printf("Iniciando escucha pasiva en la interfaz '%s' para el puerto UDP %d", ifaceName, port)

	handle, err := pcap.OpenLive(ifaceName, 1024, true, pcap.BlockForever)
	if err != nil {
		log.Fatalf("FATAL: Error al abrir la interfaz '%s': %v", ifaceName, err)
	}
	defer handle.Close()

	bpfFilter := fmt.Sprintf("udp and port %d", port)
	if err := handle.SetBPFFilter(bpfFilter); err != nil {
		log.Fatalf("FATAL: Error al establecer el filtro BPF ('%s'): %v", bpfFilter, err)
	}
	log.Printf("Filtro BPF aplicado: '%s'", bpfFilter)

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())

	log.Println("Esperando paquetes...")
	for packet := range packetSource.Packets() {
		var srcIP net.IP
		// Extraemos la capa de red para obtener la IP de origen.
		// Este enfoque es agnóstico a IPv4/IPv6.
		if netLayer := packet.NetworkLayer(); netLayer != nil {
			srcIP = netLayer.NetworkFlow().Src().Raw()
		}

		// Solo procesamos el paquete si tiene una capa de aplicación (payload UDP)
		// y hemos podido determinar su IP de origen.
		if appLayer := packet.ApplicationLayer(); appLayer != nil && srcIP != nil {
			packetInfo := PacketInfo{
				Payload:  appLayer.Payload(),
				SourceIP: srcIP,
			}
			// Enviamos la estructura completa al procesador.
			packetsCh <- packetInfo
		}
	}
}
