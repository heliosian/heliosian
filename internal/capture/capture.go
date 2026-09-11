// Package capture screenshots pages with a headless (or remote) Chrome.
package capture

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

type Options struct {
	URL    string
	Wait   string
	Remote bool
	Cookie string
	// Click is one selector, or several separated by "|", each waited for
	// and clicked in turn; Settle is how long to wait after the last.
	Click  string
	Settle time.Duration
	// Width and Height set the viewport; zero means 1280x800.
	Width, Height int
}

// PNG captures one page as a full-page screenshot at a 1280×800 viewport.
func PNG(opts Options) ([]byte, error) {
	ctx := context.Background()
	var cancelAllocator context.CancelFunc
	if opts.Remote {
		ctx, cancelAllocator = chromedp.NewRemoteAllocator(ctx, "http://localhost:9222")
	} else {
		ctx, cancelAllocator = chromedp.NewExecAllocator(ctx, append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("ignore-certificate-errors", true))...)
	}
	defer cancelAllocator()
	ctx, cancelBrowser := chromedp.NewContext(ctx)
	defer cancelBrowser()
	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	defer cancelTimeout()
	width, height := opts.Width, opts.Height
	if width == 0 || height == 0 {
		width, height = 1280, 800
	}
	actions := []chromedp.Action{chromedp.EmulateViewport(int64(width), int64(height))}
	if opts.Cookie != "" {
		name, value, ok := strings.Cut(opts.Cookie, "=")
		if !ok {
			return nil, fmt.Errorf("cookie must be name=value")
		}
		actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
			return network.SetCookie(name, value).WithDomain("who.local.heliosian.com").WithPath("/").Do(ctx)
		}))
	}
	var png []byte
	actions = append(actions,
		chromedp.Navigate(opts.URL),
		chromedp.WaitVisible(opts.Wait, chromedp.ByQuery),
	)
	if opts.Click != "" {
		for _, sel := range strings.Split(opts.Click, "|") {
			sel = strings.TrimSpace(sel)
			actions = append(actions,
				chromedp.WaitVisible(sel, chromedp.ByQuery),
				chromedp.Click(sel, chromedp.ByQuery),
				chromedp.Sleep(500*time.Millisecond),
			)
		}
	}
	if opts.Settle > 0 {
		actions = append(actions, chromedp.Sleep(opts.Settle))
	}
	actions = append(actions, chromedp.FullScreenshot(&png, 90))
	if err := chromedp.Run(ctx, actions...); err != nil {
		return nil, fmt.Errorf("capture %s: %w", opts.URL, err)
	}
	return png, nil
}
