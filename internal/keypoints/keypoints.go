// Package keypoints writes the key points of each school email - the
// newsletter, the all-family and classroom lists - once it is in, for
// Heliosian's Inbox widget: a few short lines, what to do and by
// when first, read by Claude from the email's words and kept in the
// artifacts Documents tab's Key Points column.
package keypoints

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
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

// Revision is the day the audience question last changed - it learned to
// name grades as well as classrooms. An email in the window judged before
// it is read again, and its new answer written over the old in place, so
// it never goes unjudged - and out of the widget - on the way. Moving it
// on reads the window again after a change to the question.
const Revision = "2026-09-26"

// perRun caps how many emails one pass reads, so a backlog - the first run,
// or a day of many emails - spreads over several passes.
const perRun = 8

// every is how often a pass runs.
const every = 10 * time.Minute

const system = `You read one email the Helios School community received - the school's newsletter, or a message to all the families or to one class - and do two things.

First, list its key points for a parent skimming their week. Write three to five points, fewer for a short email. Lead with what a family must do or know by a date: deadlines, events with their day and time, things to bring, forms to send. Then the rest that matters most. Each point is one plain sentence under twenty words, naming the day ("Tue, Sep 29") where the email gives one. Say only what the email says; no greetings, no sign-offs, no advice of your own. If the email has nothing worth a point - an automatic notice, an empty message - return no points.

Second, say whom it was written to. The school's classrooms and grades are listed with the email, and so are the classrooms its sender teaches, when they teach any. A classroom holds two grades (Condors are 5th and 6th graders), so say each as the email does. If the email is written to the families or students of particular classrooms - its greeting ("Hi Condor Families"), its sign-off, or what it is about (one class's play, trip or homework) says so - list those classrooms. If it is written to particular grades - "Dear Parents of 2nd, 4th, 6th, and 8th graders", "for our 8th graders" - list those grades, and not the classrooms that hold them. List both only when it names both ("6th graders in Condors"). Use the names given. If it is for the whole school, or a program you cannot tie to classrooms or grades, or you cannot tell, list neither. A teacher writing about their own class is writing to that class, even without a greeting.`

var schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"points":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "the key points, most pressing first"},
		"classrooms": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "the classrooms it was written to, by the names given; none for the whole school, for grades alone, or when unclear"},
		"grades":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "the grades it was written to, by the names given; none unless the email names grades"},
	},
	"required":             []string{"points", "classrooms", "grades"},
	"additionalProperties": false,
}

// Email is one email as Claude is given it: its subject, when and by whom
// it was sent, its words, the school's classrooms and grades, and the
// classrooms its sender teaches.
type Email struct {
	Title, Date, Author, Markdown string
	Classrooms, Grades, Teaches   []string
}

// Reading is what an email comes to: its key points, and the classrooms
// and grades it was written to - neither for the whole school.
type Reading struct {
	Points     []string
	Classrooms []string
	Grades     []string
}

// Summarizer reads an email.
type Summarizer interface {
	Read(ctx context.Context, e Email) (Reading, error)
}

// School is what the pass needs of the directory: the school's classrooms
// and grades, and the classrooms an email's sender teaches, by the name the
// email gives.
type School interface {
	Classrooms() []string
	Grades() []string
	Teaches(author string) []string
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

func (c *Claude) Read(ctx context.Context, e Email) (Reading, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	markdown := e.Markdown
	if len(markdown) > maxEmail {
		markdown = markdown[:maxEmail]
	}
	teaches := "none"
	if len(e.Teaches) > 0 {
		teaches = strings.Join(e.Teaches, ", ")
	}
	prompt := fmt.Sprintf("The school's classrooms: %s\nThe school's grades: %s\nThe sender teaches: %s\n\nSubject: %s\nFrom: %s\nSent: %s\n\n%s",
		strings.Join(e.Classrooms, ", "), strings.Join(e.Grades, ", "), teaches, e.Title, e.Author, e.Date, markdown)
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
			return Reading{}, err
		}
	}
	if err := stream.Err(); err != nil {
		return Reading{}, err
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return Reading{}, fmt.Errorf("claude declined: %s", resp.StopDetails.Explanation)
	}
	if resp.StopReason != anthropic.StopReasonEndTurn {
		return Reading{}, fmt.Errorf("claude stopped early: %s", resp.StopReason)
	}
	text := &strings.Builder{}
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	var out struct {
		Points     []string `json:"points"`
		Classrooms []string `json:"classrooms"`
		Grades     []string `json:"grades"`
	}
	if err := json.Unmarshal([]byte(text.String()), &out); err != nil {
		return Reading{}, fmt.Errorf("read claude's answer: %w", err)
	}
	reading := Reading{Points: clean(out.Points), Classrooms: known(out.Classrooms, e.Classrooms), Grades: known(out.Grades, e.Grades)}
	slog.InfoContext(ctx, "keypoints: read an email", "title", e.Title, "points", len(reading.Points), "classrooms", strings.Join(reading.Classrooms, ", "), "grades", strings.Join(reading.Grades, ", "), "input_tokens", resp.Usage.InputTokens+resp.Usage.CacheReadInputTokens+resp.Usage.CacheCreationInputTokens, "output_tokens", resp.Usage.OutputTokens)
	return reading, nil
}

// known keeps the classrooms or grades Claude named that the school has,
// by the school's own spelling, each once.
func known(named, school []string) []string {
	out := []string{}
	for _, n := range named {
		for _, c := range school {
			if strings.EqualFold(strings.TrimSpace(n), c) && !slices.Contains(out, c) {
				out = append(out, c)
			}
		}
	}
	return out
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
// its first sentence, as its points, and the classrooms its sender teaches
// as whom it went to - no key, no cost.
type Fake struct{}

func (Fake) Read(_ context.Context, e Email) (Reading, error) {
	return Reading{Points: fakePoints(e.Markdown), Classrooms: e.Teaches}, nil
}

func fakePoints(markdown string) []string {
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
	return out
}

// Missing is the school emails within the window not yet read - no
// audience written - newest first, then those judged before Revision,
// newest first.
func Missing(m *artifacts.Model, now time.Time) []*artifacts.Document {
	since := now.Add(-Window).Format("2006-01-02")
	unread, stale := []*artifacts.Document{}, []*artifacts.Document{}
	for _, d := range m.Documents {
		if d.Date < since || !artifacts.School(d) {
			continue
		}
		if m.Audience[d.Key] == "" {
			unread = append(unread, d)
		} else if m.Judged[d.Key] < Revision {
			stale = append(stale, d)
		}
	}
	return append(unread, stale...)
}

// Pass reads the next few emails not yet read, writing each one's points
// and whom it went to - the classrooms and grades named, or Everyone when
// Claude names neither.
func Pass(ctx context.Context, cache *artifacts.Cache, s Summarizer, school School, now time.Time) int {
	written := 0
	for _, d := range Missing(cache.Model(), now) {
		if written == perRun {
			break
		}
		reading, err := s.Read(ctx, Email{Title: d.Title, Date: d.Date, Author: d.Author, Markdown: d.Markdown, Classrooms: school.Classrooms(), Grades: school.Grades(), Teaches: school.Teaches(d.Author)})
		if err != nil {
			slog.ErrorContext(ctx, "keypoints: read an email", "error", err, "key", d.Key, "title", d.Title)
			return written
		}
		audience := artifacts.Everyone
		if named := append(slices.Clone(reading.Classrooms), reading.Grades...); len(named) > 0 {
			audience = strings.Join(named, ", ")
		}
		if err := cache.SetPoints(ctx, "keypoints", d.Key, reading.Points, audience, now.Format("2006-01-02")); err != nil {
			slog.ErrorContext(ctx, "[ERROR] keypoints: write the points", "error", err, "key", d.Key)
			return written
		}
		written++
	}
	return written
}

// Run passes over the mail every few minutes, from a minute after the start
// so the caches have settled.
func Run(cache *artifacts.Cache, s Summarizer, school School) {
	time.Sleep(time.Minute)
	for {
		if n := Pass(context.Background(), cache, s, school, time.Now()); n > 0 {
			slog.Info("keypoints: wrote points", "emails", n)
		}
		time.Sleep(every)
	}
}
