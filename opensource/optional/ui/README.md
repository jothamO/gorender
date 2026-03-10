# UI Module (Optional)

Goal: provide a non-terminal workflow without changing core renderer guarantees.

## Product Split

- Core preview (`gorender preview`) stays a strict render debugger:
  deterministic frame seek, param probing, and contract verification.
- Smooth player experience is a separate optional surface:
  it should not block P0 render/output work and must not couple to core runtime paths.

## Constraints

- Must use same backend API/engine as CLI.
- Must be separately packaged or build-tagged.
- Must not affect core render defaults or budgets when disabled.

## Current MVP

- `gorendersd --ui` serves a lightweight smooth-player UI at `/ui/`.
- UI creates jobs via `/jobs`, polls progress, and supports download/playback of completed outputs.
- UI is disabled by default and does not change API behavior when not enabled.
