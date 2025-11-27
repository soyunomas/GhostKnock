# Changelog

Todos los cambios notables en este proyecto serán documentados en este archivo.

## [Unreleased]

## [2.0.0]

### Security
- **Protección Anti-Replay Atómica (Fix TOCTOU):** Se ha rediseñado la lógica de la caché de firmas implementando una estrategia de "Reserva Pesimista". La firma del paquete se registra y bloquea atómicamente **antes** de realizar operaciones criptográficas o liberar el mutex. Esto cierra una vulnerabilidad crítica de condición de carrera que permitía ataques de repetición mediante inundación simultánea (Race Condition) y mitiga ataques de agotamiento de CPU.
- **Cifrado de Extremo a Extremo (Confidencialidad):** Se ha implementado un cifrado de clave pública (X25519, `nacl/box`) obligatorio para todo el payload. Ahora, la acción y los parámetros enviados son indescifrables para cualquier observador en la red, eliminando por completo las fugas de información y garantizando la confidencialidad. La autenticación se mantiene con la firma Ed25519 original.
- **Ejecución Asíncrona (Prevención de Bloqueo):** La ejecución de comandos ahora se realiza en Goroutines independientes ("Fire-and-Forget"). Esto evita que acciones de larga duración (ej. actualizaciones del sistema) bloqueen el bucle principal de recepción de paquetes, garantizando que el servidor siga escuchando nuevos knocks mientras procesa tareas en segundo plano.
- **Privacidad de Logs (Redacción de Secretos):** Se introduce la directiva `sensitive_params` en la configuración de acciones. Los parámetros marcados en esta lista serán sustituidos por `*****` en los registros del sistema y de depuración, evitando que secretos (como contraseñas o tokens) queden expuestos en texto plano en el disco.
- **Validación Estricta de Longitud de Paquete (Anti-DoS/Allocation):** Se ha establecido un límite estricto de 1KB para los payloads UDP en la capa de escucha (`listener`). Los paquetes que exceden este tamaño se descartan inmediatamente antes de pasar a la lógica de negocio, previniendo ataques de agotamiento de memoria.
- **Certificación de Robustez (Fuzzing):** Se ha integrado una suite de pruebas de *Fuzzing* para el listener de red y el deserializador del protocolo, validando la estabilidad ante entradas maliciosas.
- **Endurecimiento de Concurrencia en Cooldowns:** Se ha implementado un bloqueo estricto (`Mutex`) para la gestión de los tiempos de enfriamiento, eliminando vulnerabilidades de condición de carrera en la lógica de limitación de acciones.
- **Endurecimiento contra Inyección de Argumentos:** Se ha modificado la validación de parámetros (`-args`) para prohibir que los valores comiencen con un guion (`-`). Esto previene que un parámetro pueda ser interpretado como un flag por el comando subyacente (ej. `ls -R`), cerrando un vector de ataque de inyección de argumentos.

### Added
- **Identidad Propia del Servidor:** El demonio `ghostknockd` ahora requiere su propio par de claves Ed25519 para el descifrado. La clave privada se especifica en `config.yaml` a través de la nueva directiva `server_private_key_path`.
- **Validación de Configuración Avanzada:** El demonio `ghostknockd` ahora soporta el flag `-t` para realizar una validación exhaustiva del archivo de configuración sin necesidad de iniciar el servicio, reportando errores de sintaxis y lógica con número de línea.
- **Transparencia de Versión:** Todos los ejecutables ahora soportan el flag `-version`.

### Changed
- **BREAKING CHANGE: Protocolo de Red v2:** El protocolo de comunicación ha sido actualizado para ser incompatible con versiones anteriores debido a la adición del cifrado obligatorio.
- **BREAKING CHANGE: Nuevo Flag de Cliente Requerido:** El cliente `ghostknock` ahora requiere el flag `-server-pubkey` para especificar la clave pública del servidor.
- **BREAKING CHANGE: Configuración de Servidor Modificada:** El archivo `config.yaml` ahora requiere la directiva `server_private_key_path` en el nivel raíz.
- **Configuración de Seguridad Flexible:** Se ha movido la configuración de parámetros de seguridad clave (ventana anti-replay, cooldown por defecto) a una nueva sección opcional `security:` en `config.yaml`.

### Fixed
- **Herencia de Cooldown por Defecto (Zero-Value Trap):** Se corrigió un error lógico donde omitir `cooldown_seconds` desactivaba el enfriamiento en lugar de heredar el valor global.
- **Consistencia de Validación:** La herramienta de validación (`-t`) ahora marca estrictamente el campo `listener.interface` como obligatorio.

## [1.1.0]

### Added
- **Parámetros Dinámicos:** El cliente `ghostknock` ahora soporta el flag `-args "key=val"`.
- **Soporte Nativo para Windows:** Se añaden objetivos de compilación y binarios `.exe`.
- **Empaquetado Modular:** El `Makefile` genera paquetes `.deb` separados para servidor y cliente.

### Security
- **Arquitectura "Authenticate-then-Parse":** Refactorización para verificar firmas antes de deserializar JSON.
- **Sanitización Estricta de Parámetros:** Lista blanca de caracteres (`^[a-zA-Z0-9._-]+$`).
- **Endurecimiento del Sistema de Archivos:** Permisos restrictivos (`0700`) en `/etc/ghostknock` para paquetes .deb.

### Fixed
- Se corrigió una condición de carrera potencial en el mapa de cooldowns.

## [1.0.0]

- Lanzamiento inicial del proyecto GhostKnock.
