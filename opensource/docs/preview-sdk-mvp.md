# Preview SDK MVP

This is the first implementation step for the player/preview SDK surface.

Command:

```powershell
.\bin\gorender.exe preview --url http://127.0.0.1:8080/moments-mm57lkdh --addr 127.0.0.1:8090 --fps 30 --width 720 --height 1280
```

Open:

- `http://127.0.0.1:8090`

Current capabilities:

- seek slider + frame input
- progress bar
- play/pause preview stepping
- FPS override
- query param preview (`key=value`, one per line)

Optional default params:

```powershell
.\bin\gorender.exe preview --url http://127.0.0.1:8080/moments-mm57lkdh --param theme=dark --param locale=en
```

Notes:

- This is an embeddable local preview control surface, not a full hosted player SDK yet.
- It targets deterministic render-preview workflows and parameter debugging.
