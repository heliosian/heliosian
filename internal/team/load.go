package team

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/config"
	"heliosian/internal/store"
)

const (
	appName       = "events"
	categoriesTab = "Categories"
	activitiesTab = "Activities"
	volunteersTab = "Volunteers"
	linksTab      = "Links"
	settingsTab   = "Settings"
	adminsTab     = "Admins"
	redirectsTab  = "Redirects"
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

var legacyThemeKeys = []string{"Sidebar Color", "Sidebar Color 2", "Sidebar Text Color", "Page Color", "Page Color 2", "Logo", "Sidebar Image"}

const CompleteColumn = "Volunteers Complete"

// PriorityColumn is an activity's admin-only Priority flag (Activity.Priority).
const PriorityColumn = "Priority"

var (
	CategoryColumns  = []string{"Category ID", "Event ID", "Title", "Description", "Image", "Allow Adding", "Show On Main Page", store.OrderColumn}
	ActivityColumns  = []string{"Event ID", "Year", "Title", "Parent", "Category", "Status", "Description", "Image", "Timing", "Start", "End", "Location", "Spots", "Co-Leader Needed", "Volunteers Hidden", "Direct Sign-Up", "Pretty ID", "Allow Adding", "Flyer Image", "Highlight Headline", "Highlight Body", "Highlight Icon", "Added By", "Added", store.OrderColumn, CompleteColumn, PriorityColumn}
	VolunteerColumns = []string{"Event ID", "Email", "Position", "Note", "Added By", "Added"}
	LinkColumns      = []string{"Event ID", "Title", "URL", "Image", "Description"}
	SettingColumns   = []string{"Key", "Value"}
	RedirectColumns  = []string{"Type", "Old", "New", "Date"}
	AdminColumns     = []string{"Email"}
)

var yearForm = regexp.MustCompile(`^(\d{4}) - (\d{4})$`)

var prettyForm = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const maxPrettyLength = 40

func NormalizePretty(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func CheckPretty(pretty string) error {
	if pretty == "" {
		return nil
	}
	if len(pretty) > maxPrettyLength {
		return fmt.Errorf("pretty id %q is too long", pretty)
	}
	if !prettyForm.MatchString(pretty) {
		return fmt.Errorf("pretty id %q is not lower-case letters, digits and hyphens", pretty)
	}
	return nil
}

var emailForm = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type ImageChecker interface {
	Has(key string) (bool, error)
	Prefetch(ctx context.Context, names []string) error
}

type Link struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	Image       string `json:"image,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
}

type Highlight struct {
	Headline string `json:"headline"`
	Body     string `json:"body"`
	Icon     string `json:"icon,omitempty"`
}

func highlightOf(row map[string]string) *Highlight {
	h := &Highlight{Headline: strings.TrimSpace(row["Highlight Headline"]), Body: strings.TrimSpace(row["Highlight Body"]), Icon: strings.TrimSpace(row["Highlight Icon"])}
	if h.Headline == "" && h.Body == "" {
		return nil
	}
	return h
}

type Volunteer struct {
	Email       string `json:"email"`
	Position    string `json:"position"`
	Note        string `json:"note,omitempty"`
	RSVP        string `json:"rsvp,omitempty"`
	AddedBy     string `json:"addedBy,omitempty"`
	AddedByName string `json:"addedByName,omitempty"`
	Added       string `json:"added,omitempty"`
	Name        string `json:"name,omitempty"`
	PhotoURL    string `json:"photoUrl,omitempty"`
	Grade       string `json:"grade,omitempty"`
}

type Activity struct {
	ID                 string     `json:"id"`
	Year               string     `json:"year"`
	Title              string     `json:"title"`
	Parent             string     `json:"parent,omitempty"`
	Category           string     `json:"category,omitempty"`
	Status             string     `json:"status"`
	Description        string     `json:"description,omitempty"`
	Image              string     `json:"image,omitempty"`
	ImageURL           string     `json:"imageUrl,omitempty"`
	Flyer              string     `json:"flyer,omitempty"`
	FlyerURL           string     `json:"flyerUrl,omitempty"`
	Highlight          *Highlight `json:"highlight,omitempty"`
	Order              string     `json:"-"`
	Timing             string     `json:"timing,omitempty"`
	Start              string     `json:"start,omitempty"`
	End                string     `json:"end,omitempty"`
	Location           string     `json:"location,omitempty"`
	Spots              int        `json:"spots,omitempty"`
	CoLeaderNeeded     bool       `json:"coLeaderNeeded"`
	VolunteersComplete bool       `json:"volunteersComplete"`
	VolunteersHidden   bool       `json:"volunteersHidden"`
	DirectSignUp       bool       `json:"directSignUp"`
	// Priority is an admin's mark on something the community most needs
	// hands for, which Heliosian's Team widget lists under its own chip.
	Priority    bool        `json:"priority"`
	PrettyID    string      `json:"prettyId,omitempty"`
	AllowAdding string      `json:"allowAddingOwn,omitempty"`
	Adding      string      `json:"allowAdding"`
	AddedBy     string      `json:"addedBy,omitempty"`
	Added       string      `json:"added,omitempty"`
	Children    []*Activity `json:"children"`
	Links       []Link      `json:"links"`
	Volunteers  []Volunteer `json:"volunteers"`
	Categories  []Category  `json:"categories,omitempty"`
	// Set on ActivityFor's copy alone: every sign-up, withheld ones included.
	Taken int `json:"-"`
}

type Category struct {
	ID          string `json:"id"`
	EventID     string `json:"eventId,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Image       string `json:"image,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
	AllowAdding string `json:"allowAddingOwn,omitempty"`
	Adding      string `json:"allowAdding"`
	ShowOnMain  bool   `json:"showOnMain"`
	BuiltIn     bool   `json:"builtIn,omitempty"`
	Order       string `json:"-"`
}

const (
	AddingYes      = "Yes"
	AddingApproval = "Approval Needed"
	AddingNo       = "No"
)

var AddingPolicies = []string{AddingYes, AddingApproval, AddingNo}

func checkAdding(cell string) (string, error) {
	cell = strings.TrimSpace(cell)
	if cell == "" || slices.Contains(AddingPolicies, cell) {
		return cell, nil
	}
	return "", fmt.Errorf("allow adding %q is not %s, or blank", cell, strings.Join(AddingPolicies, ", "))
}

const UncategorizedID = "uncategorized"

func uncategorized() *Category {
	return &Category{ID: UncategorizedID, Title: "Uncategorized",
		Description: "Things that have not been sorted into a category yet", Adding: AddingNo, ShowOnMain: true, BuiltIn: true}
}

type Settings struct {
	ExpenseFormURL string `json:"expenseFormUrl"`
	Intro          string `json:"intro"`
}

type Redirect struct {
	Type string `json:"type"`
	Old  string `json:"old"`
	New  string `json:"new"`
	Date string `json:"date,omitempty"`
	cell string
}

const (
	RedirectActivity = "Activity"
	RedirectAdmin    = "Admin"
)

func redirectPath(cell string) string {
	path := strings.TrimSpace(cell)
	if path == "" {
		return ""
	}
	if isURL(path) {
		u, err := url.Parse(path)
		if err != nil {
			return ""
		}
		path = u.Path
		if path == "" {
			path = "/"
		}
	}
	if !strings.HasPrefix(path, "/") {
		path = "/v/" + path
	}
	if path != "/" {
		path = strings.TrimRight(path, "/")
	}
	return path
}

func redirectTo(cell string) string {
	to := strings.TrimSpace(cell)
	if isURL(to) {
		return to
	}
	return redirectPath(to)
}

func isURL(s string) bool {
	return strings.HasPrefix(strings.ToLower(s), "http://") || strings.HasPrefix(strings.ToLower(s), "https://")
}

type Model struct {
	Categories []Category  `json:"categories"`
	Activities []*Activity `json:"activities"`
	Settings   Settings    `json:"settings"`
	Redirects  []Redirect  `json:"redirects"`
	Skipped    Skipped     `json:"-"`
	byID       map[string]*Activity
	categories map[string]*Category
	pretty     map[string]*Activity
	admins     []string
	notify     map[string]map[string]bool
}

func (m *Model) ByPretty(pretty string) *Activity {
	return m.pretty[NormalizePretty(pretty)]
}

func (m *Model) PathOf(a *Activity) string {
	if a.Parent == "" {
		if a.PrettyID != "" {
			return "/v/" + a.PrettyID
		}
		return "/activities/" + a.ID
	}
	seg := a.ID
	if a.PrettyID != "" {
		seg = a.PrettyID
	}
	return m.PathOf(m.byID[a.Parent]) + "/" + seg
}

func (m *Model) walk(path string) *Activity {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	if len(segs) < 2 {
		return nil
	}
	var node *Activity
	switch segs[0] {
	case "v":
		node = m.pretty[NormalizePretty(segs[1])]
	case "activities":
		node = m.byID[segs[1]]
	}
	for _, seg := range segs[2:] {
		if node == nil {
			return nil
		}
		next := (*Activity)(nil)
		for _, c := range node.Children {
			if c.ID == seg || (c.PrettyID != "" && c.PrettyID == NormalizePretty(seg)) {
				next = c
				break
			}
		}
		node = next
	}
	return node
}

func (m *Model) Resolve(path string) *Activity {
	at := redirectPath(path)
	for hops := 0; hops < 20 && at != "" && !isURL(at); hops++ {
		if a := m.walk(at); a != nil {
			return a
		}
		at = m.moved(at)
	}
	return nil
}

func (m *Model) moved(at string) string {
	moved, matched := "", ""
	for _, r := range m.Redirects {
		if strings.EqualFold(r.Old, at) {
			moved, matched = r.New, at
		} else if strings.HasPrefix(strings.ToLower(at), strings.ToLower(r.Old)+"/") && len(r.Old) > len(matched) {
			moved, matched = strings.TrimSuffix(r.New, "/")+at[len(r.Old):], r.Old
		}
	}
	return moved
}

func (m *Model) Destination(path string) string {
	start := redirectPath(path)
	if start == "" || m.walk(start) != nil {
		return ""
	}
	at, seen := start, map[string]bool{strings.ToLower(start): true}
	for hops := 0; hops < 20; hops++ {
		next := m.moved(at)
		if next == "" {
			break
		}
		if isURL(next) {
			return next
		}
		if a := m.walk(next); a != nil {
			return m.PathOf(a)
		}
		if seen[strings.ToLower(next)] {
			return ""
		}
		seen[strings.ToLower(next)] = true
		at = next
	}
	if at == start || !onSite(at) {
		return ""
	}
	return at
}

func onSite(path string) bool {
	return strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") && !strings.HasPrefix(path, "/\\")
}

func (m *Model) redirect(old string) *Redirect {
	if old == "" {
		return nil
	}
	for i, r := range m.Redirects {
		if strings.EqualFold(r.Old, old) {
			return &m.Redirects[i]
		}
	}
	return nil
}

func (m *Model) withRedirect(r Redirect, replacing *Redirect) *Model {
	next := *m
	next.Redirects = []Redirect{}
	for _, existing := range m.Redirects {
		if replacing == nil || existing.cell != replacing.cell {
			next.Redirects = append(next.Redirects, existing)
		}
	}
	next.Redirects = append(next.Redirects, r)
	return &next
}

func (m *Model) notifyPrefs(email string) map[string]bool {
	return m.notify[strings.ToLower(strings.TrimSpace(email))]
}

type Skipped struct {
	Deleted    int
	Orphans    int
	Volunteers int
	Links      int
	Duplicates int
	PrettyIDs  int
}

func (m *Model) Activity(id string) *Activity {
	return m.byID[id]
}

func (m *Model) Category(id string) *Category {
	return m.categories[id]
}

func (m *Model) Root(a *Activity) *Activity {
	for a.Parent != "" {
		a = m.byID[a.Parent]
	}
	return a
}

func (a *Activity) Descendants() []*Activity {
	out := []*Activity{}
	var walk func([]*Activity)
	walk = func(list []*Activity) {
		for _, c := range list {
			out = append(out, c)
			walk(c.Children)
		}
	}
	walk(a.Children)
	return out
}

func (a *Activity) CoChairs() []string {
	out := []string{}
	for _, v := range a.Volunteers {
		if v.Position == PositionCoChair {
			out = append(out, v.Email)
		}
	}
	return out
}

func (a *Activity) volunteer(email string) *Volunteer {
	for i, v := range a.Volunteers {
		if strings.EqualFold(v.Email, email) {
			return &a.Volunteers[i]
		}
	}
	return nil
}

func (m *Model) Runs(a *Activity, email string) bool {
	for node := a; node != nil; {
		if node.IsCoChair(email) {
			return true
		}
		if node.Parent == "" {
			return false
		}
		node = m.Activity(node.Parent)
	}
	return false
}

func (a *Activity) IsCoChair(email string) bool {
	return slices.Contains(a.CoChairs(), strings.ToLower(email))
}

func yesNo(cell string, blank bool) (bool, error) {
	switch cell {
	case "Yes":
		return true, nil
	case "No":
		return false, nil
	case "":
		return blank, nil
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

func SchoolYear(t time.Time) string {
	start := t.Year()
	if t.Month() < time.July {
		start--
	}
	return fmt.Sprintf("%d - %d", start, start+1)
}

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

func checkID(kind, title, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%s %q has no %s id", kind, title, kind)
	}
	return nil
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

func parseSettings(rows []store.Row) (Settings, map[string]map[string]bool, error) {
	values := map[string]string{}
	notify := map[string]map[string]bool{}
	for _, row := range rows {
		key := row["Key"]
		if email, ok := strings.CutPrefix(strings.ToLower(strings.TrimSpace(key)), notifyPrefix); ok {
			if notify[email] == nil {
				notify[email] = map[string]bool{}
			}
			for _, k := range strings.Split(row["Value"], ",") {
				if k = strings.TrimSpace(k); k != "" {
					notify[email][k] = true
				}
			}
			continue
		}
		if slices.Contains(legacyThemeKeys, key) {
			continue
		}
		if !slices.Contains(settingKeys, key) {
			return Settings{}, nil, fmt.Errorf("%s has unknown key %q", settingsTab, key)
		}
		if _, dup := values[key]; dup {
			return Settings{}, nil, fmt.Errorf("%s has duplicate key %q", settingsTab, key)
		}
		values[key] = row["Value"]
	}
	for _, key := range settingKeys {
		if values[key] == "" {
			return Settings{}, nil, fmt.Errorf("%s is missing %q", settingsTab, key)
		}
	}
	if !strings.HasPrefix(values[ExpenseFormKey], "https://") {
		return Settings{}, nil, fmt.Errorf("setting %q must be a full https:// url", ExpenseFormKey)
	}
	return Settings{ExpenseFormURL: values[ExpenseFormKey], Intro: values[IntroKey]}, notify, nil
}

func imageNames(rows ...[]store.Row) []string {
	names := []string{}
	for _, table := range rows {
		for _, row := range table {
			for _, column := range []string{"Image", "Flyer Image"} {
				if row[column] != "" {
					names = append(names, row[column])
				}
			}
		}
	}
	return names
}

func BuildModel(ctx context.Context, tables store.Tables, images ImageChecker) (*Model, error) {
	settings, notify, err := parseSettings(tables[settingsTab])
	if err != nil {
		return nil, err
	}
	if err := images.Prefetch(ctx, imageNames(tables[categoriesTab], tables[activitiesTab], tables[linksTab])); err != nil {
		return nil, err
	}
	model := &Model{Categories: []Category{}, Activities: []*Activity{}, Settings: settings, notify: notify,
		byID: map[string]*Activity{}, categories: map[string]*Category{}, pretty: map[string]*Activity{}, Redirects: []Redirect{}}
	admins := []string{}
	for _, row := range tables[adminsTab] {
		admins = append(admins, row["Email"])
	}
	model.admins = config.NormalizeEmails(admins)
	scoped := []*Category{}
	for _, row := range tables[categoriesTab] {
		id, title := strings.TrimSpace(row["Category ID"]), row["Title"]
		if err := checkTitle("category", title); err != nil {
			return nil, err
		}
		if err := checkID("category", title, id); err != nil {
			return nil, err
		}
		if model.categories[id] != nil {
			return nil, fmt.Errorf("categories share id %q", id)
		}
		image, err := imageURL(images, row["Image"])
		if err != nil {
			return nil, fmt.Errorf("category %q: %w", title, err)
		}
		adding, err := checkAdding(row["Allow Adding"])
		if err != nil {
			return nil, fmt.Errorf("category %q: %w", title, err)
		}
		onMain, err := yesNo(row["Show On Main Page"], true)
		if err != nil {
			return nil, fmt.Errorf("category %q: show on main page %w", title, err)
		}
		order := strings.TrimSpace(row[store.OrderColumn])
		if err := store.CheckKey(order); err != nil {
			return nil, fmt.Errorf("category %q: %w", title, err)
		}
		c := &Category{
			ID: id, EventID: strings.TrimSpace(row["Event ID"]), Title: title, Description: row["Description"],
			Image: row["Image"], ImageURL: image, AllowAdding: adding, ShowOnMain: onMain || strings.TrimSpace(row["Event ID"]) != "", Order: order,
		}
		model.categories[id] = c
		if c.EventID == "" {
			c.Adding = c.AllowAdding
			if c.Adding == "" {
				c.Adding = AddingNo
			}
			model.Categories = append(model.Categories, *c)
		} else {
			scoped = append(scoped, c)
		}
	}
	slices.SortStableFunc(model.Categories, func(a, b Category) int { return store.CompareKeys(a.Order, b.Order) })

	all := []*Activity{}
	for _, row := range tables[activitiesTab] {
		a, err := parseActivity(row, images)
		if err != nil {
			return nil, err
		}
		if a == nil {
			model.Skipped.Deleted++
			continue
		}
		if other := model.byID[a.ID]; other != nil {
			return nil, fmt.Errorf("activities %q and %q share event id %q", other.Title, a.Title, a.ID)
		}
		model.byID[a.ID] = a
		all = append(all, a)
	}
	all, model.Skipped.Orphans = dropOrphans(all, model.byID)
	roots, err := attachChildren(all, model.byID)
	if err != nil {
		return nil, err
	}
	for _, a := range all {
		if a.PrettyID == "" || a.Parent != "" {
			continue
		}
		if other := model.pretty[a.PrettyID]; other != nil {
			if other.Year >= a.Year {
				a.PrettyID = ""
				model.Skipped.PrettyIDs++
				continue
			}
			other.PrettyID = ""
			model.Skipped.PrettyIDs++
		}
		model.pretty[a.PrettyID] = a
	}
	for _, a := range all {
		seen := map[string]bool{}
		for _, c := range a.Children {
			if c.PrettyID == "" {
				continue
			}
			if seen[c.PrettyID] {
				c.PrettyID = ""
				model.Skipped.PrettyIDs++
				continue
			}
			seen[c.PrettyID] = true
		}
	}
	for _, row := range tables[redirectsTab] {
		from, to := redirectPath(row["Old"]), redirectTo(row["New"])
		if from == "" || to == "" {
			continue
		}
		model.Redirects = append(model.Redirects, Redirect{Type: strings.TrimSpace(row["Type"]), Old: from, New: to, Date: row["Date"], cell: row["Old"]})
	}
	model.Activities = roots
	var resolveAdding func(a *Activity, inherited string)
	resolveAdding = func(a *Activity, inherited string) {
		a.Adding = a.AllowAdding
		if a.Adding == "" {
			a.Adding = inherited
		}
		for _, c := range a.Children {
			resolveAdding(c, a.Adding)
		}
	}
	for _, a := range roots {
		resolveAdding(a, AddingNo)
	}
	for _, c := range scoped {
		owner := model.byID[c.EventID]
		if owner == nil {
			return nil, fmt.Errorf("category %q names unknown event id %q", c.Title, c.EventID)
		}
		c.Adding = c.AllowAdding
		if c.Adding == "" {
			c.Adding = owner.Adding
		}
		if owner.Parent != "" {
			return nil, fmt.Errorf("category %q belongs to %q, which is not a root event", c.Title, owner.Title)
		}
		owner.Categories = append(owner.Categories, *c)
	}
	for _, a := range roots {
		slices.SortStableFunc(a.Categories, func(x, y Category) int { return store.CompareKeys(x.Order, y.Order) })
	}
	fallback := false
	for _, a := range all {
		c := model.categories[a.Category]
		switch {
		case c != nil && a.Parent == "" && c.EventID == "":
		case c != nil && a.Parent != "" && c.EventID == model.Root(a).ID:
		case a.Parent == "":
			a.Category = UncategorizedID
			fallback = true
		default:
			a.Category = ""
		}
	}
	if fallback && model.categories[UncategorizedID] == nil {
		c := uncategorized()
		model.categories[c.ID] = c
		model.Categories = append(model.Categories, *c)
	}

	seen := map[string]bool{}
	for _, row := range tables[volunteersTab] {
		id := strings.TrimSpace(row["Event ID"])
		if id == "" {
			model.Skipped.Volunteers++
			continue
		}
		email := strings.ToLower(row["Email"])
		if !emailForm.MatchString(email) {
			return nil, fmt.Errorf("volunteer row %v has invalid email", row)
		}
		a := model.Activity(id)
		if a == nil {
			model.Skipped.Volunteers++
			continue
		}
		if !slices.Contains(Positions, row["Position"]) {
			return nil, fmt.Errorf("volunteer %s on %q: position %q is not one of %s", email, a.Title, row["Position"], strings.Join(Positions, ", "))
		}
		if err := checkAdded(row["Added"]); err != nil {
			return nil, fmt.Errorf("volunteer %s on %q: %w", email, a.Title, err)
		}
		key := id + "\x00" + email
		if seen[key] {
			model.Skipped.Duplicates++
			continue
		}
		seen[key] = true
		a.Volunteers = append(a.Volunteers, Volunteer{
			Email: email, Position: row["Position"], Note: row["Note"],
			AddedBy: strings.ToLower(row["Added By"]), Added: row["Added"],
		})
	}

	linkKeys := map[string]bool{}
	for _, row := range tables[linksTab] {
		id, title := strings.TrimSpace(row["Event ID"]), row["Title"]
		if err := checkTitle("link", title); err != nil {
			return nil, err
		}
		a := model.Activity(id)
		if a == nil {
			model.Skipped.Links++
			continue
		}
		if err := checkURL(row["URL"]); err != nil {
			return nil, fmt.Errorf("link %q on %q: %w", title, a.Title, err)
		}
		image, err := imageURL(images, row["Image"])
		if err != nil {
			return nil, fmt.Errorf("link %q on %q: %w", title, a.Title, err)
		}
		key := id + "\x00" + title
		if linkKeys[key] {
			return nil, fmt.Errorf("duplicate link %q on %q (%s)", title, a.Title, id)
		}
		linkKeys[key] = true
		a.Links = append(a.Links, Link{Title: title, URL: row["URL"], Description: strings.TrimSpace(row["Description"]), Image: row["Image"], ImageURL: image})
	}
	return model, nil
}

func parseActivity(row map[string]string, images ImageChecker) (*Activity, error) {
	title, year := row["Title"], row["Year"]
	if err := checkTitle("activity", title); err != nil {
		return nil, err
	}
	fail := func(err error) (*Activity, error) {
		return nil, fmt.Errorf("activity %q in %s: %w", title, year, err)
	}
	if strings.TrimSpace(row["Event ID"]) == "" {
		return nil, nil
	}
	if year != "" || strings.TrimSpace(row["Parent"]) == "" {
		if err := CheckYear(year); err != nil {
			return fail(err)
		}
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
	coLeader, err := yesNo(row["Co-Leader Needed"], true)
	if err != nil {
		return fail(fmt.Errorf("co-leader needed %w", err))
	}
	hidden, err := yesNo(row["Volunteers Hidden"], false)
	if err != nil {
		return fail(fmt.Errorf("volunteers hidden %w", err))
	}
	complete, err := yesNo(row[CompleteColumn], false)
	if err != nil {
		return fail(fmt.Errorf("volunteers complete %w", err))
	}
	direct, err := yesNo(row["Direct Sign-Up"], true)
	if err != nil {
		return fail(fmt.Errorf("direct sign-up %w", err))
	}
	priority, err := yesNo(row[PriorityColumn], false)
	if err != nil {
		return fail(fmt.Errorf("priority %w", err))
	}
	pretty := NormalizePretty(row["Pretty ID"])
	if err := CheckPretty(pretty); err != nil {
		return fail(err)
	}
	allowAdding, err := checkAdding(row["Allow Adding"])
	if err != nil {
		return fail(err)
	}
	flyer, err := imageURL(images, row["Flyer Image"])
	if err != nil {
		return fail(fmt.Errorf("flyer %w", err))
	}
	order := strings.TrimSpace(row[store.OrderColumn])
	if err := store.CheckKey(order); err != nil {
		return fail(err)
	}
	return &Activity{
		ID: strings.TrimSpace(row["Event ID"]), Year: year, Title: title, Parent: strings.TrimSpace(row["Parent"]),
		Category: strings.TrimSpace(row["Category"]), Status: row["Status"],
		Description: row["Description"], Image: row["Image"], ImageURL: image, Flyer: row["Flyer Image"], FlyerURL: flyer, Highlight: highlightOf(row),
		Order:  order,
		Timing: row["Timing"], Start: row["Start"], End: row["End"], Location: row["Location"], Spots: spots,
		CoLeaderNeeded: coLeader, VolunteersComplete: complete, VolunteersHidden: hidden, DirectSignUp: direct, Priority: priority, PrettyID: pretty, AllowAdding: allowAdding,
		AddedBy: strings.ToLower(row["Added By"]), Added: row["Added"],
		Children: []*Activity{}, Links: []Link{}, Volunteers: []Volunteer{},
	}, nil
}

func dropOrphans(all []*Activity, byID map[string]*Activity) ([]*Activity, int) {
	dropped := 0
	for {
		kept := all[:0:0]
		for _, a := range all {
			if a.Parent != "" && byID[a.Parent] == nil {
				delete(byID, a.ID)
				dropped++
				continue
			}
			kept = append(kept, a)
		}
		if len(kept) == len(all) {
			return kept, dropped
		}
		all = kept
	}
}

func attachChildren(all []*Activity, byID map[string]*Activity) ([]*Activity, error) {
	roots := []*Activity{}
	for _, a := range all {
		if a.Parent == "" {
			roots = append(roots, a)
			continue
		}
		parent := byID[a.Parent]
		if parent == a {
			return nil, fmt.Errorf("activity %q in %s is its own parent", a.Title, a.Year)
		}
		parent.Children = append(parent.Children, a)
	}
	for _, p := range all {
		slices.SortStableFunc(p.Children, func(x, y *Activity) int { return store.CompareKeys(x.Order, y.Order) })
	}
	for _, a := range all {
		seen := map[string]bool{a.ID: true}
		p := a
		for p.Parent != "" {
			p = byID[p.Parent]
			if seen[p.ID] {
				return nil, fmt.Errorf("activity %q in %s is inside a parent loop", a.Title, a.Year)
			}
			seen[p.ID] = true
		}
		if a.Year == "" {
			a.Year = p.Year
		} else if a.Year != p.Year {
			return nil, fmt.Errorf("activity %q in %s has its parent %q in %s", a.Title, a.Year, byID[a.Parent].Title, p.Year)
		}
	}
	return roots, nil
}
