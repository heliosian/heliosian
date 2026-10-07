package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"heliosian/internal/db"
	"heliosian/internal/store"
)

const (
	maxOutput     = 200 << 10
	referrerRows  = 20
	listRows      = 500
	documentNodes = 300
	maxAncestors  = 50
	eventsDays    = 14
)

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

type call struct {
	ctx context.Context
	m   *db.Model
	env db.Env
	s   *Server
}

func (c call) run(q tree) (db.Result, *db.Query, error) {
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

func (c call) compact(table string, row store.Row) store.Row {
	out := store.Row{}
	for k, v := range row {
		if v != "" {
			out[k] = v
		}
	}
	if href := db.Link(table, row, c.s.origin); href != "" {
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

type rows struct {
	Query    string                          `json:"query,omitempty"`
	Count    int                             `json:"count"`
	Rows     []store.Row                     `json:"rows"`
	More     int                             `json:"more,omitempty"`
	Included map[string]map[string]store.Row `json:"included,omitempty"`
}

func (c call) shape(res db.Result, q *db.Query, keep int) rows {
	out := rows{Count: len(res.IDs), Rows: []store.Row{}}
	if q != nil {
		out.Query = q.String()
	}
	for i, row := range res.Rows() {
		if keep > 0 && i == keep {
			out.More = len(res.IDs) - keep
			break
		}
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

func refColumns(t *db.Table) []string {
	out := []string{}
	for _, c := range t.Columns {
		if c.Kind == db.Ref && c.Target != "" && !c.Private {
			out = append(out, c.Name)
		}
	}
	return out
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

func addTool[In any](s *Server, name, description string, run func(c call, in In) (any, error)) {
	tool := &sdk.Tool{Name: name, Description: description, Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true}}
	sdk.AddTool(s.server, tool, func(ctx context.Context, req *sdk.CallToolRequest, in In) (*sdk.CallToolResult, any, error) {
		m := s.deps.Data.Model()
		email := req.Extra.TokenInfo.UserID
		viewer := m.SignedIn(email)
		if viewer == "" {
			return nil, nil, fmt.Errorf("%s is not in the directory", email)
		}
		start := time.Now()
		out, err := run(call{ctx: ctx, m: m, env: db.Env{Viewer: viewer, Now: s.deps.Now()}, s: s}, in)
		slog.InfoContext(ctx, "mcp: tool", "tool", name, "viewer", viewer, "took", time.Since(start).Round(time.Millisecond), "error", err)
		if err != nil {
			return nil, nil, err
		}
		text, ok := out.(string)
		if !ok {
			raw, err := json.Marshal(out)
			if err != nil {
				return nil, nil, err
			}
			text = string(raw)
		}
		if len(text) > maxOutput {
			return nil, nil, fmt.Errorf("the answer runs to %d bytes, past the %d this tool returns; narrow it, or add a limit", len(text), maxOutput)
		}
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: text}}}, nil, nil
	})
}

type none struct{}

type queryIn struct {
	Query string `json:"query" jsonschema:"the query in the query language, one (from TABLE ...) expression"`
}

type tableIn struct {
	Table string `json:"table" jsonschema:"the table's name, upper case, as describe_schema lists it"`
}

type idIn struct {
	ID string `json:"id" jsonschema:"the row's ID"`
}

type wordsIn struct {
	Words string `json:"words" jsonschema:"what to look for, in words"`
}

type peopleIn struct {
	Words     string `json:"words,omitempty" jsonschema:"words of a name, job title, about-me or the like, matched by the search index"`
	Role      string `json:"role,omitempty" jsonschema:"student, parent or staff"`
	Grade     string `json:"grade,omitempty" jsonschema:"a student's grade: K or 1 to 8"`
	Classroom string `json:"classroom,omitempty" jsonschema:"a student's classroom, by name"`
}

type eventsIn struct {
	From string `json:"from,omitempty" jsonschema:"the first day, YYYY-MM-DD; today when left out"`
	To   string `json:"to,omitempty" jsonschema:"the last day, YYYY-MM-DD; two weeks after from when left out"`
}

func (s *Server) tools() {
	addTool(s, "helios_search", "Search Helios School's community data - people, families, classrooms, events, volunteer activities, parties, email lists, and the newsletters and school mail - by words and by meaning. Use it first for any question about Helios, the school, a family, child, teacher or classroom there, or what the school has sent out, such as \"who teaches the Jays\", \"when is picture day\" or \"what did the newsletter say about the auction\". Answers each hit's ID, table, name, summary and href, best first; helios_get or helios_read_document opens one. Every tool gives a record with a page on the Helios apps its href: link to it whenever you name the record, and never make up an address.", search)
	addTool(s, "helios_whoami", "The person connected to Helios School's community data: their record, their addresses, their family and children, the classrooms, sign-ups, tickets and email lists they are in, the groups they manage and the Helios apps they are an admin of. Use it for questions about \"my family\", \"my kids\" or \"my classes\" at Helios.", whoami)
	addTool(s, "helios_events", "Helios School's calendar: school and community events starting in a range of days, in order, with the category each sits under. Use it for what is coming up at Helios.", events)
	addTool(s, "helios_find_people", "People in the Helios School directory by any of words (a name, job title or the like), role (student, parent or staff), grade and classroom, with their classroom, crew and department named.", findPeople)
	addTool(s, "helios_read_document", "A Helios School document by ID - a newsletter or other school email, a page or file it linked, a PDF's reading, or a Helios Wiki page - read whole: the tree of its parts, links, images and extracts, and the text of each.", readDocument)
	addTool(s, "helios_get", "One record of Helios School's community data by ID: its filled fields, the names of what it refers to, and the records elsewhere that point at it, the first few of each.", get)
	addTool(s, "helios_group", "A Helios School group by ID in depth - a family, classroom, grade, event, volunteer activity, party, email list or any other group: who manages it and through which group, its members with why each is in, its rules, and the groups under it.", group)
	addTool(s, "helios_query", "Run a query in Helios's query language over the school community's data, as the person connected. Answers the query's canonical form, how many rows matched, the rows with their filled columns, and every row the includes brought by table and ID. helios_describe_schema and helios_describe_table give the tables.", query)
	addTool(s, "helios_describe_schema", "Every table in Helios School's community data model with what it holds and its column names, for writing a helios_query. helios_describe_table gives a table's columns in full.", describeSchema)
	addTool(s, "helios_describe_table", "One table of Helios School's community data model in full: each column with its kind, what it holds, the table a reference points at and every enum value with its meaning, and the columns elsewhere that point at this table.", describeTable)
	addTool(s, "helios_policies", "The policies over Helios School's community data: the definitions a helios_query may call by name (visible, manages, household, admin_of and the rest) and every clause deciding who reads and writes what.", policies)
}

func describeSchema(c call, _ none) (any, error) {
	out := &strings.Builder{}
	for _, t := range db.Tables {
		fmt.Fprintf(out, "%s: %s", t.Name, t.Description)
		if t.Generated {
			out.WriteString(" Worked out from other tables.")
		}
		names := []string{}
		for _, col := range t.Columns {
			if !col.Private {
				names = append(names, col.Name)
			}
		}
		fmt.Fprintf(out, "\n  columns: %s\n\n", strings.Join(names, ", "))
	}
	return out.String(), nil
}

func describeTable(c call, in tableIn) (any, error) {
	t, ok := db.Lookup(strings.ToUpper(strings.TrimSpace(in.Table)))
	if !ok {
		return nil, fmt.Errorf("no table %q; describe_schema lists them", in.Table)
	}
	out := db.DescribeTable(*t)
	referrers := []string{}
	for _, other := range db.Tables {
		for _, col := range other.Columns {
			if col.Kind == db.Ref && col.Target == t.Name && !col.Private {
				referrers = append(referrers, other.Name+"."+col.Name)
			}
		}
	}
	if len(referrers) > 0 {
		out += "Pointed at by: " + strings.Join(referrers, ", ") + "\n"
	}
	return out, nil
}

func policies(c call, _ none) (any, error) {
	return db.PolicySource, nil
}

func query(c call, in queryIn) (any, error) {
	q, err := db.Parse(in.Query)
	if err != nil {
		return nil, err
	}
	return c.shape(c.m.Run(c.ctx, q, c.env), q, 0), nil
}

func whoami(c call, _ none) (any, error) {
	out := map[string]any{}
	for _, part := range []struct {
		name string
		q    tree
	}{
		{"person", tree{"from": "PERSON", "where": []any{eq("id", path("@viewer"))}}},
		{"addresses", tree{"from": "PERSON_EMAIL", "where": []any{eq("person", path("@viewer"))}}},
		{"memberships", tree{"from": "MEMBER", "where": []any{eq("person", path("@viewer"))}, "include": []any{"group"}}},
		{"manages", tree{"from": "GROUP", "as": "g", "where": []any{tree{"manages": []any{path("@g")}}}, "order": []any{tree{"path": "name", "dir": "asc"}}, "limit": listRows}},
		{"admin_of", tree{"from": "APP", "where": []any{tree{"admin_of": []any{path("key")}}}}},
	} {
		res, q, err := c.run(part.q)
		if err != nil {
			return nil, err
		}
		out[part.name] = c.shape(res, q, 0)
	}
	return out, nil
}

type hit struct {
	ID      string `json:"id"`
	Table   string `json:"table"`
	Name    string `json:"name,omitempty"`
	Href    string `json:"href,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type named struct {
	Name string `json:"name"`
	Href string `json:"href,omitempty"`
}

func (c call) named(table string, row store.Row) named {
	return named{Name: title(row), Href: db.Link(table, row, c.s.origin)}
}

func (c call) names(ids []string) (map[string]named, error) {
	byTable := map[string][]string{}
	for _, id := range ids {
		table, ok := db.TableOf(id)
		if ok {
			byTable[table] = append(byTable[table], id)
		}
	}
	out := map[string]named{}
	for table, ids := range byTable {
		res, _, err := c.run(tree{"from": table, "where": []any{among("id", ids)}})
		if err != nil {
			return nil, err
		}
		for _, row := range res.Rows() {
			out[row["id"]] = c.named(table, row)
		}
	}
	return out, nil
}

func (c call) hits(found []db.SearchHit) ([]hit, error) {
	ids := []string{}
	for _, h := range found {
		ids = append(ids, h.ID)
	}
	names, err := c.names(ids)
	if err != nil {
		return nil, err
	}
	out := []hit{}
	for _, h := range found {
		table, _ := db.TableOf(h.ID)
		out = append(out, hit{ID: h.ID, Table: table, Name: names[h.ID].Name, Href: names[h.ID].Href, Summary: h.Summary})
	}
	return out, nil
}

func search(c call, in wordsIn) (any, error) {
	words := strings.TrimSpace(in.Words)
	if words == "" {
		return nil, errors.New("words is required")
	}
	out := map[string]any{}
	byWords, err := c.hits(c.s.deps.Search.Words(c.m, c.env, words))
	if err != nil {
		return nil, err
	}
	out["words"] = byWords
	meaning, err := c.s.deps.Search.Meaning(c.ctx, c.m, c.env, words)
	if err != nil {
		slog.ErrorContext(c.ctx, "mcp: search by meaning", "error", err)
		out["meaningError"] = "the search by meaning failed"
		return out, nil
	}
	byMeaning, err := c.hits(meaning)
	if err != nil {
		return nil, err
	}
	out["meaning"] = byMeaning
	return out, nil
}

type pointers struct {
	Count int         `json:"count"`
	Rows  []store.Row `json:"rows"`
	More  int         `json:"more,omitempty"`
}

func get(c call, in idIn) (any, error) {
	t, err := tableOf(in.ID, "")
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(in.ID)
	refs := refColumns(t)
	q := tree{"from": t.Name, "where": []any{eq("id", id)}}
	if len(refs) > 0 {
		include := []any{}
		for _, r := range refs {
			include = append(include, r)
		}
		q["include"] = include
	}
	res, _, err := c.run(q)
	if err != nil {
		return nil, err
	}
	if len(res.IDs) == 0 {
		return nil, fmt.Errorf("no %s %s that you can see", t.Name, id)
	}
	row := c.compact(t.Name, res.Resources[t.Name][id])
	names := map[string]named{}
	for _, name := range refs {
		col, _ := t.Column(name)
		if target, ok := res.Resources[col.Target][row[name]]; ok {
			names[row[name]] = c.named(col.Target, target)
		}
	}
	pointed := map[string]pointers{}
	for _, other := range db.Tables {
		for _, col := range other.Columns {
			if col.Kind != db.Ref || col.Target != t.Name || col.Private {
				continue
			}
			found, _, err := c.run(tree{"from": other.Name, "where": []any{eq(col.Name, id)}})
			if err != nil {
				return nil, err
			}
			if len(found.IDs) == 0 {
				continue
			}
			p := pointers{Count: len(found.IDs), Rows: []store.Row{}}
			for i, r := range found.Rows() {
				if i == referrerRows {
					p.More = len(found.IDs) - referrerRows
					break
				}
				p.Rows = append(p.Rows, c.compact(other.Name, r))
			}
			pointed[other.Name+"."+col.Name] = p
		}
	}
	return map[string]any{"table": t.Name, "row": row, "names": names, "pointedAtBy": pointed}, nil
}

type person struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Href    string `json:"href,omitempty"`
	Reasons string `json:"reasons,omitempty"`
	Via     string `json:"via,omitempty"`
}

func (c call) person(id string, row store.Row) person {
	if row == nil {
		return person{ID: id, Name: id}
	}
	n := c.named("PERSON", row)
	return person{ID: id, Name: n.Name, Href: n.Href}
}

func (c call) row(table, id string) (store.Row, bool, error) {
	res, _, err := c.run(tree{"from": table, "where": []any{eq("id", id)}})
	if err != nil || len(res.IDs) == 0 {
		return nil, false, err
	}
	return res.Rows()[0], true, nil
}

func group(c call, in idIn) (any, error) {
	if _, err := tableOf(in.ID, "GROUP"); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(in.ID)
	self, ok, err := c.row("GROUP", id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("no group %s that you can see", id)
	}
	managing := []string{}
	for cur, depth := self, 0; depth < maxAncestors; depth++ {
		if cur["managed_by"] != "" && !slices.Contains(managing, cur["managed_by"]) {
			managing = append(managing, cur["managed_by"])
		}
		if cur["parent"] == "" {
			break
		}
		next, ok, err := c.row("GROUP", cur["parent"])
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		cur = next
	}
	managers := []person{}
	if len(managing) > 0 {
		res, _, err := c.run(tree{"from": "EFFECTIVE_MEMBER", "where": []any{among("group", managing)}, "include": []any{"person", "group"}})
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for _, m := range res.Rows() {
			if seen[m["person"]] {
				continue
			}
			seen[m["person"]] = true
			p := c.person(m["person"], res.Resources["PERSON"][m["person"]])
			p.Via = title(res.Resources["GROUP"][m["group"]])
			managers = append(managers, p)
		}
	}
	res, _, err := c.run(tree{"from": "EFFECTIVE_MEMBER", "where": []any{eq("group", id)}, "include": []any{"person"}})
	if err != nil {
		return nil, err
	}
	members := []person{}
	for i, m := range res.Rows() {
		if i == listRows {
			break
		}
		p := c.person(m["person"], res.Resources["PERSON"][m["person"]])
		p.Reasons = m["reasons"]
		members = append(members, p)
	}
	rules, rulesQuery, err := c.run(tree{"from": "RULE", "where": []any{eq("group", id)}, "order": []any{tree{"path": "order", "dir": "asc"}}, "include": []any{"target", "person", "within"}})
	if err != nil {
		return nil, err
	}
	under, underQuery, err := c.run(tree{"from": "GROUP", "where": []any{eq("parent", id)}, "order": []any{tree{"path": "order", "dir": "asc"}}, "limit": listRows})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"group":        c.compact("GROUP", self),
		"managers":     managers,
		"memberCount":  len(res.IDs),
		"members":      members,
		"rules":        c.shape(rules, rulesQuery, 0),
		"under":        c.shape(under, underQuery, 0),
		"membersShown": len(members),
	}, nil
}

func readDocument(c call, in idIn) (any, error) {
	if _, err := tableOf(in.ID, "DOCUMENT"); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(in.ID)
	root, ok, err := c.row("DOCUMENT", id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("no document %s that you can see", id)
	}
	type node struct {
		row   store.Row
		depth int
	}
	nodes := []node{{row: root}}
	frontier := []string{id}
	for depth := 1; len(frontier) > 0 && len(nodes) < documentNodes; depth++ {
		res, _, err := c.run(tree{"from": "DOCUMENT", "where": []any{among("parent", frontier)}, "order": []any{tree{"path": "order", "dir": "asc"}}})
		if err != nil {
			return nil, err
		}
		frontier = []string{}
		for _, r := range res.Rows() {
			nodes = append(nodes, node{row: r, depth: depth})
			frontier = append(frontier, r["id"])
		}
	}
	contents := []string{}
	for _, n := range nodes {
		if n.row["content"] != "" {
			contents = append(contents, n.row["content"])
		}
	}
	mimes := map[string]string{}
	if len(contents) > 0 {
		res, _, err := c.run(tree{"from": "CONTENT", "where": []any{among("id", contents)}})
		if err != nil {
			return nil, err
		}
		for _, r := range res.Rows() {
			mimes[r["id"]] = r["mime"]
		}
	}
	outline := &strings.Builder{}
	texts := &strings.Builder{}
	skipped := 0
	for _, n := range nodes {
		r := n.row
		what := r["kind"]
		if what == "" {
			what = r["relation"]
		}
		fmt.Fprintf(outline, "%s- %s %s", strings.Repeat("  ", n.depth), r["id"], what)
		for _, col := range []string{"name", "published", "filename", "url", "fetch"} {
			if r[col] != "" {
				fmt.Fprintf(outline, " | %s: %s", col, r[col])
			}
		}
		if href := db.Link("DOCUMENT", r, c.s.origin); href != "" && href != r["url"] {
			fmt.Fprintf(outline, " | href: %s", href)
		}
		mime := mimes[r["content"]]
		if mime != "" {
			fmt.Fprintf(outline, " | %s", mime)
		}
		outline.WriteString("\n")
		if mime != "text/markdown" {
			continue
		}
		name, _, ok := c.m.BlobCell(nil, c.env, r["content"], "blob")
		if !ok {
			continue
		}
		body, _, err := c.s.deps.Bucket.Get(c.ctx, name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", r["content"], err)
		}
		if texts.Len()+len(body) > maxOutput/2 {
			skipped++
			continue
		}
		fmt.Fprintf(texts, "\n## %s (%s)\n\n%s\n", what, r["id"], body)
	}
	out := "# " + title(root) + "\n\n"
	if href := db.Link("DOCUMENT", root, c.s.origin); href != "" {
		out += href + "\n\n"
	}
	out += outline.String()
	if len(nodes) >= documentNodes {
		out += fmt.Sprintf("\n(the tree is cut at %d documents)\n", documentNodes)
	}
	out += texts.String()
	if skipped > 0 {
		out += fmt.Sprintf("\n(%d more extracts left out for length; helios_read_document on an extract's ID reads it)\n", skipped)
	}
	return out, nil
}

var roles = map[string]string{"student": "students", "parent": "parents", "staff": "staff"}

func findPeople(c call, in peopleIn) (any, error) {
	where := []any{}
	if words := strings.TrimSpace(in.Words); words != "" {
		ids := []string{}
		for _, h := range c.s.deps.Search.Words(c.m, c.env, words) {
			if table, _ := db.TableOf(h.ID); table == "PERSON" {
				ids = append(ids, h.ID)
			}
		}
		if len(ids) == 0 {
			return rows{Rows: []store.Row{}}, nil
		}
		where = append(where, among("id", ids))
	}
	if in.Role != "" {
		slug, ok := roles[strings.ToLower(strings.TrimSpace(in.Role))]
		if !ok {
			return nil, fmt.Errorf("role is student, parent or staff, not %q", in.Role)
		}
		where = append(where, tree{"exists": tree{"from": "EFFECTIVE_MEMBER", "where": []any{eq("person", path("@p")), eq("group.slug", slug)}}})
	}
	if in.Grade != "" {
		where = append(where, eq("grade", strings.TrimSpace(in.Grade)))
	}
	if in.Classroom != "" {
		where = append(where, eq("classroom.name", strings.TrimSpace(in.Classroom)))
	}
	if len(where) == 0 {
		return nil, errors.New("name at least one of words, role, grade and classroom")
	}
	res, q, err := c.run(tree{"from": "PERSON", "as": "p", "where": where, "order": []any{tree{"path": "name_sort", "dir": "asc"}}, "limit": listRows, "include": []any{"classroom", "crew", "department"}})
	if err != nil {
		return nil, err
	}
	return c.shape(res, q, 0), nil
}

func events(c call, in eventsIn) (any, error) {
	from := c.env.Now
	if in.From != "" {
		day, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(in.From), c.env.Now.Location())
		if err != nil {
			return nil, fmt.Errorf("from is YYYY-MM-DD, not %q", in.From)
		}
		from = day
	}
	to := from.AddDate(0, 0, eventsDays)
	if in.To != "" {
		day, err := time.ParseInLocation(time.DateOnly, strings.TrimSpace(in.To), c.env.Now.Location())
		if err != nil {
			return nil, fmt.Errorf("to is YYYY-MM-DD, not %q", in.To)
		}
		to = day
	}
	res, q, err := c.run(tree{
		"from": "GROUP",
		"where": []any{
			eq("kind", "event"),
			tree{">=": []any{path("start"), from.Format(time.DateOnly)}},
			tree{"<": []any{path("start"), to.AddDate(0, 0, 1).Format(time.DateOnly)}},
		},
		"order":   []any{tree{"path": "start", "dir": "asc"}},
		"limit":   listRows,
		"include": []any{"parent"},
	})
	if err != nil {
		return nil, err
	}
	return c.shape(res, q, 0), nil
}
