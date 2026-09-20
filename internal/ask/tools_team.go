package ask

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"heliosian/internal/calendar"
	"heliosian/internal/team"
)

// allOfTheYear is the ceiling on what volunteer_opportunities returns: high
// enough that a school year's portal comes back whole, since an answer about
// what still needs people is wrong if it only saw part of it.
const allOfTheYear = 500

// bulkBudget is what the year's listing may take before its detail is thinned.
// The output guard refuses anything past maxToolOutput outright, and a refusal
// sends the model round again with a narrower question - so a year that would
// not fit loses its descriptions and its name lists rather than losing rows,
// and nothing on the portal becomes invisible.
const bulkBudget = 30000

// fits says whether a listing is inside the budget.
func fits(cards []activityCard) bool {
	raw, err := json.Marshal(cards)
	return err != nil || len(raw) <= bulkBudget
}

// thin cuts a listing down to the budget in the order the answers can best
// afford: first the descriptions, which a listing rarely needs in full, and
// only then the volunteer names. The names go last because without them the
// model asks get_activity for each thing in turn to get them back, which costs
// more rounds than it saves.
func thin(cards []activityCard) ([]activityCard, string) {
	if fits(cards) {
		return cards, ""
	}
	out := make([]activityCard, len(cards))
	for i, c := range cards {
		c.Description = clip(c.Description, 100)
		out[i] = c
	}
	if fits(out) {
		return out, "Descriptions are cut short here, since the whole year is listed; get_activity has any one of these in full."
	}
	for i := range out {
		out[i].Volunteers = nil
	}
	return out, "Descriptions are cut short here and the volunteer names left out, since the whole year is listed; get_activity has any one of these in full."
}

// activityCard is one thing on HCA-Team as the tools answer it: the event,
// committee, role or shift, when it is (the event's day when it has none
// of its own), what it needs, who runs it and who signed up as the portal
// would show them to this viewer, and where the household stands.
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

func (v *viewer) activityCard(a *team.Activity) activityCard {
	runs := v.team.Runs(a, v.email)
	start, end, from := v.dates(a)
	c := activityCard{
		ID: a.ID, Title: a.Title, Status: a.Status, Timing: a.Timing, Start: start, End: end, When: v.timing(start, end), DatesFrom: from, Past: v.over(a), Location: a.Location, Description: clip(a.Description, 400),
		Spots: a.Spots, Taken: len(a.Volunteers), Full: a.VolunteersComplete || (a.Spots > 0 && len(a.Volunteers) >= a.Spots), NeedsCoLead: a.CoLeaderNeeded,
		CoChairs: []string{}, Hidden: a.VolunteersHidden && !runs, Link: teamBase + v.team.PathOf(a),
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
		} else if runs || !a.VolunteersHidden || v.family[vol.Email] {
			words := name
			if vol.Note != "" {
				words += " (" + vol.Note + ")"
			}
			c.Volunteers = append(c.Volunteers, words)
		}
		if v.family[vol.Email] {
			words := name + ": " + vol.Position
			if vol.Email == v.email {
				words = "you: " + vol.Position
			}
			c.Household = append(c.Household, words)
		}
	}
	return c
}

// dates is when a thing happens: its own start and end, or the nearest
// thing above it that has any - a role or a station under an event takes
// the event's day - with the title the dates came from when they are not
// its own.
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

// over says a thing has happened: marked done, or its last day is past,
// its day being the event's for a thing with none of its own.
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
		"limit":        integer("How many things to return, the whole year unless said."),
	},
	run: func(v *viewer, input json.RawMessage) (any, error) {
		if v.team == nil {
			return nil, fmt.Errorf("HCA-Team is not loaded right now")
		}
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
		// A school year's portal is a few hundred things at most, and a
		// question about what still needs people is wrong if it answers from
		// the first 80 of 113. Everything the year holds goes back, and the
		// output guard refuses the rare result that is genuinely too big.
		limit := limitOf(in.Limit, allOfTheYear, allOfTheYear)
		out := []activityCard{}
		total := 0
		var walk func(a *team.Activity)
		walk = func(a *team.Activity) {
			if !v.team.VisibleTo(a, v.email, false) {
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
		things, thinned := thin(out)
		answer := map[string]any{"today": v.now.Format(calendar.DateFormat), "year": year, "things": things, "matched": total, "shown": len(things), "expenseForm": v.team.Settings.ExpenseFormURL}
		if thinned != "" {
			answer["detail"] = thinned
		}
		return answer, nil
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
		if v.team == nil {
			return nil, fmt.Errorf("HCA-Team is not loaded right now")
		}
		in, err := decodeInput[struct{ ID, Path string }](input)
		if err != nil {
			return nil, err
		}
		a := v.team.Activity(strings.TrimSpace(in.ID))
		if a == nil && strings.TrimSpace(in.Path) != "" {
			a = v.team.Resolve(strings.TrimPrefix(strings.TrimSpace(in.Path), teamBase))
		}
		if a == nil || !v.team.VisibleTo(a, v.email, false) {
			return nil, fmt.Errorf("nothing on HCA-Team matches that")
		}
		under := []activityCard{}
		for _, d := range a.Descendants() {
			if v.team.VisibleTo(d, v.email, false) {
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
