# Gorender Performance Checklist

Last updated: 2026-03-09

## Status

- [x] Set viewport once per worker session
  - Implemented in `internal/pipeline/renderer.go` during worker session initialization.

- [ ] Skip Framer Motion runtime in render mode (full)
  - Reverted static DOM wrapper approach because it broke interpolation/position parity.
  - Remaining work: keep deterministic parity while removing Framer runtime overhead safely.

- [x] Frame-ready contract upgrade
  - Implemented `window.__FRAME_READY__ === frame` checks (with fallback to `__READY__`).

- [x] Capture JPEG in fast mode
  - Implemented browser capture format support (`png`/`jpeg`).
  - Default behavior: `fast` profile uses JPEG unless overridden.
  - CLI override: `--capture-format png|jpeg`.

- [x] Profile split: capture vs encode timing counters
  - Browser split added: seek, ready wait, screenshot totals + averages.
  - FFmpeg split added: encode, upscale, mux, total.

- [x] Keep `frame-step=1` for parity, use `>1` for iteration
  - Available and documented by `--frame-step`.

## Recent defaults

- Inline render defaults now target `720x1280` render dimensions and auto-upscale to `1080x1920` output.
