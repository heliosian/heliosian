package ask

import (
	"encoding/json"
	"fmt"

	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
)

type partyCard struct {
	Title        string   `json:"title"`
	Subtitle     string   `json:"subtitle,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	NeedToKnow   string   `json:"needToKnow,omitempty"`
	Category     string   `json:"category,omitempty"`
	Start        string   `json:"start,omitempty"`
	End          string   `json:"end,omitempty"`
	When         string   `json:"whenAgainstToday,omitempty"`
	Past         bool     `json:"past"`
	Location     string   `json:"location,omitempty"`
	Address      string   `json:"address,omitempty"`
	Price        float64  `json:"price"`
	Unit         string   `json:"unit,omitempty"`
	Capacity     int      `json:"capacity,omitempty"`
	Sold         int      `json:"sold"`
	Remaining    int      `json:"remaining"`
	Waiting      int      `json:"waiting"`
	Availability string   `json:"availability"`
	Hosts        []string `json:"hosts"`
	Audience     string   `json:"audience,omitempty"`
	Who          []string `json:"who"`
	DropOff      bool     `json:"dropOff,omitempty"`
	ParentTicket bool     `json:"parentTicketRequired,omitempty"`
	Household    []string `json:"household,omitempty"`
	Attendees    []string `json:"attendees,omitempty"`
	Link         string   `json:"link"`
}

func (v *viewer) partyCard(raw *celebrate.Party) partyCard {
	p := raw.For(v.partyAs, v.sources.CelebrateDirectory)
	c := partyCard{
		Title: p.Title, Subtitle: p.Subtitle, Summary: p.Summary, NeedToKnow: clip(p.NeedToKnow, 400), Category: p.Category, Start: p.Start, End: p.End, When: v.timing(p.Start, p.End), Past: p.Past(v.now), Location: p.Location, Address: p.Address,
		Price: p.Price, Unit: p.Unit, Capacity: p.Capacity, Sold: p.Sold(), Remaining: p.Remaining(), Waiting: p.Waiting(), Availability: p.Availability(v.now),
		Hosts: v.names(p.HostEmails), Audience: p.Audience, Who: []string{}, DropOff: p.DropOff, ParentTicket: p.ParentTicket, Link: celebrateBase + v.celebrate.PathOf(p),
	}
	if p.Adults {
		c.Who = append(c.Who, "adults")
	}
	if p.Students {
		c.Who = append(c.Who, "students")
	}
	for _, t := range p.Tickets {
		holder := t.Name
		if t.Email != "" {
			holder = v.name(t.Email)
		}
		words := holder
		if t.Status == celebrate.TicketWaitlist {
			words = fmt.Sprintf("%s (waitlist, %d)", holder, t.Quantity)
		}
		if v.partyAs.Mine(t.Purchaser) || v.partyAs.Mine(t.Email) {
			c.Household = append(c.Household, words)
		}
		c.Attendees = append(c.Attendees, words)
	}
	return c
}

var parties = tool{
	name:        "parties",
	description: "The fun(d)raiser parties on Helios Celebrate: the current celebration, and every party still to come with its date, place, price, tickets left, waitlist, hosts, who may come and who holds tickets, plus the viewer's household's own tickets; each says where it stands against today. Parties that have happened are counted and left out unless asked for. One call is enough: it returns every party at once.",
	words:       "Looking at the parties",
	properties: map[string]any{
		"query":        str("Words to find in a title, summary or category."),
		"include_past": boolean("Also the parties that have already happened."),
	},
	run: func(v *viewer, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct {
			Query       string
			IncludePast bool `json:"include_past"`
		}](input)
		if err != nil {
			return nil, err
		}
		var celebration any
		if c := v.celebrate.Current(); c != nil {
			celebration = map[string]string{"title": c.Title, "subtitle": c.Subtitle, "start": c.Start, "end": c.End, "location": c.Location, "description": clip(c.Description, 600)}
		}
		out := []partyCard{}
		past := 0
		for _, p := range v.celebrate.SortedParties("") {
			if !p.VisibleTo(v.partyAs) {
				continue
			}
			if in.Query != "" && !contains(p.Title, in.Query) && !contains(p.Summary, in.Query) && !contains(p.Category, in.Query) && !contains(p.Subtitle, in.Query) {
				continue
			}
			if p.Past(v.now) && !in.IncludePast {
				past++
				continue
			}
			out = append(out, v.partyCard(p))
		}
		return map[string]any{"today": v.now.Format(calendar.DateFormat), "celebration": celebration, "parties": out, "pastPartiesLeftOut": past, "intro": clip(v.celebrate.Settings.PartiesIntro, 600), "ticketNote": clip(v.celebrate.Settings.TicketNote, 400)}, nil
	},
}
