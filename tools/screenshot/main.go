package main

import (
	"flag"
	"log/slog"
	"os"
	"path/filepath"

	"heliosian/internal/capture"
	"heliosian/internal/logging"
)

func main() {
	url := flag.String("url", "https://who.heliosiandev.com:8080/people", "page to capture")
	out := flag.String("out", "local/screenshots/capture.png", "output png path")
	wait := flag.String("wait", "body", "css selector that must be visible before capturing")
	remote := flag.Bool("remote", false, "attach to the capture browser on "+capture.DevTools+" instead of launching headless chrome")
	cookie := flag.String("cookie", "", "name=value cookie(s) to set for the url's host before navigating, ; separated")
	click := flag.String("click", "", "css selector(s) to click after the wait selector appears, several separated by |, each waited for in turn")
	settle := flag.Duration("settle", 0, "extra time to wait after the last click, e.g. 3s")
	width := flag.Int("width", 1280, "viewport width")
	height := flag.Int("height", 800, "viewport height")
	flag.Parse()
	png, err := capture.PNG(capture.Options{URL: *url, Wait: *wait, Remote: *remote, Cookie: *cookie, Click: *click, Settle: *settle, Width: *width, Height: *height})
	if err != nil {
		logging.Fatal("capture", "error", err)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		logging.Fatal("create output dir", "error", err)
	}
	if err := os.WriteFile(*out, png, 0o644); err != nil {
		logging.Fatal("write screenshot", "path", *out, "error", err)
	}
	slog.Info("captured", "url", *url, "path", *out)
}
