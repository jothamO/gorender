# Troubleshooting

## `net::ERR_CONNECTION_REFUSED`

Cause:
- target URL is not reachable from Chrome worker.

Checks:
1. Confirm frontend dev server is running.
2. Prefer `127.0.0.1` if `localhost` has IPv6 mismatch.
3. Open URL manually in local browser.

## `auto duration source could not resolve frames`

Cause:
- `--duration-source auto` could not read metadata/ready state.

Checks:
1. Ensure page sets `window.__GORENDER_META__`.
2. Ensure page sets `window.__READY__` and `window.__FRAME_READY__`.
3. Retry with fixed/manual source to isolate contract issue:
   - `--duration-source fixed --slides ...`
   - `--duration-source manual --slide-durations-ms ...`

## `ready timed out after ...`

Cause:
- frame never reached deterministic ready state.

Checks:
1. Confirm render path calls ready assignment after fonts/assets settle.
2. Remove blocking async flows in render mode.
3. Lower workers temporarily to debug (`--workers 1`).

## `chrome failed to start`

Cause:
- browser binary discovery/startup failure.

Checks:
1. Run `gorender check`.
2. Ensure Chrome/Edge binary is installed and executable.
3. Test with fewer workers to reduce startup pressure.

## `ffmpeg not found` / `ffprobe not found`

Cause:
- ffmpeg tools missing from PATH.

Fix:
1. Install ffmpeg.
2. Add ffmpeg directory to PATH.
3. Restart shell and rerun `gorender check`.

## Render is too slow

Actions:
1. Use a speed-oriented preset (for iteration): `--preset speed-balanced` or `--preset cpu-constrained`.
2. Reduce render resolution (`--width/--height`) and upscale at output if acceptable.
3. Keep `--workers` aligned with machine capacity; increase only if stable.
4. Use `parity` command to quantify speed/quality tradeoff before promotion.

## Visual mismatch from frontend

Checks:
1. Verify deterministic frame math (no wall-clock animation).
2. Ensure per-slide durations are consistent between frontend and render metadata.
3. Validate with `parity` SSIM/PSNR and a manual visual spot-check.

