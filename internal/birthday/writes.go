package birthday

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

const (
	remindersActor = "reminders"
	invitesActor   = "invites"
)

type charityEdit struct {
	Name          string `json:"name"`
	DonationLink  string `json:"donationLink"`
	About         string `json:"about"`
	EIN           string `json:"ein"`
	Allowed       *bool  `json:"allowed"`
	WhyNotAllowed string `json:"whyNotAllowed"`
}

type charityAllowance struct {
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
	ActAsTeam     = access.Named("birthday.act-as-team")
	Configure     = access.Named("birthday.configure")
	DeleteAnyNote = access.Named("birthday.delete-any-note")
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

func (m *Model) findStaff(email string) error {
	if m.Skipped(email) {
		return access.Invalid("%s asked to be left out", email)
	}
	if !m.InPipeline(email) {
		return access.Missing("%s has no birthday on file", email)
	}
	return nil
}

func (m *Model) teamStaff(actor access.Actor, email string) error {
	if err := m.requireTeam(actor); err != nil {
		return err
	}
	return m.findStaff(email)
}

func yearRow(email, year string) store.Row {
	return store.Row{"Email": email, "Year": year}
}

func (m *Model) assign(actor access.Actor, email, rawTo string, isAdmin func(string) bool, now time.Time) ([]store.Op, string, error) {
	if err := m.teamStaff(actor, email); err != nil {
		return nil, "", err
	}
	to := mail.Normalize(rawTo)
	if to == "" {
		to = actor.Email
	}
	if err := checkEmail(to); err != nil {
		return nil, "", access.Invalid("%v", err)
	}
	if to != actor.Email && !m.OnTeam(to) && !isAdmin(to) {
		return nil, "", access.Invalid("%s is not on the birthday team", to)
	}
	return []store.Op{store.Upsert(assignmentsTab, yearRow(email, m.year(now)), store.Row{"Assigned To": to, "Assigned On": dayOf(now)})}, to, nil
}

func (m *Model) canUnassign(actor access.Actor, email string, now time.Time) error {
	if err := m.teamStaff(actor, email); err != nil {
		return err
	}
	if _, ok := m.Assignment(email, m.year(now)); !ok {
		return access.Invalid("nobody is assigned")
	}
	return nil
}

func (m *Model) unassign(actor access.Actor, email string, now time.Time) ([]store.Op, error) {
	if err := m.canUnassign(actor, email, now); err != nil {
		return nil, err
	}
	return []store.Op{store.Delete(assignmentsTab, yearRow(email, m.year(now)))}, nil
}

func (m *Model) canOutreach(actor access.Actor, email string, contacted bool, now time.Time) error {
	if err := m.teamStaff(actor, email); err != nil {
		return err
	}
	if _, done := m.OutreachFor(email, m.year(now)); done == contacted {
		return access.Invalid("already so")
	}
	return nil
}

func (m *Model) outreach(actor access.Actor, email string, contacted bool, now time.Time) ([]store.Op, error) {
	if err := m.canOutreach(actor, email, contacted, now); err != nil {
		return nil, err
	}
	match := yearRow(email, m.year(now))
	if !contacted {
		return []store.Op{store.Delete(outreachTab, match)}, nil
	}
	return []store.Op{store.Upsert(outreachTab, match, store.Row{"Contacted On": dayOf(now), "Contacted By": actor.Email})}, nil
}

func (m *Model) saveDonation(actor access.Actor, email, charityKey, note string, now time.Time) ([]store.Op, *Charity, error) {
	if err := m.teamStaff(actor, email); err != nil {
		return nil, nil, err
	}
	charity := m.Charity(strings.TrimSpace(charityKey))
	if charity == nil {
		return nil, nil, access.Invalid("pick a charity from the list")
	}
	if !charity.Allowed {
		return nil, nil, access.Invalid("%s is not an allowed charity: %s", charity.Name, charity.WhyNotAllowed)
	}
	if len(note) > maxTextLength {
		return nil, nil, access.Invalid("the note is too long")
	}
	cells := store.Row{"Charity": charity.ID, "Note": strings.TrimSpace(note), "Recorded On": dayOf(now), "Recorded By": actor.Email}
	return []store.Op{store.Upsert(donationsTab, yearRow(email, m.year(now)), cells)}, charity, nil
}

func (m *Model) canChangeDonation(actor access.Actor, d Donation) error {
	if err := m.teamStaff(actor, d.Email); err != nil {
		return err
	}
	return nil
}

func (m *Model) deleteDonation(actor access.Actor, d Donation) ([]store.Op, error) {
	if err := m.canChangeDonation(actor, d); err != nil {
		return nil, err
	}
	return []store.Op{store.Delete(donationsTab, yearRow(d.Email, d.Year))}, nil
}

func (m *Model) canMarkUsed(actor access.Actor, d Donation, used bool) error {
	if err := m.canChangeDonation(actor, d); err != nil {
		return err
	}
	if (d.UsedOn != "") == used {
		return access.Invalid("already so")
	}
	return nil
}

func (m *Model) markUsed(actor access.Actor, d Donation, used bool, now time.Time) ([]store.Op, error) {
	if err := m.canMarkUsed(actor, d, used); err != nil {
		return nil, err
	}
	cells := store.Row{"Used On": "", "Used By": ""}
	if used {
		cells = store.Row{"Used On": dayOf(now), "Used By": actor.Email}
	}
	return []store.Op{store.Update(donationsTab, yearRow(d.Email, d.Year), cells)}, nil
}

func (m *Model) saveBirthday(actor access.Actor, email, birthday, override string) ([]store.Op, error) {
	if err := m.requireTeam(actor); err != nil {
		return nil, err
	}
	if err := checkEmail(email); err != nil {
		return nil, access.Invalid("%v", err)
	}
	pinned := strings.TrimSpace(override)
	if pinned != "" && m.NewsletterDate(pinned) == nil {
		return nil, access.Invalid("pick a newsletter date from the list")
	}
	cells := store.Row{"Birthday": strings.TrimSpace(birthday), "Newsletter Override": pinned}
	return []store.Op{store.Upsert(birthdaysTab, store.Row{"Email": email}, cells)}, nil
}

func (m *Model) recorded(email string) bool {
	for _, a := range m.Assignments {
		if a.Email == email {
			return true
		}
	}
	for _, o := range m.Outreach {
		if o.Email == email {
			return true
		}
	}
	for _, d := range m.Donations {
		if d.Email == email {
			return true
		}
	}
	return slices.ContainsFunc(m.Notes, func(n Note) bool { return n.Email == email })
}

func (m *Model) canDeleteBirthday(actor access.Actor, email string) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	if m.Birthday(email) == nil {
		return access.Missing("no such birthday")
	}
	if m.recorded(email) {
		return access.Invalid("remove their assignments, outreach, donations, and notes first")
	}
	return nil
}

func (m *Model) deleteBirthday(actor access.Actor, email string) ([]store.Op, error) {
	if err := m.canDeleteBirthday(actor, email); err != nil {
		return nil, err
	}
	return []store.Op{store.Delete(birthdaysTab, store.Row{"Email": email})}, nil
}

func (m *Model) saveParticipation(actor access.Actor, email, level, note string) ([]store.Op, error) {
	if err := m.requireTeam(actor); err != nil {
		return nil, err
	}
	if err := checkEmail(email); err != nil {
		return nil, access.Invalid("%v", err)
	}
	if !slices.Contains(Levels, level) {
		return nil, access.Invalid("level must be one of %s", strings.Join(Levels, ", "))
	}
	if len(note) > maxTextLength {
		return nil, access.Invalid("the note is too long")
	}
	return []store.Op{store.Upsert(birthdaysTab, store.Row{"Email": email}, store.Row{"Participation": level, "Note": strings.TrimSpace(note)})}, nil
}

func (m *Model) canClearParticipation(actor access.Actor, email string) error {
	if err := m.requireTeam(actor); err != nil {
		return err
	}
	if b := m.Birthday(email); b == nil || b.Level == "" {
		return access.Missing("no preference is recorded")
	}
	return nil
}

func (m *Model) deleteParticipation(actor access.Actor, email string) ([]store.Op, error) {
	if err := m.canClearParticipation(actor, email); err != nil {
		return nil, err
	}
	match := store.Row{"Email": email}
	if m.Birthday(email).Birthday == "" {
		return []store.Op{store.Delete(birthdaysTab, match)}, nil
	}
	return []store.Op{store.Update(birthdaysTab, match, store.Row{"Participation": "", "Note": ""})}, nil
}

func (m *Model) canAddNote(actor access.Actor, email string) error {
	if err := m.requireTeam(actor); err != nil {
		return err
	}
	if b := m.Birthday(email); b == nil || b.Birthday == "" {
		return access.Missing("%s has no birthday on file", email)
	}
	return nil
}

func (m *Model) addNote(actor access.Actor, email, text string, now time.Time) ([]store.Op, error) {
	if err := m.canAddNote(actor, email); err != nil {
		return nil, err
	}
	note := strings.TrimSpace(text)
	if note == "" || len(note) > maxTextLength {
		return nil, access.Invalid("the note is empty or too long")
	}
	return []store.Op{store.Insert(notesTab, store.Row{"Email": email, "Note": note, "Added By": actor.Email, "Added": dayOf(now)})}, nil
}

func (m *Model) canDeleteNote(actor access.Actor, n Note) error {
	if err := m.requireTeam(actor); err != nil {
		return err
	}
	if !actor.May(DeleteAnyNote) && n.AddedBy != actor.Email {
		return access.Forbidden("only the note's author or an admin can remove it")
	}
	return nil
}

func (m *Model) deleteNote(actor access.Actor, n Note) ([]store.Op, error) {
	if err := m.canDeleteNote(actor, n); err != nil {
		return nil, err
	}
	return []store.Op{store.Delete(notesTab, store.Row{"Email": n.Email, "Note": n.Note, "Added By": n.AddedBy, "Added": n.Added})}, nil
}

func (m *Model) charityRow(edit charityEdit) store.Row {
	return store.Row{
		"Name": strings.TrimSpace(edit.Name), "Donation Link": strings.TrimSpace(edit.DonationLink),
		"About": strings.TrimSpace(edit.About), "EIN": strings.TrimSpace(edit.EIN),
	}
}

func (m *Model) addCharity(actor access.Actor, edit charityEdit, key string, now time.Time) ([]store.Op, error) {
	if err := m.requireTeam(actor); err != nil {
		return nil, err
	}
	row := m.charityRow(edit)
	if m.charityNamed(row["Name"]) != nil {
		return nil, access.Invalid("%q is already on the list", row["Name"])
	}
	allowed, why := true, ""
	if edit.Allowed != nil && actor.May(Configure) {
		allowed, why = *edit.Allowed, strings.TrimSpace(edit.WhyNotAllowed)
	}
	if allowed {
		why = ""
	}
	row["Charity ID"], row["Added On"] = key, dayOf(now)
	row["Allowed"], row["Why Not Allowed"] = cells.YesNoCell(allowed), why
	return []store.Op{store.Insert(charitiesTab, row)}, nil
}

func (m *Model) editCharity(actor access.Actor, c *Charity, edit charityEdit) ([]store.Op, error) {
	if err := m.requireTeam(actor); err != nil {
		return nil, err
	}
	row := m.charityRow(edit)
	if other := m.charityNamed(row["Name"]); other != nil && other.ID != c.ID {
		return nil, access.Invalid("%q is already on the list", row["Name"])
	}
	return []store.Op{store.Update(charitiesTab, store.Row{"Charity ID": c.ID}, row)}, nil
}

func allowCharity(actor access.Actor, c *Charity, a charityAllowance) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	why := strings.TrimSpace(a.WhyNotAllowed)
	if a.Allowed {
		why = ""
	}
	return []store.Op{store.Update(charitiesTab, store.Row{"Charity ID": c.ID}, store.Row{"Allowed": cells.YesNoCell(a.Allowed), "Why Not Allowed": why})}, nil
}

func (m *Model) canDeleteCharity(actor access.Actor, c *Charity) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	if m.Settings.DefaultCharity == c.ID {
		return access.Invalid("pick another default charity first")
	}
	for _, d := range m.Donations {
		if d.Charity == c.ID {
			return access.Invalid("donations name this charity; it can be marked not allowed instead")
		}
	}
	return nil
}

func (m *Model) deleteCharity(actor access.Actor, c *Charity) ([]store.Op, error) {
	if err := m.canDeleteCharity(actor, c); err != nil {
		return nil, err
	}
	return []store.Op{store.Delete(charitiesTab, store.Row{"Charity ID": c.ID})}, nil
}

func (m *Model) addNewsletterDate(actor access.Actor, raw, key string) ([]store.Op, string, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, "", err
	}
	date := strings.TrimSpace(raw)
	if _, err := ParseDate(date); err != nil {
		return nil, "", access.Invalid("%v", err)
	}
	if m.newsletterOn(date) != nil {
		return nil, "", access.Invalid("%s is already on the list", date)
	}
	return []store.Op{store.Insert(newsletterDatesTab, store.Row{"Newsletter Date ID": key, "Date": date})}, date, nil
}

func (m *Model) moveNewsletterDate(actor access.Actor, n *NewsletterDate, rawDate string) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	date := strings.TrimSpace(rawDate)
	if _, err := ParseDate(date); err != nil {
		return nil, access.Invalid("%v", err)
	}
	if date == n.Date {
		return nil, nil
	}
	if m.newsletterOn(date) != nil {
		return nil, access.Invalid("%s is already on the list", date)
	}
	return []store.Op{store.Update(newsletterDatesTab, store.Row{"Newsletter Date ID": n.ID}, store.Row{"Date": date})}, nil
}

func deleteNewsletterDate(actor access.Actor, n *NewsletterDate) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	return []store.Op{
		store.Update(birthdaysTab, store.Row{"Newsletter Override": n.ID}, store.Row{"Newsletter Override": ""}),
		store.Delete(newsletterDatesTab, store.Row{"Newsletter Date ID": n.ID}),
	}, nil
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

func (m *Model) addTeamMember(actor access.Actor, rawEmail, rawRole string) ([]store.Op, TeamMember, error) {
	member := TeamMember{Email: mail.Normalize(rawEmail), Role: strings.TrimSpace(rawRole)}
	if member.Email == "" {
		member.Email = actor.Email
	}
	if member.Role == "" {
		member.Role = RoleVolunteer
	}
	if !actor.May(Configure) && (member.Email != actor.Email || member.Role != RoleVolunteer) {
		return nil, TeamMember{}, access.Forbidden("admin access required")
	}
	if err := checkEmail(member.Email); err != nil {
		return nil, TeamMember{}, access.Invalid("%v", err)
	}
	if !slices.Contains(Roles, member.Role) {
		return nil, TeamMember{}, access.Invalid("%q is not a role", member.Role)
	}
	if slices.Contains(m.Team, member) {
		return nil, member, nil
	}
	return []store.Op{store.Insert(teamTab, store.Row{"Email": member.Email, "Role": member.Role})}, member, nil
}

func removeTeamMember(actor access.Actor, member TeamMember) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	return []store.Op{store.Delete(teamTab, store.Row{"Email": member.Email, "Role": member.Role})}, nil
}

func inviteRow(actor access.Actor, email, year, key string, now time.Time) store.Op {
	return store.Insert(invitesTab, store.Row{"Invite ID": key, "Email": email, "Year": year, "Requested On": dayOf(now), "Requested By": actor.Email})
}

func (m *Model) requestInvite(actor access.Actor, email, key string, now time.Time) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	if err := m.findStaff(email); err != nil {
		return nil, err
	}
	return []store.Op{inviteRow(actor, email, m.year(now), key, now)}, nil
}

func queueInvite(actor access.Actor, sv StaffView, key string, now time.Time) ([]store.Op, error) {
	if err := requireSystem(actor, invitesActor); err != nil {
		return nil, err
	}
	return []store.Op{inviteRow(actor, sv.Email, sv.Year, key, now)}, nil
}

func recordInvite(actor access.Actor, inv Invite, sv StaffView, now time.Time) ([]store.Op, error) {
	if err := requireSystem(actor, invitesActor); err != nil {
		return nil, err
	}
	return []store.Op{store.Update(invitesTab, store.Row{"Invite ID": inv.ID}, store.Row{"Sent To": sv.AssignedTo, "Ask Day": sv.RequestBy, "Sent On": dayOf(now)})}, nil
}

func exportOps(actor access.Actor, items []exported, now time.Time) ([]store.Op, []store.Op) {
	day := dayOf(now)
	rows, marks := []store.Op{}, []store.Op{}
	for _, it := range items {
		cells := maps.Clone(it.donation)
		cells["Used On"], cells["Used By"] = day, actor.Email
		rows = append(rows, store.Insert(sharedNewsletterTab, it.row))
		marks = append(marks, store.Upsert(donationsTab, yearRow(it.email, it.year), cells))
	}
	return rows, marks
}

func weeklyExport(actor access.Actor, items []exported, now time.Time) ([]store.Op, []store.Op, error) {
	if err := requireSystem(actor, exportActor); err != nil {
		return nil, nil, err
	}
	rows, marks := exportOps(actor, items, now)
	return rows, marks, nil
}

func (m *Model) shareIssue(actor access.Actor, items []exported, now time.Time) ([]store.Op, []store.Op, error) {
	if err := m.requireTeam(actor); err != nil {
		return nil, nil, err
	}
	rows, marks := exportOps(actor, items, now)
	return rows, marks, nil
}

func recordReminder(actor access.Actor, sv StaffView, kind, to string, today time.Time) ([]store.Op, error) {
	if err := requireSystem(actor, remindersActor); err != nil {
		return nil, err
	}
	cells := store.Row{"Email": sv.Email, "Year": sv.Year, "Kind": kind, "Sent On": today.Format(DateFormat), "Sent To": to}
	return []store.Op{store.Insert(remindersTab, cells)}, nil
}
