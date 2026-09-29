package model

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	ResourceHTML = "html"
	ResourceText = "text"
)

type SavedResource struct {
	URL        string `json:"url"`
	Title      string `json:"title"`
	LinkedFrom string `json:"linkedFrom,omitempty"`
	Fetched    string `json:"fetched"`
	Format     string `json:"format"`
	Body       string `json:"body"`
}

func (r SavedResource) Key() string {
	return DocumentKey(r.URL)
}

func readResource(raw []byte) (SavedDocument, error) {
	var r SavedResource
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	if r.URL == "" || strings.TrimSpace(r.Title) == "" || r.Fetched == "" || (r.Format != ResourceHTML && r.Format != ResourceText) {
		return nil, fmt.Errorf("the resource has no address, title, fetch time or known format")
	}
	return r, nil
}

func (r SavedResource) Build(links *LinkResolver, embeddingModel string) (*Document, error) {
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
	case ResourceHTML:
		if markdown, err = Markdown(r.Body, links.Links(base)); err != nil {
			return nil, fmt.Errorf("%s: %w", r.URL, err)
		}
	case ResourceText:
		markdown = fromLines(r.Body)
	default:
		return nil, fmt.Errorf("%s: no format %q", r.URL, r.Format)
	}
	return finishDocument(&Document{
		Key:      DocumentKey(r.URL),
		Title:    strings.TrimSpace(r.Title),
		Author:   "Helios School",
		Kind:     DocumentKindPortal,
		Channel:  "portal",
		Source:   r.URL,
		Model:    embeddingModel,
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
