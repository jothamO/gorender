# Changelog

All notable changes to this project will be documented in this file.

The format is based on Keep a Changelog and this project follows Semantic Versioning.

## [Unreleased]

### Added

- Open-source planning track under `opensource/`.
- Lightweight charter and performance budget documents.
- Frontend contract, compatibility matrix, presets/duration, and troubleshooting docs.
- Draft governance set: CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, SUPPORT.
- CI workflow drafts promoted to `.github/workflows`:
  - `ci.yml`
  - `parity-gate.yml`
  - `benchmark-regression.yml`
  - `release.yml`

### Changed

- Root README rewritten to lightweight-first core positioning.
- `render` now always uses the main runtime path (removed user-facing `--experimental-pipeline` toggle).
- `bench` command now supports duration controls and defaults to `--no-audio-discovery` for more stable benchmark runs.

## [0.1.0] - 2026-03-10

### Added

- Core renderer CLI commands (`render`, `bench`, `parity`, `check`).
- Presets and duration-source model (`auto|manual|fixed`).
- Experimental runtime as default engine with preset bundles.
- Audio discovery/mux flow and parity metric tooling (SSIM/PSNR).
