package db

import (
	"maps"
	"slices"
	"strings"

	"heliosian/internal/store"
)

// The consent step decides which rows may leave to anyone but the import, and nothing else.
// It runs once per build, after every part; docs/datamodel.md, Consent step, says exactly what it hides.
func consentStep(m *Model) error {
	hidden := map[string]bool{}
	for _, row := range m.Table("PERSON").All() {
		if row["source"] != "guest" && !strings.EqualFold(row["consent"], "listed") {
			hidden[row["id"]] = true
		}
	}
	for _, row := range m.Table("GROUP").All() {
		if strings.EqualFold(row["kind"], "family") && !strings.EqualFold(row["consent"], "listed") {
			hidden[row["id"]] = true
		}
	}
	for grew := true; grew; {
		grew = false
		for _, t := range Tables {
			if t.Generated {
				continue
			}
			for _, row := range m.Table(t.Name).All() {
				if !hidden[row["id"]] && namesHidden(&t, row, hidden) {
					hidden[row["id"]] = true
					grew = true
				}
			}
		}
	}
	consented := map[string]*View{}
	for _, t := range Tables {
		if t.Generated {
			continue
		}
		rows := m.Table(t.Name)
		v := &View{rows: rows, shown: make([]store.Row, len(rows.rows)), all: []store.Row{}}
		for i, row := range rows.rows {
			if hidden[row["id"]] {
				continue
			}
			v.shown[i] = withoutPrivate(&t, row)
			v.all = append(v.all, v.shown[i])
		}
		consented[t.Name] = v
	}
	m.consented = consented
	return nil
}

func namesHidden(t *Table, row store.Row, hidden map[string]bool) bool {
	for _, c := range t.Columns {
		for _, id := range references(c, row[c.Name]) {
			if hidden[id] {
				return true
			}
		}
	}
	return false
}

func withoutPrivate(t *Table, row store.Row) store.Row {
	if !slices.ContainsFunc(t.Columns, func(c Column) bool { return c.Private }) {
		return row
	}
	out := maps.Clone(row)
	for _, c := range t.Columns {
		if c.Private {
			delete(out, c.Name)
		}
	}
	return out
}
