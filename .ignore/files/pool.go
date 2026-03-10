package browser

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/chromedp"
	"go.uber.org/zap"
)

// Browser wraps a single persistent Chrome instance.
type Browser struct {
	id      int
	ctx     context.Context
	cancel  context.CancelFunc
	healthy atomic.Bool
	mu      sync.Mutex
}

// ID returns the browser's pool index.
func (b *Browser) ID() int { return b.id }

// Context returns the chromedp context for this browser.
func (b *Browser) Context() context.Context { return b.ctx }

// IsHealthy reports whether the browser is alive.
func (b *Browser) IsHealthy() bool { return b.healthy.Load() }

// Pool manages a fixed set of persistent browser instances.
// Browsers are pre-warmed at startup and reused across render jobs.
type Pool struct {
	available chan *Browser
	all       []*Browser
	size      int
	opts      PoolOptions
	log       *zap.Logger
	mu        sync.Mutex
	closed    bool
}

// PoolOptions configures the browser pool.
type PoolOptions struct {
	// Size is the number of concurrent browser instances. Defaults to 4.
	Size int

	// ChromeFlags are additional flags passed to Chrome.
	// Common additions: "--disable-web-security" for local dev CORS.
	ChromeFlags []string

	// HeadlesNew forces --headless=new (Chrome 112+). Recommended.
	HeadlessNew bool

	// WindowWidth / WindowHeight set the browser viewport.
	// Should match your composition dimensions.
	WindowWidth  int
	WindowHeight int

	// AcquireTimeout is how long Acquire() waits for a free browser.
	// Defaults to 30s.
	AcquireTimeout time.Duration
}

func (o *PoolOptions) defaults() {
	if o.Size == 0 {
		o.Size = 4
	}
	if o.WindowWidth == 0 {
		o.WindowWidth = 1080
	}
	if o.WindowHeight == 0 {
		o.WindowHeight = 1920
	}
	if o.AcquireTimeout == 0 {
		o.AcquireTimeout = 30 * time.Second
	}
}

// NewPool creates and warms a browser pool.
// All browsers are launched before returning.
func NewPool(ctx context.Context, opts PoolOptions, log *zap.Logger) (*Pool, error) {
	opts.defaults()

	p := &Pool{
		available: make(chan *Browser, opts.Size),
		all:       make([]*Browser, 0, opts.Size),
		size:      opts.Size,
		opts:      opts,
		log:       log,
	}

	for i := 0; i < opts.Size; i++ {
		b, err := p.launch(ctx, i)
		if err != nil {
			p.Close()
			return nil, fmt.Errorf("launching browser %d: %w", i, err)
		}
		p.all = append(p.all, b)
		p.available <- b
		log.Info("browser launched", zap.Int("id", i))
	}

	return p, nil
}

// launch starts a single Chrome instance and returns it.
func (p *Pool) launch(parentCtx context.Context, id int) (*Browser, error) {
	allocOpts := chromedp.DefaultExecAllocatorOptions[:]

	// Core flags for headless video rendering.
	allocOpts = append(allocOpts,
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", false), // keep GPU for canvas perf
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-sync", true),
		chromedp.WindowSize(p.opts.WindowWidth, p.opts.WindowHeight),
	)

	if p.opts.HeadlessNew {
		allocOpts = append(allocOpts, chromedp.Flag("headless", "new"))
	}

	for _, flag := range p.opts.ChromeFlags {
		allocOpts = append(allocOpts, chromedp.Flag(flag, true))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(parentCtx, allocOpts...)

	browserCtx, browserCancel := chromedp.NewContext(allocCtx,
		chromedp.WithLogf(func(format string, args ...interface{}) {
			p.log.Sugar().Debugf("[chrome:%d] "+format, append([]interface{}{id}, args...)...)
		}),
	)

	// Run a no-op to actually launch the process.
	if err := chromedp.Run(browserCtx); err != nil {
		allocCancel()
		browserCancel()
		return nil, fmt.Errorf("starting chrome: %w", err)
	}

	b := &Browser{
		id:     id,
		ctx:    browserCtx,
		cancel: func() { browserCancel(); allocCancel() },
	}
	b.healthy.Store(true)
	return b, nil
}

// Acquire checks out a healthy browser from the pool.
// Blocks until one is available or the timeout elapses.
func (p *Pool) Acquire(ctx context.Context) (*Browser, error) {
	timeout := time.After(p.opts.AcquireTimeout)
	for {
		select {
		case b := <-p.available:
			if b.IsHealthy() {
				return b, nil
			}
			// Unhealthy — try to replace it.
			p.log.Warn("replacing unhealthy browser", zap.Int("id", b.id))
			b.cancel()
			fresh, err := p.launch(ctx, b.id)
			if err != nil {
				p.log.Error("failed to replace browser", zap.Int("id", b.id), zap.Error(err))
				// Put a dead browser back so the channel stays full,
				// but keep looping — the next iteration will retry.
				p.available <- b
				continue
			}
			p.mu.Lock()
			p.all[b.id] = fresh
			p.mu.Unlock()
			return fresh, nil

		case <-timeout:
			return nil, fmt.Errorf("pool: timed out waiting for available browser after %s", p.opts.AcquireTimeout)

		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// Release returns a browser to the pool.
// If the browser is marked unhealthy, it will be replaced on next Acquire.
func (p *Pool) Release(b *Browser) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.available <- b
	}
}

// MarkUnhealthy flags a browser for replacement on next Acquire.
// Call this when a render task fails in a way that may have corrupted
// the browser's page state.
func (p *Pool) MarkUnhealthy(b *Browser) {
	b.healthy.Store(false)
	p.log.Warn("browser marked unhealthy", zap.Int("id", b.id))
}

// Len returns the total pool size.
func (p *Pool) Len() int { return p.size }

// Close shuts down all browser instances.
func (p *Pool) Close() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()

	for _, b := range p.all {
		if b != nil {
			b.cancel()
		}
	}
	p.log.Info("browser pool closed")
}
