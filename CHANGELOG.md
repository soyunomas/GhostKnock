# Changelog

Todos los cambios notables en este proyecto serán documentados en este archivo.

## [Unreleased]

### Added
- **Hot Reload (Configuración en Caliente):** Se ha implementado soporte para la señal `SIGHUP`. Ahora es posible recargar la configuración (usuarios, claves, acciones, lista negra) sin detener el servicio enviando `systemctl reload ghostknockd`. La caché de seguridad se mantiene intacta.
- **Optimización de Defensa (Blacklist):** Nueva directiva `deny_ips` en `security`. Permite definir una lista de IPs o rangos CIDR que serán descartados instantáneamente ("Short-Circuit"). Esto ocurre antes de cualquier operación criptográfica o de rate-limit, reduciendo drásticamente el consumo de CPU ante ataques desde orígenes conocidos.
- **Empaquetado (Logrotate):** Se ha añadido configuración automática de `logrotate` en el paquete `.deb`. El log `/var/log/ghostknockd.log` se rota diariamente y se retiene 14 días.

## [2.0.0]

### Security (Hardening & Architecture)
- **Protección Anti-Replay Híbrida (Check-Verify-Lock):** Se ha implementado una estrategia de doble verificación. El sistema consulta la caché de lectura rápida (`RLock`) antes de procesar, realiza la validación criptográfica y finalmente adquiere un bloqueo de escritura para evitar condiciones de carrera. Esta arquitectura prioriza la **protección de la memoria RAM** (evitando el envenenamiento de caché con datos basura) sobre el ahorro de CPU, garantizando la estabilidad del servicio bajo carga.
- **Defensa contra Agotamiento de Memoria (Anti-OOM):** Implementación de límites estrictos (`hard-caps`) en las tablas de rastreo de IPs (`ipLimiters`). Se ha añadido una lógica de **"Purga Parcial Aleatoria"** que elimina el 10% de las entradas más antiguas cuando la tabla se llena, garantizando que el servidor nunca colapse por falta de RAM ante un ataque de *IP Spoofing* masivo.
- **Prevención de "Fork Bomb" (Límite de Procesos):** Se introduce un semáforo de ejecución (`executionSem`) que limita estrictamente el número de comandos concurrentes (default: 10). Si el servidor recibe más peticiones válidas de las que puede procesar, las rechaza inmediatamente ("Fail-Fast") en lugar de encolarlas, protegiendo la tabla de procesos del sistema operativo.
- **Arquitectura de Listener "Fail-Fast" (Anti-Bufferbloat):** El canal de recepción de paquetes se ha reducido y se ha vuelto **No-Bloqueante**. Bajo saturación extrema, el listener descarta paquetes silenciosamente en lugar de bloquearse o acumular latencia, priorizando la frescura de los datos (`timestamps`) y la supervivencia del servicio.
- **Cifrado de Extremo a Extremo (Confidencialidad):** Se ha implementado cifrado obligatorio para todo el payload utilizando derivación de claves `X25519` a partir de la identidad `Ed25519` (vía hashing SHA-512 y clamping). Ahora, la acción y los parámetros enviados son indescifrables para cualquier observador en la red.
- **Integridad de Datos en Apagado (Graceful Shutdown):** Se han añadido `WaitGroups` para monitorizar los subprocesos de ejecución. El demonio ahora espera a que terminen los scripts críticos (ej. actualizaciones, backups) antes de cerrarse al recibir `SIGTERM`, evitando la corrupción de datos.
- **Optimización de Captura (Modo No-Promiscuo):** El servidor ahora procesa estrictamente los paquetes destinados a su propia interfaz de red (MAC/IP), ignorando el tráfico ajeno. Esto mantiene la invisibilidad (Stealth) y reduce el consumo de CPU al no procesar ruido de broadcast/multicast.
- **Sanitización de Logs de Entrada:** Se introduce la directiva `sensitive_params`. Los parámetros de entrada marcados serán sustituidos por `*****` en los registros del sistema antes de ser procesados, reduciendo el riesgo de exposición de secretos en disco.
- **Optimización de Heap (Zero-Allocation):** Se ha optimizado el acceso a los mapas de caché utilizando conversiones directas de punteros en las búsquedas, evitando que el Garbage Collector se sature limpiando miles de strings temporales durante un ataque.

### Added
- **Identidad Propia del Servidor:** El demonio `ghostknockd` ahora requiere su propio par de claves Ed25519 especificado en `server_private_key_path`.
- **Validación de Configuración Avanzada:** Flag `-t` para validar exhaustivamente el archivo de configuración.
- **Transparencia de Versión:** Flag `-version` en todos los binarios.
- **Visibilidad Operativa:** Sistema de métricas internas ("Heartbeat") que reporta estadísticas de paquetes procesados y descartados cada 10 segundos, permitiendo detectar saturación sin inundar los logs de disco.

### Changed
- **BREAKING CHANGE: Protocolo de Red v2:** Incompatible con v1 debido al cifrado obligatorio.
- **BREAKING CHANGE: Nuevo Flag de Cliente:** `ghostknock` ahora requiere `-server-pubkey`.
- **BREAKING CHANGE: Configuración:** `config.yaml` requiere `server_private_key_path`.
- **Timeout Forzoso:** Se aplica un timeout de seguridad por defecto (30s) a cualquier acción que no tenga uno definido explícitamente.

### Fixed
- **Bug Lógico en Limpieza de Cooldowns:** Se corrigió un error donde el recolector de basura eliminaba registros de cooldowns largos (>1h) antes de tiempo. Se ha ampliado la ventana de retención a 24h.
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
