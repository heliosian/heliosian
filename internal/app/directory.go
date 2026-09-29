package app

import (
	"slices"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/celebrate"
	"heliosian/internal/home"
	"heliosian/internal/mail"
	"heliosian/internal/model"
	"heliosian/internal/team"
)

func magicTags(directory *model.Directory, parties *celebrate.Model, activities *team.Model) func(owner string, now time.Time) []model.MagicTag {
	return func(owner string, now time.Time) []model.MagicTag {
		lists := append(directory.RoomParentTags(owner), parties.Lists(directory, owner, now)...)
		return append(lists, activities.Lists(directory, owner, now)...)
	}
}

func magicTagKeys(parties *celebrate.Model, activities *team.Model) func() []string {
	return func() []string {
		out := []string{}
		for _, p := range parties.Parties {
			out = append(out, model.MagicTagParty+":"+p.ID)
		}
		for _, a := range activities.Activities {
			out = append(out, model.MagicTagActivity+":"+a.ID)
		}
		return out
	}
}

func audience(cache *model.DirectoryCache, teamCache *team.Cache, celebrateCache *celebrate.Cache) func() model.AudienceSources {
	return func() model.AudienceSources {
		directory := cache.Model()
		lists := magicTags(directory, celebrateCache.Model(), teamCache.Model())
		return model.AudienceSources{Directory: directory, MagicTags: func(owner string) []model.MagicTag {
			return lists(owner, time.Now().In(model.Location))
		}}
	}
}

func spoofPerson(cache *model.DirectoryCache) func(email string) (auth.Person, bool) {
	return func(email string) (auth.Person, bool) {
		directory := cache.Model()
		p := directory.Person(directory.Resolve(mail.Normalize(email)))
		if p == nil {
			return auth.Person{}, false
		}
		return auth.Person{Email: p.Email, FullName: p.FullName, Words: p.Words()}, true
	}
}

func calendarLists(cache *model.DirectoryCache, lists func(email string) []model.MagicTag) func(email string) []model.PickerList {
	return func(email string) []model.PickerList {
		out := []model.PickerList{}
		directory := cache.Model()
		for _, t := range directory.Tags(email) {
			out = append(out, model.PickerList{Key: model.TagKey(t.ID), Name: t.Name, Kind: "tag", People: t.People})
		}
		for _, t := range directory.SharedTags(email) {
			out = append(out, model.PickerList{Key: model.TagKey(t.ID), Name: t.Name + " (" + t.OwnerName + "'s)", Kind: "tag", People: t.People})
		}
		for _, list := range lists(email) {
			if list.Archived {
				continue
			}
			out = append(out, model.PickerList{Key: list.Key, Name: list.Name, Kind: list.Kind, People: list.People})
		}
		return out
	}
}

type upcomingEvents struct {
	cache     *model.CalendarCache
	directory func() *model.Directory
	linked    func(email string) []model.Linked
}

func (u upcomingEvents) list(email, token string) home.Upcoming {
	calendar := u.cache.Model()
	out := home.Upcoming{Events: calendar.UpcomingUnder(u.directory(), email, u.linked(email), time.Now().In(model.Location), 6, token)}
	out.Calendars, out.Default, out.Calendar = savedCalendars(calendar, email, token)
	return out
}

func savedCalendars(calendar *model.Calendar, email, token string) (list []home.SavedCalendar, def, current string) {
	for _, f := range calendar.MyCalendars(email) {
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
	calendar := u.cache.Model()
	m := calendar.MonthUnder(u.directory(), email, u.linked(email), time.Now().In(model.Location), month, token)
	_, _, current := savedCalendars(calendar, email, token)
	return home.Month{Month: m.Month, Today: m.Today, Days: m.Days, Events: m.Events, Calendar: current}
}
