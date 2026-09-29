package model

import (
	"context"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/api"
	"heliosian/internal/id"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const birthdaysHost = "birthday"

type BirthdaysWorld struct {
	Model     *Birthdays
	Directory *Directory
	emails    map[string]string
	keys      map[string]string
	order     []string
}

func NewBirthdaysWorld(m *Birthdays, d *Directory) BirthdaysWorld {
	w := BirthdaysWorld{Model: m, Directory: d, emails: map[string]string{}, keys: map[string]string{}}
	add := func(p *Person, email string) {
		key := m.birthdayID(p.Email)
		if _, ok := w.emails[key]; ok {
			return
		}
		w.emails[key], w.keys[email] = email, key
		w.order = append(w.order, key)
	}
	for _, b := range m.Birthdays {
		if p := d.Person(d.Resolve(b.Email)); p != nil {
			add(p, b.Email)
		}
	}
	for i := range d.People {
		p := &d.People[i]
		if _, ok := w.emails[m.birthdayID(p.Email)]; p.IsStaff && !ok {
			add(p, p.Email)
		}
	}
	slices.SortStableFunc(w.order, func(a, b string) int {
		return strings.Compare(strings.ToLower(w.name(a)), strings.ToLower(w.name(b)))
	})
	return w
}

func (w BirthdaysWorld) name(key string) string {
	return w.person(w.emails[key]).FullName
}

func (w BirthdaysWorld) person(email string) *Person {
	return w.Directory.Person(w.Directory.Resolve(email))
}

func (w BirthdaysWorld) personID(email string) []string {
	if p := w.person(email); p != nil && p.ID != "" {
		return []string{p.ID}
	}
	return nil
}

func (m *Birthdays) yearOf(now time.Time) BirthdayYear {
	month, day, _ := ParseMonthDay(m.Settings.YearStart)
	return BirthdayYearContaining(now, month, day)
}

func (m *Birthdays) year(now time.Time) string {
	return m.yearOf(now).Label
}

func midnight(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func birthdayDayOf(now time.Time) string {
	return now.Format(DateFormat)
}

func (w BirthdaysWorld) staffOf(email string, now time.Time) StaffView {
	b := w.Model.Birthday(email)
	if b == nil {
		b = &Birthday{Email: email}
	}
	return birthdayViewer{directory: w.Directory}.staff(w.Model, b, w.Model.yearOf(now), midnight(now))
}

func (w BirthdaysWorld) staff(key string, now time.Time) (StaffView, bool) {
	email, ok := w.emails[key]
	if !ok {
		return StaffView{}, false
	}
	return w.staffOf(email, now), true
}

type urgency struct {
	When string `json:"when"`
	Step string `json:"step"`
}

func urgencyOf(sv StaffView, today string) *urgency {
	if sv.Stage == StageComplete {
		return nil
	}
	steps := []struct{ step, day string }{
		{LateInfo, ifElse(sv.Donation == nil, sv.DueBy)},
		{LateOutreach, ifElse(sv.Stage == StageOutreach, sv.RequestBy)},
		{LateNewsletter, ifElse(sv.Stage == StageNewsletter, sv.NewsletterDate)},
	}
	for _, when := range []string{"late", "today"} {
		for _, s := range steps {
			if s.day != "" && ((when == "late" && s.day < today) || (when == "today" && s.day == today)) {
				return &urgency{When: when, Step: s.step}
			}
		}
	}
	return nil
}

func ifElse(keep bool, day string) string {
	if keep {
		return day
	}
	return ""
}

type mine struct {
	Mine bool `json:"mine"`
}

type birthdayResource struct {
	Email            string   `json:"email"`
	Missing          bool     `json:"missing,omitempty"`
	Year             string   `json:"year"`
	Birthday         string   `json:"birthday,omitempty"`
	BirthdayThisYear string   `json:"birthdayThisYear,omitempty"`
	NewsletterDate   string   `json:"newsletterDate,omitempty"`
	RequestBy        string   `json:"requestBy,omitempty"`
	DueBy            string   `json:"dueBy,omitempty"`
	Override         string   `json:"override,omitempty"`
	Level            string   `json:"level,omitempty"`
	LevelNote        string   `json:"levelNote,omitempty"`
	Stage            string   `json:"stage,omitempty"`
	Urgency          *urgency `json:"urgency,omitempty"`
	Assigned         bool     `json:"assigned"`
	AssignedTo       string   `json:"assignedTo,omitempty"`
	AssignedOn       string   `json:"assignedOn,omitempty"`
	ContactedOn      string   `json:"contactedOn,omitempty"`
	ContactedBy      string   `json:"contactedBy,omitempty"`
	Path             string   `json:"path"`
	App              string   `json:"app"`
	Me               mine     `json:"me"`
}

type donationResource struct {
	Year       string `json:"year"`
	Charity    string `json:"charity"`
	Note       string `json:"note,omitempty"`
	RecordedOn string `json:"recordedOn"`
	RecordedBy string `json:"recordedBy,omitempty"`
	UsedOn     string `json:"usedOn,omitempty"`
	UsedBy     string `json:"usedBy,omitempty"`
}

type noteResource struct {
	Note    string `json:"note"`
	AddedBy string `json:"addedBy"`
	Added   string `json:"added"`
}

type charityResource struct {
	Charity
	Path string `json:"path"`
	App  string `json:"app"`
}

type newsletterDateResource struct {
	Date string `json:"date"`
	Path string `json:"path"`
	App  string `json:"app"`
}

type teamResource struct {
	Email string `json:"email,omitempty"`
	Role  string `json:"role"`
	Me    mine   `json:"me"`
}

type birthdayStanding struct {
	Team      bool `json:"team"`
	Volunteer bool `json:"volunteer"`
	Comms     bool `json:"comms"`
	CommsOnly bool `json:"commsOnly"`
}

type yearResource struct {
	Current string `json:"current"`
	Last    string `json:"last"`
	Start   string `json:"start"`
	End     string `json:"end"`
}

type birthdaySettingsResource struct {
	Settings *BirthdaysSettings `json:"settings,omitempty"`
	Year     yearResource       `json:"year"`
	Me       birthdayStanding   `json:"me"`
}

type inviteResource struct {
	Year        string `json:"year"`
	RequestedOn string `json:"requestedOn"`
	RequestedBy string `json:"requestedBy"`
	SentTo      string `json:"sentTo,omitempty"`
	AskDay      string `json:"askDay,omitempty"`
	SentOn      string `json:"sentOn,omitempty"`
}

func CharityPath(name string) string {
	return "/charities/" + url.PathEscape(name)
}

func NewsletterPath(date string) string {
	return "/newsletters/" + date
}

type birthdayResources struct {
	cache    *BirthdaysCache
	joinHome func(ctx context.Context, email string) error
}

func BirthdayResources(c *BirthdaysCache, joinHome func(ctx context.Context, email string) error) []api.Type[BirthdaysWorld] {
	r := birthdayResources{cache: c, joinHome: joinHome}
	return []api.Type[BirthdaysWorld]{r.birthdays(), r.donations(), r.notes(), r.charities(), r.newsletterDates(), r.team(), r.settings(), r.invites()}
}

func (r birthdayResources) stage(wr api.Write[BirthdaysWorld], ops []store.Op, err error) error {
	if err != nil {
		return err
	}
	return r.cache.Stage(wr.Tx, ops...)
}

func birthdayLogAfter(wr api.Write[BirthdaysWorld], msg string, args ...any) {
	actor := wr.Query.Actor.Email
	ctx := wr.Request.Context()
	wr.Tx.After(func() { slog.InfoContext(ctx, msg, append([]any{"actor", actor}, args...)...) })
}

func sees(w BirthdaysWorld, q api.Query) bool {
	return w.Model.Sees(q.Actor)
}

func birthdayCan(err error) bool {
	return err == nil
}

func (r birthdayResources) birthdays() api.Type[BirthdaysWorld] {
	email := func(w BirthdaysWorld, key string) string { return w.emails[key] }
	return api.Type[BirthdaysWorld]{
		Name:  "birthdays",
		Shape: birthdayResource{},
		Has:   func(w BirthdaysWorld, key string) bool { _, ok := w.emails[key]; return ok },
		Get: func(w BirthdaysWorld, q api.Query, key string) (any, bool) {
			sv, ok := w.staff(key, q.Now)
			if !ok || !sees(w, q) {
				return nil, false
			}
			out := birthdayResource{
				Email: sv.Email, Missing: w.Model.Birthday(sv.Email) == nil, Year: sv.Year, Birthday: sv.Birthday,
				BirthdayThisYear: sv.BirthdayThisYear, NewsletterDate: sv.NewsletterDate, RequestBy: sv.RequestBy, DueBy: sv.DueBy,
				Override: sv.Override, Level: sv.Level, LevelNote: sv.LevelNote, Stage: sv.Stage, Urgency: urgencyOf(sv, birthdayDayOf(q.Now)),
				Assigned: sv.AssignedTo != "", AssignedOn: sv.AssignedOn, ContactedOn: sv.ContactedOn, ContactedBy: sv.ContactedBy,
				Path: staffPath(sv.Email), App: birthdaysHost, Me: mine{Mine: sv.AssignedTo != "" && sv.AssignedTo == q.Actor.Email},
			}
			if sv.AssignedTo != "" && w.personID(sv.AssignedTo) == nil {
				out.AssignedTo = sv.AssignedTo
			}
			return out, true
		},
		List: func(w BirthdaysWorld, q api.Query) []string {
			if !sees(w, q) {
				return []string{}
			}
			return slices.Clone(w.order)
		},
		Aliases: func(w BirthdaysWorld) map[string]string {
			out := map[string]string{}
			for alias, p := range w.Directory.PersonAliases() {
				if key := w.Model.birthdayID(p.Email); w.emails[key] != "" {
					out[alias] = key
				}
			}
			for address, key := range w.keys {
				out[address] = key
			}
			return out
		},
		Create: api.Make(func(wr api.Write[BirthdaysWorld], in struct {
			Email    string `json:"email"`
			Birthday string `json:"birthday"`
			Override string `json:"override"`
			Level    string `json:"level"`
			Note     string `json:"note"`
		}) (string, error) {
			p := wr.S.person(strings.ToLower(strings.TrimSpace(in.Email)))
			if p == nil {
				return "", access.Invalid("%s is not in the directory", in.Email)
			}
			ops, err := wr.S.Model.saveBirthday(wr.Query.Actor, p.Email, in.Birthday, in.Override)
			if in.Level != "" {
				ops, err = wr.S.Model.saveParticipation(wr.Query.Actor, p.Email, in.Level, in.Note)
			}
			birthdayLogAfter(wr, "birthday: added birthday", "email", p.Email, "birthday", in.Birthday, "level", in.Level)
			return wr.S.Model.birthdayID(p.Email), r.stage(wr, ops, err)
		}),
		Relations: map[string]api.Relation[BirthdaysWorld]{
			"person": {Type: "people", List: func(w BirthdaysWorld, _ api.Query, key string) []string { return w.personID(email(w, key)) }},
			"assignee": {Type: "people", List: func(w BirthdaysWorld, q api.Query, key string) []string {
				if a, ok := w.Model.Assignment(email(w, key), w.Model.year(q.Now)); ok {
					return w.personID(a.AssignedTo)
				}
				return nil
			}},
			"donation": {Type: "donations", List: func(w BirthdaysWorld, q api.Query, key string) []string {
				return w.donationIn(email(w, key), w.Model.year(q.Now))
			}},
			"last-donation": {Type: "donations", List: func(w BirthdaysWorld, q api.Query, key string) []string {
				return w.donationIn(email(w, key), ShiftYearSpan(w.Model.year(q.Now), -1))
			}},
			"notes": {Type: "birthday-notes", Many: true, List: func(w BirthdaysWorld, _ api.Query, key string) []string {
				out := []string{}
				for _, n := range w.Model.Notes {
					if n.Email == email(w, key) {
						out = append(out, w.Model.noteID(n))
					}
				}
				return out
			}},
			"invites": {Type: "birthday-invites", Many: true, List: func(w BirthdaysWorld, q api.Query, key string) []string {
				out := []string{}
				for _, inv := range w.Model.invitesFor(email(w, key), w.Model.year(q.Now)) {
					out = append(out, inv.ID)
				}
				return out
			}},
		},
		Actions: map[string]api.Action[BirthdaysWorld]{
			"assign": api.Do(func(w BirthdaysWorld, q api.Query, key string) bool {
				return birthdayCan(w.Model.teamStaff(q.Actor, email(w, key)))
			}, func(wr api.Write[BirthdaysWorld], in struct {
				To string `json:"to"`
			}) error {
				ops, to, err := wr.S.Model.assign(wr.Query.Actor, email(wr.S, wr.ID), in.To, r.cache.IsAdmin, wr.Query.Now)
				birthdayLogAfter(wr, "birthday: assigned", "email", email(wr.S, wr.ID), "to", to)
				return r.stage(wr, ops, err)
			}),
			"unassign": api.Do(func(w BirthdaysWorld, q api.Query, key string) bool {
				return birthdayCan(w.Model.canUnassign(q.Actor, email(w, key), q.Now))
			}, func(wr api.Write[BirthdaysWorld], _ serve.None) error {
				ops, err := wr.S.Model.unassign(wr.Query.Actor, email(wr.S, wr.ID), wr.Query.Now)
				birthdayLogAfter(wr, "birthday: unassigned", "email", email(wr.S, wr.ID))
				return r.stage(wr, ops, err)
			}),
			"contact":   r.outreachAction(true),
			"uncontact": r.outreachAction(false),
			"donate": api.Do(func(w BirthdaysWorld, q api.Query, key string) bool {
				return birthdayCan(w.Model.teamStaff(q.Actor, email(w, key)))
			}, func(wr api.Write[BirthdaysWorld], in struct {
				Charity string `json:"charity"`
				Note    string `json:"note"`
			}) error {
				ops, charity, err := wr.S.Model.saveDonation(wr.Query.Actor, email(wr.S, wr.ID), in.Charity, in.Note, wr.Query.Now)
				if err == nil {
					birthdayLogAfter(wr, "birthday: saved donation", "email", email(wr.S, wr.ID), "charity", charity.Name)
				}
				return r.stage(wr, ops, err)
			}),
			"set": api.Do(func(w BirthdaysWorld, q api.Query, _ string) bool { return birthdayCan(w.Model.requireTeam(q.Actor)) }, func(wr api.Write[BirthdaysWorld], in struct {
				Birthday string `json:"birthday"`
				Override string `json:"override"`
			}) error {
				ops, err := wr.S.Model.saveBirthday(wr.Query.Actor, email(wr.S, wr.ID), in.Birthday, in.Override)
				birthdayLogAfter(wr, "birthday: saved birthday", "email", email(wr.S, wr.ID), "birthday", in.Birthday, "override", in.Override)
				return r.stage(wr, ops, err)
			}),
			"delete": api.Do(func(w BirthdaysWorld, q api.Query, key string) bool {
				return birthdayCan(w.Model.canDeleteBirthday(q.Actor, email(w, key)))
			}, func(wr api.Write[BirthdaysWorld], _ serve.None) error {
				ops, err := wr.S.Model.deleteBirthday(wr.Query.Actor, email(wr.S, wr.ID))
				birthdayLogAfter(wr, "birthday: removed birthday", "email", email(wr.S, wr.ID))
				return r.stage(wr, ops, err)
			}),
			"participation": api.Do(func(w BirthdaysWorld, q api.Query, _ string) bool { return birthdayCan(w.Model.requireTeam(q.Actor)) }, func(wr api.Write[BirthdaysWorld], in struct {
				Level string `json:"level"`
				Note  string `json:"note"`
			}) error {
				ops, err := wr.S.Model.saveParticipation(wr.Query.Actor, email(wr.S, wr.ID), in.Level, in.Note)
				birthdayLogAfter(wr, "birthday: saved participation", "email", email(wr.S, wr.ID), "level", in.Level)
				return r.stage(wr, ops, err)
			}),
			"clear-participation": api.Do(func(w BirthdaysWorld, q api.Query, key string) bool {
				return birthdayCan(w.Model.canClearParticipation(q.Actor, email(w, key)))
			}, func(wr api.Write[BirthdaysWorld], _ serve.None) error {
				ops, err := wr.S.Model.deleteParticipation(wr.Query.Actor, email(wr.S, wr.ID))
				birthdayLogAfter(wr, "birthday: removed participation", "email", email(wr.S, wr.ID))
				return r.stage(wr, ops, err)
			}),
			"note": api.Do(func(w BirthdaysWorld, q api.Query, key string) bool {
				return birthdayCan(w.Model.canAddNote(q.Actor, email(w, key)))
			}, func(wr api.Write[BirthdaysWorld], in struct {
				Note string `json:"note"`
			}) error {
				ops, err := wr.S.Model.addNote(wr.Query.Actor, email(wr.S, wr.ID), in.Note, wr.Query.Now)
				birthdayLogAfter(wr, "birthday: added note", "email", email(wr.S, wr.ID))
				return r.stage(wr, ops, err)
			}),
		},
	}
}

func (r birthdayResources) outreachAction(contacted bool) api.Action[BirthdaysWorld] {
	return api.Do(func(w BirthdaysWorld, q api.Query, key string) bool {
		return birthdayCan(w.Model.canOutreach(q.Actor, w.emails[key], contacted, q.Now))
	}, func(wr api.Write[BirthdaysWorld], _ serve.None) error {
		ops, err := wr.S.Model.outreach(wr.Query.Actor, wr.S.emails[wr.ID], contacted, wr.Query.Now)
		birthdayLogAfter(wr, "birthday: outreach", "email", wr.S.emails[wr.ID], "contacted", contacted)
		return r.stage(wr, ops, err)
	})
}

func (w BirthdaysWorld) donationIn(email, year string) []string {
	if _, ok := w.Model.Donation(email, year); ok {
		return []string{w.Model.donationID(email, year)}
	}
	return nil
}

func (m *Birthdays) donationByID(key string) (Donation, bool) {
	k, ok := m.donationIDs[key]
	if !ok {
		return Donation{}, false
	}
	return m.Donations[k], true
}

func (r birthdayResources) donations() api.Type[BirthdaysWorld] {
	used := func(on bool) api.Action[BirthdaysWorld] {
		return api.Do(func(w BirthdaysWorld, q api.Query, key string) bool {
			d, _ := w.Model.donationByID(key)
			return birthdayCan(w.Model.canMarkUsed(q.Actor, d, on))
		}, func(wr api.Write[BirthdaysWorld], _ serve.None) error {
			d, _ := wr.S.Model.donationByID(wr.ID)
			ops, err := wr.S.Model.markUsed(wr.Query.Actor, d, on, wr.Query.Now)
			birthdayLogAfter(wr, "birthday: marked donation", "used", on, "email", d.Email, "year", d.Year)
			return r.stage(wr, ops, err)
		})
	}
	return api.Type[BirthdaysWorld]{
		Name:  "donations",
		Shape: donationResource{},
		Has:   func(w BirthdaysWorld, key string) bool { _, ok := w.Model.donationByID(key); return ok },
		Get: func(w BirthdaysWorld, q api.Query, key string) (any, bool) {
			d, ok := w.Model.donationByID(key)
			if !ok || !sees(w, q) {
				return nil, false
			}
			return donationResource{Year: d.Year, Charity: d.Charity, Note: d.Note, RecordedOn: d.RecordedOn, RecordedBy: d.RecordedBy, UsedOn: d.UsedOn, UsedBy: d.UsedBy}, true
		},
		List: func(w BirthdaysWorld, q api.Query) []string {
			if !sees(w, q) {
				return []string{}
			}
			out := []string{}
			for key := range w.Model.donationIDs {
				out = append(out, key)
			}
			slices.Sort(out)
			return out
		},
		Relations: map[string]api.Relation[BirthdaysWorld]{
			"charity": {Type: "charities", List: func(w BirthdaysWorld, _ api.Query, key string) []string {
				d, _ := w.Model.donationByID(key)
				return []string{d.Charity}
			}},
		},
		Actions: map[string]api.Action[BirthdaysWorld]{
			"use":   used(true),
			"unuse": used(false),
			"delete": api.Do(func(w BirthdaysWorld, q api.Query, key string) bool {
				d, _ := w.Model.donationByID(key)
				return birthdayCan(w.Model.canChangeDonation(q.Actor, d))
			}, func(wr api.Write[BirthdaysWorld], _ serve.None) error {
				d, _ := wr.S.Model.donationByID(wr.ID)
				ops, err := wr.S.Model.deleteDonation(wr.Query.Actor, d)
				birthdayLogAfter(wr, "birthday: removed donation", "email", d.Email, "year", d.Year)
				return r.stage(wr, ops, err)
			}),
		},
	}
}

func (m *Birthdays) noteByID(key string) (BirthdayNote, bool) {
	i, ok := m.noteIDs[key]
	if !ok {
		return BirthdayNote{}, false
	}
	return m.Notes[i], true
}

func (r birthdayResources) notes() api.Type[BirthdaysWorld] {
	return api.Type[BirthdaysWorld]{
		Name:  "birthday-notes",
		Shape: noteResource{},
		Has:   func(w BirthdaysWorld, key string) bool { _, ok := w.Model.noteByID(key); return ok },
		Get: func(w BirthdaysWorld, q api.Query, key string) (any, bool) {
			n, ok := w.Model.noteByID(key)
			if !ok || !sees(w, q) {
				return nil, false
			}
			return noteResource{Note: n.Note, AddedBy: n.AddedBy, Added: n.Added}, true
		},
		List: func(w BirthdaysWorld, q api.Query) []string {
			if !sees(w, q) {
				return []string{}
			}
			out := []string{}
			for _, n := range w.Model.Notes {
				out = append(out, w.Model.noteID(n))
			}
			return out
		},
		Relations: map[string]api.Relation[BirthdaysWorld]{
			"author": {Type: "people", List: func(w BirthdaysWorld, _ api.Query, key string) []string {
				n, _ := w.Model.noteByID(key)
				return w.personID(n.AddedBy)
			}},
		},
		Actions: map[string]api.Action[BirthdaysWorld]{
			"delete": api.Do(func(w BirthdaysWorld, q api.Query, key string) bool {
				n, _ := w.Model.noteByID(key)
				return birthdayCan(w.Model.canDeleteNote(q.Actor, n))
			}, func(wr api.Write[BirthdaysWorld], _ serve.None) error {
				n, _ := wr.S.Model.noteByID(wr.ID)
				ops, err := wr.S.Model.deleteNote(wr.Query.Actor, n)
				birthdayLogAfter(wr, "birthday: removed note", "email", n.Email)
				return r.stage(wr, ops, err)
			}),
		},
	}
}

func (r birthdayResources) charities() api.Type[BirthdaysWorld] {
	return api.Type[BirthdaysWorld]{
		Name:  "charities",
		Shape: charityResource{},
		Has:   func(w BirthdaysWorld, key string) bool { return w.Model.Charity(key) != nil },
		Get: func(w BirthdaysWorld, q api.Query, key string) (any, bool) {
			c := w.Model.Charity(key)
			if c == nil || !sees(w, q) {
				return nil, false
			}
			return charityResource{Charity: *c, Path: CharityPath(c.Name), App: birthdaysHost}, true
		},
		List: func(w BirthdaysWorld, q api.Query) []string {
			if !sees(w, q) {
				return []string{}
			}
			out := []string{}
			for _, c := range w.Model.Charities {
				out = append(out, c.ID)
			}
			return out
		},
		Aliases: func(w BirthdaysWorld) map[string]string {
			out := map[string]string{}
			for _, c := range w.Model.Charities {
				out[c.Name] = c.ID
			}
			return out
		},
		Create: api.Make(func(wr api.Write[BirthdaysWorld], in charityEdit) (string, error) {
			key := id.New(wr.Taken)
			ops, err := wr.S.Model.addCharity(wr.Query.Actor, in, key, wr.Query.Now)
			birthdayLogAfter(wr, "birthday: added charity", "id", key, "charity", in.Name)
			return key, r.stage(wr, ops, err)
		}),
		Actions: map[string]api.Action[BirthdaysWorld]{
			"edit": api.Do(func(w BirthdaysWorld, q api.Query, _ string) bool { return birthdayCan(w.Model.requireTeam(q.Actor)) }, func(wr api.Write[BirthdaysWorld], in charityEdit) error {
				ops, err := wr.S.Model.editCharity(wr.Query.Actor, wr.S.Model.Charity(wr.ID), in)
				birthdayLogAfter(wr, "birthday: edited charity", "id", wr.ID, "charity", in.Name)
				return r.stage(wr, ops, err)
			}),
			"allow": api.Do(func(_ BirthdaysWorld, q api.Query, _ string) bool { return birthdayCan(requireAdmin(q.Actor)) }, func(wr api.Write[BirthdaysWorld], in charityAllowance) error {
				ops, err := allowCharity(wr.Query.Actor, wr.S.Model.Charity(wr.ID), in)
				birthdayLogAfter(wr, "birthday: allowed charity", "id", wr.ID, "allowed", in.Allowed)
				return r.stage(wr, ops, err)
			}),
			"delete": api.Do(func(w BirthdaysWorld, q api.Query, key string) bool {
				return birthdayCan(w.Model.canDeleteCharity(q.Actor, w.Model.Charity(key)))
			}, func(wr api.Write[BirthdaysWorld], _ serve.None) error {
				ops, err := wr.S.Model.deleteCharity(wr.Query.Actor, wr.S.Model.Charity(wr.ID))
				birthdayLogAfter(wr, "birthday: removed charity", "id", wr.ID)
				return r.stage(wr, ops, err)
			}),
		},
	}
}

type newsletterDateBody struct {
	Date string `json:"date"`
}

func (r birthdayResources) newsletterDates() api.Type[BirthdaysWorld] {
	return api.Type[BirthdaysWorld]{
		Name:  "newsletter-dates",
		Shape: newsletterDateResource{},
		Has:   func(w BirthdaysWorld, key string) bool { return w.Model.NewsletterDate(key) != nil },
		Get: func(w BirthdaysWorld, q api.Query, key string) (any, bool) {
			n := w.Model.NewsletterDate(key)
			if n == nil || !sees(w, q) {
				return nil, false
			}
			return newsletterDateResource{Date: n.Date, Path: NewsletterPath(n.Date), App: birthdaysHost}, true
		},
		List: func(w BirthdaysWorld, q api.Query) []string {
			if !sees(w, q) {
				return []string{}
			}
			out := []string{}
			for _, n := range w.Model.NewsletterDates {
				out = append(out, n.ID)
			}
			return out
		},
		Aliases: func(w BirthdaysWorld) map[string]string {
			out := map[string]string{}
			for _, n := range w.Model.NewsletterDates {
				out[n.Date] = n.ID
			}
			return out
		},
		Create: api.Make(func(wr api.Write[BirthdaysWorld], in newsletterDateBody) (string, error) {
			key := id.New(wr.Taken)
			ops, date, err := wr.S.Model.addNewsletterDate(wr.Query.Actor, in.Date, key)
			birthdayLogAfter(wr, "birthday: added newsletter date", "id", key, "date", date)
			return key, r.stage(wr, ops, err)
		}),
		Actions: map[string]api.Action[BirthdaysWorld]{
			"move": api.Do(func(_ BirthdaysWorld, q api.Query, _ string) bool { return birthdayCan(requireAdmin(q.Actor)) }, func(wr api.Write[BirthdaysWorld], in newsletterDateBody) error {
				n := wr.S.Model.NewsletterDate(wr.ID)
				birthdayLogAfter(wr, "birthday: moved newsletter date", "id", n.ID, "from", n.Date, "to", in.Date)
				ops, err := wr.S.Model.moveNewsletterDate(wr.Query.Actor, n, in.Date)
				return r.stage(wr, ops, err)
			}),
			"delete": api.Do(func(_ BirthdaysWorld, q api.Query, _ string) bool { return birthdayCan(requireAdmin(q.Actor)) }, func(wr api.Write[BirthdaysWorld], _ serve.None) error {
				n := wr.S.Model.NewsletterDate(wr.ID)
				birthdayLogAfter(wr, "birthday: removed newsletter date", "id", n.ID, "date", n.Date)
				ops, err := deleteNewsletterDate(wr.Query.Actor, n)
				return r.stage(wr, ops, err)
			}),
			"share": api.Do(func(w BirthdaysWorld, q api.Query, key string) bool {
				return birthdayCan(w.Model.requireTeam(q.Actor)) && len(w.toExport(w.Model.NewsletterDate(key).Date, q.Now)) > 0
			}, func(wr api.Write[BirthdaysWorld], _ serve.None) error {
				n := wr.S.Model.NewsletterDate(wr.ID)
				items := wr.S.toExport(n.Date, wr.Query.Now)
				rows, marks, err := wr.S.Model.shareIssue(wr.Query.Actor, items, wr.Query.Now)
				if err != nil {
					return err
				}
				birthdayLogAfter(wr, "birthday: copied to the shared sheet", "issue", n.Date, "copied", len(rows))
				return r.cache.stageExport(wr.Tx, rows, marks)
			}),
		},
	}
}

func (m *Birthdays) teamByID(key string) (TeamMember, bool) {
	i, ok := m.teamIDs[key]
	if !ok {
		return TeamMember{}, false
	}
	return m.Team[i], true
}

func (r birthdayResources) team() api.Type[BirthdaysWorld] {
	return api.Type[BirthdaysWorld]{
		Name:  "birthday-team",
		Shape: teamResource{},
		Has:   func(w BirthdaysWorld, key string) bool { _, ok := w.Model.teamByID(key); return ok },
		Get: func(w BirthdaysWorld, q api.Query, key string) (any, bool) {
			t, ok := w.Model.teamByID(key)
			if !ok || !sees(w, q) {
				return nil, false
			}
			out := teamResource{Role: t.Role, Me: mine{Mine: t.Email == q.Actor.Email}}
			if w.personID(t.Email) == nil {
				out.Email = t.Email
			}
			return out, true
		},
		List: func(w BirthdaysWorld, q api.Query) []string {
			if !sees(w, q) {
				return []string{}
			}
			out := []string{}
			for _, t := range w.Model.Team {
				out = append(out, w.Model.teamID(t))
			}
			return out
		},
		Relations: map[string]api.Relation[BirthdaysWorld]{
			"person": {Type: "people", List: func(w BirthdaysWorld, _ api.Query, key string) []string {
				t, _ := w.Model.teamByID(key)
				return w.personID(t.Email)
			}},
		},
		Create: api.Make(func(wr api.Write[BirthdaysWorld], in TeamMember) (string, error) {
			ops, member, err := wr.S.Model.addTeamMember(wr.Query.Actor, in.Email, in.Role)
			if err != nil {
				return "", err
			}
			birthdayLogAfter(wr, "birthday: added team member", "email", member.Email, "role", member.Role)
			if member.Email == wr.Query.Actor.Email {
				ctx := wr.Request.Context()
				wr.Tx.After(func() {
					if err := r.joinHome(ctx, member.Email); err != nil {
						slog.ErrorContext(ctx, "birthday: put a joiner on the app's list", "error", err, "email", member.Email)
					}
				})
			}
			return wr.S.Model.teamID(member), r.stage(wr, ops, nil)
		}),
		Actions: map[string]api.Action[BirthdaysWorld]{
			"delete": api.Do(func(_ BirthdaysWorld, q api.Query, _ string) bool { return birthdayCan(requireAdmin(q.Actor)) }, func(wr api.Write[BirthdaysWorld], _ serve.None) error {
				t, _ := wr.S.Model.teamByID(wr.ID)
				ops, err := removeTeamMember(wr.Query.Actor, t)
				birthdayLogAfter(wr, "birthday: removed team member", "email", t.Email, "role", t.Role)
				return r.stage(wr, ops, err)
			}),
		},
	}
}

func (m *Birthdays) standing(actor access.Actor) birthdayStanding {
	out := birthdayStanding{Team: m.Sees(actor)}
	for _, t := range m.Team {
		if t.Email != actor.Email {
			continue
		}
		out.Volunteer = out.Volunteer || t.Role == RoleVolunteer
		out.Comms = out.Comms || t.Role == RoleComms
	}
	out.CommsOnly = out.Comms && !out.Volunteer && !actor.May(ConfigureBirthdays)
	return out
}

func (r birthdayResources) settings() api.Type[BirthdaysWorld] {
	return api.Type[BirthdaysWorld]{
		Name:  "birthday-settings",
		Shape: birthdaySettingsResource{},
		Has:   func(w BirthdaysWorld, key string) bool { return key == w.Model.settingsID() },
		Get: func(w BirthdaysWorld, q api.Query, key string) (any, bool) {
			if key != w.Model.settingsID() {
				return nil, false
			}
			year := w.Model.yearOf(q.Now)
			out := birthdaySettingsResource{
				Year: yearResource{Current: year.Label, Last: ShiftYearSpan(year.Label, -1), Start: dateCell(year.Start), End: dateCell(year.End.AddDate(0, 0, -1))},
				Me:   w.Model.standing(q.Actor),
			}
			if sees(w, q) {
				s := w.Model.Settings
				out.Settings = &s
			}
			return out, true
		},
		List: func(w BirthdaysWorld, _ api.Query) []string { return []string{w.Model.settingsID()} },
		Relations: map[string]api.Relation[BirthdaysWorld]{
			"viewer": {Type: "people", List: func(w BirthdaysWorld, q api.Query, _ string) []string { return w.personID(q.Actor.Email) }},
		},
		Actions: map[string]api.Action[BirthdaysWorld]{
			"edit": api.Do(func(_ BirthdaysWorld, q api.Query, _ string) bool { return birthdayCan(requireAdmin(q.Actor)) }, func(wr api.Write[BirthdaysWorld], in BirthdaysSettings) error {
				ops, err := birthdaySettingsOps(wr.Query.Actor, in)
				birthdayLogAfter(wr, "birthday: changed the settings")
				return r.stage(wr, ops, err)
			}),
		},
	}
}

func (m *Birthdays) inviteByID(key string) (BirthdayInvite, bool) {
	i, ok := m.inviteIDs[key]
	if !ok {
		return BirthdayInvite{}, false
	}
	return m.Invites[i], true
}

func (r birthdayResources) invites() api.Type[BirthdaysWorld] {
	return api.Type[BirthdaysWorld]{
		Name:  "birthday-invites",
		Shape: inviteResource{},
		Has:   func(w BirthdaysWorld, key string) bool { _, ok := w.Model.inviteByID(key); return ok },
		Get: func(w BirthdaysWorld, q api.Query, key string) (any, bool) {
			inv, ok := w.Model.inviteByID(key)
			if !ok || !sees(w, q) {
				return nil, false
			}
			return inviteResource{Year: inv.Year, RequestedOn: inv.RequestedOn, RequestedBy: inv.RequestedBy, SentTo: inv.SentTo, AskDay: inv.AskDay, SentOn: inv.SentOn}, true
		},
		List: func(w BirthdaysWorld, q api.Query) []string {
			if !sees(w, q) {
				return []string{}
			}
			out := []string{}
			for _, inv := range w.Model.Invites {
				out = append(out, inv.ID)
			}
			return out
		},
		Create: api.Make(func(wr api.Write[BirthdaysWorld], in struct {
			Birthday string `json:"birthday"`
		}) (string, error) {
			address, ok := wr.S.emails[in.Birthday]
			if !ok {
				return "", access.Missing("no birthday %s", in.Birthday)
			}
			key := id.New(wr.Taken)
			ops, err := wr.S.Model.requestInvite(wr.Query.Actor, address, key, wr.Query.Now)
			birthdayLogAfter(wr, "birthday: invite requested", "id", key, "email", address)
			return key, r.stage(wr, ops, err)
		}),
	}
}
