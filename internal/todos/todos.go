package todos

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"heliosian/internal/access"
	"heliosian/internal/claude"
	"heliosian/internal/model"
	"heliosian/internal/store"
)

const claudeModel = "claude-sonnet-5"

const maxEmail = 24 << 10

const Revision = "2026-09-30"

var actor = access.System("to-dos")

const system = `You read one email the Helios School community received - the school's newsletter, a message to all the families or to one class, or a post to a parent email list - and list what it asks families to do.

A to-do is something a parent or student has to act on: a form to fill out or return, something to sign, pay, buy, bring, send or sign up for, a reply or an RSVP, a deadline to meet. Leave out what only informs: an event with nothing to do before it, a schedule change, news, thanks. An event is a to-do only when the email asks for something before it - a sign-up, a ticket, a permission slip, something to bring. A call for volunteers counts when the email asks its readers directly. If the email asks nothing of its readers, return no to-dos.

First say in asks, in a sentence or two, what the email asks its readers to do, or that it asks nothing. Then list a to-do for each thing it asks.

For each to-do:
- title: the action, starting with a verb, at most eight words ("Return the tide pool permission slip").
- summary: five to ten words on what it is for ("Signed slip for the Jays' tide pool trip").
- details: one or two plain sentences with what a parent needs to do it - what, by when, where it goes, what it costs. Say only what the email says.
- link: the web address the email gives for doing it - the form, the sign-up, the payment page - copied exactly as it appears; empty when it gives none. Never an email address. When the email gives a different link for each class or grade, leave link empty and say in details to use the link for your child's class.
- due: the day it has to be done by, as YYYY-MM-DD, worked out from the email's words and the day it was sent; for something to bring or do at an event, the event's day; empty when the email names no day.

The list already holds the to-dos given with the email, from earlier emails. Leave out any to-do already there, however this email words it.`

type field struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// The model writes an object's fields in the order the schema lists them, and a map would list them
// alphabetically, details first; structs keep the title first.
type toDoFields struct {
	Title   field `json:"title"`
	Summary field `json:"summary"`
	Details field `json:"details"`
	Link    field `json:"link"`
	Due     field `json:"due"`
}

var schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"asks": field{"string", "in a sentence or two, what the email asks its readers to do, or that it asks nothing"},
		"todos": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": toDoFields{
					Title:   field{"string", "the action, starting with a verb, at most eight words"},
					Summary: field{"string", "five to ten words on what it is for"},
					Details: field{"string", "one or two sentences"},
					Link:    field{"string", "the web address for doing it, copied from the email, or empty"},
					Due:     field{"string", "YYYY-MM-DD, or empty"},
				},
				"required":             []string{"title", "summary", "details", "link", "due"},
				"additionalProperties": false,
			},
			"description": "what the email asks families to do, soonest first",
		},
	},
	"required":             []string{"asks", "todos"},
	"additionalProperties": false,
}

type Listed struct {
	Title, Due, To string
}

type Email struct {
	Title, Date, Author, To, Markdown string
	Listed                            []Listed
}

type Claude struct {
	client anthropic.Client
}

func New(key string) *Claude {
	return &Claude{client: anthropic.NewClient(option.WithAPIKey(key))}
}

func (c *Claude) Read(ctx context.Context, e Email) ([]model.ToDo, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	markdown := e.Markdown
	if len(markdown) > maxEmail {
		markdown = markdown[:maxEmail]
	}
	sent := e.Date
	if day, err := time.ParseInLocation(model.DateFormat, e.Date, model.Location); err == nil {
		sent = day.Format("Monday, January 2, 2006")
	}
	listed := []string{}
	for _, l := range e.Listed {
		line := "- " + l.Title + " (for " + l.To
		if l.Due != "" {
			line += ", due " + l.Due
		}
		listed = append(listed, line+")")
	}
	if len(listed) == 0 {
		listed = append(listed, "(nothing yet)")
	}
	prompt := fmt.Sprintf("Already on the list:\n%s\n\nSubject: %s\nFrom: %s\nSent to: %s\nSent: %s\n\n%s",
		strings.Join(listed, "\n"), e.Title, e.Author, e.To, sent, markdown)
	var out struct {
		Asks  string   `json:"asks"`
		ToDos []answer `json:"todos"`
	}
	raw, err := claude.JSON(ctx, c.client, anthropic.MessageNewParams{
		Model:        claudeModel,
		MaxTokens:    3000,
		System:       []anthropic.TextBlockParam{{Text: system, CacheControl: anthropic.NewCacheControlEphemeralParam()}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortMedium, Format: anthropic.JSONOutputFormatParam{Schema: schema}},
	}, &out)
	if err != nil {
		return nil, err
	}
	toDos := clean(out.ToDos, e.Markdown)
	slog.InfoContext(ctx, "todos: read an email", "title", e.Title, "asks", out.Asks, "asked", len(out.ToDos), "kept", len(toDos))
	if len(toDos) < len(out.ToDos) {
		slog.InfoContext(ctx, "todos: claude's whole answer, some of it left out", "title", e.Title, "answer", raw)
	}
	return toDos, nil
}

type answer struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Details string `json:"details"`
	Link    string `json:"link"`
	Due     string `json:"due"`
}

var spaces = regexp.MustCompile(`\s+`)

func tidy(s string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(s, " "))
}

func clean(answers []answer, markdown string) []model.ToDo {
	out := []model.ToDo{}
	titles := []string{}
	for _, a := range answers {
		t := model.ToDo{Title: strings.TrimRight(tidy(a.Title), "."), Summary: tidy(a.Summary), Details: tidy(a.Details), Link: tidy(a.Link), Due: tidy(a.Due)}
		if t.Link != "" && !strings.Contains(markdown, t.Link) {
			slog.Info("todos: a link the email does not hold, dropped", "title", t.Title, "link", t.Link)
			t.Link = ""
		}
		if t.Due != "" {
			if _, err := time.ParseInLocation(model.DateFormat, t.Due, model.Location); err != nil {
				slog.Info("todos: a due day that is not a date, dropped", "title", t.Title, "due", t.Due)
				t.Due = ""
			}
		}
		if t.Summary == "" || t.Details == "" {
			slog.Info("todos: a to-do with no summary or details left out", "title", t.Title)
			continue
		}
		if err := model.CheckToDo(t); err != nil {
			slog.Info("todos: a to-do left out", "error", err)
			continue
		}
		if slices.Contains(titles, strings.ToLower(t.Title)) {
			continue
		}
		titles = append(titles, strings.ToLower(t.Title))
		out = append(out, t)
	}
	return out
}

func Missing(m *model.Documents, now time.Time) []*model.Document {
	since := now.In(model.Location).Add(-model.ToDoWindow).Format(model.DateFormat)
	out := []*model.Document{}
	for _, d := range m.Documents {
		if d.Date < since || !d.ToDoSource() || m.ToDosRead[d.Key] >= Revision {
			continue
		}
		out = append(out, d)
	}
	slices.Reverse(out)
	return out
}

func listed(m *model.Model, except string, now time.Time) []Listed {
	today := now.In(model.Location).Format(model.DateFormat)
	out := []Listed{}
	for _, t := range m.Documents.ToDos {
		d := m.Documents.Document(t.Document)
		if t.Document == except || d == nil || !m.Documents.Current(t, today) {
			continue
		}
		out = append(out, Listed{Title: t.Title, Due: t.Due, To: m.DocumentSentTo(d)})
	}
	return out
}

func Pass(ctx context.Context, s *model.Store, c *Claude, now time.Time) int {
	written := 0
	for _, d := range Missing(s.Model().Documents, now) {
		m := s.Model()
		toDos, err := c.Read(ctx, Email{Title: d.Title, Date: d.Date, Author: d.Author, To: m.DocumentSentTo(d), Markdown: d.Markdown, Listed: listed(m, d.Key, now)})
		if err != nil {
			slog.ErrorContext(ctx, "todos: read an email", "error", err, "key", d.Key, "title", d.Title)
			return written
		}
		if err := s.SetToDos(ctx, actor, d.Key, toDos, Revision); err != nil {
			slog.ErrorContext(ctx, "todos: write the to-dos", "error", err, "key", d.Key)
			return written
		}
		written++
	}
	return written
}

func Start(s *model.Store, queue *store.Queue, c *Claude) {
	wake := make(chan struct{}, 1)
	var seen *model.Documents
	queue.OnSwap(func() {
		docs := s.Model().Documents
		if docs == seen {
			return
		}
		seen = docs
		select {
		case wake <- struct{}{}:
		default:
		}
	})
	go func() {
		for range wake {
			if n := Pass(context.Background(), s, c, time.Now()); n > 0 {
				slog.Info("todos: read emails", "emails", n)
			}
		}
	}()
}
