package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"heliosian/internal/tomarkdown"
)

type DocumentMessage struct {
	MessageID string   `json:"messageId"`
	Subject   string   `json:"subject"`
	Date      string   `json:"date"`
	From      string   `json:"from"`
	Sender    string   `json:"sender,omitempty"`
	To        []string `json:"to,omitempty"`
	CC        []string `json:"cc,omitempty"`
	ListID    string   `json:"listId,omitempty"`
	Channel   string   `json:"channel"`
	Kind      string   `json:"kind"`
	HTML      string   `json:"html,omitempty"`
	Text      string   `json:"text,omitempty"`
}

type SavedDocument interface {
	Key() string
	Build(links *tomarkdown.LinkResolver, embeddingModel string) (*Document, error)
}

func (m DocumentMessage) Key() string {
	if m.Kind == DocumentKindGroup {
		return DocumentKey(m.Channel + "/" + m.MessageID)
	}
	return DocumentKey(m.MessageID)
}

func (p SavedPage) Key() string {
	return DocumentKey(p.URL)
}

func ReadSaved(path string) (SavedDocument, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var probe struct {
		URL    string `json:"url"`
		Format string `json:"format"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	read := readMessage
	if probe.Format != "" {
		read = readResource
	} else if probe.URL != "" {
		read = readPage
	}
	saved, err := read(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return saved, nil
}

func (m DocumentMessage) Broadcast() (channel, kind string, ok bool) {
	if m.Channel != "" && m.Kind != "" {
		return m.Channel, m.Kind, true
	}
	from := m.Sender
	if from == "" {
		from = m.From
	}
	return MailChannel(m.ListID, from)
}

func readMessage(raw []byte) (SavedDocument, error) {
	var m DocumentMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if m.MessageID == "" || m.Date == "" {
		return nil, fmt.Errorf("the message has no id or date")
	}
	return m, nil
}

var ErrNoWords = errors.New("the message has no words")

var ErrNotBroadcast = errors.New("the message is not one the community was sent")

func (m DocumentMessage) Build(links *tomarkdown.LinkResolver, embeddingModel string) (*Document, error) {
	channel, kind, ok := m.Broadcast()
	if !ok {
		return nil, fmt.Errorf("%s: %w", m.MessageID, ErrNotBroadcast)
	}
	day, err := time.Parse(time.RFC3339, m.Date)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.MessageID, err)
	}
	markdown := ""
	if page := strings.TrimSpace(m.HTML); page != "" {
		if markdown, err = tomarkdown.HTML(page, links.Links(&url.URL{})); err != nil {
			return nil, fmt.Errorf("%s: %w", m.MessageID, err)
		}
	} else {
		markdown = tomarkdown.Text(m.Text)
	}
	title := strings.TrimSpace(m.Subject)
	if title == "" {
		title = "(no subject)"
	}
	return finishDocument(&Document{
		Key:      m.Key(),
		Title:    title,
		Time:     day.In(Location).Format("15:04"),
		Author:   strings.TrimSpace(m.From),
		Kind:     kind,
		Channel:  channel,
		Source:   "mail:" + m.MessageID,
		Model:    embeddingModel,
		Markdown: tomarkdown.Trim(markdown),
	}, day)
}

func finishDocument(doc *Document, day time.Time) (*Document, error) {
	if doc.Markdown = strings.TrimSpace(doc.Markdown); doc.Markdown == "" {
		return nil, fmt.Errorf("%s: %w", doc.Source, ErrNoWords)
	}
	doc.Date = day.In(Location).Format(DateFormat)
	doc.Chunks = Chunks(doc.Markdown)
	return doc, nil
}
