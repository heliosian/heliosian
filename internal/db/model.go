package db

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"heliosian/internal/cells"
	"heliosian/internal/store"
)

type Rows struct {
	table *Table
	rows  []store.Row
	byKey map[string]int
	refs  map[string]map[string][]int
}

type Sheet map[string]*Rows

type Model struct {
	People    Sheet
	Groups    Sheet
	Documents Sheet
	Mail      Sheet
	Config    Sheet
	derived   *derived
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
	_, ok = m.Table(table).Get(id)
	return ok
}

func (r *Rows) Len() int {
	return len(r.rows)
}

func (r *Rows) All() []store.Row {
	return r.rows
}

func (r *Rows) Get(key ...string) (store.Row, bool) {
	i, ok := r.byKey[strings.Join(key, "\x00")]
	if !ok {
		return nil, false
	}
	return r.rows[i], true
}

func (r *Rows) Referencing(column, id string) []store.Row {
	out := []store.Row{}
	for _, i := range r.refs[column][id] {
		out = append(out, r.rows[i])
	}
	return out
}

func keyOf(t *Table, row store.Row) string {
	parts := []string{}
	for _, k := range t.Key {
		parts = append(parts, row[k])
	}
	return strings.Join(parts, "\x00")
}

func buildRows(t *Table, raw []store.Row) (*Rows, error) {
	out := &Rows{table: t, byKey: map[string]int{}, refs: map[string]map[string][]int{}}
	for _, row := range raw {
		if err := t.Check(row); err != nil {
			return nil, err
		}
		if t.Generate != nil {
			row = maps.Clone(row)
			t.Generate(row)
		}
		key := keyOf(t, row)
		if _, dup := out.byKey[key]; dup {
			return nil, fmt.Errorf("%s: two rows have the key %s", t.Name, describeKey(t, row))
		}
		out.byKey[key] = len(out.rows)
		for _, c := range t.Columns {
			for _, value := range references(c, row[c.Name]) {
				if out.refs[c.Name] == nil {
					out.refs[c.Name] = map[string][]int{}
				}
				out.refs[c.Name][value] = append(out.refs[c.Name][value], len(out.rows))
			}
		}
		out.rows = append(out.rows, row)
	}
	return out, nil
}

func references(c Column, cell string) []string {
	switch c.Kind {
	case Ref:
		if cell == "" {
			return nil
		}
		return []string{cell}
	case Refs:
		return cells.SplitList(cell)
	}
	return nil
}

func describeKey(t *Table, row store.Row) string {
	parts := []string{}
	for _, k := range t.Key {
		parts = append(parts, k+"="+row[k])
	}
	return strings.Join(parts, "; ")
}

func personNames(row map[string]string) {
	for _, name := range []string{"name_long", "name_short", "name_sort"} {
		row[name] = row[name+"_override"]
		if row[name] == "" {
			row[name] = row[name+"_import"]
		}
	}
	switch {
	case row["name_long"] != "":
		row["name_show"] = row["name_long"]
	case row["source"] == "guest":
		row["name_show"] = "Guest"
	default:
		row["name_show"] = "[Missing Name]"
	}
}

func checkPrimaryEmails(people Sheet) error {
	primaries := map[string]int{}
	for _, row := range people["PERSON_EMAIL"].rows {
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

func checkRuleProperties(groups Sheet) error {
	person, _ := Lookup("PERSON")
	for _, row := range groups["RULE"].rows {
		if property := row["property"]; property != "" {
			if _, ok := person.Column(property); !ok {
				return fmt.Errorf("RULE %s: property %s is no PERSON column", describeKey(groups["RULE"].table, row), property)
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
						return fmt.Errorf("%s %s: %s %s names no row", t.Name, describeKey(&t, row), c.Name, value)
					}
				}
			}
		}
	}
	return nil
}
