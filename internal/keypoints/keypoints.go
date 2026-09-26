// Package keypoints writes the key points of each school email - the
// newsletter, the all-family and classroom lists - once it is in, for
// Heliosian's From the School widget: a few short lines, what to do and by
// when first, read by Claude from the email's words and kept in the
// artifacts Documents tab's Key Points column.
package keypoints

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"heliosian/internal/artifacts"
)

// model reads the emails: quick and inexpensive, and a summary wants no
// more.
const model = "claude-sonnet-5"

// maxEmail is how much of an email Claude is given; a newsletter runs long,
// and its key points are near its top.
const maxEmail = 24 << 10

// Window is how far back the widget reads, and so how far back points are
// written: a week's mail, and a week more for the edges of it.
const Window = 14 * 24 * time.Hour

// perRun caps how many emails one pass reads, so a backlog - the first run,
// or a day of many emails - spreads over several passes.
const perRun = 8

// every is how often a pass runs.
const every = 10 * time.Minute

const system = `You read one email the Helios School community received - the school's newsletter, or a message to all the families or to one class - and list its key points for a parent skimming their week.

Write three to five points, fewer for a short email. Lead with what a family must do or know by a date: deadlines, events with their day and time, things to bring, forms to send. Then the rest that matters most. Each point is one plain sentence under twenty words, naming the day ("Tue, Sep 29") where the email gives one. Say only what the email says; no greetings, no sign-offs, no advice of your own. If the email has nothing worth a point - an automatic notice, an empty message - return no points.`

var schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"points": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "the key points, most pressing first"},
	},
	"required":             []string{"points"},
	"additionalProperties": false,
}

// Summarizer reads an email's key points.
type Summarizer interface {
	Points(ctx context.Context, title, date, markdown string) ([]string, error)
}

// Claude is the Summarizer that asks Claude.
type Claude struct {
	client anthropic.Client
}

// New is Claude with the key, or nil with none.
func New(key string) *Claude {
	if key == "" {
		return nil
	}
	return &Claude{client: anthropic.NewClient(option.WithAPIKey(key))}
}

func (c *Claude) Points(ctx context.Context, title, date, markdown string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if len(markdown) > maxEmail {
		markdown = markdown[:maxEmail]
	}
	prompt := fmt.Sprintf("Subject: %s\nSent: %s\n\n%s", title, date, markdown)
	stream := c.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:        model,
		MaxTokens:    2000,
		System:       []anthropic.TextBlockParam{{Text: system, CacheControl: anthropic.NewCacheControlEphemeralParam()}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortLow, Format: anthropic.JSONOutputFormatParam{Schema: schema}},
	})
	resp := anthropic.Message{}
	for stream.Next() {
		if err := resp.Accumulate(stream.Current()); err != nil {
			return nil, err
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return nil, fmt.Errorf("claude declined: %s", resp.StopDetails.Explanation)
	}
	if resp.StopReason != anthropic.StopReasonEndTurn {
		return nil, fmt.Errorf("claude stopped early: %s", resp.StopReason)
	}
	text := &strings.Builder{}
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	var out struct {
		Points []string `json:"points"`
	}
	if err := json.Unmarshal([]byte(text.String()), &out); err != nil {
		return nil, fmt.Errorf("read claude's answer: %w", err)
	}
	slog.InfoContext(ctx, "keypoints: read an email", "title", title, "points", len(out.Points), "input_tokens", resp.Usage.InputTokens+resp.Usage.CacheReadInputTokens+resp.Usage.CacheCreationInputTokens, "output_tokens", resp.Usage.OutputTokens)
	return clean(out.Points), nil
}

// clean keeps the points that say something, trimmed, one line each.
func clean(points []string) []string {
	out := []string{}
	for _, p := range points {
		if p = strings.Join(strings.Fields(p), " "); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Fake is the sample server's Summarizer: an email's section headings, or
// its first sentence, as its points - no key, no cost.
type Fake struct{}

func (Fake) Points(_ context.Context, title, _, markdown string) ([]string, error) {
	out := []string{}
	for _, line := range strings.Split(markdown, "\n") {
		if heading, ok := strings.CutPrefix(strings.TrimSpace(line), "#"); ok && len(out) < 4 {
			if h := strings.TrimSpace(strings.TrimLeft(heading, "#")); h != "" {
				out = append(out, h)
			}
		}
	}
	if len(out) == 0 {
		first, _, _ := strings.Cut(strings.Join(strings.Fields(markdown), " "), ". ")
		if first != "" {
			out = append(out, first)
		}
	}
	return out, nil
}

// Missing is the school emails within the window that have no points yet,
// newest first.
func Missing(m *artifacts.Model, now time.Time) []*artifacts.Document {
	since := now.Add(-Window).Format("2006-01-02")
	out := []*artifacts.Document{}
	for _, d := range m.Documents {
		if d.Date >= since && artifacts.School(d) && len(m.Points[d.Key]) == 0 && !tried[d.Key] {
			out = append(out, d)
		}
	}
	return out
}

// tried remembers the emails that came back with no points, so a notice with
// nothing in it is asked about once a run of the server rather than every
// pass.
var tried = map[string]bool{}

// Pass writes the points of the next few emails missing them.
func Pass(ctx context.Context, cache *artifacts.Cache, s Summarizer, now time.Time) int {
	written := 0
	for _, d := range Missing(cache.Model(), now) {
		if written == perRun {
			break
		}
		points, err := s.Points(ctx, d.Title, d.Date, d.Markdown)
		if err != nil {
			slog.ErrorContext(ctx, "keypoints: read an email", "error", err, "key", d.Key, "title", d.Title)
			return written
		}
		if len(points) == 0 {
			tried[d.Key] = true
			continue
		}
		if err := cache.SetPoints(ctx, "keypoints", d.Key, points); err != nil {
			slog.ErrorContext(ctx, "[ERROR] keypoints: write the points", "error", err, "key", d.Key)
			return written
		}
		written++
	}
	return written
}

// Run passes over the mail every few minutes, from a minute after the start
// so the caches have settled.
func Run(cache *artifacts.Cache, s Summarizer) {
	time.Sleep(time.Minute)
	for {
		if n := Pass(context.Background(), cache, s, time.Now()); n > 0 {
			slog.Info("keypoints: wrote points", "emails", n)
		}
		time.Sleep(every)
	}
}
