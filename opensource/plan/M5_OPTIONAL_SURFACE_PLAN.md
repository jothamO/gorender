# M5 Optional Surface Plan

## Goal

Expand optional product surfaces (UI and template packs) without increasing core runtime weight or destabilizing existing render behavior.

## Scope

- Optional UI module plan and acceptance criteria
- Template pack strategy and distribution model
- Core isolation audit process and release gate

## Workstreams

## 1) Optional UI Module

- Define packaging model:
  - separate binary (`gorendersd`) or build-tagged UI module
- Keep API parity with CLI-backed engine
- Ensure disabled-by-default behavior in core path

Deliverables:

- `optional/ui/IMPLEMENTATION_PLAN.md`
- integration checklist for API compatibility

## 2) Template Pack Strategy

- Keep built-in templates minimal (`vanilla`)
- Maintain optional lightweight pack (`svelte`)
- Move heavier stacks (`react`, `next`) to external/community packs

Deliverables:

- `optional/templates/IMPLEMENTATION_PLAN.md`
- template governance policy (versioning, ownership, support level)

## 3) Core Isolation Audit

- Formal audit before every release
- Verify optional modules do not alter:
  - default command behavior
  - dependency tree for core paths
  - performance budgets in core mode

Deliverable:

- `core/CORE_ISOLATION_AUDIT_CHECKLIST.md`

## Exit Criteria (M5 Complete)

- UI plan approved and linked to release process
- Template strategy approved and documented
- At least one full isolation audit performed with passing result
- Promotion checklist updated to require isolation audit evidence

