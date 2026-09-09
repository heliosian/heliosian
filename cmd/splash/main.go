// Command splash extracts the original app's ios splash screens into web/who/brand/splash.
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
)

func main() {
	raw, err := os.ReadFile("screenshots/who-old/brand/page-source.html")
	if err != nil {
		log.Fatalf("[ERROR] read page source: %v", err)
	}
	blobs := regexp.MustCompile(`decodeURIComponent\("([^"]+)"\)`).FindAllStringSubmatch(string(raw), -1)
	links := [][]string{}
	linkRE := regexp.MustCompile(`<link rel="apple-touch-startup-image" media="([^"]+)" href="([^"]+)"`)
	for _, blob := range blobs {
		decoded, err := url.PathUnescape(blob[1])
		if err != nil {
			log.Fatalf("[ERROR] decode blob: %v", err)
		}
		decoded = strings.ReplaceAll(decoded, `\"`, `"`)
		links = append(links, linkRE.FindAllStringSubmatch(decoded, -1)...)
	}
	if len(links) == 0 {
		log.Fatal("[ERROR] no splash links found in page source")
	}
	mediaRE := regexp.MustCompile(`device-width: (\d+)px\) and \(device-height: (\d+)px\) and \(-webkit-device-pixel-ratio: (\d+)\) and \(orientation: (\w+)\)`)
	if err := os.MkdirAll("web/who/brand/splash", 0o755); err != nil {
		log.Fatalf("[ERROR] create splash dir: %v", err)
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
		if err := os.WriteFile("web/who/brand/splash/"+name, data, 0o644); err != nil {
			log.Fatalf("[ERROR] write %s: %v", name, err)
		}
		fmt.Printf("<link rel=\"apple-touch-startup-image\" media=\"%s\" href=\"/brand/splash/%s\">\n", media, name)
	}
}
