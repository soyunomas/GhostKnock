# 👻 GhostKnock

[![Licencia: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Release](https://img.shields.io/badge/release-v2.1.0-blue.svg)](https://github.com/soyunomas/GhostKnock/releases)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20windows-lightgrey.svg)]()

**GhostKnock** es un sistema de **ejecución remota segura, invisible y confidencial**.

Permite disparar comandos predefinidos en un servidor enviando un único paquete UDP cifrado.

El servidor escucha pasivamente el tráfico. Si recibe un paquete con una firma válida y un payload cifrado para él, lo descifra y ejecuta la acción asociada. Si no, el paquete es ignorado silenciosamente, haciendo que el servidor sea **indetectable** y su comunicación **indescifrable**.

---

## ✨ Características

*   🛡️ **Invisible por Diseño (Stealth):**
    *   **Sin Puertos Abiertos:** El servidor no mantiene puertos "a la escucha" en el sentido tradicional. No aparecerá en herramientas de monitorización (como `netstat`) ni responderá a intentos de conexión.
    *   **Indetectable externamente:** Ante un escaneo de red, el puerto parecerá estar **cerrado** o filtrado. El servidor captura los paquetes de forma pasiva, procesando silenciosamente solo aquellos que son legítimos y descartando el resto sin emitir respuesta.

*   🚀 **Monitorización Pasiva y Eficiente:**
    *   **Filtrado en Origen:** GhostKnock aplica filtros a nivel de sistema operativo (BPF). El kernel solo notifica a la aplicación cuando llega un paquete UDP al puerto exacto, garantizando un consumo de CPU prácticamente nulo incluso en redes con mucho tráfico.
    *   **Protección Short-Circuit (Nueva v2.1):** Capacidad de definir una lista negra (`deny_ips`) que descarta tráfico de atacantes conocidos *antes* de realizar cualquier operación criptográfica, ahorrando recursos de CPU.

*   🔧 **Tuning y Escalabilidad (Nuevo v2.1):**
    *   **Gestión de Recursos:** Sección `tuning` dedicada para ajustar buffers de red, timeouts de captura (`pcap`) y límites de memoria. Permite escalar desde dispositivos IoT (Raspberry Pi) hasta servidores Enterprise con alto tráfico.
    *   **Logging Flexible:** Soporte nativo para logs estructurados en **JSON** (para SIEMs como ELK/Datadog) y redirección a `stdout` o ficheros.

*   🔐 **Seguridad y Privacidad (Hardening):**
    *   **Cifrado de Extremo a Extremo:** Utiliza estándares modernos (`Ed25519` + `X25519`) para autenticación y confidencialidad. Solo el servidor puede leer qué comando estás enviando.
    *   **Anti-Replay Híbrido:** Sistema de doble verificación (lectura rápida + bloqueo de escritura) que detecta y rechaza paquetes duplicados para evitar la reutilización de credenciales.
    *   **Memoria Blindada (Anti-OOM):** Arquitectura diseñada para evitar el agotamiento de memoria ante ataques masivos, con purga automática de tablas de rastreo y límites estrictos configurables (`max_tracked_ips`).

*   🪝 **Sistema de Hooks (Event Driven):**
    *   **Integración y Auditoría:** Ejecuta scripts externos antes (`pre_execute`), después (`on_success`/`on_error`) o al revertir (`on_revert`) una acción.
    *   **Contexto Inyectado:** Los scripts reciben automáticamente variables de entorno con el usuario, IP, acción y parámetros (`GK_USER`, `GK_IP`, `GK_PARAM_*`). Ideal para notificaciones (Telegram, Slack) o logs centralizados.

*   👮 **Principio de Mínimo Privilegio:**
    *   Puedes configurar comandos para que se ejecuten como usuarios restringidos (ej. `www-data`), limitando el impacto en el sistema.
    *   **Shell Personalizable (Nuevo v2.1):** Posibilidad de definir el intérprete de comandos (ej. `/bin/ash`, `/bin/rbash`) para entornos minimalistas o restringidos.

*   🔄 **Gestión en Caliente (Hot Reload):**
    *   Permite añadir usuarios, rotar claves, modificar acciones y ajustar parámetros de logging editando la configuración y recargando el servicio (`systemctl reload`) **sin detener el servicio** y manteniendo intacta la caché de seguridad.

*   ⚡ **Perfiles de Cliente:**
    *   El cliente CLI soporta un archivo de configuración (`profiles.yaml`) para definir alias de conexión, evitando tener que escribir IPs y rutas de claves repetidamente.

---

## ⚠️ Comportamiento de Seguridad y Limitaciones

GhostKnock ha sido diseñado priorizando la **supervivencia del servidor** sobre la disponibilidad.

| Limitación | Escenario del Usuario | Comportamiento | Razón de Seguridad |
| :--- | :--- | :--- | :--- |
| **Límite de Procesos** | Se intentan lanzar más de 10 comandos simultáneos. | **RECHAZADO.** El servidor ignora el comando. | **Anti-Fork Bomb.** Protege la tabla de procesos del OS. |
| **Rate Limit (IP)** | Múltiples comandos en < 1 seg desde la misma IP. | **SILENCIO TOTAL.** Se ignoran las ráfagas. | **Anti-DoS.** Protege CPU y Memoria. |
| **Lista Negra (IP)** | IP presente en `deny_ips` envía tráfico. | **DESCARTE INMEDIATO.** | **Short-Circuit.** Ahorro total de CPU. |
| **Replay Cache** | Se reenvía un paquete idéntico. | **IGNORADO.** | **Anti-Replay.** Evita reutilización de credenciales. |
| **Timeout Forzoso** | Un script se cuelga indefinidamente. | **KILL.** Proceso eliminado (default 30s). | **Recuperación de Recursos.** |
| **Recarga (Reload)** | Se recarga configuración con `SIGHUP`. | **PERSISTE.** La caché de seguridad se mantiene. | **Continuidad.** No se pierden protecciones. |

---

## 📦 Instalación

### Opción A: Paquetes .deb (Debian/Ubuntu/Mint)

Descarga la última versión desde [Releases](https://github.com/soyunomas/GhostKnock/releases).

*   **Para el Servidor (Demonio + Herramientas):**
    ```bash
    sudo dpkg -i ghostknock_2.1.0_amd64.deb
    # Se instala el servicio systemd, logrotate y se asegura /etc/ghostknock
    ```

*   **Para Clientes Remotos (Solo Herramientas):**
    ```bash
    sudo dpkg -i ghostknock-client_2.1.0_amd64.deb
    ```

### Opción B: Ejecutables para Windows

Descarga `ghostknock.exe` y `ghostknock-keygen.exe` desde Releases. No requieren instalación.

### Opción C: Compilación Manual

```bash
make build          # Compila para Linux
make build-windows  # Compila .exe para Windows
