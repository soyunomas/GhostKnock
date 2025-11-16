// ghostknock es el cliente CLI para enviar "knocks" criptográficamente firmados.
package main

import (
	"crypto/ed25519"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"

	// Esta ruta DEBE COINCIDIR con la línea 'module' en tu archivo go.mod
	"github.com/your-org/ghostknock/internal/protocol"
)

const (
	defaultKeyFile = "id_ed25519"
)

func main() {
	// 1. Configurar y parsear los argumentos de la línea de comandos.
	host := flag.String("host", "", "Host o dirección IP del servidor GhostKnock (requerido)")
	port := flag.Int("port", 3001, "Puerto UDP en el que el servidor escucha")
	action := flag.String("action", "", "ActionID a solicitar (requerido)")
	keyFile := flag.String("key", "", "Ruta a la clave privada ed25519 (por defecto: ~/.config/ghostknock/id_ed25519)")
	flag.Parse()

	if *host == "" || *action == "" {
		fmt.Println("Error: los argumentos -host y -action son requeridos.")
		flag.Usage()
		os.Exit(1)
	}

	log.SetFlags(0)
	log.Printf("Preparando knock para la acción '%s' en %s:%d...", *action, *host, *port)

	// 2. DETERMINAR LA RUTA DE LA CLAVE PRIVADA
	var finalKeyPath string
	if *keyFile != "" {
		// El usuario especificó una clave, la usamos.
		finalKeyPath = *keyFile
		log.Printf("Usando clave privada especificada: %s", finalKeyPath)
	} else {
		// El usuario no especificó una clave, buscamos la predeterminada.
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("FATAL: No se pudo determinar el directorio home para buscar la clave por defecto: %v", err)
		}
		finalKeyPath = filepath.Join(homeDir, ".config", "ghostknock", defaultKeyFile)
		log.Printf("Usando clave privada por defecto: %s", finalKeyPath)
	}

	// 3. Cargar la clave privada del fichero.
	privateKeyBytes, err := os.ReadFile(finalKeyPath)
	if err != nil {
		log.Fatalf("FATAL: No se pudo leer la clave privada '%s'. ¿Ejecutaste ghostknock-keygen? Error: %v", finalKeyPath, err)
	}

	// VALIDACIÓN DE SEGURIDAD: Una clave privada ed25519 siempre tiene 64 bytes.
	if len(privateKeyBytes) != ed25519.PrivateKeySize {
		log.Fatalf("FATAL: El archivo de clave privada '%s' tiene un tamaño incorrecto. Se esperaba %d bytes, pero tiene %d.", finalKeyPath, ed25519.PrivateKeySize, len(privateKeyBytes))
	}
	privateKey := ed25519.PrivateKey(privateKeyBytes)

	// 4. Crear y serializar el payload del protocolo.
	payload := protocol.NewPayload(*action)

	serializedPayload, err := payload.Serialize()
	if err != nil {
		log.Fatalf("FATAL: No se pudo serializar el payload: %v", err)
	}

	// 5. Firmar el payload serializado.
	signature := ed25519.Sign(privateKey, serializedPayload)

	// 6. Construir el mensaje final: [firma][payload serializado]
	finalMessage := append(signature, serializedPayload...)

	// 7. Enviar el mensaje en un único paquete UDP.
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

	log.Printf("-- Knock enviado (%d bytes).", bytesSent)
}
