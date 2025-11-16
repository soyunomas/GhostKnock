# 👻 GhostKnock

**GhostKnock** es una reimplementación moderna del concepto de *port knocking*, diseñada para ser **segura**, **flexible** y **discreta**.
A diferencia de técnicas tradicionales basadas en secuencias de paquetes TCP/UDP —fáciles de detectar o falsificar— GhostKnock utiliza **criptografía de clave pública (`ed25519`)** para autenticar y autorizar la ejecución de acciones remotas mediante **un único paquete UDP**.

El sistema está escrito en **Go**, generando binarios autocontenidos y sin dependencias externas, perfectos para despliegues en sistemas Linux.

---

## ✨ Características Principales

*   🔐 **Seguridad Criptográfica:** Cada knock es un payload firmado digitalmente. El servidor verifica su autenticidad e integridad con la clave pública del usuario.

*   🛡 **Defensa Anti-Replay:** Protección en dos capas con ventana de tiempo estricta y cooldown por acción para evitar ejecuciones repetidas.

*   🧩 **Configuración Declarativa:** Un solo archivo `config.yaml` define usuarios, claves públicas y acciones permitidas de forma clara y legible.

*   🕵️ **Bajo Perfil:** Escucha pasiva mediante `pcap` sin abrir puertos → **invisible para escaneos de red**.

*   ⚙️ **Acciones Flexibles:** Ejecuta cualquier comando del sistema, con plantillas seguras (`text/template`) y capacidades de rollback automático.

*   📜 **Logging Estructurado:** Registra todas las actividades en un formato de texto estructurado (clave=valor), ideal para auditoría y análisis con herramientas como `grep`, `awk` o `fail2ban`.

*   🛑 **Cierre Controlado (Graceful Shutdown):** Responde a las señales del sistema (`SIGINT`, `SIGTERM`) para un apagado limpio, asegurando que todos los recursos se liberen correctamente.

---

## 🛠 Instalación y Uso

### 1. Prerrequisitos

*   Go 1.21+
*   libpcap (librería de captura de paquetes)
    *   Debian/Ubuntu: `sudo apt-get update && sudo apt-get install -y libpcap-dev`
    *   RHEL/CentOS/Fedora: `sudo yum install -y libpcap-devel`

### 2. Compilación e Instalación (La Vía Rápida)

Gracias al `Makefile` incluido, el proceso es simple y sigue las convenciones de Linux.

```bash
# 1. Clonar el repositorio
git clone https://github.com/soyunomas/GhostKnock.git
cd GhostKnock

# 2. Compilar e instalar los binarios en /usr/local/bin
#    y el archivo de configuración de ejemplo en /etc/ghostknock/
sudo make install
```
Este comando hará que `ghostknockd`, `ghostknock`, y `ghostknock-keygen` estén disponibles en todo el sistema.

### 3. Configuración del Servidor

#### a) Generar Claves para un Cliente

En tu máquina local (o en el cliente que enviará los knocks), genera un par de claves.

```bash
# Genera un par de claves: id_ed25519 (privada) y id_ed25519.pub (pública)
ghostknock-keygen
```
Copia el contenido de la clave pública (la cadena larga en Base64) que se muestra en la terminal. La necesitarás para el siguiente paso. Guarda el archivo `id_ed25519` en un lugar seguro en tu máquina cliente.

#### b) Crear el Archivo de Configuración en el Servidor

El `Makefile` instaló una plantilla de configuración. Cópiala y edítala.

```bash
# Copia la plantilla a la configuración activa
sudo cp /etc/ghostknock/config.yaml.example /etc/ghostknock/config.yaml

# Edita el archivo con tu editor preferido
sudo nano /etc/ghostknock/config.yaml
```

Dentro del archivo, como mínimo, debes:
1.  Ajustar la `interface` de red en la que escuchará el servidor.
2.  Pegar la clave pública generada en el paso anterior en la sección `users`.
3.  Definir las `actions` que ese usuario puede ejecutar.

### 4. Ejecución

**En el Servidor:**

```bash
# Inicia el demonio. Usamos -config para ser explícitos.
sudo ghostknockd -config /etc/ghostknock/config.yaml
```

**En la Máquina Cliente:**

Asegúrate de tener el binario `ghostknock` y el archivo de clave privada `id_ed25519` en el mismo directorio.

```bash
# Envía un knock para ejecutar la acción "open-ssh-port"
./ghostknock -host IP_DEL_SERVIDOR -action open-ssh-port
```

---

## 📄 `config.yaml` Explicado y Ejemplos

Este es el corazón de GhostKnock. A continuación se muestra un ejemplo completo y comentado con casos de uso prácticos para un administrador de sistemas.

```yaml
# ==============================================================================
# Archivo de Configuración de GhostKnockd (/etc/ghostknock/config.yaml)
# ==============================================================================

listener:
  # Interfaz de red en la que escuchar ("any" para todas).
  interface: "eth0"
  # Puerto UDP en el que se esperan los "knocks".
  port: 3001
  # (OPCIONAL) Escucha solo en una IP específica de la interfaz.
  # listen_ip: "192.168.1.100"

users:
  - name: "sysadmin_laptop"
    public_key: "PEGA_AQUI_TU_CLAVE_PUBLICA"
    actions:
      - "open-ssh-port"
      - "start-ssh-service"
      - "stop-ssh-service"
      - "reboot-server"
      - "trigger-backup"

  - name: "monitoring_script"
    public_key: "OTRA_CLAVE_PUBLICA_PARA_AUTOMATIZACION"
    actions:
      - "clear-redis-cache"

actions:
  # ==========================================================
  # EJEMPLO 1: Abrir temporalmente el puerto SSH a tu IP
  # ==========================================================
  "open-ssh-port":
    command: "iptables -I INPUT 1 -p tcp -s {{.SourceIP}} --dport 22 -j ACCEPT"
    revert_command: "iptables -D INPUT -p tcp -s {{.SourceIP}} --dport 22 -j ACCEPT"
    revert_delay_seconds: 300 # El puerto se cierra automáticamente tras 5 minutos.

  # ==========================================================
  # EJEMPLO 2: Iniciar/Detener un servicio (ej. SSHD)
  # ==========================================================
  "start-ssh-service":
    command: "systemctl start sshd"
    revert_command: ""
    revert_delay_seconds: 0

  "stop-ssh-service":
    command: "systemctl stop sshd"
    revert_command: ""
    revert_delay_seconds: 0
    
  # ==========================================================
  # EJEMPLO 3: Apagar o reiniciar el servidor (¡USAR CON PRECAUCIÓN!)
  # ==========================================================
  "reboot-server":
    # Añadimos un pequeño retardo para que el cliente UDP no reciba un error de red.
    command: "sleep 2 && reboot"
    revert_command: ""
    revert_delay_seconds: 0

  "shutdown-server":
    command: "sleep 2 && shutdown -h now"
    revert_command: ""
    revert_delay_seconds: 0

  # ==========================================================
  # EJEMPLO 4: Disparar un script de backup personalizado
  # ==========================================================
  "trigger-backup":
    command: "/usr/local/scripts/backup_database.sh"
    revert_command: ""
    revert_delay_seconds: 0

  # ==========================================================
  # EJEMPLO 5: Limpiar la caché de una aplicación (ej. Redis)
  # ==========================================================
  "clear-redis-cache":
    command: "redis-cli FLUSHALL"
    revert_command: ""
    revert_delay_seconds: 0
```

---

## 🗺 Hoja de Ruta del Proyecto

### ✅ Fase I: Estado Inicial

*   [x] Configuración en `config.yaml`
*   [x] Soporte para múltiples claves públicas
*   [x] Validación de firmas

### ✅ Fase II: Interacción Segura

*   [x] Captura de IP de origen
*   [x] Paquete `executor`
*   [x] Plantillas seguras + acciones de reversión
*   [x] Integración completa en servidor

### ✅ Fase III: Defensa Activa

*   [x] Sistema anti-replay avanzado
*   [x] Rate limiting por IP
*   [x] Logging estructurado (a archivo `/var/log/ghostknockd.log`)
*   [x] Graceful shutdown (cierre controlado)

### 🟡 Fase IV: Usabilidad Avanzada — **EN PROGRESO**

*   [x] Makefile para automatizar compilación e instalación.
*   [ ] Configuración del cliente mejorada (buscar claves en `~/.config/ghostknock/`).
*   [ ] **Implementar opciones de configuración avanzadas para robustez y seguridad:**
    *   [x] **A nivel de Servidor:**
        *   `log_level`: Para poder ajustar la verbosidad de los logs (debug, info, warn) desde la configuración, sin necesidad de recompilar.
        *   `pid_file`: Para generar un archivo PID, facilitando la integración con scripts de monitorización y gestión de servicios (`systemd`, `monit`, etc.).
    *   [ ] **A nivel de Acción:**
        *   `timeout_seconds`: Para terminar automáticamente comandos que se cuelgan, previniendo procesos zombie y liberando recursos del sistema.
        *   `cooldown_seconds` (por acción): Para definir un enfriamiento específico por acción, permitiendo políticas de seguridad más granulares para operaciones críticas.
        *   `run_as_user`: Para ejecutar comandos con privilegios reducidos, aplicando el principio de mínimo privilegio y reduciendo drásticamente la superficie de ataque.
    *   [ ] **A nivel de Usuario:**
        *   `source_ips`: Para restringir desde qué direcciones IP puede operar un usuario, añadiendo una capa de seguridad crucial que ata una clave criptográfica a una ubicación de red.
*   [ ] Empaquetado (Systemd service, .deb/.rpm).

---

## 📄 Licencia

Este proyecto está bajo la **Licencia MIT**. Consulta el archivo `LICENSE` para más información.
