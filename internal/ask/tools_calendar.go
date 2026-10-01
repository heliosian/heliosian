package ask

import (
	"encoding/json"

	"heliosian/internal/model"
)

const eventFields = "id,title,start,end,allDay,location,description~400,classrooms,calendar-tags.name,day-type.name,availability,invited,me.answer,minePeople.name,minePeople.note,minePeople.mine,hostNames,link"

var calendarEvents = tool{
	name:        "calendar_events",
	description: "Events from the school calendar (Helios When) in a date range, the other apps' parties and HCA events folded in, each with the viewer's own standing: me.answer is yes, no, maybe or hidden, invited marks an event they were invited to, and minePeople is how their household stands. Without dates, the next two weeks; with a query, the whole calendar.",
	words:       "Looking at the calendar",
	properties: map[string]any{
		"from":      str("First day, like 2026-09-24. Today unless said."),
		"to":        str("Last day, like 2026-10-08. Two weeks after from unless said."),
		"query":     str("Words to find in a title, description or keywords."),
		"classroom": str("Only events for this classroom (and events for everyone)."),
		"tag":       str("Only events filed under this category or classroom tag."),
		"limit":     integer("How many to return, 40 unless said, 80 at most."),
	},
	run: func(t *turn, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct {
			From, To, Query, Classroom, Tag string
			Limit                           int
		}](input)
		if err != nil {
			return nil, err
		}
		day := today(t.clock())
		from, to := day.Format(model.DateFormat), day.AddDate(0, 0, 14).Format(model.DateFormat)
		if in.From != "" {
			start, err := date(in.From)
			if err != nil {
				return nil, err
			}
			from, to = in.From, start.AddDate(0, 0, 14).Format(model.DateFormat)
		}
		if in.To != "" {
			if _, err := date(in.To); err != nil {
				return nil, err
			}
			to = in.To
		}
		if in.Query != "" && in.From == "" && in.To == "" {
			from, to = "", ""
		}
		limit := limitOf(in.Limit, 40, 80)
		v := params("from", from, "to", to, "q", in.Query, "classroom", in.Classroom, "tag", in.Tag, "include", "calendar-tags,day-type", "limit", shown(limit))
		out, err := t.ask(query{name: "events", path: collection("events", v), fields: fields(eventFields), limit: limit})
		if err != nil {
			return nil, err
		}
		out["from"], out["to"] = from, to
		return out, nil
	},
}

var dayPlan = tool{
	name:        "day_plan",
	description: "What kind of school day a date is, classroom by classroom: Regular, Early Dismissal, No School and so on with the day's hours, and the all-day events that set it. Today unless a date is given; the viewer's own classrooms unless one is named, every classroom for someone with none. No plans means the date is outside the school year, or a weekend.",
	words:       "Checking the day plan",
	properties: map[string]any{
		"date":      str("The day, like 2026-09-24. Today unless said."),
		"classroom": str("One classroom, else the viewer's own."),
	},
	run: func(t *turn, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct{ Date, Classroom string }](input)
		if err != nil {
			return nil, err
		}
		day := today(t.clock())
		if in.Date != "" {
			if day, err = date(in.Date); err != nil {
				return nil, err
			}
		}
		v := params("date", day.Format(model.DateFormat), "classroom", in.Classroom, "include", "day-type,set-by")
		if in.Classroom == "" {
			v.Set("mine", "true")
		}
		spec := "classroom,date,weekday,schoolYear,yearStarts,yearEnds,day-type.name,day-type.blocks,set-by.title,set-by.link"
		out, err := t.ask(query{name: "plans", path: collection("day-plans", v), fields: fields(spec)})
		if err != nil {
			return nil, err
		}
		out["date"], out["weekday"] = day.Format(model.DateFormat), day.Weekday().String()
		return out, nil
	},
}
