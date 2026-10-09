package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"heliosian/internal/blob"
	"heliosian/internal/db"
	"heliosian/internal/store"
)

const (
	listRows      = 500
	referrerRows  = 20
	documentNodes = 300
	maxAncestors  = 50
	documentText  = 30000
	clipped       = 400
)

type Deps struct {
	Data   *db.Store
	Search *db.Searcher
	Bucket *blob.Bucket
	Origin func(app string) string
	Now    func() time.Time
}

type Tool struct {
	Name        string
	Description string
	Words       string
	Schema      *jsonschema.Schema
	run         func(c *call, input json.RawMessage) (any, error)
}

type Set struct {
	deps  Deps
	Tools []Tool
}

type Answer struct {
	Text  string
	Links map[string]string
}

func New(deps Deps) *Set {
	return &Set{deps: deps, Tools: all}
}

func (s *Set) Tool(name string) (Tool, bool) {
	i := slices.IndexFunc(s.Tools, func(t Tool) bool { return t.Name == name })
	if i < 0 {
		return Tool{}, false
	}
	return s.Tools[i], true
}

func (s *Set) Run(ctx context.Context, m *db.Model, viewer, name string, input json.RawMessage) (Answer, error) {
	t, ok := s.Tool(name)
	if !ok {
		return Answer{}, fmt.Errorf("there is no tool called %s", name)
	}
	if len(bytes.TrimSpace(input)) == 0 {
		input = json.RawMessage(`{}`)
	}
	c := &call{ctx: ctx, m: m, env: db.Env{Viewer: viewer, Now: s.deps.Now()}, deps: s.deps, links: map[string]string{}}
	start := time.Now()
	out, err := t.run(c, input)
	slog.InfoContext(ctx, "tools: run", "tool", name, "viewer", viewer, "took", time.Since(start).Round(time.Millisecond), "error", err)
	if err != nil {
		return Answer{}, err
	}
	text, ok := out.(string)
	if !ok {
		encoded := &bytes.Buffer{}
		encoder := json.NewEncoder(encoded)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(out); err != nil {
			return Answer{}, err
		}
		text = strings.TrimSpace(encoded.String())
	}
	return Answer{Text: text, Links: c.links}, nil
}

func define[In any](name, words, description string, run func(c *call, in In) (any, error)) Tool {
	schema, err := jsonschema.For[In](nil)
	if err != nil {
		panic(fmt.Sprintf("tools: %s's input: %v", name, err))
	}
	return Tool{Name: name, Words: words, Description: description, Schema: schema, run: func(c *call, input json.RawMessage) (any, error) {
		var in In
		if err := json.Unmarshal(input, &in); err != nil {
			return nil, fmt.Errorf("the input could not be read: %w", err)
		}
		return run(c, in)
	}}
}

type tree = map[string]any

func path(p string) tree {
	return tree{"path": p}
}

func eq(p string, v any) tree {
	return tree{"=": []any{path(p), v}}
}

func among(p string, values []string) tree {
	args := []any{path(p)}
	for _, v := range values {
		args = append(args, v)
	}
	return tree{"in": args}
}

func asc(p string) []any {
	return []any{tree{"path": p, "dir": "asc"}}
}

type call struct {
	ctx   context.Context
	m     *db.Model
	env   db.Env
	deps  Deps
	mu    sync.Mutex
	links map[string]string
}

func (c *call) run(q tree) (db.Result, *db.Query, error) {
	raw, err := json.Marshal(q)
	if err != nil {
		return db.Result{}, nil, err
	}
	query, err := db.ParseJSON(raw)
	if err != nil {
		return db.Result{}, nil, fmt.Errorf("a built-in query was refused: %w", err)
	}
	return c.m.Run(c.ctx, query, c.env), query, nil
}

func (c *call) rows(q tree) ([]store.Row, db.Result, error) {
	res, _, err := c.run(q)
	if err != nil {
		return nil, res, err
	}
	return res.Rows(), res, nil
}

func (c *call) row(table, id string) (store.Row, bool, error) {
	rows, _, err := c.rows(tree{"from": table, "where": []any{eq("id", id)}})
	if err != nil || len(rows) == 0 {
		return nil, false, err
	}
	return rows[0], true, nil
}

func (c *call) href(table string, row store.Row) string {
	href := c.m.Link(table, row, c.deps.Origin)
	if href != "" && table != "DOCUMENT" {
		c.note(href, row["id"])
	}
	return href
}

func (c *call) note(href, id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.links[href] = id
}

func (c *call) compact(table string, row store.Row) store.Row {
	out := store.Row{}
	for k, v := range row {
		if v != "" {
			out[k] = v
		}
	}
	if href := c.href(table, row); href != "" {
		out["href"] = href
	}
	return out
}

func title(row store.Row) string {
	for _, c := range []string{"name_show", "name", "address", "key"} {
		if row[c] != "" {
			return row[c]
		}
	}
	return row["id"]
}

type named struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Href string `json:"href,omitempty"`
}

func (c *call) named(table string, row store.Row) named {
	return named{ID: row["id"], Name: title(row), Href: c.href(table, row)}
}

type rows struct {
	Query    string                          `json:"query,omitempty"`
	Count    int                             `json:"count"`
	Rows     []store.Row                     `json:"rows"`
	More     int                             `json:"more,omitempty"`
	Included map[string]map[string]store.Row `json:"included,omitempty"`
}

func (c *call) shape(res db.Result, q *db.Query) rows {
	out := rows{Count: len(res.IDs), Rows: []store.Row{}}
	if q != nil {
		out.Query = q.String()
	}
	for _, row := range res.Rows() {
		out.Rows = append(out.Rows, c.compact(res.Table, row))
	}
	for table, byID := range res.Resources {
		for id, row := range byID {
			if table == res.Table && slices.Contains(res.IDs, id) {
				continue
			}
			if out.Included == nil {
				out.Included = map[string]map[string]store.Row{}
			}
			if out.Included[table] == nil {
				out.Included[table] = map[string]store.Row{}
			}
			out.Included[table][id] = c.compact(table, row)
		}
	}
	return out
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + "…"
}

func tableOf(id, want string) (*db.Table, error) {
	name, ok := db.TableOf(strings.TrimSpace(id))
	if !ok {
		return nil, fmt.Errorf("%q is not an ID", id)
	}
	if want != "" && name != want {
		return nil, fmt.Errorf("%s is a %s, not a %s", id, name, want)
	}
	t, _ := db.Lookup(name)
	return t, nil
}

func day(now time.Time, cell, field string) (time.Time, error) {
	if strings.TrimSpace(cell) == "" {
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), nil
	}
	t, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(cell), now.Location())
	if err != nil {
		return t, fmt.Errorf("%s is YYYY-MM-DD, not %q", field, cell)
	}
	return t, nil
}

var all = []Tool{
	whoami, search, similar, events, days, findPeople, classroom, group, nearbyFamilies,
	activities, parties, lists, links, readDocument, get, query, describeSchema, describeTable, policies,
}
