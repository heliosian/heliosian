package db

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	netmail "net/mail"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/claude"
	"heliosian/internal/store"
)

const (
	ClassifyModel = "claude-sonnet-5-5"
	classifyActor = "classify"
	classifyLimit = 24 << 10
)

const classifySystem = `You read one email the Helios School community received - the school's newsletter, a notice from the school's mailer, a message to all the families or to one class - and say whom it was written to.

The school's classrooms and grades are listed with the email, and so are the classrooms its sender teaches, when they teach any. A classroom holds two grades (Condors are 5th and 6th graders), so say each as the email does. If the email is written to the families or students of particular classrooms - its greeting ("Hi Condor Families"), its sign-off, or what it is about (one class's play, trip or homework) says so - list those classrooms. If it is written to particular grades - "Dear Parents of 2nd, 4th, 6th, and 8th graders", "for our 8th graders" - list those grades, and not the classrooms that hold them. List both only when it names both ("6th graders in Condors"). The middle school is grades 5 through 8, so an email to the middle school or its families is written to Grade 5, Grade 6, Grade 7 and Grade 8. The Yellowstone trip is for 7th and 8th graders, so an email about it is written to Grade 7 and Grade 8. Use the names given. If it is for the whole school, or a program you cannot tie to classrooms or grades, or you cannot tell, list neither. An email to one lit circle, or another group smaller than a classroom or grade, is written to none of them: say nobody, and list neither. A teacher writing about their own class is writing to that class, even without a greeting.`

var classifySchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"classrooms": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "the classrooms it was written to, by the names given; none for the whole school, for grades alone, or when unclear"},
		"grades":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "the grades it was written to, by the names given; none unless the email names grades"},
		"nobody":     map[string]any{"type": "boolean", "description": "true when it is written to a group smaller than any classroom or grade, such as one lit circle"},
	},
	"required":             []string{"classrooms", "grades", "nobody"},
	"additionalProperties": false,
}

type audience struct {
	Classrooms []string `json:"classrooms"`
	Grades     []string `json:"grades"`
	Nobody     bool     `json:"nobody"`
}

type Classifier struct {
	s       *Store
	queue   *store.Queue
	bucket  *blob.Bucket
	client  anthropic.Client
	skipped map[string]bool
	poke    chan struct{}
}

func StartClassifier(s *Store, queue *store.Queue, bucket *blob.Bucket, anthropicKey string) *Classifier {
	c := &Classifier{s: s, queue: queue, bucket: bucket, client: anthropic.NewClient(option.WithAPIKey(anthropicKey)), skipped: map[string]bool{}, poke: make(chan struct{}, 1)}
	go c.run()
	queue.OnSwap(func() {
		select {
		case c.poke <- struct{}{}:
		default:
		}
	})
	return c
}

func (c *Classifier) run() {
	for range c.poke {
		for _, id := range c.pending() {
			start := time.Now()
			outcome, err := c.classify(id)
			if err != nil {
				c.skipped[id] = true
				slog.Error("classify", "document", id, "error", err)
				continue
			}
			if outcome == "" {
				continue
			}
			slog.Info("classify: done", "document", id, "sent_to", outcome, "took", time.Since(start).Round(time.Millisecond))
		}
	}
}

func (c *Classifier) pending() []string {
	m := c.s.Model()
	links := m.Table("DOCUMENT_GROUP")
	out := []string{}
	for _, row := range m.Table("DOCUMENT").All() {
		if row["parent"] != "" || !slices.Contains([]string{"newsletter", "list"}, row["kind"]) || c.skipped[row["id"]] {
			continue
		}
		if !slices.ContainsFunc(links.Referencing("document", row["id"]), func(l store.Row) bool { return l["relation"] == "sent_to" }) {
			out = append(out, row["id"])
		}
	}
	return out
}

func (m *Model) bodyMarkdown(root store.Row) (string, bool) {
	if root["extracted"] == "" {
		return "", false
	}
	docs, contents := m.Table("DOCUMENT"), m.Table("CONTENT")
	parts := docs.Referencing("parent", root["id"])
	slices.SortFunc(parts, func(a, b store.Row) int { return store.CompareKeys(a["order"], b["order"]) })
	bodies := map[string]string{}
	for _, part := range parts {
		content, _ := contents.Get(part["content"])
		kind := baseType(content["mime"])
		if kind != "text/html" && kind != "text/plain" {
			continue
		}
		if part["extracted"] == "" {
			return "", false
		}
		if _, seen := bodies[kind]; seen {
			continue
		}
		for _, extract := range docs.Referencing("parent", part["id"]) {
			bodies[kind] = extract["content"]
		}
	}
	if id, ok := bodies["text/html"]; ok {
		return id, true
	}
	return bodies["text/plain"], true
}

func (c *Classifier) classify(id string) (string, error) {
	ctx := context.Background()
	m := c.s.Model()
	root, ok := m.Table("DOCUMENT").Get(id)
	if !ok {
		return "", nil
	}
	content, ok := m.Table("CONTENT").Get(root["content"])
	if !ok {
		return "", fmt.Errorf("its content %s is missing", root["content"])
	}
	raw, _, err := c.bucket.Get(ctx, content["blob"])
	if err != nil {
		return "", err
	}
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	if listName(msg.Header.Get("List-Id")) != "" {
		c.skipped[id] = true
		return "", nil
	}
	body, ready := m.bodyMarkdown(root)
	if !ready {
		return "", nil
	}
	markdown := ""
	if body != "" {
		b, _ := m.Table("CONTENT").Get(body)
		text, _, err := c.bucket.Get(ctx, b["blob"])
		if err != nil {
			return "", err
		}
		markdown = string(text)
		if len(markdown) > classifyLimit {
			markdown = markdown[:classifyLimit]
		}
	}
	classrooms, grades := m.groupNames("classroom"), m.groupNames("grade")
	prompt := fmt.Sprintf("The school's classrooms: %s\nThe school's grades: %s\nThe classrooms its sender teaches: %s\n\nSubject: %s\nFrom: %s\nSent: %s\n\n%s",
		strings.Join(classrooms, ", "), strings.Join(grades, ", "), strings.Join(m.teaches(root["author"]), ", "),
		root["name"], msg.Header.Get("From"), root["published"], markdown)
	var out audience
	if _, err := claude.JSON(ctx, c.client, anthropic.MessageNewParams{
		Model:        ClassifyModel,
		MaxTokens:    4000,
		System:       []anthropic.TextBlockParam{{Text: classifySystem}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))},
		OutputConfig: anthropic.OutputConfigParam{Format: anthropic.JSONOutputFormatParam{Schema: classifySchema}},
	}, &out); err != nil {
		return "", err
	}
	if out.Nobody {
		return "nobody: deleted", c.discard(id)
	}
	sentTo, err := m.audienceGroups(known(out.Classrooms, classrooms), known(out.Grades, grades))
	if err != nil {
		return "", err
	}
	_, err = c.queue.Transact(ctx, access.System(classifyActor), func(tx *store.Tx) error {
		if slices.ContainsFunc(c.s.In(tx).Table("DOCUMENT_GROUP").Referencing("document", id), func(l store.Row) bool { return l["relation"] == "sent_to" }) {
			return nil
		}
		for _, group := range sentTo {
			_, ch, err := stageWrite(c.s, tx, Edit{Insert: "DOCUMENT_GROUP", Row: map[string]any{"document": id, "group": group, "relation": "sent_to"}}, classifyActor, nil, true, unchecked)
			if err != nil {
				return err
			}
			if err := fire(c.s, tx, ch, classifyActor); err != nil {
				return err
			}
		}
		return nil
	})
	return strings.Join(sentTo, " "), err
}

func (c *Classifier) discard(id string) error {
	_, err := c.queue.Transact(context.Background(), access.System(classifyActor), func(tx *store.Tx) error {
		docs := c.s.In(tx).Table("DOCUMENT")
		order := []string{}
		var walk func(string)
		walk = func(node string) {
			for _, child := range docs.Referencing("parent", node) {
				walk(child["id"])
			}
			order = append(order, node)
		}
		walk(id)
		for _, node := range order {
			_, ch, err := stageWrite(c.s, tx, Edit{Delete: node}, classifyActor, nil, true, unchecked)
			if err != nil {
				return err
			}
			if err := fire(c.s, tx, ch, classifyActor); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

func known(named, school []string) []string {
	out := []string{}
	for _, n := range named {
		for _, s := range school {
			if strings.EqualFold(strings.TrimSpace(n), s) && !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
	}
	return out
}

func (m *Model) groupNames(kind string) []string {
	out := []string{}
	for _, g := range m.Table("GROUP").All() {
		if g["kind"] == kind {
			out = append(out, g["name"])
		}
	}
	slices.Sort(out)
	return out
}

func (m *Model) teaches(person string) []string {
	if person == "" {
		return nil
	}
	groups := m.Table("GROUP")
	out := []string{}
	for _, member := range m.Table("MEMBER").Referencing("person", person) {
		g, _ := groups.Get(member["group"])
		if g["kind"] == "crew" {
			g, _ = groups.Get(g["parent"])
		}
		if g["kind"] == "classroom" && !slices.Contains(out, g["name"]) {
			out = append(out, g["name"])
		}
	}
	return out
}

func (m *Model) audienceGroups(classrooms, grades []string) ([]string, error) {
	slugs := []string{}
	for _, name := range classrooms {
		slugs = append(slugs, strings.ToLower(name)+"-parents")
	}
	if len(slugs) == 0 {
		for _, g := range m.Table("GROUP").All() {
			if g["kind"] == "grade" && slices.Contains(grades, g["name"]) {
				slugs = append(slugs, g["slug"]+"-parents")
			}
		}
	}
	if len(slugs) == 0 {
		slugs = []string{"everyone"}
	}
	return m.groupsWhere(func(g map[string]string) bool { return g["kind"] == "group" && slices.Contains(slugs, g["slug"]) }, len(slugs), strings.Join(slugs, ", "))
}
