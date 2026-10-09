package testkit

import "heliosian/internal/store"

func SchoolDay() []store.Op {
	open := func(row store.Row) store.Op {
		row["status"], row["visible_to"], row["members_visible_to"] = "open", "grp00000000004", "grp00000000004"
		return store.Insert("GROUP", row)
	}
	return []store.Op{
		open(store.Row{"id": "grp00000000504", "kind": "category", "name": "Schedule"}),
		open(store.Row{"id": "grp00000000505", "kind": "category", "parent": "grp00000000504", "name": "Regular", "description": "A full school day with aftercare."}),
		open(store.Row{"id": "grp00000000506", "kind": "day", "parent": "grp00000000505", "name": "Regular", "start": "2026-09-28", "all_day": "Yes"}),
		open(store.Row{"id": "grp00000000507", "kind": "day_part", "parent": "grp00000000506", "name": "School", "start": "2026-09-28 08:30", "end": "2026-09-28 15:00"}),
	}
}
