# Changelog

Todos los cambios notables en este proyecto serán documentados en este archivo.

## [Unreleased]

### Added
- **Transparencia de Versión:** Todos los ejecutables (`ghostknock`, `ghostknockd`, `ghostknock-keygen`) ahora soportan el flag `-version` para mostrar la versión de compilación actual.

### Changed
- **Configuración de Seguridad Flexible:** Se ha movido la configuración de parámetros de seguridad clave (como la ventana anti-replay y el cooldown por defecto) del código fuente a una nueva sección opcional `security:` en `config.yaml`. Esto permite a los administradores ajustar el balance entre seguridad y tolerancia (ej. desfases horarios) sin necesidad de recompilar.

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
