package app

import (
	"sort"
	"strings"
	"time"

	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
	"heliosian/internal/config"
	"heliosian/internal/loop"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

// loopDirectory hands Helios Loop the directory's view of people: who
// an address resolves to, the model its rules are read against, a person's
// tags, Magic Tags and the tags shared with them, everyone for the pickers,
// and the toolbar's badges.
type loopDirectory struct {
	cache     *who.Cache
	settings  *config.Cache
	team      *team.Cache
	celebrate *celebrate.Cache
}

func (d loopDirectory) Resolve(email string) string {
	return d.cache.Model().Resolve(email)
}

func (d loopDirectory) Model() *who.Model {
	return d.cache.Model()
}

func (d loopDirectory) Tags(owner string) map[string][]string {
	return d.cache.Tags(owner)
}

// Lists is a person's Magic Tags as the rules may name them: the room
// parent lists the directory itself derives, then the other apps' - never
// the groups', since a group's rule cannot name another group.
func (d loopDirectory) Lists(owner string) []who.List {
	return append(d.cache.Model().RoomParentLists(owner), SmartLists(d.cache.Model(), d.team.Model(), d.celebrate.Model(), owner, time.Now().In(calendar.Location))...)
}

// Shared is the tags other people have let this person manage.
func (d loopDirectory) Shared(email string) []who.SharedTag {
	return d.cache.SharedTags(email)
}

func loopPerson(model *who.Model, p *who.Person) loop.Person {
	return loop.Person{Email: p.Email, Name: p.FullName, PhotoURL: model.HeroPhoto(p.Email), Words: placeWords(*p)}
}

func (d loopDirectory) Person(email string) (loop.Person, bool) {
	model := d.cache.Model()
	p := model.Person(email)
	if p == nil {
		return loop.Person{}, false
	}
	return loopPerson(model, p), true
}

func (d loopDirectory) People() []loop.Person {
	model := d.cache.Model()
	out := make([]loop.Person, 0, len(model.People))
	for i := range model.People {
		if model.People[i].EmailMasked {
			continue
		}
		out = append(out, loopPerson(model, &model.People[i]))
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func (d loopDirectory) Alerts(email string) (int, bool) {
	alerts := d.cache.Alerts(email, d.settings.Settings().StaleYears)
	return alerts.Stale, alerts.Privacy
}
