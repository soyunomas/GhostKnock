### 1. Introducción y Arquitectura
*   [1.1. Concepto: Single Packet Authorization (SPA) vs Port Knocking tradicional](#11-concepto-single-packet-authorization-spa-vs-port-knocking-tradicional)
*   [1.2. Protocolo v2: Explicación del flujo criptográfico](#12-protocolo-v2-explicación-del-flujo-criptográfico)

### 2. Instalación y Despliegue
*   [2.1. Requisitos del Sistema](#21-requisitos-del-sistema)
*   [2.2. Instalación en Servidor (Linux)](#22-instalación-en-servidor-linux)
*   [2.3. Instalación de Clientes (Linux/Windows)](#23-instalación-de-clientes-linuxwindows)

### 3. Gestión de Infraestructura de Claves (PKI)
*   [3.1. Filosofía: Pares de claves y distribución](#31-filosofía-pares-de-claves-y-distribución)
*   [3.2. Identidad del Servidor (Server Keys)](#32-identidad-del-servidor-server-keys)
*   [3.3. Identidad del Cliente (Client Keys)](#33-identidad-del-cliente-client-keys)
*   [3.4. Permisos del Sistema de Archivos](#34-permisos-del-sistema-de-archivos)

### 4. Configuración del Servidor (ghostknockd)
*   [4.1. Estructura de config.yaml](#41-estructura-de-configyaml)
*   [4.2. Configuración de Red (listener)](#42-configuración-de-red-listener)
*   [4.3. Políticas de Seguridad Global](#43-políticas-de-seguridad-global)
*   [4.4. Gestión de Usuarios y ACLs](#44-gestión-de-usuarios-y-acls)

### 5. Definición de Acciones y Ejecución
*   [5.1. Anatomía de una Acción](#51-anatomía-de-una-acción)
*   [5.2. Inyección de Parámetros Dinámicos](#52-inyección-de-parámetros-dinámicos)
*   [5.3. Privacidad y Logs (Sensitive Params)](#53-privacidad-y-logs-sensitive-params)
*   [5.4. Mecanismos de Control de Flujo (Cooldown/Revert)](#54-mecanismos-de-control-de-flujo-cooldownrevert)

### 6. Uso del Cliente CLI (ghostknock)
*   [6.1. Sintaxis General y Flags](#61-sintaxis-general-y-flags)
*   [6.2. Estableciendo el contexto criptográfico](#62-estableciendo-el-contexto-criptográfico)
*   [6.3. Envío de Argumentos y Argument Parsing](#63-envío-de-argumentos-y-argument-parsing)
*   [6.4. Códigos de Salida y Manejo de Errores](#64-códigos-de-salida-y-manejo-de-errores)

### 7. Recetario de Operaciones (Use Cases)
*   [7.1. Gestión de Usuarios (User Management)](#71-gestión-de-usuarios-user-management)
*   [7.2. Gestión de Servicios (Service Management)](#72-gestión-de-servicios-service-management)
*   [7.3. Virtualización y Contenedores (Proxmox/Docker)](#73-virtualización-y-contenedores-proxmoxdocker)
*   [7.4. Redes y Seguridad (Firewall)](#74-redes-y-seguridad-firewall)
*   [7.5. DevOps y Mantenimiento](#75-devops-y-mantenimiento)

### 8. Troubleshooting y Observabilidad
*   [8.1. Interpretación de Logs](#81-interpretación-de-logs)
*   [8.2. Validación de Configuración](#82-validación-de-configuración)
*   [8.3. Errores Comunes](#83-errores-comunes)
*   [8.4. Verificación de Seguridad](#84-verificación-de-seguridad)

### 9. Anexo: Referencia Técnica
*   [9.1. Especificación del Payload JSON](#91-especificación-del-payload-json)
*   [9.2. Vectores de Ataque Mitigados](#92-vectores-de-ataque-mitigados)
*   [9.3. Flags de Compilación y Versiones](#93-flags-de-compilación-y-versiones)

# 1. Introducción y Arquitectura

GhostKnock es una implementación moderna del concepto de **Single Packet Authorization (SPA)**, diseñada para ocultar servicios críticos detrás de un firewall "invisible" que solo se abre ante solicitudes criptográficamente válidas.

A diferencia de las VPNs tradicionales o los servicios expuestos públicamente, GhostKnock no mantiene puertos TCP abiertos en estado `LISTEN` detectables por escáneres como Nmap o Shodan. El servidor captura pasivamente el tráfico UDP y solo reacciona si el paquete cumple estrictos requisitos de autenticación y confidencialidad.

## 1.1. Concepto: Single Packet Authorization (SPA) vs Port Knocking tradicional

Aunque a menudo se confunden, GhostKnock soluciona las vulnerabilidades inherentes al "Port Knocking" de la vieja escuela.

| Característica | Port Knocking Tradicional | GhostKnock (SPA) |
| :--- | :--- | :--- |
| **Mecanismo** | Secuencia de intentos de conexión a puertos cerrados (ej. 7000 → 8000 → 9000). | Un único paquete UDP con carga útil cifrada. |
| **Seguridad** | **Baja**. Vulnerable a ataques de repetición (Replay) y observación de paquetes (Sniffing). "Seguridad por oscuridad". | **Alta**. Basada en criptografía asimétrica (Curve25519). |
| **Confidencialidad** | Nula. Un observador sabe qué secuencia abre el puerto. | Total. El contenido del paquete (acción y parámetros) es indescifrable. |
| **Velocidad** | Lenta. Requiere múltiples RTT (Round Trip Time). | Inmediata. Un solo paquete dispara la acción. |
| **Detección** | Detectable por análisis de patrones de tráfico. | Indistinguible de tráfico UDP aleatorio o ruido de red. |

---

## 1.2. Protocolo v2: Explicación del flujo criptográfico

La versión 2.0.0 de GhostKnock introduce un endurecimiento significativo del protocolo mediante una arquitectura de **"Encrypt-then-Sign"** (Cifrar y luego Firmar). Esto garantiza que el servidor pueda validar la identidad del remitente antes de intentar descifrar cualquier dato, mitigando ataques de agotamiento de recursos.

### Capa de Transporte: UDP (Firewall Invisible)
GhostKnock opera exclusivamente sobre **UDP**. El servidor utiliza `libpcap` (en Linux) para inspeccionar paquetes que llegan a la interfaz de red, descartando silenciosamente cualquier paquete que no cumpla con el formato esperado.
*   **Sin ACKs:** El protocolo es unidireccional ("Fire and Forget"). El servidor **nunca** envía una respuesta al cliente, ni siquiera en caso de error. Esto evita que un atacante pueda confirmar la existencia del servicio.

### Capa de Autenticación: Firmas Ed25519
Cada paquete enviado por un cliente debe estar firmado digitalmente.
*   **Algoritmo:** Ed25519 (EdDSA).
*   **Función:** Garantiza la **Integridad** y la **Autenticidad**. El servidor verifica la firma contra la clave pública del usuario (`public_key` en `config.yaml`). Si la firma no es válida, el paquete se descarta inmediatamente, antes de cualquier operación de descifrado costosa.

### Capa de Confidencialidad: Cifrado asimétrico X25519 (nacl/box)
El cuerpo del mensaje (Payload) está cifrado de extremo a extremo para que solo el servidor objetivo pueda leerlo.
*   **Algoritmo:** X25519 (intercambio de claves Elliptic Curve Diffie-Hellman sobre Curve25519) combinado con XSalsa20 y Poly1305 (Authenticated Encryption). Implementación estándar `nacl/box` de Go.
*   **Función:** Protege la privacidad de la acción solicitada y, críticamente, de los **argumentos** (contraseñas, tokens, IPs). Un atacante que capture el paquete no podrá saber qué comando se está ejecutando.

### Estructura del Payload
Una vez descifrado y verificado, el servidor obtiene una estructura JSON con los siguientes campos obligatorios:

```json
{
  "timestamp": 1678900000000000000,
  "action_id": "open-ssh",
  "params": {
    "target_ip": "192.168.1.50",
    "user": "admin"
  }
}
```

1.  **Timestamp (`int64`)**: Marca de tiempo en nanosegundos (Unix format).
    *   Se utiliza para la protección **Anti-Replay**. Si el timestamp difiere del reloj del servidor por más de `replay_window_seconds` (por defecto 5s) o si la firma ya existe en la caché de memoria, el paquete es rechazado.
2.  **ActionID (`string`)**: Identificador de la acción a ejecutar (debe coincidir con una clave en `config.yaml`).
3.  **Params (`map[string]string`)**: Argumentos dinámicos inyectados en el comando. Estos valores son sanitizados estrictamente antes de la ejecución.

# 2. Instalación y Despliegue

Esta sección detalla los procedimientos necesarios para implementar GhostKnock en un entorno de producción. Se asume una arquitectura cliente-servidor donde el demonio (`ghostknockd`) se ejecuta en la infraestructura a proteger (Linux) y los operadores utilizan la herramienta CLI (`ghostknock`) desde sus estaciones de trabajo (Linux o Windows).

### 2.1. Requisitos del Sistema

Antes de la instalación, asegúrese de cumplir con las dependencias mínimas.

**Lado Servidor (El objetivo a proteger):**
*   **Sistema Operativo:** Linux (Kernel 4.x o superior). Probado en Debian 11/12, Ubuntu 20.04/22.04.
*   **Arquitectura:** amd64 (x86_64) o arm64.
*   **Librerías:** `libpcap0.8` (o superior) es **obligatoria** para la captura de paquetes en modo promiscuo.
*   **Gestor de Servicios:** `systemd` (para la gestión automática del demonio).
*   **Red:** Acceso root o capacidad `CAP_NET_RAW` para abrir sockets de red raw.

**Lado Cliente (El operador):**
*   **Linux:** Cualquier distribución moderna. No requiere dependencias externas (binario estático).
*   **Windows:** Windows 10/11 o Windows Server 2019+ (PowerShell o CMD).

---

### 2.2. Instalación en Servidor (Linux)

Utilice los paquetes `.deb` precompilados para garantizar una correcta configuración de permisos y la integración con systemd.

1.  **Descargar el paquete**
    Obtenga la última versión de `ghostknock_2.0.0_amd64.deb` desde el repositorio oficial o su servidor de distribución.

2.  **Instalar el paquete**
    Como usuario `root` o con `sudo`, instale el paquete. Si `libpcap` no está presente, use `apt-get` para corregirlo.

    ```bash
    # Instalación directa
    sudo dpkg -i ghostknock_2.0.0_amd64.deb

    # Si hay errores de dependencias (falta libpcap), ejecute:
    sudo apt-get update && sudo apt-get install -f
    ```

3.  **Verificar el estado del servicio**
    El instalador crea el usuario, configura los permisos en `/etc/ghostknock` y registra el servicio `ghostknockd`. Por defecto, el servicio **no se inicia automáticamente** hasta que se configure (ver Sección 4), pero debe estar cargado.

    ```bash
    systemctl status ghostknockd
    # Estado esperado: "loaded" (inactive/dead)
    ```

---

### 2.3. Instalación de Clientes (Linux/Windows)

El cliente solo requiere las herramientas CLI (`ghostknock` y `ghostknock-keygen`). No instale el paquete de servidor completo en las máquinas de los usuarios.

#### Opción A: Cliente Linux (Paquete .deb ligero)

Instale el paquete `ghostknock-client`, que excluye el demonio y los archivos de configuración del sistema.

```bash
sudo dpkg -i ghostknock-client_2.0.0_amd64.deb
```

**Verificación:**
```bash
ghostknock -version
# Salida: ghostknock version 2.0.0
```

#### Opción B: Cliente Windows (Binarios .exe)

GhostKnock no utiliza instaladores MSI en Windows; se distribuye como ejecutables portables.

1.  **Descarga y Ubicación**: Descargue `ghostknock.exe` y `ghostknock-keygen.exe`. Muévalos a una carpeta permanente, por ejemplo: `C:\Program Files\GhostKnock\`.
2.  **Variable de Entorno (Opcional pero recomendado)**: Añada la ruta al `PATH` del sistema para ejecutar los comandos desde cualquier terminal.

    **PowerShell (como Administrador):**
    ```powershell
    $path = [Environment]::GetEnvironmentVariable("Path", "Machine")
    $newPath = $path + ";C:\Program Files\GhostKnock\"
    [Environment]::SetEnvironmentVariable("Path", $newPath, "Machine")
    ```

**Verificación:**
Abra una nueva terminal (PowerShell o CMD) y ejecute:
```powershell
ghostknock.exe -version
```

# 3. Gestión de Infraestructura de Claves (PKI)

GhostKnock v2.0.0 implementa un modelo de confianza cero basado en criptografía asimétrica de doble vía. A diferencia de versiones anteriores o sistemas basados en contraseñas compartidas (PSK), esta arquitectura requiere una gestión estricta de las identidades digitales tanto del servidor como de los clientes.

### 3.1. Filosofía: Pares de claves y distribución

El sistema se basa en dos operaciones criptográficas distintas que requieren pares de claves `Ed25519`:

1.  **Autenticación (Firma):** El Cliente usa su **Clave Privada** para firmar el paquete. El Servidor usa la **Clave Pública del Cliente** para verificar que el remitente es quien dice ser.
2.  **Confidencialidad (Cifrado):** El Cliente usa la **Clave Pública del Servidor** para cifrar el payload. El Servidor usa su **Clave Privada** para descifrar el contenido.

**Matriz de Distribución:**

| Clave | Ubicación de la Parte Privada (Secreto) | Ubicación de la Parte Pública (Pública) |
| :--- | :--- | :--- |
| **Identidad Cliente** | En la PC del usuario (`~/.config/...`) | En el `config.yaml` del Servidor |
| **Identidad Servidor** | En el Servidor (`/etc/ghostknock/...`) | En la PC del usuario (argumento `-server-pubkey`) |

---

### 3.2. Identidad del Servidor (Server Keys)

Para que el servidor pueda recibir mensajes cifrados, debe generar su propia identidad. Esto se realiza una única vez durante la instalación.

**Procedimiento:**

1.  Generar el par de claves en el directorio de configuración (requiere root).
    ```bash
    sudo ghostknock-keygen -o /etc/ghostknock/server_key
    ```
    *Salida esperada:* Se crearán `server_key` (privada) y `server_key.pub` (pública).

2.  Configurar el demonio para usar esta identidad. Edite `/etc/ghostknock/config.yaml`:
    ```yaml
    server_private_key_path: "/etc/ghostknock/server_key"
    ```

3.  **Distribución:** Debe entregar el archivo `/etc/ghostknock/server_key.pub` (o su contenido) a todos los usuarios legítimos que necesiten conectarse. **Sin este archivo, los clientes no pueden cifrar mensajes para este servidor.**

---

### 3.3. Identidad del Cliente (Client Keys)

Cada operador humano o bot de automatización debe tener su propio par de claves.

**Procedimiento:**

1.  Ejecutar el generador en la máquina local (no requiere root).
    ```bash
    ghostknock-keygen
    ```
    *Comportamiento:* Si no se especifican flags, las claves se guardan en:
    *   Linux/Mac: `~/.config/ghostknock/id_ed25519`
    *   Windows: `%USERPROFILE%\.config\ghostknock\id_ed25519`

2.  Obtener la cadena pública Base64. El comando mostrará al final la cadena pública necesaria. Si necesita recuperarla después:
    ```bash
    cat ~/.config/ghostknock/id_ed25519.pub
    # Ejemplo: ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIM... (Solo la cadena Base64 es necesaria)
    ```

3.  **Autorización:** El administrador del servidor debe agregar esta cadena en la sección `users` del archivo `config.yaml` del servidor.

---

### 3.4. Permisos del Sistema de Archivos

La seguridad criptográfica es irrelevante si las claves privadas son legibles por otros usuarios del sistema. GhostKnock impone una higiene estricta de permisos.

⚠️ **ADVERTENCIA DE SEGURIDAD**: El demonio puede negarse a iniciar o advertir severamente si detecta permisos inseguros.

**En el Servidor:**
El directorio de configuración y la clave privada deben ser propiedad de `root` y no accesibles por nadie más.

```bash
# Blindar el directorio (lectura/ejecución solo para dueño)
sudo chmod 700 /etc/ghostknock

# Blindar los archivos sensibles (lectura/escritura solo para dueño)
sudo chmod 600 /etc/ghostknock/server_key
sudo chmod 600 /etc/ghostknock/config.yaml
```

**En el Cliente:**
Proteja su clave privada de usuario.

```bash
chmod 600 ~/.config/ghostknock/id_ed25519
```

# 4. Configuración del Servidor (ghostknockd)

El comportamiento del demonio se controla centralmente a través del archivo `/etc/ghostknock/config.yaml`. Este archivo utiliza el formato YAML estándar. Es vital respetar la indentación (espacios, no tabuladores) para evitar errores de sintaxis al inicio del servicio.

## 4.1. Estructura de config.yaml

El archivo se divide en bloques lógicos. A continuación se presenta la estructura esqueleto obligatoria para la versión 2.0.0.

```yaml
# 1. IDENTIDAD DEL SERVIDOR (Raíz)
server_private_key_path: "/etc/ghostknock/server_key"

# 2. CAPA DE RED
listener:
  interface: "eth0"
  port: 3001
  listen_ip: "" # Opcional

# 3. LOGGING
logging:
  log_level: "info"

# 4. GESTIÓN DE PROCESO
daemon:
  pid_file: "/var/run/ghostknockd.pid"

# 5. SEGURIDAD GLOBAL (Opcional)
security:
  replay_window_seconds: 5
  rate_limit_per_second: 1.0

# 6. USUARIOS Y PERMISOS
users:
  - name: "admin"
    public_key: "..."
    actions: ["open-ssh"]

# 7. CATÁLOGO DE ACCIONES
actions:
  "open-ssh":
    command: "..."
```

---

## 4.2. Configuración de Red (listener)

Esta sección define cómo el demonio "escucha" el tráfico entrante utilizando `libpcap`.

*   **`interface` (Requerido):** El nombre de la interfaz de red física o virtual (ej: `eth0`, `ens33`, `wg0`).
*   **`port` (Requerido):** El puerto UDP que se monitorizará.
*   **`listen_ip` (Opcional):** Permite filtrar tráfico destinado a una IP específica.

```yaml
listener:
  interface: "eth0"
  port: 3001
  listen_ip: ""
```

### 🧠 Concepto Técnico Crítico: La "Invisibilidad" y el Firewall

Para que GhostKnock sea indetectable, el puerto elegido (ej. 3001) debe estar **BLOQUEADO (`DROP`)** en el firewall de su sistema operativo.

**¿Por qué funciona esto?**
El tráfico sigue un flujo bifurcado al llegar a la tarjeta de red:

1.  **Camino A (GhostKnock):** La librería `libpcap` intercepta una **copia** del paquete directamente desde la tarjeta de red. GhostKnock lo analiza, verifica la firma criptográfica y ejecuta la acción si es válida. Esto ocurre independientemente del firewall.
2.  **Camino B (Kernel/Firewall):** El paquete original sigue su curso hacia el Kernel. Si su firewall (iptables/nftables) está configurado en `DROP`, el paquete se elimina silenciosamente.

**El error de usar ACCEPT:**
Si usted permite el tráfico (`ACCEPT`), el Kernel buscará un proceso escuchando activamente en ese puerto. Como GhostKnock usa captura pasiva y no un socket estándar, el Kernel no encontrará a nadie y responderá con un mensaje ICMP "Port Unreachable". **Esta respuesta revela al atacante que su servidor existe y está encendido.**

**Configuración Correcta (Iptables):**
Debe configurar una regla que descarte silenciosamente los paquetes.

```bash
# ✅ CORRECTO: DROP (Agujero negro, silencio total)
sudo iptables -A INPUT -p udp --dport 3001 -j DROP

# ❌ INCORRECTO: REJECT (Envía respuesta de rechazo, revela presencia)
# ❌ INCORRECTO: ACCEPT (Envía error ICMP si no hay socket bind, revela presencia)

---

## 4.3. Políticas de Seguridad Global

La sección `security:` es opcional pero altamente recomendada para entornos hostiles. Si se omite, se aplican valores por defecto seguros.

*   **`replay_window_seconds` (Default: 5):** Define la "frescura" requerida del paquete. El servidor compara el `timestamp` del payload con su reloj local. Si la diferencia es mayor a X segundos, el paquete se descarta.
    *   *Nota:* Aumentar este valor ayuda si hay desfase horario entre cliente y servidor (drift), pero incrementa teóricamente la ventana para ataques de repetición.
*   **`rate_limit_per_second` y `rate_limit_burst`:** Configuración del algoritmo *Token Bucket* para protección Anti-DoS. Limita cuántos paquetes se procesan por segundo desde una única dirección IP de origen antes de ni siquiera intentar verificar la firma criptográfica.
*   **`default_action_cooldown_seconds` (Default: 15):** Tiempo de enfriamiento base que se aplica a cualquier acción que no especifique su propio `cooldown_seconds`. Evita que un usuario ejecute comandos accidentalmente múltiples veces seguidas.

```yaml
security:
  replay_window_seconds: 10
  rate_limit_per_second: 2.0  # 2 paquetes por segundo sostenidos
  rate_limit_burst: 5         # Ráfaga máxima de 5 paquetes
```

---

## 4.4. Gestión de Usuarios y ACLs

La sección `users` define la lista blanca de identidades autorizadas. Cada entrada vincula una clave criptográfica con permisos específicos.

*   **`name`**: Etiqueta para identificar al usuario en los logs (ej: "sysadmin_juan").
*   **`public_key`**: La cadena Base64 generada por `ghostknock-keygen` del usuario. **Sin encabezados** tipo "ssh-ed25519", solo el payload Base64.
*   **`actions`**: Lista estricta de IDs de acciones que este usuario puede solicitar. Si un usuario firma un paquete pidiendo una acción que no está en esta lista, el servidor lo ignora.
*   **`source_ips` (Opcional)**: Una lista de CIDRs o IPs (v4/v6). Si se define, el servidor verificará que el paquete provenga físicamente de estas direcciones. Si está vacío, se permite acceso desde cualquier IP (confiando solo en la criptografía).

```yaml
users:
  - name: "operador_vpn"
    public_key: "MC4CAQAwBQYDK2VwBCIEIN..."
    source_ips:
      - "192.168.1.0/24"  # Solo desde la LAN
      - "10.0.0.5/32"     # Solo desde esta IP de gestión
    actions:
      - "restart-vpn"
      - "status-vpn"
```

# 5. Definición de Acciones y Ejecución

El corazón funcional de GhostKnock reside en la sección `actions` del archivo de configuración. Aquí se define qué comandos puede ejecutar el servidor y bajo qué condiciones.

## 5.1. Anatomía de una Acción

Cada acción se identifica por una clave única (ActionID) y contiene los siguientes campos configurables:

```yaml
actions:
  "backup-db":
    # [Obligatorio] El comando de shell a ejecutar
    command: "/usr/local/bin/backup.sh"
    
    # [Opcional] Usuario del sistema que ejecutará el proceso.
    # Por defecto es 'root' (si el demonio corre como root).
    # RECOMENDACIÓN: Usar usuarios con privilegios mínimos (ej: www-data).
    run_as_user: "postgres"
    
    # [Opcional] Tiempo máximo de ejecución antes de enviar SIGKILL.
    timeout_seconds: 300
```

---

## 5.2. Inyección de Parámetros Dinámicos

GhostKnock permite enviar argumentos desde el cliente para hacer los comandos dinámicos. El servidor utiliza el motor de plantillas de Go (`text/template`) para inyectar estos valores.

### Variables Disponibles
*   `{{.SourceIP}}`: La dirección IP desde la que se recibió el paquete válido. Útil para reglas de firewall.
*   `{{.Params.NOMBRE_CLAVE}}`: Valores enviados por el cliente con el flag `-args`.

### Sanitización Estricta (Hardening)
Por seguridad, **no se permite cualquier texto**. El servidor aplica una lista blanca estricta (Allowlist) a todos los parámetros recibidos. Si un parámetro contiene caracteres ilegales, el paquete se descarta.

*   **Caracteres Permitidos:** Letras (`a-z`, `A-Z`), Números (`0-9`), Punto (`.`), Guion bajo (`_`) y Guion medio (`-`).
*   **Regla Anti-Inyección de Flags:** Un parámetro **NO** puede comenzar con un guion medio (`-`). Esto previene que un usuario inyecte opciones adicionales a un comando (ej: transformar un `ls` en `ls -R`).

**Ejemplo Seguro:**
```yaml
  "ping-check":
    # El usuario envía -args "target=8.8.8.8"
    command: "ping -c 4 {{.Params.target}}"
```

---

## 5.3. Privacidad y Logs (Sensitive Params)

En la versión 2.0.0, se introdujo la directiva `sensitive_params`. Esto es crucial cuando se envían credenciales, contraseñas o tokens a través de GhostKnock.

Aunque el tráfico de red está cifrado, los logs del sistema (`/var/log/ghostknockd.log`) suelen guardar el comando ejecutado en texto plano. Esta función permite redactar (ocultar) automáticamente valores específicos en los logs.

**Configuración:**
```yaml
  "create-user":
    command: "useradd -m -p {{.Params.pass}} {{.Params.user}}"
    # Lista de claves cuyos valores serán reemplazados por '*****' en los logs
    sensitive_params:
      - "pass"
```

**Resultado en el Log:**
```text
INFO: Ejecutando comando: [REDACTADO] useradd -m -p {{.Params.pass}} {{.Params.user}} (Valores ocultos por sensitive_params)
DEBUG: params=map[user:juan pass:*****]
```

---

## 5.4. Mecanismos de Control de Flujo (Cooldown/Revert)

Para prevenir abusos y automatizar la seguridad, cada acción soporta lógica de enfriamiento y reversión.

### Cooldown (Enfriamiento)
Evita la ejecución repetida (spamming) de una misma acción.
*   **`cooldown_seconds`**: Segundos que deben pasar antes de que *ese usuario* pueda ejecutar *esa acción* de nuevo.
    *   Valor `> 0`: Tiempo específico.
    *   Valor `0`: Sin enfriamiento (permite ráfagas).
    *   Sin definir (null): Hereda el `default_action_cooldown_seconds` de la sección `security`.

### Revert (Reversión Automática)
Ideal para operaciones temporales, como abrir puertos en el firewall. GhostKnock ejecutará automáticamente un "contra-comando" después de un tiempo definido.

```yaml
  "open-ssh":
    # 1. Ejecutar esto inmediatamente
    command: "iptables -I INPUT -s {{.SourceIP}} -p tcp --dport 22 -j ACCEPT"
    
    # 2. Esperar este tiempo (ej: 5 minutos)
    revert_delay_seconds: 300
    
    # 3. Ejecutar esto automáticamente al finalizar la espera
    revert_command: "iptables -D INPUT -s {{.SourceIP}} -p tcp --dport 22 -j ACCEPT"
```

# 6. Uso del Cliente CLI (ghostknock)

El cliente `ghostknock` es una herramienta de línea de comandos ligera y sin estado. Su única función es construir el paquete cifrado, firmarlo y enviarlo a la red.

## 6.1. Sintaxis General y Flags

La estructura básica del comando es:

```bash
ghostknock -host <IP_DESTINO> -server-pubkey <ARCHIVO_PUB_SERVER> -action <ID_ACCION> [opciones]
```

| Flag | Tipo | Obligatorio | Descripción |
| :--- | :--- | :---: | :--- |
| `-host` | string | ✅ | Dirección IP o nombre de host del servidor objetivo. |
| `-server-pubkey` | path | ✅ | Ruta al archivo que contiene la clave pública del servidor (`server_key.pub`). |
| `-action` | string | ✅ | El ID de la acción a ejecutar (debe coincidir con `config.yaml` del servidor). |
| `-args` | string | ❌ | Lista de parámetros clave=valor para inyectar en el comando (Ej: `user=bob`). |
| `-key` | path | ❌ | Ruta a tu clave privada. Por defecto busca en `~/.config/ghostknock/id_ed25519`. |
| `-port` | int | ❌ | Puerto UDP destino. Por defecto: `3001`. |
| `-version` | bool | ❌ | Muestra la versión del cliente y sale. |

---

## 6.2. Estableciendo el contexto criptográfico

A partir de la versión 2.0.0, **es obligatorio especificar la clave pública del servidor** para cada solicitud. El cliente necesita esta clave para cifrar el payload de manera que solo el servidor pueda leerlo.

Existen dos claves en juego durante la ejecución:

1.  **Tu Clave Privada (`-key`)**: Se carga automáticamente desde la ruta por defecto (`~/.config/ghostknock/id_ed25519` en Linux o `%USERPROFILE%\.config\ghostknock\id_ed25519` en Windows). Si guardaste tu clave en otro lugar, úsala explícitamente.
2.  **La Clave Pública del Servidor (`-server-pubkey`)**: El administrador del sistema debe proporcionarte este archivo.

**Ejemplo de uso explícito:**

```bash
ghostknock -host 192.168.1.50 \
           -key /home/usuario/mis_claves/id_ed25519 \
           -server-pubkey /home/usuario/mis_claves/servidor_produccion.pub \
           -action status-check
```

---

## 6.3. Envío de Argumentos y Argument Parsing

El flag `-args` permite enviar datos dinámicos. El formato interno es una cadena delimitada por comas.

**Sintaxis:**
`-args "clave1=valor1,clave2=valor2"`

**Reglas de Formato:**
1.  **Comillas:** Siempre encierra la cadena de argumentos entre comillas dobles `"` para evitar que la shell interprete caracteres especiales.
2.  **Separador:** Usa comas `,` para separar múltiples parámetros.
3.  **Sin Espacios:** No dejes espacios después de la coma (ej: `key=val, key2=val` es inválido; usa `key=val,key2=val`).
4.  **Caracteres Válidos:** Solo se permiten caracteres alfanuméricos, puntos, guiones bajos y guiones medios.

**Ejemplo:**
```bash
# Correcto
ghostknock ... -args "target=10.0.0.5,level=debug"

# Incorrecto (Espacios, caracteres prohibidos)
ghostknock ... -args "target=10.0.0.5; rm -rf /" 
```

---

## 6.4. Códigos de Salida y Manejo de Errores

Debido a la naturaleza del protocolo UDP y la política de seguridad de "silencio total" del servidor, el cliente tiene una visibilidad limitada sobre el éxito de la operación.

*   **Código de Salida 0 (Éxito):** Significa que el paquete fue generado, cifrado, firmado y enviado a la red correctamente.
    *   ⚠️ **ADVERTENCIA:** Esto **NO** garantiza que el servidor lo haya recibido, aceptado o ejecutado. Si la firma es inválida, el puerto es incorrecto o el usuario no tiene permiso, el servidor descartará el paquete silenciosamente y el cliente seguirá mostrando éxito.

*   **Código de Salida 1 (Error Local):** Significa que el cliente falló antes de enviar nada.
    *   Causas comunes:
        *   Faltan argumentos obligatorios (`-host`, `-action`, etc.).
        *   Archivos de clave no encontrados o corruptos.
        *   Error de resolución DNS para el host.
        *   Formato de `-args` inválido.

**Verificación de Ejecución:**
Dado que el cliente no recibe feedback, la verificación debe hacerse por canales laterales ("Out-of-Band"):
1.  Intentar conectar al servicio solicitado (ej: intentar SSH tras el knock).
2.  Si tienes acceso, revisar los logs del servidor: `tail -f /var/log/ghostknockd.log`.

# 7. Recetario de Operaciones (Use Cases)

A continuación se presentan 20 ejemplos prácticos de configuración para distintos roles de administración de sistemas. Copie los bloques YAML en su archivo `/etc/ghostknock/config.yaml` y utilice los comandos de cliente correspondientes.

⚠️ **Nota:** Todos los ejemplos asumen que el cliente ha configurado correctamente el flag `-server-pubkey`.

## 7.1. Gestión de Usuarios (User Management)

#### 1. Lead SysAdmin
**Uso:** Crear un usuario de emergencia porque el LDAP ha caído.
*   **Restricción:** La contraseña debe ser alfanumérica por la regex (ej: `Socorro2025`). El usuario deberá cambiarla al entrar.
*   **YAML (Server):**
    ```yaml
    "create-admin":
      # Crea usuario, lo añade a sudo y asigna pass. Oculta el parametro 'pass' en logs.
      command: "useradd -m -G sudo -p $(openssl passwd -1 {{.Params.pass}}) {{.Params.user}}"
      sensitive_params: ["pass"]
    ```
*   **Cliente:**
    ```bash
    ghostknock -action create-admin -args "user=admin_temp,pass=Socorro2025" ...
    ```

#### 2. Security Officer
**Uso:** Bloquear (Lock) la cuenta de un usuario comprometido inmediatamente.
*   **YAML (Server):**
    ```yaml
    "lock-user":
      command: "usermod -L {{.Params.username}}"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action lock-user -args "username=pepe_despido" ...
    ```

#### 3. Auditor de Accesos
**Uso:** Forzar el cierre de todas las sesiones de un usuario específico.
*   **YAML (Server):**
    ```yaml
    "kill-user-sessions":
      command: "pkill -KILL -u {{.Params.username}}"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action kill-user-sessions -args "username=invitado" ...
    ```

---

## 7.2. Gestión de Servicios (Service Management)

#### 4. Web Server Admin (Nginx/Apache)
**Uso:** Reiniciar el servidor web tras cambiar una configuración, sin entrar por SSH.
*   **YAML (Server):**
    ```yaml
    "restart-web":
      command: "systemctl restart nginx"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action restart-web ...
    ```

#### 5. Database Administrator (MySQL/MariaDB)
**Uso:** Parar la base de datos para mantenimiento en frío.
*   **YAML (Server):**
    ```yaml
    "stop-db":
      command: "systemctl stop mariadb"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action stop-db ...
    ```

#### 6. Application Support (PHP-FPM/Python)
**Uso:** Reiniciar un servicio genérico pasando su nombre como parámetro.
*   **YAML (Server):**
    ```yaml
    "bounce-svc":
      command: "systemctl restart {{.Params.service}}"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action bounce-svc -args "service=php8.1-fpm" ...
    ```

#### 7. Legacy System Admin
**Uso:** Comprobar si un servicio está activo (escribe el estado en un fichero temporal público).
*   **YAML (Server):**
    ```yaml
    "check-status":
      command: "systemctl is-active {{.Params.service}} > /var/www/html/status.txt"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action check-status -args "service=ssh" ...
    ```

---

## 7.3. Virtualización y Contenedores (Proxmox/Docker)

#### 8. Proxmox VE Administrator
**Uso:** Desbloquear (unlock) una máquina virtual que se quedó pillada tras un backup fallido.
*   **YAML (Server):**
    ```yaml
    "pve-unlock":
      command: "qm unlock {{.Params.vmid}}"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action pve-unlock -args "vmid=102" ...
    ```

#### 9. Virtualization Operator
**Uso:** Iniciar una VM específica en Proxmox.
*   **YAML (Server):**
    ```yaml
    "pve-start":
      command: "qm start {{.Params.vmid}}"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action pve-start -args "vmid=200" ...
    ```

#### 10. Docker Specialist
**Uso:** Reiniciar un contenedor específico por su nombre o ID.
*   **YAML (Server):**
    ```yaml
    "docker-restart":
      command: "docker restart {{.Params.container}}"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action docker-restart -args "container=mi-app-web" ...
    ```

#### 11. Kubernetes Admin (Kubelet)
**Uso:** Reiniciar el servicio `kubelet` en un nodo trabajador que no responde.
*   **YAML (Server):**
    ```yaml
    "restart-kubelet":
      command: "systemctl restart kubelet"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action restart-kubelet ...
    ```

---

## 7.4. Redes y Seguridad (Firewall)

#### 12. Network Defender (Firewall Admin)
**Uso:** Banear una IP atacante inmediatamente en iptables.
*   **YAML (Server):**
    ```yaml
    "ban-ip":
      command: "iptables -A INPUT -s {{.Params.ip}} -j DROP"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action ban-ip -args "ip=1.2.3.4" ...
    ```

#### 13. Support Tier 2
**Uso:** Desbanear la IP de una oficina que fue bloqueada por error (Fail2Ban).
*   **YAML (Server):**
    ```yaml
    "unban-ip":
      command: "iptables -D INPUT -s {{.Params.ip}} -j DROP"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action unban-ip -args "ip=80.10.20.30" ...
    ```

#### 14. VPN Administrator (Wireguard)
**Uso:** Reiniciar la interfaz de la VPN si el túnel se ha caído.
*   **YAML (Server):**
    ```yaml
    "fix-vpn":
      command: "systemctl restart wg-quick@wg0"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action fix-vpn ...
    ```

#### 15. CISO (Modo Pánico)
**Uso:** Activar "Lockdown". Cierra SSH y administración web inmediatamente ante un ataque.
*   **YAML (Server):**
    ```yaml
    "lockdown":
      command: "systemctl stop sshd && systemctl stop webmin"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action lockdown ...
    ```

---

## 7.5. DevOps y Mantenimiento

#### 16. DevOps Engineer (Deploy)
**Uso:** Hacer un `git pull` rápido en una rama específica (solo caracteres seguros).
*   **YAML (Server):**
    ```yaml
    "git-pull":
      run_as_user: "www-data"
      command: "cd /var/www/html && git checkout {{.Params.branch}} && git pull"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action git-pull -args "branch=main" ...
    ```

#### 17. Release Manager (Modo Mantenimiento)
**Uso:** Crear el archivo flag que pone la web en "Estamos en mantenimiento".
*   **YAML (Server):**
    ```yaml
    "set-maint":
      command: "touch /var/www/html/maintenance.on"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action set-maint ...
    ```

#### 18. Backend Developer (Caché)
**Uso:** Limpiar la caché de la aplicación (ej. Laravel o Symfony).
*   **YAML (Server):**
    ```yaml
    "flush-cache":
      run_as_user: "www-data"
      command: "php /var/www/html/artisan cache:clear"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action flush-cache ...
    ```

#### 19. Backup Admin
**Uso:** Desmontar el disco duro USB de backups para protegerlo de Ransomware.
*   **YAML (Server):**
    ```yaml
    "protect-backup":
      command: "umount /mnt/usb_backup"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action protect-backup ...
    ```

#### 20. System Maintenance (Limpieza)
**Uso:** Forzar la rotación de logs (logrotate) si el disco se está llenando.
*   **YAML (Server):**
    ```yaml
    "force-rotate":
      command: "logrotate -f /etc/logrotate.conf"
    ```
*   **Cliente:**
    ```bash
    ghostknock -action force-rotate ...
    ```

    # 8. Troubleshooting y Observabilidad

Dado que GhostKnock está diseñado para ser **invisible y silencioso** (Security through Obscurity + Cryptography), diagnosticar problemas puede ser desafiante si no se sabe dónde mirar. El cliente nunca recibe confirmación de éxito o error, por lo que toda la observabilidad reside en el servidor.

## 8.1. Interpretación de Logs

El demonio registra su actividad en `/var/log/ghostknockd.log` (salvo que se configure otra ruta o se use journald).

### Niveles de Log
Configurable en `config.yaml` bajo `logging.log_level`.

*   **`INFO`**: Muestra el inicio del servicio y **knocks exitosos**.
    ```text
    INFO: Knock válido recibido y autorizado user=admin_remoto source_ip=192.168.1.50 action_id=open-ssh
    INFO: Ejecutando comando en el shell type=main command="iptables -I..."
    ```
*   **`WARN`**: Muestra paquetes **descartados** (ataques, errores de firma, replay).
    ```text
    WARN: Paquete descartado reason=invalid_signature_or_decryption_failed source_ip=203.0.113.88
    WARN: Paquete descartado reason=outside_replay_window source_ip=192.168.1.50 age_seconds=15.2
    ```
*   **`DEBUG`**: Muestra detalles internos, incluyendo parámetros descifrados.
    ℹ️ **NOTA**: Incluso en modo debug, los parámetros marcados en `sensitive_params` aparecerán como `*****`.

### Análisis de Fallos
Para depurar en tiempo real mientras lanzas peticiones desde el cliente:
```bash
tail -f /var/log/ghostknockd.log
```

---

## 8.2. Validación de Configuración

Antes de reiniciar el servicio, siempre verifica la integridad sintáctica y lógica de tu archivo `config.yaml`. GhostKnock incluye un validador estático integrado.

**Procedimiento:**

1.  Ejecuta el binario con el flag `-t`:
    ```bash
    sudo ghostknockd -t -config /etc/ghostknock/config.yaml
    ```

2.  **Salida Exitosa:**
    ```text
    Probando la configuración desde: /etc/ghostknock/config.yaml
    La sintaxis del archivo de configuración es correcta.
    ```

3.  **Salida con Error:**
    El validador indicará la línea exacta y la razón (ej: clave pública malformada, usuario sin acciones, permisos inseguros).
    ```text
    Error: La configuración es INVÁLIDA.
    Detalles: line 42: el usuario 'deploy_bot' no tiene clave pública ('public_key')
    ```

---

## 8.3. Errores Comunes

| Síntoma | Causa Probable | Solución |
| :--- | :--- | :--- |
| **El cliente da éxito, pero no pasa nada.** | El servidor descartó el paquete silenciosamente. | Revisa `/var/log/ghostknockd.log`. Si no hay logs, el paquete no llegó (firewall intermedio) o fue descartado por tamaño/formato antes del log. |
| **Log: `reason=invalid_signature...`** | 1. Clave pública del cliente incorrecta en `config.yaml`. <br> 2. Cliente usó la `-server-pubkey` incorrecta (fallo de descifrado). | Verifica que las claves coinciden en ambos extremos. Regenera si es necesario. |
| **Log: `reason=outside_replay_window`** | Desincronización de reloj entre Cliente y Servidor. | Instala/actualiza `ntp` o `chrony` en ambas máquinas. Aumenta temporalmente `replay_window_seconds`. |
| **Log: `reason=cooldown_active`** | Intentaste ejecutar la acción muy rápido. | Espera el tiempo definido en `cooldown_seconds` o redúcelo en la configuración. |
| **Error: `bind: permission denied`** | El demonio no tiene permisos `CAP_NET_RAW` o root. | Ejecuta el servicio como root o asigna capabilities al binario. |
| **Error: `open ...: permission denied`** | El demonio no puede leer `server_key`. | Asegura que el usuario del proceso (root) es dueño del archivo: `chown root:root ...`. |

⚠️ **ADVERTENCIA SOBRE CLAVES**: Nunca confundas la clave privada del servidor con la pública. Si el cliente cifra con una clave incorrecta, el servidor verá basura y descartará el paquete como "firma inválida".

---

## 8.4. Verificación de Seguridad

GhostKnock debe ser invisible. Aquí se explica cómo verificar que el sistema está funcionando como un "Black Hole".

### 1. Escaneo de Puertos (Nmap)
Desde una máquina externa, escanea el puerto UDP.
```bash
sudo nmap -sU -p 3001 <IP_SERVIDOR>
```
*   **Resultado Esperado:** `open|filtered` o `closed` (dependiendo de tu firewall perimetral).
*   **Nunca debe salir:** `open` con un banner de servicio identificado. GhostKnock no responde a pings ni handshakes.

### 2. Verificación de Llegada (Tcpdump)
Si el cliente dice "enviado" pero no ves logs en GhostKnock, verifica si los paquetes llegan a la tarjeta de red.

En el servidor:
```bash
sudo tcpdump -i any udp port 3001 -nn -X
```
Si ves paquetes llegar aquí pero GhostKnock no loguea nada (ni siquiera WARN), significa que el paquete está malformado (longitud > 1KB) o el filtro BPF está mal configurado en `config.yaml`.

# 9. Anexo: Referencia Técnica

Este anexo proporciona detalles de bajo nivel sobre la implementación interna de GhostKnock v2.0.0. Esta información es útil para auditores de seguridad y desarrolladores que deseen integrar el protocolo en sistemas de terceros.

## 9.1. Especificación del Payload JSON

El paquete UDP que viaja por la red es una estructura binaria opaca (nonce + cifrado + firma). Sin embargo, una vez que el servidor autentica y descifra el mensaje, el contenido resultante (Payload) es un objeto JSON estándar que cumple con el siguiente esquema.

**Estructura de Datos (Go struct):**
```go
type Payload struct {
    Timestamp int64             `json:"timestamp"`
    ActionID  string            `json:"action_id"`
    Params    map[string]string `json:"params,omitempty"`
}
```

**Ejemplo de Payload Descifrado:**
```json
{
  "timestamp": 1678912345000000000,
  "action_id": "open-ssh",
  "params": {
    "ip": "203.0.113.55",
    "user": "sysadmin"
  }
}
```

| Campo | Tipo | Descripción |
| :--- | :--- | :--- |
| `timestamp` | int64 | Tiempo Unix en **nanosegundos**. Crítico para la validación de ventana anti-replay. |
| `action_id` | string | Identificador exacto de la acción configurada en el servidor. Case-sensitive. |
| `params` | map | Diccionario clave-valor con argumentos dinámicos. Solo presente si el cliente envió `-args`. |

---

## 9.2. Vectores de Ataque Mitigados

GhostKnock ha sido diseñado bajo principios de "Secure by Design". A continuación se detalla cómo la arquitectura mitiga las amenazas comunes.

### 1. Ataques de Repetición (Replay Attacks)
Un atacante captura un paquete UDP válido y lo reenvía más tarde para volver a ejecutar la acción.
*   **Mitigación:**
    1.  **Ventana de Tiempo:** El servidor descarta paquetes cuyo `timestamp` difiera más de X segundos (default: 5s) del reloj del servidor.
    2.  **Caché de Unicidad:** El servidor mantiene en memoria (hash map) las firmas de todos los paquetes válidos recibidos dentro de la ventana de tiempo. Si llega un duplicado exacto, se descarta.

### 2. Denegación de Servicio (DoS / Resource Exhaustion)
Un atacante inunda el puerto con basura para saturar la CPU del servidor intentando descifrar.
*   **Mitigación:**
    1.  **Verificación Previa (Auth-then-Decrypt):** La verificación de firma Ed25519 es computacionalmente más barata que el descifrado X25519. Si la firma falla, no se intenta descifrar.
    2.  **Límite de Tamaño:** `MaxPayloadSize` está fijado en 1024 bytes (Hardcoded). Paquetes mayores se descartan en la capa de captura antes de llegar a la lógica de negocio, protegiendo la memoria RAM.
    3.  **Rate Limiting:** Implementación de Token Bucket por IP de origen.

### 3. Inyección de Comandos (RCE)
Un usuario malicioso intenta enviar argumentos como `; rm -rf /` dentro de los parámetros.
*   **Mitigación:**
    *   **Whitelist Estricta:** El ejecutor aplica la regex `^[a-zA-Z0-9._][a-zA-Z0-9._-]*$` a cada valor. Cualquier carácter fuera de este rango (espacios, punto y coma, pipes, comillas) provoca el rechazo inmediato de la acción.
    *   **Anti-Flag Injection:** Se prohíbe explícitamente que los argumentos comiencen con guion `-` para evitar alterar el comportamiento de los binarios del sistema.

### 4. Escaneo y Enumeración de Servicios
Un atacante escanea la red buscando servicios vulnerables.
*   **Mitigación:**
    *   **Silencio Total (Silent Drop):** El protocolo es UDP unidireccional sin ACKs. El servidor no emite paquetes de respuesta bajo ninguna circunstancia (ni ICMP Port Unreachable ni errores de aplicación). Para un escáner, el puerto parece un agujero negro.

---

## 9.3. Flags de Compilación y Versiones

Para verificar la integridad de los binarios o compilar desde el código fuente (para auditoría), utilice las siguientes referencias.

**Verificación de Versión en Runtime:**
Todos los binarios soportan el flag estandarizado:
```bash
ghostknock -version
ghostknockd -version
# Salida esperada: ghostknockd version 2.0.0
```

**Variables de Compilación (LDFLAGS):**
El `Makefile` inyecta la versión en el momento de la compilación para evitar discrepancias.
```makefile
LDFLAGS_VERSION := -ldflags="-X main.version=$(VERSION)"
```

**Compilación Reproducible (Recomendada):**
Para entornos de máxima seguridad, se recomienda compilar con `CGO_ENABLED=0` (estático) y stripping de símbolos para reducir tamaño y ofuscar ligeramente.

```bash
# Ejemplo para Linux AMD64 Hardened
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-s -w -X main.version=2.0.0" \
  -o ghostknockd ./cmd/ghostknockd/
```
