# GhostKnock

GhostKnock es una reimplementación moderna del concepto de "port knocking", diseñada para ser segura, flexible y de bajo perfil. En lugar de depender de secuencias de paquetes TCP/UDP predefinidas (que pueden ser fácilmente detectadas y falseadas), GhostKnock utiliza criptografía de clave pública (`ed25519`) para autenticar y autorizar la ejecución de acciones remotas a través de un único paquete UDP.

El sistema está escrito en Go y se compila en binarios autocontenidos sin dependencias externas, facilitando su despliegue en cualquier sistema Linux.

## Características Principales

*   **Seguridad Criptográfica:** Cada "knock" es un payload firmado digitalmente. El servidor verifica la autenticidad y la integridad del mensaje usando la clave pública del usuario, haciendo imposible la falsificación o modificación de solicitudes.
*   **Defensa Anti-Replay:** Incorpora una defensa de dos capas contra ataques de repetición:
    1.  **Ventana de Tiempo Estricta:** Los knocks deben ser recibidos en unos pocos segundos desde su creación.
    2.  **Cooldown de Acciones:** Evita que la misma acción sea ejecutada repetidamente en un corto período de tiempo.
*   **Configuración Declarativa:** Toda la configuración del servidor, incluyendo usuarios, claves públicas y acciones permitidas, se gestiona a través de un único archivo `config.yaml`.
*   **Bajo Perfil:** El servidor escucha pasivamente en la red (`pcap`) sin abrir ningún puerto, lo que lo hace invisible a los escaneos de red.
*   **Acciones Flexibles:** Ejecuta cualquier comando de shell definido en la configuración, utilizando plantillas seguras para insertar la IP de origen del knock y permitiendo programar acciones de reversión automáticas.

## Cómo Funciona

El ecosistema de GhostKnock se compone de tres herramientas:

1.  **`ghostknock-keygen`**: Una utilidad para generar pares de claves criptográficas `ed25519` (una clave privada `id_ed25519` y una pública `id_ed25519.pub`).
2.  **`ghostknockd`**: El demonio del servidor. Se ejecuta en la máquina que quieres proteger, cargando las claves públicas y las acciones permitidas desde `config.yaml`.
3.  **`ghostknock`**: El cliente CLI. Utiliza la clave privada para firmar una solicitud de acción y la envía como un paquete UDP al servidor.

## Instalación y Uso

A continuación se muestra un ejemplo completo para configurar y probar GhostKnock localmente.

### 1. Prerrequisitos

*   **Go** (versión 1.18 o superior).
*   **libpcap**: Librería para la captura de paquetes.
    *   En Debian/Ubuntu: `sudo apt-get update && sudo apt-get install libpcap-dev`
    *   En Red Hat/CentOS: `sudo yum install libpcap-devel`

### 2. Compilación

Clona este repositorio y compila los binarios:

```bash
go build -o ghostknockd ./cmd/ghostknockd/
go build -o ghostknock ./cmd/ghostknock/
go build -o ghostknock-keygen ./cmd/ghostknock-keygen/
```

### 3. Configuración

1.  **Genera un par de claves con un nombre descriptivo:**
    Usa el flag `-o` para especificar la ruta base de los archivos de claves. El programa no sobrescribirá claves existentes por seguridad.

    ```bash
    # Genera las claves para 'mi_portatil'
    ./ghostknock-keygen -o mi_portatil_key
    ```
    Esto creará dos archivos: `mi_portatil_key` (la clave privada) y `mi_portatil_key.pub` (la clave pública). Copia la clave pública en formato Base64 que se muestra en la terminal.

2.  **Prepara la clave privada para el cliente:**
    El cliente `ghostknock` busca por defecto una clave llamada `id_ed25519`. Para esta prueba, renombraremos nuestra clave privada recién creada.

    ```bash
    # Renombramos la clave privada para que el cliente la encuentre
    mv mi_portatil_key id_ed25519
    ```

3.  **Crea el archivo `config.yaml`:**
    Crea un archivo `config.yaml` en la raíz del proyecto con el siguiente contenido. Pega tu clave pública donde se indica.

    ```yaml
    # config.yaml
    listener:
      # Para pruebas locales, 'lo' (Linux) o 'lo0' (macOS) es ideal.
      # En producción, usa la interfaz de red pública (ej. 'eth0').
      interface: "lo"
      port: 3001

    users:
      - name: "mi_portatil"
        # Pega aquí la clave pública Base64 generada en el paso 1
        public_key: "PEGA_AQUI_TU_CLAVE_PUBLICA_BASE64"
        actions:
          - "create-test-file"

    actions:
      "create-test-file":
        # Comando para crear un archivo en /tmp con la IP y la fecha.
        # Las comillas dobles son importantes para que el shell expanda $(date).
        command: "echo \"Knock válido de {{.SourceIP}} recibido a las $(date)\" > /tmp/ghostknock_success.txt"
        # Comando para limpiar el archivo de prueba después de un tiempo.
        revert_command: "rm /tmp/ghostknock_success.txt"
        # La limpieza se ejecutará 15 segundos después del knock.
        revert_delay_seconds: 15
    ```

### 4. Ejecución

Necesitarás dos terminales en el directorio del proyecto.

**Terminal 1: Iniciar el Servidor**

El servidor necesita permisos para capturar paquetes de red.

```bash
sudo ./ghostknockd
# Salida esperada:
# ...
# El listener está activo. Procesando knocks recibidos...
```

**Terminal 2: Enviar un Knock**

```bash
./ghostknock -host 127.0.0.1 -action create-test-file
# Salida esperada:
# Preparando knock para la acción 'create-test-file' en 127.0.0.1:3001...
# ✅ ¡Éxito! Knock enviado (XXX bytes).
```

### 5. Verificación

1.  **Revisa el log del servidor (Terminal 1).** Deberías ver un mensaje de `✅ [ÉXITO]`.
2.  **Verifica que la acción se ha ejecutado.**
    ```bash
    cat /tmp/ghostknock_success.txt
    # Debería mostrar: Knock válido de 127.0.0.1 recibido a las [fecha y hora]
    ```
3.  **Espera 15 segundos** y verifica que la acción de reversión ha eliminado el archivo.
    ```bash
    ls /tmp/ghostknock_success.txt
    # Debería mostrar un error "No existe el archivo o el directorio"
    ```
## Hoja de Ruta del Proyecto

### Fase I: Estado Inicial - COMPLETADA
- [x] Implementar un sistema de configuración basado en `config.yaml`.
- [x] Cargar múltiples usuarios y claves públicas.
- [x] Validar firmas de knocks entrantes.

### Fase II: Interacción Segura con el Sistema - COMPLETADA
- [x] **Tarea 1: Capturar la IP de Origen.**
    - [x] Modificar el listener para enriquecer los datos del paquete con la IP de origen.
- [x] **Tarea 2: Definir Configuración de Acciones y Crear el Paquete `executor`.**
    - [x] Añadir sección global de `actions` a `config.yaml`.
    - [x] Crear el paquete `internal/executor` para la ejecución segura de comandos.
    - [x] Usar `text/template` para prevenir inyección de comandos.
    - [x] Implementar acciones de reversión en goroutines separadas.
- [x] **Tarea 3: Integrar el Executor en el Servidor.**
    - [x] Conectar el flujo de validación con la ejecución de la acción.

### Fase III: Robustecimiento y Defensa Activa - EN PROGRESO
- [x] **Tarea: Caché Anti-Replay Real.**
    - [x] Implementar una defensa de dos capas: ventana de tiempo estricta y cooldown de acciones por usuario para prevenir todos los tipos de ataques de repetición.
- [ ] **Tarea: Rate Limiting.**
    - [ ] Implementar un limitador de velocidad por IP de origen para mitigar ataques de fuerza bruta o DoS de bajo volumen.
- [ ] **Tarea: Logging Estructurado.**
    - [ ] Migrar del `log` estándar a una librería como `zerolog` para producir logs en formato JSON.
- [ ] **Tarea: Graceful Shutdown.**
    - [ ] Implementar el manejo de señales del sistema (`SIGINT`, `SIGTERM`) para permitir que el servidor termine de forma limpia.

### Fase IV: Usabilidad y Flujos de Trabajo Avanzados - PENDIENTE
- [ ] **Tarea: Creación de un `Makefile`.**
    - [ ] Automatizar tareas comunes como compilación, tests y limpieza.
- [ ] **Tarea: Mejoras en el Cliente `ghostknock`.**
    - [ ] Añadir la capacidad de leer la configuración desde un archivo para mejorar la experiencia de usuario.

## Licencia

Este proyecto está bajo la Licencia MIT. Consulta el archivo `LICENSE` para más detalles.
```
