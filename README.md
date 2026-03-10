# gorender

Fast, framework-agnostic video renderer for web compositions.

`gorender` is a Go-based renderer that captures deterministic browser frames and streams them directly into FFmpeg.  
Core goal: keep runtime powerful but super lightweight.

## Core Promise

- Single Go binary workflow for rendering.
- Framework-agnostic frontend contract (not React-locked).
- Deterministic frame rendering and parity validation.
- Presets for speed/quality tuning on top of one runtime engine.

## What Core Includes

- `render`: render composition to video
- `parity`: baseline vs candidate quality/speed gate (SSIM/PSNR + speedup)
- `bench`: repeat-runs throughput checks
- `check`: dependency validation

## What Core Does Not Require

- React runtime
- Node runtime for the renderer core
- Cloud provider-specific infrastructure

Optional surfaces (UI, template packs, ecosystem adapters) are tracked separately in [`opensource/`](./opensource/README.md).

## Prerequisites

- Go 1.22+
- Chrome/Chromium
- `ffmpeg` and `ffprobe`

## Build

```bash
go build -o ./bin/gorender ./cmd/gorender
```

## Quick Start

### Render from inline flags

```bash
./bin/gorender render \
  --url http://localhost:3000/comp \
  --frames 300 \
  --fps 30 \
  --out ./output/output.mp4
```

### Render with preset + auto duration

```bash
./bin/gorender render \
  --url http://localhost:8080/moments-abc123 \
  --duration-source auto \
  --preset final \
  --workers 2 \
  --out ./output/final.mp4 -v
```

### Run parity gate

```bash
./bin/gorender parity \
  --url http://localhost:8080/moments-abc123 \
  --duration-source auto \
  --preset final \
  --workers 2 \
  --target-speedup 0.30 \
  --min-ssim 0.995 \
  --min-psnr 40 -v
```

## Frontend Contract

Your page must render from `?frame=N` and signal readiness.

Required:

- `window.__READY__ = true`
- `window.__FRAME_READY__ = N`

Recommended for auto-duration:

- `window.__GORENDER_META__ = { version, status, slideDurationsMs, totalDurationMs }`

Detailed contract: [`opensource/docs/frontend-contract.md`](./opensource/docs/frontend-contract.md)

## Key Runtime Features

- Persistent Chrome worker pool
- Work-stealing scheduling
- Reorder buffer for in-order encode
- Direct FFmpeg piping
- Audio discovery + mux support
- Duration-source resolution (`auto|manual|fixed`)
- Render-size + upscale controls

## Documentation

- Profiles and sizing: [`docs/profiles.md`](./docs/profiles.md)
- Open-source execution track: [`opensource/plan/OPENSOURCE_EXECUTION_PLAN.md`](./opensource/plan/OPENSOURCE_EXECUTION_PLAN.md)
- Lightweight charter: [`opensource/core/LIGHTWEIGHT_CHARTER.md`](./opensource/core/LIGHTWEIGHT_CHARTER.md)
- Troubleshooting draft: [`opensource/docs/troubleshooting.md`](./opensource/docs/troubleshooting.md)

## Roadmap (High Level)

- Distributed render mode
- Optional UI and template packs (isolated from core)
- Release automation and checksums
- Expanded output variants

