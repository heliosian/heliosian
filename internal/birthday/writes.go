package birthday

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/config"
	"heliosian/internal/id"
	"heliosian/internal/store"
)

const remindersActor = "reminders"

type target struct {
	email, year, to, charity string
}

type charityEdit struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DonationLink  string `json:"donationLink"`
	About         string `json:"about"`
	EIN           string `json:"ein"`
	Allowed       bool   `json:"allowed"`
	WhyNotAllowed string `json:"whyNotAllowed"`
}

func (m *Model) requireTeam(actor access.Actor) error {
	if !m.Sees(actor) {
		return access.Forbidden("team membership required")
	}
	return nil
}

var (
	ActAsTeam     = access.Standing("birthday.act-as-team")
	Configure     = access.Standing("birthday.configure")
	DeleteAnyNote = access.Acting("birthday.delete-any-note")
)

var AdminAllowances = []access.Allowance{ActAsTeam, Configure, DeleteAnyNote}

func requireAdmin(actor access.Actor) error {
	if !actor.May(Configure) {
		return access.Forbidden("admin access required")
	}
	return nil
}

func requireSystem(actor access.Actor, name string) error {
	if len(actor.Allowances) > 0 || actor.Email != name {
		return access.Forbidden("only %s may make this change", name)
	}
	return nil
}

func (m *Model) findStaff(raw string) (string, error) {
	email := config.NormalizeEmail(raw)
	if m.Skipped(email) {
		return "", access.Invalid("%s asked to be left out", email)
	}
	if !m.InPipeline(email) {
		return "", access.Missing("%s has no birthday on file", email)
	}
	return email, nil
}

func (m *Model) teamStaff(actor access.Actor, raw string) (target, error) {
	if err := m.requireTeam(actor); err != nil {
		return target{}, err
	}
	email, err := m.findStaff(raw)
	if err != nil {
		return target{}, err
	}
	return target{email: email, year: m.year()}, nil
}

func (c *Cache) assign(actor access.Actor, rawEmail, rawTo string) ([]store.Op, target, error) {
	m := c.Model()
	t, err := m.teamStaff(actor, rawEmail)
	if err != nil {
		return nil, target{}, err
	}
	t.to = config.NormalizeEmail(rawTo)
	if t.to == "" {
		t.to = actor.Email
	}
	if err := checkEmail(t.to); err != nil {
		return nil, target{}, access.Invalid("%v", err)
	}
	if t.to != actor.Email && !m.OnTeam(t.to) && !c.IsAdmin(t.to) {
		return nil, target{}, access.Invalid("%s is not on the birthday team", t.to)
	}
	return []store.Op{store.Upsert(assignmentsTab, store.Row{"Email": t.email, "Year": t.year}, store.Row{"Assigned To": t.to, "Assigned On": today()})}, t, nil
}

func (m *Model) unassign(actor access.Actor, rawEmail string) ([]store.Op, target, error) {
	t, err := m.teamStaff(actor, rawEmail)
	if err != nil {
		return nil, target{}, err
	}
	return []store.Op{store.Delete(assignmentsTab, store.Row{"Email": t.email, "Year": t.year})}, t, nil
}

func (m *Model) outreach(actor access.Actor, rawEmail string, contacted bool) ([]store.Op, target, error) {
	t, err := m.teamStaff(actor, rawEmail)
	if err != nil {
		return nil, target{}, err
	}
	match := store.Row{"Email": t.email, "Year": t.year}
	if !contacted {
		return []store.Op{store.Delete(outreachTab, match)}, t, nil
	}
	return []store.Op{store.Upsert(outreachTab, match, store.Row{"Contacted On": today(), "Contacted By": actor.Email})}, t, nil
}

func (m *Model) saveDonation(actor access.Actor, rawEmail, charityKey, note string) ([]store.Op, target, error) {
	t, err := m.teamStaff(actor, rawEmail)
	if err != nil {
		return nil, target{}, err
	}
	charity := m.Charity(strings.TrimSpace(charityKey))
	if charity == nil {
		return nil, target{}, access.Invalid("pick a charity from the list")
	}
	if !charity.Allowed {
		return nil, target{}, access.Invalid("%s is not an allowed charity: %s", charity.Name, charity.WhyNotAllowed)
	}
	if len(note) > maxTextLength {
		return nil, target{}, access.Invalid("the note is too long")
	}
	t.charity = charity.Name
	cells := store.Row{"Charity": charity.ID, "Note": strings.TrimSpace(note), "Recorded On": today(), "Recorded By": actor.Email}
	return []store.Op{store.Upsert(donationsTab, store.Row{"Email": t.email, "Year": t.year}, cells)}, t, nil
}

func (m *Model) deleteDonation(actor access.Actor, rawEmail string) ([]store.Op, target, error) {
	t, err := m.teamStaff(actor, rawEmail)
	if err != nil {
		return nil, target{}, err
	}
	return []store.Op{store.Delete(donationsTab, store.Row{"Email": t.email, "Year": t.year})}, t, nil
}

func (m *Model) markUsed(actor access.Actor, rawEmail string, used bool) ([]store.Op, target, error) {
	t, err := m.teamStaff(actor, rawEmail)
	if err != nil {
		return nil, target{}, err
	}
	if _, ok := m.Donation(t.email, t.year); !ok {
		return nil, target{}, access.Invalid("record a donation first")
	}
	cells := store.Row{"Used On": "", "Used By": ""}
	if used {
		cells = store.Row{"Used On": today(), "Used By": actor.Email}
	}
	return []store.Op{store.Update(donationsTab, store.Row{"Email": t.email, "Year": t.year}, cells)}, t, nil
}

func (m *Model) saveBirthday(actor access.Actor, rawEmail, birthday, override string) ([]store.Op, string, error) {
	if err := m.requireTeam(actor); err != nil {
		return nil, "", err
	}
	email := config.NormalizeEmail(rawEmail)
	if err := checkEmail(email); err != nil {
		return nil, "", access.Invalid("%v", err)
	}
	pinned := strings.TrimSpace(override)
	if pinned != "" && m.NewsletterDate(pinned) == nil {
		return nil, "", access.Invalid("pick a newsletter date from the list")
	}
	cells := store.Row{"Birthday": strings.TrimSpace(birthday), "Newsletter Override": pinned}
	return []store.Op{store.Upsert(birthdaysTab, store.Row{"Email": email}, cells)}, email, nil
}

func (c *Cache) deleteBirthday(actor access.Actor, rawEmail string) ([]store.Op, string, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, "", err
	}
	email := config.NormalizeEmail(rawEmail)
	match := store.Row{"Email": email}
	if c.Count(birthdaysTab, match) == 0 {
		return nil, "", access.Missing("no such birthday")
	}
	for _, tab := range []string{assignmentsTab, outreachTab, donationsTab, notesTab} {
		if c.Count(tab, match) > 0 {
			return nil, "", access.Invalid("remove their assignments, outreach, donations, and notes first")
		}
	}
	return []store.Op{store.Delete(birthdaysTab, match)}, email, nil
}

func (m *Model) saveParticipation(actor access.Actor, rawEmail, level, note string) ([]store.Op, string, error) {
	if err := m.requireTeam(actor); err != nil {
		return nil, "", err
	}
	email := config.NormalizeEmail(rawEmail)
	if err := checkEmail(email); err != nil {
		return nil, "", access.Invalid("%v", err)
	}
	if !slices.Contains(Levels, level) {
		return nil, "", access.Invalid("level must be one of %s", strings.Join(Levels, ", "))
	}
	if len(note) > maxTextLength {
		return nil, "", access.Invalid("the note is too long")
	}
	return []store.Op{store.Upsert(birthdaysTab, store.Row{"Email": email}, store.Row{"Participation": level, "Note": strings.TrimSpace(note)})}, email, nil
}

func (m *Model) deleteParticipation(actor access.Actor, rawEmail string) ([]store.Op, string, error) {
	if err := m.requireTeam(actor); err != nil {
		return nil, "", err
	}
	email := config.NormalizeEmail(rawEmail)
	b := m.Birthday(email)
	if b == nil || b.Level == "" {
		return nil, "", access.Missing("no preference is recorded")
	}
	match := store.Row{"Email": email}
	if b.Birthday == "" {
		return []store.Op{store.Delete(birthdaysTab, match)}, email, nil
	}
	return []store.Op{store.Update(birthdaysTab, match, store.Row{"Participation": "", "Note": ""})}, email, nil
}

func (m *Model) addNote(actor access.Actor, rawEmail, text string) ([]store.Op, string, error) {
	if err := m.requireTeam(actor); err != nil {
		return nil, "", err
	}
	email := config.NormalizeEmail(rawEmail)
	if b := m.Birthday(email); b == nil || b.Birthday == "" {
		return nil, "", access.Missing("%s has no birthday on file", email)
	}
	note := strings.TrimSpace(text)
	if note == "" || len(note) > maxTextLength {
		return nil, "", access.Invalid("the note is empty or too long")
	}
	return []store.Op{store.Insert(notesTab, store.Row{"Email": email, "Note": note, "Added By": actor.Email, "Added": today()})}, email, nil
}

func sameCell(stored, given string) bool {
	return strings.EqualFold(strings.TrimSpace(stored), strings.TrimSpace(given))
}

func (m *Model) deleteNote(actor access.Actor, given Note) ([]store.Op, string, error) {
	if err := m.requireTeam(actor); err != nil {
		return nil, "", err
	}
	match := store.Row{"Email": config.NormalizeEmail(given.Email), "Note": given.Note, "Added By": config.NormalizeEmail(given.AddedBy), "Added": given.Added}
	if !actor.May(DeleteAnyNote) && match["Added By"] != actor.Email {
		return nil, "", access.Forbidden("only the note's author or an admin can remove it")
	}
	for _, n := range m.Notes {
		if !sameCell(n.Email, match["Email"]) || !sameCell(n.Note, match["Note"]) || !sameCell(n.AddedBy, match["Added By"]) || !sameCell(n.Added, match["Added"]) {
			continue
		}
		if !actor.May(DeleteAnyNote) && !sameCell(n.AddedBy, actor.Email) {
			return nil, "", access.Forbidden("only the note's author or an admin can remove it")
		}
	}
	return []store.Op{store.Delete(notesTab, match)}, match["Email"], nil
}

func (m *Model) saveCharity(actor access.Actor, edit charityEdit) ([]store.Op, Charity, bool, error) {
	if err := m.requireTeam(actor); err != nil {
		return nil, Charity{}, false, err
	}
	name := strings.TrimSpace(edit.Name)
	key := strings.TrimSpace(edit.ID)
	adding := key == ""
	var current *Charity
	if !adding {
		current = m.Charity(key)
		if current == nil {
			return nil, Charity{}, false, access.Missing("no such charity")
		}
	}
	allowed, why := true, ""
	switch {
	case actor.May(Configure):
		allowed, why = edit.Allowed, strings.TrimSpace(edit.WhyNotAllowed)
	case !adding:
		allowed, why = current.Allowed, current.WhyNotAllowed
	}
	if allowed {
		why = ""
	}
	row := store.Row{
		"Name": name, "Donation Link": strings.TrimSpace(edit.DonationLink), "About": strings.TrimSpace(edit.About),
		"EIN": strings.TrimSpace(edit.EIN), "Allowed": cells.YesNoCell(allowed), "Why Not Allowed": why,
	}
	if other := m.charityNamed(name); other != nil && (adding || other.ID != current.ID) {
		return nil, Charity{}, false, access.Invalid("%q is already on the list", name)
	}
	if adding {
		key = id.New(m.taken)
		row["Charity ID"] = key
		row["Added On"] = today()
		return []store.Op{store.Insert(charitiesTab, row)}, Charity{ID: key, Name: name, Allowed: allowed}, true, nil
	}
	return []store.Op{store.Update(charitiesTab, store.Row{"Charity ID": current.ID}, row)}, Charity{ID: current.ID, Name: name, Allowed: allowed}, false, nil
}

func (c *Cache) deleteCharity(actor access.Actor, key string) ([]store.Op, *Charity, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, nil, err
	}
	m := c.Model()
	charity := m.Charity(strings.TrimSpace(key))
	if charity == nil {
		return nil, nil, access.Missing("no such charity")
	}
	if m.Settings.DefaultCharity == charity.ID {
		return nil, nil, access.Invalid("pick another default charity first")
	}
	if c.Count(donationsTab, store.Row{"Charity": charity.ID}) > 0 {
		return nil, nil, access.Invalid("donations name this charity; it can be marked not allowed instead")
	}
	return []store.Op{store.Delete(charitiesTab, store.Row{"Charity ID": charity.ID})}, charity, nil
}

func (m *Model) addNewsletterDate(actor access.Actor, raw string) ([]store.Op, string, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, "", err
	}
	date := strings.TrimSpace(raw)
	if _, err := ParseDate(date); err != nil {
		return nil, "", access.Invalid("%v", err)
	}
	if m.newsletterOn(date) != nil {
		return nil, "", access.Invalid("that date is already on the list")
	}
	return []store.Op{store.Insert(newsletterDatesTab, store.Row{"Newsletter Date ID": id.New(m.taken), "Date": date})}, date, nil
}

func (m *Model) changeNewsletterDate(actor access.Actor, key, rawDate string) ([]store.Op, *NewsletterDate, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, nil, err
	}
	date := strings.TrimSpace(rawDate)
	if _, err := ParseDate(date); err != nil {
		return nil, nil, access.Invalid("%v", err)
	}
	n := m.NewsletterDate(strings.TrimSpace(key))
	if n == nil {
		return nil, nil, access.Missing("no such newsletter date")
	}
	if date == n.Date {
		return nil, n, nil
	}
	if m.newsletterOn(date) != nil {
		return nil, nil, access.Invalid("that date is already on the list")
	}
	return []store.Op{store.Update(newsletterDatesTab, store.Row{"Newsletter Date ID": n.ID}, store.Row{"Date": date})}, n, nil
}

func dropNewsletterDate(n NewsletterDate) []store.Op {
	return []store.Op{
		store.Update(birthdaysTab, store.Row{"Newsletter Override": n.ID}, store.Row{"Newsletter Override": ""}),
		store.Delete(newsletterDatesTab, store.Row{"Newsletter Date ID": n.ID}),
	}
}

func (m *Model) deleteNewsletterDate(actor access.Actor, key string) ([]store.Op, *NewsletterDate, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, nil, err
	}
	n := m.NewsletterDate(strings.TrimSpace(key))
	if n == nil {
		return nil, nil, access.Missing("no such newsletter date")
	}
	return dropNewsletterDate(*n), n, nil
}

func (m *Model) createNewsletterDates(actor access.Actor, weekday int, rawFrom, rawTo string) ([]store.Op, []string, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, nil, err
	}
	from, err := ParseDate(strings.TrimSpace(rawFrom))
	if err != nil {
		return nil, nil, access.Invalid("from: %v", err)
	}
	to, err := ParseDate(strings.TrimSpace(rawTo))
	if err != nil {
		return nil, nil, access.Invalid("to: %v", err)
	}
	if weekday < 0 || weekday > 6 {
		return nil, nil, access.Invalid("weekday must be 0 (Sunday) to 6 (Saturday)")
	}
	if to.Before(from) {
		return nil, nil, access.Invalid("the final date is before the first")
	}
	if to.Sub(from) > 366*24*time.Hour {
		return nil, nil, access.Invalid("at most a year at a time")
	}
	added := []string{}
	ops := []store.Op{}
	mint := m.minter()
	for day := from.AddDate(0, 0, (weekday-int(from.Weekday())+7)%7); !day.After(to); day = day.AddDate(0, 0, 7) {
		cell := day.Format(DateFormat)
		if m.newsletterOn(cell) != nil {
			continue
		}
		added = append(added, cell)
		ops = append(ops, store.Insert(newsletterDatesTab, store.Row{"Newsletter Date ID": mint(), "Date": cell}))
	}
	if len(added) == 0 {
		return nil, nil, access.Invalid("every one of those dates is already on the list")
	}
	return ops, added, nil
}

func (m *Model) clearFutureNewsletterDates(actor access.Actor, day string) ([]store.Op, int, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, 0, err
	}
	ops := []store.Op{}
	cleared := 0
	for _, n := range m.NewsletterDates {
		if n.Date >= day {
			ops = append(ops, dropNewsletterDate(n)...)
			cleared++
		}
	}
	return ops, cleared, nil
}

func saveSettings(actor access.Actor, s Settings) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	values := map[string]string{
		DefaultCharityKey: strings.TrimSpace(s.DefaultCharity), YearStartKey: strings.TrimSpace(s.YearStart),
		EmailSubjectKey: strings.TrimSpace(s.EmailSubject), EmailBodyKey: strings.TrimSpace(s.EmailBody),
		NoNewsletterNoteKey: strings.TrimSpace(s.NoNewsletterNote), OutreachCCKey: strings.TrimSpace(s.OutreachCC),
		RequestLeadKey: strconv.Itoa(s.RequestLeadDays), DueByLeadKey: strconv.Itoa(s.DueByLeadDays),
	}
	ops := []store.Op{}
	for _, key := range settingKeys {
		ops = append(ops, store.Upsert(settingsTab, store.Row{"Key": key}, store.Row{"Value": values[key]}))
	}
	return ops, nil
}

func (c *Cache) joinTeam(actor access.Actor) ([]store.Op, error) {
	if err := checkEmail(actor.Email); err != nil {
		return nil, access.Invalid("%v", err)
	}
	row := store.Row{"Email": actor.Email, "Role": RoleVolunteer}
	if c.Count(teamTab, row) > 0 {
		return nil, nil
	}
	return []store.Op{store.Insert(teamTab, row)}, nil
}

func teamMember(rawEmail, rawRole string) (TeamMember, error) {
	email := config.NormalizeEmail(rawEmail)
	if err := checkEmail(email); err != nil {
		return TeamMember{}, access.Invalid("%v", err)
	}
	role := strings.TrimSpace(rawRole)
	if !slices.Contains(Roles, role) {
		return TeamMember{}, access.Invalid("%q is not a role", role)
	}
	return TeamMember{Email: email, Role: role}, nil
}

func (c *Cache) addTeamMember(actor access.Actor, rawEmail, rawRole string) ([]store.Op, TeamMember, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, TeamMember{}, err
	}
	member, err := teamMember(rawEmail, rawRole)
	if err != nil {
		return nil, TeamMember{}, err
	}
	row := store.Row{"Email": member.Email, "Role": member.Role}
	if c.Count(teamTab, row) > 0 {
		return nil, member, nil
	}
	return []store.Op{store.Insert(teamTab, row)}, member, nil
}

func removeTeamMember(actor access.Actor, rawEmail, rawRole string) ([]store.Op, TeamMember, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, TeamMember{}, err
	}
	member, err := teamMember(rawEmail, rawRole)
	if err != nil {
		return nil, TeamMember{}, err
	}
	return []store.Op{store.Delete(teamTab, store.Row{"Email": member.Email, "Role": member.Role})}, member, nil
}

func exportOps(actor access.Actor, items []exported) ([]store.Op, []store.Op) {
	day := today()
	rows, marks := []store.Op{}, []store.Op{}
	for _, it := range items {
		cells := maps.Clone(it.donation)
		cells["Used On"], cells["Used By"] = day, actor.Email
		rows = append(rows, store.Insert(sharedNewsletterTab, it.row))
		marks = append(marks, store.Upsert(donationsTab, store.Row{"Email": it.email, "Year": it.year}, cells))
	}
	return rows, marks
}

func weeklyExport(actor access.Actor, items []exported) ([]store.Op, []store.Op, error) {
	if err := requireSystem(actor, exportActor); err != nil {
		return nil, nil, err
	}
	rows, marks := exportOps(actor, items)
	return rows, marks, nil
}

func (m *Model) shareIssue(actor access.Actor, key string, items func(issue string) []exported) ([]store.Op, []store.Op, string, error) {
	if err := m.requireTeam(actor); err != nil {
		return nil, nil, "", err
	}
	n := m.NewsletterDate(strings.TrimSpace(key))
	if n == nil {
		return nil, nil, "", access.Invalid("that is not a newsletter date")
	}
	rows, marks := exportOps(actor, items(n.Date))
	return rows, marks, n.Date, nil
}

func recordReminder(actor access.Actor, sv StaffView, kind, to string, today time.Time) ([]store.Op, error) {
	if err := requireSystem(actor, remindersActor); err != nil {
		return nil, err
	}
	cells := store.Row{"Email": sv.Email, "Year": sv.Year, "Kind": kind, "Sent On": today.Format(DateFormat), "Sent To": to}
	return []store.Op{store.Insert(remindersTab, cells)}, nil
}
