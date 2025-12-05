# Hoja de Ruta Técnica: Stealth & Hardening (v2.1.0)

## 🎯 Objetivo
Implementar capas adicionales de seguridad (2FA, Ofuscación) y flexibilidad operativa sin romper la compatibilidad con el protocolo v2.1.
Todas las mejoras deben ser **aditivas**: los clientes y servidores antiguos deben seguir coexistiendo.

---

## 📅 FASE 1: Blindaje del Sistema (Systemd Hardening)

Mejorar la seguridad del proceso demonio utilizando las capacidades de aislamiento de Linux. Esto no requiere cambios en el código Go, solo en el empaquetado.

### 1.1. Actualizar `packaging/systemd/ghostknockd.service`
Restringir los privilegios del proceso al mínimo absoluto.

*   **Capabilities:** Limitar a `CAP_NET_RAW` (para sniffing) y `CAP_SETUID/GID` (para cambiar de usuario).
*   **Filesystem:**
    *   `ProtectSystem=strict` (Montar `/usr`, `/boot`, `/etc` como Read-Only).
    *   `ProtectHome=true` (El demonio no debe ver `/home`).
    *   `PrivateTmp=true` (Aislamiento de `/tmp`).
*   **Network:** `RestrictAddressFamilies=AF_INET AF_INET6 AF_PACKET` (Prevenir sockets extraños).

---

## 📅 FASE 2: Ofuscación de Tráfico (Traffic Padding)

**Objetivo:** Evitar el análisis de tráfico basado en el tamaño del paquete (Side-Channel Attack). Hacer que todos los knocks tengan tamaños variables y aleatorios.

### 2.1. Modificar `internal/protocol/protocol.go`
*   Actualizar struct `Payload`:
    ```go
    type Payload struct {
        // ... campos existentes ...
        Padding string `json:"padding,omitempty"` // Ignorado por lógica, usado para entropía
    }
    ```

### 2.2. Actualizar Cliente (`cmd/ghostknock`)
*   Implementar generador de basura aleatoria (random bytes -> hex/base64).
*   Llenar el campo `Padding` con una longitud aleatoria (ej. entre 0 y 255 bytes) antes de cifrar.

### 2.3. Validación en Servidor
*   Confirmar que `json.Unmarshal` descarta silenciosamente el campo en servidores antiguos (compatibilidad garantizada).
*   En servidores nuevos, simplemente ignorar el campo tras descifrar.

---

## 📅 FASE 3: Autenticación de Segundo Factor (TOTP)

**Objetivo:** Mitigar el robo de claves privadas requiriendo un código temporal (Google Authenticator).

### 3.1. Actualizar `internal/config/config.go`
*   Añadir campo opcional a `User`:
    ```go
    type User struct {
        // ...
        TotpSecret string `yaml:"totp_secret,omitempty"` // Base32 Secret
    }
    ```

### 3.2. Actualizar Lógica (`cmd/ghostknockd/main.go` o nuevo middleware)
*   En `processKnock`, tras verificar la firma criptográfica:
    1.  Verificar si el usuario tiene `TotpSecret` configurado.
    2.  Si lo tiene, buscar el parámetro reservado `otp` en `payload.Params`.
    3.  Validar el código usando una librería estándar (ej. `github.com/pquerna/otp`).
    4.  Si falla o no existe -> Rechazar silenciosamente.

---

## 📅 FASE 4: Flexibilidad de Ejecución (Multi-Comandos y Horarios)

### 4.1. Acciones Multi-Comando (`internal/config`)
Permitir que una sola acción ejecute varios comandos secuenciales.

*   **Reto:** Mantener compatibilidad con `command: "string"`.
*   **Solución:** Implementar `UnmarshalYAML` personalizado para `Action`.
    *   Si el nodo YAML es string -> Convertir a `[]string{val}`.
    *   Si el nodo es lista -> Usar tal cual.
*   **Executor:** Actualizar `executor.go` para iterar sobre la lista de comandos. Si uno falla, detener la cadena.

### 4.2. Control Horario (Time-Based Access)
Restringir el acceso a ciertos usuarios por horario.

*   Actualizar `User` en `config.go`:
    ```go
    type User struct {
        // ...
        AllowedHours []string `yaml:"allowed_hours,omitempty"` // Ej: "09:00-17:00"
    }
    ```
*   Implementar validador de tiempo en el servidor antes de ejecutar la acción.

---

## 📅 FASE 5: Experiencia de Desarrollo (Dry-Run)

### 5.1. Flag `-dry-run`
Permitir arrancar el servidor en modo simulación para verificar configuraciones de firewall complejas sin riesgo.

*   Añadir flag en `main.go`.
*   Inyectar este estado en `executor.Execute`.
*   Si `dryRun == true`:
    *   No ejecutar `exec.Command`.
    *   Loguear: `[DRY-RUN] WOULDA EXECUTED: iptables -I INPUT...`
    *   No ejecutar reversiones reales, solo simular el delay.

---

## 🧪 Pruebas de Regresión (Checklist)

Para asegurar que v2.2.0 no rompe v2.1.0:
- [ ] Cliente v2.1 enviando a Servidor v2.2 (Sin Padding, sin TOTP) -> **Debe funcionar.**
- [ ] Cliente v2.2 enviando a Servidor v2.1 (Con Padding) -> **Debe funcionar** (JSON extra ignorado).
- [ ] Configuración v2.1 cargada en Servidor v2.2 -> **Debe funcionar** (Campos nuevos opcionales).
