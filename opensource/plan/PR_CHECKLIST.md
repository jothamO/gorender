# PR Checklist (Draft)

Use this checklist on every PR.

## Core Requirements

- [ ] Scope is clear and minimal.
- [ ] Core remains framework-agnostic.
- [ ] No unintended dependency/runtime weight added to core.
- [ ] Tests updated or added as needed.
- [ ] Docs updated for user-visible changes.

## Reliability

- [ ] `go test ./...` passes.
- [ ] Smoke render passes.
- [ ] Failure modes considered and logged clearly.

## Performance/Parity (if applicable)

- [ ] Baseline and candidate timings attached.
- [ ] SSIM/PSNR results attached or justified.
- [ ] Visual spot-check outcome noted.

## Release Safety

- [ ] Backward compatibility impact stated.
- [ ] Flags/CLI behavior changes documented.

