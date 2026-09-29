package keypoints

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"heliosian/internal/access"
	"heliosian/internal/claude"
	"heliosian/internal/model"
)

const claudeModel = "claude-sonnet-5"

const maxEmail = 24 << 10

const Revision = "2026-09-26"

const perRun = 8

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

type Email struct {
	Title, Date, Author, Markdown string
	Classrooms, Grades, Teaches   []string
}

type Reading struct {
	Points     []string
	Classrooms []string
	Grades     []string
}

type School interface {
	Classrooms() []string
	Grades() []string
	Teaches(author string) []string
}

type Claude struct {
	client anthropic.Client
}

func New(key string) *Claude {
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
	var out struct {
		Points     []string `json:"points"`
		Classrooms []string `json:"classrooms"`
		Grades     []string `json:"grades"`
	}
	if _, err := claude.JSON(ctx, c.client, anthropic.MessageNewParams{
		Model:        claudeModel,
		MaxTokens:    2000,
		System:       []anthropic.TextBlockParam{{Text: system, CacheControl: anthropic.NewCacheControlEphemeralParam()}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortLow, Format: anthropic.JSONOutputFormatParam{Schema: schema}},
	}, &out); err != nil {
		return Reading{}, err
	}
	reading := Reading{Points: clean(out.Points), Classrooms: known(out.Classrooms, e.Classrooms), Grades: known(out.Grades, e.Grades)}
	slog.InfoContext(ctx, "keypoints: read an email", "title", e.Title, "points", len(reading.Points), "classrooms", strings.Join(reading.Classrooms, ", "), "grades", strings.Join(reading.Grades, ", "))
	return reading, nil
}

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

func clean(points []string) []string {
	out := []string{}
	for _, p := range points {
		if p = strings.Join(strings.Fields(p), " "); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func Missing(m *model.Documents, now time.Time) []*model.Document {
	since := now.Add(-model.SchoolMailWindow).Format("2006-01-02")
	unread, stale := []*model.Document{}, []*model.Document{}
	for _, d := range m.Documents {
		if d.Date < since || !d.School() {
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

func Pass(ctx context.Context, cache *model.DocumentsCache, s *Claude, school School, now time.Time) int {
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
		audience := model.DocumentForEveryone
		if named := append(slices.Clone(reading.Classrooms), reading.Grades...); len(named) > 0 {
			audience = strings.Join(named, ", ")
		}
		if err := cache.SetPoints(ctx, access.System("keypoints"), d.Key, reading.Points, audience, now.Format("2006-01-02")); err != nil {
			slog.ErrorContext(ctx, "keypoints: write the points", "error", err, "key", d.Key)
			return written
		}
		written++
	}
	return written
}

func Run(cache *model.DocumentsCache, s *Claude, school School) {
	time.Sleep(time.Minute)
	for {
		if n := Pass(context.Background(), cache, s, school, time.Now()); n > 0 {
			slog.Info("keypoints: wrote points", "emails", n)
		}
		time.Sleep(every)
	}
}
