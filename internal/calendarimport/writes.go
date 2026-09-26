package calendarimport

import (
	"heliosian/internal/access"
	"heliosian/internal/store"
)

var importer = access.System("calendarimport")

func syncOps(actor access.Actor, s tabSync) (ops []store.Op, added, changed, removed int) {
	had := map[string]map[string]string{}
	for _, row := range s.before {
		had[row[s.keyCol]] = row
	}
	want := map[string]bool{}
	for _, row := range s.rows {
		key := row[s.keyCol]
		want[key] = true
		old, ok := had[key]
		if !ok {
			cells := store.Row{}
			for _, column := range s.header {
				if row[column] != "" {
					cells[column] = row[column]
				}
			}
			ops = append(ops, store.Insert(s.tab, cells))
			added++
			continue
		}
		cells := store.Row{}
		for _, column := range s.header {
			if column != s.keyCol && row[column] != old[column] {
				cells[column] = row[column]
			}
		}
		if len(cells) > 0 {
			ops = append(ops, store.Update(s.tab, store.Row{s.keyCol: key}, cells))
			changed++
		}
	}
	if !s.mirror {
		return ops, added, changed, removed
	}
	for _, row := range s.before {
		if key := row[s.keyCol]; !want[key] {
			want[key] = true
			ops = append(ops, store.Delete(s.tab, store.Row{s.keyCol: key}))
			removed++
		}
	}
	return ops, added, changed, removed
}
