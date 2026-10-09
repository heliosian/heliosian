package tools

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"heliosian/internal/store"
)

const (
	eventsDays  = 14
	eventsLimit = 80
)

type eventsIn struct {
	From      string `json:"from,omitempty" jsonschema:"the first day, YYYY-MM-DD; today when left out, unless words alone are given, which search the whole calendar"`
	To        string `json:"to,omitempty" jsonschema:"the last day, YYYY-MM-DD; two weeks after from when left out"`
	Words     string `json:"words,omitempty" jsonschema:"words to find in a title"`
	Classroom string `json:"classroom,omitempty" jsonschema:"only what is for a family of this classroom, by name, which takes in what is for everyone"`
}

type event struct {
	named
	Kind        string `json:"kind"`
	Start       string `json:"start,omitempty"`
	End         string `json:"end,omitempty"`
	AllDay      bool   `json:"allDay,omitempty"`
	Timing      string `json:"timing,omitempty"`
	Location    string `json:"location,omitempty"`
	Status      string `json:"status,omitempty"`
	Category    string `json:"category,omitempty"`
	Description string `json:"description,omitempty"`
	ForYou      bool   `json:"forYourHousehold,omitempty"`
	Answer      string `json:"yourAnswer,omitempty"`
}

func (c *call) household() []any {
	return []any{tree{"=": []any{path("person"), path("@viewer")}}, tree{"in": []any{path("person"), tree{"household": []any{path("@viewer")}}}}}
}

func (c *call) forHousehold(ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	found, _, err := c.rows(tree{"from": "EFFECTIVE_MEMBER", "where": []any{among("group", ids), tree{"or": c.household()}}})
	if err != nil {
		return nil, err
	}
	for _, m := range found {
		out[m["group"]] = true
	}
	return out, nil
}

var events = define("helios_events", "Looking at the calendar", "Helios School's calendar: school and community events, Celebrate's parties and HCA-Team's activities starting in a range of days, in order, each with its category, whether it is for the viewer's household, and the viewer's own answer about coming (yes or no; none is maybe or no answer). Use it for what is coming up at Helios. A recent email can change what the calendar says; helios_search finds one.", func(c *call, in eventsIn) (any, error) {
	where := []any{
		tree{"or": []any{eq("kind", "event"), eq("kind", "party"), tree{"and": []any{eq("kind", "activity"), tree{"!=": []any{path("parent.kind"), "activity"}}}}}},
		tree{"not": tree{"blank": path("start")}},
		tree{"not": tree{"in": []any{path("status"), "closed", "pending"}}},
	}
	words := strings.TrimSpace(in.Words)
	if words != "" {
		where = append(where, tree{"contains": []any{path("name"), words}})
	}
	if words == "" || in.From != "" || in.To != "" {
		from, err := day(c.env.Now, in.From, "from")
		if err != nil {
			return nil, err
		}
		to := from.AddDate(0, 0, eventsDays)
		if in.To != "" {
			if to, err = day(c.env.Now, in.To, "to"); err != nil {
				return nil, err
			}
		}
		where = append(where, tree{">=": []any{path("start"), from.Format(time.DateOnly)}}, tree{"<": []any{path("start"), to.AddDate(0, 0, 1).Format(time.DateOnly)}})
	}
	if room := strings.TrimSpace(in.Classroom); room != "" {
		families := tree{"select": tree{"from": "MEMBER", "column": "person", "where": []any{
			tree{"in": []any{path("group"), tree{"select": tree{"from": "MEMBER", "column": "group", "where": []any{eq("person.classroom.name", room), eq("member", "yes"), eq("group.kind", "family")}}}}},
			eq("member", "yes"),
		}}}
		where = append(where, tree{"exists": tree{"from": "EFFECTIVE_MEMBER", "where": []any{eq("group", path("@e")), tree{"in": []any{path("person"), families}}}}})
	}
	found, res, err := c.rows(tree{"from": "GROUP", "as": "e", "where": where, "order": asc("start"), "include": []any{"parent"}})
	if err != nil {
		return nil, err
	}
	more := max(0, len(found)-eventsLimit)
	found = found[:min(len(found), eventsLimit)]
	ids := []string{}
	for _, g := range found {
		ids = append(ids, g["id"])
	}
	mine, err := c.forHousehold(ids)
	if err != nil {
		return nil, err
	}
	answers, err := c.answers(found)
	if err != nil {
		return nil, err
	}
	out := []event{}
	for _, g := range found {
		e := event{named: c.named("GROUP", g), Kind: g["kind"], Start: g["start"], End: g["end"], AllDay: g["all_day"] == "Yes", Timing: g["timing"], Location: g["location"], Description: clip(g["description"], clipped), ForYou: mine[g["id"]], Answer: answers[g["id"]]}
		if g["status"] != "open" {
			e.Status = g["status"]
		}
		if parent := res.Resources["GROUP"][g["parent"]]; parent != nil {
			e.Category = parent["name"]
		}
		out = append(out, e)
	}
	answer := map[string]any{"events": out}
	if more > 0 {
		answer["more"] = more
	}
	return answer, nil
})

func (c *call) answers(groups []store.Row) (map[string]string, error) {
	out := map[string]string{}
	answering := map[string]string{}
	ids := []string{}
	for _, g := range groups {
		for _, col := range []string{"rsvp_yes", "rsvp_no"} {
			if g[col] != "" {
				answering[g[col]] = g["id"]
				ids = append(ids, g[col])
			}
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	found, _, err := c.rows(tree{"from": "MEMBER", "where": []any{among("group", ids), eq("person", path("@viewer")), eq("member", "yes")}})
	if err != nil {
		return nil, err
	}
	for _, m := range found {
		for _, g := range groups {
			if g["id"] != answering[m["group"]] {
				continue
			}
			out[g["id"]] = "yes"
			if g["rsvp_no"] == m["group"] {
				out[g["id"]] = "no"
			}
		}
	}
	return out, nil
}

type daysIn struct {
	Date      string `json:"date,omitempty" jsonschema:"the day, YYYY-MM-DD; today when left out"`
	Classroom string `json:"classroom,omitempty" jsonschema:"only the plan for this classroom, by name"`
}

type part struct {
	Name  string `json:"name"`
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

type schoolDay struct {
	Type       string   `json:"type"`
	About      string   `json:"about,omitempty"`
	Classrooms []string `json:"classrooms,omitempty"`
	ForYou     bool     `json:"forYourHousehold,omitempty"`
	Parts      []part   `json:"parts"`
	SetBy      []string `json:"setBy,omitempty"`
	FirstDay   bool     `json:"firstDayOfYear,omitempty"`
	LastDay    bool     `json:"lastDayOfYear,omitempty"`
}

var days = define("helios_days", "Checking the day plan", "What kind of school day a date is at Helios School - Regular, Early Dismissal, No School and so on - with each part's hours (dropoff, school, pickup, aftercare), the classrooms each plan is for (every classroom when none are named), whether it is for the viewer's household, and the calendar entries that set it; and the school year's first and last days. No plan means a weekend, a holiday the calendar gives no day, or a date outside the school year.", func(c *call, in daysIn) (any, error) {
	date, err := day(c.env.Now, in.Date, "date")
	if err != nil {
		return nil, err
	}
	on := date.Format(time.DateOnly)
	found, res, err := c.rows(tree{"from": "GROUP", "where": []any{eq("kind", "day"), eq("start", on)}, "include": []any{"parent"}})
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, g := range found {
		ids = append(ids, g["id"])
	}
	mine, err := c.forHousehold(ids)
	if err != nil {
		return nil, err
	}
	rooms := map[string][]string{}
	if len(ids) > 0 {
		members, mres, err := c.rows(tree{"from": "EFFECTIVE_MEMBER", "where": []any{among("group", ids)}, "include": []any{"person.classroom"}})
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			p := mres.Resources["PERSON"][m["person"]]
			name := mres.Resources["GROUP"][p["classroom"]]["name"]
			if name != "" && !slices.Contains(rooms[m["group"]], name) {
				rooms[m["group"]] = append(rooms[m["group"]], name)
			}
		}
	}
	sources := []store.Row{}
	if len(ids) > 0 {
		if sources, _, err = c.rows(tree{"from": "GROUP_SOURCE", "where": []any{among("group", ids)}}); err != nil {
			return nil, err
		}
	}
	out := []schoolDay{}
	for _, g := range found {
		d := schoolDay{Type: g["name"], ForYou: mine[g["id"]], Classrooms: rooms[g["id"]], Parts: []part{}}
		slices.Sort(d.Classrooms)
		if parent := res.Resources["GROUP"][g["parent"]]; parent != nil {
			d.About = parent["description"]
		}
		if room := strings.TrimSpace(in.Classroom); room != "" && len(d.Classrooms) > 0 && !slices.ContainsFunc(d.Classrooms, func(r string) bool { return strings.EqualFold(r, room) }) {
			continue
		}
		if len(d.Classrooms) == 0 {
			d.ForYou = true
		}
		parts, _, err := c.rows(tree{"from": "GROUP", "where": []any{eq("parent", g["id"]), eq("kind", "day_part")}, "order": asc("start")})
		if err != nil {
			return nil, err
		}
		for _, p := range parts {
			d.Parts = append(d.Parts, part{Name: p["name"], Start: clock(p["start"]), End: clock(p["end"])})
		}
		for _, s := range sources {
			if s["group"] != g["id"] {
				continue
			}
			if s["name"] != "" && !slices.Contains(d.SetBy, s["name"]) {
				d.SetBy = append(d.SetBy, s["name"])
			}
			d.FirstDay = d.FirstDay || s["marker"] == "first_day"
			d.LastDay = d.LastDay || s["marker"] == "last_day"
		}
		out = append(out, d)
	}
	answer := map[string]any{"date": on, "weekday": date.Weekday().String(), "plans": out}
	bounds, err := c.yearBounds(on)
	if err != nil {
		return nil, err
	}
	for k, v := range bounds {
		answer[k] = v
	}
	return answer, nil
})

func clock(moment string) string {
	if _, t, ok := strings.Cut(moment, " "); ok {
		return t
	}
	return moment
}

func (c *call) yearBounds(on string) (map[string]string, error) {
	t, err := time.Parse(time.DateOnly, on)
	if err != nil {
		return nil, err
	}
	start := t.Year()
	if t.Month() < time.July {
		start--
	}
	from, to := fmt.Sprintf("%d-07-01", start), fmt.Sprintf("%d-07-01", start+1)
	marked, res, err := c.rows(tree{"from": "GROUP_SOURCE", "where": []any{tree{"not": tree{"blank": path("marker")}}, tree{">=": []any{path("group.start"), from}}, tree{"<": []any{path("group.start"), to}}}, "include": []any{"group"}})
	if err != nil {
		return nil, err
	}
	out := map[string]string{"schoolYear": fmt.Sprintf("%d-%d", start, start+1)}
	for _, s := range marked {
		g := res.Resources["GROUP"][s["group"]]
		switch s["marker"] {
		case "first_day":
			out["yearStarts"] = g["start"]
		case "last_day":
			out["yearEnds"] = g["start"]
		}
	}
	return out, nil
}
