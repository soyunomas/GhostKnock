# Hoja de Ruta Técnica: Stealth & Hardening (v2.1.x / v2.2)

## 🎯 Objetivo
Implementar capas adicionales de seguridad (2FA, Ofuscación), flexibilidad operativa y validación estricta sin romper la compatibilidad con el protocolo actual.
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

## 📅 FASE 6: Validación Granular de Parámetros (Regex Custom)

Actualmente, GhostKnock usa una lista blanca estricta (alfanumérica) para los argumentos. Esto impide pasar correos electrónicos, URLs o UUIDs.

### 6.1. Definición de Validadores en Config
Permitir definir expresiones regulares personalizadas por acción y parámetro.

*   Actualizar `Action` en `config.go`:
    ```yaml
    actions:
      "create-email":
        command: "/bin/add_mail.sh {{.Params.email}}"
        # Validadores opcionales por parámetro
        validators:
          email: "^[a-z0-9._%+-]+@[a-z0-9.-]+\\.[a-z]{2,4}$"
    ```
*   **Executor:** Si existe un validador para un parámetro, usarlo en lugar de la `safeParamRegex` por defecto.

---

## 📅 FASE 7: Filtrado Geoespacial (GeoIP Hardening)

Para servidores expuestos globalmente, reducir la superficie de ataque bloqueando países enteros antes de la verificación criptográfica.

### 7.1. Integración con Base de Datos MMDB
*   Añadir soporte (opcional) para leer bases de datos GeoLite2 (MaxMind).
*   Nueva sección en `config.yaml`:
    ```yaml
    security:
      geoip_db_path: "/var/lib/GeoIP/GeoLite2-Country.mmdb"
      allow_countries: ["ES", "US", "FR"] # ISO Codes
    ```
*   **Listener Middleware:** En `processKnock`, consultar la IP en la DB local. Si el país no está en la lista blanca -> `return` inmediato (Short-circuit).
