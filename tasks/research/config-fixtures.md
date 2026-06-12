# Configuration Fixture Audit

## Scope

Phase 0 audit of automated tests, YAML files used for testing, credential
handling, filesystem writes, and root requirements.

## Initial findings

- The repository currently has fuzz tests in `internal/listener` and
  `internal/protocol`.
- Those tests construct inputs in memory and do not load `config.yaml`,
  `config.yaml.example`, or `test.yaml`.
- No automated config-loader tests or reusable config fixtures currently exist.
- `test.yaml` is a manual end-to-end example. It references `./server_key`, has a
  fixed public key, writes through an action to `/tmp/prueba.txt`, and therefore
  must not be used as an automated unit-test fixture.
- The existing automated tests do not require real keys, write to system paths,
  or require root.

## Phase 0 decision

Do not add a fixture solely for unused coverage. Future config tests must create
their private-key file under `t.TempDir()`, use a generated or structurally valid
public key, define a harmless action, and avoid privileged listeners or system
paths.

## Follow-up for later phases

- Timestamp and replay tests should isolate time and cryptographic setup in
  test helpers rather than loading example YAML.
- Hook tests should create executable scripts under `t.TempDir()`.
- Live PCAP tests must remain optional integration tests because they may require
  native capabilities or root.
