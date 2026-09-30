package model

import (
	"sort"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
)

type Child struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Grade string `json:"grade,omitempty"`
}

func activityHousehold(directory *Directory, p *Person) (adults, kids []Child) {
	if p == nil || !p.IsParent {
		return nil, nil
	}
	grown, young := directory.Household(p.Email)
	for _, a := range grown {
		adults = append(adults, Child{Email: a.Email, Name: a.FullName})
	}
	for _, k := range young {
		kids = append(kids, Child{Email: k.Email, Name: k.FullName, Grade: k.Grade})
	}
	return adults, kids
}

func parentsOf(directory *Directory, email string) []string {
	p := directory.Person(email)
	if p == nil || !p.IsStudent {
		return nil
	}
	return p.ParentContactEmails
}

func (d activityViewer) person(email string) (string, string) {
	if p := d.directory.Person(d.directory.Resolve(strings.ToLower(email))); p != nil {
		return p.FullName, d.directory.HeroPhoto(p.Email)
	}
	return cells.DisplayName(email), ""
}

type activityViewer struct {
	access.Actor
	directory *Directory
}

type Years struct {
	Current string `json:"current"`
	Last    string `json:"last"`
	Next    string `json:"next"`
}

type PersonView struct {
	Email      string `json:"email"`
	Name       string `json:"name"`
	PhotoURL   string `json:"photoUrl,omitempty"`
	Activities int    `json:"activities"`
	CoChairing int    `json:"coChairing"`
}

type ActivitiesUser struct {
	Email    string  `json:"email"`
	Name     string  `json:"name"`
	Initial  string  `json:"initial"`
	PhotoURL string  `json:"photoUrl,omitempty"`
	Spouses  []Child `json:"spouses,omitempty"`
	Children []Child `json:"children,omitempty"`
}

func visibleStatus(status, addedBy, email string, editor bool) bool {
	switch status {
	case StatusHidden:
		return editor
	case StatusPending:
		return editor || (email != "" && addedBy == email)
	}
	return true
}

func (a *Activity) shownStatus() string {
	if a.CategoryHidden && a.Status != StatusPending {
		return StatusHidden
	}
	return a.Status
}

func (m *Activities) Edits(a *Activity, v access.Actor) bool {
	return v.May(ActAsCochair) || m.Runs(a, v.Email)
}

func (m *Activities) Sees(a *Activity, v access.Actor) bool {
	return v.May(SeeAllActivities) || m.Runs(a, v.Email)
}

func (m *Activities) VisibleTo(a *Activity, v access.Actor) bool {
	for node := a; node != nil; node = m.Activity(node.Parent) {
		if !visibleStatus(node.shownStatus(), node.AddedBy, v.Email, m.Sees(node, v)) {
			return false
		}
		if node.Parent == "" {
			return true
		}
	}
	return false
}

func (m *Activities) ActivityFor(a *Activity, v access.Actor) *Activity {
	if !m.VisibleTo(a, v) {
		return nil
	}
	return m.activityFor(a, v)
}

func (m *Activities) activityFor(a *Activity, v access.Actor) *Activity {
	sees := m.Sees(a, v)
	c := *a
	c.Taken = len(a.Volunteers)
	c.Volunteers = []Volunteer{}
	for _, vol := range a.Volunteers {
		if a.VolunteersHidden && !sees && vol.Position != PositionCoChair && !v.Mine(vol.Email) {
			continue
		}
		if !sees {
			vol.AddedBy = ""
		}
		c.Volunteers = append(c.Volunteers, vol)
	}
	c.Children = []*Activity{}
	for _, child := range a.Children {
		if visibleStatus(child.shownStatus(), child.AddedBy, v.Email, m.Sees(child, v)) {
			c.Children = append(c.Children, m.activityFor(child, v))
		}
	}
	return &c
}

func (v activityViewer) volunteers(list []Volunteer) []Volunteer {
	out := []Volunteer{}
	for _, vol := range list {
		vol.Name, vol.PhotoURL = v.person(vol.Email)
		if p := v.directory.Person(vol.Email); p != nil && p.IsStudent {
			vol.Grade = p.Grade
		}
		if vol.AddedBy != "" && vol.AddedBy != vol.Email {
			vol.AddedByName, _ = v.person(vol.AddedBy)
		}
		out = append(out, vol)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Position == PositionCoChair && out[j].Position != PositionCoChair
	})
	return out
}

func (v activityViewer) people(m *Activities) []PersonView {
	byEmail := map[string]*PersonView{}
	count := func(list []Volunteer) {
		for _, vol := range list {
			p := byEmail[vol.Email]
			if p == nil {
				name, photo := v.person(vol.Email)
				p = &PersonView{Email: vol.Email, Name: name, PhotoURL: photo}
				byEmail[vol.Email] = p
			}
			p.Activities++
			if vol.Position == PositionCoChair {
				p.CoChairing++
			}
		}
	}
	for _, a := range m.Activities {
		count(a.Volunteers)
		for _, c := range a.Descendants() {
			count(c.Volunteers)
		}
	}
	out := []PersonView{}
	for _, p := range byEmail {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Activities != out[j].Activities {
			return out[i].Activities > out[j].Activities
		}
		return out[i].Name < out[j].Name
	})
	return out
}
