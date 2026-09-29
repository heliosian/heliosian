package when

import (
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/model"
)

type Family struct {
	Actor access.Actor
	first map[string]string
	full  map[string]string
}

func FamilyOf(model *model.Directory, email string) Family {
	f := Family{
		Actor: access.Actor{Email: email, Household: model.Family(email)},
		first: map[string]string{email: ""},
		full:  map[string]string{},
	}
	adults, kids := model.Household(email)
	for _, p := range append(adults, kids...) {
		if f.Actor.Household[p.Email] {
			f.full[p.Email] = p.FullName
			f.first[p.Email] = firstName(p.FullName, p.Email)
		}
	}
	if me := model.Person(email); me != nil && me.FullName != "" {
		f.full[email] = me.FullName
	}
	return f
}

func firstName(name, email string) string {
	if words := strings.Fields(name); len(words) > 0 {
		return words[0]
	}
	local, _, _ := strings.Cut(email, "@")
	return local
}

func (f Family) Has(email string) bool {
	_, ok := f.first[email]
	return ok
}

func (f Family) Me(email string) bool {
	return f.Has(email) && f.first[email] == ""
}

func (f Family) Name(email, fallback string) string {
	if n := f.full[email]; n != "" {
		return n
	}
	if fallback != "" {
		return fallback
	}
	local, _, _ := strings.Cut(email, "@")
	return local
}

type Circle struct {
	mine  bool
	names []string
}

func (c *Circle) Add(f Family, email, name string) {
	if email == "" {
		return
	}
	if f.Me(email) {
		c.mine = true
		return
	}
	who := f.first[email]
	if who == "" {
		who = firstName(name, email)
	}
	if !slices.Contains(c.names, who) {
		c.names = append(c.names, who)
	}
}

func (c Circle) Any() bool {
	return c.mine || len(c.names) > 0
}

func (c Circle) Who() []string {
	if c.mine {
		return nil
	}
	return c.names
}
