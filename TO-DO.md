# Hoja de Ruta Técnica: Configuración Avanzada y Hardening (v2.1.0)

## 🎯 Objetivo
Eliminar todas las constantes "hardcoded" del código fuente (`const`). Esto permitirá:
1.  **Escalabilidad:** Adaptar el rendimiento (buffers, timeouts) desde IoT (Raspberry Pi) hasta Servidores Enterprise.
2.  **Observabilidad:** Soportar logs estructurados (JSON) para ingestión en SIEM (ELK, Datadog, Splunk).
3.  **Portabilidad:** Permitir el uso de shells alternativos (ej. `ash` en Alpine, `zsh`, o shells restringidos) en lugar de depender de `/bin/sh`.

---

## 📅 FASE 1: Actualización del Modelo de Datos (`internal/config`)

Modificar las estructuras para soportar los nuevos parámetros.

### 1.1. Modificar `internal/config/config.go`

**A. Nueva Struct `Tuning` (Performance)**
Agrupa las constantes de rendimiento.
```go
type Tuning struct {
    // Requiere RESTART (Memoria estática)
    PacketChannelBuffer int `yaml:"packet_channel_buffer"` // Default: 100
    
    // Requiere RESTART (Configuración de Driver/Socket)
    PcapTimeoutMs       int `yaml:"pcap_timeout_ms"`       // Default: 300
    
    // Soporta RELOAD (Lógica dinámica)
    MaxTrackedIPs             int `yaml:"max_tracked_ips"`              // Default: 20000
    EvictionBatchSize         int `yaml:"eviction_batch_size"`          // Default: 2000
    CacheCleanupSeconds       int `yaml:"cache_cleanup_seconds"`        // Default: 60
    LimiterCleanupSeconds     int `yaml:"limiter_cleanup_seconds"`      // Default: 180
    LimiterEvictionAgeSeconds int `yaml:"limiter_eviction_age_seconds"` // Default: 300
}
```

**B. Actualizar Struct `Logging`**
```go
type Logging struct {
    LogLevel  string `yaml:"log_level"`  // "debug", "info", "warn", "error"
    LogFile   string `yaml:"log_file"`   // Ruta absoluta, "stdout" o "/dev/null"
    LogFormat string `yaml:"log_format"` // "text" (human readable) o "json" (machine readable)
}
```

**C. Actualizar Struct `Daemon`**
```go
type Daemon struct {
    PIDFile   string `yaml:"pid_file,omitempty"`
    ShellPath string `yaml:"shell_path"` // ej: "/bin/bash", "/bin/ash"
    ShellFlag string `yaml:"shell_flag"` // ej: "-c"
}
```

**D. Actualizar Struct `Config` (Raíz)**
```go
type Config struct {
    // ... campos existentes ...
    Daemon Daemon `yaml:"daemon"`
    Tuning Tuning `yaml:"tuning"` // Nuevo
}
```

### 1.2. Implementar "Sane Defaults" en `LoadConfig`
**CRÍTICO:** Para mantener compatibilidad hacia atrás, si los valores son 0 o vacíos, se deben asignar los defaults históricos.

```go
// Lógica de Defaults:
if cfg.Tuning.PacketChannelBuffer <= 0 { cfg.Tuning.PacketChannelBuffer = 100 }
if cfg.Tuning.PcapTimeoutMs <= 0 { cfg.Tuning.PcapTimeoutMs = 300 }
// ... (resto de defaults de Tuning)

if cfg.Daemon.ShellPath == "" { cfg.Daemon.ShellPath = "/bin/sh" }
if cfg.Daemon.ShellFlag == "" { cfg.Daemon.ShellFlag = "-c" }

if cfg.Logging.LogFile == "" { cfg.Logging.LogFile = "/var/log/ghostknockd.log" }
if cfg.Logging.LogFormat == "" { cfg.Logging.LogFormat = "text" }
```

---

## 📅 FASE 2: Refactorización del Ejecutor (`internal/executor`)

Hacer que el intérprete de comandos sea configurable.

### 2.1. Actualizar `runCommand`
Eliminar el hardcode `"/bin/sh", "-c"`.

*   **Nueva Firma:**
    `func runCommand(..., shellPath string, shellFlag string)`
*   **Implementación:**
    `cmd := exec.CommandContext(ctx, shellPath, shellFlag, finalCommand)`

### 2.2. Propagar Cambios en `Execute` y `scheduleRevert`
Ambas funciones deben recibir `config.Daemon` y pasarlo a `runCommand`.

---

## 📅 FASE 3: Refactorización del Servidor (`cmd/ghostknockd/main.go`)

Conectar la configuración dinámica y gestionar el ciclo de vida.

### 3.1. Limpieza de Constantes
Eliminar el bloque `const` global (`packetChannelBuffer`, `pcapTimeout`, etc.).

### 3.2. Actualizar `setupLogging` (Soporte JSON)
Reescribir para soportar cambio de formato y destino.

```go
func setupLogging(cfg config.Logging) {
    var writer io.Writer
    // Lógica para abrir cfg.LogFile (O_APPEND), stdout o Discard
    // ...
    
    opts := &slog.HandlerOptions{Level: parseLevel(cfg.LogLevel)}
    var handler slog.Handler
    
    if cfg.LogFormat == "json" {
        handler = slog.NewJSONHandler(writer, opts)
    } else {
        handler = slog.NewTextHandler(writer, opts)
    }
    slog.SetDefault(slog.New(handler))
}
```

### 3.3. Inicialización en `main()`
Usar `cfg.Tuning` para inicializar canales y tickers.

```go
// Inicialización del canal con tamaño dinámico
packetsCh := make(chan listener.PacketInfo, cfg.Tuning.PacketChannelBuffer)
```

### 3.4. Lógica de `reloadConfig` (Hot-Reload)
Detectar cambios que **requieren reinicio** y avisar al usuario.

```go
func (s *Server) reloadConfig() {
    newCfg, _ := config.LoadConfig(...)
    
    s.configMutex.Lock()
    defer s.configMutex.Unlock()

    // Detectar cambios estáticos (Red/Memoria)
    needsRestart := false
    if newCfg.Listener != s.config.Listener { needsRestart = true }
    if newCfg.Tuning.PacketChannelBuffer != s.config.Tuning.PacketChannelBuffer { needsRestart = true }
    if newCfg.Tuning.PcapTimeoutMs != s.config.Tuning.PcapTimeoutMs { needsRestart = true }

    if needsRestart {
        slog.Warn("RELOAD PARCIAL: Cambios detectados en Listener o Tuning (Buffer/Timeout). Estos cambios requieren reiniciar el servicio (systemctl restart). Se aplicarán solo los cambios lógicos.")
    }

    // Aplicar configuración
    s.config = newCfg
    setupLogging(newCfg.Logging) // El logging sí se recarga al vuelo
}
```

---

## 📅 FASE 4: Actualización del Listener (`internal/listener`)

### 4.1. Timeout Dinámico en `Start()`
*   **Nueva Firma:** `func Start(..., pcapTimeoutMs int, ...)`
*   **Uso:** Convertir a `time.Duration` y pasar a `pcap.OpenLive`.

---

## 📅 FASE 5: Documentación y Ejemplo

Actualizar `config.yaml.example` con las nuevas secciones.

```yaml
daemon:
  pid_file: "/var/run/ghostknockd.pid"
  shell_path: "/bin/bash" # Opcional
  shell_flag: "-c"        # Opcional

logging:
  log_level: "info"
  log_file: "/var/log/ghostknockd.log" # O "stdout"
  log_format: "json"      # O "text"

tuning:
  packet_channel_buffer: 500  # Aumentar para tráfico alto
  pcap_timeout_ms: 100
  max_tracked_ips: 50000
```
