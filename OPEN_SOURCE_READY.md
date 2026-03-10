# Open Source Production Readiness Checklist

Use this checklist to prepare `gorender` for a production-grade public GitHub release.

## 1. Repository Hygiene

- [ ] Clear repository name, description, and GitHub topics.
- [ ] Consistent module path (`github.com/<org>/gorender`) and import cleanup.
- [ ] Remove private/internal URLs, secrets, and machine-specific defaults.
- [ ] Clean root structure (`cmd/`, `internal/`, `pkg/` if needed, `docs/`, `examples/`).

## 2. README.md (Production Grade)

- [ ] One-line value proposition.
- [ ] Features and non-goals section.
- [ ] Quickstart in 2-5 commands.
- [ ] Supported OS/dependencies matrix (Go, Chrome/Chromium, ffmpeg).
- [ ] Real CLI examples (local URL, remote URL, fast vs final).
- [ ] Troubleshooting section (timeouts, Chrome startup, audio, fps/parity).
- [ ] Performance notes and benchmark method.
- [ ] Links to full docs.

## 3. Documentation Set

- [ ] `docs/architecture.md` (pipeline, workers, frame flow, ffmpeg flow).
- [ ] `docs/configuration.md` (all flags and env vars with defaults).
- [ ] `docs/profiles.md` (parity/final/fast tradeoffs).
  - Current state: expanded with preset strategy, duration-source flags, and render/upscale control model.
- [ ] `docs/faq.md`.
- [ ] `docs/contributing.md` with local dev workflow.

## 4. Licensing and Legal

- [ ] `LICENSE` (MIT/Apache-2.0/etc).
- [ ] `NOTICE` if required.
- [ ] Third-party attribution list.
- [ ] Trademark/name usage note (if applicable).

## 5. Community and Governance

- [ ] `CODE_OF_CONDUCT.md`.
- [ ] `CONTRIBUTING.md`.
- [ ] `SECURITY.md` with vulnerability reporting instructions.
- [ ] GitHub issue templates (bug, feature, question).
- [ ] Pull request template with required checks.
- [ ] `CODEOWNERS`.

## 6. Quality Gates (CI)

- [ ] CI on PR and main for build, tests, lint, format, vet.
- [ ] Cross-platform matrix (Windows, Linux, macOS).
- [ ] Smoke render test fixture with deterministic assertions.
- [ ] Required status checks before merge.

## 7. Testing Depth

- [ ] Unit tests for composition parsing, scheduler, frame ordering, ffmpeg command generation.
- [ ] Integration tests for browser pool lifecycle and retry behavior.
- [ ] Golden tests for transition/interpolation parity.
- [ ] Regression tests for known failure modes (timeouts, panic on closed channel, retry loops).

## 8. Security and Supply Chain

- [ ] Dependabot or Renovate enabled.
- [ ] `govulncheck` in CI.
- [ ] Pin GitHub Action versions.
- [ ] Minimal GitHub Actions permissions.
- [ ] Release checksums and signed artifacts.

## 9. Release and Versioning

- [ ] Semantic versioning policy.
- [ ] `CHANGELOG.md` (Keep a Changelog format).
- [ ] Automated tagged releases with OS/arch binaries.
- [ ] Install instructions per release artifact.

## 10. Developer Experience

- [ ] `Makefile` or `Taskfile` (`build`, `test`, `lint`, `smoke`).
- [ ] Example compositions and sample media.
- [ ] `.editorconfig`, lint config, and formatting rules.
- [ ] Logging levels and verbosity behavior documented.

## 11. Operational Readiness

- [ ] Safe default config for low-resource machines.
- [ ] Backpressure, timeout, and retry behavior documented.
- [ ] Observability for progress/timing/error classes.
- [ ] Consistent exit codes and error message standards.

## 12. GitHub Project Presentation

- [ ] Strong About text and short demo GIF/video.
- [ ] Discussion categories configured.
- [ ] Public roadmap/milestones.
- [ ] `good first issue` labels and onboarding issues.
