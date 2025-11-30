package listener

import (
	"net" // <<-- Faltaba este import
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// FuzzExtractPacketInfo bombardea la función de extracción con datos aleatorios.
func FuzzExtractPacketInfo(f *testing.F) {
	// 1. Añadimos casos semilla (corpus)
	f.Add([]byte("payload_corto_valido"))
	f.Add(make([]byte, 2000)) // Payload que excede el límite

	f.Fuzz(func(t *testing.T, payloadData []byte) {
		// Construimos un paquete falso usando gopacket
		// Simulamos un paquete UDP sobre IPv4
		buf := gopacket.NewSerializeBuffer()
		opts := gopacket.SerializeOptions{}
		
		eth := &layers.Ethernet{
			SrcMAC:       net.HardwareAddr{0xff, 0xaa, 0xfa, 0xaa, 0xff, 0xaa},
			DstMAC:       net.HardwareAddr{0xbd, 0xbd, 0xbd, 0xbd, 0xbd, 0xbd},
			EthernetType: layers.EthernetTypeIPv4,
		}
		ip := &layers.IPv4{
			SrcIP:    net.IP{192, 168, 1, 1},
			DstIP:    net.IP{192, 168, 1, 2},
			Protocol: layers.IPProtocolUDP,
			Version:  4,
		}
		udp := &layers.UDP{
			SrcPort: 1234,
			DstPort: 3001,
		}
		
		// El Fuzzer controla el contenido de 'payloadData'
		err := gopacket.SerializeLayers(buf, opts, eth, ip, udp, gopacket.Payload(payloadData))
		if err != nil {
			return
		}

		// Parseamos el paquete falso como lo haría el listener real
		packet := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)

		// EJECUTAMOS LA FUNCIÓN BAJO PRUEBA
		info, ok := extractPacketInfo(packet)

		// Verificaciones lógicas post-ejecución
		if ok {
			if len(info.Payload) > MaxPayloadSize {
				t.Errorf("Seguridad rota: Se aceptó un payload de %d bytes, mayor al límite de %d", len(info.Payload), MaxPayloadSize)
			}
		}
	})
}
