package db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"golang.org/x/net/html"

	"heliosian/internal/cells"
	"heliosian/internal/claude"
	"heliosian/internal/store"
	"heliosian/internal/tomarkdown"
)

const (
	linkMost     = 200
	linkContext  = 300
	linkBackoff  = 30 * time.Second
	linkMaxReply = 16000
	fetchLink    = "fetch"
)

var skipReasons = []string{"form", "signup", "social", "homepage", "per_recipient", "tracking", "media", "other"}

var unwantedLinkWords = []string{"unsubscribe", "update your preferences", "manage preferences", "manage your preferences", "view this email in your browser", "view it in your browser", "view in browser", "forward to a friend"}

var contextTags = []string{"p", "li", "td", "th", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "div"}

const linkSystem = `You read the links in one email the Helios School community received - the school's newsletter, a notice from the school's mailer, a message to all the families or to one class - and say which are worth fetching and keeping beside the email, so that the community can later ask about what they say.

Fetch a link when what it leads to holds information the email points at: a document (a Google Doc, Slides, Sheet or Drive file, or a PDF) such as a schedule, a supply list, a handbook, a menu or a flyer; or a web page that says something itself, such as an event's details, an announcement or a policy. When unsure whether a document or page says something the email points at, fetch it. A mailer's click-tracking address still leads somewhere: judge it by its words and the sentence it sits in.

Otherwise say why it is not worth fetching: a form to fill in (form); a sign-up, RSVP, volunteer or ticket page (signup); a social media profile or post (social); a site's front page that says nothing about this email (homepage); a link made for the one person it was sent to, such as an account, a preference, an RSVP or a login (per_recipient); a mailer's tracking or unsubscribe link (tracking); a video, a map, an app store page or a photo gallery (media); or anything else (other).

Answer every link, by its number and its address as given.`

var linkSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"links": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"n":        map[string]any{"type": "integer", "description": "the link's number, as given"},
					"url":      map[string]any{"type": "string", "description": "the link's address, exactly as given"},
					"decision": map[string]any{"type": "string", "enum": append([]string{fetchLink}, skipReasons...), "description": "fetch, or why it is not worth fetching"},
				},
				"required":             []string{"n", "url", "decision"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"links"},
	"additionalProperties": false,
}

type LinkChoice struct {
	URL, Text, Context string
	Skip               string
}

type LinkEmail struct {
	Subject, Kind, Sent string
}

func emailLinks(raw []byte) ([]LinkChoice, error) {
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	resolve := (&tomarkdown.LinkResolver{}).Links(&url.URL{})
	out := []LinkChoice{}
	seen := map[string]bool{}
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode || n.Data != "a" {
			continue
		}
		href := ""
		for _, attr := range n.Attr {
			if attr.Key == "href" {
				href = strings.TrimSpace(attr.Val)
			}
		}
		address := resolve(href)
		text := nodeText(n)
		lower := strings.ToLower(text + " " + address)
		if seen[address] || cells.URL(address, false) != nil || slices.ContainsFunc(unwantedLinkWords, func(w string) bool { return strings.Contains(lower, w) }) {
			continue
		}
		seen[address] = true
		out = append(out, LinkChoice{URL: address, Text: text, Context: linkSurroundings(n)})
	}
	return out, nil
}

func nodeText(n *html.Node) string {
	b := &strings.Builder{}
	for d := range n.Descendants() {
		if d.Type == html.TextNode {
			b.WriteString(d.Data)
			b.WriteString(" ")
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func linkSurroundings(n *html.Node) string {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type != html.ElementNode || !slices.Contains(contextTags, p.Data) {
			continue
		}
		text := []rune(nodeText(p))
		if len(text) > linkContext {
			return string(text[:linkContext]) + "…"
		}
		return string(text)
	}
	return ""
}

func ChooseLinks(ctx context.Context, client anthropic.Client, email LinkEmail, raw []byte) ([]LinkChoice, error) {
	links, err := emailLinks(raw)
	if err != nil {
		return nil, err
	}
	if len(links) > linkMost {
		slog.Warn("links: too many, choosing among the first", "links", len(links), "first", linkMost)
		links = links[:linkMost]
	}
	if len(links) == 0 {
		return links, nil
	}
	b := &strings.Builder{}
	fmt.Fprintf(b, "Subject: %s\nKind: %s\nSent: %s\n\nLinks:\n", email.Subject, email.Kind, email.Sent)
	for i, l := range links {
		fmt.Fprintf(b, "\n%d. %s\n   words: %s\n   sentence: %s\n", i+1, l.URL, l.Text, l.Context)
	}
	var out struct {
		Links []struct {
			N        int    `json:"n"`
			URL      string `json:"url"`
			Decision string `json:"decision"`
		} `json:"links"`
	}
	if _, err := claude.JSON(ctx, client, anthropic.MessageNewParams{
		Model:        claude.LinkModel,
		MaxTokens:    linkMaxReply,
		System:       []anthropic.TextBlockParam{{Text: linkSystem}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(b.String()))},
		OutputConfig: anthropic.OutputConfigParam{Effort: claude.LinkEffort, Format: anthropic.JSONOutputFormatParam{Schema: linkSchema}},
	}, &out); err != nil {
		if errors.Is(err, claude.ErrFinal) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", errAskAgain, err)
	}
	answered := make([]bool, len(links))
	for _, a := range out.Links {
		if a.N < 1 || a.N > len(links) || answered[a.N-1] || a.URL != links[a.N-1].URL || (a.Decision != fetchLink && !slices.Contains(skipReasons, a.Decision)) {
			answer, _ := json.Marshal(a)
			return nil, fmt.Errorf("%w: an answer that matches no link: %s", errAskAgain, answer)
		}
		answered[a.N-1] = true
		if a.Decision != fetchLink {
			links[a.N-1].Skip = a.Decision
		}
	}
	if i := slices.Index(answered, false); i >= 0 {
		return nil, fmt.Errorf("%w: link %d was not answered", errAskAgain, i+1)
	}
	return links, nil
}

func (m *Model) mailRootOf(doc store.Row) (store.Row, bool) {
	docs := m.Table("DOCUMENT")
	for doc["parent"] != "" {
		parent, ok := docs.Get(doc["parent"])
		if !ok {
			return nil, false
		}
		doc = parent
	}
	return doc, doc["kind"] == "mail"
}
