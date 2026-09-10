// Package events serves the HCA volunteer portal: what the community association
// runs each school year, and who signed up to help.
package events

import (
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"heliosian/internal/data"
)

const (
	appName       = "events"
	categoriesTab = "Categories"
	activitiesTab = "Activities"
	rolesTab      = "Roles"
	volunteersTab = "Volunteers"
	linksTab      = "Links"
	settingsTab   = "Settings"
	adminsTab     = "Admins"
	changeLogTab  = "Change Log"
)

const (
	DateFormat     = "2006-01-02"
	DateTimeFormat = "2006-01-02 15:04"
	maxTitleLength = 120
	maxTextLength  = 6000
	maxURLLength   = 1000
)

const (
	StatusPending = "Pending"
	StatusOpen    = "Open"
	StatusDone    = "Done"
	StatusHidden  = "Hidden"
)

var Statuses = []string{StatusPending, StatusOpen, StatusDone, StatusHidden}

const (
	PositionVolunteer = "Volunteer"
	PositionOpen      = "Open to Co-Chair"
	PositionCoChair   = "Co-Chair"
)

var Positions = []string{PositionVolunteer, PositionOpen, PositionCoChair}

const (
	ExpenseFormKey = "Expense Form URL"
	IntroKey       = "Intro"
)

var settingKeys = []string{ExpenseFormKey, IntroKey}

var (
	CategoryColumns  = []string{"Title", "Description"}
	ActivityColumns  = []string{"Year", "Title", "Category", "Status", "Description", "Image", "Timing", "Start", "End", "Location", "Spots", "Co-Leader Needed", "Volunteers Hidden", "Direct Sign-Up", "Added By", "Added"}
	RoleColumns      = []string{"Year", "Activity", "Parent", "Title", "Group", "Status", "Description", "Image", "Start", "End", "Spots", "Co-Leader Needed", "Volunteers Hidden", "Added By", "Added"}
	VolunteerColumns = []string{"Year", "Activity", "Role", "Email", "Position", "Note", "Added By", "Added"}
	LinkColumns      = []string{"Year", "Activity", "Role", "Title", "URL", "Image"}
	SettingColumns   = []string{"Key", "Value"}
	AdminColumns     = []string{"Email"}
	ChangeLogColumns = []string{"Timestamp", "Actor", "Action", "Kind", "Year", "Activity", "Role", "Title", "Email", "Details"}
)

var yearForm = regexp.MustCompile(`^(\d{4}) - (\d{4})$`)

var emailForm = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type ImageChecker interface {
	Has(key string) (bool, error)
	Prefetch(names []string) error
}

type Link struct {
	Title    string `json:"title"`
	URL      string `json:"url"`
	Image    string `json:"image,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
}

type Volunteer struct {
	Email    string `json:"email"`
	Position string `json:"position"`
	Note     string `json:"note,omitempty"`
	AddedBy  string `json:"addedBy,omitempty"`
	Added    string `json:"added,omitempty"`
	Name     string `json:"name,omitempty"`
	PhotoURL string `json:"photoUrl,omitempty"`
}

type Role struct {
	Year             string      `json:"year"`
	Activity         string      `json:"activity"`
	Parent           string      `json:"parent,omitempty"`
	Title            string      `json:"title"`
	Group            string      `json:"group,omitempty"`
	Status           string      `json:"status"`
	Description      string      `json:"description,omitempty"`
	Image            string      `json:"image,omitempty"`
	ImageURL         string      `json:"imageUrl,omitempty"`
	Start            string      `json:"start,omitempty"`
	End              string      `json:"end,omitempty"`
	Spots            int         `json:"spots,omitempty"`
	CoLeaderNeeded   bool        `json:"coLeaderNeeded"`
	VolunteersHidden bool        `json:"volunteersHidden"`
	AddedBy          string      `json:"addedBy,omitempty"`
	Added            string      `json:"added,omitempty"`
	Roles            []*Role     `json:"roles"`
	Links            []Link      `json:"links"`
	Volunteers       []Volunteer `json:"volunteers"`
}

type Activity struct {
	Year             string      `json:"year"`
	Title            string      `json:"title"`
	Category         string      `json:"category"`
	Status           string      `json:"status"`
	Description      string      `json:"description,omitempty"`
	Image            string      `json:"image,omitempty"`
	ImageURL         string      `json:"imageUrl,omitempty"`
	Timing           string      `json:"timing,omitempty"`
	Start            string      `json:"start,omitempty"`
	End              string      `json:"end,omitempty"`
	Location         string      `json:"location,omitempty"`
	Spots            int         `json:"spots,omitempty"`
	CoLeaderNeeded   bool        `json:"coLeaderNeeded"`
	VolunteersHidden bool        `json:"volunteersHidden"`
	DirectSignUp     bool        `json:"directSignUp"`
	AddedBy          string      `json:"addedBy,omitempty"`
	Added            string      `json:"added,omitempty"`
	Roles            []*Role     `json:"roles"`
	Links            []Link      `json:"links"`
	Volunteers       []Volunteer `json:"volunteers"`
	roles            map[string]*Role
}

type Category struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

type Settings struct {
	ExpenseFormURL string `json:"expenseFormUrl"`
	Intro          string `json:"intro"`
}

// Model is the sheet organized: categories in row order, activities in row order
// holding their roles as the tree the Parent column describes, and every
// volunteer under the activity or role they signed up for.
type Model struct {
	Categories []Category  `json:"categories"`
	Activities []*Activity `json:"activities"`
	Settings   Settings    `json:"settings"`
	byKey      map[string]*Activity
}

func activityKey(year, title string) string {
	return year + "\x00" + title
}

func (m *Model) Activity(year, title string) *Activity {
	return m.byKey[activityKey(year, title)]
}

func (m *Model) HasCategory(title string) bool {
	for _, c := range m.Categories {
		if c.Title == title {
			return true
		}
	}
	return false
}

// Role finds a role anywhere in the activity's tree by its title, which is
// unique within the activity.
func (a *Activity) Role(title string) *Role {
	return a.roles[title]
}

// AllRoles walks the tree in row order.
func (a *Activity) AllRoles() []*Role {
	out := []*Role{}
	var walk func([]*Role)
	walk = func(roles []*Role) {
		for _, r := range roles {
			out = append(out, r)
			walk(r.Roles)
		}
	}
	walk(a.Roles)
	return out
}

// CoChairs lists who chairs the activity itself.
func (a *Activity) CoChairs() []string {
	out := []string{}
	for _, v := range a.Volunteers {
		if v.Position == PositionCoChair {
			out = append(out, v.Email)
		}
	}
	return out
}

func (a *Activity) IsCoChair(email string) bool {
	return slices.Contains(a.CoChairs(), strings.ToLower(email))
}

type Tables struct {
	Categories []map[string]string
	Activities []map[string]string
	Roles      []map[string]string
	Volunteers []map[string]string
	Links      []map[string]string
	Settings   []map[string]string
	Admins     []map[string]string
}

func ReadTables(source data.Source) (*Tables, error) {
	type table struct {
		name   string
		want   []string
		header []string
		rows   []map[string]string
		err    error
	}
	categories := &table{name: categoriesTab, want: CategoryColumns}
	activities := &table{name: activitiesTab, want: ActivityColumns}
	roles := &table{name: rolesTab, want: RoleColumns}
	volunteers := &table{name: volunteersTab, want: VolunteerColumns}
	links := &table{name: linksTab, want: LinkColumns}
	settings := &table{name: settingsTab, want: SettingColumns}
	admins := &table{name: adminsTab, want: AdminColumns}
	changeLog := &table{name: changeLogTab, want: ChangeLogColumns}
	read := []*table{categories, activities, roles, volunteers, links, settings, admins}
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
		Categories: categories.rows, Activities: activities.rows, Roles: roles.rows,
		Volunteers: volunteers.rows, Links: links.rows, Settings: settings.rows, Admins: admins.rows,
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

// ParseWhen reads a Start or End cell: a day, or a day with a wall-clock time.
func ParseWhen(cell string) (time.Time, error) {
	if t, err := time.Parse(DateTimeFormat, cell); err == nil {
		return t, nil
	}
	if t, err := time.Parse(DateFormat, cell); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("%q is not a date like 2026-09-24 or 2026-09-24 16:00", cell)
}

func checkSpan(start, end string) error {
	var from time.Time
	if start != "" {
		t, err := ParseWhen(start)
		if err != nil {
			return fmt.Errorf("start %w", err)
		}
		from = t
	}
	if end == "" {
		return nil
	}
	to, err := ParseWhen(end)
	if err != nil {
		return fmt.Errorf("end %w", err)
	}
	if start == "" {
		return fmt.Errorf("has an end but no start")
	}
	if to.Before(from) {
		return fmt.Errorf("ends before it starts")
	}
	return nil
}

func checkAdded(cell string) error {
	if cell == "" {
		return nil
	}
	if _, err := time.Parse(DateFormat, cell); err != nil {
		return fmt.Errorf("added date %q is not like 2026-09-24", cell)
	}
	return nil
}

func parseSpots(cell string) (int, error) {
	if cell == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(cell)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("spots %q must be a positive whole number or blank", cell)
	}
	return n, nil
}

func checkStatus(cell string) error {
	if !slices.Contains(Statuses, cell) {
		return fmt.Errorf("status %q is not one of %s", cell, strings.Join(Statuses, ", "))
	}
	return nil
}

// CheckYear accepts a school year written as its two calendar years.
func CheckYear(year string) error {
	m := yearForm.FindStringSubmatch(year)
	if m == nil {
		return fmt.Errorf("year %q is not like 2026 - 2027", year)
	}
	from, _ := strconv.Atoi(m[1])
	to, _ := strconv.Atoi(m[2])
	if to != from+1 {
		return fmt.Errorf("year %q does not span consecutive years", year)
	}
	return nil
}

// SchoolYear is the year a date falls in: school years turn over on July 1.
func SchoolYear(t time.Time) string {
	start := t.Year()
	if t.Month() < time.July {
		start--
	}
	return fmt.Sprintf("%d - %d", start, start+1)
}

// ShiftYear moves a well-formed year by n years.
func ShiftYear(year string, n int) string {
	m := yearForm.FindStringSubmatch(year)
	from, _ := strconv.Atoi(m[1])
	return fmt.Sprintf("%d - %d", from+n, from+n+1)
}

func imageURL(images ImageChecker, name string) (string, error) {
	if name == "" {
		return "", nil
	}
	found, err := images.Has(name)
	if err != nil {
		return "", fmt.Errorf("image %q: %w", name, err)
	}
	if !found {
		return "", fmt.Errorf("image %q does not exist", name)
	}
	return "/" + name, nil
}

func checkTitle(kind, title string) error {
	if title == "" {
		return fmt.Errorf("%s has no title", kind)
	}
	if len(title) > maxTitleLength {
		return fmt.Errorf("%s title %q is too long", kind, title)
	}
	if title != strings.TrimSpace(title) {
		return fmt.Errorf("%s title %q has surrounding spaces", kind, title)
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
	if !strings.HasPrefix(values[ExpenseFormKey], "https://") {
		return Settings{}, fmt.Errorf("setting %q must be a full https:// url", ExpenseFormKey)
	}
	return Settings{ExpenseFormURL: values[ExpenseFormKey], Intro: values[IntroKey]}, nil
}

// BuildModel validates every row and refuses the whole set on the first problem,
// the stance every app here takes: a sheet edit that breaks a rule surfaces as a
// refused load, never as a page quietly missing an event.
func imageNames(rows ...[]map[string]string) []string {
	names := []string{}
	for _, table := range rows {
		for _, row := range table {
			if row["Image"] != "" {
				names = append(names, row["Image"])
			}
		}
	}
	return names
}

func BuildModel(tables *Tables, images ImageChecker) (*Model, error) {
	settings, err := parseSettings(tables.Settings)
	if err != nil {
		return nil, err
	}
	if err := images.Prefetch(imageNames(tables.Activities, tables.Roles, tables.Links)); err != nil {
		return nil, err
	}
	model := &Model{Categories: []Category{}, Activities: []*Activity{}, Settings: settings, byKey: map[string]*Activity{}}
	for _, row := range tables.Categories {
		title := row["Title"]
		if err := checkTitle("category", title); err != nil {
			return nil, err
		}
		if model.HasCategory(title) {
			return nil, fmt.Errorf("duplicate category %q", title)
		}
		model.Categories = append(model.Categories, Category{Title: title, Description: row["Description"]})
	}

	for _, row := range tables.Activities {
		a, err := parseActivity(row, model, images)
		if err != nil {
			return nil, err
		}
		if _, dup := model.byKey[activityKey(a.Year, a.Title)]; dup {
			return nil, fmt.Errorf("duplicate activity %q in %s", a.Title, a.Year)
		}
		model.byKey[activityKey(a.Year, a.Title)] = a
		model.Activities = append(model.Activities, a)
	}

	for _, row := range tables.Roles {
		r, err := parseRole(row, images)
		if err != nil {
			return nil, err
		}
		a := model.Activity(r.Year, r.Activity)
		if a == nil {
			return nil, fmt.Errorf("role %q names unknown activity %q in %s", r.Title, r.Activity, r.Year)
		}
		if _, dup := a.roles[r.Title]; dup {
			return nil, fmt.Errorf("duplicate role %q in %s (%s)", r.Title, a.Title, a.Year)
		}
		a.roles[r.Title] = r
	}
	for _, a := range model.Activities {
		if err := attachRoles(a, tables.Roles); err != nil {
			return nil, err
		}
	}

	seen := map[string]bool{}
	for _, row := range tables.Volunteers {
		year, activity, role := row["Year"], row["Activity"], row["Role"]
		email := strings.ToLower(row["Email"])
		if !emailForm.MatchString(email) {
			return nil, fmt.Errorf("volunteer row %v has invalid email", row)
		}
		a := model.Activity(year, activity)
		if a == nil {
			return nil, fmt.Errorf("volunteer %s names unknown activity %q in %s", email, activity, year)
		}
		if !slices.Contains(Positions, row["Position"]) {
			return nil, fmt.Errorf("volunteer %s on %q: position %q is not one of %s", email, activity, row["Position"], strings.Join(Positions, ", "))
		}
		if err := checkAdded(row["Added"]); err != nil {
			return nil, fmt.Errorf("volunteer %s on %q: %w", email, activity, err)
		}
		key := activityKey(year, activity) + "\x00" + role + "\x00" + email
		if seen[key] {
			return nil, fmt.Errorf("volunteer %s is listed twice on %q %q in %s", email, activity, role, year)
		}
		seen[key] = true
		v := Volunteer{Email: email, Position: row["Position"], Note: row["Note"], AddedBy: strings.ToLower(row["Added By"]), Added: row["Added"]}
		if role == "" {
			a.Volunteers = append(a.Volunteers, v)
			continue
		}
		r := a.Role(role)
		if r == nil {
			return nil, fmt.Errorf("volunteer %s names unknown role %q on %q in %s", email, role, activity, year)
		}
		r.Volunteers = append(r.Volunteers, v)
	}

	linkKeys := map[string]bool{}
	for _, row := range tables.Links {
		year, activity, role, title := row["Year"], row["Activity"], row["Role"], row["Title"]
		if err := checkTitle("link", title); err != nil {
			return nil, err
		}
		a := model.Activity(year, activity)
		if a == nil {
			return nil, fmt.Errorf("link %q names unknown activity %q in %s", title, activity, year)
		}
		if err := checkURL(row["URL"]); err != nil {
			return nil, fmt.Errorf("link %q on %q: %w", title, activity, err)
		}
		image, err := imageURL(images, row["Image"])
		if err != nil {
			return nil, fmt.Errorf("link %q on %q: %w", title, activity, err)
		}
		key := activityKey(year, activity) + "\x00" + role + "\x00" + title
		if linkKeys[key] {
			return nil, fmt.Errorf("duplicate link %q on %q %q in %s", title, activity, role, year)
		}
		linkKeys[key] = true
		l := Link{Title: title, URL: row["URL"], Image: row["Image"], ImageURL: image}
		if role == "" {
			a.Links = append(a.Links, l)
			continue
		}
		r := a.Role(role)
		if r == nil {
			return nil, fmt.Errorf("link %q names unknown role %q on %q in %s", title, role, activity, year)
		}
		r.Links = append(r.Links, l)
	}
	return model, nil
}

func parseActivity(row map[string]string, model *Model, images ImageChecker) (*Activity, error) {
	title, year := row["Title"], row["Year"]
	if err := checkTitle("activity", title); err != nil {
		return nil, err
	}
	fail := func(err error) (*Activity, error) {
		return nil, fmt.Errorf("activity %q in %s: %w", title, year, err)
	}
	if err := CheckYear(year); err != nil {
		return fail(err)
	}
	if !model.HasCategory(row["Category"]) {
		return fail(fmt.Errorf("names unknown category %q", row["Category"]))
	}
	if err := checkStatus(row["Status"]); err != nil {
		return fail(err)
	}
	if len(row["Description"]) > maxTextLength {
		return fail(fmt.Errorf("description is too long"))
	}
	if err := checkSpan(row["Start"], row["End"]); err != nil {
		return fail(err)
	}
	if err := checkAdded(row["Added"]); err != nil {
		return fail(err)
	}
	spots, err := parseSpots(row["Spots"])
	if err != nil {
		return fail(err)
	}
	image, err := imageURL(images, row["Image"])
	if err != nil {
		return fail(err)
	}
	coLeader, err := yesNo(row["Co-Leader Needed"])
	if err != nil {
		return fail(fmt.Errorf("co-leader needed %w", err))
	}
	hidden, err := yesNo(row["Volunteers Hidden"])
	if err != nil {
		return fail(fmt.Errorf("volunteers hidden %w", err))
	}
	direct, err := yesNo(row["Direct Sign-Up"])
	if err != nil {
		return fail(fmt.Errorf("direct sign-up %w", err))
	}
	return &Activity{
		Year: year, Title: title, Category: row["Category"], Status: row["Status"],
		Description: row["Description"], Image: row["Image"], ImageURL: image,
		Timing: row["Timing"], Start: row["Start"], End: row["End"], Location: row["Location"], Spots: spots,
		CoLeaderNeeded: coLeader, VolunteersHidden: hidden, DirectSignUp: direct,
		AddedBy: strings.ToLower(row["Added By"]), Added: row["Added"],
		Roles: []*Role{}, Links: []Link{}, Volunteers: []Volunteer{}, roles: map[string]*Role{},
	}, nil
}

func parseRole(row map[string]string, images ImageChecker) (*Role, error) {
	title := row["Title"]
	if err := checkTitle("role", title); err != nil {
		return nil, err
	}
	fail := func(err error) (*Role, error) {
		return nil, fmt.Errorf("role %q on %q in %s: %w", title, row["Activity"], row["Year"], err)
	}
	if err := checkStatus(row["Status"]); err != nil {
		return fail(err)
	}
	if len(row["Description"]) > maxTextLength {
		return fail(fmt.Errorf("description is too long"))
	}
	if err := checkSpan(row["Start"], row["End"]); err != nil {
		return fail(err)
	}
	if err := checkAdded(row["Added"]); err != nil {
		return fail(err)
	}
	spots, err := parseSpots(row["Spots"])
	if err != nil {
		return fail(err)
	}
	image, err := imageURL(images, row["Image"])
	if err != nil {
		return fail(err)
	}
	coLeader, err := yesNo(row["Co-Leader Needed"])
	if err != nil {
		return fail(fmt.Errorf("co-leader needed %w", err))
	}
	hidden, err := yesNo(row["Volunteers Hidden"])
	if err != nil {
		return fail(fmt.Errorf("volunteers hidden %w", err))
	}
	if row["Parent"] == title {
		return fail(fmt.Errorf("is its own parent"))
	}
	return &Role{
		Year: row["Year"], Activity: row["Activity"], Parent: row["Parent"], Title: title, Group: row["Group"],
		Status: row["Status"], Description: row["Description"], Image: row["Image"], ImageURL: image,
		Start: row["Start"], End: row["End"], Spots: spots, CoLeaderNeeded: coLeader, VolunteersHidden: hidden,
		AddedBy: strings.ToLower(row["Added By"]), Added: row["Added"],
		Roles: []*Role{}, Links: []Link{}, Volunteers: []Volunteer{},
	}, nil
}

// attachRoles hangs an activity's roles on their parents in row order and refuses
// a parent that does not exist or a chain that loops back on itself.
func attachRoles(a *Activity, rows []map[string]string) error {
	for _, row := range rows {
		if row["Year"] != a.Year || row["Activity"] != a.Title {
			continue
		}
		r := a.roles[row["Title"]]
		if r.Parent == "" {
			a.Roles = append(a.Roles, r)
			continue
		}
		parent := a.roles[r.Parent]
		if parent == nil {
			return fmt.Errorf("role %q on %q in %s names unknown parent %q", r.Title, a.Title, a.Year, r.Parent)
		}
		parent.Roles = append(parent.Roles, r)
	}
	for _, r := range a.roles {
		seen := map[string]bool{}
		for p := r; p.Parent != ""; p = a.roles[p.Parent] {
			if seen[p.Title] {
				return fmt.Errorf("role %q on %q in %s is inside a parent loop", r.Title, a.Title, a.Year)
			}
			seen[p.Title] = true
		}
	}
	return nil
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
	case categoriesTab:
		return t.Categories
	case activitiesTab:
		return t.Activities
	case rolesTab:
		return t.Roles
	case volunteersTab:
		return t.Volunteers
	case linksTab:
		return t.Links
	case settingsTab:
		return t.Settings
	}
	return t.Admins
}

func (t *Tables) setTab(name string, rows []map[string]string) {
	switch name {
	case categoriesTab:
		t.Categories = rows
	case activitiesTab:
		t.Activities = rows
	case rolesTab:
		t.Roles = rows
	case volunteersTab:
		t.Volunteers = rows
	case linksTab:
		t.Links = rows
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
