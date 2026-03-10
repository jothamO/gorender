package preview

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"net/http"
	"strings"
)

type Config struct {
	BaseURL string
	FPS     int
	Width   int
	Height  int
	Params  map[string]string
}

type viewModel struct {
	BaseURL    string
	FPS        int
	Width      int
	Height     int
	ParamsJSON template.JS
}

func NewHandler(cfg Config) (http.Handler, error) {
	cfg.BaseURL = sanitizeBaseURL(cfg.BaseURL)
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	parsedBase, err := url.Parse(cfg.BaseURL)
	if err != nil || parsedBase.Scheme == "" || parsedBase.Host == "" {
		return nil, fmt.Errorf("base URL must be an absolute http(s) URL")
	}
	if parsedBase.Scheme != "http" && parsedBase.Scheme != "https" {
		return nil, fmt.Errorf("base URL scheme must be http or https")
	}
	if cfg.FPS <= 0 {
		cfg.FPS = 30
	}
	if cfg.Width <= 0 {
		cfg.Width = 720
	}
	if cfg.Height <= 0 {
		cfg.Height = 1280
	}
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	b, err := json.Marshal(cfg.Params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}
	vm := viewModel{
		BaseURL:    cfg.BaseURL,
		FPS:        cfg.FPS,
		Width:      cfg.Width,
		Height:     cfg.Height,
		ParamsJSON: template.JS(string(b)),
	}
	tpl, err := template.New("preview").Parse(previewHTML)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tpl.Execute(w, vm)
	})
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return mux, nil
}

func sanitizeBaseURL(raw string) string {
	v := strings.TrimSpace(raw)
	trimTokens := []string{`%2522`, `%22`, `\"`, `"`, `'`}
	for {
		prev := v
		v = strings.TrimSpace(v)
		for _, tok := range trimTokens {
			v = strings.TrimPrefix(v, tok)
			v = strings.TrimSuffix(v, tok)
		}
		if v == prev {
			break
		}
	}
	return v
}

const previewHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1.0" />
<title>gorender preview</title>
<style>
  :root {
    --bg: #0b0e12;
    --panel: #11171d;
    --panel-2: #0f1419;
    --border: #26313d;
    --text: #dbe7f3;
    --muted: #8aa0b7;
    --ok: #33d17a;
    --warn: #ffd166;
    --danger: #ff6b6b;
    --accent: #4fc3f7;
    --radius: 8px;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0;
    min-height: 100vh;
    background: radial-gradient(1200px 700px at 80% -10%, #1a2733 0%, var(--bg) 55%);
    color: var(--text);
    font-family: "Segoe UI", Tahoma, sans-serif;
    display: grid;
    grid-template-rows: auto 1fr auto;
  }
  header, footer {
    padding: 14px 20px;
    border-bottom: 1px solid var(--border);
    background: rgba(10, 14, 18, 0.75);
    backdrop-filter: blur(4px);
  }
  footer {
    border-bottom: 0;
    border-top: 1px solid var(--border);
    color: var(--muted);
    font-size: 12px;
    display: flex;
    gap: 14px;
  }
  .title {
    display: flex;
    align-items: center;
    gap: 12px;
    font-family: Consolas, "Courier New", monospace;
    font-size: 13px;
  }
  .title strong {
    color: var(--ok);
    font-size: 16px;
  }
  .base-url {
    margin-left: auto;
    max-width: 52vw;
    color: var(--muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  main {
    display: grid;
    grid-template-columns: minmax(320px, 420px) 1fr;
    min-height: 0;
  }
  .controls {
    border-right: 1px solid var(--border);
    background: linear-gradient(180deg, var(--panel) 0%, var(--panel-2) 100%);
    padding: 18px;
    overflow: auto;
  }
  .group { margin-bottom: 18px; }
  .group h2 {
    margin: 0 0 10px;
    color: var(--muted);
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 1.5px;
    font-family: Consolas, "Courier New", monospace;
  }
  .row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px;
  }
  .field {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-bottom: 8px;
  }
  label {
    font-size: 12px;
    color: var(--muted);
    font-family: Consolas, "Courier New", monospace;
  }
  input, textarea, button {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: #0f151b;
    color: var(--text);
    padding: 10px;
    font-size: 13px;
    outline: none;
    width: 100%;
  }
  textarea {
    min-height: 120px;
    resize: vertical;
    font-family: Consolas, "Courier New", monospace;
  }
  input:focus, textarea:focus {
    border-color: var(--accent);
  }
  .actions {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 8px;
  }
  button {
    cursor: pointer;
    font-weight: 600;
  }
  #apply {
    background: var(--ok);
    color: #05110a;
    border-color: var(--ok);
  }
  .status {
    margin-top: 10px;
    font-size: 12px;
    color: var(--muted);
    font-family: Consolas, "Courier New", monospace;
  }
  .status .ok { color: var(--ok); }
  .status .warn { color: var(--warn); }
  .status .danger { color: var(--danger); }
  .progress {
    margin-top: 8px;
    height: 4px;
    background: #1b2530;
    border-radius: 2px;
    overflow: hidden;
  }
  .progress > div {
    width: 0%;
    height: 100%;
    background: linear-gradient(90deg, #2bb673 0%, #4fc3f7 100%);
    transition: width 0.16s linear;
  }
  .stage {
    display: grid;
    place-items: center;
    padding: 20px;
    min-height: 0;
  }
  .frame-wrap {
    width: min(100%, {{.Width}}px);
    aspect-ratio: {{.Width}} / {{.Height}};
    border: 1px solid var(--border);
    border-radius: 10px;
    overflow: hidden;
    background: #000;
  }
  iframe {
    width: 100%;
    height: 100%;
    border: 0;
    display: block;
    background: #000;
  }
  .hint {
    margin-top: 10px;
    color: var(--muted);
    font-size: 12px;
  }
  .hint a {
    color: var(--accent);
    text-decoration: none;
  }
  @media (max-width: 980px) {
    main { grid-template-columns: 1fr; }
    .controls { border-right: 0; border-bottom: 1px solid var(--border); }
  }
</style>
</head>
<body>
  <header>
    <div class="title">
      <strong>gorender</strong>
      <span>preview sdk</span>
      <span class="base-url" id="baseUrlText"></span>
    </div>
  </header>

  <main>
    <section class="controls">
      <div class="group">
        <h2>Timeline</h2>
        <div class="row">
          <div class="field">
            <label for="frame">Frame</label>
            <input id="frame" type="number" min="0" value="0" />
          </div>
          <div class="field">
            <label for="maxFrame">Max Frame</label>
            <input id="maxFrame" type="number" min="1" value="300" />
          </div>
        </div>
        <div class="field">
          <label for="seek">Seek</label>
          <input id="seek" type="range" min="0" max="300" value="0" />
        </div>
      </div>

      <div class="group">
        <h2>Render Params</h2>
        <div class="field">
          <label for="fps">FPS</label>
          <input id="fps" type="number" min="1" value="{{.FPS}}" />
        </div>
        <div class="field">
          <label for="params">Query Params (key=value per line)</label>
          <textarea id="params"></textarea>
        </div>
      </div>

      <div class="group">
        <h2>Actions</h2>
        <div class="actions">
          <button id="play" type="button">Play</button>
          <button id="pause" type="button">Pause</button>
          <button id="apply" type="button">Apply</button>
        </div>
        <div class="progress"><div id="progressFill"></div></div>
        <div id="status" class="status"></div>
      </div>
    </section>

    <section class="stage">
      <div class="frame-wrap">
        <iframe id="preview" allow="autoplay" sandbox="allow-scripts allow-same-origin"></iframe>
      </div>
      <div class="hint">
        iframe blocked? Open directly:
        <a id="directLink" href="#" target="_blank" rel="noopener">frame URL</a>
      </div>
    </section>
  </main>

  <footer>
    <span>gorender</span>
    <span>deterministic frame preview</span>
  </footer>

  <script>
    const rawBaseUrl = {{printf "%q" .BaseURL}};
    const defaultParams = {{.ParamsJSON}};
    const frameInput = document.getElementById('frame');
    const maxFrameInput = document.getElementById('maxFrame');
    const seekInput = document.getElementById('seek');
    const fpsInput = document.getElementById('fps');
    const paramsInput = document.getElementById('params');
    const iframe = document.getElementById('preview');
    const progressFill = document.getElementById('progressFill');
    const status = document.getElementById('status');
    const directLink = document.getElementById('directLink');
    const baseUrlText = document.getElementById('baseUrlText');
    let timer = null;

    function normalizeBaseUrl(raw) {
      let v = String(raw || '').trim();
      const tokens = ['%2522', '%22', '\\"', '"', "'"];
      while (true) {
        const prev = v;
        v = v.trim();
        for (const tok of tokens) {
          if (v.startsWith(tok)) v = v.slice(tok.length);
          if (v.endsWith(tok)) v = v.slice(0, v.length - tok.length);
        }
        if (prev === v) break;
      }
      return v;
    }

    function parseParamsText(text) {
      const out = {};
      String(text || '').split('\n').forEach((line) => {
        const row = line.trim();
        if (!row || row.startsWith('#')) return;
        const i = row.indexOf('=');
        if (i <= 0) return;
        const k = row.slice(0, i).trim();
        const v = row.slice(i + 1).trim();
        if (k) out[k] = v;
      });
      return out;
    }

    function paramsToText(obj) {
      return Object.entries(obj || {}).map(([k, v]) => k + '=' + v).join('\n');
    }

    function intValue(el, fallback) {
      const n = parseInt(el.value || '', 10);
      return Number.isFinite(n) ? n : fallback;
    }

    function clampFrame(frame, max) {
      const m = Math.max(1, max);
      return Math.max(0, Math.min(frame, m));
    }

    const cleanBaseUrl = normalizeBaseUrl(rawBaseUrl);
    baseUrlText.textContent = cleanBaseUrl;
    paramsInput.value = paramsToText(defaultParams);

    iframe.addEventListener('load', () => {
      if (status.dataset.mode !== 'error') {
        status.innerHTML = '<span class="ok">loaded</span> ';
      }
    });
    iframe.addEventListener('error', () => {
      status.dataset.mode = 'error';
      status.innerHTML = '<span class="danger">iframe load error</span>';
    });

    function buildFrameURL() {
      const max = Math.max(1, intValue(maxFrameInput, 300));
      const frame = clampFrame(intValue(frameInput, 0), max);
      const fps = Math.max(1, intValue(fpsInput, 30));
      frameInput.value = String(frame);
      seekInput.max = String(max);
      seekInput.value = String(frame);
      const merged = Object.assign({}, defaultParams, parseParamsText(paramsInput.value));
      const u = new URL(cleanBaseUrl);
      Object.entries(merged).forEach(([k, v]) => u.searchParams.set(k, String(v)));
      u.searchParams.set('frame', String(frame));
      u.searchParams.set('fps', String(fps));
      // Hint to the embedded app that this is deterministic preview navigation.
      // Apps can use this to disable beforeunload prompts during frame stepping.
      u.searchParams.set('gr_preview', '1');
      return { url: u.toString(), frame, max, fps };
    }

    function render() {
      let payload;
      try {
        payload = buildFrameURL();
      } catch (_) {
        status.dataset.mode = 'error';
        status.innerHTML = '<span class="danger">invalid base url</span>';
        return;
      }
      const pct = (payload.frame / payload.max) * 100;
      progressFill.style.width = pct.toFixed(2) + '%';
      const msg = 'frame ' + payload.frame + ' / ' + payload.max + ' @ ' + payload.fps + 'fps';
      if (status.dataset.mode !== 'error') {
        status.innerHTML = '<span class="warn">loading...</span> ' + msg;
      }
      iframe.src = payload.url;
      directLink.href = payload.url;
    }

    function startPlay() {
      if (timer) return;
      timer = setInterval(() => {
        const max = Math.max(1, intValue(maxFrameInput, 300));
        const next = (clampFrame(intValue(frameInput, 0), max) + 1) % (max + 1);
        frameInput.value = String(next);
        render();
      }, 1000 / Math.max(1, intValue(fpsInput, 30)));
    }

    function stopPlay() {
      if (!timer) return;
      clearInterval(timer);
      timer = null;
    }

    document.getElementById('play').addEventListener('click', startPlay);
    document.getElementById('pause').addEventListener('click', stopPlay);
    document.getElementById('apply').addEventListener('click', render);
    frameInput.addEventListener('input', render);
    seekInput.addEventListener('input', () => {
      frameInput.value = seekInput.value;
      render();
    });
    maxFrameInput.addEventListener('input', render);
    fpsInput.addEventListener('input', render);
    render();
  </script>
</body>
</html>`
