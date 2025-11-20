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
	"strings" // <<-- NUEVA IMPORTACIÓN

	// Esta ruta DEBE COINCIDIR con la línea 'module' en tu archivo go.mod
	"github.com/your-org/ghostknock/internal/protocol"
)

// version se establece en tiempo de compilación usando ldflags.
var version = "dev"

const (
	defaultKeyFile = "id_ed25519"
)

func main() {
	// 1. Configurar y parsear los argumentos de la línea de comandos.
	showVersion := flag.Bool("version", false, "Muestra la versión de la aplicación y sale.")
	host := flag.String("host", "", "Host o dirección IP del servidor GhostKnock (requerido)")
	port := flag.Int("port", 3001, "Puerto UDP en el que el servidor escucha")
	action := flag.String("action", "", "ActionID a solicitar (requerido)")
	keyFile := flag.String("key", "", "Ruta a la clave privada ed25519 (por defecto: ~/.config/ghostknock/id_ed25519)")
	// Nuevo flag para argumentos
	args := flag.String("args", "", "Argumentos opcionales para la acción, formato: clave=valor,clave2=valor2")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ghostknock version %s\n", version)
		os.Exit(0)
	}

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
		finalKeyPath = *keyFile
		log.Printf("Usando clave privada especificada: %s", finalKeyPath)
	} else {
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

	if len(privateKeyBytes) != ed25519.PrivateKeySize {
		log.Fatalf("FATAL: El archivo de clave privada '%s' tiene un tamaño incorrecto. Se esperaba %d bytes, pero tiene %d.", finalKeyPath, ed25519.PrivateKeySize, len(privateKeyBytes))
	}
	privateKey := ed25519.PrivateKey(privateKeyBytes)

	// 4. Crear y rellenar el payload.
	payload := protocol.NewPayload(*action)

	// --- LÓGICA DE PARSING DE ARGUMENTOS ---
	if *args != "" {
		pairs := strings.Split(*args, ",")
		for _, pair := range pairs {
			if pair == "" {
				continue
			}
			// SplitN asegura que solo rompemos en el primer '='
			kv := strings.SplitN(pair, "=", 2)
			if len(kv) != 2 {
				log.Fatalf("Error de formato en argumentos: '%s'. Debe ser clave=valor.", pair)
			}
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])

			// Añadimos al mapa de parámetros
			payload.Params[key] = value
		}
		if len(payload.Params) > 0 {
			log.Printf("Adjuntando %d parámetros al payload.", len(payload.Params))
		}
	}
	// ---------------------------------------

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
