# Phase 1 Security Review

## Scope

Independent expert review and local adversarial testing of timestamp freshness,
replay expiration, cache concurrency, cleaner behavior, and hot reload.

## Confirmed Phase 1 findings

### High — Replay window increase during hot reload

An increased window could make packets acceptable after their signatures had
already expired or been purged.

Resolution:

- `replay_window_seconds` changes are not applied during hot reload.
- The active value is preserved and the daemon logs that restart is required.

### Medium — Positive duration overflow

Duration arithmetic could wrap to a short positive value, bypassing a
non-positive overflow check.

Resolution:

- Configuration accepts only values from 1 to 3600 seconds.
- Runtime conversion independently rejects out-of-range values.
- Per-packet expiration replaces additive maximum-TTL arithmetic.

### Medium — Unnecessary replay-cache retention

Using the maximum symmetric TTL retained normal current-timestamp packets twice
as long as necessary.

Resolution:

- Expiration is calculated as `payload timestamp + past window + guard`.
- A minimum one-second retention protects accepted packets at the old boundary.
- A future-boundary packet remains cached through its entire acceptance window.

### Low — Missing adversarial tests

Resolution:

- Added config-boundary tests.
- Added hot-reload preservation tests.
- Added cleaner timing tests.
- Added invalid-timestamp non-execution tests.
- Added concurrent `processKnock` single-execution tests.

### Low — Explicit zero bypassed the documented range

An explicit YAML value of zero was treated like an omitted field and replaced
with the default before validation.

Resolution:

- YAML decoding records whether `replay_window_seconds` was present.
- Omitted values receive the default; explicit zero is rejected.
- `LoadConfig` tests use temporary keys and configuration files.

## Verification

```bash
GOCACHE=/tmp/ghostknock-go-build go test ./... -count=1
GOCACHE=/tmp/ghostknock-go-build go test -race ./... -count=1
GOCACHE=/tmp/ghostknock-go-build go test -race ./cmd/ghostknockd \
  -run 'TestConcurrentProcessKnockExecutesOnce|TestReplayCheckAndStoreIsAtomic|TestReplayCleanerKeepsSignatureThroughAcceptanceWindow' \
  -count=100
GOCACHE=/tmp/ghostknock-go-build go vet ./...
make build
gosec ./...
govulncheck ./...
```

Results:

- Tests and race tests pass.
- The focused concurrency tests pass 100 repetitions under the race detector.
- Build passes.
- Vet retains the pre-existing IPv6 address-format finding in the client.
- Gosec reports 12 pre-existing findings; none point to the Phase 1 additions.

## Pre-existing external vulnerability

`govulncheck` reports `GO-2026-4971` in the installed Go 1.25.5 standard
library. The Windows client reaches the affected `net.Dial` symbol. The issue is
fixed in Go 1.25.10 and should be addressed through the build toolchain rather
than mixed into the replay fix.

## Residual risk

- The replay cache remains unbounded. The 3600-second maximum and per-packet
  expiration reduce exposure, but a cache capacity policy cannot safely evict
  signatures without a broader replay-state design.
- Restart clears in-memory replay state, as it did before Phase 1.
