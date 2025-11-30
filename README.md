# 👻 GhostKnock

[![Licencia: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/badge/release-v2.0.0-blue.svg)](https://github.com/soyunomas/GhostKnock/releases)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20windows-lightgrey.svg)]()

**GhostKnock** es un sistema de **ejecución remota segura, invisible y confidencial**.

Permite disparar comandos predefinidos en un servidor enviando un único paquete UDP cifrado.

El servidor escucha pasivamente el tráfico. Si recibe un paquete con una firma válida y un payload cifrado para él, lo descifra y ejecuta la acción asociada. Si no, el paquete es ignorado silenciosamente, haciendo que el servidor sea **indetectable** y su comunicación **indescifrable**.

---

## ✨ Características

* 🔐 **Criptografía Fuerte de Doble Capa:**
    * **Autenticación:** Firmas `Ed25519` para verificar la identidad del remitente.
    * **Confidencialidad:** Cifrado de extremo a extremo con `X25519` (`nacl/box`) para ocultar la acción y los parámetros, evitando fugas de información.

* 🧩 **Parámetros Dinámicos:**
    * El cliente puede enviar argumentos (por ejemplo IPs o nombres de servicio) que se inyectan de forma segura en los comandos del servidor.

* 🛡️ **Seguridad Ofensiva/Defensiva**
    * **Invisible por Diseño:**  
        No expone puertos TCP ni mantiene sockets abiertos. El servidor opera en modo *listener* pasivo, analizando únicamente el tráfico UDP entrante sin responder a paquetes no válidos. Funciona incluso detrás de firewalls con **todos los puertos cerrados**, sin ser detectable por escáneres de red convencionales.
    * **Anti-Replay:**  
        Cada solicitud incluye un *timestamp* y un identificador único. El servidor usa una caché temporal para evitar reutilización maliciosa de paquetes capturados.
    * **Sanitización Estricta:**  
        Todos los parámetros se validan mediante una **allowlist**, bloqueando intentos de inyección de comandos.
    * **Anti-DoS:**  
        Los paquetes que no superan la verificación criptográfica previa se descartan inmediatamente para reducir el consumo de recursos.
* ⚡ **Multiplataforma:**  
    Cliente nativo para **Linux** y **Windows**.

* ⚙️ **Automatización:**  
    Ideal para tareas de CI/CD, recuperación de desastres y gestión de accesos de emergencia.
---


## 📦 Instalación

### Opción A: Paquetes .deb (Debian/Ubuntu/Mint)

Descarga la última versión desde [Releases](https://github.com/soyunomas/GhostKnock/releases).

*   **Para el Servidor (Demonio + Herramientas):**
    ```bash
    sudo dpkg -i ghostknock_2.0.0_amd64.deb
    # Se instala el servicio systemd y se asegura el directorio /etc/ghostknock
    ```

*   **Para Clientes Remotos (Solo Herramientas):**
    ```bash
    sudo dpkg -i ghostknock-client_2.0.0_amd64.deb
    ```

### Opción B: Ejecutables para Windows

Descarga `ghostknock.exe` y `ghostknock-keygen.exe` desde Releases. No requieren instalación. Úsalos directamente desde PowerShell o CMD.

### Opción C: Compilación Manual

Requiere Go 1.21+ y `libpcap-dev` (en Linux).
```bash
git clone https://github.com/soyunomas/GhostKnock.git
cd GhostKnock
make build          # Compila para Linux
make build-windows  # Compila .exe para Windows
```

---

## 🚀 Guía de Inicio Rápido (Protocolo v2 con Cifrado)

### 1. Generar la Identidad del Servidor (En el Servidor)
El servidor necesita su propio par de claves para el cifrado.

```bash
# Como root en el servidor
sudo ghostknock-keygen -o /etc/ghostknock/server_key
# Salida: Claves generadas en /etc/ghostknock/server_key y /etc/ghostknock/server_key.pub
# ¡Asegura los permisos!
sudo chmod 600 /etc/ghostknock/server_key*
```
> **Comparte de forma segura el archivo `/etc/ghostknock/server_key.pub` con todos los clientes.**

### 2. Generar tu Identidad de Cliente (En tu PC)
Necesitas un par de claves: la privada se queda contigo, la pública va al servidor.

```bash
# En tu máquina local (Linux, Mac, Windows)
ghostknock-keygen
```
> **Copia la cadena Base64 de clave pública que aparece en la terminal.**

### 3. Configurar el Servidor
Edita el archivo `/etc/ghostknock/config.yaml` y añade dos cosas: la ruta a la clave privada del servidor y los datos de tu usuario cliente.

```yaml
# Indicar al servidor dónde está su propia identidad secreta
server_private_key_path: "/etc/ghostknock/server_key"

users:
  - name: "admin_remoto"
    public_key: "PEGA_TU_CLAVE_PUBLICA_DE_CLIENTE_AQUI..."
    actions:
      - "write-test"
      - "open-ssh"

actions:
  "write-test":
    command: 'echo "Test OK. P1={{.Params.p1}} P2={{.Params.p2}}" > /tmp/prueba.txt'
    cooldown_seconds: 0
```

### 4. Preparar el Cliente
En tu PC, guarda el archivo `server_key.pub` que te dio el administrador. Por ejemplo, en `~/.config/ghostknock/server.pub`.

### 5. Iniciar el Servicio en el Servidor
```bash
sudo systemctl restart ghostknockd
```

### 6. Enviar tu primer Knock Cifrado
Ahora debes especificar la clave pública del servidor para que el cliente sepa cómo cifrar el mensaje.

```bash
# Linux
ghostknock -host IP_DEL_SERVIDOR \
           -server-pubkey ~/.config/ghostknock/server.pub \
           -action write-test \
           -args "p1=Hola,p2=Mundo"

# Windows
.\ghostknock.exe -host IP_DEL_SERVIDOR `
                 -server-pubkey C:\Users\TuUser\.config\ghostknock\server.pub `
                 -action write-test `
                 -args "p1=Hola,p2=Mundo"
```

---

## 💡 Recetario: 11 Ejemplos Prácticos

A continuación se presentan configuraciones para `config.yaml` y el comando del cliente correspondiente.

> ⚠️ **Nota de Seguridad sobre Parámetros:**
> Los argumentos pasados con `-args` solo permiten: **Letras (a-Z), Números (0-9), Puntos (.), Guiones bajos (_) y Guiones medios (-)**.
> Cualquier otro carácter (espacios, :, /, ;) provocará el rechazo del paquete.
> Además, los parámetros no pueden comenzar con un guion (`-`) para evitar inyección de flags.

### 1. Test de Verificación (Hola Mundo)
Crea un archivo para verificar que el sistema procesa parámetros correctamente.

*   **Config (Server):**
    ```yaml
    "write-test":
      command: 'echo "Este es el parametro1={{.Params.p1}}, parametro2={{.Params.p2}}" > /tmp/prueba.txt'
      cooldown_seconds: 0
    ```
*   **Cliente:**
    ```bash
    ghostknock -host 127.0.0.1 -action write-test -args "p1=ValorUno,p2=Valor_Dos" -server-pubkey RUTA_A_SERVER.PUB
    ```

### 2. Abrir SSH Temporalmente (Port Knocking 2.0)
Abre el puerto 22 solo para tu IP actual y lo cierra automáticamente tras 5 minutos.

*   **Config (Server):**
    ```yaml
    "open-ssh":
      command: "iptables -I INPUT 1 -p tcp -s {{.SourceIP}} --dport 22 -j ACCEPT"
      revert_command: "iptables -D INPUT -p tcp -s {{.SourceIP}} --dport 22 -j ACCEPT"
      revert_delay_seconds: 300
    ```
*   **Cliente:**
    ```bash
    ghostknock -host MISERVIDOR -action open-ssh -server-pubkey RUTA_A_SERVER.PUB
    ```

### 3. Reiniciar Servicios Específicos
Reinicia un servicio pasando su nombre como parámetro.

*   **Config (Server):**
    ```yaml
    "restart-svc":
      command: "systemctl restart {{.Params.name}}"
      timeout_seconds: 10
    ```
*   **Cliente:**
    ```bash
    ghostknock -host MISERVIDOR -action restart-svc -args "name=nginx" -server-pubkey RUTA_A_SERVER.PUB
    ```

### 4. Banear IP Atacante (Firewall)
Si detectas un ataque desde una IP, bloquéala remotamente.

*   **Config (Server):**
    ```yaml
    "ban-ip":
      command: "iptables -A INPUT -s {{.Params.target}} -j DROP"
    ```
*   **Cliente:**
    ```bash
    ghostknock -host MISERVIDOR -action ban-ip -args "target=192.168.50.5" -server-pubkey RUTA_A_SERVER.PUB
    ```

### 5. Despliegue Rápido (Git Pull)
Actualiza el código de una aplicación web para una rama concreta.

*   **Config (Server):**
    ```yaml
    "deploy-app":
      run_as_user: "www-data"
      command: "cd /var/www/html && git fetch && git checkout {{.Params.branch}} && git pull"
    ```
*   **Cliente:**
    ```bash
    ghostknock -host MISERVIDOR -action deploy-app -args "branch=main" -server-pubkey RUTA_A_SERVER.PUB
    ```

### 6. Gestión de Contenedores Docker
Reinicia un contenedor Docker específico.

*   **Config (Server):**
    ```yaml
    "docker-bounce":
      command: "docker restart {{.Params.container}}"
    ```
*   **Cliente:**
    ```bash
    ghostknock -host MISERVIDOR -action docker-bounce -args "container=api-gateway" -server-pubkey RUTA_A_SERVER.PUB
    ```

### 7. Modo "Pánico" (Lockdown)
Cierra todo el tráfico entrante nuevo en caso de emergencia de seguridad.

*   **Config (Server):**
    ```yaml
    "lockdown":
      command: "iptables -P INPUT DROP"
    ```
*   **Cliente:**
    ```bash
    ghostknock -host MISERVIDOR -action lockdown -server-pubkey RUTA_A_SERVER.PUB
    ```

### 8. Wake-on-LAN Proxy
Enciende una máquina de la red interna.

*   **Config (Server):**
    ```yaml
    "wol-pc":
      command: "wakeonlan {{.Params.mac}}"
    ```
*   **Cliente:**
    ```bash
    ghostknock -host MISERVIDOR -action wol-pc -args "mac=aa-bb-cc-dd-ee-ff" -server-pubkey RUTA_A_SERVER.PUB
    ```

### 9. Actualización del Sistema
Lanza una actualización de paquetes del sistema operativo.

*   **Config (Server):**
    ```yaml
    "sys-update":
      command: "apt-get update && apt-get upgrade -y"
      timeout_seconds: 600
      cooldown_seconds: 3600
    ```
*   **Cliente:**
    ```bash
    ghostknock -host MISERVIDOR -action sys-update -server-pubkey RUTA_A_SERVER.PUB
    ```

### 10. Creación de Usuario (Con Privacidad)
Crea un usuario en el sistema pasando la contraseña. Gracias a `sensitive_params`, la contraseña no aparecerá en los logs del sistema.

*   **Config (Server):**
    ```yaml
    "create-user":
      command: "useradd -m -p $(openssl passwd -1 {{.Params.password}}) {{.Params.username}}"
      sensitive_params:
        - "password"
    ```
*   **Cliente:**
    ```bash
    ghostknock -host MISERVIDOR -action create-user -args "username=invitado,password=Secreto.123" -server-pubkey RUTA_A_SERVER.PUB
    ```
*   **Resultado Log:** `command="[REDACTADO] useradd ... (Valores ocultos por sensitive_params)"` y `params=map[password:***** username:invitado]`

---

## ⚙️ Referencia de Configuración Completa (`config.yaml`)

Aquí se detallan todas las opciones disponibles para configurar el demonio.

| Sección | Campo | Tipo | Obligatorio | Descripción |
| :--- | :--- | :--- | :---: | :--- |
| *(Raíz)* | `server_private_key_path` | string | ✅ | Ruta al archivo de clave privada `ed25519` del servidor, usado para descifrar los payloads. |
| **`listener`** | `interface` | string | ✅ | Interfaz de red para escuchar (ej: `eth0`, `any`). |
| | `port` | int | ✅ | Puerto UDP a escuchar (ej: `3001`). |
| | `listen_ip` | string | ❌ | (Opcional) Si se define, escucha solo en esta IP específica. Por defecto: `""` (Todas). |
| **`logging`** | `log_level` | string | ✅ | Nivel de log: `debug`, `info`, `warn`, `error`. |
| **`daemon`** | `pid_file` | string | ❌ | Ruta al archivo PID (ej: `/var/run/ghostknockd.pid`). |
| **`security`** | *(opcional)* | | | |
| | `replay_window_seconds` | int | ❌ | Ventana de tiempo (segundos) para aceptar un knock. Aumentar para tolerar desfase horario, pero incrementa riesgo de replay. Por defecto: `5`. |
| | `default_action_cooldown_seconds` | int | ❌ | Cooldown global (segundos) para acciones sin `cooldown_seconds` propio. Por defecto: `15`. |
| | `rate_limit_per_second` | float | ❌ | (Avanzado) Paquetes por segundo permitidos por IP para Anti-DoS. Por defecto: `1.0`. |
| | `rate_limit_burst` | int | ❌ | (Avanzado) Ráfaga de paquetes permitida por IP para Anti-DoS. Por defecto: `3`. |
| **`users`** | `name` | string | ✅ | Identificador del usuario para los logs. |
| | `public_key` | string | ✅ | Clave pública `ed25519` en formato Base64. |
| | `actions` | list | ✅ | Lista de IDs de acciones que este usuario puede ejecutar. |
| | `source_ips` | list | ❌ | Lista de IPs/CIDRs permitidos (ej: `["192.168.1.50/32"]`). Si está vacío, permite todas. |
| **`actions`** | *(key)* | string | ✅ | El ID de la acción (debe coincidir con `users.actions`). |
| | `command` | string | ✅ | Comando de shell a ejecutar. Soporta variables `{{.Params.x}}` y `{{.SourceIP}}`. |
| | `run_as_user` | string | ❌ | Usuario del sistema que ejecuta el comando. Por defecto: `root` (si el demonio es root). |
| | `timeout_seconds` | int | ❌ | Tiempo máximo de ejecución. Si se excede, el comando se mata (SIGKILL). |
| | `cooldown_seconds` | int | ❌ | Tiempo de espera antes de permitir ejecutar esta acción de nuevo. `0` sin cooldown, `-1` usa el global. |
| | `revert_command` | string | ❌ | Comando que se ejecuta automáticamente tras el retraso. |
| | `revert_delay_seconds`| int | ❌ | Segundos a esperar antes de ejecutar `revert_command`. |
| | `sensitive_params` | list | ❌ | Lista de nombres de parámetros que deben ser ocultados (`*****`) en los logs del sistema. |

---

## 📄 Licencia

Este proyecto se distribuye bajo la **Licencia MIT**.
