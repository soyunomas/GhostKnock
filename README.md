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
    *   **Filtrado en Origen:** A diferencia de un *sniffer* convencional que analiza todo el tráfico, GhostKnock aplica filtros a nivel de sistema operativo (BPF). El kernel solo notifica a la aplicación cuando llega un paquete UDP al puerto exacto, garantizando un consumo de CPU prácticamente nulo incluso en redes con mucho tráfico.

*   🔐 **Seguridad y Privacidad:**
    *   **Cifrado de Extremo a Extremo:** Utiliza estándares modernos (`Ed25519` + `X25519`) para garantizar dos cosas: que solo tú puedes enviar la orden (autenticación) y que nadie pueda leer qué comando o parámetros estás enviando (confidencialidad).
    *   **Protección Anti-Replay:** Implementa un mecanismo de "uso único". Si un paquete válido es interceptado y reenviado posteriormente, el servidor lo detectará como duplicado y lo rechazará automáticamente.

*   👮 **Principio de Mínimo Privilegio:**
    *   Aunque el proceso principal requiere permisos elevados para monitorear la red, tiene la capacidad de **degradar sus privilegios** automáticamente al ejecutar una acción. Puedes configurar comandos para que se ejecuten como usuarios restringidos (ej. `www-data`), limitando el impacto en el sistema.

*   🧩 **Flexibilidad Operativa:**
    *   **Parámetros Dinámicos:** Permite inyectar argumentos variables (como direcciones IP, nombres de usuarios o IDs de contenedores) dentro de los comandos del servidor de forma segura, gracias a una validación estricta de caracteres.

---

## ⚠️ Comportamiento de Seguridad y Limitaciones

GhostKnock ha sido diseñado priorizando la **supervivencia del servidor** sobre la disponibilidad. Si el sistema está saturado o detecta anomalías, rechazará peticiones para protegerse.

| Limitación | Escenario del Usuario | Comportamiento | Razón de Seguridad |
| :--- | :--- | :--- | :--- |
| **Límite de Procesos (Semáforo)** | Se intentan lanzar más de 10 comandos simultáneos (ej. múltiples backups o updates). | **RECHAZADO.** El servidor ignora el comando y registra un error. No se encola. | **Anti-Fork Bomb.** Evita el agotamiento de la tabla de procesos del sistema operativo. |
| **Rate Limit (IP)** | Se envían múltiples comandos en menos de 1 segundo desde la misma IP. | **SILENCIO TOTAL.** A partir del 3º paquete en ráfaga, se ignoran. | **Anti-DoS.** Protege CPU y Memoria contra inundaciones. |
| **Replay Cache** | Se reenvía un paquete idéntico (mismo nonce/firma) dentro de la ventana de tiempo. | **IGNORADO.** | **Anti-Replay.** Evita la reutilización de credenciales interceptadas. |
| **Buffer de Red** | Ataque DDoS masivo en curso. Usuario legítimo intenta conectar. | **POSIBLE DESCARTE.** Si el buffer de entrada se llena, se descartan los nuevos paquetes. | **Anti-Latencia.** Preferimos descartar a procesar paquetes viejos (bufferbloat). |
| **Desincronización Reloj** | El reloj del cliente difiere más de 5s del servidor. | **IGNORADO.** El timestamp es inválido. | **Anti-Replay.** Reduce la ventana de oportunidad de ataque. |
| **Payload Size** | Se intenta enviar un argumento gigante (>1KB). | **SILENCIO TOTAL.** | **Anti-Allocation DoS.** Previene agotamiento de RAM. |
| **Timeout Forzoso** | Un script ejecutado se cuelga indefinidamente. | **KILL.** El proceso es eliminado tras el tiempo límite (default 30s). | **Recuperación de Recursos.** Libera los slots de ejecución. |
| **Reinicio Seguro** | Se detiene el servicio (`stop`) mientras hay comandos corriendo. | **ESPERA.** El servicio espera a que terminen los hijos. | **Integridad de Datos.** Evita corromper operaciones críticas. |

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

## 🧱 Configuración del Firewall (Crucial para Invisibilidad)

Para que GhostKnock sea verdaderamente invisible, **el sistema operativo no debe responder** cuando reciba un paquete en el puerto UDP configurado.

Si no configuras el firewall, tu servidor Linux responderá con un mensaje ICMP "Port Unreachable", revelando a un atacante que el servidor existe y está activo.

Debes configurar tu firewall para **DESCARTAR (DROP/DENY)** explícitamente el tráfico en el puerto de escucha. **GhostKnock seguirá recibiendo los paquetes** porque los captura a bajo nivel (sniffing antes del firewall).

### Si usas UFW (Ubuntu/Debian por defecto)
Asumiendo que has configurado el puerto `3001` en `config.yaml`:

```bash
# Denegar explícitamente el tráfico UDP en el puerto 3001
sudo ufw deny 3001/udp
sudo ufw reload
```
> **Verificación:** Si escaneas el puerto con `nmap -sU -p 3001`, debería aparecer como `open|filtered` (lo ideal) o no responder en absoluto. Nunca debe aparecer como `closed` (que implica respuesta ICMP).

### Si usas iptables puro
```bash
# Insertar regla para DESCARTAR paquetes, evitando respuesta ICMP.
# GhostKnock (libpcap) verá el paquete antes de que iptables lo tire.
sudo iptables -I INPUT -p udp --dport 3001 -j DROP

# (Opcional) Guardar las reglas para que persistan tras reiniciar
# sudo netfilter-persistent save  # En Debian/Ubuntu
# sudo service iptables save      # En CentOS/RHEL
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

## 💡 Recetario: Ejemplos Prácticos

A continuación se presentan configuraciones para `config.yaml` y el comando del cliente correspondiente.

> ⚠️ **IMPORTANTE: Gestión de Timeouts y Corrupción de Datos**
> Por defecto, GhostKnock mata (`SIGKILL`) cualquier comando que tarde más de **30 segundos** para liberar recursos.
>
> Para tareas críticas como **actualizaciones de sistema (`apt`), backups o despliegues**, DEBES aumentar el valor de `timeout_seconds` explícitamente. Si el proceso se mata a la mitad, podrías dejar bloqueos de archivos (locks) huérfanos o bases de datos corruptas.
>
> **Regla de Oro:** Calcula el tiempo máximo que tarda tu comando en el peor escenario y multiplícalo por 2.

> ⚠️ **Nota de Seguridad sobre Parámetros:**
> Los argumentos pasados con `-args` solo permiten: **Letras (a-Z), Números (0-9), Puntos (.), Guiones bajos (_) y Guiones medios (-)**.

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
      # Aumentado a 20s para servicios lentos al arrancar
      timeout_seconds: 20
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
      # Aumentado a 300s (5 min) por si la red es lenta o hay post-hooks pesados
      timeout_seconds: 300
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
      # Docker puede tardar en detener un contenedor gracefuly
      timeout_seconds: 60
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
Lanza una actualización de paquetes del sistema operativo. **¡CUIDADO CON EL TIMEOUT!**

*   **Config (Server):**
    ```yaml
    "sys-update":
      # Usar siempre -y para evitar que el comando espere input
      command: "apt-get update && apt-get upgrade -y"
      # IMPRESCINDIBLE: 20 minutos. Si se corta antes, 'dpkg' quedará bloqueado.
      timeout_seconds: 1200
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
