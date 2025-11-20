# 👻 GhostKnock

[![Licencia: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/badge/release-v1.1.0-blue.svg)](https://github.com/soyunomas/GhostKnock/releases)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20windows-lightgrey.svg)]()

**GhostKnock** es un sistema de **ejecución remota segura e invisible**.

Permite disparar comandos predefinidos en un servidor enviando un único paquete UDP. A diferencia del "port knocking" tradicional, GhostKnock no depende de secuencias secretas de puertos, sino de **criptografía de clave pública (Ed25519)**.

El servidor escucha pasivamente el tráfico de red. Si recibe un paquete con una firma válida, ejecuta la acción asociada. Si la firma es inválida, el paquete es ignorado silenciosamente, haciendo que el servidor sea **indetectable** a escaneos de puertos.

---

## ✨ Características

*   🔐 **Criptografía Fuerte:** Autenticación mediante firmas `Ed25519`. Sin contraseñas ni secretos compartidos.
*   🧩 **Parámetros Dinámicos:** El cliente puede enviar argumentos (ej. IPs, nombres de servicio) que se inyectan de forma segura en los comandos del servidor.
*   🛡️ **Seguridad Ofensiva/Defensiva:**
    *   **Invisible:** No abre puertos TCP.
    *   **Anti-Replay:** Protección contra ataques de repetición mediante timestamp.
    *   **Sanitización Estricta:** Los parámetros entrantes pasan por una lista blanca (`Allowlist`) para prevenir inyección de comandos.
    *   **Anti-DoS:** Verificación criptográfica previa al procesamiento de datos.
*   ⚡ **Multiplataforma:** Cliente nativo para **Linux** y **Windows**.
*   ⚙️ **Automatización:** Ideal para tareas de CI/CD, recuperación de desastres y gestión de accesos de emergencia.

---

## 📦 Instalación

### Opción A: Paquetes .deb (Debian/Ubuntu/Mint)

Descarga la última versión desde [Releases](https://github.com/soyunomas/GhostKnock/releases).

*   **Para el Servidor (Demonio + Herramientas):**
    ```bash
    sudo dpkg -i ghostknock_1.1.0_amd64.deb
    # Se instala el servicio systemd y se asegura el directorio /etc/ghostknock
    ```

*   **Para Clientes Remotos (Solo Herramientas):**
    ```bash
    sudo dpkg -i ghostknock-client_1.1.0_amd64.deb
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

## 🚀 Guía de Inicio Rápido

### 1. Generar tus Llaves (En tu PC Cliente)
Necesitas un par de claves. La **privada** se queda en tu PC, la **pública** va al servidor.

```bash
# Linux / Mac
ghostknock-keygen
# Salida: Clave pública guardada en ~/.config/ghostknock/id_ed25519.pub

# Windows (PowerShell)
.\ghostknock-keygen.exe
```
> **Copia la cadena Base64 que aparece en la terminal.** Esa es tu clave pública.

### 2. Configurar el Servidor
Edita el archivo `/etc/ghostknock/config.yaml`:

```yaml
users:
  - name: "admin_remoto"
    public_key: "PEGA_TU_CLAVE_PUBLICA_AQUI_..."
    actions:
      - "write-test"
      - "open-ssh"

actions:
  "write-test":
    command: 'echo "Test OK. P1={{.Params.p1}} P2={{.Params.p2}}" > /tmp/prueba.txt'
    cooldown_seconds: 0
```

### 3. Iniciar el Servicio
```bash
sudo systemctl restart ghostknockd
```

### 4. Enviar tu primer Knock
```bash
# Linux
ghostknock -host IP_DEL_SERVIDOR -action write-test -args "p1=Hola,p2=Mundo"

# Windows
.\ghostknock.exe -host IP_DEL_SERVIDOR -action write-test -args "p1=Hola,p2=Mundo"
```

---

## 💡 Recetario: 10 Ejemplos Prácticos

A continuación se presentan configuraciones para `config.yaml` y el comando del cliente correspondiente.

> ⚠️ **Nota de Seguridad sobre Parámetros:**
> Los argumentos pasados con `-args` solo permiten: **Letras (a-Z), Números (0-9), Puntos (.), Guiones bajos (_) y Guiones medios (-)**.
> Cualquier otro carácter (espacios, :, /, ;) provocará el rechazo del paquete.

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
    ghostknock -host 127.0.0.1 -action write-test -args "p1=ValorUno,p2=Valor_Dos"
    ```
*   **Resultado:** `cat /tmp/prueba.txt` mostrará el contenido.

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
    ghostknock -host MISERVIDOR -action open-ssh
    ```

### 3. Reiniciar Servicios Específicos
Reinicia un servicio pasando su nombre como parámetro. Útil para servidores web o bases de datos.

*   **Config (Server):**
    ```yaml
    "restart-svc":
      command: "systemctl restart {{.Params.name}}"
      timeout_seconds: 10
    ```
*   **Cliente:**
    ```bash
    ghostknock -host MISERVIDOR -action restart-svc -args "name=nginx"
    ```

### 4. Banear IP Atacante (Firewall)
Si detectas un ataque desde una IP, bloquéala remotamente sin necesidad de entrar por SSH.

*   **Config (Server):**
    ```yaml
    "ban-ip":
      command: "iptables -A INPUT -s {{.Params.target}} -j DROP"
    ```
*   **Cliente:**
    ```bash
    ghostknock -host MISERVIDOR -action ban-ip -args "target=192.168.50.5"
    ```

### 5. Despliegue Rápido (Git Pull)
Actualiza el código de una aplicación web para una rama concreta.

*   **Config (Server):**
    ```yaml
    "deploy-app":
      # Ejecutamos como www-data por seguridad
      run_as_user: "www-data"
      command: "cd /var/www/html && git fetch && git checkout {{.Params.branch}} && git pull"
    ```
*   **Cliente:**
    ```bash
    ghostknock -host MISERVIDOR -action deploy-app -args "branch=main"
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
    ghostknock -host MISERVIDOR -action docker-bounce -args "container=api-gateway"
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
    ghostknock -host MISERVIDOR -action lockdown
    ```

### 8. Mantenimiento y Limpieza
Ejecuta scripts de mantenimiento preexistentes en el servidor.

*   **Config (Server):**
    ```yaml
    "cleanup":
      command: "/opt/scripts/rotate_logs.sh {{.Params.mode}}"
    ```
*   **Cliente:**
    ```bash
    ghostknock -host MISERVIDOR -action cleanup -args "mode=full"
    ```

### 9. Wake-on-LAN Proxy
Enciende una máquina de la red interna.
*Nota: Usamos guiones en la MAC porque los dos puntos (:) no están permitidos en los parámetros.*

*   **Config (Server):**
    ```yaml
    "wol-pc":
      command: "wakeonlan {{.Params.mac}}"
    ```
*   **Cliente:**
    ```bash
    ghostknock -host MISERVIDOR -action wol-pc -args "mac=aa-bb-cc-dd-ee-ff"
    ```

### 10. Actualización del Sistema
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
    ghostknock -host MISERVIDOR -action sys-update
    ```

---

## ⚙️ Referencia de Configuración Completa (`config.yaml`)

Aquí se detallan todas las opciones disponibles para configurar el demonio.

| Sección | Campo | Tipo | Obligatorio | Descripción |
| :--- | :--- | :--- | :---: | :--- |
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

---

## 📄 Licencia

Este proyecto se distribuye bajo la **Licencia MIT**.
