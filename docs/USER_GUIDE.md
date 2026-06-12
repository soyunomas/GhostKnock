# Manual de Usuario y Referencia Técnica: GhostKnock v2.2.0

**GhostKnock** es un sistema avanzado de **Single Packet Authorization (SPA)** diseñado para ocultar servicios críticos. A diferencia del "Port Knocking" tradicional, GhostKnock utiliza un único paquete UDP cifrado y firmado criptográficamente para ejecutar comandos en un servidor, manteniendo el puerto de escucha invisible ante escaneos de red.

---

## 📚 Tabla de Contenidos

1.  [Conceptos y Arquitectura](#1-conceptos-y-arquitectura)
2.  [Compilación e Instalación](#2-compilación-e-instalación)
3.  [Gestión de Identidades (PKI)](#3-gestión-de-identidades-pki)
4.  [Configuración del Servidor](#4-configuración-del-servidor)
5.  [Configuración del Cliente y Perfiles](#5-configuración-del-cliente-y-perfiles)
6.  [Uso Avanzado: 2FA, Hooks y Tuning](#6-uso-avanzado-2fa-hooks-y-tuning)
7.  [Recetario de Acciones (Ejemplos)](#7-recetario-de-acciones-ejemplos)
8.  [Troubleshooting](#8-troubleshooting)

---

## 1. Conceptos y Arquitectura

GhostKnock implementa un modelo de seguridad basado en **Single Packet Authorization (SPA)**, diseñado para proteger infraestructuras críticas eliminando por completo su exposición a Internet, aplicando principios de **Zero Trust** a nivel de red.

### 1.1. Filosofía: La "Invisibilidad Activa"
En una arquitectura de red convencional, los servicios (SSH, VPN, Paneles de Administración) deben estar "escuchando" en puertos abiertos para aceptar conexiones. Esto genera una **Superficie de Ataque Expuesta**: cualquier actor malicioso puede escanear la IP, descubrir el servicio, determinar su versión y lanzar ataques de fuerza bruta o exploits.

**El Paradigma GhostKnock:**
GhostKnock invierte este modelo. El servidor mantiene todos sus puertos cerrados (Firewall en modo DROP). Para el mundo exterior, el servidor no existe o está desconectado.

El acceso se concede mediante un mecanismo de "Autenticación Primero, Conexión Después":
1.  El cliente envía un único paquete UDP "fantasma" que contiene credenciales criptográficas.
2.  El servidor valida silenciosamente este paquete.
3.  Si es válido, el servidor modifica dinámicamente sus reglas de firewall para permitir el acceso *únicamente* a la IP de origen del solicitante y por un tiempo limitado.
4.  Si es inválido, el paquete se descarta sin generar respuesta alguna, manteniendo la invisibilidad total.

---

### 1.2. Arquitectura del Servidor (Motor de Captura Nativo)

La versión 2.2 de GhostKnock utiliza una arquitectura de bajo nivel diseñada para el sigilo absoluto y el alto rendimiento, operando independientemente de la pila de red tradicional del sistema operativo.

#### A. Sockets Crudos (AF_PACKET)
A diferencia de las aplicaciones estándar que abren un socket y esperan conexiones (`bind`/`listen`), el demonio `ghostknockd` se conecta directamente a la capa de enlace de datos del Kernel de Linux utilizando sockets `AF_PACKET`.
*   **Indetectable Localmente:** Herramientas de auditoría como `netstat`, `ss` o `lsof` no mostrarán a GhostKnock escuchando en ningún puerto.
*   **Inspección Pre-Firewall:** GhostKnock captura y analiza las tramas Ethernet *antes* de que el firewall del sistema operativo (iptables/nftables) las procese. Esto permite configurar el firewall para **bloquear todo el tráfico entrante**, mientras GhostKnock sigue recibiendo las instrucciones de apertura legítimas.

#### B. Pipeline de Procesamiento "Fail-Fast"
Para garantizar la resistencia ante ataques de Denegación de Servicio (DoS), el servidor procesa cada paquete en un pipeline estricto de cinco etapas, diseñado para descartar tráfico ilegítimo con el mínimo coste computacional:

1.  **Filtrado temprano en el parser nativo:** El listener descarta tramas que no
    sean IPv4/UDP, no estén dirigidas al puerto configurado o no coincidan con
    `listen_ip` cuando esté definido.
2.  **Lista Negra (Short-Circuit):** Se verifica la IP de origen contra la lista `deny_ips`. Si coincide, se descarta instantáneamente en memoria.
3.  **Rate Limiting (Token Bucket):** Se aplica un límite de velocidad por IP. Si una IP inunda el servidor, sus paquetes se descartan antes de llegar a la capa criptográfica.
4.  **Verificación de Firma (Ed25519):** El servidor comprueba la firma digital del paquete. **Esta es la barrera crítica:** si la firma no es matemáticamente válida, el paquete se elimina sin intentar descifrarlo, protegiendo la CPU contra el agotamiento.
5.  **Descifrado y Ejecución:** Solo si supera las 4 barreras anteriores, se descifra el payload (X25519) y se ejecuta la acción solicitada.

---

### 1.3. Protocolo Criptográfico y Seguridad de Datos

GhostKnock v2 utiliza un protocolo binario personalizado, encapsulado en UDP, que garantiza tres pilares de seguridad: **Autenticidad, Integridad y Confidencialidad**.

**Estructura del Paquete en la Red:**
`[ Firma (64 Bytes) ] + [ Nonce (24 Bytes) ] + [ Payload Cifrado (Variable) ]`

#### Componentes del Payload Seguro:
El cliente construye un contenedor de datos (Payload) antes de cifrarlo, que incluye:

1.  **Timestamp de Alta Precisión:** Una marca de tiempo (nanosegundos) utilizada para prevenir ataques de repetición. El servidor rechaza cualquier paquete fuera de una ventana temporal estricta (por defecto, 5 segundos).
2.  **ActionID:** El identificador único de la acción que se desea ejecutar (ej. `open-ssh`).
3.  **Params:** Argumentos dinámicos para la acción (ej. `target_ip=1.2.3.4`), validados contra una lista blanca de caracteres seguros.
4.  **Traffic Padding (Ofuscación):** El cliente inyecta automáticamente una cantidad aleatoria de bytes "basura" (0-255 bytes).

#### Mecanismos de Defensa:
*   **Cifrado Asimétrico (Curve25519/XSalsa20/Poly1305):** Asegura que solo el servidor, poseedor de la clave privada, pueda leer el contenido del comando. Un atacante que capture el tráfico solo verá ruido aleatorio.
*   **Anti-Replay (Caché de Firmas):** El servidor mantiene una memoria temporal de las firmas procesadas. Si un atacante captura un paquete válido e intenta reenviarlo, el servidor detecta que la firma ya fue utilizada y lo bloquea instantáneamente.
*   **Anti-Fingerprinting (Side-Channel Defense):** Gracias al *Traffic Padding*, el tamaño del paquete cifrado varía en cada envío. Esto impide que un observador pasivo deduzca qué comando se está ejecutando basándose en la longitud de los datos transmitidos.

## 2. Compilación e Instalación

GhostKnock está diseñado para ser portátil y fácil de desplegar. Al estar escrito en Go, se compila en un único binario estático que no requiere librerías compartidas externas en el sistema destino, facilitando su instalación en cualquier distribución Linux moderna.

### 2.1. Requisitos Previos

Para compilar el proyecto desde el código fuente, necesitará un entorno de desarrollo con:

*   **Lenguaje Go:** Versión 1.21 o superior.
*   **Make:** Herramienta de automatización de compilación (estándar en Linux).
*   **Git:** Para clonar el repositorio.

**Sistemas Operativos Soportados:**
*   **Servidor:** Linux (Kernel 4.x o superior). Requiere capacidades de red (`AF_PACKET`) nativas del kernel.
*   **Cliente:** Linux, Windows (amd64/arm64), macOS, *BSD.

---

### 2.2. Compilación desde Código Fuente

El proceso de compilación genera binarios **100% estáticos** (`CGO_ENABLED=0`). Esto significa que el ejecutable resultante contiene todo lo necesario para funcionar y no depende de versiones específicas de `libc` o `libpcap` en el sistema host.

**1. Obtener el código fuente:**
```bash
git clone https://github.com/soyunomas/GhostKnock.git
cd GhostKnock
```

**2. Compilar para Linux (Servidor y Cliente):**
Este comando genera los binarios nativos para su máquina actual.
```bash
make build-linux
```
*   *Resultado:* Se generan los ejecutables en `cmd/ghostknockd/` (Servidor), `cmd/ghostknock/` (Cliente) y `cmd/ghostknock-keygen/` (Generador de claves).

**3. Compilación Cruzada para Windows (Opcional):**
Puede generar los ejecutables `.exe` para clientes Windows directamente desde su entorno Linux. No requiere herramientas extra.
```bash
make build-windows
```
*   *Resultado:* Se generan `ghostknock.exe` y `ghostknock-keygen.exe`.

**4. Instalación en el Sistema (Linux):**
Este comando copia los binarios a `/usr/local/bin`, establece los archivos de configuración base en `/etc/ghostknock` y registra el servicio en Systemd.
```bash
sudo make install
```

---

### 2.3. Instalación vía Paquetes .deb (Debian/Ubuntu/Mint)

Para despliegues en producción en sistemas basados en Debian, se recomienda utilizar los paquetes precompilados, ya que gestionan automáticamente los permisos de usuario, la creación del servicio systemd y la rotación de logs.

**A. Instalación del Servidor (Daemon + Herramientas):**
Instale este paquete **únicamente** en la máquina que debe ser protegida.
```bash
sudo dpkg -i ghostknock_2.2.0_amd64.deb
```
*   Inicia automáticamente el servicio (inactivo por defecto hasta que se configure).
*   Crea el usuario de sistema seguro si es necesario.
*   Instala la configuración de `logrotate`.

**B. Instalación del Cliente (Solo Herramientas):**
Instale este paquete en los ordenadores de los administradores o usuarios remotos.
```bash
sudo dpkg -i ghostknock-client_2.2.0_amd64.deb
```
*   Incluye `ghostknock` (CLI de conexión) y `ghostknock-keygen`.
*   **No** instala el demonio ni el servicio systemd.

---

### 2.4. Generación y Gestión de Claves

GhostKnock utiliza criptografía asimétrica Ed25519. La utilidad incluida `ghostknock-keygen` facilita este proceso generando las claves y mostrando la salida pública en el formato correcto.

**Uso de la herramienta:**
```bash
ghostknock-keygen
```
*   Genera `id_ed25519` (Privada) y `id_ed25519.pub` (Pública).
*   Muestra la clave pública en terminal lista para copiar.

**Conversión Manual con `base64`:**
Los archivos de configuración (`config.yaml` y `profiles.yaml`) esperan las claves **estrictamente en formato Base64**. Si usted ya dispone de un archivo de clave binaria (raw) o necesita extraer la cadena de texto nuevamente desde un archivo existente sin usar la herramienta `keygen`, puede utilizar el comando estándar de Linux:

```bash
# Para obtener la cadena Base64 de una clave pública existente:
base64 -w 0 clave_publica.pub

# Para obtener la cadena Base64 de una clave privada (ej. para profiles.yaml):
base64 -w 0 clave_privada
```
> **Nota:** El flag `-w 0` es crucial para evitar saltos de línea en la salida, asegurando que la cadena sea válida para el archivo YAML.

## 3. Gestión de Identidades (PKI)

GhostKnock usa criptografía asimétrica. No hay contraseñas compartidas.

### 1. Generar Identidad del Servidor
Se realiza una sola vez en la máquina que ejecutará `ghostknockd`.

```bash
sudo ghostknock-keygen -o /etc/ghostknock/server_key
sudo chmod 600 /etc/ghostknock/server_key
```
> **Nota:** Debe distribuir el archivo **PÚBLICO** (`server_key.pub`) a todos los clientes autorizados.

### 2. Generar Identidad del Usuario (Cliente)
Cada persona que necesite acceso debe generar su propio par de claves en su PC.

```bash
ghostknock-keygen
# Salida por defecto: ~/.config/ghostknock/id_ed25519
```
> **Nota:** El usuario debe enviar el contenido de su clave **PÚBLICA** (cadena Base64) al administrador para que sea añadida al `config.yaml` del servidor.

---

## 4. Configuración del Servidor

El archivo maestro es `/etc/ghostknock/config.yaml`.

### Configuración del Firewall (CRÍTICO)
Para que GhostKnock sea invisible, **el sistema operativo debe ignorar el puerto UDP**. Si no configura el firewall, el kernel responderá con un "ICMP Port Unreachable", revelando su presencia.

**Ejemplo con IPTABLES:**
```bash
# Descartar todo el tráfico al puerto 3001.
# GhostKnock verá el paquete antes de que iptables lo descarte gracias a AF_PACKET.
sudo iptables -I INPUT -p udp --dport 3001 -j DROP
```

### Referencia de `config.yaml`

```yaml
server_private_key_path: "/etc/ghostknock/server_key"

listener:
  interface: "eth0"  # Interfaz WAN. Use "any" con precaución.
  port: 3001         # Puerto invisible.

tuning:
  packet_channel_buffer: 100   # Buffer para ráfagas de tráfico
  max_tracked_ips: 20000       # Límite de memoria para anti-DoS
  pcap_timeout_ms: 300         # Timeout de lectura (nombre legacy)

security:
  deny_ips: ["1.2.3.4", "10.0.0.0/8"] # Blacklist (Drop instantáneo)
  rate_limit_per_second: 1.0          # Paquetes por segundo por IP
  replay_window_seconds: 5            # Ventana temporal pasada/futura

users:
  - name: "admin"
    public_key: "BASE64_KEY_CLIENTE..."
    totp_secret: "JBSWY3DPEHPK3PXP"   # Opcional: Secreto 2FA (Base32)
    actions: ["open-ssh", "reboot"]

actions:
  "open-ssh":
    command: "iptables -I INPUT 1 -p tcp -s {{.SourceIP}} --dport 22 -j ACCEPT"
    revert_command: "iptables -D INPUT -p tcp -s {{.SourceIP}} --dport 22 -j ACCEPT"
    revert_delay_seconds: 60
```

`replay_window_seconds` debe estar entre 1 y 3600 segundos. El servidor
rechaza timestamps anteriores a `now-X` y posteriores a `now+X`; mantenga
sincronizados mediante NTP los relojes del cliente y del servidor. Cambiar esta
ventana requiere reiniciar el daemon.

---

## 5. Configuración del Cliente y Perfiles

El cliente `ghostknock` es una herramienta de línea de comandos (CLI) ligera y estática. Su única función es construir el payload, firmarlo, cifrarlo y enviarlo al servidor. No mantiene estado ni requiere servicios en segundo plano.

### 5.1. Ubicación y Estructura de Archivos

Aunque el cliente puede funcionar pasando todos los argumentos manualmente, se recomienda encarecidamente el uso de **Perfiles** para simplificar la operativa diaria.

#### Archivo de Configuración Global (`profiles.yaml`)
El cliente busca automáticamente este archivo en las rutas estándar del sistema operativo:

*   **Linux / macOS:** `~/.config/ghostknock/profiles.yaml`
*   **Windows:** `%APPDATA%\ghostknock\profiles.yaml` (Generalmente `C:\Users\SuUsuario\AppData\Roaming\ghostknock\profiles.yaml`)

#### Archivo de Clave Privada (Identidad)
Por defecto, el cliente buscará su clave privada en:
*   **Linux / macOS:** `~/.config/ghostknock/id_ed25519`
*   **Windows:** `%APPDATA%\ghostknock\id_ed25519`

> **Nota:** Puede generar estas claves ejecutando `ghostknock-keygen`.

---

### 5.2. Definición de Perfiles

El archivo `profiles.yaml` permite definir múltiples entornos (Producción, Staging, Casa, VPN) con sus respectivas credenciales y destinos.

**Estructura del YAML:**

```yaml
profiles:
  # --- PERFIL 1: SERVIDOR DE PRODUCCIÓN ---
  prod:
    # [Requerido] IP o Dominio del servidor
    host: "203.0.113.10"

    # [Opcional] Puerto UDP (Default: 3001)
    port: 3001

    # [Requerido] Ruta a la clave PÚBLICA del servidor (.pub)
    # Necesaria para cifrar el mensaje de forma que solo el servidor lo lea.
    server_pubkey: "/home/admin/.ssh/keys/servidor_prod.pub"

    # [Opcional] Ruta a TU clave privada
    # Si se omite, usa la default (~/.config/ghostknock/id_ed25519)
    key: "/home/admin/.ssh/keys/id_ed25519_prod"

  # --- PERFIL 2: OFICINA (WINDOWS) ---
  office:
    host: "vpn.corp.local"
    port: 443
    # En Windows, use doble barra invertida o barra simple
    server_pubkey: "C:\\Seguridad\\Keys\\office_server.pub"

  # --- PERFIL 3: RASPBERRY PI ---
  pi:
    host: "192.168.1.200"
    server_pubkey: "/home/user/.config/ghostknock/pi.pub"
```

---

### 5.3. Modos de Ejecución y Prioridad

El cliente resuelve la configuración siguiendo un orden estricto de precedencia:
**Flags Manuales (`-host`) > Perfil (`-profile`) > Valores por Defecto**

#### A. Modo Manual (Sin Perfil)
Útil para pruebas rápidas o scripts de un solo uso. Requiere especificar todo.

```bash
ghostknock -host 192.168.1.50 \
           -port 3001 \
           -server-pubkey ./server_key.pub \
           -key ./my_private_key \
           -action open-ssh
```

#### B. Modo Perfil (Recomendado)
Usa la configuración predefinida en el YAML.

```bash
ghostknock -profile prod -action open-ssh
```

#### C. Modo Híbrido (Sobrescritura)
Puede cargar un perfil base y modificar solo un parámetro al vuelo. Esto es muy útil para probar el mismo entorno en diferentes IPs (ej. tras una migración) sin editar el archivo YAML.

```bash
# Carga claves y puerto del perfil 'prod', pero fuerza una IP distinta
ghostknock -profile prod -host 10.0.0.99 -action status
```

---

### 5.4. Argumentos Dinámicos (`-args`)

Muchas acciones en el servidor requieren parámetros (ej. qué servicio reiniciar o qué IP banear). Estos se pasan con el flag `-args` en formato `clave=valor`.

**Sintaxis:**
*   Pares separados por comas: `key1=val1,key2=val2`
*   Sin espacios alrededor del `=` ni de las comas.

**Ejemplos:**

1.  **Reinicio de Servicio:**
    ```bash
    ghostknock -profile prod -action restart-svc -args "name=postgresql"
    ```

2.  **Banear IP:**
    ```bash
    ghostknock -profile prod -action ban-ip -args "target=45.33.22.11"
    ```

3.  **Uso con 2FA (OTP):**
    ```bash
    ghostknock -profile prod -action open-vpn -args "otp=849201"
    ```

4.  **Combinado:**
    ```bash
    ghostknock -profile prod -action create-user -args "user=invitado,pass=Temp1234,otp=999111"
    ```

> **Restricciones de Seguridad:** El cliente permite enviar cualquier cadena,
> pero el servidor rechaza los parámetros que no cumplan estas reglas:
> los nombres deben coincidir con `^[A-Za-z_][A-Za-z0-9_]{0,63}$`; los valores
> deben coincidir con `^[A-Za-z0-9._][A-Za-z0-9._-]*$`, no pueden ser `..` ni
> empezar por `-`. La validación ocurre antes de cualquier hook, log o comando.
> No se aceptan claves que colisionen al convertirse a mayúsculas. `otp` está
> reservado para 2FA y los templates deben usar acceso directo
> `{{.Params.nombre}}`; los accesos dinámicos con `index` se rechazan.

---

### 5.5. Automatización e Integración (Shell Aliases)

Para administradores de sistemas que usan GhostKnock intensivamente, se recomienda crear alias en el shell para reducir la fricción.

#### En Bash / Zsh (Linux/macOS)
Añada a su `.bashrc` o `.zshrc`:

```bash
# Alias simple
alias gk='ghostknock -profile prod'

# Función para pasar acción y argumentos rápidamente
function kn() {
    # Uso: kn open-ssh [args]
    ACTION=$1
    shift
    ARGS=$1
    if [ -z "$ARGS" ]; then
        ghostknock -profile prod -action "$ACTION"
    else
        ghostknock -profile prod -action "$ACTION" -args "$ARGS"
    fi
}
```

**Uso:** `kn restart-svc name=nginx`

#### En PowerShell (Windows)
Añada a su `$PROFILE`:

```powershell
function gk {
    param(
        [string]$Action,
        [string]$Args
    )
    $cmd = @("ghostknock.exe", "-profile", "prod", "-action", $Action)
    if ($Args) { $cmd += "-args"; $cmd += $Args }
    & $cmd
}
```

**Uso:** `gk open-ssh` o `gk restart-svc "name=nginx"`

---

## 6. Uso Avanzado: 2FA, Hooks, Tuning y Rendimiento

Esta sección detalla las capacidades avanzadas de GhostKnock v2.2 para entornos corporativos que requieren niveles superiores de seguridad, integración con sistemas de monitorización y ajuste fino de rendimiento.

### 6.1. Autenticación de Dos Factores (TOTP)

GhostKnock implementa el estándar RFC 6238 (Time-Based One-Time Password), compatible con aplicaciones como Google Authenticator, Authy, Microsoft Authenticator o FreeOTP. Esto añade una capa de seguridad: incluso si su clave privada `id_ed25519` es robada, el atacante no podrá ejecutar comandos sin el código temporal de su dispositivo móvil.

#### A. Configuración en el Servidor
Debe generar un secreto en formato Base32 para cada usuario que requiera 2FA. Puede usar herramientas online o el comando `qrencode` en Linux.

1.  **Generar Secreto (Ejemplo manual):**
    Un secreto Base32 típico tiene 16 o 32 caracteres (A-Z, 2-7).
    Ejemplo: `JBSWY3DPEHPK3PXP`

2.  **Configurar `config.yaml`:**
    ```yaml
    users:
      - name: "admin_vip"
        public_key: "..."
        # Habilita el requisito de OTP para este usuario
        totp_secret: "JBSWY3DPEHPK3PXP"
    ```

3.  **Añadir a la App Móvil:**
    Introduzca el secreto manualmente en su aplicación de autenticación.

#### B. Uso desde el Cliente
Cuando `totp_secret` está definido, el servidor **rechazará silenciosamente** cualquier knock que no incluya el parámetro `otp`.

```bash
# El código 123456 es el que muestra su app en ese instante
ghostknock -profile prod -action open-ssh -args "otp=123456"

# Combinando OTP con otros argumentos
ghostknock -profile prod -action restart-svc -args "svc=nginx,otp=123456"
```

> **Nota Técnica:** El servidor permite una ventana de desincronización de ±30 segundos (1 intervalo anterior y 1 posterior) para compensar retrasos de red o relojes ligeramente desajustados.

---

### 6.2. Sistema de Hooks (Automatización y Auditoría)

Los "Hooks" permiten disparar scripts externos en respuesta a eventos del ciclo de vida de GhostKnock. Son fundamentales para la integración con SIEMs, sistemas de notificaciones (Slack/Telegram/Email) o validaciones de seguridad complejas.

#### A. Ciclo de Vida y Tipos de Hooks

1.  **`pre_execute` (Global y Por Acción):**
    *   **Comportamiento:** Síncrono y Bloqueante.
    *   **Uso:** Auditoría previa o validación condicional extra.
    *   **Efecto:** Si el script devuelve un código de salida distinto de 0, **la acción se cancela** y no se ejecuta.

2.  **`on_success` / `on_error`:**
    *   **Comportamiento:** Asíncrono (Fire-and-Forget).
    *   **Uso:** Notificaciones y logging. No bloquean el hilo principal.

3.  **`on_revert`:**
    *   **Comportamiento:** Asíncrono.
    *   **Uso:** Notificar cuando una regla de firewall temporal se ha cerrado automáticamente.

#### B. Variables de Entorno (Contexto)
GhostKnock inyecta el contexto de la petición en el entorno del script. No se pasan como argumentos CLI para evitar problemas de parsing.

| Variable | Descripción | Ejemplo |
| :--- | :--- | :--- |
| `GK_USER` | Nombre del usuario autenticado (según config). | `admin` |
| `GK_IP` | Dirección IP de origen del paquete. | `203.0.113.55` |
| `GK_ACTION` | ID de la acción solicitada. | `open-ssh` |
| `GK_STAGE` | Etapa actual del ciclo de vida. | `global_pre`, `action_post`, `global_error` |
| `GK_STATUS` | Resultado de la operación (`success` o `error`). | `success` |
| `GK_ERROR_MSG` | Si hubo error, el mensaje descriptivo. | `exit status 127` |
| `GK_PARAM_*` | Argumentos dinámicos enviados por el cliente (Mayúsculas). | `GK_PARAM_TARGET`, `GK_PARAM_OTP` |

Los hooks solo reciben parámetros después de superar la validación estricta.
Los scripts deben citar siempre variables como `"$GK_PARAM_TARGET"` y no deben
usar `eval`. Los nombres incluidos en `sensitive_params` se comparan sin
distinguir mayúsculas; sus valores se redactan también en stdout/stderr de
comandos y hooks.

#### C. Ejemplo de Hook: Notificación a Slack
Guarde este script en `/usr/local/bin/gk_slack.sh` y dele permisos de ejecución (`chmod +x`).

```bash
#!/bin/bash
# Webhook URL de Slack (Configurar la suya propia)
WEBHOOK_URL="https://hooks.slack.com/services/XXX/YYY/ZZZ"

TEXT="👻 *GhostKnock Event*
*User:* $GK_USER
*IP:* $GK_IP
*Action:* $GK_ACTION
*Status:* $GK_STATUS"

if [ "$GK_STATUS" == "error" ]; then
    TEXT="$TEXT\n*Error:* $GK_ERROR_MSG"
fi

curl -X POST -H 'Content-type: application/json' \
    --data "{\"text\":\"$TEXT\"}" $WEBHOOK_URL
```

Configuración en `config.yaml`:
```yaml
hooks:
  on_success: "/usr/local/bin/gk_slack.sh"
  on_error: "/usr/local/bin/gk_slack.sh"
```

---

## 6.3. Tuning y Rendimiento (Escalabilidad)

GhostKnock v2.2 permite ajustar el motor interno para adaptarse al hardware y al perfil de tráfico, escalando desde dispositivos IoT limitados hasta servidores de alto tráfico expuestos a Internet.

#### A. Gestión de Memoria (Anti-OOM)
Para prevenir ataques de denegación de servicio que intenten agotar la memoria RAM creando millones de sesiones falsas, GhostKnock impone límites estrictos.

*   **`max_tracked_ips` (Default: 20000):**
    Define cuántas direcciones IP únicas se rastrean simultáneamente para el Rate Limiting. Cada entrada consume ~100 bytes. 20k entradas consumen ~2MB de RAM.
*   **`eviction_batch_size` (Default: 2000):**
    Si la tabla se llena, el servidor elimina aleatoriamente esta cantidad de IPs antiguas para hacer espacio a las nuevas. Esto garantiza que el servidor **nunca** deje de procesar tráfico nuevo y **nunca** colapse por falta de memoria (OOM Kill), degradando suavemente la precisión del rate-limit bajo ataque masivo.

#### B. Latencia de Red y Buffers
*   **`packet_channel_buffer` (Default: 100):**
    Es la cola interna entre el listener (captura) y los workers (criptografía). Un valor alto absorbe mejor las ráfagas repentinas de tráfico (bursts), evitando la pérdida de paquetes legítimos durante picos de carga.
*   **`pcap_timeout_ms` (Default: 300ms):**
    Mantiene su nombre por compatibilidad y controla el deadline de lectura del
    listener nativo.
    *   *Bajo (10ms):* Reacción casi instantánea, mayor uso de CPU. Ideal para tiempo real.
    *   *Alto (1000ms):* El sistema "duerme" más, ahorrando CPU. Mayor latencia de reacción.

#### C. Lista Negra de Cortocircuito (Short-Circuit)
La opción `deny_ips` en `security` es la defensa más eficiente en términos de rendimiento. Las IPs listadas aquí se descartan **antes** de cualquier operación criptográfica costosa.

```yaml
security:
  deny_ips:
    - "192.168.1.50"      # IP Única
    - "10.0.0.0/8"        # Subred completa (Millones de IPs descartadas en nanosegundos)
```

---

### Guía de Configuración por Hardware

Copie y pegue estos valores en su sección `tuning` del archivo `config.yaml` según su dispositivo:

#### 1. Dispositivos IoT / Low-End (Raspberry Pi Zero/3, Routers)
**Objetivo:** Minimizar el uso de RAM y ciclos de CPU. Priorizar estabilidad sobre latencia.

```yaml
tuning:
  packet_channel_buffer: 50      # Cola corta para ahorrar RAM
  max_tracked_ips: 2000          # Suficiente para redes domésticas (~200KB RAM)
  pcap_timeout_ms: 1000          # Despertar CPU solo 1 vez por segundo
  eviction_batch_size: 500       # Limpieza agresiva si se llena la memoria
```

#### 2. Servidor VPS Estándar (1-2 vCPU, 2GB+ RAM)
**Objetivo:** Equilibrio general. Configuración recomendada para la mayoría de usuarios.

```yaml
tuning:
  packet_channel_buffer: 100     # Valor por defecto equilibrado
  max_tracked_ips: 20000         # Protege contra ataques moderados (~2MB RAM)
  pcap_timeout_ms: 300           # Latencia imperceptible para humanos
  eviction_batch_size: 2000
```

#### 3. Servidor Enterprise / Alto Tráfico (Metal, 10Gbps Uplink)
**Objetivo:** Máximo rendimiento y resistencia a ataques DDoS volumétricos. El consumo de RAM es secundario.

```yaml
tuning:
  packet_channel_buffer: 2000    # Absorbe ráfagas masivas sin perder paquetes
  max_tracked_ips: 100000        # Rastrea 100k atacantes simultáneos (~15MB RAM)
  pcap_timeout_ms: 10            # Latencia ultra-baja (Real-time)
  eviction_batch_size: 5000      # Rotación rápida de tablas
```

### 6.4. Gestión en Caliente (Hot Reload)

GhostKnock soporta la recarga de configuración sin interrumpir el servicio. Esto es vital para añadir usuarios o cambiar claves sin cerrar la ventana de seguridad.

**Comando:**
```bash
sudo systemctl reload ghostknockd
# O alternativamente:
sudo kill -HUP <PID>
```

**Tabla de Efectos de la Recarga:**

| Configuración Modificada | ¿Aplica en Caliente? | Notas |
| :--- | :---: | :--- |
| **Usuarios / Claves** | ✅ SÍ | Nuevos usuarios funcionan inmediatamente. |
| **Acciones / Comandos** | ✅ SÍ | Comandos actualizados se usarán en el siguiente knock. |
| **Hooks** | ✅ SÍ | Se actualizan las rutas de los scripts. |
| **Logging (Nivel/Archivo)** | ✅ SÍ | Cambia de `info` a `debug` sin reiniciar. |
| **Blacklist (`deny_ips`)** | ✅ SÍ | Bloqueo inmediato de nuevas IPs. |
| **Network (`interface`, `port`)** | ❌ NO | Requiere `restart` para volver a atar el socket. |
| **Tuning (`buffers`, `timeout`)** | ❌ NO | Requiere `restart` para reasignar memoria. |

---

### 6.5. Ofuscación de Tráfico (Traffic Padding)

Esta es una característica automática y transparente del protocolo v2.

**El Problema:**
En protocolos cifrados, un atacante que monitorice la red podría deducir qué comando se está enviando basándose simplemente en el **tamaño** del paquete cifrado (ej: un comando "reboot" es más corto que "update-system-full").

**La Solución de GhostKnock:**
El cliente (`ghostknock`) añade automáticamente una cantidad aleatoria de bytes basura (0 a 255 bytes) al final del payload JSON antes de cifrarlo.
*   El servidor descifra el paquete, ignora el campo de `padding` y ejecuta el comando.
*   Para un observador externo (ISP, Hacker en Wi-Fi), todos los paquetes tienen tamaños distintos y aleatorios, haciendo imposible el análisis de patrones por longitud (Side-Channel Attack).

No requiere configuración por parte del usuario.

---

## 7. Recetario de Acciones (Ejemplos)

### 1. "Port Knocking 2.0": Abrir SSH temporalmente
Abre el puerto 22 solo para su IP y lo cierra automáticamente tras 30 segundos.

*   **Server Config:**
    ```yaml
    "open-ssh":
      command: "iptables -I INPUT 1 -p tcp -s {{.SourceIP}} --dport 22 -j ACCEPT"
      revert_command: "iptables -D INPUT -p tcp -s {{.SourceIP}} --dport 22 -j ACCEPT"
      revert_delay_seconds: 30
    ```
*   **Cliente:** `ghostknock -profile prod -action open-ssh`

### 2. Gestión de Systemd con Parámetros
Reinicia cualquier servicio pasando su nombre.

*   **Server Config:**
    ```yaml
    "restart-svc":
      command: "systemctl restart {{.Params.svc}}"
      timeout_seconds: 10
    ```
*   **Cliente:** `ghostknock -profile prod -action restart-svc -args "svc=nginx"`

### 3. Banear atacante (Defensa Activa)
Bloquea una IP maliciosa permanentemente.

*   **Server Config:**
    ```yaml
    "ban-ip":
      command: "iptables -A INPUT -s {{.Params.target}} -j DROP"
    ```
*   **Cliente:** `ghostknock -profile prod -action ban-ip -args "target=192.168.1.55"`

### 4. Crear Usuario (Ocultando contraseña en logs)
*   **Server Config:**
    ```yaml
    "create-user":
      command: "useradd -p $(openssl passwd -1 {{.Params.pass}}) {{.Params.user}}"
      sensitive_params: ["pass"] # Reemplaza 'pass' por '*****' en logs
    ```
*   **Cliente:** `ghostknock -profile prod -action create-user -args "user=bob,pass=Secreto123"`

---

## 8. Troubleshooting y Diagnóstico Avanzado

GhostKnock es, por diseño, un sistema **silencioso y unidireccional**. El cliente envía el paquete y termina inmediatamente con éxito (código 0) independientemente de lo que ocurra en el servidor. Esto hace que el diagnóstico dependa casi exclusivamente de la observabilidad en el lado del servidor.

### 8.1. Metodología de Diagnóstico (Flujo de Verificación)

Si una acción no se ejecuta, siga este orden estricto para aislar el problema:

1.  **Capa de Red:** ¿Llega el paquete UDP a la interfaz física del servidor?
2.  **Capa de Captura:** ¿GhostKnockd está viendo el paquete?
3.  **Capa Criptográfica:** ¿Es válida la firma y el descifrado?
4.  **Capa de Lógica:** ¿Está el usuario autorizado, el 2FA correcto y el rate-limit respetado?
5.  **Capa de Ejecución:** ¿Falló el comando del sistema operativo (PATH, Permisos)?

---

### 8.2. Diagnóstico de Red y Captura (Nivel 1 y 2)

**Síntoma:** El cliente envía el knock, pero no aparece NADA en los logs del servidor (ni siquiera en modo `debug`).

1.  **Verificar llegada de paquetes (tcpdump):**
    Ejecute esto en el servidor mientras lanza un knock desde el cliente.
    ```bash
    # Verifique tráfico UDP en el puerto 3001 en cualquier interfaz
    sudo tcpdump -i any udp port 3001 -nn -vv
    ```
    *   **Si NO ve paquetes:** El tráfico está siendo bloqueado antes de llegar al servidor (Firewall perimetral, AWS Security Groups, NAT mal configurado en el router).
    *   **Si ve paquetes:** El tráfico llega a la tarjeta de red. Pase al paso 2.

2.  **Verificar Interfaz de Escucha:**
    Revise `config.yaml`. Si `listener.interface` está configurado como `eth0` pero el tráfico llega por `eth1`, GhostKnock no lo verá.
    *   **Solución:** Cambie a `interface: "any"` temporalmente para probar.

3.  **Conflicto de Firewall Local (UFW/Iptables):**
    Aunque GhostKnock usa captura cruda con `AF_PACKET`, ciertas configuraciones agresivas de `nftables` o `RP_FILTER` pueden afectar la recepción del tráfico.
    *   Asegúrese de que la regla sea `DROP` y no `REJECT`.
    *   Asegúrese de no tener reglas de *pre-routing* que desvíen el tráfico.

---

### 8.3. Matriz de Errores en Logs (Nivel 3 y 4)

Configure `log_level: "debug"` en `config.yaml` y recargue el servicio. Busque estos mensajes clave en `/var/log/ghostknockd.log`:

| Mensaje de Log (Aprox.) | Causa Probable | Solución |
| :--- | :--- | :--- |
| **(Nada / Silencio)** | Paquete malformado o clave pública del servidor incorrecta en el cliente. | El cliente no está usando la `server_pubkey` correcta. El servidor no puede descifrar ni el encabezado, por lo que lo trata como basura y lo ignora. |
| `signature verification failed` | El servidor recibió el paquete, pero la firma no coincide. | El cliente está firmando con una clave privada cuya clave pública **no** está en la sección `users` del `config.yaml` del servidor. |
| `decryption failed` | Firma válida, pero el contenido es ilegible. | Corrupción de datos en tránsito o configuración incorrecta de pares de claves (raro en v2.2). Regenerar claves. |
| `replay detected (cache/time)` | El paquete es idéntico a uno anterior o muy viejo. | 1. El cliente reenvió el mismo paquete.<br>2. Relojes desincronizados (NTP).<br>3. `replay_window_seconds` es muy estricto para la latencia de red actual. |
| `packet timestamp too old/future` | Desincronización horaria severa. | Instale `chrony` o `ntp` en servidor y cliente. Verifique la zona horaria (UTC recomendado internamente). |
| `user not authorized for action` | El usuario existe, pero pidió una acción prohibida. | Revise la lista `actions` dentro del bloque del usuario en `config.yaml`. |
| `source ip not allowed` | Restricción de IP (ACL) activada. | El usuario tiene configurado `source_ips`, y su IP actual no coincide con la lista blanca. |
| `rate limit exceeded` | Ataque o script muy agresivo. | El cliente envió demasiados paquetes en <1s. Aumente `rate_limit_burst` en `config.yaml`. |

---

### 8.4. Problemas de 2FA / TOTP

**Síntoma:** El log muestra `2FA required but missing` o `Invalid OTP code`.

1.  **Código Faltante:**
    El cliente olvidó añadir el argumento.
    *   *Solución:* `ghostknock ... -args "otp=123456"`
2.  **Código Inválido (Drift de Tiempo):**
    El algoritmo TOTP es extremadamente sensible al tiempo. Si el servidor y el móvil tienen una diferencia de >30 segundos, el código fallará.
    *   *Solución:* Sincronice el reloj del servidor (`timedatectl`). GhostKnock v2.2 permite una ventana de +/- 1 intervalo (30s de margen).

---

### 8.5. Fallos en Ejecución de Comandos (Nivel 5)

**Síntoma:** El log dice `Action authorized` y luego `Command execution failed` o `exit status 127`.

1.  **Error "exit status 127" (Command not found):**
    El entorno de `systemd` es minimalista. No tiene el mismo `PATH` que su usuario de consola.
    *   *Mal:* `command: "docker restart ..."`
    *   *Bien:* `command: "/usr/bin/docker restart ..."` (Use siempre rutas absolutas).

2.  **Error de Permisos (`run_as_user`):**
    Si define `run_as_user: "www-data"`, ese usuario puede no tener permisos para ejecutar el comando o escribir en el archivo de salida.
    *   *Diagnóstico:* Intente ejecutar el comando manualmente: `sudo -u www-data /ruta/comando`.
    *   *Caso común:* Intentar reiniciar servicios (`systemctl`) con un usuario no root suele fallar a menos que se configure `sudoers` sin password.

3.  **Timeout (`context deadline exceeded`):**
    El comando tardó más de `timeout_seconds`.
    *   *Solución:* Aumente el valor en la definición de la acción. Comandos como `apt-get update` requieren >300s.

4.  **Inyección de Parámetros Fallida:**
    El log muestra `security: invalid characters in param`.
    *   GhostKnock v2.2 prohíbe caracteres como `;`, `&`, `|`, `` ` ``, `$`, y argumentos que empiecen por `-` (para evitar inyección de flags).
    *   *Solución:* Simplifique los argumentos o use un script wrapper en bash que reciba argumentos simples y construya el comando complejo internamente.

---

### 8.6. Debugging de Hooks

**Síntoma:** Los scripts `on_success` o `pre_execute` no se lanzan o fallan.

1.  **Permisos de Ejecución:**
    Asegúrese de que el script tenga bit de ejecución: `chmod +x /usr/local/bin/myscript.sh`.
2.  **Shebang:**
    El script debe empezar con `#!/bin/bash` (o el intérprete correspondiente).
3.  **Variables de Entorno:**
    Recuerde que los hooks reciben la información vía ENV VARS (`GK_USER`, `GK_IP`), no como argumentos de línea de comandos.
    *   *Debug:* Cree un hook dummy:
        ```bash
        #!/bin/bash
        env > /tmp/gk_hook_debug.log
        ```
    Revise el archivo generado para ver qué variables está recibiendo.

---

### 8.7. Problemas con Systemd y Capabilities

Si ha activado el **Hardening** en `ghostknockd.service` (descomentando `CapabilityBoundingSet`):

*   **Error:** `socket: operation not permitted` al inicio.
    *   *Causa:* Falta `CAP_NET_RAW` para abrir el socket de captura.
*   **Error:** Fallo al cambiar de usuario (`setuid`).
    *   *Causa:* Falta `CAP_SETUID` o `CAP_SETGID`.
*   **Solución:** Edite el servicio (`systemctl edit ghostknockd`) y asegure las capacidades mínimas:
    ```ini
    [Service]
    CapabilityBoundingSet=CAP_NET_RAW CAP_SETUID CAP_SETGID CAP_DAC_OVERRIDE
    ```
    *(CAP_DAC_OVERRIDE puede ser necesario si el demonio necesita leer logs o archivos restringidos).*
