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
	"time"

	"github.com/mdlayher/packet"
	"github.com/soyunomas/ghostknock/internal/config"
	"golang.org/x/sys/unix"
)

// Constantes de Protocolo
const (
	ethTypeIPv4 = 0x0800
	ethTypeVLAN = 0x8100
	ipProtoUDP  = 17
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

	// 1. Resolver la Interfaz de Red
	var ifi *net.Interface
	var ifaceName string

	if listenerCfg.Interface == "any" || listenerCfg.Interface == "" {
		// nil hace que mdlayher/packet escuche en todas las interfaces
		ifi = nil
		ifaceName = "all/any"
	} else {
		ifi, err = net.InterfaceByName(listenerCfg.Interface)
		if err != nil {
			slog.Error("FATAL: No se pudo encontrar la interfaz de red", "interface", listenerCfg.Interface, "error", err)
			os.Exit(1)
		}
		ifaceName = ifi.Name
	}

	// 2. Abrir el Socket RAW (AF_PACKET)
	// Usamos packet.Raw para recibir las cabeceras Ethernet completas.
	// Filtramos por ETH_P_IP para recibir solo tráfico IP y ahorrar llamadas del kernel.
	conn, err := packet.Listen(ifi, packet.Raw, unix.ETH_P_IP, nil)
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
		"mode", "Zero-Copy Parser")

	// 3. Buffer de Lectura (Hot Path)
	// MTU estándar (1500) + Overhead Ethernet/VLAN/Jumbo. 2048 es seguro y eficiente (potencia de 2).
	buf := make([]byte, 2048)

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

// parsePacket implementa un parser manual de headers para evitar allocs.
// Retorna true y la struct si el paquete es un candidato válido (UDP + Puerto correcto).
func parsePacket(data []byte, targetPort int, targetIP net.IP) (bool, PacketInfo) {
	// --- CAPA 1: ETHERNET ---
	// Longitud mínima: Header Ethernet (14 bytes)
	if len(data) < 14 {
		return false, PacketInfo{}
	}

	// Bytes 12-13: EtherType
	ethType := binary.BigEndian.Uint16(data[12:14])
	offset := 14

	// Manejo de VLAN (802.1Q)
	if ethType == ethTypeVLAN {
		// Necesitamos al menos 4 bytes más para el tag VLAN y el siguiente EtherType
		if len(data) < 18 {
			return false, PacketInfo{}
		}
		// El EtherType real está después del tag VLAN (bytes 16-17)
		ethType = binary.BigEndian.Uint16(data[16:18])
		offset += 4
	}

	// Solo nos interesa IPv4 (0x0800)
	if ethType != ethTypeIPv4 {
		return false, PacketInfo{}
	}

	// --- CAPA 2: IPv4 ---
	// data[offset] es el inicio del header IP.
	// Necesitamos al menos 20 bytes de header IP.
	if len(data) < offset+20 {
		return false, PacketInfo{}
	}

	ipHeader := data[offset:]

	// Validar Versión (4) - Nibble alto del byte 0
	if (ipHeader[0] >> 4) != 4 {
		return false, PacketInfo{}
	}

	// Validar Protocolo (Byte 9). UDP = 17.
	if ipHeader[9] != ipProtoUDP {
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

	// Extraer IP Origen (Bytes 12-15 del header IP)
	// Hacemos copia segura para no fugar memoria del buffer grande
	srcIP := make(net.IP, 4)
	copy(srcIP, ipHeader[12:16])

	// --- CAPA 3: UDP ---
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
