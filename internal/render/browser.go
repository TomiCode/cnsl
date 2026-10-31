package render

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/remote"
)

// Browser owns one long-lived headless Chrome and screenshots pages in short-lived tabs.
// It starts lazily on the first screenshot and restarts if the browser goes away.
type Browser struct {
	url string // remote DevTools URL; empty = launch a local Chrome/Edge
	log *slog.Logger
	sem chan struct{}

	mu        sync.Mutex
	browser   context.Context
	cancelAll func()
}

func NewBrowser(chromeURL string, concurrency int, log *slog.Logger) *Browser {
	return &Browser{url: chromeURL, log: log, sem: make(chan struct{}, max(concurrency, 1))}
}

func (b *Browser) context() (context.Context, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.browser != nil && b.browser.Err() == nil {
		return b.browser, nil
	}

	var (
		alloc       context.Context
		cancelAlloc context.CancelFunc
	)
	if b.url != "" {
		alloc, cancelAlloc = remote.NewAllocator(context.Background(), b.url)
	} else {
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("hide-scrollbars", true),
			chromedp.Flag("font-render-hinting", "none"),
			chromedp.Flag("force-color-profile", "srgb"),
		)
		alloc, cancelAlloc = chromedp.NewExecAllocator(context.Background(), opts...)
	}
	ctx, cancelCtx := chromedp.NewContext(alloc)
	if err := chromedp.Do(ctx); err != nil { // starts or connects to the browser
		cancelCtx()
		cancelAlloc()
		return nil, fmt.Errorf("start browser: %w", err)
	}
	b.browser = ctx
	b.cancelAll = func() { cancelCtx(); cancelAlloc() }
	where := "local"
	if b.url != "" {
		where = b.url
	}
	b.log.Info("headless browser ready", "browser", where)
	return ctx, nil
}

// Screenshot loads url in a viewport of cssW×cssH CSS pixels at the given device pixel ratio
// and returns a PNG of the viewport (cssW·scale × cssH·scale pixels).
func (b *Browser) Screenshot(ctx context.Context, url string, cssW, cssH int, scale float64) ([]byte, error) {
	select {
	case b.sem <- struct{}{}:
		defer func() { <-b.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	browser, err := b.context()
	if err != nil {
		return nil, err
	}
	tab, closeTab := chromedp.NewContext(browser)
	defer closeTab()
	tab, cancel := context.WithTimeout(tab, 30*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel) // the caller's cancellation also ends the tab
	defer stop()

	err = chromedp.Do(tab,
		chromedp.EmulateViewport(int64(cssW), int64(cssH), chromedp.EmulateScale(scale)),
		chromedp.Navigate(url),
		waitReady(),
	)
	var png []byte
	if err == nil {
		png, err = chromedp.Run(tab, chromedp.CaptureScreenshot())
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("render timed out: %w", err)
		}
		return nil, err
	}
	return png, nil
}

// waitReady waits for fonts and for the framework runtime (terminalize) to finish layout.
// The runtime sets window.TRMNL_PLUGINS_READY; pages without it are ready after 10 s at most.
func waitReady() chromedp.Action[chromedp.Void] {
	return chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
		if _, err := chromedp.Evaluate[bool](`document.fonts.ready.then(() => true)`, chromedp.EvalAwaitPromise)(ctx, t); err != nil {
			return err
		}
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			ready, err := chromedp.Evaluate[bool](`window.TRMNL_PLUGINS_READY === true || window.CNSL_NO_RUNTIME === true`)(ctx, t)
			if err != nil {
				return err
			}
			if ready {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(50 * time.Millisecond):
			}
		}
		return nil // render what we have rather than fail
	})
}

func (b *Browser) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cancelAll != nil {
		b.cancelAll()
		b.browser, b.cancelAll = nil, nil
	}
}
