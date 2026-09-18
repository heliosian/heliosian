package artifacts

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/html"

	"heliosian/internal/calendar"
)

const Site = "https://www.heliosschool.org"

var excluded = []string{
	"/about/staff-and-faculty", "/school-calendar",
	"/website-instructions", "/header-14", "/header-14/header-14-instructions",
	"/footer-1", "/footer-1/footer-1/footer-1-instructions",
	"/slideshow-12", "/slideshow-12/slideshow-12/slideshow-12-instructions",
	"/social-panel-2", "/social-panel-2/social-panel-2/social-2-instructions",
	"/showcase-6", "/showcase-6/showcase-6/showcase-6-instructions",
	"/mission-4", "/mission-4/mission-4/mission-statement-4-instructions", "/mission-4/mission-4/mission-4-video-option",
	"/login", "/search-results", "/site-map", "/site-map23", "/404-page-not-found", "/faculty-portal",
	"/privacy-policy", "/privacy-policy23", "/accessibility-statement", "/accessibility23",
	"/facebook-ad-1", "/summercamps-clone-test", "/admissions/admissions/steps-admissions",
}

type Page struct {
	URL     string `json:"url"`
	Fetched string `json:"fetched"`
	HTML    string `json:"html"`
}

func Excluded(address string) bool {
	u, err := url.Parse(address)
	if err != nil {
		return false
	}
	return slices.Contains(excluded, strings.TrimSuffix(u.Path, "/"))
}

var ErrExcluded = errors.New("the page is not one the documents carry")

func BuildPage(p Page, model string) (*Document, error) {
	if Excluded(p.URL) {
		return nil, fmt.Errorf("%s: %w", p.URL, ErrExcluded)
	}
	base, err := url.Parse(p.URL)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.URL, err)
	}
	root, err := html.Parse(strings.NewReader(p.HTML))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.URL, err)
	}
	main := find(root, func(n *html.Node) bool { return n.Data == "main" })
	head := find(root, func(n *html.Node) bool { return n.Data == "title" })
	published := find(root, func(n *html.Node) bool { return n.Data == "meta" && attr(n, "name") == "page-published" })
	if main == nil || head == nil || published == nil {
		return nil, fmt.Errorf("%s: the page has no main content, title or publication date", p.URL)
	}
	day, err := time.Parse(time.RFC3339, attr(published, "content"))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.URL, err)
	}
	strip(main)
	body := &strings.Builder{}
	if err := html.Render(body, main); err != nil {
		return nil, fmt.Errorf("%s: %w", p.URL, err)
	}
	markdown, err := Markdown(body.String(), func(href string) string {
		target, err := base.Parse(href)
		if err != nil {
			return ""
		}
		return target.String()
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.URL, err)
	}
	if markdown = strings.TrimSpace(markdown); markdown == "" {
		return nil, fmt.Errorf("%s: %w", p.URL, ErrNoWords)
	}
	title := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text(head)), "- Helios School"))
	doc := &Document{
		Key:      Key(p.URL),
		Title:    title,
		Date:     day.In(calendar.Location).Format(calendar.DateFormat),
		Author:   "Helios School",
		Kind:     KindPage,
		Channel:  "website",
		Source:   p.URL,
		Model:    model,
		Markdown: markdown,
	}
	doc.Chunks = Chunks(markdown)
	return doc, nil
}

func strip(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		classes := strings.Fields(attr(c, "class"))
		if c.Type == html.ElementNode && (c.Data == "nav" || c.Data == "form" ||
			slices.Contains(classes, "fsPageTitle") || slices.Contains(classes, "fsNavigation") || slices.Contains(classes, "fsTabsNav")) {
			n.RemoveChild(c)
		} else {
			strip(c)
		}
		c = next
	}
}

func find(n *html.Node, match func(*html.Node) bool) *html.Node {
	if n.Type == html.ElementNode && match(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := find(c, match); found != nil {
			return found
		}
	}
	return nil
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func text(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	out := ""
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out += text(c)
	}
	return out
}

func ReadPage(path string) (Page, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Page{}, err
	}
	var p Page
	if err := json.Unmarshal(raw, &p); err != nil {
		return Page{}, fmt.Errorf("%s: %w", path, err)
	}
	if p.URL == "" || p.HTML == "" {
		return Page{}, fmt.Errorf("%s: the page has no address or no text", path)
	}
	return p, nil
}
