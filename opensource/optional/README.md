# Optional Modules Policy

This folder defines add-on capabilities that must remain isolated from core.

## Rules

- Optional modules must be disabled by default.
- Optional modules must not pull heavyweight runtime dependencies into core binary paths.
- Optional module docs must clearly state tradeoffs and intended user profile.

## Current Optional Tracks

- `ui/`: non-technical user interface layer
- `templates/`: starter packs for framework ecosystems

## Planning Docs

- `ui/IMPLEMENTATION_PLAN.md`
- `templates/IMPLEMENTATION_PLAN.md`
