//go:build linux

package listener

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"golang.org/x/net/bpf"
)

// constructPacket fabrica el datagrama IPv4 que AF_PACKET/SOCK_DGRAM entrega.
func constructPacket(proto uint8, srcIP, dstIP net.IP, srcPort, dstPort int, payload []byte) []byte {
	buf := new(bytes.Buffer)

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
	binary.Write(buf, binary.BigEndian, uint16(srcPort)) // SrcPort
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
	fragment := constructPacket(17, srcIP, dstIP, 12345, targetPort, payload)
	binary.BigEndian.PutUint16(fragment[6:8], 0x2000)

	tests := []struct {
		name       string
		packetData []byte
		targetIP   net.IP
		wantValid  bool
	}{
		{
			name:       "Valid IPv4 UDP Packet",
			packetData: constructPacket(17, srcIP, dstIP, 12345, targetPort, payload),
			wantValid:  true,
		},
		{
			name:       "Configured Destination IP Matches",
			packetData: constructPacket(17, srcIP, dstIP, 12345, targetPort, payload),
			targetIP:   dstIP,
			wantValid:  true,
		},
		{
			name:       "Configured Destination IP Does Not Match",
			packetData: constructPacket(17, srcIP, dstIP, 12345, targetPort, payload),
			targetIP:   net.IP{10, 0, 0, 2},
			wantValid:  false,
		},
		{
			name:       "Invalid IP Version",
			packetData: append([]byte{0x60}, make([]byte, 39)...),
			wantValid:  false,
		},
		{
			name:       "Invalid Protocol (TCP)",
			packetData: constructPacket(6, srcIP, dstIP, 12345, targetPort, payload),
			wantValid:  false,
		},
		{
			name:       "IPv4 Fragment",
			packetData: fragment,
			wantValid:  false,
		},
		{
			name:       "Source Port Match Does Not Bypass Destination Port",
			packetData: constructPacket(17, srcIP, dstIP, targetPort, 9999, payload),
			wantValid:  false,
		},
		{
			name:       "Empty Payload",
			packetData: constructPacket(17, srcIP, dstIP, 12345, targetPort, []byte{}),
			wantValid:  false,
		},
		{
			name:       "Truncated Packet (Header Only)",
			packetData: constructPacket(17, srcIP, dstIP, 12345, targetPort, payload)[:25],
			wantValid:  false,
		},
		{
			name: "Total Length Smaller Than UDP Header",
			packetData: func() []byte {
				packetData := constructPacket(17, srcIP, dstIP, 12345, targetPort, payload)
				binary.BigEndian.PutUint16(packetData[2:4], 24)
				return packetData
			}(),
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, info := parsePacket(tt.packetData, targetPort, tt.targetIP)
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

func TestParseListenIPv4(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantIP  net.IP
		wantErr bool
	}{
		{name: "Empty", value: ""},
		{name: "IPv4", value: "203.0.113.10", wantIP: net.IP{203, 0, 113, 10}},
		{name: "Invalid", value: "not-an-ip", wantErr: true},
		{name: "IPv6 Unsupported", value: "2001:db8::1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseListenIPv4(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseListenIPv4() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !got.Equal(tt.wantIP) {
				t.Fatalf("parseListenIPv4() = %v, want %v", got, tt.wantIP)
			}
		})
	}
}

func TestResolveCaptureInterfaceAny(t *testing.T) {
	for _, value := range []string{"any", "", " any "} {
		t.Run(value, func(t *testing.T) {
			ifi, name, err := resolveCaptureInterface(value)
			if err != nil {
				t.Fatalf("resolveCaptureInterface() error = %v", err)
			}
			if ifi == nil {
				t.Fatal("resolveCaptureInterface() returned a nil interface")
			}
			if ifi.Index != 0 {
				t.Fatalf("interface index = %d, want 0", ifi.Index)
			}
			if name != "any" {
				t.Fatalf("interface name = %q, want any", name)
			}
		})
	}
}

func TestBuildBPFFilter(t *testing.T) {
	const targetPort = 3001

	srcIP := net.IP{192, 0, 2, 10}
	dstIP := net.IP{203, 0, 113, 20}
	otherIP := net.IP{203, 0, 113, 21}
	payload := []byte("knock")

	valid := constructPacket(ipProtoUDP, srcIP, dstIP, 40000, targetPort, payload)
	withOptions := addIPv4Options(t, valid, []byte{1, 1, 1, 1})
	fragment := append([]byte(nil), valid...)
	binary.BigEndian.PutUint16(fragment[6:8], 0x2000)
	fragmentOffset := append([]byte(nil), valid...)
	binary.BigEndian.PutUint16(fragmentOffset[6:8], 1)
	dontFragment := append([]byte(nil), valid...)
	binary.BigEndian.PutUint16(dontFragment[6:8], 0x4000)
	invalidVersion := append([]byte(nil), valid...)
	invalidVersion[0] = 0x65
	invalidIHL := append([]byte(nil), valid...)
	invalidIHL[0] = 0x44

	tests := []struct {
		name       string
		targetIP   net.IP
		packetData []byte
		wantAccept bool
	}{
		{
			name:       "UDP destination port",
			packetData: valid,
			wantAccept: true,
		},
		{
			name:       "IPv4 options preserve UDP offset",
			packetData: withOptions,
			wantAccept: true,
		},
		{
			name:       "destination IP matches",
			targetIP:   dstIP,
			packetData: valid,
			wantAccept: true,
		},
		{
			name:       "destination IP differs",
			targetIP:   otherIP,
			packetData: valid,
		},
		{
			name:       "source port match is rejected",
			packetData: constructPacket(ipProtoUDP, srcIP, dstIP, targetPort, 9999, payload),
		},
		{
			name:       "wrong destination port",
			packetData: constructPacket(ipProtoUDP, srcIP, dstIP, 40000, 9999, payload),
		},
		{
			name:       "TCP rejected",
			packetData: constructPacket(6, srcIP, dstIP, 40000, targetPort, payload),
		},
		{
			name:       "invalid IP version rejected",
			packetData: invalidVersion,
		},
		{
			name:       "invalid IHL rejected",
			packetData: invalidIHL,
		},
		{
			name:       "more fragments rejected",
			packetData: fragment,
		},
		{
			name:       "fragment offset rejected",
			packetData: fragmentOffset,
		},
		{
			name:       "dont fragment accepted",
			packetData: dontFragment,
			wantAccept: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := buildBPFFilter(targetPort, tt.targetIP)
			if err != nil {
				t.Fatalf("buildBPFFilter() error = %v", err)
			}

			instructions, allDecoded := bpf.Disassemble(raw)
			if !allDecoded {
				t.Fatal("BPF program did not fully disassemble")
			}
			vm, err := bpf.NewVM(instructions)
			if err != nil {
				t.Fatalf("bpf.NewVM() error = %v", err)
			}
			got, err := vm.Run(tt.packetData)
			if err != nil {
				t.Fatalf("BPF VM run error = %v", err)
			}
			if (got > 0) != tt.wantAccept {
				t.Fatalf("BPF accepted = %v, want %v", got > 0, tt.wantAccept)
			}
		})
	}
}

func TestBuildBPFFilterRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		port     int
		targetIP net.IP
	}{
		{name: "zero port", port: 0},
		{name: "port above maximum", port: 65536},
		{name: "IPv6 destination", port: 3001, targetIP: net.ParseIP("2001:db8::1")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := buildBPFFilter(tt.port, tt.targetIP); err == nil {
				t.Fatal("buildBPFFilter() expected an error")
			}
		})
	}
}

func addIPv4Options(t *testing.T, packetData, options []byte) []byte {
	t.Helper()

	if len(options) == 0 || len(options)%4 != 0 {
		t.Fatal("IPv4 options must be a non-empty multiple of four bytes")
	}
	ipOffset := 0
	udpOffset := 20
	if len(packetData) < udpOffset {
		t.Fatal("packet is too short for IPv4 options")
	}

	result := make([]byte, 0, len(packetData)+len(options))
	result = append(result, packetData[:udpOffset]...)
	result = append(result, options...)
	result = append(result, packetData[udpOffset:]...)
	result[ipOffset] = 0x40 | byte((20+len(options))/4)
	totalLength := binary.BigEndian.Uint16(result[ipOffset+2 : ipOffset+4])
	binary.BigEndian.PutUint16(result[ipOffset+2:ipOffset+4], totalLength+uint16(len(options)))
	return result
}
