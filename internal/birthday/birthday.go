// Package birthday serves the Helios Birthday Team app: every staff member's
// birthday walked through outreach, a charity choice, and the newsletter.
package birthday

import (
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"heliosian/internal/data"
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
	changeLogTab       = "Change Log"
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
)

var settingKeys = []string{DefaultCharityKey, YearStartKey, EmailSubjectKey, EmailBodyKey, NoNewsletterNoteKey}

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
	ChangeLogColumns      = []string{"Timestamp", "Actor", "Action", "Kind", "Email", "Year", "Details"}
)

var emailForm = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// Birthday is one staff member's row: their birthday, the newsletter that
// should carry it when the usual pick is wrong, and their standing wish about
// taking part. Someone who opted out entirely may have no birthday on file.
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
	Settings        Settings
	byEmail         map[string]*Birthday
	byCharity       map[string]*Charity
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

type Tables struct {
	Birthdays       []map[string]string
	Assignments     []map[string]string
	Outreach        []map[string]string
	Donations       []map[string]string
	Notes           []map[string]string
	Charities       []map[string]string
	NewsletterDates []map[string]string
	Settings        []map[string]string
	Admins          []map[string]string
}

func ReadTables(source data.Source) (*Tables, error) {
	type table struct {
		name   string
		want   []string
		header []string
		rows   []map[string]string
		err    error
	}
	birthdays := &table{name: birthdaysTab, want: BirthdayColumns}
	assignments := &table{name: assignmentsTab, want: AssignmentColumns}
	outreach := &table{name: outreachTab, want: OutreachColumns}
	donations := &table{name: donationsTab, want: DonationColumns}
	notes := &table{name: notesTab, want: NoteColumns}
	charities := &table{name: charitiesTab, want: CharityColumns}
	dates := &table{name: newsletterDatesTab, want: NewsletterDateColumns}
	settings := &table{name: settingsTab, want: SettingColumns}
	admins := &table{name: adminsTab, want: AdminColumns}
	changeLog := &table{name: changeLogTab, want: ChangeLogColumns}
	read := []*table{birthdays, assignments, outreach, donations, notes, charities, dates, settings, admins}
	var wg sync.WaitGroup
	for _, t := range read {
		wg.Go(func() {
			t.header, t.rows, t.err = source.Table(appName, t.name)
		})
	}
	wg.Go(func() {
		changeLog.header, changeLog.err = source.Header(appName, changeLog.name)
	})
	wg.Wait()
	for _, t := range append(read, changeLog) {
		if t.err != nil {
			return nil, t.err
		}
		if err := data.CheckColumns(t.name, t.header, t.want); err != nil {
			return nil, err
		}
	}
	return &Tables{
		Birthdays: birthdays.rows, Assignments: assignments.rows,
		Outreach: outreach.rows, Donations: donations.rows, Notes: notes.rows, Charities: charities.rows,
		NewsletterDates: dates.rows, Settings: settings.rows, Admins: admins.rows,
	}, nil
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
		if !slices.Contains(settingKeys, key) {
			return Settings{}, fmt.Errorf("%s has unknown key %q", settingsTab, key)
		}
		if _, dup := values[key]; dup {
			return Settings{}, fmt.Errorf("%s has duplicate key %q", settingsTab, key)
		}
		values[key] = row["Value"]
	}
	for _, key := range settingKeys {
		if values[key] == "" {
			return Settings{}, fmt.Errorf("%s is missing %q", settingsTab, key)
		}
	}
	if _, _, err := ParseMonthDay(values[YearStartKey]); err != nil {
		return Settings{}, fmt.Errorf("setting %q: %w", YearStartKey, err)
	}
	return Settings{
		DefaultCharity: values[DefaultCharityKey], YearStart: values[YearStartKey],
		EmailSubject: values[EmailSubjectKey], EmailBody: values[EmailBodyKey], NoNewsletterNote: values[NoNewsletterNoteKey],
	}, nil
}

// BuildModel validates every row and refuses the whole set on the first problem,
// the stance every app here takes: a sheet edit that breaks a rule surfaces as a
// refused load, never as a page quietly missing a birthday.
func BuildModel(tables *Tables) (*Model, error) {
	settings, err := parseSettings(tables.Settings)
	if err != nil {
		return nil, err
	}
	model := &Model{
		Birthdays: []Birthday{}, Assignments: map[string]Assignment{},
		Outreach: map[string]Outreach{}, Donations: map[string]Donation{}, Notes: []Note{}, Charities: []Charity{},
		NewsletterDates: []string{}, Settings: settings, byEmail: map[string]*Birthday{}, byCharity: map[string]*Charity{},
	}
	for _, row := range tables.Charities {
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

	for _, row := range tables.NewsletterDates {
		if _, err := ParseDate(row["Date"]); err != nil {
			return nil, fmt.Errorf("newsletter date %w", err)
		}
		if slices.Contains(model.NewsletterDates, row["Date"]) {
			return nil, fmt.Errorf("newsletter date %q is listed twice", row["Date"])
		}
		model.NewsletterDates = append(model.NewsletterDates, row["Date"])
	}
	sort.Strings(model.NewsletterDates)

	for _, row := range tables.Birthdays {
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
		if err := checkDate("birthday", row["Birthday"]); err != nil {
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

	for _, row := range tables.Assignments {
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

	for _, row := range tables.Outreach {
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

	for _, row := range tables.Donations {
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

	for _, row := range tables.Notes {
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

func cloneRows(rows []map[string]string) []map[string]string {
	out := make([]map[string]string, len(rows))
	for i, row := range rows {
		out[i] = maps.Clone(row)
	}
	return out
}

func applyCells(row, cells map[string]string) {
	for column, value := range cells {
		if value == "" {
			delete(row, column)
			continue
		}
		row[column] = value
	}
}

func rowMatches(row, match map[string]string) bool {
	for column, value := range match {
		if !strings.EqualFold(strings.TrimSpace(row[column]), value) {
			return false
		}
	}
	return true
}

func (t *Tables) tab(name string) []map[string]string {
	switch name {
	case birthdaysTab:
		return t.Birthdays
	case assignmentsTab:
		return t.Assignments
	case outreachTab:
		return t.Outreach
	case donationsTab:
		return t.Donations
	case notesTab:
		return t.Notes
	case charitiesTab:
		return t.Charities
	case newsletterDatesTab:
		return t.NewsletterDates
	case settingsTab:
		return t.Settings
	}
	return t.Admins
}

func (t *Tables) setTab(name string, rows []map[string]string) {
	switch name {
	case birthdaysTab:
		t.Birthdays = rows
	case assignmentsTab:
		t.Assignments = rows
	case outreachTab:
		t.Outreach = rows
	case donationsTab:
		t.Donations = rows
	case notesTab:
		t.Notes = rows
	case charitiesTab:
		t.Charities = rows
	case newsletterDatesTab:
		t.NewsletterDates = rows
	case settingsTab:
		t.Settings = rows
	default:
		t.Admins = rows
	}
}

// with mirrors what data.Writer.Set is about to do to one tab, so the model can be
// rebuilt and checked before anything is persisted. A nil match appends.
func (t *Tables) with(tab string, match, cells map[string]string) *Tables {
	out := *t
	rows := cloneRows(t.tab(tab))
	found := false
	for _, row := range rows {
		if match != nil && rowMatches(row, match) {
			applyCells(row, cells)
			found = true
		}
	}
	if !found {
		row := map[string]string{}
		applyCells(row, match)
		applyCells(row, cells)
		rows = append(rows, row)
	}
	out.setTab(tab, rows)
	return &out
}

func (t *Tables) without(tab string, match map[string]string) *Tables {
	out := *t
	rows := []map[string]string{}
	for _, row := range t.tab(tab) {
		if !rowMatches(row, match) {
			rows = append(rows, row)
		}
	}
	out.setTab(tab, rows)
	return &out
}

func (t *Tables) count(tab string, match map[string]string) int {
	n := 0
	for _, row := range t.tab(tab) {
		if rowMatches(row, match) {
			n++
		}
	}
	return n
}

func (t *Tables) withAdmins(emails []string) *Tables {
	out := *t
	out.Admins = make([]map[string]string, 0, len(emails))
	for _, email := range emails {
		out.Admins = append(out.Admins, map[string]string{"Email": email})
	}
	return &out
}
