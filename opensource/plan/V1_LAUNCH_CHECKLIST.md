# v1 Launch Checklist (Draft)

## Documentation

- [x] Core README is lightweight-first and accurate.
- [x] Frontend contract doc finalized.
- [x] Troubleshooting + compatibility matrix published.

## Governance

- [x] CONTRIBUTING published.
- [x] CODE_OF_CONDUCT published.
- [x] SECURITY policy published.
- [x] SUPPORT policy published.

## Quality

- [ ] CI matrix green.
- [x] Parity gate workflow validated.
- [x] Benchmark workflow validated.

## Release

- [x] Version/tag policy applied.
- [x] Changelog finalized.
- [x] Binaries + checksums generated.
- [x] Release notes published.

## Evidence

- Workflow files promoted to `.github/workflows/` (`ci.yml`, `parity-gate.yml`, `benchmark-regression.yml`, `release.yml`).
- Local parity validation passed:
  - URL: `http://127.0.0.1:8080/moments-mm57lkdh`
  - Output: `speedup 36.34%`, `SSIM 0.999366`, `PSNR 55.224 dB`
- Release artifacts + checksums:
  - `opensource/releases/v1.0.0-rc1/artifacts/`
  - `SHA256SUMS.txt`
- Changelog:
  - `CHANGELOG.md`
- Release notes:
  - `opensource/releases/v1.0.0-rc1/RELEASE_NOTES.md`

## Remaining Blockers

- CI matrix has not yet completed a real GitHub Actions run.
- None beyond CI matrix run status.

## Latest Validation Commands

- Parity:
  - `.\bin\gorender.exe parity --url http://127.0.0.1:8080/moments-mm57lkdh --frames 60 --fps 30 --preset final --workers 1 --no-audio-discovery --target-speedup 0 --min-ssim 0.90 --min-psnr 20 -v`
- Benchmark:
  - `.\bin\gorender.exe bench --url http://127.0.0.1:8080/moments-mm57lkdh --frames 30 --fps 30 --width 720 --height 1280 --runs 1 --workers 1 --preset final --no-audio-discovery --continue-on-error -v`
