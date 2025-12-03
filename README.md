# 👻 GhostKnock

[![Licencia: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/badge/release-v2.0.0-blue.svg)](https://github.com/soyunomas/GhostKnock/releases)
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
    *   **Protección Short-Circuit (Nueva):** Capacidad de definir una lista negra (`deny_ips`) que descarta tráfico de atacantes conocidos *antes* de realizar cualquier operación criptográfica, ahorrando recursos.

*   🔐 **Seguridad y Privacidad (Hardening):**
    *   **Cifrado de Extremo a Extremo:** Utiliza estándares modernos (`Ed25519` + `X25519`) para autenticación y confidencialidad. Solo el servidor puede leer qué comando estás enviando.
    *   **Anti-Replay Híbrido:** Sistema de doble verificación (lectura rápida + bloqueo de escritura) que detecta y rechaza paquetes duplicados para evitar la reutilización de credenciales.
    *   **Memoria Blindada (Anti-OOM):** Arquitectura diseñada para evitar el agotamiento de memoria ante ataques masivos, con purga automática de tablas de rastreo y límites estrictos.

*   👮 **Principio de Mínimo Privilegio:**
    *   Puedes configurar comandos para que se ejecuten como usuarios restringidos (ej. `www-data`), limitando el impacto en el sistema.

*   🔄 **Gestión en Caliente (Hot Reload):**
    *   Permite añadir usuarios, rotar claves o modificar acciones editando la configuración y recargando el servicio (`systemctl reload`) **sin detener el servicio** y manteniendo intacta la caché de seguridad.

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
    sudo dpkg -i ghostknock_2.0.0_amd64.deb
    # Se instala el servicio systemd, logrotate y se asegura /etc/ghostknock
    ```

*   **Para Clientes Remotos (Solo Herramientas):**
    ```bash
    sudo dpkg -i ghostknock-client_2.0.0_amd64.deb
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

## ⚡ Truco Pro: Simplificación con Alias

El comando completo puede resultar largo. Puedes configurar un alias en tu shell (`~/.bashrc`, `~/.zshrc` o PowerShell) para fijar los parámetros estáticos (Host y Claves).

**En Linux / Mac:**
```bash
# Añadir a tu .bashrc
alias gk='ghostknock -host 192.168.1.50 -server-pubkey ~/.config/ghostknock/server.pub'
```

**Uso simplificado:**
```bash
gk -action open-ssh
gk -action ban-ip -args "target=1.2.3.4"
```

---

## 💡 Recetario: Ejemplos Prácticos

### 1. Hola Mundo (Test)
*   **Config:** `command: 'echo "Hola {{.Params.msg}}" > /tmp/hi'`
*   **Cliente:** `ghostknock ... -action test -args "msg=Mundo"`

### 2. Banear IP (Firewall Dinámico)
*   **Config:** `command: "iptables -A INPUT -s {{.Params.ip}} -j DROP"`
*   **Cliente:** `ghostknock ... -action ban-ip -args "ip=1.2.3.4"`

### 3. Despliegue Web (Usuario restringido)
*   **Config:**
    ```yaml
    "deploy":
      run_as_user: "www-data"
      command: "cd /var/www/app && git pull"
      timeout_seconds: 120
    ```
*   **Cliente:** `ghostknock ... -action deploy`

### 4. Creación de Usuario (Privacidad en Logs)
Crea un usuario pasando contraseña. `sensitive_params` la ocultará en `/var/log/ghostknockd.log`.

*   **Config:**
    ```yaml
    "create-user":
      command: "useradd -p {{.Params.pass}} {{.Params.user}}"
      sensitive_params: ["pass"]
    ```
*   **Cliente:** `ghostknock ... -action create-user -args "user=bob,pass=S3creto"`
*   **Log:** `command="[REDACTADO]..." params=map[pass:***** user:bob]`

---

## ⚙️ Referencia de Configuración Completa (`config.yaml`)

| Sección | Campo | Tipo | Obligatorio | Descripción |
| :--- | :--- | :--- | :---: | :--- |
| *(Raíz)* | `server_private_key_path` | string | ✅ | Ruta a la clave privada del servidor. |
| **`listener`** | `interface` | string | ✅ | Interfaz de red (ej: `eth0`, `any`). **Requiere restart.** |
| | `port` | int | ✅ | Puerto UDP. **Requiere restart.** |
| | `listen_ip` | string | ❌ | Escuchar solo en una IP específica. |
| **`security`** | `deny_ips` | list | ❌ | Lista negra de IPs o rangos CIDR (ej: `["1.2.3.4", "10.0.0.0/8"]`). Drop instantáneo. |
| | `replay_window_seconds` | int | ❌ | Ventana de tiempo para aceptar un knock (Default: 5s). |
| | `rate_limit_per_second` | float | ❌ | Paquetes por segundo por IP (Default: 1.0). |
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

---

## 📄 Licencia

Este proyecto se distribuye bajo la **Licencia MIT**.
