# Phase 2 Security Review

## Scope

Independent expert review and local adversarial testing of parameter validation,
template analysis, hook ordering, environment construction, sensitive output,
TOTP interaction, asynchronous hooks, and reverts.

## Confirmed findings and resolutions

### High — Required-parameter bypass through valid template syntax

Regex matching did not recognize whitespace, function arguments, `index`, or
indirect aliases.

Resolution:

- Required parameters are extracted from the parsed `text/template` AST.
- Direct `.Params.name` access is supported.
- Dynamic indexing, indirect aliases, whole-map access, and root rendering are
  rejected before configuration or execution can proceed.
- `Execute` always audits the actual template and does not trust supplied metadata.

### Medium — Case-sensitive sensitive-parameter handling

`sensitive_params: [token]` did not redact a valid `Token` parameter even though
both become the same uppercase hook environment name.

Resolution:

- Sensitive names are validated for environment compatibility.
- Matching and redaction are case-insensitive.
- Equivalent duplicate names are rejected.

### Medium — TOTP environment collision

`otp` and `OTP` could converge on `GK_PARAM_OTP` after the verified lowercase
entry was removed.

Resolution:

- Parameter keys, values, and case-insensitive collisions are validated before
  TOTP extraction.
- `otp` is reserved case-insensitively and cannot reach hooks.

### Low — Mutable maps observed asynchronously

Post-hooks and reverts retained the caller's parameter map.

Resolution:

- `Execute` clones params and sensitive-name slices before validation.
- Tests verify post-hook and revert snapshots.

## Verification

```bash
GOCACHE=/tmp/ghostknock-go-build go test ./... -count=1
GOCACHE=/tmp/ghostknock-go-build go test -race ./... -count=1
GOCACHE=/tmp/ghostknock-go-build go test -race ./internal/config ./internal/executor ./cmd/ghostknockd -count=100
GOCACHE=/tmp/ghostknock-go-build go vet ./...
GOCACHE=/tmp/ghostknock-go-build make build
gosec ./...
govulncheck ./...
```

Results:

- All tests and race tests pass.
- Focused packages pass 100 race-enabled repetitions.
- Build passes.
- No Phase 2 findings were added by gosec.
- Final expert review found no residual defects introduced by Phase 2.

## Pre-existing debt

- `/bin/sh -c` legacy execution remains.
- Post-hook and revert goroutines are not included in the server execution wait group.
- Reverts use in-memory sleeps and are lost on restart.
- The IPv6 vet warning remains in the client.
- Go 1.25.5 is affected by `GO-2026-4971`; fixed in Go 1.25.10.
