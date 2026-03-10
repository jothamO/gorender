# CI Workflow Drafts (OpenSource Track)

These are draft workflows for M3 reliability.  
They are intentionally stored in `opensource/ci/` first, so they do not affect current project behavior.

## Included Drafts

- `github-workflows/ci.yml`: build + test matrix (Windows/Linux/macOS)
- `github-workflows/parity-gate.yml`: parity validation workflow (speed + SSIM + PSNR)
- `github-workflows/benchmark-regression.yml`: benchmark capture workflow
- `github-workflows/release.yml`: release build artifact workflow (draft)

## Promotion Path

When approved, copy these files to:

- `.github/workflows/ci.yml`
- `.github/workflows/parity-gate.yml`
- `.github/workflows/benchmark-regression.yml`
- `.github/workflows/release.yml`

Then add repo secrets/variables needed by parity and benchmark runs.

## Notes

- Parity and benchmark workflows need a reachable render target URL.
- For hosted CI without an app fixture, use `workflow_dispatch` inputs or self-hosted runners.
- Keep core workflow lightweight: compile + tests should always run without external web app dependencies.
