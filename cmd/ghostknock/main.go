// ghostknock es el cliente CLI para enviar "knocks" criptográficamente firmados.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/your-org/ghostknock/internal/protocol"
	"gopkg.in/yaml.v3"
)

// version se establece en tiempo de compilación.
var version = "dev"

const (
	defaultKeyFilename = "id_ed25519"
	profilesFilename   = "profiles.yaml"
)

// Profile define la estructura de un perfil de conexión.
type Profile struct {
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	ServerPubkey string `yaml:"server_pubkey"`
	Key          string `yaml:"key"` // Ruta a la clave privada del cliente
}

// ProfilesConfig mapea nombres de perfil a su configuración.
type ProfilesConfig struct {
	Profiles map[string]Profile `yaml:"profiles"`
}

func main() {
	// 1. Definición de Flags
	showVersion := flag.Bool("version", false, "Muestra la versión.")
	flagProfile := flag.String("profile", "", "Nombre del perfil a usar (definido en profiles.yaml)")
	flagHost := flag.String("host", "", "Host/IP del servidor")
	flagPort := flag.Int("port", 0, "Puerto UDP (default 3001)")
	flagAction := flag.String("action", "", "ActionID a solicitar")
	flagKey := flag.String("key", "", "Ruta clave privada cliente")
	flagServerPub := flag.String("server-pubkey", "", "Ruta clave pública servidor")
	flagArgs := flag.String("args", "", "Argumentos: key=val,key2=val2")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ghostknock version %s\n", version)
		os.Exit(0)
	}

	log.SetFlags(0)

	// 2. Cargar Perfil si se solicita
	var profile Profile
	if *flagProfile != "" {
		loadedProfile, err := loadProfile(*flagProfile)
		if err != nil {
			log.Fatalf("Error al cargar perfil '%s': %v", *flagProfile, err)
		}
		profile = loadedProfile
		log.Printf("Perfil cargado: %s", *flagProfile)
	}

	// 3. Consolidar Configuración (Prioridad: Flag > Perfil > Default)
	host := *flagHost
	if host == "" {
		host = profile.Host
	}

	port := *flagPort
	if port == 0 {
		if profile.Port != 0 {
			port = profile.Port
		} else {
			port = 3001 // Default global
		}
	}

	serverPubPath := *flagServerPub
	if serverPubPath == "" {
		serverPubPath = profile.ServerPubkey
	}

	clientKeyPath := *flagKey
	if clientKeyPath == "" {
		clientKeyPath = profile.Key
	}

	// 4. Validaciones Finales
	if host == "" || *flagAction == "" || serverPubPath == "" {
		fmt.Println("Error: Faltan argumentos requeridos.")
		fmt.Println("Debe especificar Host, Action y Server-Pubkey mediante flags o un perfil.")
		fmt.Println("\nUso con perfil: ghostknock -profile miperfil -action restart-nginx")
		fmt.Println("Uso manual:     ghostknock -host 1.2.3.4 -server-pubkey s.pub -action ...")
		flag.Usage()
		os.Exit(1)
	}

	// 5. Resolución de la Clave Privada del Cliente
	if clientKeyPath == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			// Fallback para sistemas raros o contenedores sin HOME definido correctamente
			home, _ := os.UserHomeDir()
			configDir = filepath.Join(home, ".config")
		} else {
			// En Linux: ~/.config
			// En Windows: %APPDATA%
		}
		clientKeyPath = filepath.Join(configDir, "ghostknock", defaultKeyFilename)
	}
	log.Printf("Preparando knock para '%s' en %s:%d...", *flagAction, host, port)

	// 6. Carga de Claves Criptográficas
	privateKey, err := loadPrivateKey(clientKeyPath)
	if err != nil {
		log.Fatalf("FATAL: Error clave privada: %v", err)
	}

	serverPubKey, err := loadPublicKey(serverPubPath)
	if err != nil {
		log.Fatalf("FATAL: Error clave pública servidor: %v", err)
	}

	// 7. Construcción del Payload
	payload := protocol.NewPayload(*flagAction)
	if *flagArgs != "" {
		parseArgs(payload, *flagArgs)
	}

	// --- FASE 2: Traffic Padding (Ofuscación) ---
	addTrafficPadding(payload)

	// 8. Cifrado y Envío
	finalMessage, err := protocol.EncryptAndSign(payload, privateKey, serverPubKey)
	if err != nil {
		log.Fatalf("FATAL: Error cifrado: %v", err)
	}

	sendPacket(host, port, finalMessage)
}

// --- Funciones Auxiliares ---

// addTrafficPadding rellena el campo Padding con basura aleatoria
// para variar el tamaño del paquete cifrado final.
func addTrafficPadding(p *protocol.Payload) {
	// 1. Decidir longitud aleatoria (0 a 255 bytes)
	nBig, err := rand.Int(rand.Reader, big.NewInt(256))
	if err != nil {
		// Si falla el RNG, continuamos sin padding (fail safe)
		return
	}
	n := int(nBig.Int64())

	if n == 0 {
		return
	}

	// 2. Generar bytes aleatorios
	randomBytes := make([]byte, n)
	if _, err := rand.Read(randomBytes); err != nil {
		return
	}

	// 3. Codificar a String (Base64) para compatibilidad JSON
	p.Padding = base64.StdEncoding.EncodeToString(randomBytes)
}

func loadProfile(profileName string) (Profile, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return Profile{}, fmt.Errorf("no se pudo determinar directorio de config: %v", err)
	}
	// Windows: %APPDATA%\ghostknock\profiles.yaml
	// Linux:   ~/.config/ghostknock/profiles.yaml
	path := filepath.Join(configDir, "ghostknock", profilesFilename)

	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, fmt.Errorf("no se pudo leer %s: %w", path, err)
	}

	var cfg ProfilesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Profile{}, fmt.Errorf("error de sintaxis YAML en %s: %w", path, err)
	}

	p, ok := cfg.Profiles[profileName]
	if !ok {
		return Profile{}, fmt.Errorf("el perfil '%s' no existe en el archivo", profileName)
	}
	return p, nil
}

func loadPrivateKey(path string) (ed25519.PrivateKey, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("lectura fallida '%s': %w", path, err)
	}
	if len(bytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("tamaño incorrecto (esperado %d, obtenido %d)", ed25519.PrivateKeySize, len(bytes))
	}
	return ed25519.PrivateKey(bytes), nil
}

func loadPublicKey(path string) (ed25519.PublicKey, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("lectura fallida '%s': %w", path, err)
	}
	if len(bytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("tamaño incorrecto (esperado %d, obtenido %d)", ed25519.PublicKeySize, len(bytes))
	}
	return ed25519.PublicKey(bytes), nil
}

func parseArgs(p *protocol.Payload, argsStr string) {
	pairs := strings.Split(argsStr, ",")
	for _, pair := range pairs {
		if pair == "" {
			continue
		}
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			log.Fatalf("Formato argumento inválido: '%s'. Use key=val", pair)
		}
		p.Params[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
	}
}

func sendPacket(host string, port int, data []byte) {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.Dial("udp", addr)
	if err != nil {
		log.Fatalf("FATAL: Error red: %v", err)
	}
	defer conn.Close()

	n, err := conn.Write(data)
	if err != nil {
		log.Fatalf("FATAL: Error envío: %v", err)
	}
	log.Printf("-- Knock enviado (%d bytes) a %s", n, addr)
}
