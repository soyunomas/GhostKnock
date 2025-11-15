// El paquete config gestiona la carga y validación de la configuración del servidor.
package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Action define una plantilla de comando y su comportamiento de reversión.
type Action struct {
	Command            string `yaml:"command"`
	RevertCommand      string `yaml:"revert_command"`
	RevertDelaySeconds int    `yaml:"revert_delay_seconds"`
}

// Config es la estructura raíz de nuestro archivo de configuración.
type Config struct {
	Listener Listener          `yaml:"listener"`
	Users    []User            `yaml:"users"`
	Actions  map[string]Action `yaml:"actions"` // Mapa de ActionID a su definición
}

// Listener define en qué interfaz y puerto escucha el servidor.
type Listener struct {
	Interface string `yaml:"interface"`
	Port      int    `yaml:"port"`
}

// User define un usuario autorizado.
type User struct {
	Name             string   `yaml:"name"`
	PublicKeyB64     string   `yaml:"public_key"`
	AllowedActions   []string `yaml:"actions"`
	DecodedPublicKey ed25519.PublicKey
}

// LoadConfig lee y parsea el archivo de configuración YAML desde la ruta especificada.
// También realiza una validación crítica de los datos cargados.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer el archivo de configuración en '%s': %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("error al parsear el archivo de configuración YAML: %w", err)
	}

	if err := validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("configuración inválida: %w", err)
	}

	return &cfg, nil
}

// validateConfig realiza comprobaciones de sanidad en la configuración cargada.
func validateConfig(cfg *Config) error {
	if cfg.Listener.Port <= 0 || cfg.Listener.Port > 65535 {
		return fmt.Errorf("puerto de escucha inválido: %d", cfg.Listener.Port)
	}
	if cfg.Listener.Interface == "" {
		return fmt.Errorf("la interfaz de escucha no puede estar vacía")
	}
	if len(cfg.Users) == 0 {
		return fmt.Errorf("no se han definido usuarios en la sección 'users'")
	}
	if len(cfg.Actions) == 0 {
		return fmt.Errorf("no se han definido acciones en la sección 'actions'")
	}

	// Primero, validamos todos los usuarios.
	for i := range cfg.Users {
		user := &cfg.Users[i] // Usamos un puntero para modificar el slice original

		if user.Name == "" {
			return fmt.Errorf("el usuario en la posición %d no tiene nombre ('name')", i)
		}
		if user.PublicKeyB64 == "" {
			return fmt.Errorf("el usuario '%s' no tiene clave pública ('public_key')", user.Name)
		}

		pkBytes, err := base64.StdEncoding.DecodeString(user.PublicKeyB64)
		if err != nil {
			return fmt.Errorf("la clave pública del usuario '%s' no es un Base64 válido: %w", user.Name, err)
		}
		if len(pkBytes) != ed25519.PublicKeySize {
			return fmt.Errorf("la clave pública del usuario '%s' tiene un tamaño incorrecto: se esperaban %d bytes, tiene %d", user.Name, ed25519.PublicKeySize, len(pkBytes))
		}
		user.DecodedPublicKey = ed25519.PublicKey(pkBytes)

		if len(user.AllowedActions) == 0 {
			return fmt.Errorf("el usuario '%s' no tiene acciones permitidas ('actions')", user.Name)
		}

		actionSet := make(map[string]struct{})
		for _, action := range user.AllowedActions {
			if _, exists := actionSet[action]; exists {
				return fmt.Errorf("el usuario '%s' tiene la acción duplicada: '%s'", user.Name, action)
			}
			actionSet[action] = struct{}{}
		}
	}

	// Segundo, validamos que cada acción permitida a un usuario exista en la sección global de 'actions'.
	// Esta es una validación de integridad referencial.
	for _, user := range cfg.Users {
		for _, actionID := range user.AllowedActions {
			if _, ok := cfg.Actions[actionID]; !ok {
				return fmt.Errorf("el usuario '%s' tiene permitida la acción '%s', pero esta acción no está definida en la sección global 'actions'", user.Name, actionID)
			}
		}
	}

	return nil
}
