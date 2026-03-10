# gorender vs Revideo vs Remotion (Feature Comparison)

As of: 2026-03-10

## Executive Snapshot

- `gorender` is currently strongest on your target use case: deterministic, server-side web capture with strong speed/parity tuning and a single Go binary workflow.
- `Remotion` is strongest as a full platform/ecosystem (Studio, Player, Lambda/Cloud options, broader output variants, mature docs/community).
- `Revideo` is strong as an open-source TypeScript library stack with player + renderer + production deployment patterns, but with a different runtime model (browser/WebCodecs-centered rendering flow).

## Scorecard (Current State)

Scale: `1` (weak) to `5` (strong), based on current implemented/documented capabilities.

| Area | gorender | Revideo | Remotion |
|---|---:|---:|---:|
| Raw server render speed tuning | 5 | 4 | 4 |
| Deterministic parity tooling (SSIM/PSNR gate) | 5 | 2 | 3 |
| Framework agnosticism (URL contract) | 5 | 3 | 2 |
| Distributed/cloud rendering out of the box | 2 | 3 | 5 |
| Preview/player/editor ecosystem | 1 | 4 | 5 |
| Output format breadth (GIF/still/sequence/audio-only/transparent) | 2 | 3 | 5 |
| Maturity of docs + ecosystem | 2 | 3 | 5 |
| Licensing flexibility for companies | 5 | 5 | 3 |

## Feature Matrix

| Capability | gorender | Revideo | Remotion | Where gorender stands |
|---|---|---|---|---|
| Core render model | Headless Chrome pool + Go scheduler + FFmpeg pipe | Browser-side frame draw/WebCodecs + backend audio/FFmpeg | React composition runtime + CLI/SSR/cloud options | Ahead on low-level render control; behind in productized UX |
| Runtime default | Experimental engine is now default; presets layer on top | TS/Node stack | TS/Node stack | Ahead for binary ops simplicity |
| Parallelism | Multi-worker pool; frame chunking/work balancing | Worker/partial rendering documented | Local + distributed Lambda rendering | Competitive locally; behind on built-in distributed orchestration |
| Auto duration contract | `--duration-source auto/manual/fixed`, page metadata contract | Scene/project driven timeline | Composition-driven timeline | Ahead for URL-page contract flexibility |
| Audio | Auto-discovery + mux into output | Backend audio extraction + merge | Rich audio workflows and variants | Competitive for current story use case |
| Quality gates | Built-in parity command with speed + SSIM + PSNR thresholds | Not documented as first-class CLI parity gate | Quality tooling exists, but not this exact integrated gate | Ahead |
| Resolution strategy | Render-size + optional upscale controls (`--no-upscale`, explicit targets) | Standard render settings | Output scaling and many encoding options | Competitive |
| Output variants | MP4 focus today | MP4-focused workflow | Video, still, sequence, GIF, transparent, audio-only | Behind |
| Embedded player | None native | `@revideo/player-react` | `@remotion/player` | Behind |
| Cloud product | No first-class managed/distributed service yet | Platform + deployment guides | Lambda product + Cloud Run alpha + GH Actions path | Behind |
| License | Repo currently open-source style (project-controlled) | MIT | Source-available with company licensing tiers | Ahead vs Remotion for permissive usage |

## Where gorender is Ahead Right Now

- Deterministic parity workflow integrated into CLI (`parity` with speedup + SSIM + PSNR gates).
- High practical speed on your current workload with preset-driven tuning and 720->1080 upscale path.
- Framework-agnostic render contract (render any web app by URL + ready/frame signals).
- Operational simplicity (Go binary, no Node runtime required for renderer core).

## Where gorender is Behind

- No native preview/player/editor experience.
- No first-class distributed rendering product (Lambda/Cloud orchestration equivalent).
- Smaller docs/community footprint and fewer turnkey deployment recipes.
- Narrower output/export variants compared with Remotion.

## What To Build Next (Priority Order)

1. `P0` Distributed render mode: shard frames across multiple nodes/instances, merge centrally.
2. `P0` Player/preview SDK (at least embeddable progress + seek + param preview).
3. `P0` Output variants: stills, sequence, GIF, transparent video, audio-only.
4. `P1` Job API + queue hardening (idempotency, retries, webhooks, tenancy limits).
5. `P1` Observability pack (trace spans for capture/encode/mux, per-job diagnostics export).
6. `P1` Golden visual regression suite for transition/position/font parity.
7. `P2` Public examples/templates (Next.js, plain React, Vue) and one-click deployment guides.

## Bottom Line

For your current `makemoments` pipeline, `gorender` is already competitive-to-ahead on speed and deterministic parity control.  
For broader open-source market parity with Revideo/Remotion, the biggest gaps are ecosystem product surface (player/editor/cloud distribution), not core rendering mechanics.

## Sources

External:
- Remotion homepage: https://www.remotion.dev/
- Remotion render docs: https://www.remotion.dev/docs/render
- Remotion Lambda: https://www.remotion.dev/lambda
- Remotion Player: https://www.remotion.dev/player
- Remotion license: https://remotion.dev/license
- Revideo docs home: https://docs.re.video/
- Revideo rendering docs: https://docs.re.video/rendering-videos/
- Revideo repo (MIT, feature notes): https://github.com/redotvideo/revideo

Local gorender references:
- [README.md](/C:/Users/Evelyn/Documents/gorender/README.md)
- [docs/profiles.md](/C:/Users/Evelyn/Documents/gorender/docs/profiles.md)
- [cmd/gorender/main.go](/C:/Users/Evelyn/Documents/gorender/cmd/gorender/main.go)
- [render.go](/C:/Users/Evelyn/Documents/gorender/render.go)
- [internal/presets/presets.go](/C:/Users/Evelyn/Documents/gorender/internal/presets/presets.go)
