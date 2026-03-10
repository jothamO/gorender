# Building gorender — A Go-Based Video Render Engine

*Conversation between Jotham and Claude*

---

## Remotion but in Go

**Jotham:** Let's talk about building our own remotion but on golang. Remotion needs a faster, smarter competitor

**Claude:**

### Remotion's core model (and its pain points)

Remotion works by driving a headless Chromium instance frame-by-frame, using `currentFrame` injected into React to let you express video as a function of time. The output is frames → ffmpeg → video.

Its real weaknesses:

- **Slow**: Chromium is heavy. Rendering is single-threaded per composition unless you shard across Lambda instances (expensive).
- **JS/React lock-in**: Your entire composition must be React. No escape hatch.
- **Memory hungry**: Each headless browser eats ~300–500MB RAM.
- **Complex infra for scale**: Remotion Lambda is clever but opaque and costly.
- **No real GPU acceleration**: It's all CPU-bound Chromium painting.

### What a Go-based competitor could look like

**Option A: Go orchestrates headless Chromium (like Remotion, but smarter)**
Use chromedp or rod to drive Chrome, but with Go handling the parallelism, job queue, and frame stitching. Faster scheduling, better resource management, but still Chromium-dependent.

**Option B: Go + a native renderer (no browser)**
More ambitious. Possible backends: Gio/Ebitengine (native Go 2D, GPU-accelerated), Skia via CGo (same engine Chrome uses), Cairo via CGo, Blend2D.

**Option C: Go as the backbone, pluggable renderers**
Go owns the timeline, asset pipeline, job queue, and FFmpeg integration — pluggable renderer backends.

### The sweet spot

Go orchestration + Chromium rendering, but with a much smarter parallel pipeline than WVC/Remotion. 3–5x throughput without the complexity of a native renderer.

---

## Architecture Deep Dive

**Jotham:** I like your sweet spot idea

**Claude:**

### The core insight

Remotion's Lambda approach parallelizes by spinning up N Lambda functions per render. Go gives you a single binary managing a **pool of persistent browser instances**, shared memory for assets, and a tight render loop — no cloud required.

### Architecture

```
                    ┌─────────────────────────────┐
                    │      Go Render Server        │
                    │                              │
  Composition  ───► │  Timeline   Job Scheduler    │
  (JSON/YAML)       │  Parser  ──► (work-stealing  │
                    │              queue)           │
                    │                              │
                    │  Asset Cache (shared memory) │
                    │  fonts, images, audio blobs  │
                    └────────────┬────────────────┘
                                 │ assigns frame ranges
                    ┌────────────▼────────────────┐
                    │      Browser Pool            │
                    │  [Chrome 1] [Chrome 2] ...   │
                    │  persistent, pre-warmed      │
                    │  each owns N frames          │
                    └────────────┬────────────────┘
                                 │ PNG frame stream
                    ┌────────────▼────────────────┐
                    │     FFmpeg Pipe Manager      │
                    │  reassembles ordered frames  │
                    │  mixes audio tracks          │
                    │  outputs MP4/WebM            │
                    └─────────────────────────────┘
```

### Key innovations over Remotion

1. **Persistent browser pool** — browsers stay alive, navigate to composition URL, receive `seekToFrame(n)` calls. No reload cost.
2. **Work-stealing frame scheduler** — fast browsers naturally grab more work via shared channel. No stragglers.
3. **Shared asset cache** — CDP fetch interception serves assets from one in-memory store across all browsers.
4. **Frame pipeline with backpressure** — reorder buffer sequences out-of-order frames before streaming to ffmpeg stdin.

### Comparison

| | Remotion | WVC | gorender |
|---|---|---|---|
| Language | Node | Node | Go |
| Browser reuse | No | Partial | Full pool |
| Framework lock-in | React | Any | Any |
| Parallelism model | Lambda sharding | Limited | Work-stealing pool |
| Asset sharing | No | No | Yes |
| Self-hosted | Yes (complex) | Yes | Yes (single binary) |
| Cold start | High | Medium | Near zero |

---

## Initial Sketch

**Jotham:** Yes. Start sketching

**Claude:** *(Built 11 files)*

### Package structure

```
gorender/
├── render.go                          # Top-level Render() function
├── cmd/gorender/main.go               # CLI binary
├── internal/
│   ├── composition/composition.go     # JSON/YAML schema + defaults
│   ├── browser/pool.go                # Persistent Chrome pool
│   ├── scheduler/scheduler.go         # Work-stealing frame queue
│   ├── cache/asset_cache.go           # Shared asset cache + CDP interception
│   ├── pipeline/renderer.go           # Frame capture + reorder buffer
│   └── ffmpeg/writer.go               # FFmpeg pipe + audio mux
└── configs/
    ├── example.json                   # Example composition file
    └── frontend-contract.js           # The JS side any web comp must implement
```

### The frontend contract

Any web app becomes renderable by implementing one thing:

```js
const frame = parseInt(new URLSearchParams(location.search).get('frame') ?? '0');
const t = frame / 30; // seconds

// ... render your frame using t ...

document.fonts.ready.then(() => {
  window.__READY__ = true;
});
```

Framework doesn't matter — React, Vue, Svelte, vanilla JS all work.

---

## Are We a Match for Remotion?

**Jotham:** What next? Are we a match for remotion at this point?

**Claude:**

Honestly, no — not yet. Here's where we stand:

### What we have

A solid **architectural skeleton**. The key ideas are all there but it's a sketch, not a product.

### Critical gaps (blockers)

- No tests — not a single one
- Asset interception is incomplete — cache misses fall through without storing the response
- No retry logic — a frame that fails just fails
- `ReorderBuffer.Push` isn't thread-safe — race condition waiting to happen
- `OnProgress` is defined but never called

### Feature gaps vs Remotion

- No render server / HTTP API
- No hot reload / preview mode
- No composition introspection
- No partial re-renders
- No GIF optimization
- No sequence/spring/interpolate utilities

### The gap map

```
                    Remotion          gorender (now)
─────────────────────────────────────────────────────
Core rendering      ████████████      ████░░░░░░░░
Parallelism         ████░░░░░░░░      ████████░░░░  ← actually ahead here
Asset perf          ██░░░░░░░░░░      ██████░░░░░░  ← ahead in theory
DX / Studio         ████████████      ░░░░░░░░░░░░
Animation utils     ████████████      ░░░░░░░░░░░░
Test coverage       ████████████      ░░░░░░░░░░░░
Production hardness ████████████      ███░░░░░░░░░
```

---

## Defining the Target

**Jotham:** render engine that powers MakeMoments specifically. If it does the job well, then that's a test case for opensource

**Claude:**

Perfect. Target is clear: **replace WVC reliably for MakeMoments, prove it works, then open source a battle-tested thing rather than a theory.**

### Build order

**Phase 1 — Make it correct (this week)**
- Thread-safe ReorderBuffer
- Complete asset interception
- Frame retry logic
- Wire up OnProgress

**Phase 2 — HTTP render server (next week)**
This replaces Express + WVC + PQueue. One Go binary your backend POSTs to:

```
POST /jobs          → { "jobId": "j_abc123", "status": "queued" }
GET  /jobs/{id}     → { "status": "rendering", "progress": 0.42, "eta": "14s" }
GET  /jobs/{id}/download → streams MP4
```

**Phase 3 — Production hardening**
- Auto-delete MP4s after N hours (replaces your cron)
- Crash recovery for in-flight jobs
- Memory cap on concurrent jobs
- `GET /health` with pool status

**Phase 4 — Integration test against StoryViewer**

```go
func TestStoryViewerRenders(t *testing.T) {
    // Hit https://staging.makemoments.xyz/story/test-fixture
    // Render 30 frames
    // Assert: no errors, output MP4 exists, duration correct
}
```

---

## Building the HTTP Render Server

**Jotham:** Yes proceed

**Claude:** *(Built 10 more files)*

### New packages added

**`jobs/store.go`** — Thread-safe in-memory job registry.

```go
type Status string

const (
    StatusQueued    Status = "queued"
    StatusRendering Status = "rendering"
    StatusDone      Status = "done"
    StatusFailed    Status = "failed"
)
```

Designed so the interface can be backed by Redis or SQLite later without changing call sites.

**`jobs/queue.go`** — The dispatch engine. Owns:
- Browser pool across all jobs
- Concurrency semaphore (prevents OOM on constrained VPS)
- Retry logic — 3 attempts with exponential backoff
- Progress tracking piped through the frame channel
- Hourly cleanup loop (replaces your PM2 cron)

**`server/server.go`** — REST API:

| Method | Route | Description |
|--------|-------|-------------|
| `GET` | `/health` | Pool status, job counts |
| `POST` | `/jobs` | Submit render job → 202 |
| `GET` | `/jobs` | List all jobs |
| `GET` | `/jobs/{id}` | Poll job status |
| `GET` | `/jobs/{id}/download` | Stream MP4 (202 if still rendering) |
| `DELETE` | `/jobs/{id}` | Cancel / remove |

**`cmd/gorendersd/main.go`** — Server binary with graceful SIGINT/SIGTERM shutdown.

### Bugs fixed

- `ReorderBuffer` race condition — mutex + `sync.Once` for channel close
- Asset interception cache-miss — now calls `fetch.GetResponseBody` and stores response

### Tests written

- `reorder_test.go` — 5 cases including concurrent push race
- `scheduler_test.go` — work-stealing, cancellation, error propagation
- `store_test.go` — concurrent access, prune logic, ordering
- `server_test.go` — all HTTP handlers including auth
- `integration/storyviewer_test.go` — real regression test, runs with `-tags=integration`

### Deploy on your VPS

```bash
make build

./bin/gorendersd \
  --addr :8080 \
  --api-key $RENDER_API_KEY \
  --max-jobs 2 \
  --workers 4 \
  --output-dir /var/gorender/output \
  --retention 24
```

Your MakeMoments backend POSTs to `http://localhost:8080/jobs` — same pattern as Express+WVC, different engine underneath.

---

## How Can We Beat Remotion?

**Jotham:** Ok. Seems good. How can we be better than remotion? Brutal fact only

**Claude:**

**You can't beat Remotion on features.** They have a 4-year head start, a Studio, a JS animation library, Lambda infrastructure, and a large user base filing edge cases. Trying to out-feature them is a losing game.

### The only real advantages you can have

**1. Speed — provable with numbers.**
Remotion on a single machine is genuinely slow. If gorender renders a 30-second 1080×1920 clip 3× faster on the same hardware, that's a concrete, measurable, marketable fact. Right now you don't know if that's true. You need the benchmark.

**2. No framework lock-in.**
Remotion requires React. Full stop. Any team on Vue, Svelte, vanilla canvas, or Three.js is excluded. gorender's "any URL" contract is a real differentiator — but only if you document it clearly and it actually works reliably.

**3. Single binary, zero cloud dependency.**
Remotion Lambda is genuinely complex and expensive. A team that wants self-hosted, predictable-cost rendering on their own VPS has no good option today. gorender could own that niche entirely.

### The brutal facts that should worry you

- You have zero production renders. Remotion has millions. Every edge case you haven't hit yet — font loading races, canvas taint issues, memory leaks in long browser sessions, corrupted frames on crash — Remotion has already solved.
- The asset cache and interception code is still largely untested against real compositions. It might silently serve stale assets and you wouldn't know.
- Browser pool health is naive. One Chrome process leaking memory over 500 renders will quietly degrade performance. You have no metrics to detect this.
- No one will trust a render engine from a company they've never heard of for production video without a public track record. MakeMoments itself is that track record — but only once you're actually using it in production.

### The honest path to being better

Ship it on MakeMoments. Run it for 6 months. Fix the real failures. Publish the benchmark. Open source a thing that has rendered a million real videos, not a thing that looks good on paper.

That's the only version of "better than Remotion" that's credible.

---

## Content Scaling Issues

**Jotham:** An issue I'm currently having with remotion is content scaling in the rendered videos. The videos look different from my storypreview/storyviewer. Is this a common remotion issue?

**Claude:**

Yes, extremely common. It's one of Remotion's most complained-about problems.

### Why it happens in Remotion

Remotion renders in a headless browser at your composition dimensions, but your preview runs in a browser window where the composition is **scaled down to fit the screen** using CSS transforms. CSS `transform: scale()` and actual viewport rendering produce subtly different results for:

- **Font rendering** — subpixel hinting changes at different scales
- **Border radius and shadows** — scale transforms don't recompute these, they just squish pixels
- **Canvas elements** — if your canvas is sized to the preview container rather than the composition dimensions, it renders at wrong resolution
- **`devicePixelRatio`** — headless Chrome typically has DPR=1, your dev machine has DPR=2
- **Viewport-relative units** — `vw`, `vh`, `vmin` resolve against the actual viewport in headless Chrome, not the composition frame

### Your specific situation is actually worse

Because you're not using Remotion's React model — you have a real StoryViewer that runs at whatever size the browser window is, then gets screenshot by WVC. So you have **two mismatches** stacked:

1. The preview in your editor is scaled to fit the screen
2. WVC loads the StoryViewer URL in a headless browser at full 1080×1920, but if anything in your CSS is responding to viewport size rather than the composition container size, it diverges

The coordinate alignment bugs you already fixed (`top-[52px]` reference frame, percentage-based transforms) were symptoms of exactly this.

### The gorender advantage here

Because gorender sets the exact viewport to your composition dimensions:

```go
chromedp.EmulateViewport(int64(r.comp.Width), int64(r.comp.Height))
```

The headless browser's viewport **is** 1080×1920. `vw` and `vh` resolve correctly. There's no scale transform involved.

### The fix for preview mismatch

Make the preview scale via a CSS transform on a wrapper, while the inner composition always thinks it's at full size:

```jsx
// The composition always renders at full size internally.
// Only the outer wrapper scales.
<div
  style={{
    width: COMP_WIDTH,
    height: COMP_HEIGHT,
    transform: `scale(${previewScale})`,
    transformOrigin: 'top left',
  }}
>
  <StoryViewer /> {/* never knows it's being scaled */}
</div>
```

Critically — make sure nothing inside StoryViewer uses `vw`, `vh`, `window.innerWidth`, or `window.innerHeight`. Everything must be relative to the composition container dimensions, not the viewport.

---

*End of conversation*
