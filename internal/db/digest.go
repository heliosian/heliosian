package db

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	netmail "net/mail"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/claude"
	"heliosian/internal/store"
)

const (
	digestActor   = "digest"
	digestLimit   = 24 << 10
	digestBackoff = 30 * time.Second
	MailWindow    = 14 * 24 * time.Hour
)

const DigestRevision = "2026-10-17.1"

const digestSystem = `You read one email the Helios School community received - the school's newsletter, a message to all the families or to one class, or a post to a parent email list - and do three things.

First, sum it up in one plain sentence under twenty words, for a parent deciding whether to open it: what the email is about and what it asks, if anything ("The Jays visit the tide pools Oct 9 and need signed slips and drivers."). Name no sender and add nothing the email does not say.

Second, list its key points for a parent skimming their week. Write three to five points, fewer for a short email. Lead with what a family must do or know by a date: deadlines, events with their day and time, things to bring, forms to send. Then the rest that matters most. Each point is one plain sentence under twenty words, naming the day ("Tue, Sep 29") where the email gives one. Say only what the email says; no greetings, no sign-offs, no advice of your own. If the email has nothing worth a point - an automatic notice, an empty message - return no points.

Third, list what it asks families to do. A to-do is something a parent or student has to act on: a form to fill out or return, something to sign, pay, buy, bring, send or sign up for, a reply or an RSVP, a deadline to meet. Leave out what only informs: an event with nothing to do before it, a schedule change, news, thanks. An event is a to-do only when the email asks for something before it - a sign-up, a ticket, a permission slip, something to bring. A call for volunteers counts when the email asks its readers directly. If the email asks nothing of its readers, return no to-dos.

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

type digestField struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type digestList struct {
	Type        string `json:"type"`
	Items       any    `json:"items"`
	Description string `json:"description"`
}

// The model writes an object's fields in the order the schema lists them, and a map would list them
// alphabetically; structs keep the points ahead of the to-dos that name them, and each to-do's title first.
type digestToDoFields struct {
	Title   digestField `json:"title"`
	Summary digestField `json:"summary"`
	Details digestField `json:"details"`
	Link    digestField `json:"link"`
	Due     digestField `json:"due"`
	Point   digestField `json:"point"`
}

type digestRepeatFields struct {
	Point  digestField `json:"point"`
	Listed digestField `json:"listed"`
}

type digestFields struct {
	Summary digestField `json:"summary"`
	Points  digestList  `json:"points"`
	Asks    digestField `json:"asks"`
	ToDos   digestList  `json:"todos"`
	Repeats digestList  `json:"repeats"`
}

var digestSchema = map[string]any{
	"type": "object",
	"properties": digestFields{
		Summary: digestField{"string", "one plain sentence under twenty words on what the email is about"},
		Points:  digestList{"array", digestField{"string", "one key point"}, "the key points, most pressing first"},
		Asks:    digestField{"string", "in a sentence or two, what the email asks its readers to do, or that it asks nothing"},
		ToDos: digestList{
			Type: "array",
			Items: map[string]any{
				"type": "object",
				"properties": digestToDoFields{
					Title:   digestField{"string", "the action, starting with a verb, at most eight words"},
					Summary: digestField{"string", "five to ten words on what it is for"},
					Details: digestField{"string", "one or two sentences"},
					Link:    digestField{"string", "the web address for doing it, copied from the email, or empty"},
					Due:     digestField{"string", "YYYY-MM-DD, or empty"},
					Point:   digestField{"integer", "the number of the key point that says the same thing, or 0"},
				},
				"required":             []string{"title", "summary", "details", "link", "due", "point"},
				"additionalProperties": false,
			},
			Description: "what the email asks families to do, soonest first",
		},
		Repeats: digestList{
			Type: "array",
			Items: map[string]any{
				"type": "object",
				"properties": digestRepeatFields{
					Point:  digestField{"integer", "the number of the key point"},
					Listed: digestField{"integer", "the number of the to-do already on the list that it says the same as"},
				},
				"required":             []string{"point", "listed"},
				"additionalProperties": false,
			},
			Description: "each key point that repeats a to-do already on the list",
		},
	},
	"required":             []string{"summary", "points", "asks", "todos", "repeats"},
	"additionalProperties": false,
}

type digestToDo struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Details string `json:"details"`
	Link    string `json:"link"`
	Due     string `json:"due"`
	Point   int    `json:"point"`
}

type digestRepeat struct {
	Point  int `json:"point"`
	Listed int `json:"listed"`
}

type digestAnswer struct {
	Summary string         `json:"summary"`
	Points  []string       `json:"points"`
	Asks    string         `json:"asks"`
	ToDos   []digestToDo   `json:"todos"`
	Repeats []digestRepeat `json:"repeats"`
}

type listedToDo struct {
	id, title, due, to, published, order string
}

type Digest struct {
	s       *Store
	queue   *store.Queue
	bucket  *blob.Bucket
	client  anthropic.Client
	skipped map[string]bool
	in      *inbox
	now     func() time.Time
}

func StartDigest(s *Store, queue *store.Queue, bucket *blob.Bucket, anthropicKey string, now func() time.Time) *Digest {
	d := &Digest{s: s, queue: queue, bucket: bucket, client: anthropic.NewClient(option.WithAPIKey(anthropicKey)), skipped: map[string]bool{}, in: s.Model().services.inbox(digestInbox), now: now}
	go d.run()
	d.in.wake.poke()
	return d
}

func (d *Digest) run() {
	for range d.in.wake {
		d.in.take()
		again := false
		for _, id := range d.pending() {
			start := time.Now()
			err := d.read(id)
			if errors.Is(err, errAskAgain) {
				slog.Error("digest, to be asked again", "document", id, "error", err)
				time.Sleep(digestBackoff)
				again = true
				continue
			}
			if err != nil {
				d.skipped[id] = true
				slog.Error("digest", "document", id, "error", err)
				continue
			}
			slog.Info("digest: read", "document", id, "took", time.Since(start).Round(time.Millisecond))
		}
		if again {
			d.in.wake.poke()
		}
	}
}

func (d *Digest) since() string {
	return d.now().In(School).Add(-MailWindow).Format(DateLayout)
}

func (d *Digest) pending() []string {
	m := d.s.Model()
	since := d.since()
	rows := []store.Row{}
	for _, row := range m.Table("DOCUMENT").All() {
		if row["parent"] != "" || row["kind"] != "mail" || row["published"] < since || row["digest"] >= DigestRevision || d.skipped[row["id"]] {
			continue
		}
		if _, ready := m.bodyMarkdown(row); !ready || len(m.sentTo(row["id"])) == 0 {
			continue
		}
		rows = append(rows, row)
	}
	slices.SortFunc(rows, func(a, b store.Row) int { return strings.Compare(a["published"], b["published"]) })
	out := []string{}
	for _, row := range rows {
		out = append(out, row["id"])
	}
	return out
}

func (m *Model) sentTo(document string) []string {
	out := []string{}
	for _, link := range m.Table("DOCUMENT_GROUP").Referencing("document", document) {
		if link["relation"] == "sent_to" {
			out = append(out, link["group"])
		}
	}
	return out
}

func (m *Model) sentToNames(document string) string {
	names := []string{}
	for _, g := range m.sentTo(document) {
		row, _ := m.Table("GROUP").Get(g)
		names = append(names, row["name"])
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

func (d *Digest) listed(m *Model, except string) []listedToDo {
	since, today := d.since(), d.now().In(School).Format(DateLayout)
	docs := m.Table("DOCUMENT")
	out := []listedToDo{}
	for _, t := range m.Table("TODO").All() {
		doc, ok := docs.Get(t["document"])
		if !ok || doc["id"] == except || doc["published"] < since || (t["due"] != "" && t["due"] < today) {
			continue
		}
		out = append(out, listedToDo{id: t["id"], title: t["title"], due: t["due"], to: m.sentToNames(doc["id"]), published: doc["published"], order: t["order"]})
	}
	slices.SortFunc(out, func(a, b listedToDo) int {
		if c := strings.Compare(a.published, b.published); c != 0 {
			return c
		}
		return store.CompareKeys(a.order, b.order)
	})
	return out
}

func (d *Digest) markdown(ctx context.Context, m *Model, root store.Row) (string, string, error) {
	content, ok := m.Table("CONTENT").Get(root["content"])
	if !ok {
		return "", "", fmt.Errorf("its content %s is missing", root["content"])
	}
	raw, _, err := d.bucket.Get(ctx, content["blob"])
	if err != nil {
		return "", "", err
	}
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return "", "", err
	}
	body, _ := m.bodyMarkdown(root)
	if body == "" {
		return msg.Header.Get("From"), "", nil
	}
	b, _ := m.Table("CONTENT").Get(body)
	text, _, err := d.bucket.Get(ctx, b["blob"])
	if err != nil {
		return "", "", err
	}
	markdown := string(text)
	if len(markdown) > digestLimit {
		markdown = markdown[:digestLimit]
	}
	return msg.Header.Get("From"), markdown, nil
}

func (d *Digest) read(id string) error {
	ctx := context.Background()
	m := d.s.Model()
	root, ok := m.Table("DOCUMENT").Get(id)
	if !ok {
		return nil
	}
	from, markdown, err := d.markdown(ctx, m, root)
	if err != nil {
		return err
	}
	listed := d.listed(m, id)
	lines := []string{}
	for i, l := range listed {
		line := fmt.Sprintf("%d. %s (for %s", i+1, l.title, l.to)
		if l.due != "" {
			line += ", due " + l.due
		}
		lines = append(lines, line+")")
	}
	if len(lines) == 0 {
		lines = append(lines, "(nothing yet)")
	}
	sent := root["published"]
	if at, err := time.ParseInLocation(cells.StampFormat, root["published"], School); err == nil {
		sent = at.Format("Monday, January 2, 2006")
	}
	prompt := fmt.Sprintf("Already on the list:\n%s\n\nSubject: %s\nFrom: %s\nSent to: %s\nSent: %s\n\n%s",
		strings.Join(lines, "\n"), root["name"], from, m.sentToNames(id), sent, markdown)
	var out digestAnswer
	raw, err := claude.JSON(ctx, d.client, anthropic.MessageNewParams{
		Model:        claude.DigestModel,
		MaxTokens:    4000,
		System:       []anthropic.TextBlockParam{{Text: digestSystem, CacheControl: anthropic.NewCacheControlEphemeralParam()}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))},
		OutputConfig: anthropic.OutputConfigParam{Effort: claude.DigestEffort, Format: anthropic.JSONOutputFormatParam{Schema: digestSchema}},
	}, &out)
	if err != nil {
		if errors.Is(err, claude.ErrFinal) {
			return err
		}
		return fmt.Errorf("%w: %w", errAskAgain, err)
	}
	points := digestPoints(out.Points)
	repeated := digestRepeats(out.Repeats, len(points), listed)
	toDos := digestUnrepeated(digestClean(out.ToDos, markdown, len(points)), repeated)
	slog.Info("digest: read an email", "document", id, "title", root["name"], "summary", tidy(out.Summary), "points", len(points), "asks", out.Asks, "asked", len(out.ToDos), "kept", len(toDos), "repeats", len(repeated))
	if len(toDos) < len(out.ToDos) {
		slog.Info("digest: claude's whole answer, some of it left out", "document", id, "answer", raw)
	}
	return d.write(id, tidy(out.Summary), points, repeated, toDos)
}

func (d *Digest) write(id, summary string, points []string, repeated map[int]string, toDos []digestToDo) error {
	_, err := d.queue.Transact(context.Background(), access.System(digestActor), func(tx *store.Tx) error {
		m := d.s.In(tx)
		if _, ok := m.Table("DOCUMENT").Get(id); !ok {
			return nil
		}
		repeats := []string{}
		for _, point := range slices.Sorted(maps.Keys(repeated)) {
			if _, ok := m.Table("TODO").Get(repeated[point]); ok {
				repeats = append(repeats, strconv.Itoa(point)+": "+repeated[point])
			}
		}
		w := &pendingWrites{}
		w.add(Edit{Set: id, Cells: map[string]any{"summary": summary, "key_points": strings.Join(points, "\n"), "repeats": strings.Join(repeats, "\n"), "digest": DigestRevision}})
		held := map[string]store.Row{}
		for _, t := range m.Table("TODO").Referencing("document", id) {
			held[strings.ToLower(t["title"])] = t
		}
		orders := store.Order(make([]string, len(toDos)))
		for i, t := range toDos {
			point := ""
			if t.Point > 0 {
				point = strconv.Itoa(t.Point)
			}
			values := map[string]any{"point": point, "title": t.Title, "summary": t.Summary, "details": t.Details, "link": t.Link, "due": t.Due, "order": orders[i]}
			if old, ok := held[strings.ToLower(t.Title)]; ok {
				delete(held, strings.ToLower(t.Title))
				w.add(Edit{Set: old["id"], Cells: values})
				continue
			}
			values["document"] = id
			w.add(Edit{Insert: "TODO", Row: values})
		}
		for _, old := range held {
			w.add(Edit{Delete: old["id"]})
		}
		_, err := stageEdits(d.s, tx, Env{System: digestActor}, w.edits, digestActor, true)
		return err
	})
	return err
}

func digestRepeats(answers []digestRepeat, points int, listed []listedToDo) map[int]string {
	out := map[int]string{}
	for _, r := range answers {
		if r.Point < 1 || r.Point > points || r.Listed < 1 || r.Listed > len(listed) {
			slog.Info("digest: a repeat naming no point or no listed to-do, dropped", "point", r.Point, "listed", r.Listed)
			continue
		}
		if _, taken := out[r.Point]; !taken {
			out[r.Point] = listed[r.Listed-1].id
		}
	}
	return out
}

func digestUnrepeated(toDos []digestToDo, repeated map[int]string) []digestToDo {
	out := []digestToDo{}
	for _, t := range toDos {
		if _, ok := repeated[t.Point]; ok {
			slog.Info("digest: a to-do on a point that repeats another email's, dropped", "title", t.Title, "point", t.Point)
			continue
		}
		out = append(out, t)
	}
	return out
}

var spaces = regexp.MustCompile(`\s+`)

func tidy(s string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(s, " "))
}

func digestPoints(points []string) []string {
	out := []string{}
	for _, p := range points {
		if p = tidy(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func digestClean(answers []digestToDo, markdown string, points int) []digestToDo {
	out := []digestToDo{}
	titles := []string{}
	for _, a := range answers {
		t := digestToDo{Title: strings.TrimRight(tidy(a.Title), "."), Summary: tidy(a.Summary), Details: tidy(a.Details), Link: tidy(a.Link), Due: tidy(a.Due), Point: a.Point}
		if t.Link != "" && (!strings.Contains(markdown, t.Link) || !strings.HasPrefix(t.Link, "http")) {
			slog.Info("digest: a link the email does not hold, dropped", "title", t.Title, "link", t.Link)
			t.Link = ""
		}
		if t.Due != "" {
			if _, err := time.ParseInLocation(DateLayout, t.Due, School); err != nil {
				slog.Info("digest: a due day that is not a date, dropped", "title", t.Title, "due", t.Due)
				t.Due = ""
			}
		}
		if t.Point < 0 || t.Point > points {
			slog.Info("digest: a point the email's points do not have, dropped", "title", t.Title, "point", t.Point)
			t.Point = 0
		}
		if t.Title == "" || t.Summary == "" || t.Details == "" {
			slog.Info("digest: a to-do with no title, summary or details left out", "title", t.Title)
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
