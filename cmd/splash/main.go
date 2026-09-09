// Command splash downloads an original app's ios splash screens from its Glide manifest into a brand directory.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	manifest := flag.String("manifest", "", "the captured Glide manifest json")
	out := flag.String("out", "", "directory to write the splash pngs into, e.g. web/public/home/brand/splash")
	flag.Parse()
	if *manifest == "" || *out == "" {
		log.Fatal("[ERROR] --manifest and --out are required")
	}
	raw, err := os.ReadFile(*manifest)
	if err != nil {
		log.Fatalf("[ERROR] read manifest: %v", err)
	}
	var parsed struct {
		Head string `json:"glidePWAAddToHead"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		log.Fatalf("[ERROR] parse manifest: %v", err)
	}
	linkRE := regexp.MustCompile(`<link rel="apple-touch-startup-image" media="([^"]+)" href="([^"]+)"`)
	links := linkRE.FindAllStringSubmatch(parsed.Head, -1)
	if len(links) == 0 {
		log.Fatal("[ERROR] no splash links found in manifest")
	}
	mediaRE := regexp.MustCompile(`device-width: (\d+)px\) and \(device-height: (\d+)px\) and \(-webkit-device-pixel-ratio: (\d+)\) and \(orientation: (\w+)\)`)
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatalf("[ERROR] create %s: %v", *out, err)
	}
	for _, link := range links {
		media, href := link[1], link[2]
		mm := mediaRE.FindStringSubmatch(media)
		if mm == nil {
			log.Fatalf("[ERROR] unparsed media query: %s", media)
		}
		name := fmt.Sprintf("splash-%sx%s-%sx-%s.png", mm[1], mm[2], mm[3], mm[4])
		resp, err := http.Get(strings.ReplaceAll(href, " ", "%20"))
		if err != nil {
			log.Fatalf("[ERROR] fetch %s: %v", href, err)
		}
		if resp.StatusCode != http.StatusOK {
			log.Fatalf("[ERROR] fetch %s: %s", href, resp.Status)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Fatalf("[ERROR] read %s: %v", href, err)
		}
		if err := os.WriteFile(filepath.Join(*out, name), data, 0o644); err != nil {
			log.Fatalf("[ERROR] write %s: %v", name, err)
		}
		fmt.Printf("<link rel=\"apple-touch-startup-image\" media=\"%s\" href=\"/brand/splash/%s\">\n", media, name)
	}
}
