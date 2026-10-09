package tools

import (
	"errors"
	"fmt"
	"strings"

	"heliosian/internal/db"
	"heliosian/internal/store"
)

type none struct{}

type queryIn struct {
	Query string `json:"query" jsonschema:"the query in the query language, one (from TABLE ...) expression"`
}

type tableIn struct {
	Table string `json:"table" jsonschema:"the table's name, upper case, as helios_describe_schema lists it"`
}

type idIn struct {
	ID string `json:"id" jsonschema:"the row's ID"`
}

type searchIn struct {
	Words  string         `json:"words" jsonschema:"what to look for, in words"`
	Limits map[string]int `json:"limits,omitempty" jsonschema:"how many results to answer for each of GROUP, PERSON and DOCUMENT, by table, 0 to leave a table out; 10 for any table not named"`
}

var search = define("helios_search", "Searching", "Search Helios School's community data - people, families, classrooms, events, volunteer activities, parties, email lists, and the newsletters, school mail, wiki pages and shared files - by name and by meaning. Use it first for any question about Helios, the school, a family, child, teacher or classroom there, or what the school has sent out, such as \"who teaches the Jays\", \"when is picture day\" or \"what did the newsletter say about the auction\". Answers a list for each table - GROUP, PERSON and DOCUMENT - best first: the rows whose name holds every word, then the rows whose text lies close in meaning, each with the score it ranked by; an empty list means nothing close enough. A group or person result is its ID, name, summary and href. A document result is a bundle of at most five refs, best first: the matching extracts of one email or file, with near-identical copies from other emails or files that had no result of their own; each ref gives the extract, the document it was read from, how that stands to its email or file (attached file, email body, image, linked page), its href, that email's or file's ID and name, and the extract's summary. helios_read_document opens any document, helios_get any other result, and helios_similar every copy. A newer document supersedes an older one. Every tool gives a record with a page on the Helios apps its href: link to it whenever you name the record, and never make up an address.", func(c *call, in searchIn) (any, error) {
	words := strings.TrimSpace(in.Words)
	if words == "" {
		return nil, errors.New("words is required")
	}
	limits, err := db.SearchLimits(in.Limits)
	if err != nil {
		return nil, err
	}
	out, err := c.deps.Search.Search(c.ctx, c.m, c.env, words, limits)
	if err != nil {
		return nil, err
	}
	for _, results := range out {
		for _, r := range results {
			if r.ID != "" && r.Href != "" {
				c.note(r.Href, r.ID)
			}
		}
	}
	return out, nil
})

var similar = define("helios_similar", "Finding copies", "Every near-identical copy of a Helios School document, by the ID of an email, a part or an extract that helios_search answered - the same newsletter, PDF or page attached to or sent in other emails - each copy an extract with the document it was read from, how that stands to its email or file, its href, that email's or file's ID and name, and its summary, for when the differences between copies matter: which email carried it, when, or a copy that changed slightly.", func(c *call, in idIn) (any, error) {
	return c.deps.Search.Similar(c.m, c.env, strings.TrimSpace(in.ID))
})

var describeSchema = define("helios_describe_schema", "Reading the data model", "Every table in Helios School's community data model with what it holds and its column names, for writing a helios_query. helios_describe_table gives a table's columns in full.", func(c *call, _ none) (any, error) {
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
})

var describeTable = define("helios_describe_table", "Reading the data model", "One table of Helios School's community data model in full: each column with its kind, what it holds, the table a reference points at and every enum value with its meaning, and the columns elsewhere that point at this table.", func(c *call, in tableIn) (any, error) {
	t, ok := db.Lookup(strings.ToUpper(strings.TrimSpace(in.Table)))
	if !ok {
		return nil, fmt.Errorf("no table %q; helios_describe_schema lists them", in.Table)
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
})

var policies = define("helios_policies", "Reading the policies", "The policies over Helios School's community data: the definitions a helios_query may call by name (visible, manages, household, admin_of and the rest) and every clause deciding who reads and writes what.", func(c *call, _ none) (any, error) {
	return db.PolicySource, nil
})

var query = define("helios_query", "Querying the data", "Run a query in Helios's query language over the school community's data, as the person connected. Answers the query's canonical form, how many rows matched, the rows with their filled columns, and every row the includes brought by table and ID. helios_describe_schema and helios_describe_table give the tables.", func(c *call, in queryIn) (any, error) {
	q, err := db.Parse(in.Query)
	if err != nil {
		return nil, err
	}
	return c.shape(c.m.Run(c.ctx, q, c.env), q), nil
})

type pointers struct {
	Count int         `json:"count"`
	Rows  []store.Row `json:"rows"`
	More  int         `json:"more,omitempty"`
}

var get = define("helios_get", "Looking something up", "One record of Helios School's community data by ID: its filled fields, the names of what it refers to, and the records elsewhere that point at it, the first few of each. A person also comes with each family they are in and its members, each marked student, parent or staff.", func(c *call, in idIn) (any, error) {
	t, err := tableOf(in.ID, "")
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(in.ID)
	refs := []string{}
	for _, col := range t.Columns {
		if col.Kind == db.Ref && col.Target != "" && !col.Private {
			refs = append(refs, col.Name)
		}
	}
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
	out := map[string]any{"table": t.Name, "row": row, "names": names, "pointedAtBy": pointed}
	if t.Name == "PERSON" {
		families, err := c.families(id)
		if err != nil {
			return nil, err
		}
		out["families"] = families
	}
	return out, nil
})

var readDocument = define("helios_read_document", "Reading a document", "A Helios School document by ID - a newsletter or other school email, a page or file it linked, a PDF's reading, a shared file or a Helios Wiki page - read whole: the tree of its parts, links, images and extracts, and the text of each.", func(c *call, in idIn) (any, error) {
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
		found, _, err := c.rows(tree{"from": "DOCUMENT", "where": []any{among("parent", frontier)}, "order": asc("order")})
		if err != nil {
			return nil, err
		}
		frontier = []string{}
		for _, r := range found {
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
		found, _, err := c.rows(tree{"from": "CONTENT", "where": []any{among("id", contents)}})
		if err != nil {
			return nil, err
		}
		for _, r := range found {
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
		if href := c.href("DOCUMENT", r); href != "" && href != r["url"] {
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
		body, _, err := c.deps.Bucket.Get(c.ctx, name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", r["content"], err)
		}
		if texts.Len()+len(body) > documentText {
			skipped++
			continue
		}
		fmt.Fprintf(texts, "\n## %s (%s)\n\n%s\n", what, r["id"], body)
	}
	out := "# " + title(root) + "\n\n"
	if href := c.href("DOCUMENT", root); href != "" {
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
})
