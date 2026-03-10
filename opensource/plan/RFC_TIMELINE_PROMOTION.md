# RFC: Timeline Module Promotion (Phase 1)

## Status

- Date: 2026-03-10
- Owner: Codex
- Stage: Approved and closed (guarded/optional), evidence captured for current workload

## Goal

Promote the isolated deterministic timeline primitive into core math paths
without changing runtime behavior.

## Scope (Phase 1)

- Wire `internal/composition.ComputeTotalFramesFromDurationsMs` to use:
  - `opensource/engine/timeline.New`
  - `Timeline.TotalFrames`
- Preserve existing semantics:
  - validate `durations > 0`
  - validate `fps > 0`
  - compute frame count with `ceil(totalMs * fps / 1000)`
- Add core adapter for frame-location mapping:
  - `internal/composition.LocateFrameInDurations`
  - backed by `Timeline.LocateFrame`
- Promote guarded execution wiring in renderer:
  - emit deterministic query hints (`gr_*`) from adapter
  - inject `window.__GORENDER_TIMELINE__` per frame during render path

## Why This Is Safe

- Call site behavior remains unchanged.
- Existing duration tests remain green.
- New test added for fractional frame rounding (`1001ms @ 30fps => 31`).
- No CLI flags or runtime defaults changed.

## Next Phase

- Keep `timeline-resolver` optional and OFF by default in `render`.
- Revisit guard removal only after broader multi-workload evidence.

## Evidence (User Run)

- Command mode: `parity --parity-runs 3 --timeline-resolver`
- Workload context: MakeMoments URL with 4 slides
- Result:
  - `baseline (median): 1m50.796s`
  - `experimental (median): 1m36.231s`
  - `speedup (median): 13.15%`
  - `ssim(all, worst): 0.998696`
  - `psnr(avg, worst): 50.284 dB`
  - Speed gate status at `--target-speedup 0.20`: failed (quality passed)

Decision recorded:

- For now, keep `timeline-resolver` optional (guard OFF by default), as agreed.

## Approval

- Approved by project owner on 2026-03-10.
- RFC closed with current decision:
  - Keep guarded optional timeline resolver behavior.
  - Do not promote to default runtime behavior yet.
