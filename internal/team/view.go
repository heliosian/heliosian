package team

import (
	"sort"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/config"
	"heliosian/internal/who"
)

type Child struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Grade string `json:"grade,omitempty"`
}

func household(model *who.Model, p *who.Person) (adults, kids []Child) {
	if p == nil || !p.IsParent {
		return nil, nil
	}
	grown, young := model.Household(p.Email)
	for _, a := range grown {
		adults = append(adults, Child{Email: a.Email, Name: a.FullName})
	}
	for _, k := range young {
		kids = append(kids, Child{Email: k.Email, Name: k.FullName, Grade: k.Grade})
	}
	return adults, kids
}

func parentsOf(model *who.Model, email string) []string {
	p := model.Person(email)
	if p == nil || !p.IsStudent {
		return nil
	}
	return p.ParentContactEmails
}

func (d viewer) person(email string) (string, string) {
	if p := d.directory.Person(d.directory.Resolve(strings.ToLower(email))); p != nil {
		return p.FullName, d.directory.HeroPhoto(p.Email)
	}
	return cells.DisplayName(email), ""
}

type viewer struct {
	access.Actor
	directory *who.Model
}

type Years struct {
	Current string `json:"current"`
	Last    string `json:"last"`
	Next    string `json:"next"`
}

type ActivityView struct {
	*Activity
	Children   []*ActivityView `json:"children"`
	Volunteers []Volunteer     `json:"volunteers"`
	Taken      int             `json:"taken"`
	CanEdit    bool            `json:"canEdit"`
	Runs       bool            `json:"runs,omitempty"`
	Started    bool            `json:"started,omitempty"`
	Invited    bool            `json:"invited,omitempty"`
	EmailList  string          `json:"emailList,omitempty"`
}

type EmailListLookup func(id string) string

type EventRSVPs struct {
	Sent    bool
	Answers map[string]string
}

type RSVPLookup func(id string) *EventRSVPs

type PersonView struct {
	Email      string `json:"email"`
	Name       string `json:"name"`
	PhotoURL   string `json:"photoUrl,omitempty"`
	Activities int    `json:"activities"`
	CoChairing int    `json:"coChairing"`
}

type View struct {
	User        User              `json:"user"`
	Years       Years             `json:"years"`
	Settings    Settings          `json:"settings"`
	Categories  []Category        `json:"categories"`
	Activities  []ActivityView    `json:"activities"`
	People      []PersonView      `json:"people,omitempty"`
	Redirects   []Redirect        `json:"redirects"`
	ImageSearch bool              `json:"imageSearch"`
	GradeColors map[string]string `json:"gradeColors,omitempty"`
}

type User struct {
	Email    string  `json:"email"`
	Name     string  `json:"name"`
	Initial  string  `json:"initial"`
	PhotoURL string  `json:"photoUrl,omitempty"`
	IsAdmin  bool    `json:"isAdmin"`
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

func (m *Model) Edits(a *Activity, v access.Actor) bool {
	return v.May(ActAsCochair) || m.Runs(a, v.Email)
}

func (m *Model) Sees(a *Activity, v access.Actor) bool {
	return v.May(SeeAll) || m.Runs(a, v.Email)
}

func (m *Model) VisibleTo(a *Activity, v access.Actor) bool {
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

func (m *Model) ActivityFor(a *Activity, v access.Actor) *Activity {
	if !m.VisibleTo(a, v) {
		return nil
	}
	return m.activityFor(a, v)
}

func (m *Model) activityFor(a *Activity, v access.Actor) *Activity {
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

func (v viewer) volunteers(list []Volunteer) []Volunteer {
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

func (v viewer) activity(model *Model, a *Activity, runs bool, lists EmailListLookup) ActivityView {
	chairs := runs || a.IsCoChair(v.Email)
	view := ActivityView{
		Activity:   a,
		Children:   []*ActivityView{},
		Volunteers: v.volunteers(a.Volunteers),
		Taken:      a.Taken,
		CanEdit:    model.Edits(a, v.Actor),
		Runs:       chairs,
	}
	for _, c := range a.Children {
		child := v.activity(model, c, chairs, lists)
		view.Children = append(view.Children, &child)
	}
	if chairs && lists != nil {
		view.EmailList = lists(a.ID)
	}
	return view
}

func Render(model *Model, directory *who.Model, settings *config.Settings, as access.Actor, now time.Time) View {
	return RenderWith(model, directory, settings, nil, nil, as, now)
}

func RenderWith(model *Model, directory *who.Model, settings *config.Settings, rsvps RSVPLookup, lists EmailListLookup, as access.Actor, now time.Time) View {
	v := viewer{Actor: as, directory: directory}
	email, admin := as.Email, as.May(SeeAll)
	name, photo := v.person(email)
	spouses, children := household(directory, directory.Person(email))
	current := SchoolYear(now)
	view := View{
		User:        User{Email: email, Name: name, Initial: strings.ToUpper(name[:1]), PhotoURL: photo, IsAdmin: admin, Spouses: spouses, Children: children},
		Years:       Years{Current: current, Last: ShiftYear(current, -1), Next: ShiftYear(current, 1)},
		Settings:    model.Settings,
		Categories:  model.Categories,
		Activities:  []ActivityView{},
		Redirects:   model.Redirects,
		GradeColors: settings.GradeColors,
	}
	for _, raw := range model.Activities {
		a := model.ActivityFor(raw, as)
		if a == nil {
			continue
		}
		av := v.activity(model, a, false, lists)
		if model.Sees(raw, as) && rsvps != nil {
			if r := rsvps(a.ID); r != nil {
				av.Started, av.Invited = true, r.Sent
				if r.Sent {
					for i := range av.Volunteers {
						av.Volunteers[i].RSVP = r.Answers[directory.Resolve(strings.ToLower(av.Volunteers[i].Email))]
					}
				}
			}
		}
		view.Activities = append(view.Activities, av)
	}
	if admin {
		view.People = v.people(model)
	}
	return view
}

func (v viewer) people(model *Model) []PersonView {
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
	for _, a := range model.Activities {
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
