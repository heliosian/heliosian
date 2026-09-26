package ask

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"heliosian/internal/calendar"
	"heliosian/internal/team"
)

type activityCard struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Under       string   `json:"under,omitempty"`
	Category    string   `json:"category,omitempty"`
	Status      string   `json:"status"`
	Timing      string   `json:"timing,omitempty"`
	Start       string   `json:"start,omitempty"`
	End         string   `json:"end,omitempty"`
	When        string   `json:"whenAgainstToday,omitempty"`
	DatesFrom   string   `json:"datesFrom,omitempty"`
	Past        bool     `json:"past"`
	Location    string   `json:"location,omitempty"`
	Description string   `json:"description,omitempty"`
	Spots       int      `json:"spots,omitempty"`
	Taken       int      `json:"taken"`
	Full        bool     `json:"full,omitempty"`
	NeedsCoLead bool     `json:"coLeaderNeeded,omitempty"`
	CoChairs    []string `json:"coChairs"`
	Volunteers  []string `json:"volunteers,omitempty"`
	Hidden      bool     `json:"volunteerListPrivate,omitempty"`
	Household   []string `json:"household,omitempty"`
	Link        string   `json:"link"`
}

func (v *viewer) activityCard(raw *team.Activity) activityCard {
	a := v.team.ActivityFor(raw, v.teamAs)
	editor := v.team.Edits(a, v.teamAs)
	start, end, from := v.dates(a)
	c := activityCard{
		ID: a.ID, Title: a.Title, Status: a.Status, Timing: a.Timing, Start: start, End: end, When: v.timing(start, end), DatesFrom: from, Past: v.over(a), Location: a.Location, Description: clip(a.Description, 400),
		Spots: a.Spots, Taken: a.Taken, Full: a.VolunteersComplete || (a.Spots > 0 && a.Taken >= a.Spots), NeedsCoLead: a.CoLeaderNeeded,
		CoChairs: []string{}, Hidden: a.VolunteersHidden && !editor, Link: teamBase + v.team.PathOf(a),
	}
	if a.Parent != "" {
		if parent := v.team.Activity(a.Parent); parent != nil {
			c.Under = parent.Title
		}
	}
	if cat := v.team.Category(a.Category); cat != nil {
		c.Category = cat.Title
	}
	for _, vol := range a.Volunteers {
		name := v.name(vol.Email)
		if vol.Position == team.PositionCoChair {
			c.CoChairs = append(c.CoChairs, name)
		} else {
			words := name
			if vol.Note != "" {
				words += " (" + vol.Note + ")"
			}
			c.Volunteers = append(c.Volunteers, words)
		}
		if v.teamAs.Mine(vol.Email) {
			words := name + ": " + vol.Position
			if vol.Email == v.email {
				words = "you: " + vol.Position
			}
			c.Household = append(c.Household, words)
		}
	}
	return c
}

func (v *viewer) dates(a *team.Activity) (start, end, from string) {
	for node := a; node != nil; node = v.team.Activity(node.Parent) {
		if node.Start != "" || node.End != "" {
			if node != a {
				from = node.Title
			}
			return node.Start, node.End, from
		}
		if node.Parent == "" {
			break
		}
	}
	return "", "", ""
}

func (v *viewer) over(a *team.Activity) bool {
	if a.Status == team.StatusDone {
		return true
	}
	start, end, _ := v.dates(a)
	cell := end
	if cell == "" {
		cell = start
	}
	if cell == "" {
		return false
	}
	last, err := team.ParseWhen(cell)
	if err != nil {
		return false
	}
	today := time.Date(v.now.Year(), v.now.Month(), v.now.Day(), 0, 0, 0, 0, time.UTC)
	return last.Before(today)
}

var volunteerOpportunities = tool{
	name:        "volunteer_opportunities",
	description: "What the HCA runs this school year on HCA-Team and who has signed up: every event with the committees, roles and shifts under it, each with its spots and who is on it, as the portal shows this viewer. What has already happened is left out unless asked for.",
	words:       "Looking at HCA-Team",
	properties: map[string]any{
		"query":        str("Words to find in a title or description."),
		"year":         str("A school year like 2026 - 2027; the current one unless said."),
		"include_past": boolean("Also what has already happened or is done."),
		"limit":        integer("How many things to return, 40 unless said, 80 at most."),
	},
	run: func(v *viewer, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct {
			Query, Year string
			IncludePast bool `json:"include_past"`
			Limit       int
		}](input)
		if err != nil {
			return nil, err
		}
		year := strings.TrimSpace(in.Year)
		if year == "" {
			year = team.SchoolYear(v.now)
		}
		limit := limitOf(in.Limit, 40, 80)
		out := []activityCard{}
		total := 0
		var walk func(a *team.Activity)
		walk = func(a *team.Activity) {
			if !v.team.VisibleTo(a, v.teamAs) {
				return
			}
			if (in.IncludePast || !v.over(a)) && (in.Query == "" || contains(a.Title, in.Query) || contains(a.Description, in.Query)) {
				total++
				if len(out) < limit {
					out = append(out, v.activityCard(a))
				}
			}
			for _, c := range a.Children {
				walk(c)
			}
		}
		for _, a := range v.team.Activities {
			if a.Year == year {
				walk(a)
			}
		}
		return map[string]any{"today": v.now.Format(calendar.DateFormat), "year": year, "things": out, "matched": total, "shown": len(out), "expenseForm": v.team.Settings.ExpenseFormURL}, nil
	},
}

var getActivity = tool{
	name:        "get_activity",
	description: "One thing on HCA-Team in full, by id or by its HCA-Team link (like the link a calendar event gives, or a path like /v/international-night/poland): the thing, its links, and everything under it.",
	words:       "Reading an HCA-Team page",
	properties: map[string]any{
		"id":   str("The thing's id, from volunteer_opportunities."),
		"path": str("Its HCA-Team link as another tool gave it, or its path on the site."),
	},
	run: func(v *viewer, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct{ ID, Path string }](input)
		if err != nil {
			return nil, err
		}
		a := v.team.Activity(strings.TrimSpace(in.ID))
		if a == nil && strings.TrimSpace(in.Path) != "" {
			a = v.team.Resolve(strings.TrimPrefix(strings.TrimSpace(in.Path), teamBase))
		}
		if a == nil || !v.team.VisibleTo(a, v.teamAs) {
			return nil, fmt.Errorf("nothing on HCA-Team matches that")
		}
		under := []activityCard{}
		for _, d := range a.Descendants() {
			if v.team.VisibleTo(d, v.teamAs) {
				under = append(under, v.activityCard(d))
			}
		}
		links := []map[string]string{}
		for _, l := range a.Links {
			links = append(links, map[string]string{"title": l.Title, "url": l.URL, "description": l.Description})
		}
		card := v.activityCard(a)
		card.Description = clip(a.Description, 3000)
		var highlight any
		if a.Highlight != nil {
			highlight = map[string]string{"headline": a.Highlight.Headline, "body": a.Highlight.Body}
		}
		return map[string]any{"thing": card, "highlight": highlight, "links": links, "under": under, "isEvent": a.Parent == ""}, nil
	},
}
