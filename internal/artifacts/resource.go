package artifacts

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
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

func readResource(raw []byte) (Saved, error) {
	var r Resource
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	if r.URL == "" || strings.TrimSpace(r.Title) == "" || r.Fetched == "" || (r.Format != FormatHTML && r.Format != FormatText) {
		return nil, fmt.Errorf("the resource has no address, title, fetch time or known format")
	}
	return r, nil
}

func (r Resource) Build(links *Resolver, model string) (*Document, error) {
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
		if markdown, err = Markdown(r.Body, links.Links(base)); err != nil {
			return nil, fmt.Errorf("%s: %w", r.URL, err)
		}
	case FormatText:
		markdown = fromLines(r.Body)
	default:
		return nil, fmt.Errorf("%s: no format %q", r.URL, r.Format)
	}
	return finish(&Document{
		Key:      Key(r.URL),
		Title:    strings.TrimSpace(r.Title),
		Author:   "Helios School",
		Kind:     KindPortal,
		Channel:  "portal",
		Source:   r.URL,
		Model:    model,
		Markdown: markdown,
	}, day)
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
