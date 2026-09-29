package calendarimport

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"heliosian/internal/model"
)

const pageURL = model.SchoolCalendarPage

var retryWaits = []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute}

func fetch(ctx context.Context, address string) ([]byte, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	for attempt := 0; ; attempt++ {
		resp, err := client.Get(address)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusOK {
			return body, nil
		}
		if resp.StatusCode != http.StatusTooManyRequests || attempt == len(retryWaits) {
			return nil, fmt.Errorf("get %s: %s", address, resp.Status)
		}
		slog.WarnContext(ctx, "calendar import: fetch throttled", "url", address, "status", resp.Status, "retry-after", resp.Header.Get("Retry-After"), "wait", retryWaits[attempt])
		time.Sleep(retryWaits[attempt])
	}
}

var pdfLink = regexp.MustCompile(`href="([^"]+\.pdf)"`)

func findPDF(page []byte) (string, error) {
	match := pdfLink.FindSubmatch(page)
	if match == nil {
		return "", fmt.Errorf("no pdf link on %s", pageURL)
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		return "", err
	}
	link, err := url.Parse(html.UnescapeString(string(match[1])))
	if err != nil {
		return "", err
	}
	return base.ResolveReference(link).String(), nil
}

func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

const lineBreak = "\x00"

var blockTags = map[string]bool{
	"p": true, "div": true, "li": true, "blockquote": true, "tr": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

func flatten(text string) (string, error) {
	if strings.Contains(text, "<") {
		parent := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
		nodes, err := html.ParseFragment(strings.NewReader(text), parent)
		if err != nil {
			return "", err
		}
		out := &strings.Builder{}
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.TextNode {
				out.WriteString(n.Data)
			}
			if n.Type == html.ElementNode && n.Data == "br" {
				out.WriteString(lineBreak)
			}
			before := out.Len()
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
			if n.Type == html.ElementNode && n.Data == "a" {
				for _, attr := range n.Attr {
					if attr.Key == "href" && attr.Val != "" && !strings.Contains(out.String()[before:], attr.Val) {
						out.WriteString(" " + attr.Val)
					}
				}
			}
			if n.Type == html.ElementNode && blockTags[n.Data] {
				out.WriteString(lineBreak)
			}
		}
		for _, n := range nodes {
			walk(n)
		}
		text = strings.ReplaceAll(out.String(), lineBreak, "\n")
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = collapse(line)
	}
	flat := strings.Join(lines, "\n")
	for strings.Contains(flat, "\n\n\n") {
		flat = strings.ReplaceAll(flat, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(flat), nil
}
