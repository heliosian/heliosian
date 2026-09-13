// Package calendar holds the school calendar: imported events, the day plan, and the rules that layer them.
package calendar

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/logging"
)

const (
	appName         = "calendar"
	GoogleTab       = "Google Import"
	PDFTab          = "PDF Import"
	EventsTab       = "Events"
	EnrichmentTab   = "Enrichment"
	OverridesTab    = "Overrides"
	DayTypesTab     = "Day Types"
	DayOverridesTab = "Day Overrides"
	TagsTab         = "Tags"
	AdminsTab       = "Admins"
	FeedsTab        = "Feeds"
	ChangeLogTab    = "Change Log"
)

const (
	DateFormat      = "2006-01-02"
	DateTimeFormat  = "2006-01-02 15:04"
	TimeFormat      = "15:04"
	RegularDayType  = "Regular"
	NoSchoolDayType = "No School"
	Clear           = "-"
	SourceGoogle    = "google"
	SourcePDF       = "pdf"
	SourceSheet     = "sheet"
	MarkerFirstDay  = "First Day"
	MarkerLastDay   = "Last Day"
	maxTitleLength  = 200
	maxTextLength   = 6000
)

var (
	GoogleColumns      = []string{"Key", "Start", "End", "Title", "Location", "Description", "Updated", "Sequence"}
	PDFColumns         = []string{"Key", "Year", "Start", "End", "Title", "Day Type", "Tags", "Marker", "PDF"}
	EventColumns       = []string{"Event ID", "Start", "End", "Title", "Location", "Description", "Tags", "Day Type", "Keywords", "Added By", "Added"}
	EnrichmentColumns  = []string{"Event ID", "Tags", "Day Type", "Keywords", "Input Hash", "Model", "Enriched"}
	OverrideColumns    = []string{"Event ID", "Title", "Start", "End", "Location", "Description", "Tags", "Day Type", "Keywords", "Hidden", "Note"}
	DayTypeColumns     = []string{"Day Type", "Dropoff Start", "Dropoff End", "School Start", "School End", "Pickup Start", "Pickup End", "Aftercare Start", "Aftercare End"}
	DayOverrideColumns = []string{"Date", "Classrooms", "Day Type", "Note"}
	TagColumns         = []string{"Tag", "Description"}
	AdminColumns       = []string{"Email"}
	FeedColumns        = []string{"Token", "Email", "Name", "Classrooms", "Tags", "Created"}
	ChangeLogColumns   = []string{"Timestamp", "Actor", "Action", "Tab", "Key", "Column", "From", "To"}
)

var emailForm = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

const maxFeedNameLength = 80

var Blocks = []string{"Dropoff", "School", "Pickup", "Aftercare"}

// Location is the school's clock: every date and time in the sheet is wall-clock there.
var Location = mustLocation("America/Los_Angeles")

func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		logging.Fatal("load time zone", "name", name, "error", err)
	}
	return loc
}

// Classroom is one homeroom as the directory knows it: its band, the grades
// its students are in, and its crews when it has them.
type Classroom struct {
	Name   string   `json:"name"`
	Band   string   `json:"band,omitempty"`
	Grades []string `json:"grades"`
	Crews  []string `json:"crews,omitempty"`
}

type Roster struct {
	Classrooms []Classroom
}

// Names lists the classrooms in the directory's order.
func (r Roster) Names() []string {
	out := make([]string, 0, len(r.Classrooms))
	for _, c := range r.Classrooms {
		out = append(out, c.Name)
	}
	return out
}

func (r Roster) has(name string) bool {
	return slices.Contains(r.Names(), name)
}

// Event carries every tag that applies, the classrooms among them being the
// narrowest audience; Classrooms is just those, in the roster's order.
type Event struct {
	ID          string   `json:"id"`
	Source      string   `json:"source"`
	Title       string   `json:"title"`
	Start       string   `json:"start"`
	End         string   `json:"end"`
	AllDay      bool     `json:"allDay"`
	Location    string   `json:"location,omitempty"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags"`
	Classrooms  []string `json:"classrooms"`
	DayType     string   `json:"dayType,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
	Marker      string   `json:"marker,omitempty"`
	Updated     string   `json:"updated,omitempty"`
	Hidden      bool     `json:"-"`
	duplicate   bool
	start, end  time.Time
}

// Dates lists every day the event touches, as sheet dates.
func (e *Event) Dates() []string {
	out := []string{}
	for d := e.start; !d.After(e.end); d = d.AddDate(0, 0, 1) {
		out = append(out, d.Format(DateFormat))
	}
	return out
}

type Block struct {
	Name  string `json:"name"`
	Start string `json:"start"`
	End   string `json:"end"`
}

type DayType struct {
	Name   string  `json:"name"`
	Blocks []Block `json:"blocks"`
}

type Year struct {
	Label    string `json:"label"`
	FirstDay string `json:"firstDay"`
	LastDay  string `json:"lastDay"`
}

// Tag is one word of the vocabulary events are filed under, with the
// description the classifier is given for it.
type Tag struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// A feed with no Classrooms carries every classroom, and one with no Tags
// carries every tag; the token is the whole secret.
type Feed struct {
	Token      string   `json:"token"`
	Email      string   `json:"email"`
	Name       string   `json:"name"`
	Classrooms []string `json:"classrooms"`
	Tags       []string `json:"tags"`
	Created    string   `json:"created"`
}

func (f Feed) Carries(e *Event) bool {
	if len(f.Classrooms) > 0 && len(e.Classrooms) > 0 && !overlaps(f.Classrooms, e.Classrooms) {
		return false
	}
	return len(f.Tags) == 0 || overlaps(f.Tags, e.Tags)
}

func overlaps(a, b []string) bool {
	for _, item := range a {
		if slices.Contains(b, item) {
			return true
		}
	}
	return false
}

// Model is the sheet organized: visible events in date order, the day types,
// the tags, the day plan as date then classroom, and the school years the
// plan spans.
type Model struct {
	Events   []*Event
	DayTypes []DayType
	Tags     []Tag
	Days     map[string]map[string]string
	Years    []Year
	Roster   Roster
	Feeds    []Feed
	Hidden   int
	// Duplicates counts the events folded into another that says the same
	// thing: the same days, day type, and tags from a second source.
	Duplicates int
	Skipped    map[string]int
	byID       map[string]*Event
	byToken    map[string]*Feed
	tags       map[string]bool
}

func (m *Model) Event(id string) *Event {
	return m.byID[id]
}

func (m *Model) Feed(token string) *Feed {
	return m.byToken[token]
}

func (m *Model) DayType(name string) *DayType {
	for i := range m.DayTypes {
		if m.DayTypes[i].Name == name {
			return &m.DayTypes[i]
		}
	}
	return nil
}

// Plan is the day type one classroom has on a date, or none when the date is
// outside the school year and nothing says otherwise.
func (m *Model) Plan(date, classroom string) (DayType, bool) {
	name, ok := m.Days[date][classroom]
	if !ok {
		return DayType{}, false
	}
	return *m.DayType(name), true
}

// SchoolYear labels the school year a date falls in, years turning over on
// the first of July so the summer belongs to the year it precedes.
func SchoolYear(t time.Time) string {
	start := t.Year()
	if t.Month() < time.July {
		start--
	}
	return fmt.Sprintf("%d-%d", start, start+1)
}

type Tables struct {
	Google       []map[string]string
	PDF          []map[string]string
	Events       []map[string]string
	Enrichment   []map[string]string
	Overrides    []map[string]string
	DayTypes     []map[string]string
	DayOverrides []map[string]string
	Tags         []map[string]string
	Admins       []map[string]string
	Feeds        []map[string]string
}

func ReadTables(source data.Source) (*Tables, error) {
	type table struct {
		name   string
		want   []string
		header []string
		rows   []map[string]string
	}
	google := &table{name: GoogleTab, want: GoogleColumns}
	pdf := &table{name: PDFTab, want: PDFColumns}
	events := &table{name: EventsTab, want: EventColumns}
	enrichment := &table{name: EnrichmentTab, want: EnrichmentColumns}
	overrides := &table{name: OverridesTab, want: OverrideColumns}
	dayTypes := &table{name: DayTypesTab, want: DayTypeColumns}
	dayOverrides := &table{name: DayOverridesTab, want: DayOverrideColumns}
	tags := &table{name: TagsTab, want: TagColumns}
	admins := &table{name: AdminsTab, want: AdminColumns}
	feeds := &table{name: FeedsTab, want: FeedColumns}
	changeLog := &table{name: ChangeLogTab, want: ChangeLogColumns}
	read := []*table{google, pdf, events, enrichment, overrides, dayTypes, dayOverrides, tags, admins, feeds}
	names := []string{}
	for _, t := range read {
		names = append(names, t.name)
	}
	tabs, err := source.Tabs(appName, names, []string{changeLog.name})
	if err != nil {
		return nil, err
	}
	for _, t := range append(read, changeLog) {
		t.header, t.rows = tabs[t.name].Header, tabs[t.name].Rows
		if err := data.CheckColumns(t.name, t.header, t.want); err != nil {
			return nil, err
		}
	}
	return &Tables{
		Google: google.rows, PDF: pdf.rows, Events: events.rows, Enrichment: enrichment.rows,
		Overrides: overrides.rows, DayTypes: dayTypes.rows, DayOverrides: dayOverrides.rows, Tags: tags.rows, Admins: admins.rows, Feeds: feeds.rows,
	}, nil
}

func cloneRows(rows []map[string]string) []map[string]string {
	out := make([]map[string]string, len(rows))
	for i, row := range rows {
		out[i] = maps.Clone(row)
	}
	return out
}

func (t *Tables) WithFeed(cells map[string]string) *Tables {
	out := *t
	out.Feeds = append(cloneRows(t.Feeds), maps.Clone(cells))
	return &out
}

func (t *Tables) WithoutFeed(token string) *Tables {
	out := *t
	out.Feeds = []map[string]string{}
	for _, row := range t.Feeds {
		if row["Token"] != token {
			out.Feeds = append(out.Feeds, row)
		}
	}
	return &out
}

// SplitList reads a comma-separated cell.
func SplitList(cell string) []string {
	out := []string{}
	for _, item := range strings.Split(cell, ",") {
		item = strings.TrimSpace(item)
		if item != "" && !slices.Contains(out, item) {
			out = append(out, item)
		}
	}
	return out
}

func JoinList(items []string) string {
	return strings.Join(items, ", ")
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

func parseDate(cell string) (time.Time, error) {
	t, err := time.ParseInLocation(DateFormat, cell, Location)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is not a date like 2026-09-24", cell)
	}
	return t, nil
}

// parseWhen reads a start and end: both dates for an all-day event, both
// date-times otherwise, and a blank end means the start.
func parseWhen(start, end string) (time.Time, time.Time, bool, error) {
	if start == "" {
		return time.Time{}, time.Time{}, false, fmt.Errorf("has no start")
	}
	if end == "" {
		end = start
	}
	if s, err := time.ParseInLocation(DateFormat, start, Location); err == nil {
		e, err := time.ParseInLocation(DateFormat, end, Location)
		if err != nil {
			return s, e, true, fmt.Errorf("end %q is not a date like the start", end)
		}
		if e.Before(s) {
			return s, e, true, fmt.Errorf("ends before it starts")
		}
		return s, e, true, nil
	}
	s, err := time.ParseInLocation(DateTimeFormat, start, Location)
	if err != nil {
		return s, s, false, fmt.Errorf("start %q is not 2026-09-24 or 2026-09-24 16:00", start)
	}
	e, err := time.ParseInLocation(DateTimeFormat, end, Location)
	if err != nil {
		return s, e, false, fmt.Errorf("end %q is not a date-time like the start", end)
	}
	if e.Before(s) {
		return s, e, false, fmt.Errorf("ends before it starts")
	}
	return s, e, false, nil
}

func parseTime(what, cell string) (time.Time, error) {
	t, err := time.Parse(TimeFormat, cell)
	if err != nil {
		return t, fmt.Errorf("%s %q is not a time like 15:15", what, cell)
	}
	return t, nil
}

func parseDayTypes(rows []map[string]string) ([]DayType, error) {
	out := []DayType{}
	for _, row := range rows {
		name := row["Day Type"]
		if name == "" {
			return nil, fmt.Errorf("%s has a row with no name", DayTypesTab)
		}
		for _, d := range out {
			if d.Name == name {
				return nil, fmt.Errorf("day type %q is listed twice", name)
			}
		}
		dt := DayType{Name: name, Blocks: []Block{}}
		for _, block := range Blocks {
			start, end := row[block+" Start"], row[block+" End"]
			if start == "" && end == "" {
				continue
			}
			if start == "" || end == "" {
				return nil, fmt.Errorf("day type %q: %s needs both a start and an end", name, block)
			}
			from, err := parseTime(block+" start", start)
			if err != nil {
				return nil, fmt.Errorf("day type %q: %w", name, err)
			}
			to, err := parseTime(block+" end", end)
			if err != nil {
				return nil, fmt.Errorf("day type %q: %w", name, err)
			}
			if !to.After(from) {
				return nil, fmt.Errorf("day type %q: %s ends before it starts", name, block)
			}
			dt.Blocks = append(dt.Blocks, Block{Name: block, Start: from.Format(TimeFormat), End: to.Format(TimeFormat)})
		}
		out = append(out, dt)
	}
	found := false
	for _, d := range out {
		if d.Name == RegularDayType {
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("%s has no %q row", DayTypesTab, RegularDayType)
	}
	return out, nil
}

func parseTags(rows []map[string]string) ([]Tag, error) {
	out := []Tag{}
	for _, row := range rows {
		name := row["Tag"]
		if name == "" {
			return nil, fmt.Errorf("%s has a row with no name", TagsTab)
		}
		for _, t := range out {
			if t.Name == name {
				return nil, fmt.Errorf("tag %q is listed twice", name)
			}
		}
		if row["Description"] == "" {
			return nil, fmt.Errorf("tag %q has no description", name)
		}
		out = append(out, Tag{Name: name, Description: row["Description"]})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s has no rows", TagsTab)
	}
	return out, nil
}

type builder struct {
	model *Model
	err   error
}

// refuse records the first rule broken; the build stops at the end of its
// pass and reports it.
func (b *builder) refuse(format string, args ...any) {
	if b.err == nil {
		b.err = fmt.Errorf(format, args...)
	}
}

func (b *builder) checkDayType(name string) error {
	if name != "" && b.model.DayType(name) == nil {
		return fmt.Errorf("day type %q is not in %s", name, DayTypesTab)
	}
	return nil
}

// checkTags accepts a tag from the Tags tab or a classroom the directory has.
func (b *builder) checkTags(tags []string) error {
	for _, t := range tags {
		if !b.model.tags[t] && !b.model.Roster.has(t) {
			return fmt.Errorf("tag %q is neither in %s nor a classroom", t, TagsTab)
		}
	}
	return nil
}

func (b *builder) add(e *Event) error {
	if e.ID == "" {
		return fmt.Errorf("%s row has no id", e.Source)
	}
	if b.model.byID[e.ID] != nil {
		return fmt.Errorf("event %q is listed twice", e.ID)
	}
	b.model.byID[e.ID] = e
	b.model.Events = append(b.model.Events, e)
	return nil
}

func checkTitle(title string) error {
	if title == "" {
		return fmt.Errorf("has no title")
	}
	if len(title) > maxTitleLength {
		return fmt.Errorf("title is too long")
	}
	return nil
}

func (b *builder) event(source, id string, row map[string]string) (*Event, error) {
	e := &Event{ID: id, Source: source, Title: row["Title"], Location: row["Location"], Description: row["Description"]}
	fail := func(err error) (*Event, error) {
		return nil, fmt.Errorf("%s row %q: %w", source, id, err)
	}
	if err := checkTitle(e.Title); err != nil {
		return fail(err)
	}
	if len(e.Description) > maxTextLength {
		return fail(fmt.Errorf("description is too long"))
	}
	start, end, allDay, err := parseWhen(row["Start"], row["End"])
	if err != nil {
		return fail(err)
	}
	e.start, e.end, e.AllDay = start, end, allDay
	e.Start, e.End = row["Start"], row["End"]
	if e.End == "" {
		e.End = e.Start
	}
	e.Tags = SplitList(row["Tags"])
	if err := b.checkTags(e.Tags); err != nil {
		return fail(err)
	}
	e.DayType = row["Day Type"]
	if err := b.checkDayType(e.DayType); err != nil {
		return fail(err)
	}
	e.Keywords = SplitList(row["Keywords"])
	return e, nil
}

func union(a, b []string) []string {
	out := slices.Clone(a)
	for _, item := range b {
		if !slices.Contains(out, item) {
			out = append(out, item)
		}
	}
	return out
}

// applyEnrichment adds what the classifier concluded: its tags join the
// import's, its day type fills a blank one, its keywords join.
func (b *builder) applyEnrichment(rows []map[string]string) error {
	for _, row := range rows {
		id := row["Event ID"]
		e := b.model.byID[id]
		if e == nil {
			b.model.Skipped["enrichment of an event no tab has"]++
			continue
		}
		fail := func(err error) error {
			return fmt.Errorf("enrichment of %q: %w", id, err)
		}
		tags := SplitList(row["Tags"])
		if err := b.checkTags(tags); err != nil {
			return fail(err)
		}
		e.Tags = union(e.Tags, tags)
		if e.DayType == "" {
			e.DayType = row["Day Type"]
			if err := b.checkDayType(e.DayType); err != nil {
				return fail(err)
			}
		}
		e.Keywords = union(e.Keywords, SplitList(row["Keywords"]))
	}
	return nil
}

func override(cell string, target *string) {
	switch cell {
	case "":
	case Clear:
		*target = ""
	default:
		*target = cell
	}
}

func overrideList(cell string, target *[]string) {
	switch cell {
	case "":
	case Clear:
		*target = []string{}
	default:
		*target = SplitList(cell)
	}
}

func (b *builder) applyOverrides(rows []map[string]string) error {
	seen := map[string]bool{}
	for _, row := range rows {
		id := row["Event ID"]
		e := b.model.byID[id]
		if e == nil {
			b.model.Skipped["override of an event no tab has"]++
			continue
		}
		if seen[id] {
			return fmt.Errorf("override of %q is listed twice", id)
		}
		seen[id] = true
		fail := func(err error) error {
			return fmt.Errorf("override of %q: %w", id, err)
		}
		if row["Title"] == Clear {
			return fail(fmt.Errorf("a title cannot be cleared"))
		}
		override(row["Title"], &e.Title)
		if err := checkTitle(e.Title); err != nil {
			return fail(err)
		}
		if row["Start"] == Clear {
			return fail(fmt.Errorf("a start cannot be cleared"))
		}
		override(row["Start"], &e.Start)
		override(row["End"], &e.End)
		start, end, allDay, err := parseWhen(e.Start, e.End)
		if err != nil {
			return fail(err)
		}
		e.start, e.end, e.AllDay = start, end, allDay
		if e.End == "" {
			e.End = e.Start
		}
		override(row["Location"], &e.Location)
		override(row["Description"], &e.Description)
		if len(e.Description) > maxTextLength {
			return fail(fmt.Errorf("description is too long"))
		}
		overrideList(row["Tags"], &e.Tags)
		if err := b.checkTags(e.Tags); err != nil {
			return fail(err)
		}
		override(row["Day Type"], &e.DayType)
		if err := b.checkDayType(e.DayType); err != nil {
			return fail(err)
		}
		overrideList(row["Keywords"], &e.Keywords)
		hidden, err := yesNo(row["Hidden"])
		if err != nil {
			return fail(fmt.Errorf("hidden %w", err))
		}
		e.Hidden = hidden
	}
	return nil
}

// settle derives what follows from the layered tags: the classrooms among
// them and keywords that never repeat a tag, and refuses an event whose
// tags or day type cannot be right.
func (b *builder) settle() {
	for _, e := range b.model.Events {
		e.Classrooms = []string{}
		for _, name := range b.model.Roster.Names() {
			if slices.Contains(e.Tags, name) {
				e.Classrooms = append(e.Classrooms, name)
			}
		}
		keywords := []string{}
		for _, k := range e.Keywords {
			repeats := false
			for _, t := range e.Tags {
				if strings.EqualFold(k, t) {
					repeats = true
				}
			}
			if !repeats {
				keywords = append(keywords, k)
			}
		}
		e.Keywords = keywords
		if e.Hidden {
			continue
		}
		if len(e.Tags) == 0 {
			b.refuse("%s %q (%s) has no tags, so it matches nobody", e.Source, e.Title, e.ID)
		}
		if e.DayType != "" && !e.AllDay {
			b.refuse("%s %q (%s) is timed but carries the day type %q, which only an all-day event can", e.Source, e.Title, e.ID, e.DayType)
		}
	}
}

func (b *builder) checkFeed(f Feed) error {
	if f.Name == "" {
		return fmt.Errorf("has no name")
	}
	if len(f.Name) > maxFeedNameLength {
		return fmt.Errorf("name is too long")
	}
	if !emailForm.MatchString(f.Email) || f.Email != strings.ToLower(f.Email) {
		return fmt.Errorf("email %q is not a lowercase address", f.Email)
	}
	for _, c := range f.Classrooms {
		if !b.model.Roster.has(c) {
			return fmt.Errorf("%q is not a classroom", c)
		}
	}
	for _, t := range f.Tags {
		if !b.model.tags[t] {
			return fmt.Errorf("%q is not in %s", t, TagsTab)
		}
	}
	return nil
}

func (b *builder) feeds(rows []map[string]string) error {
	for _, row := range rows {
		f := Feed{
			Token: strings.TrimSpace(row["Token"]), Email: strings.TrimSpace(row["Email"]), Name: strings.TrimSpace(row["Name"]),
			Classrooms: SplitList(row["Classrooms"]), Tags: SplitList(row["Tags"]), Created: row["Created"],
		}
		if f.Token == "" {
			return fmt.Errorf("%s has a row with no token", FeedsTab)
		}
		if b.model.byToken[f.Token] != nil {
			return fmt.Errorf("feed token %q is listed twice", f.Token)
		}
		if err := b.checkFeed(f); err != nil {
			return fmt.Errorf("feed %q: %w", f.Token, err)
		}
		b.model.Feeds = append(b.model.Feeds, f)
		b.model.byToken[f.Token] = &b.model.Feeds[len(b.model.Feeds)-1]
	}
	for i := range b.model.Feeds {
		b.model.byToken[b.model.Feeds[i].Token] = &b.model.Feeds[i]
	}
	return nil
}

var sourceRank = map[string]int{SourceSheet: 0, SourceGoogle: 1, SourcePDF: 2}

func preferred(e, other *Event) bool {
	if (e.Marker != "") != (other.Marker != "") {
		return e.Marker != ""
	}
	if sourceRank[e.Source] != sourceRank[other.Source] {
		return sourceRank[e.Source] < sourceRank[other.Source]
	}
	return len(e.Dates()) > len(other.Dates())
}

// claims is what an all-day event says, one claim per school day per
// classroom: the day type it imposes, or its title when it imposes none.
// Weekends are nobody's claim, so a span written across one is covered by
// entries for its weekdays.
func claims(e *Event) []string {
	if !e.AllDay {
		return nil
	}
	what := e.DayType
	if what == "" {
		what = "title:" + strings.ToLower(strings.Join(strings.Fields(e.Title), " "))
	}
	out := []string{}
	for _, date := range e.Dates() {
		if day, _ := parseDate(date); day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		for _, c := range e.Classrooms {
			out = append(out, date+"|"+c+"|"+what)
		}
	}
	return out
}

// dedupe folds events that say the same thing from two sources. Events are
// taken in order of preference - one carrying a year marker first, then a
// hand-added row over the feed over the PDF, then the longer span - and each
// is kept unless it adds nothing. An all-day event is a set of claims, one
// per school day per classroom, and adds nothing when events already kept
// state every one of them: four one-day feed entries cover a four-day PDF
// entry, and two feed weeks cover a PDF span written across the weekend
// between them. A timed event adds nothing when one already kept has its
// start, end, and tags. The rest are hidden and counted, so the day plan,
// the lists, and the feeds all see one.
func (b *builder) dedupe() {
	order := []*Event{}
	for _, e := range b.model.Events {
		if !e.Hidden {
			order = append(order, e)
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return preferred(order[i], order[j]) })
	seen := map[string]bool{}
	claimed := map[string]bool{}
	for _, e := range order {
		claims := claims(e)
		if len(claims) == 0 {
			tags := slices.Clone(e.Tags)
			slices.Sort(tags)
			key := e.Start + "|" + e.End + "|" + e.DayType + "|" + strings.Join(tags, ",")
			if seen[key] {
				e.Hidden, e.duplicate = true, true
			}
			seen[key] = true
			continue
		}
		fresh := false
		for _, claim := range claims {
			if !claimed[claim] {
				fresh = true
			}
		}
		if !fresh {
			e.Hidden, e.duplicate = true, true
			continue
		}
		for _, claim := range claims {
			claimed[claim] = true
		}
	}
}

func (b *builder) years(pdfRows []map[string]string) error {
	type marks struct{ first, last []string }
	byYear := map[string]*marks{}
	for _, row := range pdfRows {
		e := b.model.byID[row["Key"]]
		if e == nil || e.Hidden || e.Marker == "" {
			continue
		}
		year := row["Year"]
		m := byYear[year]
		if m == nil {
			m = &marks{}
			byYear[year] = m
		}
		if e.Marker == MarkerFirstDay {
			m.first = append(m.first, e.Start)
		} else {
			m.last = append(m.last, e.Start)
		}
	}
	for _, year := range slices.Sorted(func(yield func(string) bool) {
		for y := range byYear {
			if !yield(y) {
				return
			}
		}
	}) {
		m := byYear[year]
		if len(m.first) != 1 || len(m.last) != 1 {
			return fmt.Errorf("school year %s needs exactly one %s and one %s in %s, has %d and %d", year, MarkerFirstDay, MarkerLastDay, PDFTab, len(m.first), len(m.last))
		}
		if m.last[0] < m.first[0] {
			return fmt.Errorf("school year %s ends before it starts", year)
		}
		b.model.Years = append(b.model.Years, Year{Label: year, FirstDay: m.first[0], LastDay: m.last[0]})
	}
	return nil
}

// assign records a day type for classrooms on a date within one layer. Two
// claims in one layer that differ are refused, unless one of them is No
// School, which wins: a break the feed also marks as a no-aftercare day or
// a half day is still a break.
func (b *builder) assign(layer map[string]map[string]string, date string, classrooms []string, name, by string) {
	if layer[date] == nil {
		layer[date] = map[string]string{}
	}
	for _, c := range classrooms {
		current, ok := layer[date][c]
		switch {
		case !ok || current == name:
			layer[date][c] = name
		case name == NoSchoolDayType:
			layer[date][c] = name
		case current == NoSchoolDayType:
		default:
			b.refuse("%s gives %s both %q and %q on %s", by, c, current, name, date)
		}
	}
}

func (b *builder) merge(layer map[string]map[string]string) {
	for date, byClassroom := range layer {
		if b.model.Days[date] == nil {
			b.model.Days[date] = map[string]string{}
		}
		for c, name := range byClassroom {
			b.model.Days[date][c] = name
		}
	}
}

func (b *builder) days(dayOverrides []map[string]string) error {
	m := b.model
	everyone := m.Roster.Names()
	for _, y := range m.Years {
		first, _ := parseDate(y.FirstDay)
		last, _ := parseDate(y.LastDay)
		for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
			if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
				continue
			}
			m.Days[d.Format(DateFormat)] = map[string]string{}
			for _, c := range everyone {
				m.Days[d.Format(DateFormat)][c] = RegularDayType
			}
		}
	}
	for _, source := range []string{SourcePDF, SourceGoogle, SourceSheet} {
		layer := map[string]map[string]string{}
		for _, e := range m.Events {
			if e.Source != source || e.Hidden || e.DayType == "" || !e.AllDay {
				continue
			}
			for _, date := range e.Dates() {
				b.assign(layer, date, e.Classrooms, e.DayType, "the "+source+" events")
			}
		}
		b.merge(layer)
	}
	layer := map[string]map[string]string{}
	for _, row := range dayOverrides {
		date := row["Date"]
		fail := func(err error) error {
			return fmt.Errorf("day override on %q: %w", date, err)
		}
		if _, err := parseDate(date); err != nil {
			return fail(err)
		}
		classrooms := SplitList(row["Classrooms"])
		for _, c := range classrooms {
			if !m.Roster.has(c) {
				return fail(fmt.Errorf("%q is not a classroom", c))
			}
		}
		if len(classrooms) == 0 {
			classrooms = everyone
		}
		name := row["Day Type"]
		if name == "" || m.DayType(name) == nil {
			return fail(fmt.Errorf("day type %q is not in %s", name, DayTypesTab))
		}
		b.assign(layer, date, classrooms, name, DayOverridesTab)
	}
	b.merge(layer)
	return nil
}

// BuildModel validates every row and refuses the whole set on the first
// rule broken, the stance every app here takes. Events layer import, then
// enrichment joining it, then overrides winning over both; the day plan is
// Regular across each school year, then every all-day event's day type by
// source, then Day Overrides.
func BuildModel(tables *Tables, roster Roster) (*Model, error) {
	dayTypes, err := parseDayTypes(tables.DayTypes)
	if err != nil {
		return nil, err
	}
	tags, err := parseTags(tables.Tags)
	if err != nil {
		return nil, err
	}
	m := &Model{
		Events: []*Event{}, DayTypes: dayTypes, Tags: tags, Days: map[string]map[string]string{}, Years: []Year{},
		Roster: roster, Feeds: []Feed{}, Skipped: map[string]int{}, byID: map[string]*Event{}, byToken: map[string]*Feed{}, tags: map[string]bool{},
	}
	for _, t := range tags {
		m.tags[t.Name] = true
	}
	b := &builder{model: m}
	for _, row := range tables.Google {
		e, err := b.event(SourceGoogle, row["Key"], row)
		if err != nil {
			return nil, err
		}
		e.Updated = row["Updated"]
		if err := b.add(e); err != nil {
			return nil, err
		}
	}
	for _, row := range tables.PDF {
		e, err := b.event(SourcePDF, row["Key"], row)
		if err != nil {
			return nil, err
		}
		if want := SchoolYear(e.start); row["Year"] != want {
			return nil, fmt.Errorf("%s row %q: year %q should be %s", PDFTab, e.ID, row["Year"], want)
		}
		e.Marker = row["Marker"]
		if e.Marker != "" && e.Marker != MarkerFirstDay && e.Marker != MarkerLastDay {
			return nil, fmt.Errorf("%s row %q: marker %q is not blank, %s, or %s", PDFTab, e.ID, e.Marker, MarkerFirstDay, MarkerLastDay)
		}
		if err := b.add(e); err != nil {
			return nil, err
		}
	}
	for _, row := range tables.Events {
		e, err := b.event(SourceSheet, row["Event ID"], row)
		if err != nil {
			return nil, err
		}
		if err := b.add(e); err != nil {
			return nil, err
		}
	}
	if err := b.applyEnrichment(tables.Enrichment); err != nil {
		return nil, err
	}
	if err := b.applyOverrides(tables.Overrides); err != nil {
		return nil, err
	}
	sort.SliceStable(m.Events, func(i, j int) bool {
		if !m.Events[i].start.Equal(m.Events[j].start) {
			return m.Events[i].start.Before(m.Events[j].start)
		}
		if m.Events[i].Title != m.Events[j].Title {
			return m.Events[i].Title < m.Events[j].Title
		}
		return m.Events[i].ID < m.Events[j].ID
	})
	b.settle()
	b.dedupe()
	if err := b.years(tables.PDF); err != nil {
		return nil, err
	}
	if err := b.days(tables.DayOverrides); err != nil {
		return nil, err
	}
	if err := b.feeds(tables.Feeds); err != nil {
		return nil, err
	}
	visible := []*Event{}
	for _, e := range m.Events {
		if e.Hidden {
			if e.duplicate {
				m.Duplicates++
			} else {
				m.Hidden++
			}
			delete(m.byID, e.ID)
			continue
		}
		visible = append(visible, e)
	}
	m.Events = visible
	if b.err != nil {
		return nil, b.err
	}
	return m, nil
}
