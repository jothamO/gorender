# Open-Source Workspace

This folder is an isolated track for preparing `gorender` for public open-source release without disrupting the current production-stable implementation.

## Purpose

- Keep current renderer behavior stable at project root.
- Iterate open-source packaging, docs, governance, and future refactors here first.
- Promote changes back to root only when validated.

## Working Model

1. Prototype or document in `opensource/`.
2. Validate with tests and parity checks.
3. Promote selected changes to main code paths via focused PR/patch.

## Folder Layout

- `plan/`: roadmaps, milestone definitions, acceptance criteria.
- `docs/`: open-source-specific docs drafts.
- `governance/`: contribution, support, release process drafts.
- `backlog/`: prioritized issues/tasks for open-source readiness.
- `core/`: lightweight core charter, constraints, and acceptance budgets.
- `optional/`: non-core modules (GUI, template packs, ecosystem adapters).
- `ci/`: draft CI/reliability workflows before promotion to `.github/workflows`.

## Initial Milestones

- M1: Super-lightweight charter + measurable budgets.
- M2: Core-only docs and compatibility contract.
- M3: CI/release guardrails enforcing size and performance constraints.
- M4: Optional module packaging with strict isolation from core.

## Guardrails

- No production behavior change should be considered final until parity checks pass.
- Preserve current runtime defaults unless explicitly approved for promotion.
- Every promoted optimization must carry parity evidence (SSIM/PSNR + visual review).
- Core must remain framework-agnostic. Framework templates are adapters, not requirements.
- Optional modules must never increase core binary/runtime budgets by default.

## Start Here

- [Lightweight Charter](./core/LIGHTWEIGHT_CHARTER.md)
- [Performance And Size Budgets](./core/PERFORMANCE_BUDGETS.md)
- [Core Isolation Audit Checklist](./core/CORE_ISOLATION_AUDIT_CHECKLIST.md)
- [Open-Source Execution Plan](./plan/OPENSOURCE_EXECUTION_PLAN.md)
- [M5 Optional Surface Plan](./plan/M5_OPTIONAL_SURFACE_PLAN.md)
- [Milestone Tracker](./plan/MILESTONE_TRACKER.md)
- [Promotion Checklist](./plan/PROMOTION_CHECKLIST.md)
- [PR Checklist](./plan/PR_CHECKLIST.md)
- [Versioning/Changelog Policy](./plan/VERSIONING_CHANGELOG_POLICY.md)
- [Release Automation Plan](./plan/RELEASE_AUTOMATION_PLAN.md)
- [v1 Launch Checklist](./plan/V1_LAUNCH_CHECKLIST.md)
- [CI Drafts](./ci/README.md)
