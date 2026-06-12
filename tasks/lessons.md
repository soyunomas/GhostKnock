# Lessons Learned

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
