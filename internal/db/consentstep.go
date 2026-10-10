package db

import (
	"context"
	"maps"
	"slices"
	"time"

	"heliosian/internal/store"
	"heliosian/internal/trace"
)

func consentStep(ctx context.Context, m *Model) error {
	hiding := tallied(ctx, "hidden")
	hidden := map[string]bool{}
	r := m.bareRun(true, time.Now())
	for _, t := range Tables {
		show, ok := policies.show[t.Name]
		if !ok {
			continue
		}
		for _, row := range m.Table(t.Name).All() {
			if !show.eval(&frame{table: &t, row: row, name: "row", run: r}) {
				hidden[row["id"]] = true
			}
		}
	}
	passes := 0
	for grew := true; grew; {
		grew = false
		passes++
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
	hiding()
	trace.From(ctx).Set("passes", passes)
	trace.From(ctx).Set("hidden", len(hidden))
	viewing := tallied(ctx, "views")
	defer viewing()
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
