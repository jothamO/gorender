# gorender v1.0.0-rc1 Release Notes (Draft)

Release date: 2026-03-10  
Tag target: `v1.0.0-rc1`

## Summary

This release candidate packages the lightweight-first open-source baseline:

- core renderer remains framework-agnostic and Go-native
- open-source governance/process docs are in place
- CI/parity/benchmark/release workflows are added for GitHub Actions

## Highlights

- Preset-driven render runtime with duration-source resolution.
- Parity tooling (`SSIM` / `PSNR` + speed targets) available via CLI.
- Open-source planning and release process scaffold completed.

## Included Artifacts

- `gorender` binaries
- `gorendersd` binaries
- SHA256 checksums file

## Known Gaps Before Stable v1

- CI matrix must be green in GitHub Actions (first full run pending).
- Parity and benchmark workflows require validated target URL in CI.
- Final changelog cut and stable tag (`v1.0.0`) still pending.

