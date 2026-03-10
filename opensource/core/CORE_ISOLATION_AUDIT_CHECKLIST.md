# Core Isolation Audit Checklist

Use this checklist before promoting optional-surface changes or cutting a release candidate.

## Audit Metadata

- Date: 2026-03-10
- Auditor: Codex
- Candidate branch/commit: local workspace (docs/planning promotion set)
- Release target: Open-source planning baseline

## A) Dependency Isolation (Pass/Fail)

- [x] No new framework/runtime dependencies imported into core CLI path (`cmd/gorender`, `render.go`, `internal/*` core modules).
- [x] Optional module dependencies are scoped to optional surfaces only.
- [x] `go.mod` additions are justified and categorized (core vs optional).

Evidence:

- dependency diff attached: no `go.mod`/`go.sum` changes in this promotion set
- package ownership notes attached: changes are docs/process artifacts in `opensource/` and root `README.md`

## B) Build Isolation (Pass/Fail)

- [x] Core CLI builds successfully with optional modules disabled.
- [x] Optional modules can be built independently.
- [x] Build scripts clearly separate core and optional targets.

Commands run:

- `go build -o ./bin/gorender ./cmd/gorender`
- optional build command(s): `go build -o ./bin/gorendersd ./cmd/gorendersd`

## C) Runtime Behavior Isolation (Pass/Fail)

- [x] Default `gorender render` behavior unchanged.
- [x] Default presets/flags unchanged unless explicitly versioned/deprecated.
- [x] Optional module enablement does not alter core output determinism.

Checks:

- baseline command: n/a (docs/process-only change set)
- candidate command: n/a (docs/process-only change set)
- output comparison notes: no runtime code changes in this promotion set

## D) Performance Budget Isolation (Pass/Fail)

- [ ] Core mode startup remains within budget.
- [ ] Core render throughput remains within budget.
- [ ] No unexplained memory regression in core mode.

Metrics:

- baseline render duration: not collected in this shell session
- candidate render duration: not collected in this shell session
- delta: n/a

## E) Parity Quality Isolation (Pass/Fail)

- [ ] SSIM threshold met for affected scenarios.
- [ ] PSNR threshold met for affected scenarios.
- [ ] Visual spot-check confirms no unintended regressions.

Metrics:

- SSIM: not collected in this shell session
- PSNR: not collected in this shell session
- visual notes: n/a (docs/process-only change set)

## F) Documentation and Process Isolation (Pass/Fail)

- [x] Optional module docs clearly state disabled-by-default behavior.
- [x] Promotion checklist includes isolation evidence requirement.
- [x] Release notes classify optional changes separately from core.

## Final Decision

- [x] PASS: candidate may be promoted
- [ ] FAIL: remediation required before promotion

Blockers/Notes:

- `go test ./...` passed.
- Core binaries build succeeded for `gorender` and `gorendersd`.
- `gorender check` failed in this shell due missing PATH resolution for `ffmpeg/chrome`; environment-specific and does not reflect code regressions.
