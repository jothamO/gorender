# Open-Source Execution Plan

## Objective

Publish `gorender` as a production-grade open-source project while preserving a super-lightweight core.

## Phase 1: Charter And Core Contracts

- Finalize lightweight charter and core/non-core boundaries.
- Publish frontend contract reference docs.
- Document stable CLI behavior and compatibility policy.

## Phase 2: Documentation Hardening

- Architecture, troubleshooting, performance tuning, and preset docs.
- Migration guidance (`--profile` aliasing to preset model).
- Minimal quickstart with deterministic vanilla example.

## Phase 3: CI And Quality Gates

- Build/test matrix (Windows/Linux/macOS).
- Lint + vet + smoke render checks.
- Parity gate workflow for optimization changes.
- Benchmark capture workflow for regression tracking.

## Phase 4: Governance And Release

- CONTRIBUTING, CODE_OF_CONDUCT, SECURITY, SUPPORT.
- Semantic versioning and changelog policy.
- Automated tagged release with binaries/checksums.

## Phase 5: Optional Module Expansion

- Keep core preview as strict render debugger; build smooth player as separate optional module/packaging.
- Optional template packs with strict isolation.
- Cloud/distributed rendering RFC in backlog.

## Phase 6: Engine-Building (Isolated)

- Build pure, deterministic engine primitives in `opensource/engine/*`.
- Keep new engine packages stdlib-first and test-driven.
- Promote engine modules into core only behind parity/perf gates.
