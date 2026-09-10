package birthday

import (
	"sort"
	"strings"
	"time"
)

// Person is what the directory knows about someone the app names.
type Person struct {
	Email      string `json:"email"`
	Name       string `json:"name"`
	PhotoURL   string `json:"photoUrl,omitempty"`
	JobTitle   string `json:"jobTitle,omitempty"`
	Department string `json:"department,omitempty"`
}

// Directory is what the app asks of the school directory: who a signed-in
// address really is, who someone is, every staff member, and the order
// departments are listed in.
type Directory interface {
	Resolve(email string) string
	Person(email string) (Person, bool)
	Staff() []Person
	Departments() []string
}

// displayName reads a name out of an address for someone the directory does not
// list, so a list never shows a bare email.
func displayName(email string) string {
	local, _, _ := strings.Cut(email, "@")
	words := strings.FieldsFunc(local, func(r rune) bool { return r == '.' || r == '_' || r == '-' })
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

type viewer struct {
	directory Directory
}

// person looks someone up through the directory's aliases, so a row recorded
// under a secondary address still finds them.
func (v viewer) person(email string) (Person, bool) {
	if p, ok := v.directory.Person(v.directory.Resolve(email)); ok {
		return p, true
	}
	return Person{Email: email, Name: displayName(email)}, false
}

// StaffView is one staff member's birthday this year with everything derived
// from it and everything recorded about it.
type StaffView struct {
	Person
	InDirectory      bool      `json:"inDirectory"`
	Year             string    `json:"year"`
	Birthday         string    `json:"birthday,omitempty"`
	BirthdayThisYear string    `json:"birthdayThisYear,omitempty"`
	NewsletterDate   string    `json:"newsletterDate,omitempty"`
	RequestBy        string    `json:"requestBy,omitempty"`
	Override         string    `json:"override,omitempty"`
	Level            string    `json:"level,omitempty"`
	LevelNote        string    `json:"levelNote,omitempty"`
	Stage            string    `json:"stage,omitempty"`
	AssignedTo       string    `json:"assignedTo,omitempty"`
	AssignedToName   string    `json:"assignedToName,omitempty"`
	AssignedOn       string    `json:"assignedOn,omitempty"`
	ContactedOn      string    `json:"contactedOn,omitempty"`
	ContactedBy      string    `json:"contactedBy,omitempty"`
	Donation         *Donation `json:"donation,omitempty"`
	LastDonation     *Donation `json:"lastDonation,omitempty"`
	Notes            []Note    `json:"notes"`
}

type YearView struct {
	Current string `json:"current"`
	Last    string `json:"last"`
	Start   string `json:"start"`
	End     string `json:"end"`
}

type User struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Initial  string `json:"initial"`
	PhotoURL string `json:"photoUrl,omitempty"`
	IsAdmin  bool   `json:"isAdmin"`
}

type View struct {
	User            User        `json:"user"`
	Year            YearView    `json:"year"`
	Today           string      `json:"today"`
	Settings        Settings    `json:"settings"`
	Staff           []StaffView `json:"staff"`
	Skipped         []StaffView `json:"skipped"`
	Missing         []StaffView `json:"missing"`
	Charities       []Charity   `json:"charities"`
	NewsletterDates []string    `json:"newsletterDates"`
	Departments     []string    `json:"departments"`
}

func dateCell(t time.Time) string {
	return t.Format(DateFormat)
}

func (v viewer) staff(model *Model, b *Birthday, year Year, today time.Time) StaffView {
	person, listed := v.person(b.Email)
	// The row's own address stays the key every write uses, whatever the
	// directory calls the person.
	person.Email = b.Email
	sv := StaffView{Person: person, InDirectory: listed, Year: year.Label, Birthday: b.Birthday, Override: b.Override, Level: b.Level, LevelNote: b.Note, Notes: []Note{}}
	if b.Birthday == "" {
		return sv
	}
	birthday, _ := ParseDate(b.Birthday)
	occurrence := year.Occurrence(birthday)
	sv.BirthdayThisYear = dateCell(occurrence)
	newsletter, hasNewsletter := year.Newsletter(occurrence, model.NewsletterDates)
	if b.Override != "" {
		newsletter, _ = ParseDate(b.Override)
		hasNewsletter = true
	}
	if hasNewsletter {
		sv.NewsletterDate = dateCell(newsletter)
		sv.RequestBy = dateCell(RequestBy(newsletter))
	}
	if a, ok := model.Assignment(b.Email, year.Label); ok {
		to, _ := v.person(a.AssignedTo)
		sv.AssignedTo, sv.AssignedToName, sv.AssignedOn = a.AssignedTo, to.Name, a.AssignedOn
	}
	contacted := false
	if o, ok := model.OutreachFor(b.Email, year.Label); ok {
		contacted = true
		sv.ContactedOn, sv.ContactedBy = o.ContactedOn, o.ContactedBy
	}
	donated, used := false, false
	if d, ok := model.Donation(b.Email, year.Label); ok {
		donated, used = true, d.UsedOn != ""
		sv.Donation = &d
	}
	if d, ok := model.Donation(b.Email, ShiftYear(year.Label, -1)); ok {
		sv.LastDonation = &d
	}
	sv.Stage = Stage(sv.Level, contacted, donated, used, RequestBy(newsletter), hasNewsletter, today)
	for _, n := range model.Notes {
		if n.Email == b.Email {
			sv.Notes = append(sv.Notes, n)
		}
	}
	return sv
}

// Render is the model as one signed-in person sees it.
func Render(model *Model, directory Directory, email string, admin bool, now time.Time) View {
	v := viewer{directory: directory}
	me, _ := v.person(email)
	month, day, _ := ParseMonthDay(model.Settings.YearStart)
	year := YearContaining(now, month, day)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	view := View{
		User:            User{Email: email, Name: me.Name, Initial: strings.ToUpper(me.Name[:1]), PhotoURL: me.PhotoURL, IsAdmin: admin},
		Year:            YearView{Current: year.Label, Last: ShiftYear(year.Label, -1), Start: dateCell(year.Start), End: dateCell(year.End.AddDate(0, 0, -1))},
		Today:           dateCell(today),
		Settings:        model.Settings,
		Staff:           []StaffView{},
		Skipped:         []StaffView{},
		Missing:         []StaffView{},
		Charities:       model.Charities,
		NewsletterDates: model.NewsletterDates,
		Departments:     directory.Departments(),
	}
	// Someone the directory no longer lists has left the school: their row
	// keeps its history but they are nobody's job.
	recorded := map[string]bool{}
	for i := range model.Birthdays {
		b := &model.Birthdays[i]
		recorded[directory.Resolve(b.Email)] = true
		sv := v.staff(model, b, year, today)
		if !sv.InDirectory {
			continue
		}
		if sv.Level == LevelSkip {
			view.Skipped = append(view.Skipped, sv)
			continue
		}
		view.Staff = append(view.Staff, sv)
	}
	sort.SliceStable(view.Staff, func(i, j int) bool {
		if view.Staff[i].BirthdayThisYear != view.Staff[j].BirthdayThisYear {
			return view.Staff[i].BirthdayThisYear < view.Staff[j].BirthdayThisYear
		}
		return view.Staff[i].Name < view.Staff[j].Name
	})
	sort.SliceStable(view.Skipped, func(i, j int) bool { return view.Skipped[i].Name < view.Skipped[j].Name })
	for _, p := range directory.Staff() {
		if recorded[p.Email] {
			continue
		}
		view.Missing = append(view.Missing, StaffView{Person: p, InDirectory: true, Year: year.Label, Notes: []Note{}})
	}
	sort.SliceStable(view.Missing, func(i, j int) bool { return view.Missing[i].Name < view.Missing[j].Name })
	return view
}
