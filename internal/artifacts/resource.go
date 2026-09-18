package artifacts

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"heliosian/internal/calendar"
)

const (
	FormatHTML = "html"
	FormatText = "text"
)

type Resource struct {
	URL        string `json:"url"`
	Title      string `json:"title"`
	LinkedFrom string `json:"linkedFrom,omitempty"`
	Fetched    string `json:"fetched"`
	Format     string `json:"format"`
	Body       string `json:"body"`
}

func (r Resource) Key() string {
	return Key(r.URL)
}

func (r Resource) Build(links *Resolver, model string) (*Document, error) {
	return BuildResource(r, links, model)
}

func ReadResource(path string) (Resource, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Resource{}, err
	}
	var r Resource
	if err := json.Unmarshal(raw, &r); err != nil {
		return Resource{}, fmt.Errorf("%s: %w", path, err)
	}
	if r.URL == "" || strings.TrimSpace(r.Title) == "" || r.Fetched == "" || (r.Format != FormatHTML && r.Format != FormatText) {
		return Resource{}, fmt.Errorf("%s: the resource has no address, title, fetch time or known format", path)
	}
	return r, nil
}

func BuildResource(r Resource, links *Resolver, model string) (*Document, error) {
	base, err := url.Parse(r.URL)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", r.URL, err)
	}
	day, err := time.Parse(time.RFC3339, r.Fetched)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", r.URL, err)
	}
	markdown := ""
	switch r.Format {
	case FormatHTML:
		absolute := func(href string) string {
			target, err := base.Parse(href)
			if err != nil {
				return ""
			}
			return unwrapGoogle(target).String()
		}
		hrefs := []string{}
		for _, href := range Hrefs(r.Body) {
			hrefs = append(hrefs, absolute(href))
		}
		links.Warm(hrefs)
		if markdown, err = Markdown(r.Body, func(href string) string { return links.Resolve(absolute(href)) }); err != nil {
			return nil, fmt.Errorf("%s: %w", r.URL, err)
		}
	case FormatText:
		markdown = fromLines(r.Body)
	default:
		return nil, fmt.Errorf("%s: no format %q", r.URL, r.Format)
	}
	if markdown = strings.TrimSpace(markdown); markdown == "" {
		return nil, fmt.Errorf("%s: %w", r.URL, ErrNoWords)
	}
	doc := &Document{
		Key:      Key(r.URL),
		Title:    strings.TrimSpace(r.Title),
		Date:     day.In(calendar.Location).Format(calendar.DateFormat),
		Author:   "Helios School",
		Kind:     KindPortal,
		Channel:  "portal",
		Source:   r.URL,
		Model:    model,
		Markdown: markdown,
	}
	doc.Chunks = Chunks(markdown)
	return doc, nil
}

func unwrapGoogle(u *url.URL) *url.URL {
	if (u.Host != "www.google.com" && u.Host != "google.com") || u.Path != "/url" {
		return u
	}
	target, err := url.Parse(u.Query().Get("q"))
	if err != nil || target.Host == "" {
		return u
	}
	return target
}

var slideNumber = regexp.MustCompile(`^\d+$`)

func fromLines(text string) string {
	text = strings.NewReplacer("\r\n", "\n", "\f", "\n\n", "\v", "\n").Replace(text)
	blocks := []string{}
	for _, block := range strings.Split(text, "\n\n") {
		lines := []string{}
		for _, line := range strings.Split(block, "\n") {
			line = strings.Join(strings.Fields(line), " ")
			if line == "" || slideNumber.MatchString(line) {
				continue
			}
			lines = append(lines, line)
		}
		if len(lines) > 0 {
			blocks = append(blocks, strings.Join(lines, "\n"))
		}
	}
	return strings.Join(blocks, "\n\n")
}
