# Changelog

Todos los cambios notables en este proyecto serán documentados en este archivo.

## [Unreleased]

## [2.0.0]

### Security (Hardening & Architecture)
- **Protección Anti-Replay de Doble Verificación (Double-Check Locking):** Se ha implementado una estrategia híbrida de "Lectura Rápida / Escritura Atómica". El sistema verifica la caché (barato) antes de la criptografía (caro) para mitigar inundaciones de CPU, y vuelve a verificar bajo bloqueo estricto antes de guardar para evitar condiciones de carrera (Race Conditions). Esto reemplaza la lógica anterior, ofreciendo resistencia tanto a agotamiento de CPU como a inconsistencias de memoria.
- **Defensa contra Agotamiento de Memoria (Anti-OOM):** Implementación de límites estrictos (`hard-caps`) en las tablas de rastreo de IPs (`ipLimiters`). Se ha añadido una lógica de **"Purga Parcial Aleatoria"** que elimina el 10% de las entradas más antiguas cuando la tabla se llena, garantizando que el servidor nunca colapse por falta de RAM ante un ataque de *IP Spoofing* masivo.
- **Prevención de "Fork Bomb" (Límite de Procesos):** Se introduce un semáforo de ejecución (`executionSem`) que limita estrictamente el número de comandos concurrentes (default: 10). Si el servidor recibe más peticiones válidas de las que puede procesar, las rechaza inmediatamente ("Fail-Fast") en lugar de encolarlas, protegiendo la tabla de procesos del sistema operativo.
- **Arquitectura de Listener "Fail-Fast" (Anti-Bufferbloat):** El canal de recepción de paquetes se ha reducido y se ha vuelto **No-Bloqueante**. Bajo saturación extrema, el listener descarta paquetes silenciosamente en lugar de bloquearse o acumular latencia, priorizando la frescura de los datos (`timestamps`) y la supervivencia del servicio.
- **Cifrado de Extremo a Extremo (Confidencialidad):** Se ha implementado un cifrado de clave pública (X25519, `nacl/box`) obligatorio para todo el payload. Ahora, la acción y los parámetros enviados son indescifrables para cualquier observador en la red.
- **Integridad de Datos en Apagado (Graceful Shutdown):** Se han añadido `WaitGroups` para monitorizar los subprocesos de ejecución. El demonio ahora espera a que terminen los scripts críticos (ej. actualizaciones, backups) antes de cerrarse al recibir `SIGTERM`, evitando la corrupción de datos o instalaciones a medias.
- **Optimización de Captura (Modo No-Promiscuo):** El servidor ahora procesa estrictamente los paquetes destinados a su propia interfaz de red (MAC/IP), ignorando el tráfico ajeno (ruido de broadcast/multicast). Esto **mantiene la invisibilidad total** del servicio (Stealth) y su compatibilidad con firewalls, pero reduce drásticamente el consumo de CPU al no procesar tráfico irrelevante.
- **Privacidad de Logs (Redacción de Secretos):** Se introduce la directiva `sensitive_params`. Los parámetros marcados serán sustituidos por `*****` en los registros, evitando que secretos queden expuestos en disco.
- **Optimización de Heap (Zero-Allocation):** Se ha optimizado el acceso a los mapas de caché utilizando conversiones directas de punteros (`string(bytes)` en lookup), evitando que el Garbage Collector se sature limpiando miles de strings temporales durante un ataque.

### Added
- **Identidad Propia del Servidor:** El demonio `ghostknockd` ahora requiere su propio par de claves Ed25519. Clave privada especificada en `config.yaml` (`server_private_key_path`).
- **Validación de Configuración Avanzada:** Flag `-t` para validar exhaustivamente el archivo de configuración.
- **Transparencia de Versión:** Flag `-version` en todos los binarios.
- **Visibilidad Operativa:** Sistema de métricas internas ("Heartbeat") que reporta estadísticas de paquetes procesados y descartados cada 10 segundos, permitiendo detectar ataques sin saturar los logs de disco.

### Changed
- **BREAKING CHANGE: Protocolo de Red v2:** Incompatible con v1 debido al cifrado obligatorio.
- **BREAKING CHANGE: Nuevo Flag de Cliente:** `ghostknock` ahora requiere `-server-pubkey`.
- **BREAKING CHANGE: Configuración:** `config.yaml` requiere `server_private_key_path`.
- **Timeout Forzoso:** Se aplica un timeout de seguridad por defecto (30s) a cualquier acción que no tenga uno definido explícitamente.

### Fixed
- **Bug Lógico en Limpieza de Cooldowns:** Se corrigió un error donde el recolector de basura eliminaba registros de cooldowns largos (>1h) antes de tiempo, permitiendo la ejecución prematura de acciones restringidas. Se ha ampliado la ventana de retención a 24h.
- **Herencia de Cooldown:** Corregido error donde omitir `cooldown_seconds` desactivaba el enfriamiento.

## [1.1.0]

### Added
- **Parámetros Dinámicos:** Flag `-args "key=val"`.
- **Soporte Windows:** Binarios `.exe`.
- **Empaquetado:** Generación de `.deb`.

### Security
- **Authenticate-then-Parse:** Verificación de firmas antes de JSON unmarshal.
- **Sanitización:** Lista blanca de caracteres en argumentos.
- **Endurecimiento del Sistema de Archivos:** Permisos restrictivos (`0700`) en `/etc/ghostknock` para paquetes .deb.

### Fixed
- Se corrigió una condición de carrera potencial en el mapa de cooldowns.

## [1.0.0]

- Lanzamiento inicial del proyecto GhostKnock.
