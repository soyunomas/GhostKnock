# Changelog

Todos los cambios notables en este proyecto serán documentados en este archivo.

## [Unreleased]

### Added
- **Validación de Configuración Avanzada:** El demonio `ghostknockd` ahora soporta el flag `-t` para realizar una validación exhaustiva del archivo de configuración sin necesidad de iniciar el servicio. Este sistema es capaz de detectar tanto errores de sintaxis (ej. tipos de datos incorrectos) como errores de lógica (ej. claves públicas en formato no válido) y reporta el problema exacto junto con el **número de línea** donde ocurrió, facilitando enormemente la depuración y previniendo caídas por configuraciones incorrectas.
- **Transparencia de Versión:** Todos los ejecutables (`ghostknock`, `ghostknockd`, `ghostknock-keygen`) ahora soportan el flag `-version` para mostrar la versión de compilación actual.

### Changed
- **Configuración de Seguridad Flexible:** Se ha movido la configuración de parámetros de seguridad clave (como la ventana anti-replay y el cooldown por defecto) del código fuente a una nueva sección opcional `security:` en `config.yaml`. Esto permite a los administradores ajustar el balance entre seguridad y tolerancia (ej. desfases horarios) sin necesidad de recompilar.

### Security
- **Mitigación de Ataques de Replay:** El demonio ahora mantiene una caché de firmas de paquetes válidos durante la ventana anti-replay. Cualquier paquete con una firma duplicada dentro de esta ventana es descartado, previniendo que un atacante pueda re-ejecutar un comando capturado múltiples veces.
- **Endurecimiento contra Inyección de Argumentos:** Se ha modificado la validación de parámetros (`-args`) para prohibir que los valores comiencen con un guion (`-`). Esto previene que un parámetro pueda ser interpretado como un flag por el comando subyacente (ej. `ls -R`), cerrando un vector de ataque de inyección de argumentos.

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

[Unreleased]: https://github.com/soyunomas/GhostKnock/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/soyunomas/GhostKnock/compare/v1.0.0...v1.1.0
