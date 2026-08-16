// Command screenshot captures a page from the local dev server as a PNG.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func main() {
	url := flag.String("url", "http://localhost:8080/people", "page to capture")
	out := flag.String("out", "screenshots/capture.png", "output png path")
	wait := flag.String("wait", "body", "css selector that must be visible before capturing")
	remote := flag.Bool("remote", false, "attach to the capture browser on localhost:9222 instead of launching headless chrome")
	cookie := flag.String("cookie", "", "name=value cookie to set for localhost before navigating")
	click := flag.String("click", "", "css selector to click after the wait selector appears")
	flag.Parse()
	ctx := context.Background()
	if *remote {
		var cancelAllocator context.CancelFunc
		ctx, cancelAllocator = chromedp.NewRemoteAllocator(ctx, "http://localhost:9222")
		defer cancelAllocator()
	}
	ctx, cancelBrowser := chromedp.NewContext(ctx)
	defer cancelBrowser()
	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	defer cancelTimeout()
	actions := []chromedp.Action{chromedp.EmulateViewport(1280, 800)}
	if *cookie != "" {
		name, value, ok := strings.Cut(*cookie, "=")
		if !ok {
			log.Fatal("[ERROR] -cookie must be name=value")
		}
		actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
			return network.SetCookie(name, value).WithDomain("localhost").WithPath("/").Do(ctx)
		}))
	}
	var png []byte
	actions = append(actions,
		chromedp.Navigate(*url),
		chromedp.WaitVisible(*wait, chromedp.ByQuery),
	)
	if *click != "" {
		actions = append(actions,
			chromedp.Click(*click, chromedp.ByQuery),
			chromedp.Sleep(500*time.Millisecond),
		)
	}
	actions = append(actions, chromedp.FullScreenshot(&png, 90))
	if err := chromedp.Run(ctx, actions...); err != nil {
		log.Fatalf("[ERROR] capture %s: %v", *url, err)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatalf("[ERROR] create output dir: %v", err)
	}
	if err := os.WriteFile(*out, png, 0o644); err != nil {
		log.Fatalf("[ERROR] write %s: %v", *out, err)
	}
	log.Printf("captured %s to %s", *url, *out)
}
