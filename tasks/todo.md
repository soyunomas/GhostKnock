# GhostKnock Security Remediation TODO

## Active Plan: Native listener hardening

### Goal

Make `listener.interface: any` work on Linux without panicking, install a
kernel BPF filter for IPv4 UDP packets addressed to the configured destination
port, and make the current IPv4-only listener policy explicit.

### Scope

- Represent Linux wildcard capture with AF_PACKET interface index `0`.
- Attach classic BPF before `bind(2)` so unrelated traffic never enters the
  socket receive path.
- Use AF_PACKET cooked datagrams so filtering does not assume a fixed Ethernet
  header and works across Ethernet, loopback, and TUN interfaces.
- Filter by IPv4, UDP, non-fragmented packet, optional destination IPv4, and
  UDP destination port.
- Keep the Go packet parser as defense in depth.
- Reject IPv6 `listener.listen_ip` during configuration loading.
- Document that the daemon listener is IPv4-only in this phase.

### Non-goals

- Do not implement IPv6 packet parsing or dual-stack BPF in this change.
- Do not alter the packet format, client transport, replay, or executor.
- Do not add libpcap, CGO, or a new dependency.

### Files affected

- `internal/listener/listener_linux.go`
- `internal/listener/listener_test.go`
- `internal/config/config.go`
- `internal/config/config_test.go`
- `go.mod`
- `README.md`
- `config.yaml`
- `config.yaml.example`
- `docs/USER_GUIDE.md`
- `tasks/todo.md`
- `tasks/lessons.md`
- `tasks/research/native-listener-hardening.md`
- `tasks/research/native-listener-review.md`

### Risks

- Incorrect BPF offsets or jump distances could drop valid packets.
- Wildcard capture could regress if a nil interface is passed to the packet
  library.
- A fixed Ethernet header assumption would make `any` fail on non-Ethernet
  interfaces.
- An undocumented IPv4-only policy could make IPv6 deployments fail silently.

### Verification

```bash
GOCACHE=/tmp/ghostknock-go-build go test ./internal/listener ./internal/config -count=1
GOCACHE=/tmp/ghostknock-go-build go test ./... -count=1
GOCACHE=/tmp/ghostknock-go-build go test -race ./... -count=1
GOCACHE=/tmp/ghostknock-go-build go vet ./...
GOCACHE=/tmp/ghostknock-go-build make build
```

### Rollback

Revert the listener, config validation, tests, and matching documentation as
one unit. Do not retain the `any` resolver without its regression test or retain
the IPv4-only documentation without fail-fast validation.

### Tasks

- [x] TASK-0301: Implement real `interface: any`
  - Files: `internal/listener/listener_linux.go`, `internal/listener/listener_test.go`
  - Risk: nil interface panic or accidental bind to one interface.
  - Tests: wildcard resolver returns a non-nil interface with index `0`.
  - Acceptance: `any` and the compatibility empty value use AF_PACKET ifindex `0`.
  - Status: Complete.
  - Notes: `any` is represented by a non-nil synthetic interface with index
    `0`, matching Linux AF_PACKET wildcard semantics without triggering the
    packet library's nil dereference. `SOCK_DGRAM` removes the physical link
    header so the parser works across heterogeneous interfaces.

- [x] TASK-0302: Install destination-oriented kernel BPF
  - Files: `internal/listener/listener_linux.go`, `internal/listener/listener_test.go`
  - Risk: false rejection from malformed offsets, IPv4 options, or fragmentation.
  - Tests: accept correct UDP destination; reject source-port-only match, TCP,
    wrong destination IP, wrong destination port, and fragments.
  - Acceptance: BPF is assembled and supplied in `packet.Config` before bind.
  - Status: Complete.
  - Notes:
    - The filter is supplied through `packet.Config.Filter`, which the library
      installs before `bind(2)`.
    - VM tests cover destination port, optional destination IPv4, IPv4 options,
      TCP, source-port-only matches, and fragments.
    - A live AF_PACKET open was not possible in this environment because the
      current user has neither root nor `CAP_NET_RAW`.

- [x] TASK-0303: Make IPv6 policy explicit
  - Files: config validation/tests and listener documentation.
  - Risk: configurations previously accepted but ineffective now fail fast.
  - Tests: IPv4/empty `listen_ip` accepted; IPv6 and malformed addresses rejected.
  - Acceptance: daemon listener is explicitly IPv4-only and errors during config load.
  - Status: Complete.
  - Notes: The daemon listener remains IPv4-only; malformed and IPv6
    `listen_ip` values now fail during `LoadConfig`, and documentation states
    the limitation.

## Result: Native listener hardening

### Changes made

- Replaced the broken nil wildcard with AF_PACKET `ifindex=0`.
- Switched capture to cooked AF_PACKET datagrams to remove fixed link-header
  assumptions from `interface: any`.
- Added a classic BPF program installed before bind for IPv4, UDP,
  non-fragmented traffic, optional destination IPv4, and destination port.
- Retained equivalent parser checks as defense in depth.
- Made IPv4-only support an explicit, fail-fast configuration policy.
- Added BPF VM, wildcard-interface, parser, and configuration tests.
- Addressed the independent review by replacing fixed Ethernet offsets with
  cooked AF_PACKET datagrams and tightening malformed-packet validation.

### Verification results

- `go test ./internal/listener ./internal/config -count=1`: pass.
- `go test ./... -count=1`: pass.
- `go test -race ./... -count=1`: pass.
- `make build`: pass.
- `go vet ./...`: only the pre-existing client address-format finding remains
  at `cmd/ghostknock/main.go:241`.
- `git diff --check`: pass.

## Integration Plan: Rebase `dev` onto `main`

### Goal

Rebase the seven `dev` commits onto the current security-remediated `main`
without losing either branch's behavior.

### Scope

- Preserve the timestamp, replay, parameter-validation, and redaction fixes.
- Preserve the native Linux listener and cross-platform credential abstraction.
- Keep `main` unchanged and protect both original tips with backup branches.
- Correct integration regressions discovered by tests or semantic review.

### Non-goals

- Do not push or force-update `origin/dev`.
- Do not advance the planned BPF hardening phase.
- Do not merge branches or rewrite `main`.

### Risks

- The listener replacement can silently drop existing `listen_ip` semantics.
- The documentation rewrite can overwrite security guidance from `main`.
- Rebased history requires a force-with-lease push if published.

### Verification

```bash
GOCACHE=/tmp/ghostknock-go-build go test ./... -count=1
GOCACHE=/tmp/ghostknock-go-build go test -race ./... -count=1
GOCACHE=/tmp/ghostknock-go-build go vet ./...
GOCACHE=/tmp/ghostknock-go-build make build
git range-diff b013e3a..backup/dev-before-main-rebase-20260612 main..dev
```

### Rollback

Reset local `dev` to `backup/dev-before-main-rebase-20260612`; `main` is
unchanged and also protected by `backup/main-before-dev-rebase-20260612`.

### Task

- [x] TASK-INTEGRATE-001: Rebase and verify `dev`
  - Files: branch history, listener implementation/tests, integration docs.
  - Risk: loss of security fixes or public listener configuration behavior.
  - Tests: global test, race, vet, build, range-diff, clean worktree.
  - Acceptance: `dev` is linear on `main`, both feature sets remain, and rollback refs exist.
  - Status: Complete.
  - Notes:
    - Seven `dev` commits were rebased onto `main`; `main` was not rewritten.
    - Backup refs preserve both original branch tips.
    - The native listener now preserves `listen_ip` filtering and destination-port semantics.
    - Linux tests, race tests, builds, and Windows client cross-builds pass.
    - `go vet` retains only the pre-existing IPv6 address-format finding at
      `cmd/ghostknock/main.go:241`.
    - Build commands emitted non-fatal module stat-cache warnings because
      `/home/yo/go/pkg/mod/cache` is read-only in this environment.

## Result: Rebase `dev` onto `main`

### Changes made

- Replayed the seven `dev` commits on top of the security-remediated `main`.
- Preserved the lowercase Go module path and all Phase 1/2 security behavior.
- Combined the portable credential helper with the validated executor.
- Replaced the legacy `libpcap` listener with the native Linux listener.
- Restored `listen_ip` behavior and documented that the current parser is IPv4-only.
- Reconciled the rewritten user guide with the security guidance from `main`.

### Verification results

- `go test ./... -count=1`: pass.
- `go test -race ./... -count=1`: pass.
- `make build`: pass.
- `make build-windows`: pass.
- `go vet ./...`: only the documented pre-existing IPv6 client finding remains.
- `git range-diff`: all seven original `dev` commits are accounted for.

## Active Plan

### Goal

Complete Phase 2 by validating all parameter names and values before any log,
hook, command, or scheduled revert can observe them.

### Scope

- Add a shared `ValidateParams` function for keys and values.
- Validate before debug logging and before constructing any hook context.
- Keep the same strict value character set and `..` rejection.
- Validate hook environment keys and reject case-insensitive collisions.
- Preserve nil and empty parameter maps as valid.
- Ensure sensitive hook output is redacted before logging.
- Add temporary, non-destructive hook tests and document safe hook usage.
- Run an independent expert security review after implementation.

### Non-goals

- Do not relax the parameter value regex.
- Do not add typed parameter schemas or new configuration fields.
- Do not redesign shell execution or implement `exec.argv`.
- Do not change timestamp, replay, BPF, or systemd behavior.
- Do not advance to Phase 3 without explicit user approval.

### Files affected

- `internal/executor/validation.go`
- `internal/executor/executor.go`
- `internal/executor/hooks.go`
- `internal/executor/executor_test.go`
- `internal/config/config.go`
- `internal/config/template_params.go`
- `internal/config/template_params_test.go`
- `cmd/ghostknockd/main.go`
- `cmd/ghostknockd/main_test.go`
- `README.md`
- `config.yaml.example`
- `docs/USER_GUIDE.md`
- `tasks/todo.md`
- `tasks/lessons.md` if the final review finds a new lesson
- `tasks/research/phase2-security-review.md`

### Risks

- Existing hooks may rely on parameter names that are invalid environment keys.
- Case-insensitive key collisions could create ambiguous `GK_PARAM_*` values.
- Logging validation errors or hook output could expose sensitive values.
- Post-hooks and reverts run asynchronously, so tests must avoid timing races.

### Verification

```bash
GOCACHE=/tmp/ghostknock-go-build go test ./internal/executor -count=1
GOCACHE=/tmp/ghostknock-go-build go test -race ./internal/executor -count=100
GOCACHE=/tmp/ghostknock-go-build go test ./... -count=1
GOCACHE=/tmp/ghostknock-go-build go test -race ./... -count=1
GOCACHE=/tmp/ghostknock-go-build go vet ./...
GOCACHE=/tmp/ghostknock-go-build make build
gosec ./...
govulncheck ./...
git status --short
```

### Rollback

Remove the shared validator and its tests, restore the previous executor and
hook logging behavior together, and revert only Phase 2 documentation. Validation
must never remain in `runCommand` while being removed from the pre-hook boundary.

## Tasks

- [x] TASK-0201: Create `ValidateParams`
  - Objective: Reject invalid parameter names and values before any hook or command.
  - Files: `internal/executor/validation.go`, `internal/executor/executor.go`
  - Risk: Stricter key validation can reject previously accepted but unsafe hook environment names.
  - Tests: Nil, empty, valid keys/values, invalid keys, invalid values, `..`, case-insensitive collisions.
  - Acceptance: `Execute` validates before logs/hooks and `runCommand` uses the same validator defensively.
  - Status: Complete.
  - Notes:
    - Key regex is `^[A-Za-z_][A-Za-z0-9_]{0,63}$`.
    - Values retain the existing strict regex and `..` rejection.
    - Case-insensitive environment collisions are rejected.
    - Required parameters are extracted from the parsed template AST.

- [x] TASK-0202: Add hook-order tests
  - Objective: Prove invalid parameters never reach global or action hooks.
  - Files: `internal/executor/executor_test.go`
  - Risk: Tests could accidentally execute unsafe commands or write system paths.
  - Tests: `$(...)`, spaces, leading hyphen, invalid key punctuation, valid hook, hook failure cancellation.
  - Acceptance: Tests use scripts and marker files under `t.TempDir()` only.
  - Status: Complete.
  - Notes:
    - No destructive commands or root requirements.
    - Tests cover global/action pre-hooks, post-hooks, reverts, hook failures,
      missing required params, dynamic template access, and parameter snapshots.

- [x] TASK-0203: Review hook logs and sensitive parameters
  - Objective: Prevent hook stdout/stderr from reintroducing sensitive values into logs.
  - Files: `internal/executor/hooks.go`, `internal/executor/executor_test.go`
  - Risk: Broad output suppression could reduce operational diagnostics.
  - Tests: Sensitive value printed by successful and failing hooks is redacted.
  - Acceptance: Hook output remains useful but configured sensitive values are replaced.
  - Status: Complete.
  - Notes:
    - Invalid values are not included in validation errors.
    - Sensitive names are validated and matched case-insensitively.
    - Command and hook stdout/stderr are redacted.

- [x] TASK-0101: Extract testable timestamp freshness validation
  - Objective: Explicitly reject payloads older than the past window or newer than the allowed future skew.
  - Files: `cmd/ghostknockd/main.go`, `cmd/ghostknockd/main_test.go`
  - Risk: Incorrect inclusive/exclusive boundary handling.
  - Tests: Current timestamp, both accepted boundaries, old timestamp, future timestamp, invalid durations.
  - Acceptance: Freshness tests are deterministic and use an injected `now`.
  - Status: Complete.
  - Notes:
    - Initial compatibility uses `futureSkew == replayWindow`.
    - Boundary timestamps at exactly `now-window` and `now+futureSkew` are accepted.
    - Older/newer timestamps and invalid duration inputs are rejected.

- [x] TASK-0102: Adjust replay cache expiration
  - Objective: Keep each accepted signature cached throughout the complete acceptance window.
  - Files: `cmd/ghostknockd/main.go`, `cmd/ghostknockd/main_test.go`
  - Risk: Early cache expiry permits delayed replay; excessive TTL increases bounded memory use.
  - Tests: Expiration derived from the payload timestamp still covers `window + 1s` for a future-dated packet.
  - Acceptance: Replay expiration covers the packet's complete remaining acceptance window plus guard.
  - Status: Complete.
  - Notes:
    - Expiration is `payloadTimestamp + pastWindow + 1s`.
    - A current packet is retained for six seconds; a packet five seconds in the
      future is retained for eleven seconds.
    - Tests verify coverage beyond a replay attempt at `window + 1s`.

- [x] TASK-0103: Review and enforce validation order
  - Objective: Perform freshness before an atomic replay check+store and before authorization/execution.
  - Files: `cmd/ghostknockd/main.go`, `cmd/ghostknockd/main_test.go`
  - Risk: Concurrent duplicate packets could both pass if check and store are separated.
  - Tests: Concurrent check+store permits exactly one insertion; race detector passes.
  - Acceptance: No action can execute before freshness and atomic replay storage succeed.
  - Status: Complete.
  - Notes:
    - The cheap replay fast-path remains before cryptographic verification.
    - TOTP and timestamp freshness precede atomic replay check+store.
    - Authorization and execution remain after successful replay storage.
    - A concurrent test verifies exactly one successful insertion.

- [x] TASK-0001: Create task structure
  - Objective: Establish the files required to plan and record remediation work.
  - Files: `AGENTS.md`, `tasks/todo.md`, `tasks/lessons.md`, `tasks/research/`
  - Risk: Documentation could drift from the actual repository state.
  - Tests: Confirm paths exist and inspect `git status --short`.
  - Acceptance: Required files and directory exist; this active plan is present.
  - Status: Complete.
  - Notes: The supplied lowercase `agents.md` was canonicalized as `AGENTS.md`.

- [x] TASK-0002: Execute baseline
  - Objective: Record the current test, race, vet, and build state before security changes.
  - Files: `tasks/todo.md`
  - Risk: Native dependencies or sandbox constraints may cause environmental failures.
  - Tests: `go test ./...`, `go test -race ./...`, `go vet ./...`, `make build`.
  - Acceptance: Every command has a recorded result and failures are classified.
  - Status: Complete with one pre-existing vet finding.
  - Notes:
    - Toolchain: Go 1.25.5, module directive Go 1.24.0.
    - Initial parallel runs of test, race, and vet failed because the default
      `/home/yo/.cache/go-build` is read-only in the managed environment. These
      were environmental failures, not repository failures.
    - Repeated sequentially with `GOCACHE=/tmp/ghostknock-go-build`.
    - `go test ./...`: pass.
    - `go test -race ./...`: pass.
    - `go vet ./...`: fail at `cmd/ghostknock/main.go:241`; vet reports that
      address format `"%s:%d"` passed to `net.Dial` does not support IPv6.
      This is pre-existing and was not changed in Phase 0.
    - `make build`: pass; all three Linux binaries compiled.

- [x] TASK-0003: Identify configuration fixtures
  - Objective: Ensure tests use generated or isolated data and do not rely on example secrets.
  - Files: `tasks/research/config-fixtures.md`; test files only if a gap requires a fixture.
  - Risk: Adding unnecessary fixture code could expand Phase 0 without improving coverage.
  - Tests: Inspect all `*_test.go`, test YAML files, filesystem writes, and root assumptions.
  - Acceptance: Fixture dependencies and any required follow-up are documented.
  - Status: Complete.
  - Notes:
    - No config-loader unit tests currently exist.
    - Existing fuzz tests build inputs in memory and require neither keys nor root.
    - `test.yaml` is a manual E2E example and is unsuitable as an automated fixture.
    - Detailed constraints for future fixtures are in
      `tasks/research/config-fixtures.md`.

## Completed

- [x] Native listener supports real AF_PACKET wildcard capture with `interface: any`.
- [x] Kernel BPF filters IPv4 UDP by destination port and optional destination IP.
- [x] IPv6 listener policy is explicit, documented, and rejected at config load.
- [x] Phase 1 rejects timestamps outside the symmetric past/future window.
- [x] Phase 1 replay expiration covers each packet's complete accepted window plus guard.
- [x] Phase 1 preserves atomic duplicate suppression under concurrency.
- [x] Phase 1 documentation explains symmetric skew and NTP requirements.
- [x] Phase 2 validates params before logs, hooks, commands, and reverts.
- [x] Phase 2 rejects ambiguous environment keys and dynamic Params template access.
- [x] Phase 2 redacts sensitive values from command and hook output.
- [x] Phase 2 passed independent expert review with no residual introduced defects.
- [x] Phase 0 tracking structure created.
- [x] Baseline executed and environmental retries documented.
- [x] Existing fixture dependencies audited.

## Result: Phase 1 — Timestamp and replay expiration

### Changes made

- Added explicit, testable timestamp freshness validation.
- Replaced `time.Since` with past and future comparisons against one captured `now`.
- Moved replay insertion after freshness validation.
- Added atomic cache helpers and per-packet expiration covering the remaining acceptance window and guard.
- Added unit, integration-style flow, and concurrency tests.
- Updated README, user guide, and YAML comments.

### Tests executed

```bash
GOCACHE=/tmp/ghostknock-go-build go test ./cmd/ghostknockd -count=1
GOCACHE=/tmp/ghostknock-go-build go test -race ./cmd/ghostknockd -count=1
GOCACHE=/tmp/ghostknock-go-build go test ./... -count=1
GOCACHE=/tmp/ghostknock-go-build go test -race ./... -count=1
GOCACHE=/tmp/ghostknock-go-build go vet ./...
make build
```

### Test results

- Package tests: pass.
- Package race tests: pass.
- Global tests: pass.
- Global race tests: pass.
- Build: pass.
- Vet: the pre-existing IPv6 address formatting finding remains at
  `cmd/ghostknock/main.go:241`; no new vet findings were introduced.

### Remaining risks

- `futureSkew` intentionally shares `replay_window_seconds`; a separate setting
  may be considered later but is outside this phase.
- Increasing the replay window increases replay-cache retention and memory use
  proportionally.

## Security Review: Phase 1

### Findings

- [x] High: hot reload could widen the accepted timestamp window after older
  signatures had expired or been purged.
- [x] Medium: duration arithmetic could overflow to a short positive TTL.
- [x] Medium: maximum TTL retention unnecessarily doubled memory pressure for
  typical current-timestamp packets.
- [x] Low: tests did not exercise cleaner behavior or concurrent `processKnock`
  execution.
- [x] Low: explicit YAML zero was treated as an omitted replay-window value.

### Corrections

- Replay-window changes are preserved at the active value during hot reload and
  require daemon restart.
- Configuration accepts only `replay_window_seconds` values from 1 to 3600.
- Replay expiration is calculated from each payload timestamp, reducing normal
  retention while still covering future skew.
- Tests cover configuration bounds, hot reload preservation, cleaner timing,
  invalid-timestamp non-execution, and concurrent single execution.
- LoadConfig tests distinguish an omitted replay window from explicit zero.

### Lessons added

- None. The existing writable `GOCACHE` rule was applied successfully.

## Result: Phase 2 — Validation before hooks

### Changes made

- Added shared key/value validation before any executor log or hook.
- Added defense-in-depth validation in `RunHook` and `runCommand`.
- Replaced required-parameter regex parsing with `text/template` AST analysis.
- Rejected dynamic `Params` access, root-context rendering, and environment collisions.
- Reserved `otp` for TOTP and validated collisions before extracting it.
- Added case-insensitive sensitive-parameter redaction for command and hook output.
- Cloned params before asynchronous post-hooks and reverts.
- Updated README, user guide, and example configuration.

### Tests executed

```bash
GOCACHE=/tmp/ghostknock-go-build go test ./... -count=1
GOCACHE=/tmp/ghostknock-go-build go test -race ./... -count=1
GOCACHE=/tmp/ghostknock-go-build go test -race ./internal/config ./internal/executor ./cmd/ghostknockd -count=100
GOCACHE=/tmp/ghostknock-go-build go vet ./...
GOCACHE=/tmp/ghostknock-go-build make build
gosec ./...
govulncheck ./...
```

### Test results

- Global tests and race tests: pass.
- Config, executor, and daemon tests: 100 race-enabled repetitions pass.
- Clean build: pass.
- Vet retains only the pre-existing IPv6 client finding.
- Gosec retains 12 pre-existing findings and reports none in the Phase 2 validation code.
- Govulncheck retains `GO-2026-4971` from the installed Go 1.25.5 toolchain.

### Security review

The expert review found and the implementation corrected:

- required-param bypasses using valid template syntax;
- sensitive-param leaks caused by case differences;
- `otp`/`OTP` environment collisions;
- whole-root template rendering and aliases;
- forgeable required-param metadata in programmatic actions;
- mutable params observed by asynchronous hooks/reverts.

Final expert review found no residual defects introduced by Phase 2.

### Remaining pre-existing risks

- Legacy command execution still uses `/bin/sh -c`.
- Post-hook and revert goroutines are not managed by the server execution semaphore.
- Revert scheduling uses in-memory `time.Sleep`.
- Revert-hook errors are logged by `RunHook` but ignored by the caller.
- The installed Go 1.25.5 toolchain should be upgraded to at least Go 1.25.10.

### Lessons added

- Parse structured languages through their AST rather than regex matching.
