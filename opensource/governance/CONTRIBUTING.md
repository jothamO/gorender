# Contributing Guide (Draft)

## Principles

- Keep core lightweight and framework-agnostic.
- Prefer incremental, testable changes.
- Preserve deterministic render parity.

## Workflow

1. Open or pick an issue from `opensource/backlog`.
2. Implement in the `opensource` track first when possible.
3. Run checks:
   - `go test ./...`
   - smoke render
   - parity gate evidence for performance-impacting changes
4. Complete PR checklist (see `opensource/plan/PR_CHECKLIST.md`).
5. Request review and include rationale + risk notes.

## Commit/PR Expectations

- Clear problem statement and scope.
- User-visible behavior changes documented.
- Backward-compatibility impact explicitly stated.

## Performance Changes

Any optimization PR must include:

- baseline vs candidate timing
- SSIM/PSNR metrics (or explicit reason not applicable)
- visual spot-check summary

