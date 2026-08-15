// Command screenshot captures a page from the local dev server as a PNG.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/chromedp"
)

func main() {
	url := flag.String("url", "http://localhost:8080/directory/", "page to capture")
	out := flag.String("out", "screenshots/capture.png", "output png path")
	wait := flag.String("wait", "body", "css selector that must be visible before capturing")
	flag.Parse()
	ctx, cancelBrowser := chromedp.NewContext(context.Background())
	defer cancelBrowser()
	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	defer cancelTimeout()
	var png []byte
	err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1280, 800),
		chromedp.Navigate(*url),
		chromedp.WaitVisible(*wait, chromedp.ByQuery),
		chromedp.FullScreenshot(&png, 90),
	)
	if err != nil {
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
