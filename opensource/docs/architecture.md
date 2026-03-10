# Architecture (Core Runtime)

`gorender` core is a Go-native render pipeline that captures deterministic browser frames and pipes them directly into FFmpeg.

## Flow

1. CLI parses composition + runtime options.
2. Duration resolution picks frame count:
   - explicit `--frames`
   - `--duration-source manual`
   - `--duration-source auto` via page metadata
   - fixed fallback (`slides * seconds-per-slide`)
3. Browser pool launches persistent Chrome workers.
4. Renderer seeks frame `N`, waits for ready signals.
5. Frames are captured (PNG/JPEG) and reordered.
6. FFmpeg encodes stream, optionally inline-upscales.
7. Audio tracks are auto-discovered/muxed when enabled.
8. Output file is written with progress and metrics.

## Core Components

- `cmd/gorender/main.go`: CLI and flag/option wiring.
- `render.go`: orchestration and runtime option handling.
- `internal/browser`: browser pool lifecycle and health.
- `internal/pipeline`: frame rendering, buffering, ordering.
- `internal/ffmpeg`: encode/upscale/mux pipeline.
- `internal/presets`: parameter bundles for runtime tuning.
- `internal/composition`: composition schema and duration helpers.

## Runtime Principles

- Framework-agnostic: any page can render if it honors the contract.
- Deterministic: frame-index-driven state, not wall-clock timing.
- Lightweight core: no framework runtime dependency in engine.
- Tunable: presets and explicit CLI flags for speed/quality tradeoffs.

