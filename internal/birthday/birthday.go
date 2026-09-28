package birthday

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/admins"
	"heliosian/internal/cells"
	"heliosian/internal/id"
	"heliosian/internal/store"
)

const (
	appName            = "birthdays"
	birthdaysTab       = "Birthdays"
	assignmentsTab     = "Assignments"
	outreachTab        = "Outreach"
	donationsTab       = "Donations"
	notesTab           = "Notes"
	charitiesTab       = "Charities"
	newsletterDatesTab = "Newsletter Dates"
	settingsTab        = "Settings"
	teamTab            = "Team"
	remindersTab       = "Reminders"
	invitesTab         = "Invites"
)

const (
	kindBirthday = "birthday"
	kindDonation = "birthday-donation"
	kindNote     = "birthday-note"
	kindTeam     = "birthday-team"
	kindSettings = "birthday-settings"
)

const (
	DateFormat     = "2006-01-02"
	MonthDayFormat = "01-02"
	maxNameLength  = 120
	maxTextLength  = 6000
)

const (
	LevelSkip         = "Skip"
	LevelNoNewsletter = "No Newsletter"
)

var Levels = []string{LevelSkip, LevelNoNewsletter}

const (
	StageWait       = "Wait"
	StageOutreach   = "Awaiting Outreach"
	StageResponse   = "Awaiting Response"
	StageNewsletter = "Awaiting Newsletter"
	StageComplete   = "Complete"
)

var Stages = []string{StageWait, StageOutreach, StageResponse, StageNewsletter, StageComplete}

const (
	DefaultCharityKey   = "Default Charity"
	YearStartKey        = "Year Start"
	EmailSubjectKey     = "Email Subject"
	EmailBodyKey        = "Email Body"
	NoNewsletterNoteKey = "No Newsletter Note"
	OutreachCCKey       = "Outreach CC"
	RequestLeadKey      = "Request Lead Days"
	DueByLeadKey        = "Due By Lead Days"
)

const (
	RoleVolunteer = "Volunteer"
	RoleComms     = "Comms Team"
)

var Roles = []string{RoleVolunteer, RoleComms}

var settingKeys = []string{DefaultCharityKey, YearStartKey, EmailSubjectKey, EmailBodyKey, NoNewsletterNoteKey, OutreachCCKey, RequestLeadKey, DueByLeadKey}

var optionalSettingKeys = []string{OutreachCCKey, RequestLeadKey, DueByLeadKey}

var (
	BirthdayColumns       = []string{"Email", "Birthday", "Newsletter Override", "Participation", "Note"}
	AssignmentColumns     = []string{"Email", "Year", "Assigned To", "Assigned On"}
	OutreachColumns       = []string{"Email", "Year", "Contacted On", "Contacted By"}
	DonationColumns       = []string{"Email", "Year", "Charity", "Note", "Recorded On", "Recorded By", "Used On", "Used By"}
	NoteColumns           = []string{"Email", "Note", "Added By", "Added"}
	CharityColumns        = []string{"Charity ID", "Name", "Donation Link", "About", "EIN", "Allowed", "Why Not Allowed", "Added On"}
	NewsletterDateColumns = []string{"Newsletter Date ID", "Date"}
	SettingColumns        = []string{"Key", "Value"}
	TeamColumns           = []string{"Email", "Role"}
	ReminderColumns       = []string{"Email", "Year", "Kind", "Sent On", "Sent To"}
	InviteColumns         = []string{"Invite ID", "Email", "Year", "Requested On", "Requested By", "Sent To", "Ask Day", "Sent On"}
)

var emailForm = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type Birthday struct {
	Email    string `json:"email"`
	Birthday string `json:"birthday,omitempty"`
	Override string `json:"override,omitempty"`
	Level    string `json:"level,omitempty"`
	Note     string `json:"note,omitempty"`
}

type Assignment struct {
	Email      string `json:"email"`
	Year       string `json:"year"`
	AssignedTo string `json:"assignedTo"`
	AssignedOn string `json:"assignedOn"`
}

type Outreach struct {
	Email       string `json:"email"`
	Year        string `json:"year"`
	ContactedOn string `json:"contactedOn"`
	ContactedBy string `json:"contactedBy"`
}

type Donation struct {
	Email      string `json:"email"`
	Year       string `json:"year"`
	Charity    string `json:"charity"`
	Note       string `json:"note,omitempty"`
	RecordedOn string `json:"recordedOn"`
	RecordedBy string `json:"recordedBy,omitempty"`
	UsedOn     string `json:"usedOn,omitempty"`
	UsedBy     string `json:"usedBy,omitempty"`
}

type Note struct {
	Email   string `json:"email"`
	Note    string `json:"note"`
	AddedBy string `json:"addedBy"`
	Added   string `json:"added"`
}

type Charity struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DonationLink  string `json:"donationLink"`
	About         string `json:"about,omitempty"`
	EIN           string `json:"ein,omitempty"`
	Allowed       bool   `json:"allowed"`
	WhyNotAllowed string `json:"whyNotAllowed,omitempty"`
	AddedOn       string `json:"addedOn,omitempty"`
}

type NewsletterDate struct {
	ID   string `json:"id"`
	Date string `json:"date"`
}

type Settings struct {
	DefaultCharity   string `json:"defaultCharity"`
	YearStart        string `json:"yearStart"`
	EmailSubject     string `json:"emailSubject"`
	EmailBody        string `json:"emailBody"`
	NoNewsletterNote string `json:"noNewsletterNote"`
	OutreachCC       string `json:"outreachCC"`
	RequestLeadDays  int    `json:"requestLeadDays"`
	DueByLeadDays    int    `json:"dueByLeadDays"`
}

type TeamMember struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type Invite struct {
	ID          string
	Email       string
	Year        string
	RequestedOn string
	RequestedBy string
	SentTo      string
	AskDay      string
	SentOn      string
}

type Model struct {
	Birthdays       []Birthday
	Assignments     map[string]Assignment
	Outreach        map[string]Outreach
	Donations       map[string]Donation
	Notes           []Note
	Charities       []Charity
	NewsletterDates []NewsletterDate
	Team            []TeamMember
	Reminders       map[string]bool
	Invites         []Invite
	Settings        Settings
	admins          []string
	idKey           []byte
	byEmail         map[string]*Birthday
	byCharity       map[string]*Charity
	byNewsletter    map[string]*NewsletterDate
	donationIDs     map[string]string
	noteIDs         map[string]int
	teamIDs         map[string]int
	inviteIDs       map[string]int
}

func reminderKey(email, year, kind string) string {
	return email + "\x00" + year + "\x00" + kind
}

func yearKey(email, year string) string {
	return email + "\x00" + year
}

func (m *Model) Birthday(email string) *Birthday {
	return m.byEmail[email]
}

func (m *Model) Charity(key string) *Charity {
	return m.byCharity[key]
}

func (m *Model) charityNamed(name string) *Charity {
	for i := range m.Charities {
		if m.Charities[i].Name == name {
			return &m.Charities[i]
		}
	}
	return nil
}

func (m *Model) charityName(key string) string {
	return m.Charity(key).Name
}

func (m *Model) NewsletterDate(key string) *NewsletterDate {
	return m.byNewsletter[key]
}

func (m *Model) newsletterOn(date string) *NewsletterDate {
	for i := range m.NewsletterDates {
		if m.NewsletterDates[i].Date == date {
			return &m.NewsletterDates[i]
		}
	}
	return nil
}

func (m *Model) issueDates() []string {
	dates := []string{}
	for _, n := range m.NewsletterDates {
		dates = append(dates, n.Date)
	}
	return dates
}

func (m *Model) Assignment(email, year string) (Assignment, bool) {
	a, ok := m.Assignments[yearKey(email, year)]
	return a, ok
}

func (m *Model) OutreachFor(email, year string) (Outreach, bool) {
	o, ok := m.Outreach[yearKey(email, year)]
	return o, ok
}

func (m *Model) Donation(email, year string) (Donation, bool) {
	d, ok := m.Donations[yearKey(email, year)]
	return d, ok
}

func (m *Model) Skipped(email string) bool {
	b := m.Birthday(email)
	return b != nil && b.Level == LevelSkip
}

func (m *Model) InPipeline(email string) bool {
	b := m.Birthday(email)
	return b != nil && b.Birthday != "" && b.Level != LevelSkip
}

func (m *Model) OnTeam(email string) bool {
	for _, t := range m.Team {
		if t.Email == email {
			return true
		}
	}
	return false
}

func (m *Model) Sees(v access.Actor) bool {
	return v.May(ActAsTeam) || m.OnTeam(v.Email)
}

func checkEmail(email string) error {
	if !emailForm.MatchString(email) {
		return fmt.Errorf("%q is not an email address", email)
	}
	if email != strings.ToLower(email) {
		return fmt.Errorf("email %q is not lowercase", email)
	}
	return nil
}

func ParseDate(cell string) (time.Time, error) {
	t, err := time.Parse(DateFormat, cell)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is not a date like 2026-09-24", cell)
	}
	return t, nil
}

func checkDate(what, cell string) error {
	if cell == "" {
		return nil
	}
	if _, err := ParseDate(cell); err != nil {
		return fmt.Errorf("%s %w", what, err)
	}
	return nil
}

func checkMonthDay(what, cell string) error {
	if cell == "" {
		return nil
	}
	if _, _, err := ParseMonthDay(cell); err != nil {
		return fmt.Errorf("%s %w", what, err)
	}
	return nil
}

func checkName(kind, name string) error {
	if name == "" {
		return fmt.Errorf("%s has no name", kind)
	}
	if len(name) > maxNameLength {
		return fmt.Errorf("%s name %q is too long", kind, name)
	}
	if name != strings.TrimSpace(name) {
		return fmt.Errorf("%s name %q has surrounding spaces", kind, name)
	}
	return nil
}

func parseSettings(rows []store.Row) (Settings, error) {
	values, err := store.ParseSettings(rows, settingKeys, optionalSettingKeys)
	if err != nil {
		return Settings{}, err
	}
	if _, _, err := ParseMonthDay(values[YearStartKey]); err != nil {
		return Settings{}, fmt.Errorf("setting %q: %w", YearStartKey, err)
	}
	lead, err := leadDays(values, RequestLeadKey, DefaultRequestLeadDays)
	if err != nil {
		return Settings{}, err
	}
	dueBy, err := leadDays(values, DueByLeadKey, DefaultDueByLeadDays)
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		DefaultCharity: values[DefaultCharityKey], YearStart: values[YearStartKey],
		EmailSubject: values[EmailSubjectKey], EmailBody: values[EmailBodyKey], NoNewsletterNote: values[NoNewsletterNoteKey],
		OutreachCC: strings.TrimSpace(values[OutreachCCKey]), RequestLeadDays: lead, DueByLeadDays: dueBy,
	}, nil
}

func leadDays(values map[string]string, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(values[key])
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > 60 {
		return 0, fmt.Errorf("setting %q: %q is not a number of days from 0 to 60", key, raw)
	}
	return n, nil
}

func BuildModel(tables store.Tables, idKey []byte) (*Model, error) {
	settings, err := parseSettings(tables[settingsTab])
	if err != nil {
		return nil, err
	}
	model := &Model{
		Birthdays: []Birthday{}, Assignments: map[string]Assignment{},
		Outreach: map[string]Outreach{}, Donations: map[string]Donation{}, Notes: []Note{}, Charities: []Charity{},
		NewsletterDates: []NewsletterDate{}, Team: []TeamMember{}, Reminders: map[string]bool{}, Invites: []Invite{}, Settings: settings,
		idKey: idKey, byEmail: map[string]*Birthday{}, byCharity: map[string]*Charity{}, byNewsletter: map[string]*NewsletterDate{},
		donationIDs: map[string]string{}, noteIDs: map[string]int{}, teamIDs: map[string]int{}, inviteIDs: map[string]int{},
	}
	model.admins = admins.Read(tables)
	for _, row := range tables[remindersTab] {
		model.Reminders[reminderKey(row["Email"], row["Year"], row["Kind"])] = true
	}
	seenRoles := map[string]bool{}
	for _, row := range tables[teamTab] {
		email, role := strings.ToLower(strings.TrimSpace(row["Email"])), strings.TrimSpace(row["Role"])
		if err := checkEmail(email); err != nil {
			return nil, fmt.Errorf("team member %q: %w", row["Email"], err)
		}
		if !slices.Contains(Roles, role) {
			return nil, fmt.Errorf("team member %s: role %q is not one of %s", email, role, strings.Join(Roles, ", "))
		}
		if seenRoles[email+"\x00"+role] {
			continue
		}
		seenRoles[email+"\x00"+role] = true
		model.Team = append(model.Team, TeamMember{Email: email, Role: role})
	}
	for _, row := range tables[charitiesTab] {
		name := row["Name"]
		if err := checkName("charity", name); err != nil {
			return nil, err
		}
		fail := func(err error) (*Model, error) {
			return nil, fmt.Errorf("charity %q: %w", name, err)
		}
		if model.charityNamed(name) != nil {
			return fail(fmt.Errorf("is listed twice"))
		}
		key, ok := id.Parse(row["Charity ID"])
		if !ok {
			return fail(fmt.Errorf("charity id %q is not an id", row["Charity ID"]))
		}
		if slices.ContainsFunc(model.Charities, func(c Charity) bool { return c.ID == key }) {
			return fail(fmt.Errorf("charity id %s is used twice", key))
		}
		if err := cells.URL(row["Donation Link"], false); err != nil {
			return fail(err)
		}
		if len(row["About"]) > maxTextLength {
			return fail(fmt.Errorf("about is too long"))
		}
		allowed, err := cells.YesNo(row["Allowed"], false)
		if err != nil {
			return fail(fmt.Errorf("allowed %w", err))
		}
		if err := checkDate("added on", row["Added On"]); err != nil {
			return fail(err)
		}
		model.Charities = append(model.Charities, Charity{
			ID: key, Name: name, DonationLink: row["Donation Link"], About: row["About"], EIN: row["EIN"],
			Allowed: allowed, WhyNotAllowed: row["Why Not Allowed"], AddedOn: row["Added On"],
		})
	}
	for i := range model.Charities {
		model.byCharity[model.Charities[i].ID] = &model.Charities[i]
	}
	if c := model.Charity(settings.DefaultCharity); c == nil || !c.Allowed {
		return nil, fmt.Errorf("setting %q names %q, which is not an allowed charity's id", DefaultCharityKey, settings.DefaultCharity)
	}

	for _, row := range tables[newsletterDatesTab] {
		date := row["Date"]
		if _, err := ParseDate(date); err != nil {
			return nil, fmt.Errorf("newsletter date %w", err)
		}
		if model.newsletterOn(date) != nil {
			return nil, fmt.Errorf("newsletter date %q is listed twice", date)
		}
		key, ok := id.Parse(row["Newsletter Date ID"])
		if !ok {
			return nil, fmt.Errorf("newsletter date %s: newsletter date id %q is not an id", date, row["Newsletter Date ID"])
		}
		if slices.ContainsFunc(model.NewsletterDates, func(n NewsletterDate) bool { return n.ID == key }) || model.Charity(key) != nil {
			return nil, fmt.Errorf("newsletter date id %s is used twice", key)
		}
		model.NewsletterDates = append(model.NewsletterDates, NewsletterDate{ID: key, Date: date})
	}
	sort.Slice(model.NewsletterDates, func(i, j int) bool { return model.NewsletterDates[i].Date < model.NewsletterDates[j].Date })
	for i := range model.NewsletterDates {
		model.byNewsletter[model.NewsletterDates[i].ID] = &model.NewsletterDates[i]
	}

	for _, row := range tables[birthdaysTab] {
		email := row["Email"]
		if err := checkEmail(email); err != nil {
			return nil, fmt.Errorf("birthday row: %w", err)
		}
		fail := func(err error) (*Model, error) {
			return nil, fmt.Errorf("birthday of %s: %w", email, err)
		}
		if model.Birthday(email) != nil {
			return fail(fmt.Errorf("is listed twice"))
		}
		level := row["Participation"]
		if level != "" && !slices.Contains(Levels, level) {
			return fail(fmt.Errorf("participation %q is not blank or one of %s", level, strings.Join(Levels, ", ")))
		}
		if row["Birthday"] == "" && level != LevelSkip {
			return fail(fmt.Errorf("has no birthday; only someone who opted out may go without one"))
		}
		if err := checkMonthDay("birthday", row["Birthday"]); err != nil {
			return fail(err)
		}
		if o := row["Newsletter Override"]; o != "" && model.NewsletterDate(o) == nil {
			return fail(fmt.Errorf("newsletter override %q is not a newsletter date's id", o))
		}
		if len(row["Note"]) > maxTextLength {
			return fail(fmt.Errorf("note is too long"))
		}
		model.Birthdays = append(model.Birthdays, Birthday{Email: email, Birthday: row["Birthday"], Override: row["Newsletter Override"], Level: level, Note: row["Note"]})
	}
	for i := range model.Birthdays {
		model.byEmail[model.Birthdays[i].Email] = &model.Birthdays[i]
	}

	for _, row := range tables[assignmentsTab] {
		email, year := row["Email"], row["Year"]
		fail := func(err error) (*Model, error) {
			return nil, fmt.Errorf("assignment of %s in %s: %w", email, year, err)
		}
		if model.Birthday(email) == nil || model.Birthday(email).Birthday == "" {
			return fail(fmt.Errorf("names someone with no birthday"))
		}
		if err := CheckYear(year); err != nil {
			return fail(err)
		}
		if err := checkEmail(row["Assigned To"]); err != nil {
			return fail(fmt.Errorf("assigned to %w", err))
		}
		if err := checkDate("assigned on", row["Assigned On"]); err != nil {
			return fail(err)
		}
		if _, dup := model.Assignments[yearKey(email, year)]; dup {
			return fail(fmt.Errorf("is listed twice"))
		}
		model.Assignments[yearKey(email, year)] = Assignment{Email: email, Year: year, AssignedTo: row["Assigned To"], AssignedOn: row["Assigned On"]}
	}

	for _, row := range tables[outreachTab] {
		email, year := row["Email"], row["Year"]
		fail := func(err error) (*Model, error) {
			return nil, fmt.Errorf("outreach to %s in %s: %w", email, year, err)
		}
		if model.Birthday(email) == nil || model.Birthday(email).Birthday == "" {
			return fail(fmt.Errorf("names someone with no birthday"))
		}
		if err := CheckYear(year); err != nil {
			return fail(err)
		}
		if _, err := ParseDate(row["Contacted On"]); err != nil {
			return fail(fmt.Errorf("contacted on %w", err))
		}
		if err := checkEmail(row["Contacted By"]); err != nil {
			return fail(fmt.Errorf("contacted by %w", err))
		}
		if _, dup := model.Outreach[yearKey(email, year)]; dup {
			return fail(fmt.Errorf("is listed twice"))
		}
		model.Outreach[yearKey(email, year)] = Outreach{Email: email, Year: year, ContactedOn: row["Contacted On"], ContactedBy: row["Contacted By"]}
	}

	for _, row := range tables[donationsTab] {
		email, year := row["Email"], row["Year"]
		fail := func(err error) (*Model, error) {
			return nil, fmt.Errorf("donation of %s in %s: %w", email, year, err)
		}
		if model.Birthday(email) == nil || model.Birthday(email).Birthday == "" {
			return fail(fmt.Errorf("names someone with no birthday"))
		}
		if err := CheckYear(year); err != nil {
			return fail(err)
		}
		if model.Charity(row["Charity"]) == nil {
			return fail(fmt.Errorf("names unknown charity id %q", row["Charity"]))
		}
		if len(row["Note"]) > maxTextLength {
			return fail(fmt.Errorf("note is too long"))
		}
		if _, err := ParseDate(row["Recorded On"]); err != nil {
			return fail(fmt.Errorf("recorded on %w", err))
		}
		if row["Recorded By"] != "" {
			if err := checkEmail(row["Recorded By"]); err != nil {
				return fail(fmt.Errorf("recorded by %w", err))
			}
		}
		if (row["Used On"] == "") != (row["Used By"] == "") {
			return fail(fmt.Errorf("used on and used by must be set together"))
		}
		if err := checkDate("used on", row["Used On"]); err != nil {
			return fail(err)
		}
		if row["Used By"] != "" {
			if err := checkEmail(row["Used By"]); err != nil {
				return fail(fmt.Errorf("used by %w", err))
			}
		}
		if _, dup := model.Donations[yearKey(email, year)]; dup {
			return fail(fmt.Errorf("is listed twice"))
		}
		model.Donations[yearKey(email, year)] = Donation{
			Email: email, Year: year, Charity: row["Charity"], Note: row["Note"],
			RecordedOn: row["Recorded On"], RecordedBy: row["Recorded By"], UsedOn: row["Used On"], UsedBy: row["Used By"],
		}
	}

	for _, row := range tables[notesTab] {
		email := row["Email"]
		if model.Birthday(email) == nil {
			return nil, fmt.Errorf("note on %s names someone with no birthday", email)
		}
		if row["Note"] == "" || len(row["Note"]) > maxTextLength {
			return nil, fmt.Errorf("note on %s is empty or too long", email)
		}
		if err := checkEmail(row["Added By"]); err != nil {
			return nil, fmt.Errorf("note on %s: added by %w", email, err)
		}
		if _, err := ParseDate(row["Added"]); err != nil {
			return nil, fmt.Errorf("note on %s: added %w", email, err)
		}
		model.Notes = append(model.Notes, Note{Email: email, Note: row["Note"], AddedBy: row["Added By"], Added: row["Added"]})
	}

	for _, row := range tables[invitesTab] {
		email, year := row["Email"], row["Year"]
		fail := func(err error) (*Model, error) {
			return nil, fmt.Errorf("invite for %s in %s: %w", email, year, err)
		}
		key, ok := id.Parse(row["Invite ID"])
		if !ok {
			return fail(fmt.Errorf("invite id %q is not an id", row["Invite ID"]))
		}
		if _, dup := model.inviteIDs[key]; dup {
			return fail(fmt.Errorf("invite id %s is used twice", key))
		}
		if model.Birthday(email) == nil || model.Birthday(email).Birthday == "" {
			return fail(fmt.Errorf("names someone with no birthday"))
		}
		if err := CheckYear(year); err != nil {
			return fail(err)
		}
		if _, err := ParseDate(row["Requested On"]); err != nil {
			return fail(fmt.Errorf("requested on %w", err))
		}
		if by := row["Requested By"]; by != invitesActor {
			if err := checkEmail(by); err != nil {
				return fail(fmt.Errorf("requested by %w", err))
			}
		}
		sent := row["Sent On"] != ""
		if (row["Sent To"] != "") != sent || (row["Ask Day"] != "") != sent {
			return fail(fmt.Errorf("sent on, sent to and ask day must be set together"))
		}
		if sent {
			if err := checkEmail(row["Sent To"]); err != nil {
				return fail(fmt.Errorf("sent to %w", err))
			}
			if _, err := ParseDate(row["Ask Day"]); err != nil {
				return fail(fmt.Errorf("ask day %w", err))
			}
			if _, err := ParseDate(row["Sent On"]); err != nil {
				return fail(fmt.Errorf("sent on %w", err))
			}
		}
		model.inviteIDs[key] = len(model.Invites)
		model.Invites = append(model.Invites, Invite{
			ID: key, Email: email, Year: year, RequestedOn: row["Requested On"], RequestedBy: row["Requested By"],
			SentTo: row["Sent To"], AskDay: row["Ask Day"], SentOn: row["Sent On"],
		})
	}
	model.index()
	return model, nil
}

func (m *Model) invitesFor(email, year string) []Invite {
	out := []Invite{}
	for _, inv := range m.Invites {
		if inv.Email == email && inv.Year == year {
			out = append(out, inv)
		}
	}
	return out
}

func (m *Model) index() {
	for key, d := range m.Donations {
		m.donationIDs[m.donationID(d.Email, d.Year)] = key
	}
	for i, n := range m.Notes {
		m.noteIDs[m.noteID(n)] = i
	}
	for i, t := range m.Team {
		m.teamIDs[m.teamID(t)] = i
	}
}

func (m *Model) noteID(n Note) string {
	return id.Of(m.idKey, kindNote, strings.Join([]string{n.Email, n.Note, n.AddedBy, n.Added}, "\x00"))
}

func (m *Model) donationID(email, year string) string {
	return id.Of(m.idKey, kindDonation, yearKey(email, year))
}

func (m *Model) teamID(t TeamMember) string {
	return id.Of(m.idKey, kindTeam, t.Email+"\x00"+t.Role)
}

func (m *Model) birthdayID(email string) string {
	return id.Of(m.idKey, kindBirthday, email)
}

func (m *Model) settingsID() string {
	return id.Of(m.idKey, kindSettings, "")
}
