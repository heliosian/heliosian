package app

import (
	"maps"
	"slices"
	"strings"
	"time"

	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
	"heliosian/internal/loop"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

// smartLists is Who?'s lister: the Magic Tags a person has from the other
// apps, and the groups they manage on Helios Loop.
type smartLists struct {
	cache     *who.Cache
	team      *team.Cache
	celebrate *celebrate.Cache
	loop      *loop.Cache
	directory loopDirectory
}

func (s smartLists) Lists(email string) []who.List {
	lists := SmartLists(s.cache.Model(), s.team.Model(), s.celebrate.Model(), email, time.Now().In(calendar.Location))
	return append(lists, GroupLists(s.loop.Model(), loop.SourcesOf(s.directory), email)...)
}

// GroupLists is one Magic Tag per group the person manages: its members as
// the rules pick them out now, less the person themselves, as every Magic
// Tag leaves the viewer off, and the people added by hand as guests, since
// the directory does not hold them. Groups are not read against group
// rules - a group's rule cannot name another group - so these never feed
// the evaluator, only Who?.
func GroupLists(model *loop.Model, sources loop.Sources, email string) []who.List {
	out := []who.List{}
	for _, g := range model.Groups {
		if !g.Manages(email) {
			continue
		}
		people, guests := []string{}, []who.Guest{}
		for _, member := range loop.Members(g, sources) {
			switch {
			case member == email:
			case sources.Directory.Person(member) != nil:
				people = append(people, member)
			default:
				name := member
				if added := g.Addition(member); added != nil && added.Name != "" {
					name = added.Name
				}
				guests = append(guests, who.Guest{ID: g.Name + ":" + member, Name: name, Email: member})
			}
		}
		out = append(out, who.List{Key: who.ListGroup + ":" + g.Name, Name: g.Title, Kind: who.ListGroup, People: people, Guests: guests, Archived: model.Archived(g.Name, email)})
	}
	return out
}

// SmartLists is the Magic Tags a person has from the other apps, over
// models rather than caches: the parties they host on Helios Celebrate that
// have not happened yet and the activities they co-chair on HCA-Team this
// school year. An app whose model is nil contributes nothing.
func SmartLists(directory *who.Model, portal *team.Model, site *celebrate.Model, email string, now time.Time) []who.List {
	return append(parties(directory, site, email, now), activities(directory, portal, email, now)...)
}

func parties(directory *who.Model, model *celebrate.Model, email string, now time.Time) []who.List {
	out := []who.List{}
	if model == nil {
		return out
	}
	for _, p := range model.Parties {
		if p.Past(now) || !slices.ContainsFunc(p.HostEmails, func(h string) bool { return directory.Resolve(h) == email }) {
			continue
		}
		list := who.List{Key: who.ListParty + ":" + p.ID, Name: p.Title, Kind: who.ListParty, Guests: []who.Guest{}, Hosts: resolved(directory, p.HostEmails)}
		people := map[string]bool{}
		for _, t := range p.Tickets {
			if t.Status != celebrate.TicketSold {
				continue
			}
			holder := directory.Resolve(t.Email)
			buyer := directory.Resolve(t.Purchaser)
			known := directory.Person(buyer) != nil
			if t.Email == "" || directory.Person(holder) == nil {
				guest := who.Guest{ID: t.ID, Name: t.Name, Email: t.Email}
				if guest.Name == "" {
					guest.Name = celebrate.DisplayName(t.Email)
				}
				if buyer != holder {
					guest.Purchaser = buyer
					if p := directory.Person(buyer); p != nil {
						guest.PurchaserName = p.FullName
					} else {
						guest.PurchaserName = celebrate.DisplayName(buyer)
					}
				}
				list.Guests = append(list.Guests, guest)
			} else if holder != email {
				people[holder] = true
			}
			if known && buyer != email {
				people[buyer] = true
			}
		}
		list.People = slices.Sorted(maps.Keys(people))
		slices.SortFunc(list.Guests, func(a, b who.Guest) int { return strings.Compare(a.Name, b.Name) })
		out = append(out, list)
	}
	return out
}

func resolved(directory *who.Model, emails []string) []string {
	out := []string{}
	for _, e := range emails {
		if r := directory.Resolve(e); !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out
}

func activities(directory *who.Model, model *team.Model, email string, now time.Time) []who.List {
	out := []who.List{}
	if model == nil {
		return out
	}
	chairs := func(a *team.Activity) bool {
		return slices.ContainsFunc(a.CoChairs(), func(c string) bool { return directory.Resolve(c) == email })
	}
	year := team.SchoolYear(now)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	over := func(a *team.Activity) bool {
		if a.Status == team.StatusDone {
			return true
		}
		cell := a.End
		if cell == "" {
			cell = a.Start
		}
		if cell == "" {
			return false
		}
		last, _ := team.ParseWhen(cell)
		return last.Before(today)
	}
	var walk func(a, root *team.Activity, under bool)
	walk = func(a, root *team.Activity, under bool) {
		if over(a) {
			return
		}
		mine := !under && chairs(a)
		if mine {
			list := who.List{Key: who.ListActivity + ":" + a.ID, Name: a.Title, Kind: who.ListActivity, Guests: []who.Guest{}, Hosts: resolved(directory, a.CoChairs())}
			if a != root {
				list.Name = root.Title + ": " + a.Title
			}
			people := map[string]bool{}
			for _, node := range append([]*team.Activity{a}, a.Descendants()...) {
				for _, v := range node.Volunteers {
					if person := directory.Resolve(v.Email); person != email && directory.Person(person) != nil {
						people[person] = true
					}
				}
			}
			list.People = slices.Sorted(maps.Keys(people))
			out = append(out, list)
		}
		for _, c := range a.Children {
			walk(c, root, under || mine)
		}
	}
	for _, a := range model.Activities {
		if a.Year == year {
			walk(a, a, false)
		}
	}
	return out
}
