// Command screenshot captures a page from the local dev server as a PNG.
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"heliosian/internal/capture"
)

func main() {
	url := flag.String("url", "https://who.local.heliosian.com:8080/people", "page to capture")
	out := flag.String("out", "screenshots/capture.png", "output png path")
	wait := flag.String("wait", "body", "css selector that must be visible before capturing")
	remote := flag.Bool("remote", false, "attach to the capture browser on localhost:9222 instead of launching headless chrome")
	cookie := flag.String("cookie", "", "name=value cookie to set for who.local.heliosian.com before navigating")
	click := flag.String("click", "", "css selector to click after the wait selector appears")
	flag.Parse()
	png, err := capture.PNG(capture.Options{URL: *url, Wait: *wait, Remote: *remote, Cookie: *cookie, Click: *click})
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatalf("[ERROR] create output dir: %v", err)
	}
	if err := os.WriteFile(*out, png, 0o644); err != nil {
		log.Fatalf("[ERROR] write %s: %v", *out, err)
	}
	log.Printf("captured %s to %s", *url, *out)
}
