package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const SchoolSite = "https://www.heliosschool.org"

var excludedPages = []string{
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

type SavedPage struct {
	URL     string `json:"url"`
	Fetched string `json:"fetched"`
	HTML    string `json:"html"`
}

func ExcludedPage(address string) bool {
	u, err := url.Parse(address)
	if err != nil {
		return false
	}
	return slices.Contains(excludedPages, strings.TrimSuffix(u.Path, "/"))
}

var ErrExcluded = errors.New("the page is not one the documents carry")

func (p SavedPage) Build(links *LinkResolver, embeddingModel string) (*Document, error) {
	if ExcludedPage(p.URL) {
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
	main := findNode(root, func(n *html.Node) bool { return n.Data == "main" })
	head := findNode(root, func(n *html.Node) bool { return n.Data == "title" })
	published := findNode(root, func(n *html.Node) bool { return n.Data == "meta" && nodeAttr(n, "name") == "page-published" })
	if main == nil || head == nil || published == nil {
		return nil, fmt.Errorf("%s: the page has no main content, title or publication date", p.URL)
	}
	day, err := time.Parse(time.RFC3339, nodeAttr(published, "content"))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.URL, err)
	}
	stripPageChrome(main)
	body := &strings.Builder{}
	if err := html.Render(body, main); err != nil {
		return nil, fmt.Errorf("%s: %w", p.URL, err)
	}
	markdown, err := Markdown(body.String(), links.Links(base))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.URL, err)
	}
	return finishDocument(&Document{
		Key:      DocumentKey(p.URL),
		Title:    strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(nodeText(head)), "- Helios School")),
		Author:   "Helios School",
		Kind:     DocumentKindPage,
		Channel:  "website",
		Source:   p.URL,
		Model:    embeddingModel,
		Markdown: markdown,
	}, day)
}

func stripPageChrome(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		classes := strings.Fields(nodeAttr(c, "class"))
		if c.Type == html.ElementNode && (c.Data == "nav" || c.Data == "form" ||
			slices.Contains(classes, "fsPageTitle") || slices.Contains(classes, "fsNavigation") || slices.Contains(classes, "fsTabsNav")) {
			n.RemoveChild(c)
		} else {
			stripPageChrome(c)
		}
		c = next
	}
}

func findNode(n *html.Node, match func(*html.Node) bool) *html.Node {
	if n.Type == html.ElementNode && match(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findNode(c, match); found != nil {
			return found
		}
	}
	return nil
}

func nodeAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func nodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	out := ""
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out += nodeText(c)
	}
	return out
}

func readPage(raw []byte) (SavedDocument, error) {
	var p SavedPage
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if p.URL == "" || p.HTML == "" {
		return nil, fmt.Errorf("the page has no address or no text")
	}
	return p, nil
}
