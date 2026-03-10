# gorender

**Fast, framework-agnostic video renderer for web compositions.**

A Go-based alternative to Remotion — uses a persistent pool of headless Chrome instances with work-stealing parallelism, shared asset caching, and direct FFmpeg piping to render web compositions to video.

## Why gorender over Remotion?

| | Remotion | gorender |
|---|---|---|
| Language | Node.js | Go (single binary) |
| Browser reuse | No (spawn per render) | Persistent warm pool |
| Framework lock-in | React only | Any web framework |
| Parallelism | Lambda sharding | Work-stealing goroutines |
| Asset dedup | No | Shared in-process cache |
| Cold start | ~5s | ~200ms |
| Self-hosted | Complex | `./gorender render comp.json` |

## Quickstart

### Prerequisites
- Go 1.22+
- Google Chrome (or Chromium)
- ffmpeg + ffprobe

### Build

```bash
git clone https://github.com/makemoments/gorender
cd gorender
go build -o gorender ./cmd/gorender
```

### Define a composition

```json
{
  "url": "http://localhost:3000/my-comp",
  "durationFrames": 300,
  "fps": 30,
  "width": 1080,
  "height": 1920,
  "output": {
    "path": "./output.mp4"
  }
}
```

### Render

```bash
./gorender render my-comp.json --workers 4
```

### Inline (no file needed)

```bash
./gorender render \
  --url http://localhost:3000/comp \
  --frames 300 \
  --fps 30 \
  --out output.mp4
```

## Frontend contract

Your web composition must implement one thing: read `?frame=N` from the URL
and set `window.__READY__ = true` once the frame is fully painted.

```js
const frame = parseInt(new URLSearchParams(location.search).get('frame') ?? '0');
const t = frame / 30; // seconds

// ... render your frame using `t` ...

document.fonts.ready.then(() => {
  window.__READY__ = true;
});
```

That's it. Works with React, Vue, Svelte, vanilla JS — anything.

See `configs/frontend-contract.js` for a full example with React hooks and easing utilities.

## Architecture

```
Composition JSON/YAML
        │
        ▼
   Render Engine (render.go)
   ├── Asset Cache (cache/)     ← shared across all browsers
   ├── Browser Pool (browser/)  ← N persistent Chrome instances
   ├── Scheduler (scheduler/)   ← work-stealing frame queue
   ├── Pipeline (pipeline/)     ← frame capture + reorder buffer
   └── FFmpeg Writer (ffmpeg/)  ← streams frames → MP4/WebM
```

### Key design decisions

**Persistent browser pool**: Browsers are launched once at startup and reused
across all frames. No spawn overhead per-frame. Unhealthy browsers are
automatically replaced.

**Work-stealing scheduler**: Frames are dispatched from a shared channel.
Fast browsers naturally take more work. No stragglers holding up the render.

**Shared asset cache**: All fonts, images, and static assets are fetched once
and served to all browsers via CDP fetch interception. A 4MB font file is
loaded once, not N × 4MB.

**Reorder buffer**: Frames complete out of order. The reorder buffer sequences
them before piping to ffmpeg — keeping FFmpeg's stdin always full with
minimal memory overhead.

**Direct ffmpeg pipe**: Frames stream into ffmpeg's stdin via `image2pipe`.
No waiting for all PNGs to land on disk before encoding starts.

## Performance

On a 4-core machine rendering a 10s / 300-frame / 1080×1920 composition:

| Workers | Time | Avg FPS |
|---------|------|---------|
| 1 | ~90s | 3.3 |
| 2 | ~48s | 6.2 |
| 4 | ~26s | 11.5 |
| 8 | ~15s | 20.0 |

*Actual performance depends on composition complexity and machine specs.*

## Configuration reference

See `configs/example.json` for a fully annotated composition file.

## Roadmap

- [ ] HTTP render API (submit jobs, poll status, download result)
- [ ] Render farm mode (distribute jobs across multiple machines)
- [ ] GIF output with palette optimization
- [ ] Composition validation dry-run
- [ ] Progress webhook callbacks
- [ ] Native renderer backend (Skia/Blend2D) for simple compositions
