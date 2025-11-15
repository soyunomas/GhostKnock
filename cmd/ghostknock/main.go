// ghostknock es el cliente CLI para enviar "knocks" criptográficamente firmados.
package main

import (
	"crypto/ed25519"
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	// Esta ruta DEBE COINCIDIR con la línea 'module' en tu archivo go.mod
	"github.com/your-org/ghostknock/internal/protocol"
)

const (
	privateKeyFile = "id_ed25519"
)

func main() {
	// 1. Configurar y parsear los argumentos de la línea de comandos.
	host := flag.String("host", "", "Host o dirección IP del servidor GhostKnock (requerido)")
	port := flag.Int("port", 3001, "Puerto UDP en el que el servidor escucha")
	action := flag.String("action", "", "ActionID a solicitar (requerido)")
	flag.Parse()

	if *host == "" || *action == "" {
		fmt.Println("Error: los argumentos -host y -action son requeridos.")
		flag.Usage()
		os.Exit(1)
	}

	log.SetFlags(0)
	log.Printf("Preparando knock para la acción '%s' en %s:%d...", *action, *host, *port)

	// 2. Cargar la clave privada del fichero.
	privateKeyBytes, err := os.ReadFile(privateKeyFile)
	if err != nil {
		log.Fatalf("FATAL: No se pudo leer la clave privada '%s'. ¿Ejecutaste ghostknock-keygen? Error: %v", privateKeyFile, err)
	}

	// VALIDACIÓN DE SEGURIDAD: Una clave privada ed25519 siempre tiene 64 bytes.
	if len(privateKeyBytes) != ed25519.PrivateKeySize {
		log.Fatalf("FATAL: El archivo de clave privada '%s' tiene un tamaño incorrecto. Se esperaba %d bytes, pero tiene %d.", privateKeyFile, ed25519.PrivateKeySize, len(privateKeyBytes))
	}
	privateKey := ed25519.PrivateKey(privateKeyBytes)

	// 3. Crear y serializar el payload del protocolo.
	payload := protocol.NewPayload(*action)

	serializedPayload, err := payload.Serialize()
	if err != nil {
		log.Fatalf("FATAL: No se pudo serializar el payload: %v", err)
	}

	// 4. Firmar el payload serializado.
	signature := ed25519.Sign(privateKey, serializedPayload)

	// 5. Construir el mensaje final: [firma][payload serializado]
	finalMessage := append(signature, serializedPayload...)

	// 6. Enviar el mensaje en un único paquete UDP.
	serverAddr := fmt.Sprintf("%s:%d", *host, *port)
	conn, err := net.Dial("udp", serverAddr)
	if err != nil {
		log.Fatalf("FATAL: No se pudo resolver la dirección del servidor '%s': %v", serverAddr, err)
	}
	defer conn.Close()

	bytesSent, err := conn.Write(finalMessage)
	if err != nil {
		log.Fatalf("FATAL: Error al enviar el paquete UDP: %v", err)
	}

	log.Printf("✅ ¡Éxito! Knock enviado (%d bytes).", bytesSent)
}
