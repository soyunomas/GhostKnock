## Tabla de Contenidos

### 1. Introducción y Arquitectura
*   [1.1. Concepto: Single Packet Authorization (SPA)](#11-concepto-single-packet-authorization-spa)
*   [1.2. Protocolo v2: Flujo Criptográfico y Seguridad](#12-protocolo-v2-flujo-criptográfico-y-seguridad)

### 2. Instalación y Despliegue
*   [2.1. Requisitos del Sistema](#21-requisitos-del-sistema)
*   [2.2. Instalación en Servidor (Linux)](#22-instalación-en-servidor-linux)
*   [2.3. Instalación de Clientes (Linux/Windows)](#23-instalación-de-clientes-linuxwindows)

### 3. Gestión de Infraestructura de Claves (PKI)
*   [3.1. Filosofía de Identidad](#31-filosofía-de-identidad)
*   [3.2. Generación y Distribución de Claves](#32-generación-y-distribución-de-claves)
*   [3.3. Higiene de Permisos](#33-higiene-de-permisos)

### 4. Configuración del Servidor (ghostknockd)
*   [4.1. Estructura de config.yaml](#41-estructura-de-configyaml)
*   [4.2. Red y Firewall (El secreto de la Invisibilidad)](#42-red-y-firewall-el-secreto-de-la-invisibilidad)
*   [4.3. Políticas de Seguridad (Anti-DoS/Anti-Replay)](#43-políticas-de-seguridad-anti-dosanti-replay)
*   [4.4. Control de Acceso (ACLs)](#44-control-de-acceso-acls)

### 5. Definición de Acciones y Ejecución
*   [5.1. Anatomía de una Acción](#51-anatomía-de-una-acción)
*   [5.2. Inyección de Parámetros Segura](#52-inyección-de-parámetros-segura)
*   [5.3. Privacidad en Logs (Sensitive Params)](#53-privacidad-en-logs-sensitive-params)
*   [5.4. Control de Flujo (Timeouts y Reversión)](#54-control-de-flujo-timeouts-y-reversión)

### 6. Uso del Cliente CLI (ghostknock)
*   [6.1. Sintaxis y Flags](#61-sintaxis-y-flags)
*   [6.2. Contexto Criptográfico Obligatorio](#62-contexto-criptográfico-obligatorio)
*   [6.3. Tip: Simplificación con Alias](#63-tip-pro-simplificación-con-alias)

### 7. Recetario de Operaciones (Use Cases)
*   [7.1. Gestión de Accesos (SSH/VPN)](#71-gestión-de-accesos-sshvpn)
*   [7.2. Operaciones DevOps (Docker/Git)](#72-operaciones-devops-dockergit)
*   [7.3. Mantenimiento del Sistema (Updates)](#73-mantenimiento-del-sistema-updates)

### 8. Troubleshooting y Observabilidad
*   [8.1. Interpretación de Logs](#81-interpretación-de-logs)
*   [8.2. Validación y Errores Comunes](#82-validación-y-errores-comunes)

### 9. Anexo Técnico
*   [9.1. Especificación del Payload](#91-especificación-del-payload)
*   [9.2. Vectores de Ataque Mitigados](#92-vectores-de-ataque-mitigados)

---

# 1. Introducción y Arquitectura

GhostKnock es una implementación moderna de **Single Packet Authorization (SPA)**, diseñada para ocultar servicios críticos tras un "muro invisible".

A diferencia de las VPNs o servicios públicos, GhostKnock no mantiene puertos TCP abiertos (`LISTEN`). Captura pasivamente tráfico UDP crudo y solo reacciona ante paquetes criptográficamente válidos, permaneciendo invisible ante escáneres como Nmap o Shodan.

## 1.1. Concepto: Single Packet Authorization (SPA)

GhostKnock supera las limitaciones del "Port Knocking" tradicional:

| Característica | Port Knocking Tradicional | GhostKnock (SPA) |
| :--- | :--- | :--- |
| **Mecanismo** | Secuencia de golpes (ej. 7000→8000→9000). | **Un único paquete UDP cifrado.** |
| **Seguridad** | Baja. Vulnerable a Replay y Sniffing. | **Militar.** Criptografía asimétrica (Ed25519/X25519). |
| **Confidencialidad** | Nula. Un observador ve la secuencia. | **Total.** Acción y parámetros indescifrables. |
| **Velocidad** | Lenta (múltiples RTT). | **Inmediata.** |
| **Detección** | Patrón de tráfico anómalo. | Indistinguible de ruido UDP aleatorio. |

---

## 1.2. Protocolo v2: Flujo Criptográfico y Seguridad

La versión 2.0.0 implementa una arquitectura de seguridad en profundidad:

1.  **Transporte Invisible (UDP):** El servidor usa `libpcap` para inspeccionar paquetes sin abrir sockets del sistema. **Nunca envía respuestas (ACKs/Errores)**, ni siquiera si el paquete falla.
2.  **Autenticación (Firma Ed25519):** Garantiza Integridad y Autenticidad. El servidor verifica la firma con la clave pública del usuario *antes* de intentar descifrar, protegiendo la CPU.
3.  **Confidencialidad (Cifrado X25519):** El payload (acción + argumentos) se cifra con `nacl/box` (Curve25519 + XSalsa20 + Poly1305). Solo el servidor con su clave privada puede leer qué comando se solicita.

**Estructura del Paquete en la Red:**
`[ Firma (64B) ] + [ Nonce (24B) ] + [ Payload Cifrado (N Bytes) ]`

---

# 2. Instalación y Despliegue

### 2.1. Requisitos del Sistema

**Servidor (Target):**
*   Linux (Kernel 4.x+).
*   `libpcap0.8` o superior (Requerido para captura en modo promiscuo).
*   Acceso `root` o `CAP_NET_RAW` para el demonio.
*   Firewall configurado en modo DROP (ver Sección 4.2).

**Cliente (Operador):**
*   Linux, Windows o macOS.
*   Binario estático (sin dependencias).

---

### 2.2. Instalación en Servidor (Linux)

Utilice los paquetes `.deb` para configurar automáticamente systemd y permisos.

```bash
# Instalación
sudo dpkg -i ghostknock_2.0.0_amd64.deb

# Si falta libpcap:
sudo apt-get update && sudo apt-get install -f

# Verificar estado (Inactivo por defecto hasta configurar)
systemctl status ghostknockd
```

---

### 2.3. Instalación de Clientes (Linux/Windows)

El cliente solo necesita los binarios CLI. **No instale el demonio en los clientes.**

*   **Linux:** `sudo dpkg -i ghostknock-client_2.0.0_amd64.deb`
*   **Windows:** Descargar `.exe` y añadir al `PATH`.

---

# 3. Gestión de Infraestructura de Claves (PKI)

GhostKnock v2.0 implementa un modelo **Zero-Trust** basado en criptografía asimétrica bidireccional. No hay contraseñas compartidas.

### 3.1. Filosofía de Identidad
*   **Para Autenticar (Firma):** El Cliente firma con su Privada; el Servidor verifica con la Pública del Cliente.
*   **Para Confidencialidad (Cifrado):** El Cliente cifra con la Pública del Servidor; el Servidor descifra con su Privada.

### 3.2. Generación y Distribución de Claves

**Paso 1: Identidad del Servidor (Una vez, en el servidor)**
```bash
sudo ghostknock-keygen -o /etc/ghostknock/server_key
# Comparte 'server_key.pub' con todos tus usuarios legítimos.
```
*   Configurar en `/etc/ghostknock/config.yaml`: `server_private_key_path: "/etc/ghostknock/server_key"`

**Paso 2: Identidad del Cliente (Cada usuario, en su PC)**
```bash
ghostknock-keygen
# Salida: ~/.config/ghostknock/id_ed25519
# Copia la cadena pública Base64 resultante.
```
*   El administrador debe añadir esta cadena en la sección `users` del `config.yaml` del servidor.

### 3.3. Higiene de Permisos
**CRÍTICO:** Las claves privadas deben ser legibles *solo* por su dueño.
```bash
chmod 700 /etc/ghostknock
chmod 600 /etc/ghostknock/server_key
```

---

# 4. Configuración del Servidor (ghostknockd)

Archivo maestro: `/etc/ghostknock/config.yaml`.

## 4.1. Estructura de config.yaml

```yaml
server_private_key_path: "/etc/ghostknock/server_key"

listener:
  interface: "eth0" # Interfaz WAN
  port: 3001        # Puerto UDP invisible

logging:
  log_level: "info"

security:
  replay_window_seconds: 5
  rate_limit_per_second: 1.0

users:
  - name: "admin"
    public_key: "..."
    actions: ["open-ssh", "update-sys"]

actions:
  "open-ssh":
    command: "..."
```

---

## 4.2. Red y Firewall (El secreto de la Invisibilidad)

Para que GhostKnock sea indetectable, el sistema operativo **NO DEBE** responder al tráfico en el puerto 3001.

Si no configuras el firewall, Linux enviará un **ICMP Port Unreachable**, revelando tu presencia.

**Configuración Obligatoria (IPTABLES):**
```bash
# DESCARTAR silenciosamente el tráfico.
# GhostKnock lo verá igual porque escucha antes del firewall (libpcap).
sudo iptables -A INPUT -p udp --dport 3001 -j DROP
```

---

## 4.3. Políticas de Seguridad (Anti-DoS/Anti-Replay)

*   **`replay_window_seconds`**: Ventana temporal simétrica de 1 a 3600 segundos. El servidor rechaza timestamps anteriores a `now-X` y posteriores a `now+X`; mantén ambos relojes sincronizados mediante NTP. Cambiarla requiere reiniciar el daemon.
*   **`rate_limit_per_second`**: Token Bucket por IP. Si una IP inunda el puerto, se bloquea *antes* de gastar CPU en criptografía.

---

## 4.4. Control de Acceso (ACLs)

La sección `users` es una lista blanca estricta.

```yaml
users:
  - name: "operador_vpn"
    public_key: "BASE64_KEY..."
    # Seguridad Extra: Solo permitir desde estas IPs
    source_ips: ["192.168.1.0/24", "10.0.0.5/32"]
    # Seguridad Extra: Solo permitir estas acciones
    actions: ["restart-vpn"]
```

---

# 5. Definición de Acciones y Ejecución

## 5.1. Anatomía de una Acción
```yaml
actions:
  "backup-db":
    command: "/usr/local/bin/backup.sh"
    run_as_user: "postgres" # Principio de menor privilegio
    timeout_seconds: 300    # Límite de ejecución (SIGKILL si excede)
```

## 5.2. Inyección de Parámetros Segura
Se usa templating de Go (`{{.Params.key}}`) con **Sanitización Estricta**.
*   **Nombres:** Deben cumplir `^[A-Za-z_][A-Za-z0-9_]{0,63}$`.
*   **Valores:** Deben cumplir `^[A-Za-z0-9._][A-Za-z0-9._-]*$` y no pueden ser `..`.
*   **Anti-Flag Injection:** Los valores NO pueden empezar con `-`.
*   **Antes de Hooks:** La validación ocurre antes de cualquier hook, log o comando.
*   **Variables de Entorno:** No se aceptan claves que colisionen al convertirse a mayúsculas.
*   **2FA:** `otp` es un nombre reservado y nunca se entrega como `GK_PARAM_OTP`.
*   **Templates:** Usa acceso directo `{{.Params.nombre}}`; los accesos dinámicos con `index` se rechazan.

```yaml
  "ping-check":
    # Uso: -args "target=8.8.8.8"
    command: "ping -c 4 {{.Params.target}}"
```

## 5.3. Privacidad en Logs (Sensitive Params)
Oculta secretos en `/var/log/ghostknockd.log`.

```yaml
  "create-user":
    command: "useradd -p {{.Params.pass}} {{.Params.user}}"
    sensitive_params: ["pass"] # Reemplaza 'pass' por '*****' en logs
```

Los nombres de `sensitive_params` se comparan sin distinguir mayúsculas. La
redacción también se aplica a stdout/stderr de comandos y hooks. Los scripts
de hook deben citar siempre `"$GK_PARAM_NOMBRE"` y no deben usar `eval`.

## 5.4. Control de Flujo (Timeouts y Reversión)

**Reversión Automática (Ideal para Firewalls):**
Ejecuta un comando "undo" tras un retardo.
⚠️ **Advertencia:** La cuenta atrás reside en memoria RAM. Si reinicias `ghostknockd`, la reversión se pierde.

```yaml
  "open-ssh":
    command: "iptables -I INPUT -s {{.SourceIP}} ... -j ACCEPT"
    revert_command: "iptables -D INPUT -s {{.SourceIP}} ... -j ACCEPT"
    revert_delay_seconds: 300 # 5 min
```

---

# 6. Uso del Cliente CLI (ghostknock)

## 6.1. Sintaxis y Flags

```bash
ghostknock -host <IP> -server-pubkey <PUB> -action <ID> [-args "k=v"]
```

## 6.2. Contexto Criptográfico Obligatorio
El cliente **siempre** requiere la clave pública del servidor (`-server-pubkey`) para cifrar el mensaje. Sin ella, el servidor no podría leerlo.

```bash
# Ejemplo completo
ghostknock -host 192.168.1.50 \
           -server-pubkey ~/keys/server.pub \
           -action status-check
```

---

## 6.3. Tip: Simplificación con Alias

El comando completo puede resultar largo de escribir repetidamente. Se recomienda configurar atajos para fijar los parámetros estáticos (IP y Claves).

### A. En Linux / macOS (Bash o Zsh)
Edita tu archivo de configuración (`~/.bashrc` o `~/.zshrc`) y añade:

```bash
# Define el alias fijando el Host y la Clave Pública
alias gk='ghostknock -host 192.168.1.50 -server-pubkey ~/.config/ghostknock/server.pub'
```
*Recarga la configuración con `source ~/.bashrc`.*

### B. En Windows (PowerShell)
Edita tu perfil de PowerShell (ejecuta `notepad $PROFILE` para abrirlo) y añade una función wrapper:

```powershell
function gk {
    # Ajusta las rutas a tu caso real
    $exePath = "C:\Tools\ghostknock.exe"
    $pubKey  = "$env:USERPROFILE\.config\ghostknock\server.pub"
    $target  = "192.168.1.50"

    # Pasa todos los argumentos extra (@args) al comando
    & $exePath -host $target -server-pubkey $pubKey @args
}
```
*Reinicia la terminal de PowerShell para aplicar.*

### Ejemplos de Uso (Con Alias)
Una vez configurado, el uso se simplifica drásticamente:

**Ejemplo 1: Acción simple**
```bash
# Antes: ghostknock -host ... -server-pubkey ... -action open-ssh
gk -action open-ssh
```

**Ejemplo 2: Acción con argumentos**
```bash
# Antes: ghostknock -host ... -server-pubkey ... -action ban-ip -args "target=1.2.3.4"
gk -action ban-ip -args "target=10.0.0.5"
```
---

# 7. Recetario de Operaciones (Use Cases)

### 7.1. Gestión de Accesos (SSH/VPN)

**Abrir SSH temporalmente (Port Knocking 2.0):**
```yaml
"open-ssh":
  command: "iptables -I INPUT 1 -p tcp -s {{.SourceIP}} --dport 22 -j ACCEPT"
  revert_command: "iptables -D INPUT -p tcp -s {{.SourceIP}} --dport 22 -j ACCEPT"
  revert_delay_seconds: 300
```

### 7.2. Operaciones DevOps (Docker/Git)

**Reiniciar contenedor Docker:**
```yaml
"docker-bounce":
  command: "docker restart {{.Params.container}}"
```

**Despliegue Git (Usuario restringido):**
```yaml
"git-pull":
  run_as_user: "www-data"
  command: "cd /var/www/app && git pull"
  timeout_seconds: 60
```

### 7.3. Mantenimiento del Sistema (Updates)

⚠️ **Peligro de Timeout:** Para actualizaciones (`apt/yum`), aumenta siempre el `timeout_seconds` para evitar matar el proceso `dpkg` a la mitad y corromper el sistema.

```yaml
"sys-update":
  command: "apt-get update && apt-get upgrade -y"
  timeout_seconds: 1800  # 30 minutos
  cooldown_seconds: 3600 # 1 hora
```

---

# 8. Troubleshooting y Observabilidad

GhostKnock es **silencioso por diseño**. El cliente nunca sabe si funcionó (código de salida 0 solo significa "enviado"). Debes mirar los logs del servidor.

### 8.1. Interpretación de Logs
`tail -f /var/log/ghostknockd.log`

*   `INFO`: Todo OK. Acción ejecutada.
*   `WARN`: Paquete descartado.
    *   `invalid_signature`: Claves no coinciden.
    *   `decryption_failed`: Cliente usó clave pública de servidor incorrecta.
    *   `outside_replay_window`: Reloj desincronizado (>5s).

### 8.2. Validación y Errores Comunes
*   **"No pasa nada":** Verifica el firewall (iptables). ¿Llegan los paquetes? (`tcpdump -i any udp port 3001`).
*   **Configuración Rota:** Usa `ghostknockd -t` para validar sintaxis YAML.

---

# 9. Anexo Técnico

### 9.1. Especificación del Payload
Tras descifrar, el servidor ve este JSON:
```json
{
  "timestamp": 1678912345000000000,
  "action_id": "open-ssh",
  "params": { "user": "admin" }
}
```

### 9.2. Vectores de Ataque Mitigados
*   **Replay Attacks:** Mitigados por Timestamp + Caché de Firmas.
*   **DoS (CPU):** Mitigado por Rate Limit + Validación de Firma antes de Descifrado.
*   **DoS (Memoria):** Payload limitado a 1KB.
*   **Inyección de Comandos:** Whitelist estricta de caracteres y prohibición de flags (`-`).
