package app

import (
	"maps"
	"slices"
	"time"

	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
	"heliosian/internal/events"
	"heliosian/internal/who"
)

type smartLists struct {
	cache     *who.Cache
	events    *events.Cache
	celebrate *celebrate.Cache
}

func (s smartLists) Lists(email string) []who.List {
	now := time.Now().In(calendar.Location)
	return append(s.parties(email, now), s.activities(email, now)...)
}

func (s smartLists) parties(email string, now time.Time) []who.List {
	out := []who.List{}
	model := s.celebrate.Model()
	if model == nil {
		return out
	}
	directory := s.cache.Model()
	for _, p := range model.Parties {
		if p.Past(now) || !slices.ContainsFunc(p.HostEmails, func(h string) bool { return directory.Resolve(h) == email }) {
			continue
		}
		list := who.List{Key: who.ListParty + ":" + p.ID, Name: p.Title, Kind: who.ListParty}
		people := map[string]bool{}
		for _, t := range p.Tickets {
			if t.Status != celebrate.TicketSold {
				continue
			}
			holder := directory.Resolve(t.Email)
			if t.Email == "" || directory.Person(holder) == nil {
				list.Guests++
			} else if holder != email {
				people[holder] = true
			}
			if buyer := directory.Resolve(t.Purchaser); buyer != email && directory.Person(buyer) != nil {
				people[buyer] = true
			}
		}
		list.People = slices.Sorted(maps.Keys(people))
		out = append(out, list)
	}
	return out
}

func (s smartLists) activities(email string, now time.Time) []who.List {
	out := []who.List{}
	model := s.events.Model()
	if model == nil {
		return out
	}
	directory := s.cache.Model()
	chairs := func(a *events.Activity) bool {
		return slices.ContainsFunc(a.CoChairs(), func(c string) bool { return directory.Resolve(c) == email })
	}
	year := events.SchoolYear(now)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	over := func(a *events.Activity) bool {
		if a.Status == events.StatusDone {
			return true
		}
		cell := a.End
		if cell == "" {
			cell = a.Start
		}
		if cell == "" {
			return false
		}
		last, _ := events.ParseWhen(cell)
		return last.Before(today)
	}
	var walk func(a, root *events.Activity, under bool)
	walk = func(a, root *events.Activity, under bool) {
		if over(a) {
			return
		}
		mine := !under && chairs(a)
		if mine {
			list := who.List{Key: who.ListActivity + ":" + a.ID, Name: a.Title, Kind: who.ListActivity}
			if a != root {
				list.Name = root.Title + ": " + a.Title
			}
			people := map[string]bool{}
			for _, node := range append([]*events.Activity{a}, a.Descendants()...) {
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
