// El paquete config gestiona la carga y validación de la configuración del servidor.
package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/user" // <<-- NUEVA IMPORTACIÓN

	"gopkg.in/yaml.v3"
)

// Daemon define la configuración del comportamiento del proceso del servidor.
type Daemon struct {
	PIDFile string `yaml:"pid_file,omitempty"`
}

// Logging define la configuración para los registros del servidor.
type Logging struct {
	LogLevel string `yaml:"log_level"`
}

// Action define una plantilla de comando y su comportamiento de reversión.
type Action struct {
	Command            string `yaml:"command"`
	RevertCommand      string `yaml:"revert_command"`
	RevertDelaySeconds int    `yaml:"revert_delay_seconds"`
	TimeoutSeconds     int    `yaml:"timeout_seconds,omitempty"`
	CooldownSeconds    int    `yaml:"cooldown_seconds,omitempty"`
	RunAsUser          string `yaml:"run_as_user,omitempty"` // <<-- NUEVO CAMPO
}

// Config es la estructura raíz de nuestro archivo de configuración.
type Config struct {
	Listener Listener          `yaml:"listener"`
	Logging  Logging           `yaml:"logging"`
	Daemon   Daemon            `yaml:"daemon"`
	Users    []User            `yaml:"users"`
	Actions  map[string]Action `yaml:"actions"`
}

// Listener define en qué interfaz y puerto escucha el servidor.
type Listener struct {
	Interface string `yaml:"interface"`
	Port      int    `yaml:"port"`
	ListenIP  string `yaml:"listen_ip,omitempty"`
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
	if cfg.Listener.ListenIP != "" {
		if net.ParseIP(cfg.Listener.ListenIP) == nil {
			return fmt.Errorf("el campo 'listen_ip' ('%s') no es una dirección IP válida", cfg.Listener.ListenIP)
		}
	}

	// Validación para la configuración de logging.
	if cfg.Logging.LogLevel == "" {
		// Asignar un valor por defecto si no se especifica.
		cfg.Logging.LogLevel = "info"
	}
	switch cfg.Logging.LogLevel {
	case "debug", "info", "warn", "error":
		// El valor es válido, no hacer nada.
	default:
		return fmt.Errorf("el valor de 'log_level' ('%s') es inválido; debe ser 'debug', 'info', 'warn' o 'error'", cfg.Logging.LogLevel)
	}

	if len(cfg.Users) == 0 {
		return fmt.Errorf("no se han definido usuarios en la sección 'users'")
	}
	if len(cfg.Actions) == 0 {
		return fmt.Errorf("no se han definido acciones en la sección 'actions'")
	}

	for i := range cfg.Users {
		user := &cfg.Users[i]

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

	for actionName, action := range cfg.Actions {
		if action.TimeoutSeconds < 0 {
			return fmt.Errorf("la acción '%s' tiene un 'timeout_seconds' negativo, lo cual no está permitido", actionName)
		}
		if action.CooldownSeconds < 0 {
			return fmt.Errorf("la acción '%s' tiene un 'cooldown_seconds' negativo, lo cual no está permitido", actionName)
		}
		// <<-- NUEVA VALIDACIÓN
		if action.RunAsUser != "" {
			if action.RunAsUser == "root" {
				return fmt.Errorf("la acción '%s' tiene 'run_as_user' configurado como 'root', lo cual está prohibido por seguridad", actionName)
			}
			if _, err := user.Lookup(action.RunAsUser); err != nil {
				return fmt.Errorf("la acción '%s' especifica 'run_as_user' con un usuario ('%s') que no existe en el sistema: %w", actionName, action.RunAsUser, err)
			}
		}
	}

	for _, user := range cfg.Users {
		for _, actionID := range user.AllowedActions {
			if _, ok := cfg.Actions[actionID]; !ok {
				return fmt.Errorf("el usuario '%s' tiene permitida la acción '%s', pero esta acción no está definida en la sección global 'actions'", user.Name, actionID)
			}
		}
	}

	return nil
}
