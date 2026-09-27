package app

import (
	"maps"
	"slices"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/celebrate"
	"heliosian/internal/filter"
	"heliosian/internal/home"
	"heliosian/internal/team"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

func audience(cache *who.Cache, teamCache *team.Cache, celebrateCache *celebrate.Cache) func() filter.Sources {
	return func() filter.Sources {
		model := cache.Model()
		return filter.Sources{Directory: model, Tags: model.Tags, Shared: model.SharedTags, Lists: func(owner string) []who.List {
			return append(model.RoomParentLists(owner), SmartLists(model, teamCache.Model(), celebrateCache.Model(), owner, time.Now().In(when.Location))...)
		}}
	}
}

func spoofPerson(cache *who.Cache) func(email string) (auth.Person, bool) {
	return func(email string) (auth.Person, bool) {
		model := cache.Model()
		p := model.Person(model.Resolve(strings.ToLower(strings.TrimSpace(email))))
		if p == nil {
			return auth.Person{}, false
		}
		return auth.Person{Email: p.Email, Name: p.FullName, Words: p.Words()}, true
	}
}

func spoofPeople(cache *who.Cache) func() []auth.Person {
	return func() []auth.Person {
		out := []auth.Person{}
		for _, p := range cache.Model().Listed() {
			out = append(out, auth.Person{Email: p.Email, Name: p.FullName, Words: p.Words()})
		}
		return out
	}
}

func calendarLists(cache *who.Cache, lists func(email string) []who.List) func(email string) []when.List {
	return func(email string) []when.List {
		out := []when.List{}
		model := cache.Model()
		tags := model.Tags(email)
		for _, name := range slices.Sorted(maps.Keys(tags)) {
			out = append(out, when.List{Key: "tag:" + name, Name: name, Kind: "tag", People: tags[name]})
		}
		for _, shared := range model.SharedTags(email) {
			out = append(out, when.List{Key: "shared:" + shared.Owner + ":" + shared.Name, Name: shared.Name + " (" + shared.OwnerName + "'s)", Kind: "tag", People: shared.People})
		}
		for _, list := range lists(email) {
			if list.Archived {
				continue
			}
			out = append(out, when.List{Key: list.Key, Name: list.Name, Kind: list.Kind, People: list.People})
		}
		return out
	}
}

type upcomingEvents struct {
	cache     *when.Cache
	directory func() *who.Model
	linked    func(email string) []when.Linked
}

func (u upcomingEvents) list(email, token string) home.Upcoming {
	model := u.cache.Model()
	out := home.Upcoming{Events: model.UpcomingUnder(u.directory(), email, u.linked(email), time.Now().In(when.Location), 6, token)}
	out.Calendars, out.Default, out.Calendar = savedCalendars(model, email, token)
	return out
}

func savedCalendars(model *when.Model, email, token string) (list []home.SavedCalendar, def, current string) {
	for _, f := range model.MyCalendars(email) {
		list = append(list, home.SavedCalendar{Token: f.Token, Name: f.Name, Emoji: f.Emoji, Locked: f.Locked})
	}
	def = list[0].Token
	current = def
	if slices.ContainsFunc(list, func(c home.SavedCalendar) bool { return c.Token == token }) {
		current = token
	}
	return list, def, current
}

func (u upcomingEvents) month(email, month, token string) home.Month {
	model := u.cache.Model()
	m := model.MonthUnder(u.directory(), email, u.linked(email), time.Now().In(when.Location), month, token)
	_, _, current := savedCalendars(model, email, token)
	return home.Month{Month: m.Month, Today: m.Today, Days: m.Days, Events: m.Events, Calendar: current}
}
