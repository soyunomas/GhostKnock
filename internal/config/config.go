// El paquete config gestiona la carga y validación de la configuración del servidor.
package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"strings"

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
	Command            string   `yaml:"command"`
	RevertCommand      string   `yaml:"revert_command"`
	RevertDelaySeconds int      `yaml:"revert_delay_seconds"`
	TimeoutSeconds     int      `yaml:"timeout_seconds,omitempty"`
	// Se cambia a puntero (*int) para distinguir entre 0 (sin cooldown explícito) y nil (usar global).
	CooldownSeconds    *int     `yaml:"cooldown_seconds,omitempty"`
	RunAsUser          string   `yaml:"run_as_user,omitempty"`
	SensitiveParams    []string `yaml:"sensitive_params,omitempty"`
}

// Security define parámetros de seguridad ajustables.
type Security struct {
	ReplayWindowSeconds          int     `yaml:"replay_window_seconds"`
	DefaultActionCooldownSeconds int     `yaml:"default_action_cooldown_seconds"`
	RateLimitPerSecond           float64 `yaml:"rate_limit_per_second"`
	RateLimitBurst               int     `yaml:"rate_limit_burst"`
}

// Config es la estructura raíz de nuestro archivo de configuración.
type Config struct {
	ServerPrivateKeyPath string            `yaml:"server_private_key_path"`
	Listener             Listener          `yaml:"listener"`
	Logging              Logging           `yaml:"logging"`
	Daemon               Daemon            `yaml:"daemon"`
	Security             Security          `yaml:"security"`
	Users                []User            `yaml:"users"`
	Actions              map[string]Action `yaml:"actions"`
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
	SourceIPs        []string `yaml:"source_ips,omitempty"`
	DecodedPublicKey ed25519.PublicKey
	SourceCIDRs      []*net.IPNet
}

// userAlias es un truco para evitar un bucle infinito al llamar a Decode dentro de UnmarshalYAML.
type userAlias User

// UnmarshalYAML es nuestro decodificador personalizado para la struct User.
func (u *User) UnmarshalYAML(node *yaml.Node) error {
	// 1. Decodificar en el alias para obtener los valores básicos.
	var aux userAlias
	if err := node.Decode(&aux); err != nil {
		return err
	}

	// 2. Realizar validaciones lógicas.
	if aux.Name == "" {
		return fmt.Errorf("line %d: el campo 'name' del usuario no puede estar vacío", node.Line)
	}
	if aux.PublicKeyB64 == "" {
		return fmt.Errorf("line %d: el usuario '%s' no tiene clave pública ('public_key')", node.Line, aux.Name)
	}

	pkBytes, err := base64.StdEncoding.DecodeString(aux.PublicKeyB64)
	if err != nil {
		return fmt.Errorf("line %d: la clave pública del usuario '%s' no es un Base64 válido: %w", node.Line, aux.Name, err)
	}
	if len(pkBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("line %d: la clave pública del usuario '%s' tiene un tamaño incorrecto: se esperaban %d bytes, tiene %d", node.Line, aux.Name, ed25519.PublicKeySize, len(pkBytes))
	}
	aux.DecodedPublicKey = ed25519.PublicKey(pkBytes)

	if len(aux.AllowedActions) == 0 {
		return fmt.Errorf("line %d: el usuario '%s' no tiene acciones permitidas ('actions')", node.Line, aux.Name)
	}

	if len(aux.SourceIPs) > 0 {
		aux.SourceCIDRs = make([]*net.IPNet, 0, len(aux.SourceIPs))
		for _, ipStr := range aux.SourceIPs {
			_, cidr, err := net.ParseCIDR(ipStr)
			if err != nil {
				if net.ParseIP(ipStr) != nil {
					ipStr += "/32"
					_, cidr, err = net.ParseCIDR(ipStr)
				}
			}
			if err != nil {
				return fmt.Errorf("line %d: el usuario '%s' tiene una IP/CIDR inválida en 'source_ips': '%s'", node.Line, aux.Name, ipStr)
			}
			aux.SourceCIDRs = append(aux.SourceCIDRs, cidr)
		}
	}

	*u = User(aux)
	return nil
}

// LoadConfig lee y parsea el archivo de configuración YAML.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer el archivo de configuración en '%s': %w", path, err)
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		var typeErr *yaml.TypeError
		if errors.As(err, &typeErr) {
			var errorMessages []string
			for _, e := range typeErr.Errors {
				errorMessages = append(errorMessages, "  - "+e)
			}
			return nil, fmt.Errorf("error de sintaxis en el archivo de configuración YAML:\n%s", strings.Join(errorMessages, "\n"))
		}
		return nil, fmt.Errorf("error al parsear la configuración: %w", err)
	}

	// Valores por defecto de seguridad
	if cfg.Security.ReplayWindowSeconds == 0 {
		cfg.Security.ReplayWindowSeconds = 5
	}
	if cfg.Security.DefaultActionCooldownSeconds == 0 {
		cfg.Security.DefaultActionCooldownSeconds = 15
	}
	if cfg.Security.RateLimitPerSecond == 0 {
		cfg.Security.RateLimitPerSecond = 1.0
	}
	if cfg.Security.RateLimitBurst == 0 {
		cfg.Security.RateLimitBurst = 3
	}

	if err := validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("configuración inválida: %w", err)
	}

	return &cfg, nil
}

func validateConfig(cfg *Config) error {
	if cfg.ServerPrivateKeyPath == "" {
		return errors.New("el campo 'server_private_key_path' es obligatorio en la configuración")
	}
	if _, err := os.Stat(cfg.ServerPrivateKeyPath); os.IsNotExist(err) {
		return fmt.Errorf("el archivo de clave privada del servidor '%s' no existe", cfg.ServerPrivateKeyPath)
	}

	// Se ha mantenido la corrección anterior de validación de interfaz
	if cfg.Listener.Interface == "" {
		return errors.New("el campo 'listener.interface' es obligatorio en la configuración")
	}

	if cfg.Listener.Port <= 0 || cfg.Listener.Port > 65535 {
		return fmt.Errorf("puerto de escucha inválido: %d", cfg.Listener.Port)
	}

	if len(cfg.Users) == 0 {
		return fmt.Errorf("no se han definido usuarios en la sección 'users'")
	}
	if len(cfg.Actions) == 0 {
		return fmt.Errorf("no se han definido acciones en la sección 'actions'")
	}

	for actionName, action := range cfg.Actions {
		if action.RunAsUser != "" {
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
