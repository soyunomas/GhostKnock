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
	"text/template" // <<-- NUEVO IMPORT

	"gopkg.in/yaml.v3"
)

// Tuning agrupa constantes de rendimiento configurables para escalar desde IoT hasta Enterprise.
type Tuning struct {
	// Requiere RESTART (Memoria estática)
	PacketChannelBuffer int `yaml:"packet_channel_buffer"` // Default: 100

	// Requiere RESTART (Configuración de Driver/Socket)
	PcapTimeoutMs int `yaml:"pcap_timeout_ms"` // Default: 300

	// Soporta RELOAD (Lógica dinámica)
	MaxTrackedIPs             int `yaml:"max_tracked_ips"`              // Default: 20000
	EvictionBatchSize         int `yaml:"eviction_batch_size"`          // Default: 2000
	CacheCleanupSeconds       int `yaml:"cache_cleanup_seconds"`        // Default: 60
	LimiterCleanupSeconds     int `yaml:"limiter_cleanup_seconds"`      // Default: 180
	LimiterEvictionAgeSeconds int `yaml:"limiter_eviction_age_seconds"` // Default: 300
}

// Daemon define la configuración del comportamiento del proceso del servidor.
type Daemon struct {
	PIDFile   string `yaml:"pid_file,omitempty"`
	ShellPath string `yaml:"shell_path"` // ej: "/bin/sh", "/bin/bash"
	ShellFlag string `yaml:"shell_flag"` // ej: "-c"
}

// Logging define la configuración para los registros del servidor.
type Logging struct {
	LogLevel  string `yaml:"log_level"`
	LogFile   string `yaml:"log_file"`   // Ruta absoluta, "stdout" o "/dev/null"
	LogFormat string `yaml:"log_format"` // "text" o "json"
}

// Hooks define scripts externos que se ejecutan en diferentes puntos del ciclo de vida global.
type Hooks struct {
	// Se ejecuta ANTES de la acción. Si exit code != 0, cancela la ejecución.
	PreExecute string `yaml:"pre_execute,omitempty"`

	// Se ejecuta INMEDIATAMENTE DESPUÉS de que el comando principal termine con éxito (exit 0).
	OnSuccess string `yaml:"on_success,omitempty"`

	// Se ejecuta si el comando principal falla o hace timeout.
	OnError string `yaml:"on_error,omitempty"`

	// Se ejecuta DESPUÉS de que termine el comando de reversión (revert_command).
	OnRevert string `yaml:"on_revert,omitempty"`
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

	// --- HOOKS ESPECÍFICOS DE ACCIÓN (v2.2) ---
	PreHook    string `yaml:"pre_hook,omitempty"`
	PostHook   string `yaml:"post_hook,omitempty"`
	RevertHook string `yaml:"revert_hook,omitempty"`

	// --- CAMPOS INTERNOS (OPTIMIZACIÓN v2.1) ---
	// Estos campos NO se leen del YAML, se generan al cargar la configuración.
	// Almacenan los templates pre-compilados para evitar parsing en tiempo de ejecución.
	CommandTmpl       *template.Template `yaml:"-"`
	RevertCommandTmpl *template.Template `yaml:"-"`
}

// Security define parámetros de seguridad ajustables.
type Security struct {
	ReplayWindowSeconds          int     `yaml:"replay_window_seconds"`
	DefaultActionCooldownSeconds int     `yaml:"default_action_cooldown_seconds"`
	RateLimitPerSecond           float64 `yaml:"rate_limit_per_second"`
	RateLimitBurst               int     `yaml:"rate_limit_burst"`

	// --- NUEVO: Lista Negra (Blacklist) ---
	// Lista cruda del YAML (ej. "1.2.3.4", "10.0.0.0/8")
	DenyIPs []string `yaml:"deny_ips"`

	// Estructuras internas optimizadas para búsqueda rápida (No se leen del YAML)
	deniedIPMap   map[string]struct{}
	deniedSubnets []*net.IPNet
}

// Config es la estructura raíz de nuestro archivo de configuración.
type Config struct {
	ServerPrivateKeyPath string            `yaml:"server_private_key_path"`
	Listener             Listener          `yaml:"listener"`
	Logging              Logging           `yaml:"logging"`
	Daemon               Daemon            `yaml:"daemon"`
	Tuning               Tuning            `yaml:"tuning"` // Nuevo: Configuración de rendimiento
	Security             Security          `yaml:"security"`
	GlobalHooks          Hooks             `yaml:"hooks"` // Configuración global de Hooks
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

	// =========================================================================
	// SANE DEFAULTS (Valores históricos de v2.0 para compatibilidad)
	// =========================================================================

	// --- Security Defaults ---
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

	// --- Tuning Defaults (Performance) ---
	if cfg.Tuning.PacketChannelBuffer <= 0 {
		cfg.Tuning.PacketChannelBuffer = 100
	}
	if cfg.Tuning.PcapTimeoutMs <= 0 {
		cfg.Tuning.PcapTimeoutMs = 300
	}
	if cfg.Tuning.MaxTrackedIPs <= 0 {
		cfg.Tuning.MaxTrackedIPs = 20000
	}
	if cfg.Tuning.EvictionBatchSize <= 0 {
		cfg.Tuning.EvictionBatchSize = 2000
	}
	if cfg.Tuning.CacheCleanupSeconds <= 0 {
		cfg.Tuning.CacheCleanupSeconds = 60
	}
	if cfg.Tuning.LimiterCleanupSeconds <= 0 {
		cfg.Tuning.LimiterCleanupSeconds = 180
	}
	if cfg.Tuning.LimiterEvictionAgeSeconds <= 0 {
		cfg.Tuning.LimiterEvictionAgeSeconds = 300
	}

	// --- Daemon Defaults ---
	if cfg.Daemon.ShellPath == "" {
		cfg.Daemon.ShellPath = "/bin/sh"
	}
	if cfg.Daemon.ShellFlag == "" {
		cfg.Daemon.ShellFlag = "-c"
	}

	// --- Logging Defaults ---
	if cfg.Logging.LogFile == "" {
		cfg.Logging.LogFile = "/var/log/ghostknockd.log"
	}
	if cfg.Logging.LogFormat == "" {
		cfg.Logging.LogFormat = "text"
	}
	if cfg.Logging.LogLevel == "" {
		cfg.Logging.LogLevel = "info"
	}

	// =========================================================================

	// --- PROCESAMIENTO DE LISTA NEGRA (Parseo eficiente) ---
	cfg.Security.deniedIPMap = make(map[string]struct{})
	cfg.Security.deniedSubnets = make([]*net.IPNet, 0)

	for _, entry := range cfg.Security.DenyIPs {
		// 1. Intentar parsear como CIDR (ej. 192.168.1.0/24)
		_, ipNet, err := net.ParseCIDR(entry)
		if err == nil {
			cfg.Security.deniedSubnets = append(cfg.Security.deniedSubnets, ipNet)
			continue
		}

		// 2. Intentar parsear como IP única (ej. 192.168.1.50)
		ip := net.ParseIP(entry)
		if ip != nil {
			cfg.Security.deniedIPMap[ip.String()] = struct{}{}
			continue
		}

		return nil, fmt.Errorf("entrada inválida en 'deny_ips': '%s' no es una IP ni un CIDR válido", entry)
	}

	// --- OPTIMIZACIÓN v2.1: Pre-compilación de Templates ---
	// Iteramos sobre las acciones para compilar los templates una sola vez al inicio.
	// Esto ahorra CPU en tiempo de ejecución y permite "Fail-Fast" si la sintaxis es mala.
	for name, action := range cfg.Actions {
		// Compilar Comando Principal
		if action.Command == "" {
			return nil, fmt.Errorf("la acción '%s' tiene un comando vacío", name)
		}
		tmpl, err := template.New("cmd-" + name).Parse(action.Command)
		if err != nil {
			return nil, fmt.Errorf("error de sintaxis en el template de la acción '%s': %w", name, err)
		}
		action.CommandTmpl = tmpl

		// Compilar Comando de Reversión (si existe)
		if action.RevertCommand != "" {
			revTmpl, err := template.New("rev-" + name).Parse(action.RevertCommand)
			if err != nil {
				return nil, fmt.Errorf("error de sintaxis en el template de reversión de la acción '%s': %w", name, err)
			}
			action.RevertCommandTmpl = revTmpl
		}

		// Guardar cambios en el mapa (necesario porque 'action' es una copia del valor)
		cfg.Actions[name] = action
	}

	// -------------------------------------------------------

	if err := validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("configuración inválida: %w", err)
	}

	return &cfg, nil
}

// IsIPDenied comprueba eficientemente si una IP está en la lista negra.
func (c *Config) IsIPDenied(ip net.IP) bool {
	// 1. Búsqueda O(1) en mapa de IPs exactas
	if _, ok := c.Security.deniedIPMap[ip.String()]; ok {
		return true
	}

	// 2. Búsqueda O(N) en lista de subredes
	// Esto sigue siendo muy rápido comparado con la criptografía.
	for _, subnet := range c.Security.deniedSubnets {
		if subnet.Contains(ip) {
			return true
		}
	}

	return false
}

func validateConfig(cfg *Config) error {
	if cfg.ServerPrivateKeyPath == "" {
		return errors.New("el campo 'server_private_key_path' es obligatorio en la configuración")
	}
	if _, err := os.Stat(cfg.ServerPrivateKeyPath); os.IsNotExist(err) {
		return fmt.Errorf("el archivo de clave privada del servidor '%s' no existe", cfg.ServerPrivateKeyPath)
	}

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
