# Compatibility Matrix (v1 Target)

This matrix defines what `gorender` open-source v1 intends to support.

## Runtime Dependencies

| Dependency | Required | Notes |
|---|---|---|
| Go | Yes (build from source) | Go 1.22+ recommended |
| Chrome/Chromium | Yes | Must be discoverable via PATH or configured binary |
| ffmpeg + ffprobe | Yes | Required for encode, parity metrics, and mux flows |

## Operating Systems

| OS | Status | Notes |
|---|---|---|
| Windows | Supported | Primary tested environment in this project |
| Linux | Supported target | Must validate in CI matrix |
| macOS | Supported target | Must validate in CI matrix |

## CLI Surface (Core)

| Command | Status |
|---|---|
| `render` | Stable target |
| `parity` | Stable target |
| `bench` | Stable target |
| `check` | Stable target |

## Frontend Contract

| Signal | Requirement |
|---|---|
| `?frame=N` | Required |
| `window.__READY__` | Required |
| `window.__FRAME_READY__` | Required (recommended hard requirement for deterministic mode) |
| `window.__GORENDER_META__` | Optional but recommended for auto duration |

## Out of v1 Core

- Built-in editor/player UX
- Managed cloud orchestration
- Rich export variants beyond current MP4-focused path

