# Template Packs Implementation Plan

## Objective

Offer starter templates without turning framework ecosystems into core dependencies.

## Strategy

- Built-in pack:
  - `vanilla` only (always available, zero framework dependency)
- Optional official pack:
  - `svelte` (lightweight compiled output)
- External/community packs:
  - `react`, `next`, others

## Distribution Model

- Core CLI ships only minimal built-in templates.
- Additional packs distributed as:
  - separate repositories, or
  - downloadable archives/versioned registries

## Governance

Each template pack must define:

- owner/maintainer
- support level (core-supported vs community-supported)
- version compatibility with `gorender`

## Guardrails

- Core build must not include external framework runtime artifacts.
- Template updates must not require core CLI changes unless contract evolves.
- Contract compliance tests required for template acceptance.

## Acceptance Criteria

- Built-in vanilla template remains deterministic and no-build.
- Optional packs are installable without touching core runtime dependencies.
- Compatibility table published for template pack versions.

