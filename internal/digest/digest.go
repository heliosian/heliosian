package digest

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

const Revision = "2026-10-03.1"

var actor = access.System("digest")

const system = `You read one email the Helios School community received - the school's newsletter, a message to all the families or to one class, or a post to a parent email list - and do four things.

First, sum it up in one plain sentence under twenty words, for a parent deciding whether to open it: what the email is about and what it asks, if anything ("The Jays visit the tide pools Oct 9 and need signed slips and drivers."). Name no sender and add nothing the email does not say.

Second, list its key points for a parent skimming their week. Write three to five points, fewer for a short email. Lead with what a family must do or know by a date: deadlines, events with their day and time, things to bring, forms to send. Then the rest that matters most. Each point is one plain sentence under twenty words, naming the day ("Tue, Sep 29") where the email gives one. Say only what the email says; no greetings, no sign-offs, no advice of your own. If the email has nothing worth a point - an automatic notice, an empty message - return no points.

Third, say whom it was written to. The school's classrooms and grades are listed with the email, and so are the classrooms its sender teaches, when they teach any. A classroom holds two grades (Condors are 5th and 6th graders), so say each as the email does. If the email is written to the families or students of particular classrooms - its greeting ("Hi Condor Families"), its sign-off, or what it is about (one class's play, trip or homework) says so - list those classrooms. If it is written to particular grades - "Dear Parents of 2nd, 4th, 6th, and 8th graders", "for our 8th graders" - list those grades, and not the classrooms that hold them. List both only when it names both ("6th graders in Condors"). The middle school is grades 5 through 8, so an email to the middle school or its families is written to Grade 5, Grade 6, Grade 7 and Grade 8. The Yellowstone trip is for 7th and 8th graders, so an email about it is written to Grade 7 and Grade 8. Use the names given. If it is for the whole school, or a program you cannot tie to classrooms or grades, or you cannot tell, list neither. An email to one lit circle, or another group smaller than a classroom or grade, is written to none of them: say nobody, and list neither. A teacher writing about their own class is writing to that class, even without a greeting.

Fourth, list what it asks families to do. A to-do is something a parent or student has to act on: a form to fill out or return, something to sign, pay, buy, bring, send or sign up for, a reply or an RSVP, a deadline to meet. Leave out what only informs: an event with nothing to do before it, a schedule change, news, thanks. An event is a to-do only when the email asks for something before it - a sign-up, a ticket, a permission slip, something to bring. A call for volunteers counts when the email asks its readers directly. If the email asks nothing of its readers, return no to-dos.

First say in asks, in a sentence or two, what the email asks its readers to do, or that it asks nothing. Then list a to-do for each thing it asks.

For each to-do:
- title: the action, starting with a verb, at most eight words ("Return the tide pool permission slip").
- summary: five to ten words on what it is for ("Signed slip for the Jays' tide pool trip").
- details: one or two plain sentences with what a parent needs to do it - what, by when, where it goes, what it costs. Say only what the email says.
- link: the web address the email gives for doing it - the form, the sign-up, the payment page - copied exactly as it appears; empty when it gives none. Never an email address. When the email gives a different link for each class or grade, leave link empty and say in details to use the link for your child's class.
- due: the day it has to be done by, as YYYY-MM-DD, worked out from the email's words and the day it was sent; for something to bring or do at an event, the event's day; empty when the email names no day.
- point: the number of the key point that says the same thing, 1 for the first; 0 when no point does. What a family must do leads the key points, so most to-dos have one.

The list already holds the to-dos given with the email, numbered, from earlier emails. Leave out any to-do already there, however this email words it.

Last, in repeats, name each key point that says the same thing as a to-do already on the list: the point's number, 1 for the first, and that to-do's number on the list. A reminder of an earlier email's to-do is a key point that repeats it.`

type field struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type listField struct {
	Type        string `json:"type"`
	Items       field  `json:"items"`
	Description string `json:"description"`
}

// The model writes an object's fields in the order the schema lists them, and a map would list them
// alphabetically; structs keep the points ahead of the to-dos that name them, and each to-do's title first.
type toDoFields struct {
	Title   field `json:"title"`
	Summary field `json:"summary"`
	Details field `json:"details"`
	Link    field `json:"link"`
	Due     field `json:"due"`
	Point   field `json:"point"`
}

type toDoList struct {
	Type        string `json:"type"`
	Items       any    `json:"items"`
	Description string `json:"description"`
}

type repeatFields struct {
	Point  field `json:"point"`
	Listed field `json:"listed"`
}

type readingFields struct {
	Summary    field     `json:"summary"`
	Points     listField `json:"points"`
	Classrooms listField `json:"classrooms"`
	Grades     listField `json:"grades"`
	Nobody     field     `json:"nobody"`
	Asks       field     `json:"asks"`
	ToDos      toDoList  `json:"todos"`
	Repeats    toDoList  `json:"repeats"`
}

var schema = map[string]any{
	"type": "object",
	"properties": readingFields{
		Summary:    field{"string", "one plain sentence under twenty words on what the email is about"},
		Points:     listField{"array", field{"string", "one key point"}, "the key points, most pressing first"},
		Classrooms: listField{"array", field{"string", "a classroom"}, "the classrooms it was written to, by the names given; none for the whole school, for grades alone, or when unclear"},
		Grades:     listField{"array", field{"string", "a grade"}, "the grades it was written to, by the names given; none unless the email names grades"},
		Nobody:     field{"boolean", "true when it is written to a group smaller than any classroom or grade, such as one lit circle"},
		Asks:       field{"string", "in a sentence or two, what the email asks its readers to do, or that it asks nothing"},
		ToDos: toDoList{
			Type: "array",
			Items: map[string]any{
				"type": "object",
				"properties": toDoFields{
					Title:   field{"string", "the action, starting with a verb, at most eight words"},
					Summary: field{"string", "five to ten words on what it is for"},
					Details: field{"string", "one or two sentences"},
					Link:    field{"string", "the web address for doing it, copied from the email, or empty"},
					Due:     field{"string", "YYYY-MM-DD, or empty"},
					Point:   field{"integer", "the number of the key point that says the same thing, or 0"},
				},
				"required":             []string{"title", "summary", "details", "link", "due", "point"},
				"additionalProperties": false,
			},
			Description: "what the email asks families to do, soonest first",
		},
		Repeats: toDoList{
			Type: "array",
			Items: map[string]any{
				"type": "object",
				"properties": repeatFields{
					Point:  field{"integer", "the number of the key point"},
					Listed: field{"integer", "the number of the to-do already on the list that it says the same as"},
				},
				"required":             []string{"point", "listed"},
				"additionalProperties": false,
			},
			Description: "each key point that repeats a to-do already on the list",
		},
	},
	"required":             []string{"summary", "points", "classrooms", "grades", "nobody", "asks", "todos", "repeats"},
	"additionalProperties": false,
}

type Listed struct {
	ID, Title, Due, To string
}

type repeat struct {
	Point  int `json:"point"`
	Listed int `json:"listed"`
}

type Email struct {
	Title, Date, Author, To, Markdown string
	Classrooms, Grades, Teaches       []string
	Listed                            []Listed
}

type Claude struct {
	client anthropic.Client
}

func New(key string) *Claude {
	return &Claude{client: anthropic.NewClient(option.WithAPIKey(key))}
}

type answer struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Details string `json:"details"`
	Link    string `json:"link"`
	Due     string `json:"due"`
	Point   int    `json:"point"`
}

func (c *Claude) Read(ctx context.Context, e Email) (model.Reading, error) {
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
	teaches := "none"
	if len(e.Teaches) > 0 {
		teaches = strings.Join(e.Teaches, ", ")
	}
	listed := []string{}
	for i, l := range e.Listed {
		line := fmt.Sprintf("%d. %s (for %s", i+1, l.Title, l.To)
		if l.Due != "" {
			line += ", due " + l.Due
		}
		listed = append(listed, line+")")
	}
	if len(listed) == 0 {
		listed = append(listed, "(nothing yet)")
	}
	prompt := fmt.Sprintf("The school's classrooms: %s\nThe school's grades: %s\nThe sender teaches: %s\n\nAlready on the list:\n%s\n\nSubject: %s\nFrom: %s\nSent to: %s\nSent: %s\n\n%s",
		strings.Join(e.Classrooms, ", "), strings.Join(e.Grades, ", "), teaches, strings.Join(listed, "\n"), e.Title, e.Author, e.To, sent, markdown)
	var out struct {
		Summary    string   `json:"summary"`
		Points     []string `json:"points"`
		Classrooms []string `json:"classrooms"`
		Grades     []string `json:"grades"`
		Nobody     bool     `json:"nobody"`
		Asks       string   `json:"asks"`
		ToDos      []answer `json:"todos"`
		Repeats    []repeat `json:"repeats"`
	}
	raw, err := claude.JSON(ctx, c.client, anthropic.MessageNewParams{
		Model:        claudeModel,
		MaxTokens:    4000,
		System:       []anthropic.TextBlockParam{{Text: system, CacheControl: anthropic.NewCacheControlEphemeralParam()}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortMedium, Format: anthropic.JSONOutputFormatParam{Schema: schema}},
	}, &out)
	if err != nil {
		return model.Reading{}, err
	}
	points := cleanPoints(out.Points)
	reading := model.Reading{Summary: tidy(out.Summary), Points: points, Audience: audience(out.Nobody, known(out.Classrooms, e.Classrooms), known(out.Grades, e.Grades)), ToDos: clean(out.ToDos, e.Markdown, len(points)), Repeats: repeats(out.Repeats, len(points), e.Listed)}
	slog.InfoContext(ctx, "digest: read an email", "title", e.Title, "summary", reading.Summary, "points", len(points), "audience", reading.Audience, "asks", out.Asks, "asked", len(out.ToDos), "kept", len(reading.ToDos), "repeats", len(reading.Repeats))
	if reading.Summary == "" {
		slog.InfoContext(ctx, "digest: claude wrote no summary", "title", e.Title)
	}
	if len(reading.ToDos) < len(out.ToDos) {
		slog.InfoContext(ctx, "digest: claude's whole answer, some of it left out", "title", e.Title, "answer", raw)
	}
	return reading, nil
}

func repeats(answers []repeat, points int, listed []Listed) map[int]string {
	out := map[int]string{}
	for _, r := range answers {
		if r.Point < 1 || r.Point > points || r.Listed < 1 || r.Listed > len(listed) {
			slog.Info("digest: a repeat naming no point or no listed to-do, dropped", "point", r.Point, "listed", r.Listed)
			continue
		}
		if _, taken := out[r.Point]; !taken {
			out[r.Point] = listed[r.Listed-1].ID
		}
	}
	return out
}

func audience(nobody bool, classrooms, grades []string) string {
	if nobody {
		return ""
	}
	named := append(slices.Clone(classrooms), grades...)
	if len(named) == 0 {
		return model.DocumentForEveryone
	}
	return strings.Join(named, ", ")
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

var spaces = regexp.MustCompile(`\s+`)

func tidy(s string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(s, " "))
}

func cleanPoints(points []string) []string {
	out := []string{}
	for _, p := range points {
		if p = tidy(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func clean(answers []answer, markdown string, points int) []model.ToDo {
	out := []model.ToDo{}
	titles := []string{}
	for _, a := range answers {
		t := model.ToDo{Title: strings.TrimRight(tidy(a.Title), "."), Summary: tidy(a.Summary), Details: tidy(a.Details), Link: tidy(a.Link), Due: tidy(a.Due), Point: a.Point}
		if t.Link != "" && !strings.Contains(markdown, t.Link) {
			slog.Info("digest: a link the email does not hold, dropped", "title", t.Title, "link", t.Link)
			t.Link = ""
		}
		if t.Due != "" {
			if _, err := time.ParseInLocation(model.DateFormat, t.Due, model.Location); err != nil {
				slog.Info("digest: a due day that is not a date, dropped", "title", t.Title, "due", t.Due)
				t.Due = ""
			}
		}
		if t.Point < 0 || t.Point > points {
			slog.Info("digest: a point the email's points do not have, dropped", "title", t.Title, "point", t.Point)
			t.Point = 0
		}
		if t.Summary == "" || t.Details == "" {
			slog.Info("digest: a to-do with no summary or details left out", "title", t.Title)
			continue
		}
		if err := model.CheckToDo(t); err != nil {
			slog.Info("digest: a to-do left out", "error", err)
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
		if d.Date < since || !d.ToDoSource() || m.Read[d.Key] >= Revision {
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
		out = append(out, Listed{ID: t.ID, Title: t.Title, Due: t.Due, To: m.DocumentSentTo(d)})
	}
	return out
}

func Pass(ctx context.Context, s *model.Store, c *Claude, now time.Time) int {
	written := 0
	for _, d := range Missing(s.Model().Documents, now) {
		m := s.Model()
		school := m.Directory
		reading, err := c.Read(ctx, Email{
			Title: d.Title, Date: d.Date, Author: d.Author, To: m.DocumentSentTo(d), Markdown: d.Markdown,
			Classrooms: school.ClassroomNames(), Grades: school.GradeNames(), Teaches: school.Teaches(d.Author), Listed: listed(m, d.Key, now),
		})
		if err != nil {
			slog.ErrorContext(ctx, "digest: read an email", "error", err, "key", d.Key, "title", d.Title)
			return written
		}
		if err := s.SetReading(ctx, actor, d.Key, reading, Revision); err != nil {
			slog.ErrorContext(ctx, "digest: write the reading", "error", err, "key", d.Key)
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
				slog.Info("digest: read emails", "emails", n)
			}
		}
	}()
}
