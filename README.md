# 👻 GhostKnock

[![Licencia: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/badge/release-v2.1.0-blue.svg)](https://github.com/soyunomas/GhostKnock/releases)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20windows-lightgrey.svg)]()

**GhostKnock** es un sistema de **ejecución remota segura, invisible y confidencial**.

Permite disparar comandos predefinidos en un servidor enviando un único paquete UDP cifrado.

El servidor escucha pasivamente el tráfico. Si recibe un paquete con una firma válida y un payload cifrado para él, lo descifra y ejecuta la acción asociada. Si no, el paquete es ignorado silenciosamente, haciendo que el servidor sea **indetectable** y su comunicación **indescifrable**.

---

## ✨ Características

*   🛡️ **Invisible por Diseño (Stealth):**
    *   **Sin Puertos Abiertos:** El servidor no mantiene puertos "a la escucha" en el sentido tradicional. No aparecerá en herramientas de monitorización (como `netstat`) ni responderá a intentos de conexión.
    *   **Indetectable externamente:** Ante un escaneo de red, el puerto parecerá estar **cerrado** o filtrado. El servidor captura los paquetes de forma pasiva, procesando silenciosamente solo aquellos que son legítimos y descartando el resto sin emitir respuesta.

*   🚀 **Monitorización Pasiva y Eficiente:**
    *   **Filtrado en Origen:** GhostKnock aplica filtros a nivel de sistema operativo (BPF). El kernel solo notifica a la aplicación cuando llega un paquete UDP al puerto exacto, garantizando un consumo de CPU prácticamente nulo incluso en redes con mucho tráfico.
    *   **Protección Short-Circuit (Nueva v2.1):** Capacidad de definir una lista negra (`deny_ips`) que descarta tráfico de atacantes conocidos *antes* de realizar cualquier operación criptográfica, ahorrando recursos de CPU.

*   🔧 **Tuning y Escalabilidad (Nuevo v2.1):**
    *   **Gestión de Recursos:** Sección `tuning` dedicada para ajustar buffers de red, timeouts de captura (`pcap`) y límites de memoria. Permite escalar desde dispositivos IoT (Raspberry Pi) hasta servidores Enterprise con alto tráfico.
    *   **Logging Flexible:** Soporte nativo para logs estructurados en **JSON** (para SIEMs como ELK/Datadog) y redirección a `stdout` o ficheros.

*   🔐 **Seguridad y Privacidad (Hardening):**
    *   **Cifrado de Extremo a Extremo:** Utiliza estándares modernos (`Ed25519` + `X25519`) para autenticación y confidencialidad. Solo el servidor puede leer qué comando estás enviando.
    *   **Anti-Replay Híbrido:** Sistema de doble verificación (lectura rápida + bloqueo de escritura) que detecta y rechaza paquetes duplicados para evitar la reutilización de credenciales.
    *   **Memoria Blindada (Anti-OOM):** Arquitectura diseñada para evitar el agotamiento de memoria ante ataques masivos, con purga automática de tablas de rastreo y límites estrictos configurables (`max_tracked_ips`).

*   🪝 **Sistema de Hooks (Event Driven):**
    *   **Integración y Auditoría:** Ejecuta scripts externos antes (`pre_execute`), después (`on_success`/`on_error`) o al revertir (`on_revert`) una acción.
    *   **Contexto Inyectado:** Los scripts reciben automáticamente variables de entorno con el usuario, IP, acción y parámetros (`GK_USER`, `GK_IP`, `GK_PARAM_*`). Ideal para notificaciones (Telegram, Slack) o logs centralizados.

*   👮 **Principio de Mínimo Privilegio:**
    *   Puedes configurar comandos para que se ejecuten como usuarios restringidos (ej. `www-data`), limitando el impacto en el sistema.
    *   **Shell Personalizable (Nuevo v2.1):** Posibilidad de definir el intérprete de comandos (ej. `/bin/ash`, `/bin/rbash`) para entornos minimalistas o restringidos.

*   🔄 **Gestión en Caliente (Hot Reload):**
    *   Permite añadir usuarios, rotar claves, modificar acciones y ajustar parámetros de logging editando la configuración y recargando el servicio (`systemctl reload`) **sin detener el servicio** y manteniendo intacta la caché de seguridad.

*   ⚡ **Perfiles de Cliente:**
    *   El cliente CLI soporta un archivo de configuración (`profiles.yaml`) para definir alias de conexión, evitando tener que escribir IPs y rutas de claves repetidamente.

---

## ⚠️ Comportamiento de Seguridad y Limitaciones

GhostKnock ha sido diseñado priorizando la **supervivencia del servidor** sobre la disponibilidad.

| Limitación | Escenario del Usuario | Comportamiento | Razón de Seguridad |
| :--- | :--- | :--- | :--- |
| **Límite de Procesos** | Se intentan lanzar más de 10 comandos simultáneos. | **RECHAZADO.** El servidor ignora el comando. | **Anti-Fork Bomb.** Protege la tabla de procesos del OS. |
| **Rate Limit (IP)** | Múltiples comandos en < 1 seg desde la misma IP. | **SILENCIO TOTAL.** Se ignoran las ráfagas. | **Anti-DoS.** Protege CPU y Memoria. |
| **Lista Negra (IP)** | IP presente en `deny_ips` envía tráfico. | **DESCARTE INMEDIATO.** | **Short-Circuit.** Ahorro total de CPU. |
| **Replay Cache** | Se reenvía un paquete idéntico. | **IGNORADO.** | **Anti-Replay.** Evita reutilización de credenciales. |
| **Timeout Forzoso** | Un script se cuelga indefinidamente. | **KILL.** Proceso eliminado (default 30s). | **Recuperación de Recursos.** |
| **Recarga (Reload)** | Se recarga configuración con `SIGHUP`. | **PERSISTE.** La caché de seguridad se mantiene. | **Continuidad.** No se pierden protecciones. |

---

## 📦 Instalación

### Opción A: Paquetes .deb (Debian/Ubuntu/Mint)

Descarga la última versión desde [Releases](https://github.com/soyunomas/GhostKnock/releases).

*   **Para el Servidor (Demonio + Herramientas):**
    ```bash
    sudo dpkg -i ghostknock_2.1.0_amd64.deb
    # Se instala el servicio systemd, logrotate y se asegura /etc/ghostknock
    ```

*   **Para Clientes Remotos (Solo Herramientas):**
    ```bash
    sudo dpkg -i ghostknock-client_2.1.0_amd64.deb
    ```

### Opción B: Ejecutables para Windows

Descarga `ghostknock.exe` y `ghostknock-keygen.exe` desde Releases. No requieren instalación.

### Opción C: Compilación Manual

```bash
make build          # Compila para Linux
make build-windows  # Compila .exe para Windows
```

---

## 🧱 Configuración del Firewall (Crucial para Invisibilidad)

Para que GhostKnock sea verdaderamente invisible, **el sistema operativo no debe responder** cuando reciba un paquete en el puerto UDP configurado.

Si no configuras el firewall, tu servidor Linux responderá con un mensaje ICMP "Port Unreachable", revelando a un atacante que el servidor existe.

### Si usas UFW (Ubuntu/Debian)
```bash
# Denegar explícitamente el tráfico UDP en el puerto 3001
sudo ufw deny 3001/udp
sudo ufw reload
```

### Si usas iptables puro
```bash
# Insertar regla para DESCARTAR paquetes.
# GhostKnock (libpcap) verá el paquete antes de que iptables lo tire.
sudo iptables -I INPUT -p udp --dport 3001 -j DROP
```

---

## 🚀 Guía de Inicio Rápido

### 1. Generar la Identidad del Servidor (En el Servidor)
```bash
sudo ghostknock-keygen -o /etc/ghostknock/server_key
sudo chmod 600 /etc/ghostknock/server_key*
```
> **Comparte `/etc/ghostknock/server_key.pub` con los clientes.**

### 2. Generar tu Identidad de Cliente (En tu PC)
```bash
ghostknock-keygen
# Copia la cadena Base64 pública que aparece.
```

### 3. Configurar el Servidor (`/etc/ghostknock/config.yaml`)
```yaml
server_private_key_path: "/etc/ghostknock/server_key"

users:
  - name: "admin"
    public_key: "TU_CLAVE_PUBLICA_BASE64..."
    actions: ["open-ssh", "ban-ip"]

actions:
  "open-ssh":
    command: "iptables -I INPUT 1 -p tcp -s {{.SourceIP}} --dport 22 -j ACCEPT"
    revert_command: "iptables -D INPUT -p tcp -s {{.SourceIP}} --dport 22 -j ACCEPT"
    revert_delay_seconds: 300
```

### 4. Enviar Knock Cifrado
```bash
ghostknock -host MISERVIDOR \
           -server-pubkey server_key.pub \
           -action open-ssh
```

---

## ⚡ Uso Avanzado: Perfiles de Cliente

Para evitar escribir la IP, puerto y ruta de claves en cada comando, GhostKnock v2.1 introduce los **Perfiles**.

### 1. Crear el archivo de configuración
Crea un archivo `profiles.yaml` en tu directorio de configuración:
*   **Linux:** `~/.config/ghostknock/profiles.yaml`
*   **Windows:** `%APPDATA%\ghostknock\profiles.yaml`

```yaml
profiles:
  # Perfil para el servidor de casa
  casa:
    host: "192.168.1.50"
    port: 3001
    server_pubkey: "/home/usuario/.config/ghostknock/server_casa.pub"
    
  # Perfil para producción
  prod:
    host: "203.0.113.10"
    port: 3001
    server_pubkey: "C:\\Keys\\prod_server.pub"
    key: "C:\\Keys\\id_ed25519_prod" # Clave privada específica para este server
```

### 2. Estructura y Uso de Perfiles (`profiles.yaml`)

El archivo `profiles.yaml` es el corazón de la automatización en el cliente. Permite definir múltiples entornos (Prod, Staging, Home) con sus respectivas claves y puertos.

**Ubicación del archivo:**
*   🐧 **Linux:** `~/.config/ghostknock/profiles.yaml`
*   🪟 **Windows:** `%APPDATA%\ghostknock\profiles.yaml`

**Estructura de Campos:**

| Campo | Requerido | Descripción |
| :--- | :---: | :--- |
| `host` | ✅ | Dirección IP o dominio del servidor objetivo. |
| `server_pubkey` | ✅ | Ruta local al archivo `.pub` del servidor (para cifrar el mensaje). |
| `port` | ❌ | Puerto UDP. Si se omite, usa el **3001** por defecto. |
| `key` | ❌ | Ruta a tu clave privada. Si se omite, usa la default (`~/.config/ghostknock/id_ed25519`). |

**Ejemplo de uso en terminal:**

```bash
# 1. Acceso simple usando el perfil 'casa' (usa puerto 3001 y clave default)
ghostknock -profile casa -action open-ssh

# 2. Acceso a producción (usa puerto custom y clave específica definidos en YAML)
ghostknock -profile prod -action restart-nginx

# 3. Sobrescribir el host al vuelo (útil para testenar una IP nueva con la misma config)
ghostknock -profile prod -host 10.0.0.99 -action status
```

> **Nota de Prioridad:** Los flags manuales (`-host`, `-port`, etc.) siempre tienen preferencia sobre lo definido en el perfil.

---

## 🪝 Integración: Sistema de Hooks

GhostKnock permite definir scripts externos que se ejecutan automáticamente en respuesta a eventos del sistema. Esto es fundamental para auditoría, notificaciones a Slack/Telegram o validaciones complejas.

**Configuración Global (`config.yaml`):**

```yaml
hooks:
  # Se ejecuta ANTES de la acción. Si el script sale con error (exit code != 0), 
  # la acción se cancela y no se ejecuta.
  pre_execute: "/usr/local/bin/gk_audit.sh"
  
  # Se ejecuta tras el éxito del comando principal
  on_success: "/usr/local/bin/gk_notify.sh"
  
  # Se ejecuta si el comando falla
  on_error: "/usr/local/bin/gk_alert.sh"
```

**Variables de Entorno Inyectadas:**

Los scripts reciben automáticamente el contexto de ejecución:

| Variable | Descripción |
| :--- | :--- |
| `GK_USER` | Nombre del usuario autenticado (ej: `admin`). |
| `GK_IP` | Dirección IP de origen del knock. |
| `GK_ACTION` | ID de la acción ejecutada. |
| `GK_STAGE` | Etapa actual: `global_pre`, `action_post`, `global_success`, `global_error`, `global_revert`. |
| `GK_STATUS` | Resultado final: `success` o `error`. |
| `GK_PARAM_*` | Parámetros dinámicos en mayúsculas (ej: `-args "target=1.1.1.1"` -> `GK_PARAM_TARGET`). |

---

## 💡 Recetario: Ejemplos Prácticos para SysAdmins

A continuación, se presentan casos de uso reales para la gestión diaria de infraestructura, detallando la configuración del servidor y el comando exacto del cliente.

### 1. "Port Knocking 2.0": Acceso SSH Temporal
Abre el puerto 22 **solo para tu IP actual** y lo cierra automáticamente tras 5 minutos. Ideal para mantener el SSH cerrado al mundo.

*   **Configuración del Servidor (`config.yaml`):**
    ```yaml
    "open-ssh":
      # Inserta regla ACCEPT en la posición 1 para tu IP
      command: "iptables -I INPUT 1 -p tcp -s {{.SourceIP}} --dport 22 -j ACCEPT"
      # Elimina la regla automáticamente
      revert_command: "iptables -D INPUT -p tcp -s {{.SourceIP}} --dport 22 -j ACCEPT"
      revert_delay_seconds: 300
      cooldown_seconds: 60
    ```
*   **Comando Cliente:**
    ```bash
    ghostknock -profile prod -action open-ssh
    ```

### 2. Gestión de Servicios: Reinicio Genérico
Reinicia cualquier servicio de Systemd pasando el nombre como parámetro. Útil para reiniciar Nginx, MySQL o PHP-FPM sin entrar por SSH.

*   **Configuración del Servidor:**
    ```yaml
    "systemd-restart":
      # {{.Params.svc}} se sustituye por el argumento.
      # GhostKnock valida que solo contenga caracteres seguros (a-z, 0-9, -, _).
      command: "systemctl restart {{.Params.svc}}"
      timeout_seconds: 20
    ```
*   **Comando Cliente:**
    ```bash
    # Reiniciar Nginx
    ghostknock -profile prod -action systemd-restart -args "svc=nginx"
    
    # Reiniciar Docker
    ghostknock -profile prod -action systemd-restart -args "svc=docker"
    ```

### 3. Operaciones Docker: Reinicio de Contenedor
Reinicia un contenedor específico que se ha quedado colgado.

*   **Configuración del Servidor:**
    ```yaml
    "docker-bounce":
      command: "docker restart {{.Params.id}}"
      timeout_seconds: 45
    ```
*   **Comando Cliente:**
    ```bash
    ghostknock -profile prod -action docker-bounce -args "id=api-gateway-v2"
    ```

### 4. Ciberseguridad: Banear IP Atacante (Active Defense)
Si detectas un ataque desde una IP en tus logs, bloquéala instantáneamente en el firewall.

*   **Configuración del Servidor:**
    ```yaml
    "ban-ip":
      # Bloqueo permanente (DROP)
      command: "iptables -A INPUT -s {{.Params.target}} -j DROP"
    ```
*   **Comando Cliente:**
    ```bash
    ghostknock -profile prod -action ban-ip -args "target=192.168.1.55"
    ```

### 5. Copias de Seguridad: Trigger de Backup de Base de Datos
Lanza un script de backup bajo demanda antes de una operación arriesgada.

*   **Configuración del Servidor:**
    ```yaml
    "db-backup":
      run_as_user: "postgres" # Ejecutar como usuario de base de datos
      command: "/usr/local/bin/pg_backup.sh {{.Params.db_name}}"
      timeout_seconds: 300 # Dar tiempo suficiente (5 min)
    ```
*   **Comando Cliente:**
    ```bash
    ghostknock -profile prod -action db-backup -args "db_name=clientes_prod"
    ```

### 6. Gestión de Usuarios: Creación con Credenciales Ocultas
Crea un usuario de emergencia. El uso de `sensitive_params` asegura que la contraseña nunca quede registrada en texto plano en `/var/log/ghostknockd.log`.

*   **Configuración del Servidor:**
    ```yaml
    "create-admin":
      # Crea usuario, home y asigna password encriptada
      command: "useradd -m -G sudo -p $(openssl passwd -1 {{.Params.pass}}) {{.Params.user}}"
      # IMPORTANTE: Oculta 'pass' en los logs
      sensitive_params: ["pass"]
    ```
*   **Comando Cliente:**
    ```bash
    ghostknock -profile prod -action create-admin -args "user=support_ops,pass=R3scat3_X7z!"
    ```
    *Log del servidor:* `Ejecutando comando: [REDACTADO] ... params: {user: support_ops, pass: *****}`

## ⚙️ Referencia de Configuración Completa (`config.yaml`)

| Sección | Campo | Tipo | Obligatorio | Descripción |
| :--- | :--- | :--- | :---: | :--- |
| *(Raíz)* | `server_private_key_path` | string | ✅ | Ruta a la clave privada del servidor. |
| **`listener`** | `interface` | string | ✅ | Interfaz de red (ej: `eth0`, `any`). **Requiere restart.** |
| | `port` | int | ✅ | Puerto UDP. **Requiere restart.** |
| | `listen_ip` | string | ❌ | Escuchar solo en una IP específica. |
| **`logging`** | `log_level` | string | ❌ | Nivel: `debug`, `info`, `warn`, `error`. |
| | `log_file` | string | ❌ | **(v2.1)** Ruta, `stdout` o `/dev/null`. |
| | `log_format` | string | ❌ | **(v2.1)** `text` o `json`. |
| **`daemon`** | `pid_file` | string | ❌ | Ruta al archivo PID. |
| | `shell_path` | string | ❌ | **(v2.1)** Intérprete de comandos (default: `/bin/sh`). |
| | `shell_flag` | string | ❌ | **(v2.1)** Flag de ejecución (default: `-c`). |
| **`tuning`** | `packet_channel_buffer` | int | ❌ | **(v2.1)** Buffer de paquetes UDP. **Requiere restart.** |
| | `pcap_timeout_ms` | int | ❌ | **(v2.1)** Latencia de captura. **Requiere restart.** |
| | `max_tracked_ips` | int | ❌ | **(v2.1)** Límite de IPs en memoria (Anti-OOM). |
| | `eviction_batch_size` | int | ❌ | **(v2.1)** IPs a purgar por lote. |
| **`security`** | `deny_ips` | list | ❌ | **(v2.1)** Lista negra de IPs o rangos CIDR (ej: `["1.2.3.4", "10.0.0.0/8"]`). Drop instantáneo. |
| | `replay_window_seconds` | int | ❌ | Ventana de tiempo para aceptar un knock (Default: 5s). |
| | `rate_limit_per_second` | float | ❌ | Paquetes por segundo por IP (Default: 1.0). |
| **`hooks`** | `pre_execute` | string | ❌ | **(v2.1)** Script a ejecutar antes de la acción. Bloqueante. |
| | `on_success` | string | ❌ | Script tras éxito. |
| | `on_error` | string | ❌ | Script tras error. |
| | `on_revert` | string | ❌ | Script tras reversión. |
| **`users`** | `name` | string | ✅ | Identificador del usuario. |
| | `public_key` | string | ✅ | Clave pública Base64 del cliente. |
| | `actions` | list | ✅ | Lista de IDs de acciones permitidas. |
| | `source_ips` | list | ❌ | Whitelist de IPs/CIDRs para este usuario. |
| **`actions`** | `command` | string | ✅ | Comando a ejecutar. Soporta `{{.Params.x}}`. |
| | `run_as_user` | string | ❌ | Usuario del sistema que ejecuta el comando. |
| | `timeout_seconds` | int | ❌ | Tiempo máx antes de matar proceso (Default: 30s). |
| | `cooldown_seconds` | int | ❌ | Tiempo de espera entre ejecuciones. |
| | `revert_command` | string | ❌ | Comando de reversión automática. |
| | `sensitive_params` | list | ❌ | Parámetros a ocultar en los logs (`*****`). |
| | `pre_hook` | string | ❌ | Hook específico previo a la acción. |
| | `post_hook` | string | ❌ | Hook específico posterior a la acción. |

---

## 📄 Licencia

Este proyecto se distribuye bajo la **Licencia MIT**.
