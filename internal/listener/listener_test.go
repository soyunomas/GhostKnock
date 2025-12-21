//go:build linux

package listener

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
)

// constructPacket es un helper para fabricar paquetes raw para testing sin usar gopacket
func constructPacket(ethType uint16, vlanID int, proto uint8, srcIP, dstIP net.IP, dstPort int, payload []byte) []byte {
	buf := new(bytes.Buffer)

	// --- ETHERNET ---
	// DstMAC (6) + SrcMAC (6)
	buf.Write([]byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55})
	buf.Write([]byte{0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB})

	// VLAN Tagging (802.1Q) opcional
	if vlanID > 0 {
		binary.Write(buf, binary.BigEndian, uint16(0x8100)) // EtherType VLAN
		// PRI + CFI + VLAN ID (simplificado)
		binary.Write(buf, binary.BigEndian, uint16(vlanID))
	}

	// EtherType real
	binary.Write(buf, binary.BigEndian, ethType)

	// Si no es IPv4, terminamos aquí para probar descarte por EtherType
	if ethType != 0x0800 {
		return buf.Bytes()
	}

	// --- IPv4 HEADER (Mínimo 20 bytes) ---
	// Byte 0: Version (4) + IHL (5 words = 20 bytes) -> 0x45
	buf.WriteByte(0x45)
	buf.WriteByte(0x00) // TOS

	// Total Length (IP Header + UDP Header + Payload)
	ipTotalLen := 20 + 8 + len(payload)
	binary.Write(buf, binary.BigEndian, uint16(ipTotalLen))

	buf.Write([]byte{0x00, 0x01}) // ID
	buf.Write([]byte{0x00, 0x00}) // Flags/Frag
	buf.WriteByte(64)             // TTL
	buf.WriteByte(proto)          // Protocolo
	buf.Write([]byte{0x00, 0x00}) // Checksum (ignorado por nuestro parser)
	buf.Write(srcIP.To4())
	buf.Write(dstIP.To4())

	// Si no es UDP, terminamos aquí para probar descarte por Proto
	if proto != 17 {
		return buf.Bytes()
	}

	// --- UDP HEADER (8 bytes) ---
	binary.Write(buf, binary.BigEndian, uint16(12345))   // SrcPort
	binary.Write(buf, binary.BigEndian, uint16(dstPort)) // DstPort

	// UDP Length (Header + Payload)
	udpLen := 8 + len(payload)
	binary.Write(buf, binary.BigEndian, uint16(udpLen))

	buf.Write([]byte{0x00, 0x00}) // Checksum

	// --- PAYLOAD ---
	buf.Write(payload)

	return buf.Bytes()
}

func TestParsePacket(t *testing.T) {
	targetPort := 3001
	payload := []byte("knock_knock_neo")
	srcIP := net.IP{192, 168, 1, 100}
	dstIP := net.IP{10, 0, 0, 1}

	tests := []struct {
		name       string
		packetData []byte
		wantValid  bool
	}{
		{
			name:       "Valid IPv4 UDP Packet",
			packetData: constructPacket(0x0800, 0, 17, srcIP, dstIP, targetPort, payload),
			wantValid:  true,
		},
		{
			name:       "Valid VLAN Tagged Packet",
			packetData: constructPacket(0x0800, 10, 17, srcIP, dstIP, targetPort, payload),
			wantValid:  true,
		},
		{
			name:       "Invalid EtherType (ARP)",
			packetData: constructPacket(0x0806, 0, 0, nil, nil, 0, nil),
			wantValid:  false,
		},
		{
			name:       "Invalid Protocol (TCP)",
			packetData: constructPacket(0x0800, 0, 6, srcIP, dstIP, targetPort, payload),
			wantValid:  false,
		},
		{
			name:       "Wrong Destination Port",
			packetData: constructPacket(0x0800, 0, 17, srcIP, dstIP, 9999, payload),
			wantValid:  false,
		},
		{
			name:       "Empty Payload",
			packetData: constructPacket(0x0800, 0, 17, srcIP, dstIP, targetPort, []byte{}),
			wantValid:  false,
		},
		{
			name:       "Truncated Packet (Header Only)",
			packetData: constructPacket(0x0800, 0, 17, srcIP, dstIP, targetPort, payload)[:30], // Cortamos a la mitad
			wantValid:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, info := parsePacket(tt.packetData, targetPort)
			if valid != tt.wantValid {
				t.Errorf("parsePacket() valid = %v, want %v", valid, tt.wantValid)
			}
			if valid {
				if !bytes.Equal(info.Payload, payload) {
					t.Errorf("Payload mismatch. Got %s, want %s", info.Payload, payload)
				}
				if !info.SourceIP.Equal(srcIP.To4()) {
					t.Errorf("Source IP mismatch. Got %v, want %v", info.SourceIP, srcIP)
				}
			}
		})
	}
}
