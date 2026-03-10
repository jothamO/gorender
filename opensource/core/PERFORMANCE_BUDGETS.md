# Performance And Size Budgets

## Why Budgets

Budgets prevent silent drift away from the lightweight goal.

## Budget Categories

1. Build footprint
- Core CLI binary size change must stay within agreed threshold per release.
- Any increase above threshold requires explicit sign-off and rationale.

2. Startup behavior
- `gorender --help` and command startup latency must remain stable release-to-release.

3. Render throughput
- Standard smoke render should not regress beyond threshold on reference machine.
- Track wall time and effective FPS.

4. Memory profile
- Peak memory under representative workloads must stay within budget.

5. Parity quality
- Any speed optimization requires parity validation (`SSIM`, `PSNR`, visual spot check).

## Suggested Initial Thresholds

- Render regression guard: no more than +10% wall time on benchmark scene.
- Parity guard: SSIM >= 0.995, PSNR >= 40 dB for final-profile checks.
- Core feature additions require justification if they increase complexity without measurable render or UX gain.

## Enforcement Points

- CI benchmark workflow (nightly or release-candidate)
- Parity gate workflow on optimization PRs
- Release checklist sign-off before tagging

## Notes

Exact numeric budgets can be tightened after collecting 2-3 stable release baselines.
