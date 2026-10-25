package db

import (
	"context"
	"maps"
	"slices"
	"sync"
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
	shown := len(hidden)
	refs := referrers()
	for queue := slices.Collect(maps.Keys(hidden)); len(queue) > 0; queue = queue[1:] {
		table, _ := TableOf(queue[0])
		for _, ref := range slices.Concat(refs[table], refs[""]) {
			for _, row := range m.Table(ref.table).Referencing(ref.column, queue[0]) {
				if !hidden[row["id"]] {
					hidden[row["id"]] = true
					queue = append(queue, row["id"])
				}
			}
		}
	}
	hiding()
	trace.From(ctx).Set("propagated", len(hidden)-shown)
	trace.From(ctx).Set("hidden", len(hidden))
	viewing := tallied(ctx, "views")
	defer viewing()
	flipped := map[string]bool{}
	for _, pair := range [][2]map[string]bool{{hidden, m.hidden}, {m.hidden, hidden}} {
		for id := range pair[0] {
			if !pair[1][id] {
				table, _ := TableOf(id)
				flipped[table] = true
			}
		}
	}
	consented := map[string]*View{}
	for _, t := range Tables {
		if t.Generated {
			continue
		}
		rows := m.Table(t.Name)
		if was := m.consented[t.Name]; was != nil && was.rows == rows && !flipped[t.Name] {
			consented[t.Name] = was
			continue
		}
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
	m.derived = m.derived.carry(m.consented, consented)
	m.consented, m.hidden = consented, hidden
	return nil
}

type reference struct {
	table, column string
}

var referrers = sync.OnceValue(func() map[string][]reference {
	out := map[string][]reference{}
	for _, t := range Tables {
		if t.Generated {
			continue
		}
		for _, c := range t.Columns {
			if c.Kind == Ref {
				out[c.Target] = append(out[c.Target], reference{table: t.Name, column: c.Name})
			}
		}
	}
	return out
})

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
