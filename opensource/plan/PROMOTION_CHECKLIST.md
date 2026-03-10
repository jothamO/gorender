# Promotion Checklist (OpenSource -> Root)

Use this checklist before moving changes from `opensource/` into root project paths.

## Required

- [ ] Change belongs to core scope or approved optional track.
- [ ] No unapproved framework/runtime dependency introduced into core.
- [ ] `go test ./...` passes.
- [ ] Smoke render passes.
- [ ] If performance-related: parity gate evidence attached (speedup, SSIM, PSNR).
- [ ] Docs updated for any user-visible behavior/flags.

## For Performance Changes

- [ ] Baseline vs candidate timing recorded.
- [ ] Quality metrics within thresholds.
- [ ] Visual spot-check approved.
- [ ] Regression risk documented.

## For Optional Modules

- [ ] Confirmed isolated packaging path.
- [ ] Disabled-by-default behavior verified.
- [ ] Core binary/behavior unaffected when module is off.
- [ ] Core isolation audit completed and attached (`core/CORE_ISOLATION_AUDIT_CHECKLIST.md`).

## Sign-off

- [ ] Technical sign-off
- [ ] Product/runtime sign-off
- [ ] Release sign-off
