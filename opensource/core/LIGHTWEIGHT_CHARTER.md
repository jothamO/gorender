# Lightweight Charter

## Mission

`gorender` core must be functionally strong and operationally lightweight:

- single Go CLI binary for rendering workflows
- framework-agnostic web contract
- deterministic output/parity controls
- minimal default runtime and dependency surface

## Non-Negotiables

1. Core is framework-agnostic.
2. Core does not require React, Node, or any framework runtime.
3. Optional modules (GUI/templates/adapters) are isolated from core.
4. Performance and parity are measured, not assumed.
5. Backward compatibility of stable CLI behavior is maintained intentionally.

## Core Scope (v1)

- Commands: `render`, `parity`, `bench`, `check`
- Contract: `?frame=N`, `window.__READY__`, `window.__FRAME_READY__`, `window.__GORENDER_META__`
- Duration resolution: `auto|manual|fixed`
- Presets as parameter bundles over the default runtime engine
- Audio discovery + muxing
- Parity gate: speedup + SSIM + PSNR

## Out Of Core Scope (Optional Track)

- Embedded GUI
- Framework-specific scaffolding packs
- Cloud orchestration product surface
- Rich editor/player experiences

## Design Principles

- Prefer boring primitives over heavy abstractions.
- Add knobs only when they unlock measurable value.
- Keep default path fast and simple; advanced behavior opt-in.
- Every feature should have a removal story if it adds weight without clear gains.
