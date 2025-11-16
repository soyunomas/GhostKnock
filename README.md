# 👻 GhostKnock

[![Licencia: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**GhostKnock** es un **ejecutor de acciones remotas** que se activa mediante un único paquete UDP criptográficamente firmado. Aunque inspirado en el *port knocking*, su propósito es mucho más amplio: permite ejecutar de forma segura y discreta cualquier comando preconfigurado en un servidor, haciéndolo invisible a los escaneos de red.

En lugar de secuencias de paquetes fáciles de detectar, GhostKnock utiliza criptografía de clave pública (`ed25519`) para validar cada solicitud. Esto lo convierte en una herramienta ideal para administradores de sistemas que necesitan un mecanismo de control de emergencia o de automatización que no exponga puertos ni servicios adicionales.

### Casos de Uso Típicos

GhostKnock no es solo para abrir puertos. Es una herramienta flexible para control remoto seguro:

*   **Gestión de Acceso:** Abrir/cerrar temporalmente el acceso a servicios críticos (SSH, VPN, base de datos) solo para tu IP.
*   **Control de Servicios:** Reiniciar un servicio que no responde (servidor web, aplicación, base de datos) sin necesidad de iniciar sesión.
*   **Tareas de Emergencia:** Reiniciar o apagar de forma segura un servidor que se ha vuelto inaccesible por otros medios.
*   **Automatización y Mantenimiento:** Disparar scripts de backup, limpiar cachés, o ejecutar tareas de mantenimiento programadas desde un sistema de CI/CD o un cron job.
*   **Integración con Firewalls:** Modificar dinámicamente reglas de `iptables` o `nftables`.

---

## ✨ Características Principales

*   🔐 **Seguridad Criptográfica:** Cada "knock" es un payload firmado con `ed25519`. El servidor verifica la autenticidad con la clave pública del usuario.
*   🕵️ **Bajo Perfil (Stealth):** Escucha pasivamente el tráfico de red con `pcap` sin abrir ningún puerto, haciéndolo **invisible** a escaneos de red.
*   🧩 **Configuración Declarativa:** Un único archivo `config.yaml` define usuarios, claves públicas, IPs permitidas y acciones de forma clara y legible.
*   ⚙️ **Acciones Flexibles:** Ejecuta cualquier comando del sistema, con plantillas seguras (`text/template`), acciones de reversión automáticas y timeouts.
*   🛡️ **Defensa Robusta:** Protección anti-replay con ventanas de tiempo, rate limiting por IP y cooldowns configurables por acción.
*   📜 **Logging Estructurado:** Registra todas las actividades en `/var/log/ghostknockd.log` en un formato clave-valor, ideal para auditoría y `fail2ban`.
*   📦 **Empaquetado Nativo:** Se integra como un servicio `systemd` y se distribuye como un paquete `.deb` para una instalación y gestión sencillas.

---

## 🚀 Instalación

### Opción 1: Paquete .deb (La Vía Fácil para Debian/Ubuntu/Mint)

Descarga el último paquete `.deb` desde la [página de Releases de GitHub](https://github.com/soyunomas/GhostKnock/releases/latest).

```bash
# Reemplaza la URL con el enlace directo al .deb de la última versión
wget https://github.com/soyunomas/GhostKnock/releases/download/v1.0.0/ghostknock_1.0.0_amd64.deb

# Instala el paquete. dpkg gestionará la copia de archivos y la configuración del servicio.
sudo dpkg -i ghostknock_1.0.0_amd64.deb

# Si dpkg informa de dependencias faltantes (como libpcap), este comando lo solucionará.
sudo apt-get -f install
```

### Opción 2: Desde el Código Fuente (Compilación Manual)

#### Prerrequisitos
*   Go 1.21+
*   Librería `libpcap`
    *   Debian/Ubuntu: `sudo apt-get update && sudo apt-get install -y libpcap-dev build-essential`
    *   RHEL/CentOS/Fedora: `sudo yum install -y libpcap-devel`

#### Compilación e Instalación
```bash
# 1. Clonar el repositorio
git clone https://github.com/soyunomas/GhostKnock.git
cd GhostKnock

# 2. Compilar e instalar binarios, configuración y servicio systemd.
sudo make install
```

---

## 🛠️ Configuración y Uso

### 1. Generar Claves de Cliente

En tu máquina local (la que enviará los knocks), utiliza la herramienta `ghostknock-keygen` para crear un par de claves.

#### Uso Estándar

Para generar un par de claves en la ubicación por defecto, simplemente ejecuta el comando sin argumentos:
```bash
# Genera ~/.config/ghostknock/id_ed25519 (privada) y .pub (pública)
ghostknock-keygen
```
El cliente `ghostknock` buscará automáticamente la clave en esta ubicación.

#### Ubicación Personalizada

Si necesitas gestionar múltiples identidades o guardar la clave en una ruta específica (por ejemplo, para integrarla con otros sistemas), utiliza el flag `-o`:
```bash
# Genera un par de claves llamado 'id_staging' en el directorio actual
ghostknock-keygen -o ./id_staging
```
Cuando envíes un knock con esta clave, deberás especificar su ruta con el flag `-key`:
`ghostknock -host ... -action ... -key ./id_staging`

---

En cualquier caso, después de ejecutar el comando, copia la **clave pública** en formato Base64 que se muestra en la terminal. La necesitarás para configurar el usuario en el archivo `config.yaml` del servidor.

### 2. Configurar el Servidor

1.  **Crea el archivo de configuración:** El paquete `.deb` o `make install` ya ha instalado una plantilla.
    ```bash
    sudo cp /etc/ghostknock/config.yaml.example /etc/ghostknock/config.yaml
    ```
2.  **Edita la configuración:**
    ```bash
    sudo nano /etc/ghostknock/config.yaml
    ```
    Como mínimo, debes:
    *   Ajustar la `interface` de red.
    *   Pegar la **clave pública** del cliente en la sección `users`.
    *   Definir las `actions` que ese usuario puede ejecutar.

### 3. Iniciar el Servicio

Si instalaste el `.deb` o usaste `sudo make install`, el servicio ya está configurado.

```bash
# Inicia el servicio
sudo systemctl start ghostknockd

# (Opcional) Verifica que está corriendo correctamente
sudo systemctl status ghostknockd

# (Opcional) Mira los logs en tiempo real
sudo journalctl -u ghostknockd -f
```

### 4. Enviar un Knock

Desde tu máquina cliente, con la clave privada en `~/.config/ghostknock/id_ed25519`:

```bash
# El cliente buscará la clave por defecto.
ghostknock -host IP_DEL_SERVIDOR -action open-ssh-port
```

---

## 📄 Parámetros de `config.yaml`

| Sección | Parámetro | Descripción | Valor por Defecto / Ejemplo |
| :--- | :--- | :--- | :--- |
| **`listener`** | `interface` | Interfaz de red en la que escuchar. | `"any"` |
| | `port` | Puerto UDP en el que se esperan los knocks. | `3001` |
| | `listen_ip` | (Opcional) Escucha solo en una IP específica de la interfaz. | `""` (Cualquiera) |
| **`logging`** | `log_level` | Nivel de verbosidad: "debug", "info", "warn", "error". | `"info"` |
| **`daemon`** | `pid_file` | (Opcional) Ruta para crear un archivo PID para systemd. | `"/var/run/ghostknockd.pid"` |
| **`users`** | `name` | Nombre descriptivo del usuario/cliente. | Requerido |
| | `public_key` | Clave pública del cliente en formato Base64. | Requerido |
| | `actions` | Lista de IDs de acciones que el usuario puede ejecutar. | Requerido |
| | `source_ips`| (Opcional) Restringe los knocks a IPs/CIDRs de origen. | `[]` (Cualquier IP) |
| **`actions`** | `command` | Comando a ejecutar. `{{.SourceIP}}` se sustituye por la IP del cliente. | Requerido |
| | `revert_command`| (Opcional) Comando que se ejecuta para revertir la acción principal. | `""` (Sin reversión) |
| | `revert_delay_seconds`| Segundos a esperar antes de ejecutar `revert_command`. | `0` |
| | `timeout_seconds`| (Opcional) Segundos máximos de ejecución del comando antes de terminarlo. | `0` (Sin timeout) |
| | `cooldown_seconds`| (Opcional) Segundos que deben pasar antes de que la misma acción se pueda repetir. | `-1` (Usa el cooldown global) |
| | `run_as_user`| (Opcional) Ejecuta el comando como un usuario sin privilegios. Prohibido "root". | `""` (root) |

---

## 💡 Ejemplos Prácticos de Configuración

Aquí tienes una configuración `actions` con casos de uso comunes para un administrador de sistemas.

```yaml
# /etc/ghostknock/config.yaml

# ... (secciones listener, logging, daemon, users) ...

actions:
  # ==========================================================
  # EJEMPLO 1: Abrir temporalmente el puerto SSH a tu IP
  # ==========================================================
  "open-ssh-port":
    command: "iptables -I INPUT 1 -p tcp -s {{.SourceIP}} --dport 22 -j ACCEPT"
    revert_command: "iptables -D INPUT -p tcp -s {{.SourceIP}} --dport 22 -j ACCEPT"
    revert_delay_seconds: 300 # El puerto se cierra automáticamente tras 5 minutos.
    cooldown_seconds: 60     # No se puede ejecutar más de una vez por minuto.

  # ==========================================================
  # EJEMPLO 2: Reiniciar el servidor web Nginx
  # ==========================================================
  "restart-nginx":
    command: "systemctl restart nginx"
    timeout_seconds: 20      # Si tarda más de 20s, se cancela.
    cooldown_seconds: 120    # Esperar 2 minutos antes de permitir otro reinicio.

  # ==========================================================
  # EJEMPLO 3: Disparar un script de backup personalizado
  # ==========================================================
  "trigger-backup":
    command: "/usr/local/scripts/backup_databases.sh"
    timeout_seconds: 900     # Permitir que el backup dure hasta 15 minutos.
    run_as_user: "backup"    # Ejecutar con un usuario de sistema con privilegios mínimos.

  # ==========================================================
  # EJEMPLO 4: Limpiar la caché de una aplicación web
  # ==========================================================
  "clear-app-cache":
    command: "rm -rf /var/www/my-app/cache/*"
    # Ejecutar como el usuario del servidor web previene errores de permisos
    # y limita el daño potencial si el comando es incorrecto.
    run_as_user: "www-data"
    timeout_seconds: 10
    
  # ==========================================================
  # EJEMPLO 5: Reiniciar el servidor (¡USAR CON PRECAUCIÓN!)
  # ==========================================================
  "reboot-server":
    # Un pequeño retardo asegura que la respuesta UDP se envíe antes del reinicio.
    command: "sleep 2 && reboot"
    cooldown_seconds: 3600 # No permitir reinicios accidentales seguidos.

  # ==========================================================
  # EJEMPLO 6: Actualizar todos los paquetes del sistema (apt)
  # ==========================================================
  "system-update":
    command: "apt-get update && apt-get upgrade -y"
    # Una actualización puede tardar mucho. Un timeout generoso de 15 minutos
    # previene que el proceso se quede colgado indefinidamente.
    timeout_seconds: 900
    # Esta es una operación intensiva. Un cooldown de 1 hora (3600s) previene
    # que se ejecute repetidamente por accidente o de forma maliciosa.
    cooldown_seconds: 3600
```

---

## 📄 Licencia

Este proyecto está bajo la **Licencia MIT**. Consulta el archivo `LICENSE` para más información.
