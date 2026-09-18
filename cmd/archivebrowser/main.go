// Command archiveportal crawls a signed-in site through the capture browser and saves its pages, and the Google documents they link, for importartifacts.
package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
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

var skippedPages = []string{"School-Year-Calendar"}

var personal = regexp.MustCompile(`(?i)lookbook|meet your classmates`)

var (
	publishedDoc    = regexp.MustCompile(`^https://docs\.google\.com/document/d/e/([\w-]+)/pub`)
	publishedSlides = regexp.MustCompile(`^https://docs\.google\.com/presentation/d/e/([\w-]+)`)
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
	Title string `json:"title"`
	HTML  string `json:"html"`
	Links []link `json:"links"`
	Nav   []link `json:"nav"`
}

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
		const nav = [...document.querySelectorAll("a")].map(a => ({text: a.textContent.replace(/\s+/g, " ").trim(), href: a.href}));
		resolve({title: document.title, html: parts.map(c => c.innerHTML).join("\n"), links, nav});
	};
	tick();
})`

func main() {
	start := flag.String("start", "", "the page the crawl begins at")
	prefix := flag.String("prefix", "", "the address every page of the site begins with; only these pages are followed and saved")
	selector := flag.String("selector", "", "the CSS selector of a page's content")
	out := flag.String("out", "", "the folder the pages and documents are saved in")
	flag.Parse()
	if *start == "" || *prefix == "" || *selector == "" || *out == "" {
		log.Fatal("[ERROR] --start, --prefix, --selector and --out are all required")
	}
	id, err := newTab()
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	allocCtx, _ := chromedp.NewRemoteAllocator(context.Background(), "http://localhost:9222")
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(id)))
	defer cancel()
	ctx, cancelTimeout := context.WithTimeout(ctx, 10*time.Minute)
	defer cancelTimeout()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatalf("[ERROR] %v", err)
	}

	script := strings.Replace(renderScript, "SELECTOR", strconv.Quote(*selector), 1)
	titles := map[string]string{}
	queued := map[string]bool{*start: true}
	queue := []string{*start}
	linked := []Doc{}
	saved, failed := 0, 0
	for len(queue) > 0 {
		address := queue[0]
		queue = queue[1:]
		r := rendered{}
		err := chromedp.Run(ctx, chromedp.Navigate(address), chromedp.Evaluate(script, &r, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}))
		if err != nil {
			log.Printf("[ERROR] %s: %v", address, err)
			failed++
			continue
		}
		for _, l := range r.Nav {
			href := pageAddress(l.Href)
			if !strings.HasPrefix(href, *prefix) || slices.Contains(skippedPages, path(href)) {
				continue
			}
			if titles[href] == "" {
				titles[href] = l.Text
			}
			if !queued[href] {
				queued[href] = true
				queue = append(queue, href)
			}
		}
		if !strings.HasPrefix(address, *prefix) {
			continue
		}
		name := cmp.Or(slug(strings.ReplaceAll(strings.TrimPrefix(address, *prefix), "/", "-")), "home")
		write(*out, name, Doc{URL: address, Title: cmp.Or(titles[address], r.Title), Format: "html", Body: r.HTML})
		saved++
		for _, l := range r.Links {
			linked = append(linked, Doc{URL: l.Href, Title: l.Text, LinkedFrom: address})
		}
	}
	log.Printf("%d pages saved", saved)

	downloads, err := newDownloader(ctx)
	if err != nil {
		log.Fatalf("[ERROR] downloads: %v", err)
	}
	defer downloads.close(ctx)
	handled := map[string]bool{}
	skipped := 0
	for _, doc := range linked {
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
		write(*out, name, result)
		saved++
	}
	log.Printf("%d saved, %d skipped, %d failed", saved, skipped, failed)
	if failed > 0 {
		downloads.close(ctx)
		os.Exit(1)
	}
}

func classify(address string) (string, func(context.Context, *downloader) (Doc, error)) {
	if m := publishedSlides.FindStringSubmatch(address); m != nil {
		source := "https://docs.google.com/presentation/d/e/" + m[1] + "/pub"
		return "published-" + m[1], func(ctx context.Context, _ *downloader) (Doc, error) {
			return publishedDeck(ctx, source)
		}
	}
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
	dir, err := os.MkdirTemp("", "archivebrowser")
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

var (
	svgData  = regexp.MustCompile(`SK_svgData = '((?:[^'\\]|\\.)*)'`)
	jsEscape = regexp.MustCompile(`\\(x[0-9a-fA-F]{2}|u[0-9a-fA-F]{4}|.)`)
)

func publishedDeck(ctx context.Context, source string) (Doc, error) {
	page := struct {
		Title   string `json:"title"`
		Scripts string `json:"scripts"`
	}{}
	script := `({title: document.title, scripts: [...document.scripts].map(s => s.textContent).join("\n")})`
	if err := chromedp.Run(ctx, chromedp.Navigate(source), chromedp.Evaluate(script, &page)); err != nil {
		return Doc{}, err
	}
	slides := [][]string{}
	for _, m := range svgData.FindAllStringSubmatch(page.Scripts, -1) {
		boxes, err := slideText(unescapeJS(m[1]))
		if err != nil {
			return Doc{}, fmt.Errorf("%s: %w", source, err)
		}
		slides = append(slides, boxes)
	}
	if len(slides) == 0 {
		return Doc{}, fmt.Errorf("%s (%q) has no slides: not signed in, or no longer published", source, page.Title)
	}
	onSlides := map[string]int{}
	for _, boxes := range slides {
		for _, box := range slices.Compact(slices.Sorted(slices.Values(boxes))) {
			onSlides[box]++
		}
	}
	text := []string{}
	for _, boxes := range slides {
		kept := []string{}
		for _, box := range boxes {
			if len(slides) > 2 && onSlides[box]*2 > len(slides) {
				continue
			}
			kept = append(kept, box)
		}
		text = append(text, strings.Join(kept, "\n\n"))
	}
	return Doc{URL: source, Title: strings.TrimSuffix(page.Title, " - Google Slides"), Format: "text", Body: strings.Join(text, "\f")}, nil
}

func unescapeJS(literal string) string {
	return jsEscape.ReplaceAllStringFunc(literal, func(escape string) string {
		if len(escape) > 2 {
			code, _ := strconv.ParseUint(escape[2:], 16, 32)
			return string(rune(code))
		}
		if escape[1] == 'n' {
			return "\n"
		}
		return escape[1:]
	})
}

func slideText(svg string) ([]string, error) {
	decoder := xml.NewDecoder(strings.NewReader(svg))
	decoder.Strict = false
	decoder.Entity = xml.HTMLEntity
	boxes := []string{}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return boxes, nil
		}
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		label := strings.TrimSpace(attr(start, "aria-label"))
		switch {
		case start.Name.Local == "g" && strings.HasPrefix(attr(start, "id"), "a11y-") && label != "":
			boxes = append(boxes, label)
		case start.Name.Local == "a" && len(boxes) > 0 && label == boxes[len(boxes)-1]:
			boxes = boxes[:len(boxes)-1]
		}
	}
}

func attr(e xml.StartElement, name string) string {
	for _, a := range e.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func write(out, name string, doc Doc) {
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

func pageAddress(href string) string {
	href, _, _ = strings.Cut(href, "#")
	href, _, _ = strings.Cut(href, "?")
	return href
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
