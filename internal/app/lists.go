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

func SmartLists(directory *who.Model, portal *team.Model, site *celebrate.Model, email string, now time.Time) []who.List {
	return append(parties(directory, site, email, now), activities(directory, portal, email, now)...)
}

func parties(directory *who.Model, model *celebrate.Model, email string, now time.Time) []who.List {
	out := []who.List{}
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
			} else {
				people[holder] = true
			}
			if known {
				people[buyer] = true
			}
		}
		for _, host := range list.Hosts {
			if directory.Person(host) != nil {
				people[host] = true
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
	// Every thing at or under one the viewer co-chairs is a list of theirs:
	// the event's whole team, and each committee's ("Spring Celebration:
	// Decor"), so a committee can have its own email list.
	var walk func(a, root *team.Activity, parent string)
	walk = func(a, root *team.Activity, parent string) {
		if over(a) {
			return
		}
		key := ""
		if parent != "" || chairs(a) {
			key = who.ListActivity + ":" + a.ID
			list := who.List{Key: key, Name: a.Title, Kind: who.ListActivity, Parent: parent, Guests: []who.Guest{}, Hosts: resolved(directory, a.CoChairs())}
			if a != root {
				list.Name = root.Title + ": " + a.Title
			}
			people := map[string]bool{}
			for _, node := range append([]*team.Activity{a}, a.Descendants()...) {
				for _, v := range node.Volunteers {
					if person := directory.Resolve(v.Email); directory.Person(person) != nil {
						people[person] = true
					}
				}
			}
			for _, host := range list.Hosts {
				if directory.Person(host) != nil {
					people[host] = true
				}
			}
			list.People = slices.Sorted(maps.Keys(people))
			out = append(out, list)
		}
		for _, c := range a.Children {
			walk(c, root, key)
		}
	}
	for _, a := range model.Activities {
		if a.Year == year {
			walk(a, a, "")
		}
	}
	return out
}

// activityEmailList is Team's lookup of an activity's email list: the Helios
// Loop group one of whose rules names the activity's list of volunteers
// ("activity:<id>"), by its name, or blank - so the activity page offers the
// list that is there rather than making another.
func activityEmailList(groups func() *loop.Model) team.EmailListLookup {
	return func(id string) string {
		key := who.ListActivity + ":" + id
		for _, g := range groups().Groups {
			for _, r := range g.Rules {
				if slices.Contains(r.Tags, key) {
					return g.Name
				}
			}
		}
		return ""
	}
}
