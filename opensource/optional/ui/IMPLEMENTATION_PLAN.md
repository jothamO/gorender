# Optional UI Implementation Plan

## Objective

Provide a non-terminal user experience while keeping core renderer behavior and footprint unchanged by default.

## Scope Split

1. Strict preview debugger (core surface, now):
- Keep `gorender preview` focused on deterministic frame/debug workflows.
- No smooth playback requirements in this surface.

2. Smooth player UI (optional surface, next):
- Build as isolated UI module that talks to existing APIs.
- Do not introduce dependencies into core CLI execution path.

## Architecture (Optional Smooth Player)

- Frontend: lightweight static assets (no framework requirement)
- Backend: existing render API served by `gorendersd`
- Core CLI remains unchanged and independent

Prototype references (do not promote directly without isolation review):
- `.ignore/files (2)/index.html`
- `.ignore/files (2)/ui.go`
- `.ignore/files (2)/composition.html`
- `.ignore/files (2)/Composition.jsx`
- `.ignore/files (2)/Composition.svelte`

## Packaging Options

1. Separate binary (preferred)
- `gorender` for core CLI
- `gorendersd` for server/UI features

2. Build tag
- UI assets compiled only when explicitly enabled at build time

## Guardrails

- UI code must not introduce runtime dependencies into core CLI path.
- UI availability must not alter default flags or render outcomes.
- UI-specific config must be namespaced and isolated from core command flags.
- Smooth-player state, polling, and UX code must live outside `internal/pipeline/*`.

## Delivery Phases

1. Phase A: strict debugger stability lock (core)
- Freeze strict preview semantics.
- Add regression checks for frame URL generation + control behavior.

2. Phase B: smooth-player module scaffold (optional)
- Serve static assets via isolated module (`ui` package or separate repo).
- Implement job list/progress/download flows against existing server API.

3. Phase C: smooth-player UX + controls
- Timeline scrubber, play/pause UX, render queue view, error surfaces.
- Do not add new render semantics to core for player convenience.

4. Phase D: isolation + release gating
- Verify no binary-size/startup/perf drift in core when UI disabled.
- Add optional smoke checks in CI behind explicit job matrix.

## Acceptance Criteria

- Core CLI binary size/startup unchanged within budget when UI is disabled.
- Render/parity outputs identical with and without UI module installed.
- UI path can submit jobs, track progress, and download outputs reliably.
- Smooth-player module can be versioned independently from core if needed.
