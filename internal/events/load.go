// Package events serves HCA-Team, the HCA volunteer portal: what the community
// association runs each school year, and who signed up to help.
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
	volunteersTab = "Volunteers"
	linksTab      = "Links"
	settingsTab   = "Settings"
	adminsTab     = "Admins"
	redirectsTab  = "Redirects"
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
	CategoryColumns  = []string{"Category ID", "Event ID", "Title", "Description", "Image", "Allow Adding", "Show On Main Page"}
	ActivityColumns  = []string{"Event ID", "Year", "Title", "Parent", "Category", "Status", "Description", "Image", "Timing", "Start", "End", "Location", "Spots", "Co-Leader Needed", "Volunteers Hidden", "Direct Sign-Up", "Pretty ID", "Allow Adding", "Flyer Image", "Highlight Headline", "Highlight Body", "Highlight Icon", "Added By", "Added"}
	VolunteerColumns = []string{"Event ID", "Email", "Position", "Note", "Added By", "Added"}
	LinkColumns      = []string{"Event ID", "Title", "URL", "Image", "Description"}
	SettingColumns   = []string{"Key", "Value"}
	RedirectColumns  = []string{"Type", "Old", "New", "Date"}
	AdminColumns     = []string{"Email"}
	ChangeLogColumns = []string{"Timestamp", "Actor", "Action", "Kind", "Year", "Activity", "Title", "Email", "Details"}
)

var yearForm = regexp.MustCompile(`^(\d{4}) - (\d{4})$`)

// prettyForm is what a Pretty ID may look like: the tail of hca.heliosian.com/v/...,
// lower case letters, digits and hyphens, so it types easily and reads aloud.
var prettyForm = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const maxPrettyLength = 40

// NormalizePretty is the Pretty ID as stored: trimmed and lower-cased, so
// "Applause" and "applause" are the same address.
func NormalizePretty(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// CheckPretty refuses a Pretty ID that could not be a URL segment.
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
	Prefetch(names []string) error
}

type Link struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	Image       string `json:"image,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
}

// Highlight is an activity's callout. Icon names one of the page's own icons,
// or is anything else - an emoji, say - typed into the sheet, shown as is.
type Highlight struct {
	Headline string `json:"headline"`
	Body     string `json:"body"`
	Icon     string `json:"icon,omitempty"`
}

// highlightOf reads a row's Highlight columns; a row with neither headline
// nor body has no highlight, whatever its icon cell says.
func highlightOf(row map[string]string) *Highlight {
	h := &Highlight{Headline: strings.TrimSpace(row["Highlight Headline"]), Body: strings.TrimSpace(row["Highlight Body"]), Icon: strings.TrimSpace(row["Highlight Icon"])}
	if h.Headline == "" && h.Body == "" {
		return nil
	}
	return h
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

type Activity struct {
	// ID is the key. Parent, volunteers and links all name an activity by it, so a
	// title is just a title - it can change freely and need not be unique.
	ID     string `json:"id"`
	Year   string `json:"year"`
	Title  string `json:"title"`
	Parent string `json:"parent,omitempty"`
	// Category is a Category ID: one of the page's headings for a root, or one of
	// the root event's own categories for anything under it.
	Category    string `json:"category,omitempty"`
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
	Image       string `json:"image,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
	// Flyer is the event's poster, shown in the page's rail and used for the
	// share card in place of the banner when there is one.
	Flyer    string `json:"flyer,omitempty"`
	FlyerURL string `json:"flyerUrl,omitempty"`
	// Highlight is the callout on the page - a headline, a few lines and an
	// icon - for the one thing organizers most want read; nil when there is none.
	Highlight        *Highlight `json:"highlight,omitempty"`
	Timing           string     `json:"timing,omitempty"`
	Start            string     `json:"start,omitempty"`
	End              string     `json:"end,omitempty"`
	Location         string     `json:"location,omitempty"`
	Spots            int        `json:"spots,omitempty"`
	CoLeaderNeeded   bool       `json:"coLeaderNeeded"`
	VolunteersHidden bool       `json:"volunteersHidden"`
	DirectSignUp     bool       `json:"directSignUp"`
	// PrettyID is the activity's friendly address, /v/{PrettyID}, unique across
	// every year; blank for most rows.
	PrettyID string `json:"prettyId,omitempty"`
	// AllowAdding is the row's own policy for what people may add under it,
	// blank to take the parent's; Adding is the resolved one. It is also the
	// default for the event's own categories.
	AllowAdding string      `json:"allowAddingOwn,omitempty"`
	Adding      string      `json:"allowAdding"`
	AddedBy     string      `json:"addedBy,omitempty"`
	Added       string      `json:"added,omitempty"`
	Children    []*Activity `json:"children"`
	Links       []Link      `json:"links"`
	Volunteers  []Volunteer `json:"volunteers"`
	// Categories are this root event's own, in row order; empty below the root.
	Categories []Category `json:"categories,omitempty"`
}

// A Category with no EventID is one of the headings on the opportunities page,
// and only a root activity may name it. One with an EventID belongs to that root
// event, and groups the things under it - at any depth of its tree.
type Category struct {
	ID          string `json:"id"`
	EventID     string `json:"eventId,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Image       string `json:"image,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
	// AllowAdding is the category's own policy cell (see AddingYes and friends),
	// blank to inherit; Adding is what applies once inheritance is resolved,
	// which is what pages act on.
	AllowAdding string `json:"allowAddingOwn,omitempty"`
	Adding      string `json:"allowAdding"`
	// ShowOnMain says whether the Opportunities page lists this heading's
	// events among everyone else's. A heading kept off the page still sits in
	// the rail with its count, and clicking it there shows its events. Means
	// nothing for an event's own categories.
	ShowOnMain bool `json:"showOnMain"`
	// BuiltIn marks the Uncategorized heading, which the model supplies itself
	// rather than the sheet: it cannot be edited, reordered or deleted.
	BuiltIn bool `json:"builtIn,omitempty"`
}

// An adding policy says what people who do not run a thing may do under it -
// into a category, or straight under an activity. Yes: add, and it goes live;
// Approval Needed: add, and it waits as Pending for an admin; No: only the
// organizers add. A blank cell inherits: a category from its event, an activity
// from its parent, and an event or page heading with nothing to inherit from
// takes No. Whoever runs the thing may always add, and their additions are
// live at once.
const (
	AddingYes      = "Yes"
	AddingApproval = "Approval Needed"
	AddingNo       = "No"
)

var AddingPolicies = []string{AddingYes, AddingApproval, AddingNo}

// checkAdding reads an Allow Adding cell: one of the policies, or blank.
func checkAdding(cell string) (string, error) {
	cell = strings.TrimSpace(cell)
	if cell == "" || slices.Contains(AddingPolicies, cell) {
		return cell, nil
	}
	return "", fmt.Errorf("allow adding %q is not %s, or blank", cell, strings.Join(AddingPolicies, ", "))
}

// UncategorizedID is the built-in heading for root activities whose Category
// is blank or names nothing in the Categories tab. It never lives in the sheet:
// a root saved with no category is stored blank and lands here on load.
const UncategorizedID = "uncategorized"

func uncategorized() *Category {
	return &Category{ID: UncategorizedID, Title: "Uncategorized",
		Description: "Things that have not been sorted into a category yet", Adding: AddingNo, ShowOnMain: true, BuiltIn: true}
}

type Settings struct {
	ExpenseFormURL string `json:"expenseFormUrl"`
	Intro          string `json:"intro"`
}

// A Redirect keeps an old address working after it changed: someone holding
// hca.heliosian.com/v/inight/poland still lands on the booth after the event or
// the booth was renamed. Old and New are paths as the site serves them -
// /v/inight/poland, or /activities/{id}/... for a thing under an event with no
// friendly address; a bare word is taken as /v/{word}. Type names the kind of
// thing, "Activity". A live address always wins over a redirect of the same
// name, a chain of renames is followed to its end, and a redirect of an event's
// address carries everything under it along.
type Redirect struct {
	Type string `json:"type"`
	Old  string `json:"old"`
	New  string `json:"new"`
	Date string `json:"date,omitempty"`
}

const RedirectActivity = "Activity"

// redirectPath is a redirect cell as a site path: trimmed, given the /v/ prefix
// when it is a bare friendly address, and without a trailing slash.
func redirectPath(cell string) string {
	path := strings.TrimSpace(cell)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/v/" + path
	}
	return strings.TrimRight(path, "/")
}

// Model is the sheet organized: categories in row order, then the root
// activities in row order, each holding its children as the tree the Parent
// column describes, and every volunteer under the activity they signed up for.
// Activities is the roots only; byID indexes every activity, root or child, so
// one id resolves anywhere in the tree.
type Model struct {
	// Categories are the page headings - the ones with no event. An event's own
	// live on that event.
	Categories []Category  `json:"categories"`
	Activities []*Activity `json:"activities"`
	Settings   Settings    `json:"settings"`
	Redirects  []Redirect  `json:"redirects"`
	// Skipped counts the rows a load left out on purpose: deleted activities
	// and what still pointed at them. Logged, so a typo that made a row vanish
	// shows up as a count rather than nothing at all.
	Skipped    Skipped `json:"-"`
	byID       map[string]*Activity
	categories map[string]*Category
	pretty     map[string]*Activity
}

// ByPretty finds the root activity whose friendly address this is now. Only
// roots are addressed by a bare friendly name; a thing under an event is
// addressed under the event's path - see PathOf.
func (m *Model) ByPretty(pretty string) *Activity {
	return m.pretty[NormalizePretty(pretty)]
}

// PathOf is the address a thing is served at: /v/{pretty} for a root with a
// friendly address, /activities/{id} for one without, and under an event each
// thing adds its own segment - its friendly address, or its id.
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

// walk follows one path to a thing, or nil: the first segment names a root by
// friendly address (/v/) or id (/activities/), and each further segment one of
// the children by friendly address or id. A child reached by its own id with
// no parent segment, the old /activities/{id} form, still resolves.
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

// Resolve finds the thing at a path: the one there now, or the one an old
// address has been redirected to, through any chain of renames. A redirect of
// a prefix - the event's own address - carries the rest of the path along, so
// renaming an event keeps every link into it working.
func (m *Model) Resolve(path string) *Activity {
	at := redirectPath(path)
	for hops := 0; hops < 20 && at != ""; hops++ {
		if a := m.walk(at); a != nil {
			return a
		}
		moved := ""
		for _, r := range m.Redirects {
			if strings.EqualFold(r.Old, at) {
				moved = r.New
			} else if strings.HasPrefix(strings.ToLower(at), strings.ToLower(r.Old)+"/") && len(r.Old) > len(moved) {
				moved = r.New + at[len(r.Old):]
			}
		}
		at = moved
	}
	return nil
}

// Skipped is what a load left out, by reason. A row with no Event ID is a
// deleted activity; a child pointing at an id no live row has belongs to one,
// and is deleted with it, as is a volunteer or link row whose Event ID is
// blank or names nothing. A person listed twice on the same thing is counted
// once.
type Skipped struct {
	Deleted    int
	Orphans    int
	Volunteers int
	Links      int
	Duplicates int
	// PrettyIDs is how many rows lost a Pretty ID another row also claimed;
	// the row in the latest year keeps it.
	PrettyIDs int
}

func (m *Model) Activity(id string) *Activity {
	return m.byID[id]
}

// Category finds a category by id wherever it lives - a page heading or one of
// some event's own.
func (m *Model) Category(id string) *Category {
	return m.categories[id]
}

// Root climbs to the top of an activity's tree.
func (m *Model) Root(a *Activity) *Activity {
	for a.Parent != "" {
		a = m.byID[a.Parent]
	}
	return a
}

// Descendants walks the tree under this activity in row order.
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

// Runs reports whether an address runs this activity: a co-chair of it, or of
// anything it sits under. Running an event means running the things inside it,
// which is what let a co-chair edit its roles before roles and activities became
// the same thing.
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

type Tables struct {
	Categories []map[string]string
	Activities []map[string]string
	Volunteers []map[string]string
	Links      []map[string]string
	Settings   []map[string]string
	Admins     []map[string]string
	Redirects  []map[string]string
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
	volunteers := &table{name: volunteersTab, want: VolunteerColumns}
	links := &table{name: linksTab, want: LinkColumns}
	settings := &table{name: settingsTab, want: SettingColumns}
	admins := &table{name: adminsTab, want: AdminColumns}
	redirects := &table{name: redirectsTab, want: RedirectColumns}
	changeLog := &table{name: changeLogTab, want: ChangeLogColumns}
	read := []*table{categories, activities, volunteers, links, settings, admins, redirects}
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
		Categories: categories.rows, Activities: activities.rows,
		Volunteers: volunteers.rows, Links: links.rows, Settings: settings.rows, Admins: admins.rows, Redirects: redirects.rows,
	}, nil
}

// yesNo reads a Yes/No cell; a blank takes the column's default, which is what
// a row someone typed by hand (or an older import) means by leaving it out.
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

// checkID insists on a key: without one the row cannot be named by anything
// else, and the sheet has to hand one out before the row means anything.
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
			for _, column := range []string{"Image", "Flyer Image"} {
				if row[column] != "" {
					names = append(names, row[column])
				}
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
	if err := images.Prefetch(imageNames(tables.Categories, tables.Activities, tables.Links)); err != nil {
		return nil, err
	}
	model := &Model{Categories: []Category{}, Activities: []*Activity{}, Settings: settings,
		byID: map[string]*Activity{}, categories: map[string]*Category{}, pretty: map[string]*Activity{}, Redirects: []Redirect{}}
	// Every category is parsed and indexed first; which event each scoped one
	// belongs to is checked once the activities exist.
	scoped := []*Category{}
	for _, row := range tables.Categories {
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
		c := &Category{
			ID: id, EventID: strings.TrimSpace(row["Event ID"]), Title: title, Description: row["Description"],
			Image: row["Image"], ImageURL: image, AllowAdding: adding, ShowOnMain: onMain || strings.TrimSpace(row["Event ID"]) != "",
		}
		model.categories[id] = c
		if c.EventID == "" {
			// A page heading has nothing to inherit from: blank is No.
			c.Adding = c.AllowAdding
			if c.Adding == "" {
				c.Adding = AddingNo
			}
			model.Categories = append(model.Categories, *c)
		} else {
			scoped = append(scoped, c)
		}
	}

	all := []*Activity{}
	for _, row := range tables.Activities {
		a, err := parseActivity(row, model, images)
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
	// A root's Pretty ID is one address across every year; a child's is one
	// among its siblings, under its parent's path. Saving refuses a clash, but a
	// hand-edited sheet can still carry one: among roots the row in the latest
	// year keeps the address, among siblings the first row does, and the other
	// goes without.
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
	for _, row := range tables.Redirects {
		from, to := redirectPath(row["Old"]), redirectPath(row["New"])
		if from == "" || to == "" {
			continue
		}
		model.Redirects = append(model.Redirects, Redirect{Type: strings.TrimSpace(row["Type"]), Old: from, New: to, Date: row["Date"]})
	}
	model.Activities = roots
	// Resolve the adding policies down the tree: a blank takes the parent's,
	// and a root's blank is No.
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
	// A root names a page heading; a child names one of its root event's own
	// categories, or nothing. A blank, unknown or borrowed category is not an
	// error: the row is shown as Uncategorized (a root) or without a category (a
	// child) rather than keeping the whole site from loading.
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
	for _, row := range tables.Volunteers {
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
	for _, row := range tables.Links {
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

func parseActivity(row map[string]string, model *Model, images ImageChecker) (*Activity, error) {
	title, year := row["Title"], row["Year"]
	if err := checkTitle("activity", title); err != nil {
		return nil, err
	}
	fail := func(err error) (*Activity, error) {
		return nil, fmt.Errorf("activity %q in %s: %w", title, year, err)
	}
	// A row with no Event ID is a deleted activity left in place; it is not
	// loaded, and nothing may point at it.
	if strings.TrimSpace(row["Event ID"]) == "" {
		return nil, nil
	}
	// Anything under a parent lives in the parent's year, so its own Year may
	// be left blank; attachChildren fills it in from the root.
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
	// Blank switches lean towards asking for help: a co-leader is wanted and
	// people may sign up directly unless the row says otherwise; volunteers
	// are shown unless hidden on purpose.
	coLeader, err := yesNo(row["Co-Leader Needed"], true)
	if err != nil {
		return fail(fmt.Errorf("co-leader needed %w", err))
	}
	hidden, err := yesNo(row["Volunteers Hidden"], false)
	if err != nil {
		return fail(fmt.Errorf("volunteers hidden %w", err))
	}
	direct, err := yesNo(row["Direct Sign-Up"], true)
	if err != nil {
		return fail(fmt.Errorf("direct sign-up %w", err))
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
	return &Activity{
		ID: strings.TrimSpace(row["Event ID"]), Year: year, Title: title, Parent: strings.TrimSpace(row["Parent"]),
		Category: strings.TrimSpace(row["Category"]), Status: row["Status"],
		Description: row["Description"], Image: row["Image"], ImageURL: image, Flyer: row["Flyer Image"], FlyerURL: flyer, Highlight: highlightOf(row),
		Timing: row["Timing"], Start: row["Start"], End: row["End"], Location: row["Location"], Spots: spots,
		CoLeaderNeeded: coLeader, VolunteersHidden: hidden, DirectSignUp: direct, PrettyID: pretty, AllowAdding: allowAdding,
		AddedBy: strings.ToLower(row["Added By"]), Added: row["Added"],
		Children: []*Activity{}, Links: []Link{}, Volunteers: []Volunteer{},
	}, nil
}

// dropOrphans removes every activity whose parent is not a live row - and, in
// turn, everything under those - since a missing parent is a deleted one and
// its pieces went with it. It returns how many were dropped.
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

// attachChildren hangs every activity under the parent its Parent column names,
// in row order, and returns the roots. A parent in another year is not a parent:
// the whole tree lives inside one school year.
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
	// Walk each chain to its root; anything that revisits an id on the way is
	// in a loop, and would otherwise hang every later walk of the tree. The
	// root's year is the whole chain's: a blank Year takes it, and a different
	// one is refused.
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
	case volunteersTab:
		return t.Volunteers
	case linksTab:
		return t.Links
	case settingsTab:
		return t.Settings
	case redirectsTab:
		return t.Redirects
	}
	return t.Admins
}

func (t *Tables) setTab(name string, rows []map[string]string) {
	switch name {
	case categoriesTab:
		t.Categories = rows
	case activitiesTab:
		t.Activities = rows
	case volunteersTab:
		t.Volunteers = rows
	case linksTab:
		t.Links = rows
	case settingsTab:
		t.Settings = rows
	case redirectsTab:
		t.Redirects = rows
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
