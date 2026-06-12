Copia esto como `AGENTS.md` en la raíz del repo. Está escrito para que Codex planifique, implemente con cambios pequeños, evite regresiones y no convierta los fixes de seguridad en deuda nueva.

# AGENTS.md — GhostKnock Security Remediation Roadmap

## 0. Propósito de este archivo

Este archivo define cómo deben trabajar los agentes en el repositorio GhostKnock.

Objetivo principal:

* Mitigar vulnerabilidades confirmadas sin degradar la aplicación.
* Evitar introducir nuevos bugs por parches apresurados.
* Mantener compatibilidad razonable con configuraciones existentes.
* Favorecer cambios pequeños, testeados, reversibles y documentados.
* Priorizar seguridad real sobre refactors cosméticos.

GhostKnock es una herramienta sensible: procesa paquetes UDP, valida criptografía, autoriza acciones y ejecuta comandos del sistema. Cualquier cambio en autenticación, replay protection, hooks, templates, systemd, BPF o ejecución de comandos debe tratarse como cambio de seguridad crítico.

---

## 1. Reglas globales para todos los agentes

### 1.1 Modo Plan por Defecto

Entra en modo planificación para cualquier tarea no trivial.

Una tarea es no trivial si cumple cualquiera de estas condiciones:

* Requiere 3 o más pasos.
* Toca autenticación, criptografía, replay, timestamp, TOTP, BPF, hooks, executor, systemd, config, tests de seguridad o CI.
* Cambia comportamiento público.
* Puede romper compatibilidad con `config.yaml`.
* Requiere una decisión arquitectónica.
* Puede afectar privilegios, filesystem, red o procesos del sistema.

Para tareas no triviales:

1. Escribe el plan antes de editar código.
2. Enumera archivos afectados.
3. Define riesgos de regresión.
4. Define tests de verificación.
5. Define criterio de rollback.
6. No implementes hasta tener una estrategia clara.

Si algo se tuerce:

* Detente.
* No sigas acumulando cambios.
* Registra el problema.
* Replanifica.
* Reduce el cambio al mínimo reproducible.

Usa modo planificación también para verificación, no solo para construir.

---

### 1.2 Estrategia de subagentes

Usa subagentes para investigación, exploración y análisis en paralelo.

Reglas:

* Una tarea por subagente.
* Un subagente no debe mezclar dominios.
* El contexto principal debe mantenerse limpio.
* Todo subagente debe entregar un resumen breve, verificable y accionable.
* Los resultados deben guardarse en `tasks/research/` cuando sean relevantes.

Subagentes recomendados:

* `security-replay-agent`: timestamps, replay cache, TOTP, clock skew.
* `network-bpf-agent`: BPF, PCAP, interfaces, IPv4/IPv6, firewall.
* `executor-hooks-agent`: params, hooks, shell, templates, env vars.
* `systemd-hardening-agent`: service unit, capabilities, filesystem isolation.
* `tests-ci-agent`: tests, race detector, fuzzing, CI commands.
* `compat-docs-agent`: compatibilidad, documentación, ejemplos YAML.

El agente principal debe sintetizar, no copiar ciegamente.

---

### 1.3 Bucle de auto-mejora

Después de cualquier corrección del usuario, bug encontrado, fallo de test, regresión o mala suposición:

1. Actualiza `tasks/lessons.md`.
2. Escribe la regla que evita repetir el error.
3. Añade un ejemplo concreto.
4. Revisa `tasks/lessons.md` al inicio de cada sesión.
5. Aplica la lección en el plan siguiente.

Formato obligatorio para `tasks/lessons.md`:

```md
# Lessons Learned

## YYYY-MM-DD — Título breve

### Error o riesgo detectado
Descripción concreta.

### Regla nueva
Regla accionable.

### Ejemplo
Ejemplo práctico.

### Archivos relacionados
- path/file.go
```

---

### 1.4 Verificación antes de dar por terminado

Nunca marques una tarea como completada sin demostrar que funciona.

Antes de cerrar una tarea:

* Ejecuta tests relevantes.
* Ejecuta tests globales cuando el cambio toque lógica central.
* Revisa logs si aplica.
* Revisa comportamiento negativo, no solo happy path.
* Comprueba compatibilidad con configuración existente.
* Pregúntate: “¿Un ingeniero senior aprobaría este cambio?”

No basta con que compile.

---

### 1.5 Exigir elegancia con equilibrio

Para cambios no triviales, pregunta:

> ¿Hay una forma más elegante, más pequeña o más segura?

Si la solución se siente improvisada:

* Replanifica.
* Extrae función pequeña.
* Añade test.
* Reduce duplicación.
* Evita parches “pegados con cinta”.

Pero no sobreingenierizar:

* Para cambios simples y obvios, aplica el cambio directo.
* No introduzcas frameworks.
* No añadas dependencias sin justificación fuerte.
* No hagas refactors amplios dentro de fixes críticos.

---

### 1.6 Corrección autónoma de bugs

Ante un bug confirmado:

* Arréglalo directamente.
* No pidas al usuario que cambie de contexto.
* Revisa logs.
* Reproduce el fallo.
* Añade test si el bug es reproducible.
* Soluciona fallos de CI sin guía paso a paso.
* Documenta la causa raíz.

Si no puedes reproducirlo:

* Di exactamente qué intentaste.
* Añade hipótesis.
* Añade un plan de reproducción.
* No inventes confirmaciones.

---

### 1.7 Gestión de tareas

Antes de implementar, crea o actualiza:

* `tasks/todo.md`
* `tasks/lessons.md` si aplica
* `tasks/research/*.md` si hay análisis de subagentes

Formato recomendado para `tasks/todo.md`:

```md
# GhostKnock Security Remediation TODO

## Active Plan

### Goal
...

### Scope
...

### Non-goals
...

### Risks
...

### Verification
...

## Tasks

- [ ] TASK-001: ...
  - Files:
  - Risk:
  - Tests:
  - Status:
  - Notes:

## Completed

- [x] ...
```

Cada tarea debe incluir:

* objetivo;
* archivos afectados;
* riesgo de regresión;
* tests;
* criterio de aceptación.

---

## 2. Estado de seguridad verificado

No reintroducir falsos positivos ya descartados.

### 2.1 Hallazgos confirmados

#### GK-SEC-001 — Timestamp futuro permite replay diferido

Estado: confirmado.

El servidor valida si un paquete es demasiado antiguo, pero no rechaza correctamente timestamps en el futuro.

Riesgo:

* cliente autorizado malicioso;
* cliente comprometido;
* cliente con reloj adelantado;
* paquete válido capturado que ya contiene timestamp futuro.

No asumir que un MITM pasivo puede modificar timestamps. No puede hacerlo sin romper firma/cifrado.

Objetivo:

* rechazar timestamps demasiado antiguos;
* rechazar timestamps demasiado futuros;
* ajustar TTL de replay cache para cubrir toda la ventana aceptable.

---

#### GK-SEC-002 — Hooks reciben params antes de validación estricta

Estado: confirmado.

`Execute()` ejecuta hooks antes de que `runCommand()` valide los parámetros. `RunHook()` expone params como variables de entorno `GK_PARAM_*`.

Riesgo:

* no es RCE automática;
* es riesgo indirecto si el script hook usa variables sin comillas, con `eval`, interpolación shell insegura o comandos peligrosos;
* se agrava porque el daemon corre como root por defecto.

Objetivo:

* validar claves y valores de params antes de cualquier hook;
* no exponer params inválidos a hooks;
* mantener comportamiento claro y testeado.

---

#### GK-SEC-003 — Filtro BPF no anclado a destino

Estado: confirmado.

El filtro `udp and port <X>` captura tráfico donde el puerto aparece como origen o destino.

Objetivo:

* cambiar a `udp and dst port <X>`;
* si hay `listen_ip`, usar `dst host <IP> and udp and dst port <X>`;
* añadir tests de construcción del filtro;
* evaluar pruebas de integración cuando sea posible.

---

#### GK-SEC-004 — Servicio systemd corre como root sin sandboxing

Estado: confirmado.

Riesgo:

* amplifica cualquier bug;
* cambios de hardening pueden romper acciones legítimas si se aplican sin cuidado.

Objetivo:

* introducir hardening progresivo;
* evitar romper acciones existentes como `iptables`, `systemctl`, `docker`, escrituras en `/tmp` o `/var/www`;
* documentar perfiles.

---

#### GK-SEC-005 — Uso de `/bin/sh -c` como deuda estructural

Estado: confirmado.

El regex actual mitiga inyecciones triviales. No afirmar que `--flag`, espacios o `;` pasan: actualmente no pasan.

Riesgo real:

* la seguridad del comando principal depende de un regex global estricto;
* si se relaja para soportar rutas, URLs, espacios, MACs o flags, se reabre inyección;
* el formato string dificulta validación por argumento.

Objetivo de medio plazo:

* añadir soporte nuevo para `exec.argv`;
* mantener `command` como legacy explícito;
* no auto-dividir strings por espacios.

---

### 2.2 Falsos positivos descartados

No repetir como hallazgo confirmado:

* No hay goroutine leak por paquete en el flujo actual: hay canal limitado y worker pool.
* No hay data race confirmada en hot reload: existe `sync.RWMutex`.
* No hay argument injection trivial con valores que empiezan por `-`: el regex actual bloquea guion inicial.
* No hay replay cache TTL menor que la ventana normal actual, salvo al introducir aceptación de futuro si no se ajusta TTL.
* Un MITM pasivo no puede modificar timestamp ni payload sin romper criptografía.

---

## 3. Invariantes de seguridad que no deben romperse

Toda implementación debe preservar estos invariantes:

1. El servidor nunca responde a paquetes UDP.
2. El servidor no debe ejecutar hooks ni comandos si la autenticación criptográfica falla.
3. El servidor no debe ejecutar hooks ni comandos con params inválidos.
4. El servidor no debe ejecutar una acción si el paquete está fuera de ventana temporal.
5. La replay cache debe cubrir toda la ventana durante la cual un paquete podría ser aceptado.
6. Ningún cambio debe introducir goroutines no acotadas por paquete.
7. Ningún cambio debe relajar el regex de params sin rediseñar executor y tests.
8. No usar `strings.Fields`, `Split(" ")` ni parsing manual para convertir comandos shell a argv.
9. No introducir dependencias nuevas sin justificación en `tasks/todo.md`.
10. No romper `config.yaml` existente sin ruta de migración.
11. No aplicar hardening systemd destructivo como default sin documentar impacto.
12. No guardar secretos en logs.
13. No imprimir comandos renderizados con secretos.
14. No permitir params crudos en hooks por defecto.
15. No sacrificar tests por rapidez.

---

## 4. Reglas de desarrollo eficiente en Go

### 4.1 Simplicidad

Preferir:

* funciones pequeñas;
* tipos explícitos;
* errores claros;
* tests de tabla;
* interfaces solo si hay necesidad real;
* cambios locales antes que refactors amplios.

Evitar:

* frameworks;
* global state nuevo;
* magic constants sin nombre;
* dependencias innecesarias;
* soluciones genéricas no requeridas.

---

### 4.2 Manejo de tiempo

Para lógica de replay y timestamps:

* No uses `time.Since(ts)` si `ts` puede estar en el futuro.
* Usa comparaciones explícitas con `now`.
* Calcula ventana pasada y futura con nombres claros.
* La cache de replay debe expirar después de que el paquete ya no pueda ser aceptado.
* Para tests, permite inyectar `now` o aislar la función de validación temporal.

Recomendado:

```go
func validatePayloadTimestamp(now time.Time, ts time.Time, pastWindow, futureSkew time.Duration) error {
    if pastWindow <= 0 {
        return fmt.Errorf("past window must be positive")
    }
    if futureSkew < 0 {
        return fmt.Errorf("future skew cannot be negative")
    }
    if ts.Before(now.Add(-pastWindow)) {
        return ErrInvalidTimestamp
    }
    if ts.After(now.Add(futureSkew)) {
        return ErrInvalidTimestamp
    }
    return nil
}
```

---

### 4.3 Concurrencia

Reglas:

* Mantener worker pool acotado.
* No lanzar goroutine por paquete sin semáforo o pool.
* Mantener locks con scope mínimo.
* No hacer I/O lento bajo mutex.
* No mutar config compartida; preferir snapshot inmutable.
* Ejecutar `go test -race ./...` tras cambios en config, server, cache, workers o reload.

---

### 4.4 Errores

Errores deben:

* envolver contexto con `%w`;
* no filtrar secretos;
* diferenciar error interno de fallo de autenticación;
* no generar respuestas de red;
* no loguear ruido bajo ataques de basura.

---

### 4.5 Tests

Para cada bug de seguridad:

1. Escribe test que falle antes del fix.
2. Implementa fix mínimo.
3. Ejecuta test específico.
4. Ejecuta tests globales.
5. Añade test negativo.

Comandos habituales:

```bash
go test ./...
go test -race ./...
go vet ./...
make build
```

Si están disponibles:

```bash
govulncheck ./...
staticcheck ./...
gosec ./...
```

No fallar la tarea solo porque una herramienta opcional no esté instalada. Documentar si no está disponible.

---

### 4.6 Formato

Antes de cerrar:

```bash
gofmt -w <files>
go test ./...
```

No mezclar formateo masivo con cambios funcionales si dificulta revisión.

---

## 5. Roadmap de mitigación

El roadmap está dividido en fases. No saltar fases salvo instrucción explícita.

---

# FASE 0 — Baseline, tareas y seguridad del flujo

## Objetivo

Crear base de trabajo medible antes de tocar seguridad crítica.

## Tareas

### TASK-0001 — Crear estructura de tareas

Crear:

* `tasks/todo.md`
* `tasks/lessons.md`
* `tasks/research/`

Criterio de aceptación:

* existen los archivos;
* `tasks/todo.md` contiene plan activo;
* `tasks/lessons.md` existe aunque esté vacío.

---

### TASK-0002 — Ejecutar baseline

Ejecutar:

```bash
go test ./...
go test -race ./...
go vet ./...
make build
```

Si algún comando falla:

* no arreglar todo a la vez;
* registrar fallo en `tasks/todo.md`;
* clasificar si es preexistente o causado por cambios;
* replanificar.

Criterio de aceptación:

* baseline documentado;
* salida relevante resumida;
* fallos preexistentes identificados.

---

### TASK-0003 — Identificar fixtures de config

No usar `config.yaml` de ejemplo como fixture si contiene placeholders inválidos.

Crear fixtures en tests si hace falta:

* claves temporales generadas en test;
* config mínima válida;
* acciones dummy;
* hooks dummy seguros.

Criterio de aceptación:

* tests no dependen de claves reales;
* tests no escriben en rutas del sistema;
* tests no requieren root salvo tests de integración marcados.

---

# FASE 1 — Fix crítico: timestamp futuro y replay cache

## Objetivo

Corregir replay diferido por timestamps futuros sin introducir replay residual.

## Riesgo principal

Un parche incompleto que acepte `now + window` pero mantenga TTL `window + 1s` deja un hueco de replay.

## Diseño obligatorio

Implementar validación explícita:

* timestamp no debe ser anterior a `now - replayWindow`;
* timestamp no debe ser posterior a `now + futureSkew`;
* para compatibilidad inicial, `futureSkew` puede usar `replayWindow`;
* replay cache TTL debe cubrir `replayWindow + futureSkew + guard`.

No usar solo:

```go
if time.Since(ts) > replayWindow
```

## Opción recomendada de mínimo cambio

Sin añadir config nueva todavía:

```go
window := time.Duration(currentConfig.Security.ReplayWindowSeconds) * time.Second
futureSkew := window
guard := time.Second

now := time.Now()
ts := time.Unix(0, payload.Timestamp)

if ts.Before(now.Add(-window)) || ts.After(now.Add(futureSkew)) {
    slog.Warn("Paquete fuera de ventana temporal", "user", authorizedUser.Name)
    return
}

ttl := window + futureSkew + guard
expiration := now.Add(ttl)
```

Esto mantiene tolerancia simétrica y evita replay residual.

## Opción más precisa

Calcular expiración por paquete:

```go
latestAcceptable := ts.Add(window)
expiration := latestAcceptable.Add(time.Second)

minExpiration := now.Add(time.Second)
if expiration.Before(minExpiration) {
    expiration = minExpiration
}
```

Solo usar esta opción si queda clara y testeada.

## Tareas

### TASK-0101 — Extraer validación temporal testeable

Crear función pequeña, preferiblemente en servidor o helper interno:

```go
func validatePayloadFreshness(now time.Time, payloadTS int64, pastWindow, futureSkew time.Duration) error
```

No mover criptografía.

Tests:

* acepta `now`;
* acepta `now - window + small`;
* acepta `now + futureSkew - small`;
* rechaza `now - window - small`;
* rechaza `now + futureSkew + small`;
* rechaza window inválida si aplica.

---

### TASK-0102 — Ajustar replay cache TTL

Asegurar que la firma permanece en cache durante toda la ventana aceptable.

Tests:

* paquete con timestamp `now + window` no puede repetirse después de `window + 1s`;
* paquete viejo no queda aceptado;
* replay inmediato sigue bloqueado;
* comportamiento concurrente sigue seguro.

---

### TASK-0103 — Revisar orden de validación

Orden recomendado en `processKnock`:

1. deny IP;
2. rate limit;
3. estructura mínima;
4. replay fast-path opcional por firma;
5. verify/decrypt;
6. TOTP si aplica;
7. timestamp freshness;
8. atomic check+store replay;
9. auth action/source IP/cooldown;
10. ejecutar.

No ejecutar acción antes de check+store replay.

Criterio de aceptación:

* no hay ventana de carrera que permita doble ejecución simultánea del mismo paquete;
* tests pasan con `-race`.

---

# FASE 2 — Fix crítico: validación antes de hooks

## Objetivo

Impedir que hooks reciban parámetros no validados.

## Riesgo principal

Romper hooks existentes que dependían de params más flexibles.

Decisión de seguridad:

* fallar cerrado por defecto;
* documentar cambio;
* no crear modo inseguro salvo necesidad explícita y con nombre alarmante.

## Diseño obligatorio

Extraer validación de params desde `runCommand()` a función compartida.

Validar:

* nombres de params;
* valores de params;
* `..`;
* params nil;
* longitud máxima opcional si se decide.

Regex recomendado para valores, manteniendo compatibilidad actual:

```go
^[a-zA-Z0-9._][a-zA-Z0-9._-]*$
```

Regex recomendado para claves:

```go
^[A-Za-z_][A-Za-z0-9_]{0,63}$
```

Motivo:

* los nombres acaban en variables de entorno `GK_PARAM_<KEY>`;
* claves raras complican shells y scripts;
* `text/template` ya limita nombres útiles a alfanumérico/underscore.

## Tareas

### TASK-0201 — Crear `ValidateParams`

Ubicación sugerida:

* `internal/executor/validation.go`

Función:

```go
func ValidateParams(params map[string]string) error
```

Debe ser usada:

* al inicio de `executor.Execute()`;
* dentro de `runCommand()` solo si es necesario como defensa en profundidad, pero evitar duplicar lógica inconsistente.

Criterio:

* ningún hook se ejecuta si `ValidateParams` falla.

---

### TASK-0202 — Añadir tests de hooks

Tests mínimos:

* param value `$(touch /tmp/pwn)` rechaza antes de hook;
* param value `abc def` rechaza antes de hook;
* param value `--help` rechaza antes de hook;
* param key `bad-key` rechaza antes de hook;
* param key `x=evil` rechaza antes de hook;
* param válido ejecuta hook;
* error de hook sigue cancelando acción como antes.

Usar hooks dummy temporales seguros.

No usar comandos destructivos.

---

### TASK-0203 — Revisar redacción de logs

Asegurar:

* errores de validación no imprimen secretos;
* `sensitive_params` sigue funcionando;
* stdout/stderr de hooks no filtra params sensibles en nivel info/warn.

Si se detecta fuga:

* registrar en `tasks/todo.md`;
* no meter refactor amplio salvo necesario.

---

# FASE 3 — Fix de red: BPF `dst port`

## Objetivo

Anclar captura pasiva a paquetes UDP destinados al puerto configurado.

## Diseño obligatorio

Extraer función:

```go
func buildBPFFilter(listenerCfg config.Listener) string
```

Comportamiento:

* sin `listen_ip`:

```text
udp and dst port <port>
```

* con `listen_ip`:

```text
dst host <listen_ip> and udp and dst port <port>
```

No cambiar semántica de `listen_ip` más allá de anclar puerto destino.

## Tareas

### TASK-0301 — Extraer builder de BPF

Añadir tests unitarios de strings.

Casos:

* port 3001 sin IP;
* port 3001 con IPv4;
* port 3001 con IPv6 si se soporta;
* puerto inválido no debería llegar aquí porque config lo valida.

---

### TASK-0302 — Validar con listener tests

Si no es posible hacer test live sin root:

* crear test unitario del builder;
* documentar test manual en `tasks/research/bpf-integration.md`.

Prueba manual sugerida, si hay entorno seguro:

* enviar paquete UDP con source port igual al puerto de GhostKnock y destination port distinto;
* verificar que no se procesa;
* enviar paquete con destination port correcto;
* verificar que se procesa hasta validación criptográfica.

No requerir root en tests normales de CI.

---

# FASE 4 — Hardening systemd sin degradar producto

## Objetivo

Mejorar contención del servicio sin romper acciones administrativas existentes.

## Riesgo principal

Aplicar hardening fuerte por defecto puede romper:

* `iptables`;
* `systemctl`;
* `docker`;
* escritura en `/tmp`;
* escritura en `/var/www`;
* `run_as_user`;
* acciones de mantenimiento;
* hooks que leen rutas externas.

## Estrategia

No aplicar perfil máximo directamente como default sin validación.

Implementar de forma progresiva:

1. Añadir documentación de hardening.
2. Añadir unit alternativo hardened o bloque comentado.
3. Mantener default compatible.
4. Añadir advertencias en docs.
5. Crear matriz de compatibilidad.

## Tareas

### TASK-0401 — Crear matriz de compatibilidad

Archivo sugerido:

* `docs/HARDENING.md`

Debe incluir tabla:

| Directiva | Beneficio | Riesgo de ruptura | Acciones afectadas |
| --------- | --------- | ----------------- | ------------------ |

Cubrir:

* `NoNewPrivileges`
* `CapabilityBoundingSet`
* `AmbientCapabilities`
* `ProtectSystem`
* `ProtectHome`
* `PrivateTmp`
* `ReadWritePaths`
* `RestrictAddressFamilies`
* `MemoryDenyWriteExecute`
* `LockPersonality`
* `SystemCallFilter`

---

### TASK-0402 — Añadir perfil hardened opcional

Opción preferida:

* crear `packaging/systemd/ghostknockd.hardened.service`

No reemplazar automáticamente el servicio principal.

Perfil inicial conservador:

```ini
[Service]
NoNewPrivileges=yes
RestrictAddressFamilies=AF_INET AF_INET6 AF_PACKET AF_UNIX
LockPersonality=yes
MemoryDenyWriteExecute=yes
```

No activar inicialmente `ProtectSystem=strict`, `ProtectHome=yes` ni `PrivateTmp=yes` como default sin explicar impactos.

Si se añaden:

* deben estar en hardened profile;
* deben tener `ReadWritePaths` explícitos;
* deben estar documentados.

---

### TASK-0403 — Validar service files

Si systemd está disponible:

```bash
systemd-analyze verify packaging/systemd/ghostknockd.service
systemd-analyze verify packaging/systemd/ghostknockd.hardened.service
```

Si no está disponible:

* documentar que no se pudo ejecutar;
* no afirmar validación completa.

---

# FASE 5 — Deuda estructural: ejecución sin shell

## Objetivo

Reducir dependencia de `/bin/sh -c` sin romper configs existentes.

## Regla crítica

No convertir strings shell a argv con:

* `strings.Split`;
* `strings.Fields`;
* parser manual;
* heurísticas.

Eso introduce bugs y bypasses.

## Diseño recomendado

Añadir formato nuevo junto al legacy:

```yaml
actions:
  restart-nginx:
    exec:
      argv: ["systemctl", "restart", "nginx"]

  restart-svc:
    exec:
      argv: ["systemctl", "restart", "{{.Params.name}}"]
      timeout_seconds: 20
```

Mantener formato actual:

```yaml
actions:
  old-action:
    command: "systemctl restart nginx"
```

Pero documentarlo como legacy shell mode.

## Reglas

* `exec.argv` no usa shell.
* Cada argumento se renderiza por separado.
* Params siguen validándose.
* No hay redirecciones shell en `exec.argv`.
* Para redirección, usar scripts explícitos o campos futuros.
* `command` sigue existiendo para compatibilidad, pero genera warning en docs.

## Tareas

### TASK-0501 — Diseñar config backward-compatible

Actualizar `config.Action` con campo nuevo sin romper `command`.

Ejemplo conceptual:

```go
type ExecSpec struct {
    Argv       []string `yaml:"argv"`
    WorkingDir string  `yaml:"working_dir,omitempty"`
}

type Action struct {
    Command string `yaml:"command"`
    Exec    *ExecSpec `yaml:"exec,omitempty"`
}
```

Validación:

* acción debe tener exactamente uno de `command` o `exec.argv`;
* temporalmente se puede permitir `command` legacy;
* si ambos existen, fallar config.

No implementar todavía si no hay plan completo.

---

### TASK-0502 — Implementar runner argv

Nuevo path:

```go
exec.CommandContext(ctx, argv[0], argv[1:]...)
```

No usar shell.

Tests:

* argumento con valor válido se pasa literal;
* valor inválido se rechaza;
* no se interpretan caracteres shell;
* working_dir funciona si se implementa;
* timeout funciona;
* run_as_user sigue funcionando;
* stdout/stderr logging consistente.

---

### TASK-0503 — Migrar ejemplos seguros

Migrar algunos ejemplos simples a `exec.argv`.

No migrar automáticamente comandos complejos con:

* `&&`;
* `>`;
* `;`;
* pipes;
* shell substitutions.

Para esos:

* mantener legacy;
* documentar que deben moverse a scripts controlados o acciones más pequeñas.

---

# FASE 6 — Tests, CI y documentación

## Objetivo

Prevenir regresiones.

## Tareas

### TASK-0601 — Añadir tests de seguridad

Tests obligatorios:

* `TestFreshnessRejectsFutureTimestamp`
* `TestFreshnessRejectsOldTimestamp`
* `TestReplayCacheCoversFutureSkew`
* `TestValidateParamsRejectsInvalidValuesBeforeHooks`
* `TestValidateParamsRejectsInvalidKeysBeforeHooks`
* `TestBPFFilterUsesDstPort`
* `TestLegacyShellStillWorks`
* `TestArgvExecDoesNotUseShell` cuando exista argv
* `TestReloadRace` si se puede aislar sin señales reales

---

### TASK-0602 — Añadir CI si no existe

Si el repo no tiene GitHub Actions, proponer workflow:

```yaml
name: Go CI

on:
  push:
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: go test ./...
      - run: go test -race ./...
      - run: go vet ./...
      - run: make build
```

No añadir herramientas opcionales que fallen si no están instaladas salvo que se instalen explícitamente.

---

### TASK-0603 — Documentar cambios de comportamiento

Actualizar docs:

* timestamp requiere relojes razonablemente sincronizados;
* recomendar NTP;
* explicar BPF `dst port`;
* advertir que hooks reciben solo params validados;
* explicar legacy shell mode;
* explicar hardening systemd opcional;
* añadir sección “Security Model and Limitations”.

---

## 6. Criterios de aceptación por vulnerabilidad

### GK-SEC-001 aceptado si

* timestamps futuros fuera de ventana se rechazan;
* timestamps antiguos fuera de ventana se rechazan;
* replay inmediato se rechaza;
* replay después de TTL no es aceptable mientras timestamp siga en ventana;
* tests pasan con `go test ./...`;
* tests pasan con `go test -race ./...`.

---

### GK-SEC-002 aceptado si

* ningún pre-hook se ejecuta con params inválidos;
* ningún action hook se ejecuta con params inválidos;
* nombres de params inválidos se rechazan;
* valores de params inválidos se rechazan;
* params válidos siguen funcionando;
* logs no filtran secretos;
* tests cubren orden de ejecución.

---

### GK-SEC-003 aceptado si

* builder BPF usa `dst port`;
* tests verifican filtro con y sin `listen_ip`;
* no cambia comportamiento de config no relacionada;
* documentación actualizada.

---

### GK-SEC-004 aceptado si

* no se rompe service default;
* existe perfil hardened o documentación clara;
* impactos documentados;
* verificación systemd ejecutada si está disponible.

---

### GK-SEC-005 aceptado si

* existe diseño backward-compatible;
* no se hace split inseguro de strings;
* `command` legacy sigue funcionando;
* `exec.argv` funciona sin shell;
* tests demuestran que shell metacharacters no se interpretan en argv mode.

---

## 7. Orden recomendado de PRs

No mezclar todo en un único PR.

### PR-1 — Baseline y tests iniciales

Incluye:

* `tasks/`;
* baseline documentado;
* tests de timestamp que fallan;
* tests de BPF builder si se extrae sin cambiar todavía.

---

### PR-2 — Timestamp + replay TTL

Incluye:

* validación temporal;
* TTL correcto;
* tests de replay;
* documentación NTP/replay.

No tocar hooks ni systemd.

---

### PR-3 — Validación antes de hooks

Incluye:

* `ValidateParams`;
* validación de keys y values;
* tests de hook order;
* docs sobre hooks.

No tocar executor shell aún.

---

### PR-4 — BPF `dst port`

Incluye:

* builder BPF;
* cambio a `dst port`;
* tests;
* docs.

---

### PR-5 — Hardening systemd opcional

Incluye:

* `docs/HARDENING.md`;
* unit hardened opcional;
* no romper default.

---

### PR-6 — Diseño e implementación `exec.argv`

Incluye:

* cambio de config backward-compatible;
* runner argv;
* tests;
* migración parcial de ejemplos simples;
* legacy shell documentado.

---

## 8. Prohibiciones explícitas

No hacer lo siguiente:

* No relajar regex de params para “hacer funcionar” un ejemplo.
* No aceptar rutas `/`, URLs `:`, espacios o flags en params sin rediseño formal.
* No mover a argv dividiendo strings por espacios.
* No aplicar `ProtectSystem=strict` al service default sin matriz de compatibilidad.
* No añadir `PrivateTmp=true` al default sin revisar ejemplos que usan `/tmp`.
* No eliminar `command` legacy sin migración.
* No cambiar formato de paquete de red en fixes inmediatos.
* No cambiar primitivas criptográficas en PRs de replay.
* No mezclar separación de claves Ed25519/X25519 con fixes urgentes.
* No borrar tests fuzz existentes.
* No marcar como fixed sin test.
* No asumir que un MITM puede modificar payload firmado.
* No repetir hallazgos descartados como si fueran reales.

---

## 9. Plantillas de trabajo

### 9.1 Plantilla de plan

Usar en `tasks/todo.md`:

````md
## Plan: <nombre>

### Objetivo
...

### No objetivos
...

### Archivos afectados
- ...

### Riesgos de regresión
- ...

### Diseño
...

### Tests
- ...

### Comandos de verificación
```bash
go test ./...
go test -race ./...
go vet ./...
make build
````

### Rollback

...

````

---

### 9.2 Plantilla de resultado

```md
## Resultado: <task>

### Cambios realizados
- ...

### Tests ejecutados
```bash
...
````

### Resultado de tests

...

### Riesgos pendientes

* ...

### Lecciones añadidas

* ...

````

---

## 10. Notas de diseño específicas para GhostKnock

### 10.1 Replay cache

La replay cache no debe depender únicamente de “firma vista recientemente” con TTL menor que la ventana aceptable.

Regla:

```text
replay_cache_ttl >= past_window + future_skew + guard
````

o expiración por paquete:

```text
expiration >= timestamp + past_window + guard
```

---

### 10.2 TOTP

TOTP no sustituye replay cache.

No asumir que TOTP arregla replay.

Si TOTP está activo:

* el OTP puede ser válido durante su ventana;
* replay cache sigue siendo obligatoria;
* timestamp freshness sigue siendo obligatoria.

---

### 10.3 Hooks

Los hooks son frontera de seguridad.

Reglas:

* params validados antes de hook;
* keys validadas antes de convertirse en env vars;
* valores sensibles no deben loguearse;
* docs deben exigir comillas en scripts shell;
* evitar `eval`;
* evitar interpolar params en comandos peligrosos.

Ejemplo seguro en docs:

```bash
#!/bin/sh
set -eu

case "${GK_PARAM_TARGET:-}" in
  ""|*[!A-Za-z0-9._-]*)
    exit 1
    ;;
esac

printf '%s\n' "$GK_PARAM_TARGET"
```

Ejemplo inseguro que debe marcarse como prohibido:

```bash
eval "iptables -A INPUT -s $GK_PARAM_TARGET -j DROP"
```

---

### 10.4 Shell legacy

Mientras exista `command` legacy:

* mantener regex estricta;
* mantener rechazo de guion inicial;
* mantener rechazo de espacios;
* mantener rechazo de `..`;
* no añadir excepciones por comodidad.

Si un caso de uso necesita rutas, URLs o flags:

* usar script controlado;
* usar `exec.argv`;
* o diseñar validador específico por tipo de parámetro.

---

### 10.5 Validadores tipados futuros

No implementar en fixes inmediatos salvo necesidad.

Idea de medio plazo:

```yaml
params_schema:
  service:
    type: systemd_unit
  target:
    type: ip_or_cidr
  branch:
    type: git_ref_safe
```

Esto permitiría más flexibilidad sin relajar un regex global.

No mezclar con PRs críticos.

---

## 11. Definición de terminado

Una tarea está terminada solo si:

* el plan existe;
* el cambio es pequeño y revisable;
* tests específicos existen;
* tests globales pasan o fallos preexistentes están documentados;
* no se rompió compatibilidad sin documentar;
* docs actualizadas si hay cambio de comportamiento;
* `tasks/todo.md` actualizado;
* `tasks/lessons.md` actualizado si hubo corrección o aprendizaje;
* el agente puede explicar el cambio en 5 frases.

---

## 12. Prioridad actual

Orden de trabajo recomendado:

1. Crear baseline y tareas.
2. Corregir timestamp futuro y TTL replay.
3. Validar params antes de hooks.
4. Corregir BPF a `dst port`.
5. Añadir docs y perfil systemd hardened opcional.
6. Diseñar `exec.argv` sin romper legacy shell.
7. Fortalecer CI y tests de seguridad.

No cambiar el orden salvo justificación documentada.

---

## 13. Resumen ejecutivo para agentes

GhostKnock ya tiene varias defensas correctas: worker pool, rate limit, replay cache, regex estricto, criptografía moderna y hot reload con mutex. No lo trates como código roto.

Los fallos confirmados son lógicos y de hardening:

* timestamp futuro;
* hooks antes de validación;
* BPF sin `dst port`;
* root sin sandboxing;
* shell legacy como deuda estructural.

Corrige con precisión quirúrgica. No hagas refactors amplios en fixes críticos. No introduzcas compatibilidad rota por “seguridad”. Cada cambio debe tener test, justificación y rollback.
