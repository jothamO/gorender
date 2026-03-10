# Frontend Contract (Core)

This is the minimal browser-side contract required by `gorender` core.

## Required Signals

1. `?frame=N` query param must drive deterministic frame state.
2. `window.__READY__ = true` when the requested frame is fully painted.
3. `window.__FRAME_READY__ = N` for exact per-frame readiness confirmation.

## Optional Metadata (Recommended)

Expose page metadata for duration auto-discovery:

```js
window.__GORENDER_META__ = {
  version: 1,
  status: "ok", // or "not_found"
  slideDurationsMs: [5000, 3000, 7000],
  totalDurationMs: 15000
};
```

This powers `--duration-source auto`.

## Optional Runtime Timeline Context (Guarded)

When `--timeline-resolver` is enabled, gorender also provides deterministic
timeline context per frame:

- Query hints:
  - `gr_slide`
  - `gr_in_slide_ms`
  - `gr_slide_ms`
  - `gr_t`
- Runtime object:

```js
window.__GORENDER_TIMELINE__ = {
  frame: 151,
  fps: 30,
  globalMs: 5033,
  slide: 1,
  slideStartMs: 5000,
  inSlideMs: 33,
  slideMs: 2000,
  t: 0.0165
};
```

Frontends can consume this directly for deterministic transition math.

## Determinism Rules

- Do not use wall-clock timers (`Date.now`, `performance.now`) to drive animation state.
- Do not depend on `requestAnimationFrame` timing for render logic.
- Compute state only from frame/time derived from `N` and `fps`.
- Keep layout stable across runs (avoid viewport-dependent randomness).

## Minimal Vanilla Example

```html
<script>
  const q = new URLSearchParams(location.search);
  const frame = Number(q.get("frame") || 0);
  const fps = 30;
  const t = frame / fps;

  // Render deterministic state from t.
  document.body.style.opacity = String(Math.min(1, t / 1.0));

  Promise.resolve(document.fonts ? document.fonts.ready : undefined).finally(() => {
    window.__FRAME_READY__ = frame;
    window.__READY__ = true;
  });
</script>
```

## Not Found Behavior

If content does not exist, set:

```js
window.__GORENDER_META__ = {
  version: 1,
  status: "not_found",
  slideDurationsMs: [],
  totalDurationMs: 0
};
window.__FRAME_READY__ = Number(new URLSearchParams(location.search).get("frame") || 0);
window.__READY__ = true;
```

This avoids hangs and enables explicit CLI failure messaging.
