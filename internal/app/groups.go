package app

import (
	"sort"
	"strings"

	"heliosian/internal/config"
	"heliosian/internal/groups"
	"heliosian/internal/who"
)

// groupsDirectory hands Helios Groups the directory's view of people: who
// an address resolves to, the model its rules are read against, a person's
// tags and Magic Tags, everyone for the pickers, and the toolbar's badges.
type groupsDirectory struct {
	cache    *who.Cache
	settings *config.Cache
	lists    smartLists
}

func (d groupsDirectory) Resolve(email string) string {
	return d.cache.Model().Resolve(email)
}

func (d groupsDirectory) Model() *who.Model {
	return d.cache.Model()
}

func (d groupsDirectory) Tags(owner string) map[string][]string {
	return d.cache.Tags(owner)
}

// Lists is a person's Magic Tags as Who? lists them: the room parent lists
// the directory itself derives, then the other apps'.
func (d groupsDirectory) Lists(owner string) []who.List {
	return append(d.cache.Model().RoomParentLists(owner), d.lists.Lists(owner)...)
}

func groupsPerson(model *who.Model, p *who.Person) groups.Person {
	return groups.Person{Email: p.Email, Name: p.FullName, PhotoURL: model.HeroPhoto(p.Email), Words: placeWords(*p)}
}

func (d groupsDirectory) Person(email string) (groups.Person, bool) {
	model := d.cache.Model()
	p := model.Person(email)
	if p == nil {
		return groups.Person{}, false
	}
	return groupsPerson(model, p), true
}

func (d groupsDirectory) People() []groups.Person {
	model := d.cache.Model()
	out := make([]groups.Person, 0, len(model.People))
	for i := range model.People {
		if model.People[i].EmailMasked {
			continue
		}
		out = append(out, groupsPerson(model, &model.People[i]))
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

func (d groupsDirectory) Alerts(email string) (int, bool) {
	alerts := d.cache.Alerts(email, d.settings.Settings().StaleYears)
	return alerts.Stale, alerts.Privacy
}
