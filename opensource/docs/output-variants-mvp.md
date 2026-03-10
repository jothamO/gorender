# Output Variants MVP

This is the first implementation step for output variants.

New command family:

- `gorender export still`
- `gorender export sequence`
- `gorender export gif`
- `gorender export audio`

## Still

```powershell
.\bin\gorender.exe export still --url http://127.0.0.1:8080/moments-mm57lkdh --duration-source auto --preset final --frame 120 --out .\output\still-120.png -v
```

## Image sequence

```powershell
.\bin\gorender.exe export sequence --url http://127.0.0.1:8080/moments-mm57lkdh --duration-source auto --preset final --out-dir .\output\sequence --pattern frame-%06d.png -v
```

## GIF

```powershell
.\bin\gorender.exe export gif --url http://127.0.0.1:8080/moments-mm57lkdh --duration-source auto --preset final --gif-fps 15 --out .\output\preview.gif -v
```

## Audio-only

```powershell
.\bin\gorender.exe export audio --url http://127.0.0.1:8080/moments-mm57lkdh --duration-source auto --preset final --codec mp3 --out .\output\audio-only.mp3 -v
```

## Scope Notes

- This MVP uses a rendered temp MP4 then converts via ffmpeg.
- Transparent video is intentionally deferred to a dedicated follow-up step.
