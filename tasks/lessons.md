# Lessons Learned

## 2026-06-12 — Security configuration must reject unknown fields

### Error o riesgo detectado

The YAML decoder accepts misspelled fields. A typo such as `totp_secert`
therefore validates successfully while silently disabling the intended TOTP
control.

### Regla nueva

Security-sensitive configuration must use a strict schema, including custom
YAML decoders, and tests must prove that misspelled controls fail closed.

### Ejemplo

`totp_secret` must be accepted, while `totp_secert` must make `ghostknockd -t`
fail with the exact unknown field and line.

### Archivos relacionados

- `internal/config/config.go`
- `internal/config/config_test.go`
- `tasks/research/main-security-audit-2026-06-12.md`

## 2026-06-12 — Delayed security side effects need a bounded lifecycle

### Error o riesgo detectado

Main command execution is bounded and tracked, but post-hooks and delayed
reversions are launched as independent goroutines. They can accumulate and are
not guaranteed to complete during shutdown.

### Regla nueva

Any asynchronous hook or rollback that affects security state must have a
bounded queue, explicit concurrency, cancellation, shutdown accounting, and a
documented persistence decision.

### Ejemplo

A firewall-opening action with a delayed close must not lose its close
operation merely because the daemon receives SIGTERM before the delay expires.

### Archivos relacionados

- `cmd/ghostknockd/main.go`
- `internal/executor/executor.go`
- `tasks/research/main-security-audit-2026-06-12.md`

## 2026-06-12 — Wildcard capture must be link-type independent

### Error o riesgo detectado

The first `interface: any` fix correctly used AF_PACKET index `0`, but retained
`SOCK_RAW` and a fixed 14-byte Ethernet layout. Packets from loopback or
TUN-style interfaces can have different link headers, so wildcard binding alone
did not provide wildcard processing. The same assumption also rejected VLAN
frames in BPF before the parser could handle them.

### Regla nueva

When capturing across heterogeneous interfaces, normalize the link layer before
filtering or explicitly support every link type. Kernel binding scope and
userspace parser scope must be reviewed together.

### Ejemplo

GhostKnock uses AF_PACKET `SOCK_DGRAM`, which strips the physical header and
presents one IPv4 layout to BPF and the parser across Ethernet, loopback, and
TUN interfaces while delegating VLAN normalization to the kernel.

### Archivos relacionados

- `internal/listener/listener_linux.go`
- `internal/listener/listener_test.go`
- `tasks/research/native-listener-hardening.md`

## 2026-06-12 — Verify wildcard semantics at dependency boundaries

### Error o riesgo detectado

The native listener represented `interface: any` with a nil
`*net.Interface`, but the packet library dereferences that pointer before
binding and therefore panics instead of requesting wildcard capture.

### Regla nueva

Before using nil or zero values to express operating-system wildcard behavior,
verify the wrapper library's contract and add a regression test for the exact
boundary value.

### Ejemplo

AF_PACKET wildcard capture uses a non-nil synthetic `net.Interface` whose
index is `0`; tests assert both properties before the value reaches
`packet.Listen`.

### Archivos relacionados

- `internal/listener/listener_linux.go`
- `internal/listener/listener_test.go`

## 2026-06-12 — Isolate the Go build cache in managed environments

### Error o riesgo detectado

Running baseline Go commands in parallel used the default read-only build cache
and produced setup failures unrelated to the repository. Concurrent toolchain
access also produced a transient, misleading standard-library error.

### Regla nueva

In this workspace, run baseline Go commands sequentially with a writable cache
such as `GOCACHE=/tmp/ghostknock-go-build`. Classify the first failure before
changing code.

### Ejemplo

```bash
GOCACHE=/tmp/ghostknock-go-build go test -race ./...
```

### Archivos relacionados

- `go.mod`
- `tasks/todo.md`

## 2026-06-12 — Validate duration arithmetic and reload semantics together

### Error o riesgo detectado

The initial replay fix checked only for negative overflow results and allowed the
replay window to grow during hot reload. A wrapped duration could become a short
positive TTL, and a widened window could make previously purged packets valid
again.

### Regla nueva

Validate integer configuration before converting it to `time.Duration`, use
bounded arithmetic or bounded inputs, and treat any hot-reloaded setting that
changes an acceptance window together with the state retained for that window.

### Ejemplo

`replay_window_seconds` is constrained to 1-3600 and cannot change during hot
reload; replay expiration is derived from the authenticated packet timestamp.

### Archivos relacionados

- `internal/config/config.go`
- `cmd/ghostknockd/main.go`
- `cmd/ghostknockd/main_test.go`

## 2026-06-12 — Do not infer template semantics with regex

### Error o riesgo detectado

The first Phase 2 implementation used a regex to identify required
`{{.Params.name}}` references. Valid Go template forms with whitespace,
functions, dynamic indexing, aliases, or the root context could bypass that
interpretation and allow hooks to run before missing parameters were detected.

### Regla nueva

Use the parser or AST for structured languages. For security validation, reject
dynamic constructs that cannot be resolved before side effects, and revalidate
the actual parsed object at the execution boundary.

### Ejemplo

GhostKnock walks the `text/template/parse` AST, accepts direct
`.Params.name` references, and rejects `index`, indirect aliases, and rendering
the whole root context.

### Archivos relacionados

- `internal/config/template_params.go`
- `internal/executor/validation.go`
- `internal/executor/executor.go`

## 2026-06-12 — Preserve configuration semantics across implementation replacements

### Error o riesgo detectado

Rebasing the native `AF_PACKET` listener over the previous `libpcap` listener
kept destination-port filtering but silently stopped honoring `listen_ip`.

### Regla nueva

When one subsystem replaces another, compare every public configuration field
and security invariant, not only whether the new implementation compiles.

### Ejemplo

The native packet parser must reject a packet whose destination IP differs from
configured `listener.listen_ip`, even though it already validates the UDP
destination port.

### Archivos relacionados

- `internal/listener/listener_linux.go`
- `internal/listener/listener_test.go`
- `internal/config/config.go`

## 2026-06-12 — Goroutines fire-and-forget escapan del ciclo de vida

### Error o riesgo detectado
Las tareas asíncronas lanzadas con `go fn()` dentro de `executor.Execute`
(post-hooks y reversiones) no estaban acotadas por el semáforo anti-forkbomb ni
esperadas por el `WaitGroup` de apagado. Un comando como root (reversión) podía
ejecutarse después de que el daemon reportara un cierre limpio, y la concurrencia
de fondo era ilimitada.

### Regla nueva
Toda goroutine que ejecute trabajo de seguridad (procesos, hooks, comandos como
root) debe estar acotada por un semáforo y rastreada por un WaitGroup drenable en
el apagado. Nunca usar `go fn()` suelto para comandos o hooks. Las esperas largas
(`time.Sleep`) deben ser cancelables por contexto para no bloquear el cierre.

### Ejemplo
`scheduleRevert` ahora corre como `Coordinator.goTracked(func(){ c.sleep(delay);
c.runBounded(revert) })`, y el apagado llama `executionCoord.Wait()` tras
`cancel()`, que interrumpe la espera y ejecuta la reversión de inmediato.

### Archivos relacionados
- internal/executor/coordinator.go
- internal/executor/executor.go
- cmd/ghostknockd/main.go
