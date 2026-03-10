# Render Profiles And Sizing

This document defines profile behavior, preset direction, and output sizing controls for `gorender`.

## Goals

- Keep strong visual parity with frontend playback.
- Make speed/quality/size tradeoffs explicit and repeatable.
- Allow future presets to be updated without rewriting core pipeline logic.

## Runtime engine

`gorender` now runs with the experimental runtime engine by default.

- Presets are parameter bundles on top of this engine.
- `render` always uses this main runtime path.
- `parity` still runs internal baseline-vs-main A/B for validation.

## Current Profiles

`gorender` now supports named presets via `--preset` in addition to legacy `--profile`.

## CLI controls added

- `--preset <name>`
- `--duration-source auto|manual|fixed`
- `--slide-durations-ms <csv>`
- `--default-slide-ms <int>`
- `--upscale-width <int>`
- `--upscale-height <int>`
- `--no-upscale`
- `--warmup` / `--warmup-frame` (render prewarm)
- `warmup` command (standalone warm start)

## Resolution precedence

- explicit CLI flags
- preset defaults (`--preset`, or `--profile` alias when preset is omitted)
- composition defaults

## `final`

- Intended for production parity output.
- Defaults:
  - `frame-step=1`
  - experimental pipeline enabled
  - JPEG capture (quality 90) + single-pass upscale
  - `veryfast` encoder preset with balanced CRF

## `fast`

- Intended for iteration and debugging.
- Defaults:
  - faster encoding
  - lower quality targets
  - usually `jpeg` capture

## Experimental Pipeline (Main Runtime)

Current experimental optimizations:

- optional JPEG capture with higher quality in final-profile experiments
- single-pass encode+upscale (avoids second full upscale encode pass)
- reduced memory copying in reorder buffering

Validation path:

- use `gorender parity` to compare baseline vs experimental
- gate by speedup + SSIM + PSNR thresholds

## Speed-First Balanced Preset (Proposed)

Purpose:

- keep most speed gains from experimental path
- reduce file size growth compared to pure speed-max settings
- preserve parity guardrails

Candidate settings:

- `capture-format=jpeg`
- `jpeg-quality=88-90`
- `frame-step=1`
- encoder preset `veryfast`
- `crf=21` (or `22` for smaller output)
- single-pass upscale enabled

## Future Preset Catalog (Planned)

- `parity-strict`: strongest reproducibility and regression safety.
- `final`: stable production parity default.
- `fast`: quick iteration profile.
- `speed-balanced`: speed-first balanced profile.
- `speed-max`: maximum throughput profile.
- `production-balanced`: recommended server default for speed/size/quality.
- `production-fast`: throughput first for large queues.
- `preview`: low-cost quick previews.
- `draft`: creator/dev iteration mode.
- `social-reel`: mobile/social export compatibility focused.
- `archive-master`: high-quality mezzanine output.
- `low-bandwidth`: aggressive size constraints.
- `cpu-constrained`: stable behavior on weaker VPS machines.
- `gpu-encode`: hardware-accelerated encode path (where supported).
- `deterministic-ci`: CI-safe reproducibility profile.
- `debug-trace`: diagnostic profile with verbose timings and retained intermediates.

## Sizing Model

Two independent size concepts:

- Render size: browser viewport and frame capture size (`--width`, `--height`).
- Output size: final encoded dimensions (can match render size or be upscaled).

Current behavior:

- inline defaults render at `720x1280`
- output may be upscaled to `1080x1920` when configured

## Proposed explicit size controls

- `--width` / `--height`: logical render size
- `--upscale-width` / `--upscale-height`: final output target size
- `--no-upscale`: force disable upscale even if preset defines one

Expected precedence:

- Explicit CLI flags override preset defaults.
- If upscale target equals render size, upscale is skipped automatically.
- If `--no-upscale` is set, final output stays at render size.

## Recommended usage patterns

- Parity testing: `final` + `frame-step=1`
- Performance A/B: `parity` command with speed + SSIM/PSNR gates
- Iteration: `fast` profile and reduced frame count
- Production rollout of new preset: validate with parity command before making default

## Warm Start

Use warmup when you want to reduce cold-start overhead before a render burst.

Examples:

```powershell
.\bin\gorender.exe warmup --url http://127.0.0.1:8080/moments-abc --workers 2 --ready-timeout 20s -v
```

```powershell
.\bin\gorender.exe render --url http://127.0.0.1:8080/moments-abc --duration-source auto --preset final --workers 2 --warmup --warmup-frame 0 --out .\output\final.mp4 -v
```
