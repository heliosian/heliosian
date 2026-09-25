package artifacts

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"heliosian/internal/calendar"
)

type Message struct {
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

type Saved interface {
	Key() string
	Build(links *Resolver, model string) (*Document, error)
}

func (m Message) Key() string {
	if m.Kind == KindGroup {
		return Key(m.Channel + "/" + m.MessageID)
	}
	return Key(m.MessageID)
}

func (m Message) Build(links *Resolver, model string) (*Document, error) {
	return Build(m, links, model)
}

func (p Page) Key() string {
	return Key(p.URL)
}

func (p Page) Build(_ *Resolver, model string) (*Document, error) {
	return BuildPage(p, model)
}

func ReadSaved(path string) (Saved, error) {
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
	if probe.Format != "" {
		return ReadResource(path)
	}
	if probe.URL != "" {
		return ReadPage(path)
	}
	return ReadMessage(path)
}

func (m Message) Broadcast() (channel, kind string, ok bool) {
	if m.Channel != "" && m.Kind != "" {
		return m.Channel, m.Kind, true
	}
	from := m.Sender
	if from == "" {
		from = m.From
	}
	return Channel(m.ListID, from, append(append([]string{}, m.To...), m.CC...))
}

func ReadMessage(path string) (Message, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Message{}, err
	}
	var m Message
	if err := json.Unmarshal(raw, &m); err != nil {
		return Message{}, fmt.Errorf("%s: %w", path, err)
	}
	if m.MessageID == "" || m.Date == "" {
		return Message{}, fmt.Errorf("%s: the message has no id or date", path)
	}
	return m, nil
}

var ErrNoWords = errors.New("the message has no words")

var ErrNotBroadcast = errors.New("the message is not one the community was sent")

func Build(m Message, links *Resolver, model string) (*Document, error) {
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
		if markdown, err = Markdown(page, links.Resolve); err != nil {
			return nil, fmt.Errorf("%s: %w", m.MessageID, err)
		}
	} else {
		markdown = fromText(m.Text)
	}
	if markdown = trim(markdown); markdown == "" {
		return nil, fmt.Errorf("%s: %w", m.MessageID, ErrNoWords)
	}
	title := strings.TrimSpace(m.Subject)
	if title == "" {
		title = "(no subject)"
	}
	doc := &Document{
		Key:      m.Key(),
		Title:    title,
		Date:     day.In(calendar.Location).Format(calendar.DateFormat),
		Author:   strings.TrimSpace(m.From),
		Kind:     kind,
		Channel:  channel,
		Source:   "mail:" + m.MessageID,
		Model:    model,
		Markdown: markdown,
	}
	doc.Chunks = Chunks(markdown)
	return doc, nil
}

var trailers = []string{
	"You received this message because you are subscribed to",
	"To view this discussion on the web visit",
	"To unsubscribe from this group and stop receiving emails",
}

var notices = []string{
	"This message (including any attachments) may contain confidential information",
	"Please do not forward school emails without permission",
	"Please do not forward emails without permission",
}

func trim(markdown string) string {
	blocks := strings.Split(markdown, "\n\n")
	for i, block := range blocks {
		if containsAny(block, trailers) {
			blocks = blocks[:i]
			break
		}
	}
	kept := []string{}
	for _, block := range blocks {
		if containsAny(block, notices) {
			continue
		}
		kept = append(kept, block)
	}
	for len(kept) > 0 {
		last := strings.TrimSpace(kept[len(kept)-1])
		if last != "" && last != "--" && last != "---" && last != "-" {
			break
		}
		kept = kept[:len(kept)-1]
	}
	return strings.TrimSpace(strings.Join(kept, "\n\n"))
}

func containsAny(block string, phrases []string) bool {
	for _, phrase := range phrases {
		if strings.Contains(block, phrase) {
			return true
		}
	}
	return false
}

func fromText(text string) string {
	paragraphs := []string{}
	for _, block := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
		lines := []string{}
		for _, line := range strings.Split(block, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				lines = append(lines, line)
			}
		}
		if len(lines) == 0 {
			continue
		}
		joined := lines[0]
		for _, line := range lines[1:] {
			if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, ">") || strings.HasPrefix(line, "#") {
				joined += "\n" + line
				continue
			}
			joined += " " + line
		}
		paragraphs = append(paragraphs, joined)
	}
	return strings.Join(paragraphs, "\n\n")
}
