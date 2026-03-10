# Optional UI Implementation Plan

## Objective

Provide a non-terminal user experience while keeping core renderer behavior and footprint unchanged by default.

## Architecture

- Frontend: lightweight static assets (no framework requirement)
- Backend: existing render API served by `gorendersd`
- Core CLI remains unchanged and independent

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

## Phased Delivery

1. Phase A: UX spec and API contract validation
2. Phase B: implementation with smoke tests
3. Phase C: isolation audit + release gate integration

## Acceptance Criteria

- Core CLI binary size/startup unchanged within budget when UI is disabled.
- Render/parity outputs identical with and without UI module installed.
- UI path can submit jobs, track progress, and download outputs reliably.

