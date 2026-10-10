package tools

import (
	"cmp"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/store"
)

const (
	activitiesLimit = 120
	listDomain      = "loop.heliosian.com"
)

var firstYear = regexp.MustCompile(`\d{4}`)

func (c *call) settings(app string) (map[string]string, error) {
	found, _, err := c.rows(tree{"from": "SETTING", "where": []any{eq("app", app)}})
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, s := range found {
		out[s["key"]] = s["value"]
	}
	return out, nil
}

func (c *call) past(g store.Row) bool {
	if g["status"] == "done" {
		return true
	}
	last := g["end"]
	if last == "" {
		last = g["start"]
	}
	if last == "" {
		return false
	}
	return last[:min(len(last), len(time.DateOnly))] < c.env.Now.Format(time.DateOnly)
}

func words(where []any, text string, columns ...string) []any {
	text = strings.TrimSpace(text)
	if text == "" {
		return where
	}
	alternatives := []any{}
	for _, col := range columns {
		alternatives = append(alternatives, tree{"contains": []any{path(col), text}})
	}
	return append(where, tree{"or": alternatives})
}

type placeIn struct {
	Words       string `json:"words,omitempty" jsonschema:"words to find in a name or description"`
	Year        string `json:"year,omitempty" jsonschema:"a school year, such as 2026 - 2027; the current one when left out"`
	IncludePast bool   `json:"include_past,omitempty" jsonschema:"also what has already happened or is done"`
}

type activity struct {
	named
	Heading     string   `json:"heading,omitempty"`
	Under       string   `json:"under,omitempty"`
	Status      string   `json:"status,omitempty"`
	Start       string   `json:"start,omitempty"`
	End         string   `json:"end,omitempty"`
	Timing      string   `json:"timing,omitempty"`
	Location    string   `json:"location,omitempty"`
	Description string   `json:"description,omitempty"`
	Capacity    string   `json:"capacity,omitempty"`
	Full        bool     `json:"full,omitempty"`
	LeadNeeded  bool     `json:"leadNeeded,omitempty"`
	Priority    bool     `json:"priority,omitempty"`
	Leads       []string `json:"leads,omitempty"`
	Volunteers  []string `json:"volunteers,omitempty"`
	Yours       string   `json:"yourPlace,omitempty"`
}

var activities = define("helios_activities", "Looking at HCA-Team", "What Helios School's parents' association, the HCA, runs on HCA-Team in a school year - the current one unless another is named - and who has signed up: every activity under its heading, the shifts and roles under an activity naming it as under, each with its timing, place, spots, whether it is full, whether it still wants a co-chair, its co-chairs and volunteers as HCA-Team shows them to the viewer, and the viewer's own place; plus HCA-Team's introduction and expense form. What has already happened is left out unless asked for.", func(c *call, in placeIn) (any, error) {
	roots, _, err := c.rows(tree{"from": "GROUP", "as": "r", "where": []any{eq("kind", "activity"), tree{"!=": []any{path("parent.kind"), "activity"}}, tree{"exists": tree{"from": "GROUP", "where": []any{eq("parent", path("@r")), eq("kind", "category")}}}}, "order": asc("name")})
	if err != nil {
		return nil, err
	}
	want := c.env.Now.Year()
	if c.env.Now.Month() < time.July {
		want--
	}
	if in.Year != "" {
		y, err := strconv.Atoi(firstYear.FindString(in.Year))
		if err != nil {
			return nil, fmt.Errorf("year is a school year such as 2026 - 2027, not %q", in.Year)
		}
		want = y
	}
	var root store.Row
	years := []string{}
	for _, r := range roots {
		years = append(years, r["name"])
		if y, err := strconv.Atoi(firstYear.FindString(r["name"])); err == nil && y == want {
			root = r
		}
	}
	if root == nil {
		return nil, fmt.Errorf("HCA-Team holds no school year starting in %d; it holds %s", want, strings.Join(years, ", "))
	}
	filed, _, err := c.rows(tree{"from": "GROUP", "where": []any{among("kind", []string{"activity", "category"})}})
	if err != nil {
		return nil, err
	}
	byID := map[string]store.Row{}
	for _, g := range filed {
		byID[g["id"]] = g
	}
	underRoot := func(g store.Row) bool {
		for cur, depth := byID[g["parent"]], 0; cur != nil && depth < maxAncestors; cur, depth = byID[cur["parent"]], depth+1 {
			if cur["id"] == root["id"] {
				return true
			}
		}
		return false
	}
	found, _, err := c.rows(tree{"from": "GROUP", "where": words([]any{eq("kind", "activity")}, in.Words, "name", "description"), "order": asc("order")})
	if err != nil {
		return nil, err
	}
	kept := []store.Row{}
	for _, g := range found {
		if underRoot(g) && (in.IncludePast || !c.past(g)) {
			kept = append(kept, g)
		}
	}
	more := max(0, len(kept)-activitiesLimit)
	kept = kept[:min(len(kept), activitiesLimit)]
	ids := []string{}
	for _, g := range kept {
		ids = append(ids, g["id"])
	}
	people := map[string][]store.Row{}
	names := map[string]string{}
	if len(ids) > 0 {
		signed, mres, err := c.rows(tree{"from": "MEMBER", "where": []any{among("group", ids), eq("member", "yes")}, "include": []any{"person"}})
		if err != nil {
			return nil, err
		}
		for _, m := range signed {
			people[m["group"]] = append(people[m["group"]], m)
			names[m["person"]] = title(mres.Resources["PERSON"][m["person"]])
		}
	}
	out := []activity{}
	for _, g := range kept {
		a := activity{named: c.named("GROUP", g), Start: g["start"], End: g["end"], Timing: g["timing"], Location: g["location"], Description: clip(g["description"], clipped), Capacity: g["capacity"], Full: g["join"] == "none", LeadNeeded: g["lead_needed"] == "Yes", Priority: g["priority"] == "Yes"}
		if g["status"] != "open" {
			a.Status = g["status"]
		}
		for cur, depth := byID[g["parent"]], 0; cur != nil && depth < maxAncestors; cur, depth = byID[cur["parent"]], depth+1 {
			if cur["kind"] == "category" {
				a.Heading = cur["name"]
				break
			}
		}
		if parent := byID[g["parent"]]; parent != nil && parent["kind"] == "activity" && parent["id"] != root["id"] {
			a.Under = parent["name"]
		}
		for _, m := range people[g["id"]] {
			name := names[m["person"]]
			if m["lead"] == "Yes" {
				a.Leads = append(a.Leads, name)
			} else {
				a.Volunteers = append(a.Volunteers, name)
			}
			if m["person"] == c.env.Viewer {
				a.Yours = "signed up"
				if m["lead"] == "Yes" {
					a.Yours = "co-chair"
				}
			}
		}
		out = append(out, a)
	}
	team, err := c.settings("team")
	if err != nil {
		return nil, err
	}
	answer := map[string]any{"year": root["name"], "intro": team["Intro"], "expenseForm": team["Expense Form URL"], "activities": out}
	if more > 0 {
		answer["more"] = more
	}
	return answer, nil
})

type partiesIn struct {
	Words       string `json:"words,omitempty" jsonschema:"words to find in a party's name, subtitle or description"`
	IncludePast bool   `json:"include_past,omitempty" jsonschema:"also the parties that have already happened"`
}

type partyOut struct {
	named
	Celebration   string   `json:"celebration,omitempty"`
	Subtitle      string   `json:"subtitle,omitempty"`
	Status        string   `json:"status,omitempty"`
	Start         string   `json:"start,omitempty"`
	End           string   `json:"end,omitempty"`
	Location      string   `json:"location,omitempty"`
	Address       string   `json:"address,omitempty"`
	Price         string   `json:"price,omitempty"`
	Unit          string   `json:"unit,omitempty"`
	Capacity      string   `json:"capacity,omitempty"`
	Full          bool     `json:"full,omitempty"`
	Waitlist      bool     `json:"takesWaitlist,omitempty"`
	Eligible      string   `json:"onlyFor,omitempty"`
	ParentTicket  bool     `json:"parentTicketRequired,omitempty"`
	DropOff       bool     `json:"dropOffAllowed,omitempty"`
	Description   string   `json:"description,omitempty"`
	Hosts         []string `json:"hosts,omitempty"`
	Tickets       []string `json:"ticketsShown,omitempty"`
	YourHousehold []string `json:"yourHouseholdTickets,omitempty"`
}

var parties = define("helios_parties", "Looking at the parties", "The fun(d)raiser parties on Helios Celebrate: every party still to come with its celebration, date, place, price, spots, whether it is full or takes a waitlist, who it is for, its hosts, the tickets the viewer may see and their own household's, plus the page's introduction, its ticket note and the current celebration. Parties that have happened are left out unless asked for. One call answers every party at once.", func(c *call, in partiesIn) (any, error) {
	where := words([]any{eq("kind", "party")}, in.Words, "name", "subtitle", "description")
	found, res, err := c.rows(tree{"from": "GROUP", "where": where, "order": asc("start"), "include": []any{"parent", "eligible"}})
	if err != nil {
		return nil, err
	}
	kept := []store.Row{}
	for _, g := range found {
		if in.IncludePast || !c.past(g) {
			kept = append(kept, g)
		}
	}
	ids := []string{}
	for _, g := range kept {
		ids = append(ids, g["id"])
	}
	tickets := map[string][]string{}
	ours := map[string][]string{}
	if len(ids) > 0 {
		held, mres, err := c.rows(tree{"from": "MEMBER", "where": []any{among("group", ids), eq("member", "yes")}, "include": []any{"person"}})
		if err != nil {
			return nil, err
		}
		household := map[string]bool{c.env.Viewer: true}
		mine, _, err := c.rows(tree{"from": "PERSON", "where": []any{tree{"in": []any{path("id"), tree{"household": []any{path("@viewer")}}}}}})
		if err != nil {
			return nil, err
		}
		for _, p := range mine {
			household[p["id"]] = true
		}
		for _, m := range held {
			name := title(mres.Resources["PERSON"][m["person"]])
			tickets[m["group"]] = append(tickets[m["group"]], name)
			if household[m["person"]] {
				ours[m["group"]] = append(ours[m["group"]], name)
			}
		}
	}
	out := []partyOut{}
	for _, g := range kept {
		hosts, err := c.managers(g)
		if err != nil {
			return nil, err
		}
		p := partyOut{named: c.named("GROUP", g), Subtitle: g["subtitle"], Start: g["start"], End: g["end"], Location: g["location"], Address: g["address"], Price: g["price"], Unit: g["unit"], Capacity: g["capacity"], Full: g["join"] == "none", Waitlist: g["waitlist"] != "", ParentTicket: g["parent_ticket_required"] == "Yes", DropOff: g["drop_off_allowed"] == "Yes", Description: clip(g["description"], clipped), Tickets: tickets[g["id"]], YourHousehold: ours[g["id"]]}
		if g["status"] != "open" {
			p.Status = g["status"]
		}
		if parent := res.Resources["GROUP"][g["parent"]]; parent != nil {
			p.Celebration = parent["name"]
		}
		if eligible := res.Resources["GROUP"][g["eligible"]]; eligible != nil {
			p.Eligible = eligible["name"]
		}
		for _, h := range hosts {
			p.Hosts = append(p.Hosts, h.Name)
		}
		out = append(out, p)
	}
	settings, err := c.settings("celebrate")
	if err != nil {
		return nil, err
	}
	answer := map[string]any{"intro": settings["Parties Intro"], "ticketNote": settings["Ticket Note"], "parties": out}
	if current, ok, err := c.row("GROUP", settings["Current"]); err != nil {
		return nil, err
	} else if ok {
		answer["currentCelebration"] = current["name"]
	}
	return answer, nil
})

type wordsIn struct {
	Words string `json:"words,omitempty" jsonschema:"words to find in a name or description"`
}

type list struct {
	named
	Address     string   `json:"address,omitempty"`
	Description string   `json:"description,omitempty"`
	Managers    []string `json:"managers,omitempty"`
	MemberCount int      `json:"memberCount"`
	Members     []string `json:"members,omitempty"`
	YouAreIn    bool     `json:"youAreIn,omitempty"`
}

var lists = define("helios_lists", "Looking at the email lists", "The email lists on Helios Loop the viewer can see - every group with an address: the ones they manage, the ones open to everyone and the ones they are on - each with its address, description, managers, how many are on it, its members where the viewer may see them, and whether the viewer is on it. A list's members follow from rules over the directory.", func(c *call, in wordsIn) (any, error) {
	found, _, err := c.rows(tree{"from": "GROUP", "where": words([]any{tree{"not": tree{"blank": path("slug")}}}, in.Words, "name", "slug", "description"), "order": asc("name")})
	if err != nil {
		return nil, err
	}
	out := []list{}
	for _, g := range found {
		managers, err := c.managers(g)
		if err != nil {
			return nil, err
		}
		members, count, err := c.membersOf(g["id"])
		if err != nil {
			return nil, err
		}
		l := list{named: c.named("GROUP", g), Description: clip(g["description"], clipped), MemberCount: count, Address: g["slug"] + "@" + listDomain}
		for _, m := range managers {
			l.Managers = append(l.Managers, m.Name)
		}
		for _, m := range members {
			l.Members = append(l.Members, m.Name)
			l.YouAreIn = l.YouAreIn || m.ID == c.env.Viewer
		}
		out = append(out, l)
	}
	return out, nil
})

type linkOut struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	Section     string `json:"section"`
}

var links = define("helios_links", "Looking at Heliosian's links", "The links on Heliosian's front page, the community's page of everything else it uses - the school's own sites, the handbook, lunch ordering, chats, photos and the like - each with its description and its section.", func(c *call, in wordsIn) (any, error) {
	widgets, wres, err := c.rows(tree{"from": "WIDGET", "where": []any{eq("key", "links")}, "order": asc("order"), "include": []any{"group"}})
	if err != nil {
		return nil, err
	}
	out := []linkOut{}
	for _, w := range widgets {
		if w["group"] == "" {
			continue
		}
		found, _, err := c.rows(tree{"from": "GROUP", "where": words([]any{eq("parent", w["group"]), tree{"not": tree{"blank": path("url")}}}, in.Words, "name", "description"), "order": asc("order")})
		if err != nil {
			return nil, err
		}
		section := cmp.Or(w["name"], title(wres.Resources["GROUP"][w["group"]]))
		for _, g := range found {
			out = append(out, linkOut{Name: g["name"], URL: g["url"], Description: g["description"], Section: section})
		}
	}
	if len(out) == 0 && strings.TrimSpace(in.Words) != "" {
		return nil, fmt.Errorf("no link matches %q", in.Words)
	}
	return out, nil
})
