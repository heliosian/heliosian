package birthday

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/config"
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
	adminsTab          = "Admins"
	teamTab            = "Team"
	remindersTab       = "Reminders"
)

const (
	DateFormat     = "2006-01-02"
	MonthDayFormat = "01-02"
	maxNameLength  = 120
	maxTextLength  = 6000
	maxURLLength   = 1000
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
	// OutreachCCKey is who a volunteer copies on the outreach email - the
	// HCA's address - and may be blank.
	OutreachCCKey = "Outreach CC"
	// RequestLeadKey is how many days before the newsletter the request is
	// due; blank means the default.
	RequestLeadKey = "Request Lead Days"
	// DueByLeadKey is how many days before the newsletter the birthday's
	// information - the charity - is due in; blank means the default.
	DueByLeadKey = "Due By Lead Days"
)

// The roles on the Team tab: volunteers work the birthdays and are offered
// when one is assigned; the comms team carries the donations into the
// newsletter.
const (
	RoleVolunteer = "Volunteer"
	RoleComms     = "Comms Team"
)

var Roles = []string{RoleVolunteer, RoleComms}

var settingKeys = []string{DefaultCharityKey, YearStartKey, EmailSubjectKey, EmailBodyKey, NoNewsletterNoteKey, OutreachCCKey, RequestLeadKey, DueByLeadKey}

// legacyThemeKeys are rows the Appearance panel wrote while the apps could
// be recoloured (2026-09-16); the tab may still carry them, and they are
// passed over.
var legacyThemeKeys = []string{"Sidebar Color", "Sidebar Color 2", "Sidebar Text Color", "Page Color", "Page Color 2", "Logo", "Sidebar Image"}

// optionalSettingKeys may be missing or blank.
var optionalSettingKeys = []string{OutreachCCKey, RequestLeadKey, DueByLeadKey}

var (
	BirthdayColumns       = []string{"Email", "Birthday", "Newsletter Override", "Participation", "Note"}
	AssignmentColumns     = []string{"Email", "Year", "Assigned To", "Assigned On"}
	OutreachColumns       = []string{"Email", "Year", "Contacted On", "Contacted By"}
	DonationColumns       = []string{"Email", "Year", "Charity", "Note", "Recorded On", "Recorded By", "Used On", "Used By"}
	NoteColumns           = []string{"Email", "Note", "Added By", "Added"}
	CharityColumns        = []string{"Name", "Donation Link", "About", "EIN", "Allowed", "Why Not Allowed", "Added On"}
	NewsletterDateColumns = []string{"Date"}
	SettingColumns        = []string{"Key", "Value"}
	AdminColumns          = []string{"Email"}
	TeamColumns           = []string{"Email", "Role"}
	ReminderColumns       = []string{"Email", "Year", "Kind", "Sent On", "Sent To"}
)

var emailForm = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// Birthday is one staff member's row: their birthday as a month and day,
// 08-20, the newsletter that should carry it when the usual pick is wrong, and
// their standing wish about taking part. Someone who opted out entirely may
// have no birthday on file.
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
	Name          string `json:"name"`
	DonationLink  string `json:"donationLink"`
	About         string `json:"about,omitempty"`
	EIN           string `json:"ein,omitempty"`
	Allowed       bool   `json:"allowed"`
	WhyNotAllowed string `json:"whyNotAllowed,omitempty"`
	AddedOn       string `json:"addedOn,omitempty"`
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

// TeamMember is one person in one role on the Team tab.
type TeamMember struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// Model is the sheet organized: birthdays in row order, the per-year progress
// tables keyed by email and year, charities by name, and the newsletter dates
// sorted.
type Model struct {
	Birthdays       []Birthday
	Assignments     map[string]Assignment
	Outreach        map[string]Outreach
	Donations       map[string]Donation
	Notes           []Note
	Charities       []Charity
	NewsletterDates []string
	Team            []TeamMember
	// Reminders is every reminder sent, by the birthday, year and kind, so
	// none goes twice.
	Reminders map[string]bool
	Settings  Settings
	Admins    []string
	byEmail   map[string]*Birthday
	byCharity map[string]*Charity
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

func (m *Model) Charity(name string) *Charity {
	return m.byCharity[name]
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

// Skipped reports whether a staff member asked to be left out entirely.
func (m *Model) Skipped(email string) bool {
	b := m.Birthday(email)
	return b != nil && b.Level == LevelSkip
}

// InPipeline reports whether a staff member has a birthday to work through.
func (m *Model) InPipeline(email string) bool {
	b := m.Birthday(email)
	return b != nil && b.Birthday != "" && b.Level != LevelSkip
}

// OnTeam reports whether email holds any role on the Team tab.
func (m *Model) OnTeam(email string) bool {
	for _, t := range m.Team {
		if t.Email == email {
			return true
		}
	}
	return false
}

func (m *Model) Sees(v access.Viewer) bool {
	return v.Admin || m.OnTeam(v.Email)
}

func yesNo(cell string) (bool, error) {
	switch cell {
	case "Yes":
		return true, nil
	case "No", "":
		return false, nil
	}
	return false, fmt.Errorf("%q is not Yes, No, or blank", cell)
}

func YesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func checkURL(raw string) error {
	if len(raw) > maxURLLength {
		return fmt.Errorf("url is too long")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("url %q must be an absolute http(s) url", raw)
	}
	return nil
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

// ParseDate reads a date cell, 2026-09-24.
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

func parseSettings(rows []map[string]string) (Settings, error) {
	values := map[string]string{}
	for _, row := range rows {
		key := row["Key"]
		if slices.Contains(legacyThemeKeys, key) {
			continue
		}
		if !slices.Contains(settingKeys, key) {
			return Settings{}, fmt.Errorf("%s has unknown key %q", settingsTab, key)
		}
		if _, dup := values[key]; dup {
			return Settings{}, fmt.Errorf("%s has duplicate key %q", settingsTab, key)
		}
		values[key] = row["Value"]
	}
	for _, key := range settingKeys {
		if values[key] == "" && !slices.Contains(optionalSettingKeys, key) {
			return Settings{}, fmt.Errorf("%s is missing %q", settingsTab, key)
		}
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

// leadDays reads a days-before-the-newsletter setting, 0 to 60, or its
// default when blank.
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

// BuildModel validates every row and refuses the whole set on the first problem,
// the stance every app here takes: a sheet edit that breaks a rule surfaces as a
// refused load, never as a page quietly missing a birthday.
func BuildModel(tables store.Tables) (*Model, error) {
	settings, err := parseSettings(tables[settingsTab])
	if err != nil {
		return nil, err
	}
	model := &Model{
		Birthdays: []Birthday{}, Assignments: map[string]Assignment{},
		Outreach: map[string]Outreach{}, Donations: map[string]Donation{}, Notes: []Note{}, Charities: []Charity{},
		NewsletterDates: []string{}, Team: []TeamMember{}, Reminders: map[string]bool{}, Settings: settings, byEmail: map[string]*Birthday{}, byCharity: map[string]*Charity{},
	}
	admins := []string{}
	for _, row := range tables[adminsTab] {
		admins = append(admins, row["Email"])
	}
	model.Admins = config.NormalizeEmails(admins)
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
		if model.Charity(name) != nil {
			return fail(fmt.Errorf("is listed twice"))
		}
		if err := checkURL(row["Donation Link"]); err != nil {
			return fail(err)
		}
		if len(row["About"]) > maxTextLength {
			return fail(fmt.Errorf("about is too long"))
		}
		allowed, err := yesNo(row["Allowed"])
		if err != nil {
			return fail(fmt.Errorf("allowed %w", err))
		}
		if err := checkDate("added on", row["Added On"]); err != nil {
			return fail(err)
		}
		model.Charities = append(model.Charities, Charity{
			Name: name, DonationLink: row["Donation Link"], About: row["About"], EIN: row["EIN"],
			Allowed: allowed, WhyNotAllowed: row["Why Not Allowed"], AddedOn: row["Added On"],
		})
	}
	for i := range model.Charities {
		model.byCharity[model.Charities[i].Name] = &model.Charities[i]
	}
	if c := model.Charity(settings.DefaultCharity); c == nil || !c.Allowed {
		return nil, fmt.Errorf("setting %q names %q, which is not an allowed charity", DefaultCharityKey, settings.DefaultCharity)
	}

	for _, row := range tables[newsletterDatesTab] {
		if _, err := ParseDate(row["Date"]); err != nil {
			return nil, fmt.Errorf("newsletter date %w", err)
		}
		if slices.Contains(model.NewsletterDates, row["Date"]) {
			return nil, fmt.Errorf("newsletter date %q is listed twice", row["Date"])
		}
		model.NewsletterDates = append(model.NewsletterDates, row["Date"])
	}
	sort.Strings(model.NewsletterDates)

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
		if err := checkDate("newsletter override", row["Newsletter Override"]); err != nil {
			return fail(err)
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
			return fail(fmt.Errorf("names unknown charity %q", row["Charity"]))
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
	return model, nil
}
