# Distributed Render MVP

This is the first implementation step for distributed rendering in `gorender`.

Current scope:

- shard planning (`gorender shard-plan`)
- shard-safe render offsets (`gorender render --frame-offset`)
- shard video merge (`gorender concat`)

## 1) Build shard plan

```powershell
.\bin\gorender.exe shard-plan --frames 750 --shards 3 --out .\output\shards.json
```

Example result:

- shard 0: frames `0..249`
- shard 1: frames `250..499`
- shard 2: frames `500..749`

## 2) Render each shard

Render each shard with:

- `--frames` = shard frame count
- `--frame-offset` = shard start frame

Example for 3 shards:

```powershell
.\bin\gorender.exe render --url http://127.0.0.1:8080/moments-mm57lkdh --frames 250 --fps 30 --frame-offset 0   --preset final --workers 2 --out .\output\shard-0.mp4 -v
.\bin\gorender.exe render --url http://127.0.0.1:8080/moments-mm57lkdh --frames 250 --fps 30 --frame-offset 250 --preset final --workers 2 --out .\output\shard-1.mp4 -v
.\bin\gorender.exe render --url http://127.0.0.1:8080/moments-mm57lkdh --frames 250 --fps 30 --frame-offset 500 --preset final --workers 2 --out .\output\shard-2.mp4 -v
```

## 3) Concat shard videos

```powershell
.\bin\gorender.exe concat --input .\output\shard-0.mp4 --input .\output\shard-1.mp4 --input .\output\shard-2.mp4 --out .\output\distributed-final.mp4 -v
```

## Notes

- Inputs to `concat` must be in timeline order.
- Shard outputs should use matching encode settings for concat copy mode.
- This is MVP-level distributed support and not yet full cluster orchestration.
