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
    *   **Indetectable externamente:** Ante un escaneo de red, el puerto parecerá estar **cerrado** o filtrado. El servidor captura los paquetes de forma pasiva.

*   🔐 **Seguridad y Privacidad:**
    *   **Cifrado de Extremo a Extremo:** Utiliza estándares modernos (`Ed25519` + `X25519`) para autenticación y confidencialidad.
    *   **Protección Anti-Replay:** Rechaza paquetes duplicados o interceptados.
    *   **Memoria Blindada:** Arquitectura diseñada para evitar ataques de agotamiento de memoria (Anti-OOM) y bufferbloat.

*   ⚙️ **Gestión en Caliente (Hot Reload):**
    *   **Recarga sin paradas:** Permite añadir usuarios, cambiar claves o modificar acciones editando la configuración y recargando el servicio (`systemctl reload`), sin perder la caché de seguridad ni detener el tráfico.
    *   **Logging Rotativo:** El paquete `.deb` configura automáticamente la rotación de logs para evitar llenar el disco.

*   🧩 **Flexibilidad Operativa:**
    *   **Parámetros Dinámicos:** Permite inyectar argumentos variables dentro de los comandos de forma segura.

---

## ⚠️ Comportamiento de Seguridad y Limitaciones

GhostKnock prioriza la **supervivencia del servidor** sobre la disponibilidad.

| Limitación | Escenario del Usuario | Comportamiento | Razón de Seguridad |
| :--- | :--- | :--- | :--- |
| **Límite de Procesos** | Se intentan lanzar más de 10 comandos simultáneos. | **RECHAZADO.** El servidor ignora el comando. | **Anti-Fork Bomb.** Protege la tabla de procesos del OS. |
| **Rate Limit (IP)** | Múltiples comandos en < 1 seg desde la misma IP. | **SILENCIO TOTAL.** Se ignoran las ráfagas. | **Anti-DoS.** Protege CPU y Memoria. |
| **Replay Cache** | Se reenvía un paquete idéntico. | **IGNORADO.** | **Anti-Replay.** Evita reutilización de credenciales. |
| **Timeout Forzoso** | Un script se cuelga indefinidamente. | **KILL.** Proceso eliminado (default 30s). | **Recuperación de Recursos.** |
| **Recarga (Reload)** | Se recarga configuración con `SIGHUP`. | **PERSISTE.** La caché de seguridad se mantiene. | **Continuidad.** No se pierden protecciones. |

---

## 📦 Instalación

### Opción A: Paquetes .deb (Debian/Ubuntu/Mint)

Descarga la última versión desde [Releases](https://github.com/soyunomas/GhostKnock/releases).

*   **Para el Servidor (Demonio + Herramientas + Logrotate):**
    ```bash
    sudo dpkg -i ghostknock_2.1.0_amd64.deb
    ```
    *Se instala el servicio systemd, se asegura `/etc/ghostknock` y se configura la rotación automática de logs en `/var/log/ghostknockd.log`.*

*   **Para Clientes Remotos (Solo Herramientas):**
    ```bash
    sudo dpkg -i ghostknock-client_2.1.0_amd64.deb
    ```

### Opción B: Ejecutables para Windows

Descarga `ghostknock.exe` y `ghostknock-keygen.exe` desde Releases.

### Opción C: Compilación Manual

```bash
make build          # Compila para Linux
make install        # Instala binarios, systemd y logrotate
```

---

## 🛠️ Gestión y Operaciones

Una vez instalado el servidor, puedes gestionarlo con `systemctl`.

### Iniciar / Parar
```bash
sudo systemctl start ghostknockd
sudo systemctl stop ghostknockd
```

### 🔄 Recarga en Caliente (Hot Reload)
Si modificas `config.yaml` (ej. añades un usuario o cambias un comando), no necesitas reiniciar.
```bash
sudo systemctl reload ghostknockd
```
> **Nota:** Esto actualiza usuarios, claves y acciones manteniendo la seguridad. Si cambias la **interfaz de red** o el **puerto**, DEBES hacer un `restart` completo.

### Ver Logs
```bash
sudo tail -f /var/log/ghostknockd.log
```

---

## 🧱 Configuración del Firewall (Crucial)

Para que GhostKnock sea invisible, **el sistema operativo debe DESCARTAR** el tráfico en el puerto UDP configurado, para no enviar respuestas ICMP "Port Unreachable".

### UFW (Ubuntu/Debian)
```bash
sudo ufw deny 3001/udp
sudo ufw reload
```

### IPTables
```bash
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
    actions: ["open-ssh", "update"]

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

## 💡 Recetario de Comandos

| Acción | Configuración (Server) | Comando (Cliente) |
| :--- | :--- | :--- |
| **Hola Mundo** | `command: 'echo "Hola {{.Params.msg}}" > /tmp/hi'` | `-args "msg=Mundo"` |
| **Banear IP** | `command: "iptables -A INPUT -s {{.Params.ip}} -j DROP"` | `-args "ip=1.2.3.4"` |
| **Deploy Web** | `command: "cd /var/www && git pull"` | `-action deploy` |
| **Crear Usuario** | `command: "useradd ..."`<br>`sensitive_params: ["pass"]` | `-args "pass=Secreto123"` |

> **Nota sobre Timeouts:** Para tareas largas (updates, backups), aumenta `timeout_seconds` en `config.yaml` o el servidor matará el proceso a los 30s.

---

## ⚙️ Referencia de Configuración Completa (`config.yaml`)

Aquí se detallan todas las opciones disponibles para configurar el demonio.

| Sección | Campo | Tipo | Descripción |
| :--- | :--- | :--- | :--- |
| *(Raíz)* | `server_private_key_path` | string | **[Obligatorio]** Ruta a la clave privada del servidor. |
| **`listener`** | `interface` | string | **[Obligatorio]** Interfaz de red (ej: `eth0`, `any`). **Requiere restart si cambia.** |
| | `port` | int | **[Obligatorio]** Puerto UDP. **Requiere restart si cambia.** |
| | `listen_ip` | string | (Opcional) Escuchar solo en una IP específica. |
| **`logging`** | `log_level` | string | Nivel de log: `debug`, `info`, `warn`, `error`. |
| **`security`** | `deny_ips` | list | Lista negra de IPs o rangos CIDR (ej: `["1.2.3.4", "10.0.0.0/8"]`). Los paquetes de estas fuentes se descartan instantáneamente para ahorrar CPU. |
| | `replay_window_seconds` | int | Ventana de tiempo para aceptar un knock (Default: 5s). |
| | `rate_limit_per_second` | float | Paquetes por segundo permitidos por IP (Default: 1.0). |
| | `rate_limit_burst` | int | Ráfaga máxima permitida (Default: 3). |
| **`users`** | `name` | string | Identificador del usuario. |
| | `public_key` | string | Clave pública Base64 del cliente. |
| | `actions` | list | Lista de IDs de acciones permitidas. |
| | `source_ips` | list | (Opcional) Whitelist de IPs/CIDRs para este usuario. |
| **`actions`** | `command` | string | Comando a ejecutar. Soporta `{{.Params.x}}`. |
| | `timeout_seconds` | int | Tiempo máx antes de matar el proceso (Default: 30s). |
| | `cooldown_seconds` | int | Tiempo de espera entre ejecuciones. |
| | `revert_command` | string | Comando de reversión automática. |
| | `sensitive_params` | list | Parámetros a ocultar en los logs (`*****`). |

---

## 📄 Licencia

Este proyecto se distribuye bajo la **Licencia MIT**.
