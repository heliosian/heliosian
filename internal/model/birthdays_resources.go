package model

import (
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/api"
	"heliosian/internal/id"
	"heliosian/internal/serve"
)

const birthdaysHost = "birthday"

func (b *Birthdays) index(d *Directory) {
	b.staffEmails, b.staffKeys = map[string]string{}, map[string]string{}
	add := func(p *Person, email string) {
		key := b.birthdayID(p.Email)
		if _, ok := b.staffEmails[key]; ok {
			return
		}
		b.staffEmails[key], b.staffKeys[email] = email, key
		b.staffOrder = append(b.staffOrder, key)
	}
	for _, bd := range b.Birthdays {
		if p := d.Person(d.Resolve(bd.Email)); p != nil {
			add(p, bd.Email)
		}
	}
	for i := range d.People {
		p := &d.People[i]
		if _, ok := b.staffEmails[b.birthdayID(p.Email)]; p.IsStaff && !ok {
			add(p, p.Email)
		}
	}
	name := func(key string) string {
		return d.Person(d.Resolve(b.staffEmails[key])).FullName
	}
	slices.SortStableFunc(b.staffOrder, func(x, y string) int {
		return strings.Compare(strings.ToLower(name(x)), strings.ToLower(name(y)))
	})
}

func (m *Model) birthdayPerson(email string) *Person {
	return m.Directory.Person(m.Directory.Resolve(email))
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

func (m *Model) staffOf(email string, now time.Time) StaffView {
	b := m.Birthdays.Birthday(email)
	if b == nil {
		b = &Birthday{Email: email}
	}
	return birthdayViewer{directory: m.Directory}.staff(m.Birthdays, b, m.Birthdays.yearOf(now), midnight(now))
}

func (m *Model) birthdayStaff(key string, now time.Time) (StaffView, bool) {
	email, ok := m.Birthdays.staffEmails[key]
	if !ok {
		return StaffView{}, false
	}
	return m.staffOf(email, now), true
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
	Email            string        `json:"email"`
	Missing          bool          `json:"missing,omitempty"`
	Year             string        `json:"year"`
	Birthday         string        `json:"birthday,omitempty"`
	BirthdayThisYear string        `json:"birthdayThisYear,omitempty"`
	NewsletterDate   string        `json:"newsletterDate,omitempty"`
	RequestBy        string        `json:"requestBy,omitempty"`
	DueBy            string        `json:"dueBy,omitempty"`
	Override         string        `json:"override,omitempty"`
	Level            string        `json:"level,omitempty"`
	LevelNote        string        `json:"levelNote,omitempty"`
	Stage            string        `json:"stage,omitempty"`
	Urgency          *urgency      `json:"urgency,omitempty"`
	Next             *birthdayStep `json:"next,omitempty"`
	Late             *late         `json:"late,omitempty"`
	Assigned         bool          `json:"assigned"`
	AssignedTo       string        `json:"assignedTo,omitempty"`
	AssignedOn       string        `json:"assignedOn,omitempty"`
	ContactedOn      string        `json:"contactedOn,omitempty"`
	ContactedBy      string        `json:"contactedBy,omitempty"`
	FallbackNote     string        `json:"fallbackNote,omitempty"`
	Path             string        `json:"path"`
	App              string        `json:"app"`
	Me               mine          `json:"me"`
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
	store *Store
}

func BirthdayResources(s *Store) []api.Type[*Model] {
	r := birthdayResources{store: s}
	return []api.Type[*Model]{r.birthdays(), r.donations(), r.notes(), r.charities(), r.newsletterDates(), r.team(), r.settings(), r.invites()}
}

func birthdaySees(m *Model, q api.Query) bool {
	return m.Birthdays.Sees(q.Actor)
}

func (r birthdayResources) birthdays() api.Type[*Model] {
	email := func(m *Model, key string) string { return m.Birthdays.staffEmails[key] }
	return api.Type[*Model]{
		Name:  "birthdays",
		Shape: birthdayResource{},
		Has:   func(m *Model, key string) bool { _, ok := m.Birthdays.staffEmails[key]; return ok },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			sv, ok := m.birthdayStaff(key, q.Now)
			if !ok || !birthdaySees(m, q) {
				return nil, false
			}
			out := birthdayResource{
				Email: sv.Email, Missing: m.Birthdays.Birthday(sv.Email) == nil, Year: sv.Year, Birthday: sv.Birthday,
				BirthdayThisYear: sv.BirthdayThisYear, NewsletterDate: sv.NewsletterDate, RequestBy: sv.RequestBy, DueBy: sv.DueBy,
				Override: sv.Override, Level: sv.Level, LevelNote: sv.LevelNote, Stage: sv.Stage, Urgency: urgencyOf(sv, birthdayDayOf(q.Now)), Next: nextStep(sv),
				Assigned: sv.AssignedTo != "", AssignedOn: sv.AssignedOn, ContactedOn: sv.ContactedOn, ContactedBy: sv.ContactedBy,
				Path: staffPath(sv.Email), App: birthdaysHost, Me: mine{Mine: sv.AssignedTo != "" && sv.AssignedTo == q.Actor.Email},
				Late: m.lateFor(q, sv),
			}
			_, out.FallbackNote = m.Birthdays.fallback(sv.Email, sv.Year)
			if sv.AssignedTo != "" && m.personID(sv.AssignedTo) == nil {
				out.AssignedTo = sv.AssignedTo
			}
			return out, true
		},
		List: func(m *Model, q api.Query) []string {
			if !birthdaySees(m, q) {
				return []string{}
			}
			return slices.Clone(m.Birthdays.staffOrder)
		},
		Aliases: func(m *Model) map[string]string {
			out := map[string]string{}
			for alias, p := range m.Directory.PersonAliases() {
				if key := m.Birthdays.birthdayID(p.Email); m.Birthdays.staffEmails[key] != "" {
					out[alias] = key
				}
			}
			for address, key := range m.Birthdays.staffKeys {
				out[address] = key
			}
			return out
		},
		Filters: map[string]api.Filter[*Model]{
			"late": func(m *Model, q api.Query, value string) (func(string) bool, error) {
				if value != "" && value != "true" {
					return nil, access.Invalid("late takes no value")
				}
				return func(key string) bool {
					sv, ok := m.birthdayStaff(key, q.Now)
					return ok && m.lateFor(q, sv) != nil
				}, nil
			},
		},
		Create: api.Make(func(wr api.Write[*Model], in struct {
			Email    string `json:"email"`
			Birthday string `json:"birthday"`
			Override string `json:"override"`
			Level    string `json:"level"`
			Note     string `json:"note"`
		}) (string, error) {
			p := wr.S.birthdayPerson(strings.ToLower(strings.TrimSpace(in.Email)))
			if p == nil {
				return "", access.Invalid("%s is not in the directory", in.Email)
			}
			ops, err := wr.S.Birthdays.saveBirthday(wr.Query.Actor, p.Email, in.Birthday, in.Override)
			if in.Level != "" {
				ops, err = wr.S.Birthdays.saveParticipation(wr.Query.Actor, p.Email, in.Level, in.Note)
			}
			logAfter(wr, "birthday: added birthday", "email", p.Email, "birthday", in.Birthday, "level", in.Level)
			return wr.S.Birthdays.birthdayID(p.Email), r.store.stage(wr, birthdaysAppName, ops, err)
		}),
		Relations: map[string]api.Relation[*Model]{
			"person": {Type: "people", List: func(m *Model, _ api.Query, key string) []string { return m.personID(email(m, key)) }},
			"assignee": {Type: "people", List: func(m *Model, q api.Query, key string) []string {
				if a, ok := m.Birthdays.Assignment(email(m, key), m.Birthdays.year(q.Now)); ok {
					return m.personID(a.AssignedTo)
				}
				return nil
			}},
			"donation": {Type: "donations", List: func(m *Model, q api.Query, key string) []string {
				return m.Birthdays.donationIn(email(m, key), m.Birthdays.year(q.Now))
			}},
			"last-donation": {Type: "donations", List: func(m *Model, q api.Query, key string) []string {
				return m.Birthdays.donationIn(email(m, key), ShiftYearSpan(m.Birthdays.year(q.Now), -1))
			}},
			"fallback-charity": {Type: "charities", List: func(m *Model, q api.Query, key string) []string {
				charity, _ := m.Birthdays.fallback(email(m, key), m.Birthdays.year(q.Now))
				return []string{charity}
			}},
			"notes": {Type: "birthday-notes", Many: true, List: func(m *Model, _ api.Query, key string) []string {
				out := []string{}
				for _, n := range m.Birthdays.Notes {
					if n.Email == email(m, key) {
						out = append(out, m.Birthdays.noteID(n))
					}
				}
				return out
			}},
			"invites": {Type: "birthday-invites", Many: true, List: func(m *Model, q api.Query, key string) []string {
				out := []string{}
				for _, inv := range m.Birthdays.invitesFor(email(m, key), m.Birthdays.year(q.Now)) {
					out = append(out, inv.ID)
				}
				return out
			}},
		},
		Actions: map[string]api.Action[*Model]{
			"assign": api.Do(func(m *Model, q api.Query, key string) bool {
				return permitted(m.Birthdays.teamStaff(q.Actor, email(m, key)))
			}, func(wr api.Write[*Model], in struct {
				To string `json:"to"`
			}) error {
				ops, to, err := wr.S.Birthdays.assign(wr.Query.Actor, email(wr.S, wr.ID), in.To, wr.S.AdminList("birthday").IsAdmin, wr.Query.Now)
				logAfter(wr, "birthday: assigned", "email", email(wr.S, wr.ID), "to", to)
				return r.store.stage(wr, birthdaysAppName, ops, err)
			}),
			"unassign": api.Do(func(m *Model, q api.Query, key string) bool {
				return permitted(m.Birthdays.canUnassign(q.Actor, email(m, key), q.Now))
			}, func(wr api.Write[*Model], _ serve.None) error {
				ops, err := wr.S.Birthdays.unassign(wr.Query.Actor, email(wr.S, wr.ID), wr.Query.Now)
				logAfter(wr, "birthday: unassigned", "email", email(wr.S, wr.ID))
				return r.store.stage(wr, birthdaysAppName, ops, err)
			}),
			"contact":   r.outreachAction(true),
			"uncontact": r.outreachAction(false),
			"donate": api.Do(func(m *Model, q api.Query, key string) bool {
				return permitted(m.Birthdays.teamStaff(q.Actor, email(m, key)))
			}, func(wr api.Write[*Model], in struct {
				Charity string `json:"charity"`
				Note    string `json:"note"`
			}) error {
				ops, charity, err := wr.S.Birthdays.saveDonation(wr.Query.Actor, email(wr.S, wr.ID), in.Charity, in.Note, wr.Query.Now)
				if err == nil {
					logAfter(wr, "birthday: saved donation", "email", email(wr.S, wr.ID), "charity", charity.Name)
				}
				return r.store.stage(wr, birthdaysAppName, ops, err)
			}),
			"set": api.Do(func(m *Model, q api.Query, _ string) bool { return permitted(m.Birthdays.requireTeam(q.Actor)) }, func(wr api.Write[*Model], in struct {
				Birthday string `json:"birthday"`
				Override string `json:"override"`
			}) error {
				ops, err := wr.S.Birthdays.saveBirthday(wr.Query.Actor, email(wr.S, wr.ID), in.Birthday, in.Override)
				logAfter(wr, "birthday: saved birthday", "email", email(wr.S, wr.ID), "birthday", in.Birthday, "override", in.Override)
				return r.store.stage(wr, birthdaysAppName, ops, err)
			}),
			"delete": api.Do(func(m *Model, q api.Query, key string) bool {
				return permitted(m.Birthdays.canDeleteBirthday(q.Actor, email(m, key)))
			}, func(wr api.Write[*Model], _ serve.None) error {
				ops, err := wr.S.Birthdays.deleteBirthday(wr.Query.Actor, email(wr.S, wr.ID))
				logAfter(wr, "birthday: removed birthday", "email", email(wr.S, wr.ID))
				return r.store.stage(wr, birthdaysAppName, ops, err)
			}),
			"participation": api.Do(func(m *Model, q api.Query, _ string) bool { return permitted(m.Birthdays.requireTeam(q.Actor)) }, func(wr api.Write[*Model], in struct {
				Level string `json:"level"`
				Note  string `json:"note"`
			}) error {
				ops, err := wr.S.Birthdays.saveParticipation(wr.Query.Actor, email(wr.S, wr.ID), in.Level, in.Note)
				logAfter(wr, "birthday: saved participation", "email", email(wr.S, wr.ID), "level", in.Level)
				return r.store.stage(wr, birthdaysAppName, ops, err)
			}),
			"clear-participation": api.Do(func(m *Model, q api.Query, key string) bool {
				return permitted(m.Birthdays.canClearParticipation(q.Actor, email(m, key)))
			}, func(wr api.Write[*Model], _ serve.None) error {
				ops, err := wr.S.Birthdays.deleteParticipation(wr.Query.Actor, email(wr.S, wr.ID))
				logAfter(wr, "birthday: removed participation", "email", email(wr.S, wr.ID))
				return r.store.stage(wr, birthdaysAppName, ops, err)
			}),
			"note": api.Do(func(m *Model, q api.Query, key string) bool {
				return permitted(m.Birthdays.canAddNote(q.Actor, email(m, key)))
			}, func(wr api.Write[*Model], in struct {
				Note string `json:"note"`
			}) error {
				ops, err := wr.S.Birthdays.addNote(wr.Query.Actor, email(wr.S, wr.ID), in.Note, wr.Query.Now)
				logAfter(wr, "birthday: added note", "email", email(wr.S, wr.ID))
				return r.store.stage(wr, birthdaysAppName, ops, err)
			}),
		},
	}
}

func (r birthdayResources) outreachAction(contacted bool) api.Action[*Model] {
	return api.Do(func(m *Model, q api.Query, key string) bool {
		return permitted(m.Birthdays.canOutreach(q.Actor, m.Birthdays.staffEmails[key], contacted, q.Now))
	}, func(wr api.Write[*Model], _ serve.None) error {
		ops, err := wr.S.Birthdays.outreach(wr.Query.Actor, wr.S.Birthdays.staffEmails[wr.ID], contacted, wr.Query.Now)
		logAfter(wr, "birthday: outreach", "email", wr.S.Birthdays.staffEmails[wr.ID], "contacted", contacted)
		return r.store.stage(wr, birthdaysAppName, ops, err)
	})
}

func (m *Birthdays) donationIn(email, year string) []string {
	if _, ok := m.Donation(email, year); ok {
		return []string{m.donationID(email, year)}
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

func (r birthdayResources) donations() api.Type[*Model] {
	used := func(on bool) api.Action[*Model] {
		return api.Do(func(m *Model, q api.Query, key string) bool {
			d, _ := m.Birthdays.donationByID(key)
			return permitted(m.Birthdays.canMarkUsed(q.Actor, d, on))
		}, func(wr api.Write[*Model], _ serve.None) error {
			d, _ := wr.S.Birthdays.donationByID(wr.ID)
			ops, err := wr.S.Birthdays.markUsed(wr.Query.Actor, d, on, wr.Query.Now)
			logAfter(wr, "birthday: marked donation", "used", on, "email", d.Email, "year", d.Year)
			return r.store.stage(wr, birthdaysAppName, ops, err)
		})
	}
	return api.Type[*Model]{
		Name:  "donations",
		Shape: donationResource{},
		Has:   func(m *Model, key string) bool { _, ok := m.Birthdays.donationByID(key); return ok },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			d, ok := m.Birthdays.donationByID(key)
			if !ok || !birthdaySees(m, q) {
				return nil, false
			}
			return donationResource{Year: d.Year, Charity: d.Charity, Note: d.Note, RecordedOn: d.RecordedOn, RecordedBy: d.RecordedBy, UsedOn: d.UsedOn, UsedBy: d.UsedBy}, true
		},
		List: func(m *Model, q api.Query) []string {
			if !birthdaySees(m, q) {
				return []string{}
			}
			out := []string{}
			for key := range m.Birthdays.donationIDs {
				out = append(out, key)
			}
			slices.Sort(out)
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"charity": {Type: "charities", List: func(m *Model, _ api.Query, key string) []string {
				d, _ := m.Birthdays.donationByID(key)
				return []string{d.Charity}
			}},
		},
		Actions: map[string]api.Action[*Model]{
			"use":   used(true),
			"unuse": used(false),
			"delete": api.Do(func(m *Model, q api.Query, key string) bool {
				d, _ := m.Birthdays.donationByID(key)
				return permitted(m.Birthdays.canChangeDonation(q.Actor, d))
			}, func(wr api.Write[*Model], _ serve.None) error {
				d, _ := wr.S.Birthdays.donationByID(wr.ID)
				ops, err := wr.S.Birthdays.deleteDonation(wr.Query.Actor, d)
				logAfter(wr, "birthday: removed donation", "email", d.Email, "year", d.Year)
				return r.store.stage(wr, birthdaysAppName, ops, err)
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

func (r birthdayResources) notes() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "birthday-notes",
		Shape: noteResource{},
		Has:   func(m *Model, key string) bool { _, ok := m.Birthdays.noteByID(key); return ok },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			n, ok := m.Birthdays.noteByID(key)
			if !ok || !birthdaySees(m, q) {
				return nil, false
			}
			return noteResource{Note: n.Note, AddedBy: n.AddedBy, Added: n.Added}, true
		},
		List: func(m *Model, q api.Query) []string {
			if !birthdaySees(m, q) {
				return []string{}
			}
			out := []string{}
			for _, n := range m.Birthdays.Notes {
				out = append(out, m.Birthdays.noteID(n))
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"author": {Type: "people", List: func(m *Model, _ api.Query, key string) []string {
				n, _ := m.Birthdays.noteByID(key)
				return m.personID(n.AddedBy)
			}},
		},
		Actions: map[string]api.Action[*Model]{
			"delete": api.Do(func(m *Model, q api.Query, key string) bool {
				n, _ := m.Birthdays.noteByID(key)
				return permitted(m.Birthdays.canDeleteNote(q.Actor, n))
			}, func(wr api.Write[*Model], _ serve.None) error {
				n, _ := wr.S.Birthdays.noteByID(wr.ID)
				ops, err := wr.S.Birthdays.deleteNote(wr.Query.Actor, n)
				logAfter(wr, "birthday: removed note", "email", n.Email)
				return r.store.stage(wr, birthdaysAppName, ops, err)
			}),
		},
	}
}

func (r birthdayResources) charities() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "charities",
		Shape: charityResource{},
		Has:   func(m *Model, key string) bool { return m.Birthdays.Charity(key) != nil },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			c := m.Birthdays.Charity(key)
			if c == nil || !birthdaySees(m, q) {
				return nil, false
			}
			return charityResource{Charity: *c, Path: CharityPath(c.Name), App: birthdaysHost}, true
		},
		List: func(m *Model, q api.Query) []string {
			if !birthdaySees(m, q) {
				return []string{}
			}
			out := []string{}
			for _, c := range m.Birthdays.Charities {
				out = append(out, c.ID)
			}
			return out
		},
		Aliases: func(m *Model) map[string]string {
			out := map[string]string{}
			for _, c := range m.Birthdays.Charities {
				out[c.Name] = c.ID
			}
			return out
		},
		Create: api.Make(func(wr api.Write[*Model], in charityEdit) (string, error) {
			key := id.New(wr.Taken)
			ops, err := wr.S.Birthdays.addCharity(wr.Query.Actor, in, key, wr.Query.Now)
			logAfter(wr, "birthday: added charity", "id", key, "charity", in.Name)
			return key, r.store.stage(wr, birthdaysAppName, ops, err)
		}),
		Actions: map[string]api.Action[*Model]{
			"edit": api.Do(func(m *Model, q api.Query, _ string) bool { return permitted(m.Birthdays.requireTeam(q.Actor)) }, func(wr api.Write[*Model], in charityEdit) error {
				ops, err := wr.S.Birthdays.editCharity(wr.Query.Actor, wr.S.Birthdays.Charity(wr.ID), in)
				logAfter(wr, "birthday: edited charity", "id", wr.ID, "charity", in.Name)
				return r.store.stage(wr, birthdaysAppName, ops, err)
			}),
			"allow": api.Do(func(_ *Model, q api.Query, _ string) bool { return permitted(requireAdmin(q.Actor)) }, func(wr api.Write[*Model], in charityAllowance) error {
				ops, err := allowCharity(wr.Query.Actor, wr.S.Birthdays.Charity(wr.ID), in)
				logAfter(wr, "birthday: allowed charity", "id", wr.ID, "allowed", in.Allowed)
				return r.store.stage(wr, birthdaysAppName, ops, err)
			}),
			"delete": api.Do(func(m *Model, q api.Query, key string) bool {
				return permitted(m.Birthdays.canDeleteCharity(q.Actor, m.Birthdays.Charity(key)))
			}, func(wr api.Write[*Model], _ serve.None) error {
				ops, err := wr.S.Birthdays.deleteCharity(wr.Query.Actor, wr.S.Birthdays.Charity(wr.ID))
				logAfter(wr, "birthday: removed charity", "id", wr.ID)
				return r.store.stage(wr, birthdaysAppName, ops, err)
			}),
		},
	}
}

type newsletterDateBody struct {
	Date string `json:"date"`
}

func (r birthdayResources) newsletterDates() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "newsletter-dates",
		Shape: newsletterDateResource{},
		Has:   func(m *Model, key string) bool { return m.Birthdays.NewsletterDate(key) != nil },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			n := m.Birthdays.NewsletterDate(key)
			if n == nil || !birthdaySees(m, q) {
				return nil, false
			}
			return newsletterDateResource{Date: n.Date, Path: NewsletterPath(n.Date), App: birthdaysHost}, true
		},
		List: func(m *Model, q api.Query) []string {
			if !birthdaySees(m, q) {
				return []string{}
			}
			out := []string{}
			for _, n := range m.Birthdays.NewsletterDates {
				out = append(out, n.ID)
			}
			return out
		},
		Aliases: func(m *Model) map[string]string {
			out := map[string]string{}
			for _, n := range m.Birthdays.NewsletterDates {
				out[n.Date] = n.ID
			}
			return out
		},
		Create: api.Make(func(wr api.Write[*Model], in newsletterDateBody) (string, error) {
			key := id.New(wr.Taken)
			ops, date, err := wr.S.Birthdays.addNewsletterDate(wr.Query.Actor, in.Date, key)
			logAfter(wr, "birthday: added newsletter date", "id", key, "date", date)
			return key, r.store.stage(wr, birthdaysAppName, ops, err)
		}),
		Actions: map[string]api.Action[*Model]{
			"move": api.Do(func(_ *Model, q api.Query, _ string) bool { return permitted(requireAdmin(q.Actor)) }, func(wr api.Write[*Model], in newsletterDateBody) error {
				n := wr.S.Birthdays.NewsletterDate(wr.ID)
				logAfter(wr, "birthday: moved newsletter date", "id", n.ID, "from", n.Date, "to", in.Date)
				ops, err := wr.S.Birthdays.moveNewsletterDate(wr.Query.Actor, n, in.Date)
				return r.store.stage(wr, birthdaysAppName, ops, err)
			}),
			"delete": api.Do(func(_ *Model, q api.Query, _ string) bool { return permitted(requireAdmin(q.Actor)) }, func(wr api.Write[*Model], _ serve.None) error {
				n := wr.S.Birthdays.NewsletterDate(wr.ID)
				logAfter(wr, "birthday: removed newsletter date", "id", n.ID, "date", n.Date)
				ops, err := deleteNewsletterDate(wr.Query.Actor, n)
				return r.store.stage(wr, birthdaysAppName, ops, err)
			}),
			"share": api.Do(func(m *Model, q api.Query, key string) bool {
				return permitted(m.Birthdays.requireTeam(q.Actor)) && len(m.toExport(m.Birthdays.NewsletterDate(key).Date, q.Now)) > 0
			}, func(wr api.Write[*Model], _ serve.None) error {
				n := wr.S.Birthdays.NewsletterDate(wr.ID)
				items := wr.S.toExport(n.Date, wr.Query.Now)
				rows, marks, err := wr.S.Birthdays.shareIssue(wr.Query.Actor, items, wr.Query.Now)
				if err != nil {
					return err
				}
				logAfter(wr, "birthday: copied to the shared sheet", "issue", n.Date, "copied", len(rows))
				return r.store.stageExport(wr.Tx, rows, marks)
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

func (r birthdayResources) team() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "birthday-team",
		Shape: teamResource{},
		Has:   func(m *Model, key string) bool { _, ok := m.Birthdays.teamByID(key); return ok },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			t, ok := m.Birthdays.teamByID(key)
			if !ok || !birthdaySees(m, q) {
				return nil, false
			}
			out := teamResource{Role: t.Role, Me: mine{Mine: t.Email == q.Actor.Email}}
			if m.personID(t.Email) == nil {
				out.Email = t.Email
			}
			return out, true
		},
		List: func(m *Model, q api.Query) []string {
			if !birthdaySees(m, q) {
				return []string{}
			}
			out := []string{}
			for _, t := range m.Birthdays.Team {
				out = append(out, m.Birthdays.teamID(t))
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"person": {Type: "people", List: func(m *Model, _ api.Query, key string) []string {
				t, _ := m.Birthdays.teamByID(key)
				return m.personID(t.Email)
			}},
		},
		Create: api.Make(func(wr api.Write[*Model], in TeamMember) (string, error) {
			ops, member, err := wr.S.Birthdays.addTeamMember(wr.Query.Actor, in.Email, in.Role)
			if err != nil {
				return "", err
			}
			logAfter(wr, "birthday: added team member", "email", member.Email, "role", member.Role)
			if member.Email == wr.Query.Actor.Email {
				ctx := wr.Request.Context()
				wr.Tx.After(func() {
					if err := r.store.GrantApp(ctx, "birthday", member.Email); err != nil {
						slog.ErrorContext(ctx, "birthday: put a joiner on the app's list", "error", err, "email", member.Email)
					}
				})
			}
			return wr.S.Birthdays.teamID(member), r.store.stage(wr, birthdaysAppName, ops, nil)
		}),
		Actions: map[string]api.Action[*Model]{
			"delete": api.Do(func(_ *Model, q api.Query, _ string) bool { return permitted(requireAdmin(q.Actor)) }, func(wr api.Write[*Model], _ serve.None) error {
				t, _ := wr.S.Birthdays.teamByID(wr.ID)
				ops, err := removeTeamMember(wr.Query.Actor, t)
				logAfter(wr, "birthday: removed team member", "email", t.Email, "role", t.Role)
				return r.store.stage(wr, birthdaysAppName, ops, err)
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

func (r birthdayResources) settings() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "birthday-settings",
		Shape: birthdaySettingsResource{},
		Has:   func(m *Model, key string) bool { return key == m.Birthdays.settingsID() },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			if key != m.Birthdays.settingsID() {
				return nil, false
			}
			year := m.Birthdays.yearOf(q.Now)
			out := birthdaySettingsResource{
				Year: yearResource{Current: year.Label, Last: ShiftYearSpan(year.Label, -1), Start: dateCell(year.Start), End: dateCell(year.End.AddDate(0, 0, -1))},
				Me:   m.Birthdays.standing(q.Actor),
			}
			if birthdaySees(m, q) {
				s := m.Birthdays.Settings
				out.Settings = &s
			}
			return out, true
		},
		List: func(m *Model, _ api.Query) []string { return []string{m.Birthdays.settingsID()} },
		Relations: map[string]api.Relation[*Model]{
			"viewer": {Type: "people", List: func(m *Model, q api.Query, _ string) []string { return m.personID(q.Actor.Email) }},
			"mine":   {Type: "birthdays", Many: true, List: func(m *Model, q api.Query, _ string) []string { return m.birthdayMine(q) }},
			"all":    {Type: "birthdays", Many: true, List: func(m *Model, q api.Query, _ string) []string { return m.birthdayIssues(q) }},
		},
		Actions: map[string]api.Action[*Model]{
			"edit": api.Do(func(_ *Model, q api.Query, _ string) bool { return permitted(requireAdmin(q.Actor)) }, func(wr api.Write[*Model], in BirthdaysSettings) error {
				ops, err := birthdaySettingsOps(wr.Query.Actor, in)
				logAfter(wr, "birthday: changed the settings")
				return r.store.stage(wr, birthdaysAppName, ops, err)
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

func (r birthdayResources) invites() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "birthday-invites",
		Shape: inviteResource{},
		Has:   func(m *Model, key string) bool { _, ok := m.Birthdays.inviteByID(key); return ok },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			inv, ok := m.Birthdays.inviteByID(key)
			if !ok || !birthdaySees(m, q) {
				return nil, false
			}
			return inviteResource{Year: inv.Year, RequestedOn: inv.RequestedOn, RequestedBy: inv.RequestedBy, SentTo: inv.SentTo, AskDay: inv.AskDay, SentOn: inv.SentOn}, true
		},
		List: func(m *Model, q api.Query) []string {
			if !birthdaySees(m, q) {
				return []string{}
			}
			out := []string{}
			for _, inv := range m.Birthdays.Invites {
				out = append(out, inv.ID)
			}
			return out
		},
		Create: api.Make(func(wr api.Write[*Model], in struct {
			Birthday string `json:"birthday"`
		}) (string, error) {
			address, ok := wr.S.Birthdays.staffEmails[in.Birthday]
			if !ok {
				return "", access.Missing("no birthday %s", in.Birthday)
			}
			key := id.New(wr.Taken)
			ops, err := wr.S.Birthdays.requestInvite(wr.Query.Actor, address, key, wr.Query.Now)
			logAfter(wr, "birthday: invite requested", "id", key, "email", address)
			return key, r.store.stage(wr, birthdaysAppName, ops, err)
		}),
	}
}
