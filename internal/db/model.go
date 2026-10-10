package db

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"heliosian/internal/cells"
	"heliosian/internal/store"
	"heliosian/internal/trace"
)

func tallied(ctx context.Context, name string) func() {
	start := time.Now()
	return func() { trace.From(ctx).Tally(name).Add(time.Since(start)) }
}

type Rows struct {
	table    *Table
	rows     []store.Row
	byID     map[string]int
	byUnique map[string]int
	by       map[string]map[string][]int
}

type Sheet map[string]*Rows

type Model struct {
	People    Sheet
	Groups    Sheet
	Documents Sheet
	Mail      Sheet
	Config    Sheet
	derived   *derived
	consented map[string]*View
	index     *SearchIndex
}

type View struct {
	rows  *Rows
	shown []store.Row
	all   []store.Row
}

func wholeView(rows *Rows) *View {
	return &View{rows: rows, shown: rows.rows, all: rows.rows}
}

func (m *Model) Shown(name string) *View {
	return m.consented[name]
}

func (m *Model) view(name string, whole bool) *View {
	if whole {
		return wholeView(m.Table(name))
	}
	return m.Shown(name)
}

func (v *View) Table() *Table {
	return v.rows.table
}

func (v *View) Len() int {
	return len(v.all)
}

func (v *View) All() []store.Row {
	return v.all
}

func (v *View) Get(id string) (store.Row, bool) {
	i, ok := v.rows.byID[id]
	if !ok || v.shown[i] == nil {
		return nil, false
	}
	return v.shown[i], true
}

func (v *View) Find(values ...string) (store.Row, bool) {
	if _, ok := v.rows.Find(values...); !ok {
		return nil, false
	}
	i := v.rows.byUnique[strings.Join(values, "\x00")]
	if v.shown[i] == nil {
		return nil, false
	}
	return v.shown[i], true
}

func (v *View) Referencing(column, id string) []store.Row {
	out := []store.Row{}
	for _, i := range v.rows.by[column][id] {
		if v.shown[i] != nil {
			out = append(out, v.shown[i])
		}
	}
	return out
}

func (m *Model) slot(sheet string) *Sheet {
	switch sheet {
	case PeopleSheet:
		return &m.People
	case GroupsSheet:
		return &m.Groups
	case DocumentsSheet:
		return &m.Documents
	case MailSheet:
		return &m.Mail
	case ConfigSheet:
		return &m.Config
	}
	panic("db: no sheet " + sheet)
}

func (m *Model) Table(name string) *Rows {
	t, ok := Lookup(name)
	if !ok || t.Generated {
		panic("db: no stored table " + name)
	}
	return (*m.slot(t.Sheet))[name]
}

func (m *Model) Has(id string) bool {
	table, ok := TableOf(id)
	if !ok {
		return false
	}
	if t, _ := Lookup(table); t.Generated {
		return false
	}
	_, ok = m.Table(table).Get(id)
	return ok
}

func (r *Rows) Len() int {
	return len(r.rows)
}

func (r *Rows) All() []store.Row {
	return r.rows
}

func (r *Rows) Get(id string) (store.Row, bool) {
	i, ok := r.byID[id]
	if !ok {
		return nil, false
	}
	return r.rows[i], true
}

func (r *Rows) Find(values ...string) (store.Row, bool) {
	if len(values) != len(r.table.Unique) {
		panic(fmt.Sprintf("db: %s is unique by %v", r.table.Name, r.table.Unique))
	}
	i, ok := r.byUnique[strings.Join(values, "\x00")]
	if !ok {
		return nil, false
	}
	return r.rows[i], true
}

func (r *Rows) Referencing(column, id string) []store.Row {
	out := []store.Row{}
	for _, i := range r.by[column][id] {
		out = append(out, r.rows[i])
	}
	return out
}

func uniqueOf(t *Table, row store.Row) string {
	parts := []string{}
	for _, k := range t.Unique {
		parts = append(parts, row[k])
	}
	return strings.Join(parts, "\x00")
}

func buildRows(ctx context.Context, t *Table, raw []store.Row) (*Rows, error) {
	table := trace.From(ctx).Tally(t.Name)
	start := time.Now()
	rows := []store.Row{}
	for _, row := range raw {
		if err := t.Check(row); err != nil {
			return nil, err
		}
		if t.Generate != nil {
			row = maps.Clone(row)
		}
		rows = append(rows, row)
	}
	if t.Generate != nil {
		t.Generate(rows)
	}
	table.Tally("check").Add(time.Since(start))
	start = time.Now()
	out, err := indexRows(t, rows)
	table.Tally("index").Add(time.Since(start))
	table.Set("rows", len(rows))
	return out, err
}

func indexRows(t *Table, rows []store.Row) (*Rows, error) {
	out := &Rows{table: t, byID: map[string]int{}, byUnique: map[string]int{}, by: map[string]map[string][]int{}}
	distinct := map[string]map[string]bool{}
	for _, k := range t.Distinct {
		distinct[k] = map[string]bool{}
	}
	for _, row := range rows {
		if _, dup := out.byID[row["id"]]; dup {
			return nil, fmt.Errorf("%s: two rows have the id %s", t.Name, row["id"])
		}
		out.byID[row["id"]] = len(out.rows)
		if len(t.Unique) > 0 {
			unique := uniqueOf(t, row)
			if _, dup := out.byUnique[unique]; dup {
				return nil, fmt.Errorf("%s: two rows have the same %s", t.Name, describeUnique(t, row))
			}
			out.byUnique[unique] = len(out.rows)
		}
		for _, k := range t.Distinct {
			value := strings.ToLower(row[k])
			if value == "" {
				continue
			}
			if distinct[k][value] {
				return nil, fmt.Errorf("%s: two rows have the %s %q", t.Name, k, row[k])
			}
			distinct[k][value] = true
		}
		for _, c := range t.Columns {
			if (c.Kind != Ref && c.Kind != Enum) || row[c.Name] == "" {
				continue
			}
			if out.by[c.Name] == nil {
				out.by[c.Name] = map[string][]int{}
			}
			key := indexKey(c, row[c.Name])
			out.by[c.Name][key] = append(out.by[c.Name][key], len(out.rows))
		}
		out.rows = append(out.rows, row)
	}
	return out, nil
}

func references(c Column, cell string) []string {
	if c.Kind != Ref || cell == "" {
		return nil
	}
	return []string{cell}
}

func indexKey(c Column, value string) string {
	if c.Kind == Enum {
		return strings.ToLower(strings.TrimSpace(value))
	}
	return value
}

func describeUnique(t *Table, row store.Row) string {
	parts := []string{}
	for _, k := range t.Unique {
		parts = append(parts, k+"="+row[k])
	}
	return strings.Join(parts, "; ")
}

func eachRow(f func(row map[string]string)) func(rows []store.Row) {
	return func(rows []store.Row) {
		for _, row := range rows {
			f(row)
		}
	}
}

func documentGenerated(rows []store.Row) {
	byID := map[string]store.Row{}
	for _, row := range rows {
		byID[row["id"]] = row
	}
	kinded := map[string]string{}
	var nearest func(id string, depth int) string
	nearest = func(id string, depth int) string {
		if found, ok := kinded[id]; ok {
			return found
		}
		row, ok := byID[id]
		if !ok || depth > len(rows) {
			return ""
		}
		found := id
		if row["kind"] == "" {
			found = nearest(row["parent"], depth+1)
		}
		kinded[id] = found
		return found
	}
	for _, row := range rows {
		if row["kind"] == "" {
			row["terminal"] = nearest(row["parent"], 0)
		}
		if row["relation"] != "extract" && row["relation"] != "pages" {
			continue
		}
		source := row["parent"]
		for steps := 0; steps < len(rows); steps++ {
			parent, ok := byID[source]
			if !ok || parent["relation"] != "pages" {
				break
			}
			source = parent["parent"]
		}
		row["source"] = source
	}
}

func personGenerated(row map[string]string) {
	for _, name := range []string{"name_long", "name_short", "name_sort", "grade", "classroom", "crew", "department", "job_title"} {
		row[name] = overrideOr(row, name)
	}
	row["phone"] = consented(row, "phone")
	switch {
	case row["name_long"] != "":
		row["name_show"] = row["name_long"]
	case row["source"] == "guest":
		row["name_show"] = "Guest"
	default:
		row["name_show"] = "[Missing Name]"
	}
}

func groupGenerated(row map[string]string) {
	if !strings.EqualFold(row["kind"], "family") {
		row["address"] = overrideOr(row, "address")
		row["phone"] = overrideOr(row, "phone")
		return
	}
	row["address"] = consented(row, "address")
	row["phone"] = consented(row, "phone")
}

func emailGenerated(row map[string]string) {
	row["guest"] = cells.YesNoCell(row["source"] == "guest")
}

func photoGenerated(row map[string]string) {
	row["image"] = row["crop"]
	if row["image"] == "" {
		row["image"] = row["reencode"]
	}
}

func consented(row map[string]string, name string) string {
	if !strings.EqualFold(row[name+"_consent"], "shared") {
		return ""
	}
	return overrideOr(row, name)
}

func overrideOr(row map[string]string, name string) string {
	if v := row[name+"_override"]; v != "" {
		return v
	}
	return row["vc_"+name]
}

func checkEmails(people Sheet) error {
	primaries := map[string]int{}
	for _, row := range people["PERSON_EMAIL"].rows {
		if row["address"] != strings.ToLower(strings.TrimSpace(row["address"])) {
			return fmt.Errorf("PERSON_EMAIL: %q is not written in lower case", row["address"])
		}
		if strings.Contains(row["address"], ".noemail") {
			return fmt.Errorf("PERSON_EMAIL: %q is a placeholder for no address", row["address"])
		}
		primary, err := cells.YesNo(row["primary"], false)
		if err != nil {
			return err
		}
		if _, seen := primaries[row["person"]]; !seen {
			primaries[row["person"]] = 0
		}
		if primary {
			primaries[row["person"]]++
		}
	}
	for _, person := range slices.Sorted(maps.Keys(primaries)) {
		if n := primaries[person]; n != 1 {
			return fmt.Errorf("PERSON_EMAIL: %s has %d primary addresses, not one", person, n)
		}
	}
	return nil
}

func checkPhotos(people Sheet) error {
	for _, row := range people["PHOTO"].rows {
		if (row["person"] == "") == (row["group"] == "") {
			return fmt.Errorf("PHOTO %s: a photo is of a person or of a group, not both or neither", row["id"])
		}
	}
	return nil
}

func checkDocuments(documents Sheet) error {
	parents := map[string]string{}
	kinds := map[string]string{}
	for _, row := range documents["DOCUMENT"].rows {
		parents[row["id"]], kinds[row["id"]] = row["parent"], row["kind"]
	}
	slugs := map[[2]string]string{}
	for _, row := range documents["DOCUMENT"].rows {
		if row["relation"] == "side" && kinds[row["parent"]] != "wiki" {
			return fmt.Errorf("DOCUMENT %s: a side card sits under a wiki page", row["id"])
		}
		if err := checkWikiSlug(row["slug"], row["kind"]); err != nil {
			return fmt.Errorf("DOCUMENT %s: %v", row["id"], err)
		}
		if other, taken := slugs[[2]string{row["parent"], row["slug"]}]; taken && row["slug"] != "" {
			return fmt.Errorf("DOCUMENT %s: %s, under the same page, already has the slug %q", row["id"], other, row["slug"])
		}
		slugs[[2]string{row["parent"], row["slug"]}] = row["id"]
		if row["hidden"] == "Yes" && row["kind"] != "wiki" {
			return fmt.Errorf("DOCUMENT %s: only a wiki page is hidden", row["id"])
		}
		root := row["kind"] != "" && row["relation"] == "" && row["parent"] == ""
		child := row["kind"] == "" && row["relation"] != "" && row["parent"] != ""
		subpage := row["kind"] == "wiki" && row["relation"] == "" && kinds[row["parent"]] == "wiki"
		if !root && !child && !subpage {
			return fmt.Errorf("DOCUMENT %s: a document has a kind and no parent, a relation and a parent, or is a wiki page under another", row["id"])
		}
		seen := map[string]bool{}
		for at := row["id"]; at != ""; at = parents[at] {
			if seen[at] {
				return fmt.Errorf("DOCUMENT %s: its parents lead back round to %s", row["id"], at)
			}
			seen[at] = true
		}
	}
	return nil
}

func checkRuleProperties(groups Sheet) error {
	person, _ := Lookup("PERSON")
	for _, row := range groups["RULE"].rows {
		if property := row["property"]; property != "" {
			if _, ok := person.Column(property); !ok {
				return fmt.Errorf("RULE %s: property %s is no PERSON column", row["id"], property)
			}
			if c, _ := person.Column(property); c.Private {
				return fmt.Errorf("RULE %s: property %s is private", row["id"], property)
			}
			if !policies.open["PERSON."+property] {
				return fmt.Errorf("RULE %s: property %s is not open to everyone who sees the person", row["id"], property)
			}
		}
	}
	return nil
}

func (m *Model) checkReferences() error {
	for _, t := range Tables {
		if t.Generated {
			continue
		}
		for _, row := range m.Table(t.Name).rows {
			for _, c := range t.Columns {
				for _, value := range references(c, row[c.Name]) {
					if !m.Has(value) {
						return fmt.Errorf("%s %s: %s %s names no row", t.Name, row["id"], c.Name, value)
					}
				}
			}
		}
	}
	return nil
}
