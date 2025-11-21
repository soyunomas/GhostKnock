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
	Command            string `yaml:"command"`
	RevertCommand      string `yaml:"revert_command"`
	RevertDelaySeconds int    `yaml:"revert_delay_seconds"`
	TimeoutSeconds     int    `yaml:"timeout_seconds,omitempty"`
	CooldownSeconds    int    `yaml:"cooldown_seconds,omitempty"`
	RunAsUser          string `yaml:"run_as_user,omitempty"`
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
	Listener Listener          `yaml:"listener"`
	Logging  Logging           `yaml:"logging"`
	Daemon   Daemon            `yaml:"daemon"`
	Security Security          `yaml:"security"`
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
	SourceIPs        []string `yaml:"source_ips,omitempty"`
	DecodedPublicKey ed25519.PublicKey
	SourceCIDRs      []*net.IPNet
}

// <<-- INICIO: NUEVA LÓGICA DE VALIDACIÓN CON NÚMEROS DE LÍNEA -->>

// userAlias es un truco para evitar un bucle infinito al llamar a Decode dentro de UnmarshalYAML.
type userAlias User

// UnmarshalYAML es nuestro decodificador personalizado para la struct User.
// Se ejecuta durante el parseo de YAML, dándonos acceso al nodo y su número de línea.
func (u *User) UnmarshalYAML(node *yaml.Node) error {
	// 1. Decodificar en el alias para obtener los valores básicos.
	var aux userAlias
	if err := node.Decode(&aux); err != nil {
		// Este error ya tendrá el número de línea si hay un problema de tipo.
		return err
	}

	// 2. Realizar nuestras validaciones de LÓGICA sobre los datos decodificados.
	if aux.Name == "" {
		return fmt.Errorf("line %d: el campo 'name' del usuario no puede estar vacío", node.Line)
	}
	if aux.PublicKeyB64 == "" {
		return fmt.Errorf("line %d: el usuario '%s' no tiene clave pública ('public_key')", node.Line, aux.Name)
	}

	// Validación de Base64 (¡el problema que encontraste!)
	pkBytes, err := base64.StdEncoding.DecodeString(aux.PublicKeyB64)
	if err != nil {
		return fmt.Errorf("line %d: la clave pública del usuario '%s' no es un Base64 válido: %w", node.Line, aux.Name, err)
	}
	if len(pkBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("line %d: la clave pública del usuario '%s' tiene un tamaño incorrecto: se esperaban %d bytes, tiene %d", node.Line, aux.Name, ed25519.PublicKeySize, len(pkBytes))
	}
	aux.DecodedPublicKey = ed25519.PublicKey(pkBytes) // Guardamos la clave decodificada

	if len(aux.AllowedActions) == 0 {
		return fmt.Errorf("line %d: el usuario '%s' no tiene acciones permitidas ('actions')", node.Line, aux.Name)
	}

	// Validación de Source IPs
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

	// 3. Si todo está bien, copiamos los datos del alias al struct original.
	*u = User(aux)
	return nil
}

// <<-- FIN: NUEVA LÓGICA DE VALIDACIÓN -->>

// LoadConfig lee y parsea el archivo de configuración YAML desde la ruta especificada.
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
		// Ahora los errores de lógica también tendrán número de línea gracias a UnmarshalYAML
		return nil, fmt.Errorf("error al parsear la configuración: %w", err)
	}

	// Establecer valores por defecto para la sección de seguridad
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

// validateConfig ahora se enfoca en validaciones GLOBALES que cruzan diferentes secciones.
func validateConfig(cfg *Config) error {
	if cfg.Listener.Port <= 0 || cfg.Listener.Port > 65535 {
		return fmt.Errorf("puerto de escucha inválido: %d", cfg.Listener.Port)
	}
	// ... (otras validaciones de listener y logging se mantienen aquí) ...

	if len(cfg.Users) == 0 {
		return fmt.Errorf("no se han definido usuarios en la sección 'users'")
	}
	if len(cfg.Actions) == 0 {
		return fmt.Errorf("no se han definido acciones en la sección 'actions'")
	}

	// Las validaciones específicas de 'user' se han movido a UnmarshalYAML.
	// Solo dejamos aquí las validaciones que dependen de otras secciones del config.
	
	for actionName, action := range cfg.Actions {
		if action.RunAsUser != "" {
			if _, err := user.Lookup(action.RunAsUser); err != nil {
				// No tenemos el número de línea aquí, pero es una validación del sistema, no del YAML.
				return fmt.Errorf("la acción '%s' especifica 'run_as_user' con un usuario ('%s') que no existe en el sistema: %w", actionName, action.RunAsUser, err)
			}
		}
	}

	// Validación CRÍTICA: Asegurarse de que las acciones de un usuario existan en la sección 'actions'.
	for _, user := range cfg.Users {
		for _, actionID := range user.AllowedActions {
			if _, ok := cfg.Actions[actionID]; !ok {
				// Este error es difícil de asociar a una línea, pero es una validación de consistencia.
				return fmt.Errorf("el usuario '%s' tiene permitida la acción '%s', pero esta acción no está definida en la sección global 'actions'", user.Name, actionID)
			}
		}
	}

	return nil
}
