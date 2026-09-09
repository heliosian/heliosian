package events

import (
	"sort"
	"strings"
	"time"
)

// Directory is what the portal asks of the school directory: who a signed-in
// address really is, and a name and photo for an email.
type Directory interface {
	Resolve(email string) string
	Person(email string) (name, photoURL string, ok bool)
}

// displayName reads a name out of an address for someone the directory does not
// list, so a list of volunteers never shows a bare email.
func displayName(email string) string {
	local, _, _ := strings.Cut(email, "@")
	words := strings.FieldsFunc(local, func(r rune) bool { return r == '.' || r == '_' || r == '-' })
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func (d viewer) person(email string) (string, string) {
	if name, photo, ok := d.directory.Person(email); ok {
		return name, photo
	}
	return displayName(email), ""
}

type viewer struct {
	email     string
	admin     bool
	directory Directory
}

type Years struct {
	Current string `json:"current"`
	Last    string `json:"last"`
	Next    string `json:"next"`
}

// Taken counts every sign-up, including the ones a hidden list withholds, so a
// viewer can still see whether spots remain.
type ActivityView struct {
	*Activity
	Roles      []*RoleView `json:"roles"`
	Volunteers []Volunteer `json:"volunteers"`
	Taken      int         `json:"taken"`
	CanEdit    bool        `json:"canEdit"`
}

type RoleView struct {
	*Role
	Roles      []*RoleView `json:"roles"`
	Volunteers []Volunteer `json:"volunteers"`
	Taken      int         `json:"taken"`
}

type PersonView struct {
	Email      string `json:"email"`
	Name       string `json:"name"`
	PhotoURL   string `json:"photoUrl,omitempty"`
	Activities int    `json:"activities"`
	CoChairing int    `json:"coChairing"`
}

type View struct {
	User       User           `json:"user"`
	Years      Years          `json:"years"`
	Settings   Settings       `json:"settings"`
	Categories []Category     `json:"categories"`
	Activities []ActivityView `json:"activities"`
	People     []PersonView   `json:"people,omitempty"`
}

type User struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Initial string `json:"initial"`
	IsAdmin bool   `json:"isAdmin"`
}

// canEdit reports whether the viewer runs this activity: an admin, or one of its
// co-chairs.
func (v viewer) canEdit(a *Activity) bool {
	return v.admin || a.IsCoChair(v.email)
}

// visible decides whether a pending or hidden item reaches this viewer: hidden
// ones only reach editors, pending ones also reach whoever proposed them.
func (v viewer) visible(status, addedBy string, editor bool) bool {
	switch status {
	case StatusHidden:
		return editor
	case StatusPending:
		return editor || addedBy == v.email
	}
	return true
}

// volunteers lists who signed up, named and pictured, holding back a hidden
// list from anyone but the activity's editors: those viewers see the co-chairs
// and themselves, so they know who to ask and that their own sign-up took.
func (v viewer) volunteers(list []Volunteer, hidden, editor bool) []Volunteer {
	out := []Volunteer{}
	for _, vol := range list {
		if hidden && !editor && vol.Position != PositionCoChair && vol.Email != v.email {
			continue
		}
		vol.Name, vol.PhotoURL = v.person(vol.Email)
		if !editor {
			vol.AddedBy = ""
		}
		out = append(out, vol)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Position == PositionCoChair && out[j].Position != PositionCoChair
	})
	return out
}

func (v viewer) roles(roles []*Role, hidden, editor bool) []*RoleView {
	out := []*RoleView{}
	for _, r := range roles {
		if !v.visible(r.Status, r.AddedBy, editor) {
			continue
		}
		out = append(out, &RoleView{
			Role:       r,
			Roles:      v.roles(r.Roles, hidden || r.VolunteersHidden, editor),
			Volunteers: v.volunteers(r.Volunteers, hidden || r.VolunteersHidden, editor),
			Taken:      len(r.Volunteers),
		})
	}
	return out
}

// Render is the model as one signed-in person sees it.
func Render(model *Model, directory Directory, email string, admin bool, now time.Time) View {
	v := viewer{email: email, admin: admin, directory: directory}
	name, _ := v.person(email)
	current := SchoolYear(now)
	view := View{
		User:       User{Email: email, Name: name, Initial: strings.ToUpper(name[:1]), IsAdmin: admin},
		Years:      Years{Current: current, Last: ShiftYear(current, -1), Next: ShiftYear(current, 1)},
		Settings:   model.Settings,
		Categories: model.Categories,
		Activities: []ActivityView{},
	}
	for _, a := range model.Activities {
		editor := v.canEdit(a)
		if !v.visible(a.Status, a.AddedBy, editor) {
			continue
		}
		view.Activities = append(view.Activities, ActivityView{
			Activity:   a,
			Roles:      v.roles(a.Roles, a.VolunteersHidden, editor),
			Volunteers: v.volunteers(a.Volunteers, a.VolunteersHidden, editor),
			Taken:      len(a.Volunteers),
			CanEdit:    editor,
		})
	}
	if admin {
		view.People = v.people(model)
	}
	return view
}

// people tallies every volunteer row per person, for the admin's People page.
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
		for _, r := range a.AllRoles() {
			count(r.Volunteers)
		}
	}
	out := make([]PersonView, 0, len(byEmail))
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
