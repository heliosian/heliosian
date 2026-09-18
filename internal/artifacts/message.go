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

// A Message is one piece of mail as the fetch saved it: who sent it, when,
// what it was called, the channel it went out on, and its text as the mail
// server rendered it. This is the corpus on disk; a Document is what one of
// these becomes.
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

// Broadcast is the channel and kind a message belongs to, and whether it
// belongs in the corpus at all: what the message itself says when the
// channel was not recorded with it, which is how mail arriving later is
// placed (`docs/ask/artifacts.md`).
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

// ErrNoWords is a message with nothing to read: an attachment on its own, or
// a subject and no body. There is nothing to index and nothing wrong.
var ErrNoWords = errors.New("the message has no words")

// ErrNotBroadcast is a message that went to a committee or to one person
// rather than to the community: no part of the corpus.
var ErrNotBroadcast = errors.New("the message is not one the community was sent")

// Build is the document one message becomes: its text as markdown, cut into
// chunks, ready to be embedded. The markdown comes from the HTML the mail
// carried, or from its plain text when that is all it has.
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
		links.Warm(Hrefs(page))
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
		Key:      Key(m.MessageID),
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

// trailers are what a mailing list adds after the message: the subscription
// footer, with its unsubscribe address, and the link to the thread on the
// web. Everything from there on is the machine's words, not anybody's.
var trailers = []string{
	"You received this message because you are subscribed to",
	"To view this discussion on the web visit",
	"To unsubscribe from this group and stop receiving emails",
}

// notices are the blocks a mail client puts under a signature, wherever they
// fall: they say nothing about the school and would be searched like anything
// else.
var notices = []string{
	"This message (including any attachments) may contain confidential information",
	"Please do not forward school emails without permission",
	"Please do not forward emails without permission",
}

// trim is the message without what was added around it. A list footer ends
// the message; a notice is dropped wherever it sits.
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
	// A list footer follows the signature's dashes, which go with it.
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

// fromText is a plain-text message as markdown: its own paragraphs, with the
// hard line breaks a mail client put in joined back up, since a paragraph
// broken every seventy characters reads as a list of fragments otherwise.
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
			// A line that starts a list or a quote is its own line; anything
			// else is the same sentence wrapped.
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
