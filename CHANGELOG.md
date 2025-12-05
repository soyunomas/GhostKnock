# Changelog

Todos los cambios notables en este proyecto serán documentados en este archivo.

## [2.1.0]

### Added
- **Sistema de Hooks (Event Driven):** Nueva capacidad para ejecutar scripts externos en puntos clave del ciclo de vida (`pre_execute`, `on_success`, `on_error`, `on_revert`). Permite integraciones avanzadas (notificaciones Telegram/Slack, logs a SIEM) y validaciones personalizadas. El contexto se inyecta mediante variables de entorno (`GK_USER`, `GK_IP`, `GK_STAGE`, `GK_STATUS`).
- **Configuración de Rendimiento (Tuning):** Nueva sección `tuning` en `config.yaml`. Permite ajustar finamente los buffers de red (`packet_channel_buffer`), el timeout de captura (`pcap_timeout_ms`) y los límites de memoria para el rastreo de IPs (`max_tracked_ips`), permitiendo escalar desde IoT hasta servidores Enterprise.
- **Logs Estructurados y Flexibles:** Soporte nativo para formato JSON (`log_format: json`) facilitando la ingestión en sistemas SIEM (ELK, Datadog). Además, ahora es posible redirigir los logs a `stdout` (para contenedores/Docker) o `/dev/null` mediante `log_file`.
- **Personalización del Shell:** Nueva opción en la sección `daemon` para definir el intérprete de comandos (`shell_path`) y sus flags. Permite compatibilidad con sistemas minimalistas (ej. Alpine usando `/bin/ash`) o entornos restringidos.
- **Perfiles de Cliente (Client Profiles):** Se ha eliminado la necesidad de escribir argumentos largos y repetitivos. El cliente `ghostknock` ahora soporta un archivo de configuración `profiles.yaml` (en `~/.config/ghostknock/` o `%APPDATA%\ghostknock\`) para definir hosts, puertos y claves.
- **Nuevo Flag `-profile`:** Permite cargar una configuración predefinida por nombre (ej: `ghostknock -profile prod -action restart`). Los flags manuales tienen prioridad sobre el perfil.
- **Hot Reload (Configuración en Caliente):** Se ha implementado soporte para la señal `SIGHUP`. Ahora es posible recargar la configuración (usuarios, claves, acciones, lista negra, hooks, logging) sin detener el servicio enviando `systemctl reload ghostknockd`.
- **Optimización de Defensa (Blacklist):** Nueva directiva `deny_ips` en `security`. Permite definir una lista de IPs o rangos CIDR que serán descartados instantáneamente ("Short-Circuit"). Esto ocurre antes de cualquier operación criptográfica.
- **Empaquetado (Logrotate):** Se ha añadido configuración automática de `logrotate` en el paquete `.deb`. El log `/var/log/ghostknockd.log` se rota diariamente y se retiene 14 días.
- **Ejemplos de Configuración:** Se incluye `profiles.yaml.example` en la distribución.

### Changed
- **Arquitectura Configurable:** Se han eliminado las constantes de rendimiento "hardcoded" del código fuente. El motor ahora adapta su consumo de recursos dinámicamente según la configuración cargada.
- **Dependencias del Cliente:** El binario `ghostknock` ahora integra `gopkg.in/yaml.v3` para la gestión de perfiles.

## [2.0.0]

### Security (Hardening & Architecture)
- **Protección Anti-Replay Híbrida (Check-Verify-Lock):** Se ha implementado una estrategia de doble verificación. El sistema consulta la caché de lectura rápida (`RLock`) antes de procesar, realiza la validación criptográfica y finalmente adquiere un bloqueo de escritura.
- **Defensa contra Agotamiento de Memoria (Anti-OOM):** Implementación de límites estrictos (`hard-caps`) en las tablas de rastreo de IPs (`ipLimiters`) con purga automática.
- **Prevención de "Fork Bomb" (Límite de Procesos):** Se introduce un semáforo de ejecución (`executionSem`) que limita estrictamente el número de comandos concurrentes (default: 10).
- **Arquitectura de Listener "Fail-Fast" (Anti-Bufferbloat):** El canal de recepción de paquetes se ha reducido y se ha vuelto **No-Bloqueante**.
- **Cifrado de Extremo a Extremo (Confidencialidad):** Se ha implementado cifrado obligatorio para todo el payload utilizando derivación de claves `X25519`.
- **Integridad de Datos en Apagado (Graceful Shutdown):** Se han añadido `WaitGroups` para monitorizar los subprocesos de ejecución.
- **Optimización de Captura (Modo No-Promiscuo):** El servidor ahora procesa estrictamente los paquetes destinados a su propia interfaz de red (MAC/IP).
- **Sanitización de Logs de Entrada:** Se introduce la directiva `sensitive_params`.
- **Optimización de Heap (Zero-Allocation):** Se ha optimizado el acceso a los mapas de caché.

### Added
- **Identidad Propia del Servidor:** El demonio `ghostknockd` ahora requiere su propio par de claves Ed25519 especificado en `server_private_key_path`.
- **Validación de Configuración Avanzada:** Flag `-t` para validar exhaustivamente el archivo de configuración.
- **Transparencia de Versión:** Flag `-version` en todos los binarios.
- **Visibilidad Operativa:** Sistema de métricas internas ("Heartbeat").

### Changed
- **BREAKING CHANGE: Protocolo de Red v2:** Incompatible con v1 debido al cifrado obligatorio.
- **BREAKING CHANGE: Nuevo Flag de Cliente:** `ghostknock` ahora requiere `-server-pubkey`.
- **BREAKING CHANGE: Configuración:** `config.yaml` requiere `server_private_key_path`.
- **Timeout Forzoso:** Se aplica un timeout de seguridad por defecto (30s).

### Fixed
- **Bug Lógico en Limpieza de Cooldowns:** Se corrigió un error donde el recolector de basura eliminaba registros de cooldowns largos antes de tiempo.
- **Herencia de Cooldown:** Corregido error donde omitir `cooldown_seconds` desactivaba el enfriamiento.

## [1.1.0]

### Added
- **Parámetros Dinámicos:** Flag `-args "key=val"`.
- **Soporte Windows:** Binarios `.exe`.
- **Empaquetado:** Generación de `.deb`.

### Security
- **Authenticate-then-Parse:** Verificación de firmas antes de JSON unmarshal.
- **Sanitización:** Lista blanca de caracteres en argumentos.

## [1.0.0]

- Lanzamiento inicial.
