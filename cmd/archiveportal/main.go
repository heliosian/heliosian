// Command archiveportal saves the school-wide and classroom pages of the Veracross parent portal and of the HELP site, and the Google documents they link, under imports/portal, reading them through the signed-in capture browser.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

const (
	portal = "https://portals.veracross.com/heliosschool/parent"
	help   = "https://sites.google.com/heliosns.org/help/"
	out    = "imports/portal"
)

var skippedPages = []string{"School-Year-Calendar"}

var personal = regexp.MustCompile(`(?i)lookbook|meet your classmates`)

var (
	publishedDoc    = regexp.MustCompile(`^https://docs\.google\.com/document/d/e/([\w-]+)/pub`)
	publishedSlides = regexp.MustCompile(`^https://docs\.google\.com/presentation/d/e/`)
	googleDoc       = regexp.MustCompile(`^https://docs\.google\.com/document/d/([\w-]+)`)
	googleSlides    = regexp.MustCompile(`^https://docs\.google\.com/presentation/d/([\w-]+)`)
	driveFile       = regexp.MustCompile(`^https://drive\.google\.com/file/d/([\w-]+)`)
)

type Doc struct {
	URL        string `json:"url"`
	Title      string `json:"title"`
	LinkedFrom string `json:"linkedFrom,omitempty"`
	Fetched    string `json:"fetched"`
	Format     string `json:"format"`
	Body       string `json:"body"`
}

type link struct {
	Text string `json:"text"`
	Href string `json:"href"`
}

type rendered struct {
	HTML  string `json:"html"`
	Links []link `json:"links"`
}

const navScript = `[...document.querySelectorAll("a")].map(a => ({text: a.textContent.replace(/\s+/g, " ").trim(), href: a.href}))`

// The portal draws most pages from data it loads after the page itself, so
// the content is read once it has stopped changing.
const renderScript = `new Promise(resolve => {
	let last = -1, same = 0;
	const tick = () => {
		const parts = [...document.querySelectorAll(SELECTOR)];
		const n = parts.reduce((sum, c) => sum + c.innerHTML.length, 0);
		same = n > 0 && n === last ? same + 1 : 0;
		last = n;
		if (same < 4) {
			setTimeout(tick, 500);
			return;
		}
		const links = [];
		for (const c of parts) {
			c.querySelectorAll("script").forEach(s => s.remove());
			for (const a of c.querySelectorAll("a")) {
				links.push({text: a.textContent.replace(/\s+/g, " ").trim(), href: a.href});
			}
			for (const f of c.querySelectorAll("iframe")) {
				const src = new URL(f.src, location.href);
				links.push({text: "", href: src.searchParams.get("url") || f.src});
			}
			for (const e of c.querySelectorAll("[data-embed-open-url]")) {
				links.push({text: e.textContent.replace(/\s+/g, " ").trim(), href: e.getAttribute("data-embed-open-url")});
			}
		}
		resolve({html: parts.map(c => c.innerHTML).join("\n"), links});
	};
	tick();
})`

func main() {
	id, err := newTab()
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	allocCtx, _ := chromedp.NewRemoteAllocator(context.Background(), "http://localhost:9222")
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(id)))
	defer cancel()
	ctx, cancelTimeout := context.WithTimeout(ctx, 10*time.Minute)
	defer cancelTimeout()
	if err := os.MkdirAll(out, 0o755); err != nil {
		log.Fatalf("[ERROR] %v", err)
	}

	linked := []Doc{}
	saved, failed := 0, 0
	for _, site := range []struct{ start, pages, name, selector string }{
		{portal, portal + "/pages/", "portal", ".app-container"},
		{help + "welcome", help, "help", "section"},
	} {
		nav := []link{}
		if err := chromedp.Run(ctx, chromedp.Navigate(site.start), chromedp.Evaluate(navScript, &nav)); err != nil {
			log.Fatalf("[ERROR] %s: %v", site.start, err)
		}
		pages := []link{}
		seen := map[string]bool{}
		for _, l := range nav {
			l.Href, _, _ = strings.Cut(l.Href, "?")
			if !strings.HasPrefix(l.Href, site.pages) || l.Text == "" || seen[l.Href] || slices.Contains(skippedPages, path(l.Href)) {
				continue
			}
			seen[l.Href] = true
			pages = append(pages, l)
		}
		log.Printf("%d %s pages", len(pages), site.name)
		script := strings.Replace(renderScript, "SELECTOR", strconv.Quote(site.selector), 1)
		for _, page := range pages {
			r := rendered{}
			err := chromedp.Run(ctx, chromedp.Navigate(page.Href), chromedp.Evaluate(script, &r, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
				return p.WithAwaitPromise(true)
			}))
			if err != nil {
				log.Printf("[ERROR] %s: %v", page.Href, err)
				failed++
				continue
			}
			write(site.name+"-"+slug(path(page.Href)), Doc{URL: page.Href, Title: page.Text, Format: "html", Body: r.HTML})
			saved++
			for _, l := range r.Links {
				linked = append(linked, Doc{URL: l.Href, Title: l.Text, LinkedFrom: page.Href})
			}
		}
	}

	downloads, err := newDownloader(ctx)
	if err != nil {
		log.Fatalf("[ERROR] downloads: %v", err)
	}
	defer downloads.close(ctx)
	handled := map[string]bool{}
	skipped := 0
	for _, doc := range linked {
		if publishedSlides.MatchString(doc.URL) {
			if !handled[doc.URL] {
				log.Printf("%s: a published deck carries its words only as pictures; skipped", doc.URL)
				handled[doc.URL] = true
				skipped++
			}
			continue
		}
		name, fetch := classify(doc.URL)
		if fetch == nil || handled[name] {
			continue
		}
		handled[name] = true
		if personal.MatchString(doc.Title) {
			log.Printf("%s (%q): personal; skipped", doc.URL, doc.Title)
			skipped++
			continue
		}
		result, err := fetch(ctx, downloads)
		if err != nil {
			log.Printf("[ERROR] %s: %v", doc.URL, err)
			failed++
			continue
		}
		result.LinkedFrom = doc.LinkedFrom
		write(name, result)
		saved++
	}
	log.Printf("%d saved, %d skipped, %d failed", saved, skipped, failed)
	if failed > 0 {
		downloads.close(ctx)
		os.Exit(1)
	}
}

func classify(address string) (string, func(context.Context, *downloader) (Doc, error)) {
	if m := publishedDoc.FindStringSubmatch(address); m != nil {
		source := "https://docs.google.com/document/d/e/" + m[1] + "/pub"
		return "doc-" + m[1], func(ctx context.Context, _ *downloader) (Doc, error) {
			title, body, err := inBrowser(ctx, source, source)
			if err != nil {
				return Doc{}, err
			}
			return Doc{URL: source, Title: title, Format: "html", Body: body}, nil
		}
	}
	if m := googleDoc.FindStringSubmatch(address); m != nil {
		edit := "https://docs.google.com/document/d/" + m[1] + "/edit"
		return "doc-" + m[1], func(ctx context.Context, _ *downloader) (Doc, error) {
			title, body, err := inBrowser(ctx, edit, "/document/d/"+m[1]+"/export?format=html")
			if err != nil {
				return Doc{}, err
			}
			return Doc{URL: edit, Title: strings.TrimSuffix(title, " - Google Docs"), Format: "html", Body: body}, nil
		}
	}
	if m := googleSlides.FindStringSubmatch(address); m != nil {
		edit := "https://docs.google.com/presentation/d/" + m[1] + "/edit"
		return "slides-" + m[1], func(ctx context.Context, d *downloader) (Doc, error) {
			name, body, err := d.get(ctx, "https://docs.google.com/presentation/d/"+m[1]+"/export/txt")
			if err != nil {
				return Doc{}, err
			}
			return Doc{URL: edit, Title: strings.TrimSuffix(name, ".txt"), Format: "text", Body: string(body)}, nil
		}
	}
	if m := driveFile.FindStringSubmatch(address); m != nil {
		return "file-" + m[1], func(ctx context.Context, d *downloader) (Doc, error) {
			name, body, err := d.get(ctx, "https://drive.google.com/uc?export=download&id="+m[1])
			if err != nil {
				return Doc{}, err
			}
			if !bytes.HasPrefix(body, []byte("%PDF")) {
				return Doc{}, fmt.Errorf("%s is not a pdf", name)
			}
			text, err := pdfText(body)
			if err != nil {
				return Doc{}, err
			}
			return Doc{URL: "https://drive.google.com/file/d/" + m[1] + "/view", Title: strings.TrimSuffix(name, ".pdf"), Format: "text", Body: text}, nil
		}
	}
	return "", nil
}

type downloader struct {
	dir      string
	began    chan *browser.EventDownloadWillBegin
	finished chan *browser.EventDownloadProgress
}

func newDownloader(ctx context.Context) (*downloader, error) {
	dir, err := os.MkdirTemp("", "archiveportal")
	if err != nil {
		return nil, err
	}
	d := &downloader{dir: dir, began: make(chan *browser.EventDownloadWillBegin, 16), finished: make(chan *browser.EventDownloadProgress, 16)}
	chromedp.ListenTarget(ctx, func(ev any) {
		switch e := ev.(type) {
		case *browser.EventDownloadWillBegin:
			d.began <- e
		case *browser.EventDownloadProgress:
			if e.State != browser.DownloadProgressStateInProgress {
				d.finished <- e
			}
		}
	})
	err = chromedp.Run(ctx, browser.SetDownloadBehavior(browser.SetDownloadBehaviorBehaviorAllowAndName).WithDownloadPath(dir).WithEventsEnabled(true))
	return d, err
}

func (d *downloader) close(ctx context.Context) {
	if err := chromedp.Run(ctx, browser.SetDownloadBehavior(browser.SetDownloadBehaviorBehaviorDefault)); err != nil {
		log.Printf("[ERROR] restore the browser's downloads: %v", err)
	}
	os.RemoveAll(d.dir)
}

func (d *downloader) get(ctx context.Context, address string) (string, []byte, error) {
	if err := chromedp.Run(ctx, chromedp.Navigate(address)); err != nil && !strings.Contains(err.Error(), "net::ERR_ABORTED") {
		return "", nil, err
	}
	var began *browser.EventDownloadWillBegin
	select {
	case began = <-d.began:
	case <-time.After(30 * time.Second):
		return "", nil, fmt.Errorf("%s: no download began, so not signed in or not shared with this account", address)
	}
	for {
		select {
		case e := <-d.finished:
			if e.GUID != began.GUID {
				continue
			}
			if e.State != browser.DownloadProgressStateCompleted {
				return "", nil, fmt.Errorf("%s: download %s", address, e.State)
			}
			file := filepath.Join(d.dir, e.GUID)
			body, err := os.ReadFile(file)
			os.Remove(file)
			return began.SuggestedFilename, body, err
		case <-time.After(time.Minute):
			return "", nil, fmt.Errorf("%s: download never finished", address)
		}
	}
}

func pdfText(pdf []byte) (string, error) {
	cmd := exec.Command("pdftotext", "-", "-")
	cmd.Stdin = bytes.NewReader(pdf)
	text, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("pdftotext: %w", err)
	}
	return string(text), nil
}

// Google answers a signed-in export only to the browser that holds the
// session, so the export is fetched from a page of the document's own origin.
func inBrowser(ctx context.Context, page, resource string) (string, string, error) {
	r := struct {
		Title  string `json:"title"`
		Body   string `json:"body"`
		URL    string `json:"url"`
		Status int    `json:"status"`
	}{}
	script := fmt.Sprintf(`fetch(%q, {credentials: "include"}).then(async r => ({title: document.title, body: await r.text(), url: r.url, status: r.status}))`, resource)
	err := chromedp.Run(ctx, chromedp.Navigate(page), chromedp.Evaluate(script, &r, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
		return p.WithAwaitPromise(true)
	}))
	if err != nil {
		return "", "", err
	}
	if r.Status != http.StatusOK || strings.Contains(r.URL, "accounts.google.com") || strings.Contains(r.Title, "Sign-in") {
		return "", "", fmt.Errorf("%s answered %d at %s (%q): not signed in, or not shared with this account", resource, r.Status, r.URL, r.Title)
	}
	return strings.TrimSpace(r.Title), r.Body, nil
}

func write(name string, doc Doc) {
	doc.Fetched = time.Now().UTC().Format(time.RFC3339)
	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	file := filepath.Join(out, name+".json")
	if err := os.WriteFile(file, encoded, 0o644); err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	log.Printf("%q saved as %s", doc.Title, file)
}

func path(address string) string {
	u, err := url.Parse(address)
	if err != nil {
		return ""
	}
	return filepath.Base(u.Path)
}

func slug(name string) string {
	return strings.ToLower(strings.Trim(strings.ReplaceAll(name, " ", "-"), "-"))
}

func newTab() (string, error) {
	req, err := http.NewRequest(http.MethodPut, "http://localhost:9222/json/new?url=about:blank", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("capture browser not reachable on localhost:9222, run cmd/capturebrowser first: %w", err)
	}
	defer resp.Body.Close()
	t := struct {
		ID string `json:"id"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return "", err
	}
	if t.ID == "" {
		return "", fmt.Errorf("capture browser did not create a tab")
	}
	return t.ID, nil
}
