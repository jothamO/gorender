# Open-Source Engine Workspace

This folder contains isolated, engine-first building blocks for the public
`gorender` runtime, developed without changing production render behavior until
promotion gates pass.

## Current Status

- `timeline/`: deterministic timeline primitives for frame -> slide mapping.
- `interpolate/`: deterministic easing/interpolation primitives for transition math.

## Rules

- Keep packages dependency-light (stdlib first).
- Add tests for every behavior contract before promotion.
- Promote into `internal/` only after parity and performance gates pass.
