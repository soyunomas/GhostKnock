//go:build linux

// El paquete listener se encarga de la captura de paquetes de bajo nivel.
// IMPLEMENTACIÓN NATIVA (AF_PACKET) - SIN CGO/LIBPCAP
package listener

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	"github.com/mdlayher/packet"
	"github.com/soyunomas/ghostknock/internal/config"
	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"
)

// Constantes de Protocolo
const (
	ipProtoUDP        = 17
	minIPv4HeaderLen  = 20
	captureBufferSize = 2048
)

// Start inicia la captura de tráfico usando Sockets RAW de Linux (AF_PACKET).
// Elimina la dependencia de CGo y libpcap.
func Start(ctx context.Context, listenerCfg config.Listener, pcapTimeoutMs int, packetsCh chan<- PacketInfo, onDrop func()) {
	defer close(packetsCh)

	listenIP, err := parseListenIPv4(listenerCfg.ListenIP)
	if err != nil {
		slog.Error("FATAL: listen_ip inválida para el listener nativo", "listen_ip", listenerCfg.ListenIP, "error", err)
		os.Exit(1)
	}

	// 1. Resolver la interfaz. AF_PACKET usa ifindex=0 para escuchar en todas.
	ifi, ifaceName, err := resolveCaptureInterface(listenerCfg.Interface)
	if err != nil {
		slog.Error("FATAL: No se pudo encontrar la interfaz de red", "interface", listenerCfg.Interface, "error", err)
		os.Exit(1)
	}

	filter, err := buildBPFFilter(listenerCfg.Port, listenIP)
	if err != nil {
		slog.Error("FATAL: No se pudo construir el filtro BPF", "error", err)
		os.Exit(1)
	}

	// 2. Abrir AF_PACKET en modo SOCK_DGRAM. Linux elimina la cabecera de
	// enlace, por lo que "any" funciona igual para Ethernet, loopback y TUN.
	// El BPF se instala antes de bind(2), de modo que solo UDP dirigido al
	// puerto/IP configurados entra en la cola del socket.
	conn, err := packet.Listen(ifi, packet.Datagram, unix.ETH_P_IP, &packet.Config{Filter: filter})
	if err != nil {
		slog.Error("FATAL: No se pudo abrir el socket AF_PACKET (¿tienes permisos root/CAP_NET_RAW?)", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Conversión de ms a Duration para el Deadline
	// Nota: AF_PACKET no tiene buffering del kernel configurable igual que pcap,
	// pero SetReadDeadline controla la cadencia del bucle en Go.
	readTimeout := time.Duration(pcapTimeoutMs) * time.Millisecond
	if readTimeout <= 0 {
		readTimeout = 200 * time.Millisecond
	}

	slog.Info("Iniciando escucha pasiva (Nativo/AF_PACKET)",
		"interface", ifaceName,
		"udp_port", listenerCfg.Port,
		"listen_ip", listenerCfg.ListenIP,
		"address_family", "IPv4",
		"kernel_filter", "BPF UDP dst",
		"mode", "Zero-Copy Parser")

	// 3. Buffer de Lectura (Hot Path)
	// MTU estándar (1500) + Overhead Ethernet/VLAN/Jumbo. 2048 es seguro y eficiente (potencia de 2).
	buf := make([]byte, captureBufferSize)

	slog.Info("Esperando paquetes...")

	// 4. Bucle Principal
	for {
		// Comprobación de salida limpia
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Establecemos deadline para permitir que el contexto cancele el bloqueo de lectura
		conn.SetReadDeadline(time.Now().Add(readTimeout))

		// Lectura Zero-Alloc (escribimos en el buffer existente)
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			// Si es timeout, solo iteramos (es normal para chequear ctx.Done)
			if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
				continue
			}
			// Errores "interrupted system call" son comunes al recibir señales
			if err == unix.EINTR {
				continue
			}
			// Error real de red, logueamos pero no matamos el listener
			slog.Debug("Error leyendo del socket", "error", err)
			continue
		}

		// Procesamos solo los bytes leídos (buf[:n])
		validPacket, info := parsePacket(buf[:n], listenerCfg.Port, listenIP)
		if validPacket {
			// Envío NO BLOQUEANTE al canal de procesamiento
			select {
			case packetsCh <- info:
				// OK
			default:
				// Saturación
				if onDrop != nil {
					onDrop()
				}
			}
		}
	}
}

func resolveCaptureInterface(value string) (*net.Interface, string, error) {
	name := strings.TrimSpace(value)
	if name == "" || name == "any" {
		// mdlayher/packet requiere un puntero no nil, pero transmite Index
		// directamente a sockaddr_ll. Linux define ifindex=0 como wildcard.
		return &net.Interface{Index: 0, Name: "any"}, "any", nil
	}

	ifi, err := net.InterfaceByName(name)
	if err != nil {
		return nil, "", err
	}
	return ifi, ifi.Name, nil
}

func buildBPFFilter(targetPort int, targetIP net.IP) ([]bpf.RawInstruction, error) {
	if targetPort <= 0 || targetPort > 65535 {
		return nil, fmt.Errorf("UDP destination port out of range: %d", targetPort)
	}

	var ipv4 net.IP
	if targetIP != nil {
		ipv4 = targetIP.To4()
		if ipv4 == nil {
			return nil, fmt.Errorf("BPF destination address must be IPv4")
		}
	}

	instructions := []bpf.Instruction{
		// Validate IPv4 version and a minimum 20-byte header.
		bpf.LoadAbsolute{Off: 0, Size: 1},
		bpf.ALUOpConstant{Op: bpf.ALUOpAnd, Val: 0xf0},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0x40},
		bpf.LoadAbsolute{Off: 0, Size: 1},
		bpf.ALUOpConstant{Op: bpf.ALUOpAnd, Val: 0x0f},
		bpf.JumpIf{Cond: bpf.JumpGreaterOrEqual, Val: 5},

		// Accept UDP only.
		bpf.LoadAbsolute{Off: 9, Size: 1},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: ipProtoUDP},

		// Reject all IPv4 fragments; this listener does not reassemble them.
		bpf.LoadAbsolute{Off: 6, Size: 2},
		bpf.ALUOpConstant{Op: bpf.ALUOpAnd, Val: 0x3fff},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: 0},
	}

	guardIndexes := []int{2, 5, 7, 10}
	if ipv4 != nil {
		instructions = append(instructions,
			bpf.LoadAbsolute{Off: 16, Size: 4},
			bpf.JumpIf{
				Cond: bpf.JumpEqual,
				Val:  binary.BigEndian.Uint32(ipv4),
			},
		)
		guardIndexes = append(guardIndexes, len(instructions)-1)
	}

	instructions = append(instructions,
		// X becomes the real IPv4 header length, so options are supported.
		bpf.LoadMemShift{Off: 0},
		bpf.LoadIndirect{Off: 2, Size: 2},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(targetPort)},
		bpf.RetConstant{Val: captureBufferSize},
		bpf.RetConstant{Val: 0},
	)
	guardIndexes = append(guardIndexes, len(instructions)-3)

	rejectIndex := len(instructions) - 1
	for _, index := range guardIndexes {
		jump := instructions[index].(bpf.JumpIf)
		skip := rejectIndex - index - 1
		if skip > 255 {
			return nil, fmt.Errorf("BPF program is too large")
		}
		jump.SkipFalse = uint8(skip)
		instructions[index] = jump
	}

	raw, err := bpf.Assemble(instructions)
	if err != nil {
		return nil, fmt.Errorf("assemble BPF filter: %w", err)
	}
	return raw, nil
}

func parseListenIPv4(value string) (net.IP, error) {
	if value == "" {
		return nil, nil
	}

	ip := net.ParseIP(value)
	if ip == nil {
		return nil, fmt.Errorf("must be a valid IP address")
	}
	ip = ip.To4()
	if ip == nil {
		return nil, fmt.Errorf("IPv6 is not supported by the native listener")
	}
	return ip, nil
}

// parsePacket procesa el datagrama IPv4 entregado por AF_PACKET/SOCK_DGRAM.
// Retorna true y la struct si el paquete es un candidato válido (UDP + Puerto correcto).
func parsePacket(data []byte, targetPort int, targetIP net.IP) (bool, PacketInfo) {
	// --- CAPA IPv4 ---
	if len(data) < minIPv4HeaderLen {
		return false, PacketInfo{}
	}

	ipHeader := data

	// Validar Versión (4) - Nibble alto del byte 0
	if (ipHeader[0] >> 4) != 4 {
		return false, PacketInfo{}
	}

	// Validar Protocolo (Byte 9). UDP = 17.
	if ipHeader[9] != ipProtoUDP {
		return false, PacketInfo{}
	}
	if binary.BigEndian.Uint16(ipHeader[6:8])&0x3fff != 0 {
		return false, PacketInfo{}
	}
	if targetIP != nil && !net.IP(ipHeader[16:20]).Equal(targetIP) {
		return false, PacketInfo{}
	}

	// Calcular IHL (Internet Header Length) - Nibble bajo del byte 0
	// IHL son palabras de 32 bits (4 bytes).
	ihl := (ipHeader[0] & 0x0F) * 4
	if ihl < 20 {
		return false, PacketInfo{} // Header inválido
	}
	if int(ihl) > len(ipHeader) {
		return false, PacketInfo{} // Header truncado
	}

	totalLen := int(binary.BigEndian.Uint16(ipHeader[2:4]))
	if totalLen < int(ihl)+8 || totalLen > len(ipHeader) {
		return false, PacketInfo{}
	}
	ipHeader = ipHeader[:totalLen]

	// Extraer IP Origen (Bytes 12-15 del header IP)
	// Hacemos copia segura para no fugar memoria del buffer grande
	srcIP := make(net.IP, 4)
	copy(srcIP, ipHeader[12:16])

	// --- CAPA UDP ---
	// El UDP empieza donde termina el IHL
	udpOffset := int(ihl)
	// Header UDP son 8 bytes fijos
	if len(ipHeader) < udpOffset+8 {
		return false, PacketInfo{}
	}

	udpHeader := ipHeader[udpOffset:]

	// Validar Puerto Destino (Bytes 2-3 del header UDP)
	dstPort := binary.BigEndian.Uint16(udpHeader[2:4])
	if int(dstPort) != targetPort {
		return false, PacketInfo{}
	}

	// Validar Longitud UDP (Bytes 4-5)
	// Esta longitud incluye el header UDP (8 bytes) + Payload
	udpLen := binary.BigEndian.Uint16(udpHeader[4:6])
	if udpLen < 8 {
		return false, PacketInfo{}
	}

	payloadLen := int(udpLen) - 8

	// Validar límites del payload
	if payloadLen <= 0 {
		return false, PacketInfo{} // Paquete vacío
	}
	if payloadLen > MaxPayloadSize {
		// Protección contra bufferbloat/ataques de memoria
		return false, PacketInfo{}
	}

	// Verificar que realmente tenemos esos bytes en el buffer
	if len(udpHeader) < 8+payloadLen {
		return false, PacketInfo{} // Payload truncado
	}

	// --- EXTRACCIÓN (COPY) ---
	// CRÍTICO: Debemos copiar los datos a un nuevo slice.
	// Si devolvemos una referencia a 'buf', el siguiente ReadFrom sobrescribirá los datos
	// antes de que el worker pool los procese (Condición de carrera).
	payload := make([]byte, payloadLen)
	copy(payload, udpHeader[8:8+payloadLen])

	return true, PacketInfo{
		Payload:  payload,
		SourceIP: srcIP,
	}
}
