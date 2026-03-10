package cache

import (
	"context"
	"encoding/base64"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/chromedp"
	"go.uber.org/zap"
)

// Entry is a cached asset.
type Entry struct {
	Data        []byte
	ContentType string
	CachedAt    time.Time
}

// AssetCache holds all fetched assets in memory, shared across all browser instances.
// Browsers intercept their network requests and are served from this cache,
// avoiding redundant downloads and reducing per-browser memory overhead.
type AssetCache struct {
	mu      sync.RWMutex
	entries map[string]*Entry
	log     *zap.Logger

	// Stats
	hits   atomic.Int64
	misses atomic.Int64
}

// New creates an empty AssetCache.
func New(log *zap.Logger) *AssetCache {
	return &AssetCache{
		entries: make(map[string]*Entry),
		log:     log,
	}
}

// Get retrieves an asset by URL. Returns nil if not cached.
func (c *AssetCache) Get(rawURL string) *Entry {
	key := normalizeURL(rawURL)
	c.mu.RLock()
	e := c.entries[key]
	c.mu.RUnlock()
	if e != nil {
		c.hits.Add(1)
	} else {
		c.misses.Add(1)
	}
	return e
}

// Put stores an asset. Safe for concurrent use.
func (c *AssetCache) Put(rawURL string, data []byte, contentType string) {
	key := normalizeURL(rawURL)
	c.mu.Lock()
	c.entries[key] = &Entry{
		Data:        data,
		ContentType: contentType,
		CachedAt:    time.Now(),
	}
	c.mu.Unlock()
	c.log.Debug("asset cached",
		zap.String("url", key),
		zap.Int("bytes", len(data)),
		zap.String("type", contentType),
	)
}

// Prefetch downloads a list of asset URLs into the cache before rendering starts.
// Call this with your known font/image URLs to warm the cache.
func (c *AssetCache) Prefetch(ctx context.Context, urls []string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	var wg sync.WaitGroup
	errs := make(chan error, len(urls))

	for _, rawURL := range urls {
		if c.Get(rawURL) != nil {
			continue // already cached
		}
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			if err := c.fetchAndStore(ctx, client, u); err != nil {
				errs <- fmt.Errorf("prefetch %s: %w", u, err)
			}
		}(rawURL)
	}

	wg.Wait()
	close(errs)

	var combined []string
	for err := range errs {
		combined = append(combined, err.Error())
	}
	if len(combined) > 0 {
		return fmt.Errorf("prefetch errors: %s", strings.Join(combined, "; "))
	}
	return nil
}

// PrefetchDir walks a local directory and loads all files into the cache
// under a base URL prefix. Useful for self-hosted fonts/images.
func (c *AssetCache) PrefetchDir(dir, baseURL string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		assetURL := baseURL + "/" + filepath.ToSlash(rel)
		c.Put(assetURL, data, mimeFromPath(path))
		return nil
	})
}

// Stats returns cache hit/miss counts and entry count.
func (c *AssetCache) Stats() (entries int, hits, misses int64) {
	c.mu.RLock()
	entries = len(c.entries)
	c.mu.RUnlock()
	return entries, c.hits.Load(), c.misses.Load()
}

// EnableInterception attaches fetch interception to a browser context.
// All network requests from that browser will be served from this cache
// if available, or fetched and stored on cache miss.
//
// Call this once per browser after acquiring it from the pool.
func (c *AssetCache) EnableInterception(browserCtx context.Context) error {
	// Enable fetch domain to intercept all requests.
	if err := chromedp.Run(browserCtx, fetch.Enable()); err != nil {
		return fmt.Errorf("enabling fetch interception: %w", err)
	}

	// Listen for requestPaused events in a goroutine.
	go c.handleInterceptions(browserCtx)
	return nil
}

func (c *AssetCache) handleInterceptions(ctx context.Context) {
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		paused, ok := ev.(*fetch.EventRequestPaused)
		if !ok {
			return
		}

		go func(p *fetch.EventRequestPaused) {
			requestURL := p.Request.URL

			// Only cache static assets — skip navigation requests.
			if !isStaticAsset(requestURL) {
				if err := chromedp.Run(ctx, fetch.ContinueRequest(p.RequestID)); err != nil {
					c.log.Debug("continue request failed", zap.String("url", requestURL), zap.Error(err))
				}
				return
			}

			entry := c.Get(requestURL)

			if entry != nil {
				// Serve from cache — fulfill without hitting the network.
				params := &fetch.FulfillRequestParams{
					RequestID:    p.RequestID,
					ResponseCode: 200,
					ResponseHeaders: []*fetch.HeaderEntry{
						{Name: "content-type", Value: entry.ContentType},
						{Name: "cache-control", Value: "public, max-age=31536000"},
						{Name: "x-gorender-cache", Value: "HIT"},
					},
					Body: base64.StdEncoding.EncodeToString(entry.Data),
				}
				if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
					return params.Do(ctx)
				})); err != nil {
					c.log.Debug("fulfill from cache failed", zap.Error(err))
				}
				return
			}

			// Cache miss — continue the request, then capture the response body.
			// We use fetch.ContinueRequest which lets Chrome proceed, then
			// intercept the response via a second EventRequestPaused with
			// a ResponseStatusCode set. We handle that in the same listener.
			if p.ResponseStatusCode != 0 {
				// This is the response phase of an intercepted request.
				c.captureResponse(ctx, p)
				return
			}

			// Request phase — enable response interception for this request.
			continueParams := fetch.ContinueRequest(p.RequestID)
			if err := chromedp.Run(ctx, continueParams); err != nil {
				c.log.Debug("continue (miss) failed", zap.String("url", requestURL), zap.Error(err))
			}
		}(paused)
	})
}

// captureResponse retrieves the body of a completed response and stores it.
func (c *AssetCache) captureResponse(ctx context.Context, p *fetch.EventRequestPaused) {
	if p.ResponseStatusCode < 200 || p.ResponseStatusCode >= 300 {
		// Don't cache error responses.
		chromedp.Run(ctx, fetch.ContinueRequest(p.RequestID))
		return
	}

	var body []byte
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		body, err = fetch.GetResponseBody(p.RequestID).Do(ctx)
		return err
	}))

	// Always continue regardless of whether we captured the body.
	chromedp.Run(ctx, fetch.ContinueRequest(p.RequestID))

	if err != nil {
		return
	}
	bodyBytes := body

	// Determine content type from response headers.
	contentType := "application/octet-stream"
	for _, h := range p.ResponseHeaders {
		if strings.EqualFold(h.Name, "content-type") {
			contentType = h.Value
			break
		}
	}

	c.Put(p.Request.URL, bodyBytes, contentType)
}

func (c *AssetCache) fetchAndStore(ctx context.Context, client *http.Client, rawURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Put(rawURL, data, contentType)
	return nil
}

// normalizeURL strips query strings and fragments for cache keying.
func normalizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// isStaticAsset returns true if the URL looks like a cacheable asset.
func isStaticAsset(rawURL string) bool {
	lower := strings.ToLower(rawURL)
	extensions := []string{
		".woff", ".woff2", ".ttf", ".otf", // fonts
		".png", ".jpg", ".jpeg", ".webp", ".gif", ".svg", // images
		".mp3", ".wav", ".aac", ".ogg", // audio
		".json", ".css", // data + styles
	}
	for _, ext := range extensions {
		if strings.Contains(lower, ext) {
			return true
		}
	}
	return false
}

// mimeFromPath returns a MIME type guess from a file extension.
func mimeFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	mimes := map[string]string{
		".woff":  "font/woff",
		".woff2": "font/woff2",
		".ttf":   "font/ttf",
		".otf":   "font/otf",
		".png":   "image/png",
		".jpg":   "image/jpeg",
		".jpeg":  "image/jpeg",
		".webp":  "image/webp",
		".svg":   "image/svg+xml",
		".gif":   "image/gif",
		".mp3":   "audio/mpeg",
		".wav":   "audio/wav",
		".aac":   "audio/aac",
		".json":  "application/json",
		".css":   "text/css",
	}
	if m, ok := mimes[ext]; ok {
		return m
	}
	return "application/octet-stream"
}

// Hash returns a short content hash for an asset — useful for cache busting.
func Hash(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:8])
}
