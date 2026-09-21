// Command archivesite saves every page the school website's sitemap names under local/imports/site, for importartifacts to read into Helios Ask.
package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"heliosian/internal/artifacts"
)

const (
	out = "local/imports/site"
	// The site's robots.txt asks for Crawl-delay: 5.
	crawlDelay = 5 * time.Second
)

func main() {
	client := &http.Client{Timeout: 30 * time.Second}
	site, err := url.Parse(artifacts.Site)
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	body, err := fetch(client, artifacts.Site+"/fs/pages/sitemap")
	if err != nil {
		log.Fatalf("[ERROR] sitemap: %v", err)
	}
	var sitemap struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal(body, &sitemap); err != nil {
		log.Fatalf("[ERROR] sitemap: %v", err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	log.Printf("%d pages in the sitemap", len(sitemap.URLs))
	saved, excluded, away, failed := 0, 0, 0, 0
	fetched := false
	for _, entry := range sitemap.URLs {
		loc := strings.TrimSpace(entry.Loc)
		if artifacts.Excluded(loc) {
			log.Printf("%s: excluded; skipped", loc)
			excluded++
			continue
		}
		if fetched {
			time.Sleep(crawlDelay)
		}
		fetched = true
		resp, err := get(client, loc)
		if err != nil {
			log.Printf("[ERROR] %s: %v", loc, err)
			failed++
			continue
		}
		page, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("[ERROR] %s: %v", loc, err)
			failed++
			continue
		}
		final := resp.Request.URL
		if final.Host != site.Host {
			log.Printf("%s: leaves the site for %s; skipped", loc, final)
			away++
			continue
		}
		if resp.StatusCode != http.StatusOK {
			log.Printf("[ERROR] %s: %s", loc, resp.Status)
			failed++
			continue
		}
		final.RawQuery, final.Fragment = "", ""
		if artifacts.Excluded(final.String()) {
			log.Printf("%s: becomes %s, which is excluded; skipped", loc, final)
			excluded++
			continue
		}
		encoded, err := json.MarshalIndent(artifacts.Page{
			URL:     final.String(),
			Fetched: time.Now().UTC().Format(time.RFC3339),
			HTML:    string(page),
		}, "", "  ")
		if err != nil {
			log.Fatalf("[ERROR] %v", err)
		}
		file := filepath.Join(out, fileName(final)+".json")
		if err := os.WriteFile(file, encoded, 0o644); err != nil {
			log.Fatalf("[ERROR] %v", err)
		}
		log.Printf("%s: saved as %s", loc, file)
		saved++
	}
	log.Printf("%d pages saved, %d excluded, %d leaving the site, %d failed", saved, excluded, away, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// The site answers 406 to a request with no Accept header.
func get(client *http.Client, address string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	return client.Do(req)
}

func fetch(client *http.Client, address string) ([]byte, error) {
	resp, err := get(client, address)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", address, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func fileName(u *url.URL) string {
	name := strings.ReplaceAll(strings.Trim(u.Path, "/"), "/", "-")
	if name == "" {
		return "home"
	}
	return name
}
