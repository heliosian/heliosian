package app

import (
	"slices"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/home"
	"heliosian/internal/mail"
	"heliosian/internal/model"
)

func audience(cache *model.DirectoryCache, teamCache *model.ActivitiesCache, celebrateCache *model.PartiesCache) func() model.AudienceSources {
	return func() model.AudienceSources {
		return model.DirectoryAudience(cache.Model(), celebrateCache.Model(), teamCache.Model(), time.Now().In(model.Location))
	}
}

func loopAudience(cache *model.DirectoryCache, teamCache *model.ActivitiesCache, celebrateCache *model.PartiesCache) func() model.AudienceSources {
	return func() model.AudienceSources {
		return model.EmailListAudience(cache.Model(), celebrateCache.Model(), teamCache.Model(), teamCache, time.Now().In(model.Location))
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
