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
	// Grade is a student's grade, as the directory writes it; "" for anyone else.
	Grade(email string) string
	// GradeColors is Helios Who?'s colour for each grade, so a grade reads the
	// same here as there.
	GradeColors() map[string]string
	// Parents is a student's parents' addresses, copied on the student's mail;
	// empty for anyone else.
	Parents(email string) []string
	// People lists everyone a picker may offer, in the directory's own order.
	People() []DirectoryPerson
}

// DirectoryPerson is one row of a people picker: enough to recognise someone -
// face, name, and the one word that places them (a grade, a job, "Parent").
type DirectoryPerson struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	PhotoURL string `json:"photoUrl,omitempty"`
	Title    string `json:"title,omitempty"`
	// A student's parents' addresses, so a list of volunteers can reach the
	// grown-up behind a child's sign-up. Empty for anyone who is not a student.
	IsStudent    bool     `json:"isStudent,omitempty"`
	ParentEmails []string `json:"parentEmails,omitempty"`
	// The rest fills the card that opens from a person's chip: how to reach
	// them (as the directory would show it, so a masked phone stays blank),
	// what places them, and for a parent their children.
	Pronouns   string  `json:"pronouns,omitempty"`
	Phone      string  `json:"phone,omitempty"`
	Grade      string  `json:"grade,omitempty"`
	Classroom  string  `json:"classroom,omitempty"`
	JobTitle   string  `json:"jobTitle,omitempty"`
	Department string  `json:"department,omitempty"`
	Children   []Child `json:"children,omitempty"`
	// Spouses are the other adults of the household, by address.
	Spouses []Child `json:"spouses,omitempty"`
}

// A Child is one of a parent's children as the card lists them - or another
// adult of the household: an address to open their own card, a name, and for
// a child the grade they are in.
type Child struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Grade string `json:"grade,omitempty"`
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
	Children   []*ActivityView `json:"children"`
	Volunteers []Volunteer     `json:"volunteers"`
	Taken      int             `json:"taken"`
	CanEdit    bool            `json:"canEdit"`
	// Runs says the viewer is a co-chair of this or something above it - what
	// CanEdit means for them without their admin hat, which the page can take
	// off (Super Edit Mode).
	Runs bool `json:"runs,omitempty"`
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
	// Redirects let the client send an old friendly address to where the thing
	// is now, without a round trip.
	Redirects []Redirect `json:"redirects"`
	// ImageSearch says the server can search for a picture, and ImageSources
	// lists where it can look, first first, for the picker.
	ImageSearch  bool     `json:"imageSearch"`
	ImageSources []string `json:"imageSources,omitempty"`
	// GradeColors colours the grade badges as Helios Who? does.
	GradeColors map[string]string `json:"gradeColors,omitempty"`
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
		vol.Grade = v.directory.Grade(vol.Email)
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

// children renders the tree under an activity. An editor of the root edits the
// whole tree, so canEdit is inherited rather than recomputed from co-chairs at
// each level - a co-chair of a child still edits that child, because they are a
// co-chair there. Whether a volunteer list is private is each thing's own
// switch and nothing more: an event that hides its list does not hide its
// committees'.
func (v viewer) children(list []*Activity, editor, runs bool) []*ActivityView {
	out := []*ActivityView{}
	for _, c := range list {
		own := editor || v.canEdit(c)
		if !v.visible(c.Status, c.AddedBy, own) {
			continue
		}
		chairs := runs || c.IsCoChair(v.email)
		out = append(out, &ActivityView{
			Activity:   c,
			Children:   v.children(c.Children, own, chairs),
			Volunteers: v.volunteers(c.Volunteers, c.VolunteersHidden, own),
			Taken:      len(c.Volunteers),
			CanEdit:    own,
			Runs:       chairs,
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
		User:        User{Email: email, Name: name, Initial: strings.ToUpper(name[:1]), IsAdmin: admin},
		Years:       Years{Current: current, Last: ShiftYear(current, -1), Next: ShiftYear(current, 1)},
		Settings:    model.Settings,
		Categories:  model.Categories,
		Activities:  []ActivityView{},
		Redirects:   model.Redirects,
		GradeColors: directory.GradeColors(),
	}
	for _, a := range model.Activities {
		editor := v.canEdit(a)
		if !v.visible(a.Status, a.AddedBy, editor) {
			continue
		}
		chairs := a.IsCoChair(v.email)
		view.Activities = append(view.Activities, ActivityView{
			Activity:   a,
			Children:   v.children(a.Children, editor, chairs),
			Volunteers: v.volunteers(a.Volunteers, a.VolunteersHidden, editor),
			Taken:      len(a.Volunteers),
			CanEdit:    editor,
			Runs:       chairs,
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
		for _, c := range a.Descendants() {
			count(c.Volunteers)
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
