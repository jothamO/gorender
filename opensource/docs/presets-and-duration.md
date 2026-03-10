# Presets And Duration Reference

This reference documents current stable behavior for preset and duration resolution.

## Preset Selection

Priority:

1. explicit `--preset`
2. legacy `--profile` alias mapping (`final`, `fast`)
3. per-flag explicit overrides

Explicit flags always override preset defaults.

## Duration Resolution

Render frame count precedence:

1. explicit `--frames`
2. `--duration-source manual` + `--slide-durations-ms`
3. `--duration-source auto` via `window.__GORENDER_META__`
4. fixed fallback (`--slides` and `--seconds-per-slide`)
5. fail if none resolve a valid positive frame count

### Manual Duration Format

`--slide-durations-ms` expects CSV positive integers, for example:

```powershell
--duration-source manual --slide-durations-ms 5000,3000,7000
```

## Sizing/Upscale Controls

- `--width`, `--height`: render resolution.
- `--upscale-width`, `--upscale-height`: output target resolution.
- `--no-upscale`: disable upscale even if preset defines it.

If render and upscale sizes are equal, upscale pass is skipped.

## Recommended Defaults (Current)

- Core production default path: `--preset final`
- Auto timeline: `--duration-source auto`
- Parity checks: `parity` command with speed + SSIM + PSNR thresholds.

## CLI Examples

```powershell
.\bin\gorender.exe render --url http://localhost:8080/moments-abc --duration-source auto --preset final --workers 2 --out .\output\final.mp4 -v
```

```powershell
.\bin\gorender.exe render --url http://localhost:8080/moments-abc --duration-source manual --slide-durations-ms 5000,3000,7000 --preset final --workers 2 --out .\output\manual.mp4 -v
```

