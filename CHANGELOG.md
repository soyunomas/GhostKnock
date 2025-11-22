# Changelog

Todos los cambios notables en este proyecto serán documentados en este archivo.

## [Unreleased]

## [2.0.0]

### Security
- **Cifrado de Extremo a Extremo (Confidencialidad):** Se ha implementado un cifrado de clave pública (X25519, `nacl/box`) obligatorio para todo el payload. Ahora, la acción y los parámetros enviados son indescifrables para cualquier observador en la red, eliminando por completo las fugas de información y garantizando la confidencialidad. La autenticación se mantiene con la firma Ed25519 original.
- **Privacidad de Logs (Redacción de Secretos):** Se introduce la directiva `sensitive_params` en la configuración de acciones. Los parámetros marcados en esta lista serán sustituidos por `*****` en los registros del sistema y de depuración, evitando que secretos (como contraseñas o tokens) queden expuestos en texto plano en el disco, mientras que el comando subyacente los recibe correctamente descifrados.
- **Mitigación Avanzada de Ataques de Replay:** El demonio mantiene una caché de firmas y verifica la duplicidad **antes** de realizar operaciones criptográficas costosas. Esto previene ataques de denegación de servicio (DoS) por agotamiento de CPU, además de evitar la re-ejecución lógica de comandos.
- **Endurecimiento de Concurrencia (Fix TOCTOU):** Se ha implementado un bloqueo estricto (`Mutex`) para la gestión de los tiempos de enfriamiento (cooldowns), eliminando vulnerabilidades de condición de carrera (Time-of-Check to Time-of-Use) que existían en versiones de desarrollo previas.
- **Endurecimiento contra Inyección de Argumentos:** Se ha modificado la validación de parámetros (`-args`) para prohibir que los valores comiencen con un guion (`-`). Esto previene que un parámetro pueda ser interpretado como un flag por el comando subyacente (ej. `ls -R`), cerrando un vector de ataque de inyección de argumentos.

### Added
- **Identidad Propia del Servidor:** El demonio `ghostknockd` ahora requiere su propio par de claves Ed25519 para el descifrado. La clave privada se especifica en `config.yaml` a través de la nueva directiva `server_private_key_path`.
- **Validación de Configuración Avanzada:** El demonio `ghostknockd` ahora soporta el flag `-t` para realizar una validación exhaustiva del archivo de configuración sin necesidad de iniciar el servicio. Este sistema es capaz de detectar tanto errores de sintaxis como de lógica (ej. claves en formato no válido) y reporta el problema exacto junto con el **número de línea** donde ocurrió.
- **Transparencia de Versión:** Todos los ejecutables (`ghostknock`, `ghostknockd`, `ghostknock-keygen`) ahora soportan el flag `-version` para mostrar la versión de compilación actual.

### Changed
- **BREAKING CHANGE: Protocolo de Red v2:** El protocolo de comunicación ha sido actualizado para ser incompatible con versiones anteriores debido a la adición del cifrado obligatorio.
- **BREAKING CHANGE: Nuevo Flag de Cliente Requerido:** El cliente `ghostknock` ahora requiere el flag `-server-pubkey` para especificar la clave pública del servidor, necesaria para cifrar la comunicación.
- **BREAKING CHANGE: Configuración de Servidor Modificada:** El archivo `config.yaml` ahora requiere la directiva `server_private_key_path` en el nivel raíz.
- **Configuración de Seguridad Flexible:** Se ha movido la configuración de parámetros de seguridad clave (como la ventana anti-replay y el cooldown por defecto) del código fuente a una nueva sección opcional `security:` en `config.yaml`.

### Fixed
- **Herencia de Cooldown por Defecto (Zero-Value Trap):** Se corrigió un error lógico donde omitir `cooldown_seconds` en una acción desactivaba el enfriamiento (0s) en lugar de heredar el valor global por defecto. Ahora, la ausencia del campo aplica correctamente la política de seguridad global.
- **Consistencia de Validación:** La herramienta de validación (`-t`) ahora marca estrictamente el campo `listener.interface` como obligatorio, alineando la validación estática con los requisitos reales de ejecución y evitando fallos en tiempo de ejecución.

## [1.1.0]

### Added
- **Parámetros Dinámicos:** El cliente `ghostknock` ahora soporta el flag `-args "key=val"` para enviar datos personalizados al servidor, permitiendo acciones mucho más flexibles.
- **Soporte Nativo para Windows:** Se añaden objetivos de compilación y se distribuyen binarios `.exe` para las herramientas cliente (`ghostknock.exe`, `ghostknock-keygen.exe`), permitiendo su uso en Windows sin WSL.
- **Empaquetado Modular:** El `Makefile` ahora puede generar dos paquetes `.deb` distintos: uno completo para el servidor (`ghostknock`) y otro ligero solo para las herramientas cliente (`ghostknock-client`).

### Security
- **Arquitectura "Authenticate-then-Parse":** Se refactorizó el núcleo del demonio para que la verificación criptográfica Ed25519 ocurra *antes* de cualquier intento de deserializar el payload. Esto mitiga ataques de DoS que podrían explotar payloads malformados.
- **Sanitización Estricta de Parámetros:** Todos los valores recibidos a través de `-args` son validados con una estricta lista blanca de caracteres (`^[a-zA-Z0-9._-]+$`), previniendo cualquier vector de inyección de comandos.
- **Endurecimiento del Sistema de Archivos:** Los paquetes `.deb` y el objetivo `make install` ahora aplican permisos restrictivos (`0700`) al directorio de configuración `/etc/ghostknock`, asegurando que solo `root` pueda acceder a él.

### Fixed
- Se corrigió una condición de carrera potencial en el acceso concurrente al mapa de cooldowns de acciones.

## [1.0.0]

- Lanzamiento inicial del proyecto GhostKnock.

[Unreleased]: https://github.com/soyunomas/GhostKnock/compare/v2.0.0...HEAD
[2.0.0]: https://github.com/soyunomas/GhostKnock/compare/v1.1.0...v2.0.0
[1.1.0]: https://github.com/soyunomas/GhostKnock/compare/v1.0.0...v1.1.0
