package testkit

type Insert struct {
	Table, As string
	Row       map[string]any
}

func SchoolDay() []Insert {
	open := func(as string, row map[string]any) Insert {
		row["status"], row["visible_to"], row["members_visible_to"] = "open", "grp00000000004", "grp00000000004"
		row["posting"], row["replying"] = "members", "members"
		return Insert{Table: "GROUP", As: as, Row: row}
	}
	return []Insert{
		open("schedule", map[string]any{"kind": "category", "name": "Schedule"}),
		open("regular", map[string]any{"kind": "category", "parent": "@schedule", "name": "Regular", "description": "A full school day with aftercare."}),
		open("day", map[string]any{"kind": "day", "parent": "@regular", "name": "Regular", "start": "2026-09-28", "all_day": "Yes"}),
		open("school", map[string]any{"kind": "day_part", "parent": "@day", "name": "School", "start": "2026-09-28 08:30", "end": "2026-09-28 15:00"}),
	}
}
