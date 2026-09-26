package calendar

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/config"
	"heliosian/internal/filter"
	"heliosian/internal/logging"
	"heliosian/internal/store"
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
	SettingsTab     = "Settings"
	RSVPsTab        = "RSVPs"
	InvitationsTab  = "Invitations"
	InvitesTab      = "Invites"
	InviteGroupsTab = "Invite Groups"
	BouncesTab      = "Bounces"
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
	SourceCelebrate = "celebrate"
	SourceTeam      = "team"
	TagCelebrate    = "Celebrate"
	TagHCA          = "HCA"
	TagMisc         = "Misc"
	TagGoing        = "Going"
	TagWaitlisted   = "Waitlisted"
	MineGoing       = "going"
	MineWaitlisted  = "waitlisted"
	AnswerYes       = "yes"
	AnswerNo        = "no"
	AnswerMaybe     = "maybe"
	AnswerHidden    = "hidden"
	MarkerFirstDay  = "First Day"
	MarkerLastDay   = "Last Day"
	maxTitleLength  = 200
	maxTextLength   = 6000
)

var (
	GoogleColumns      = []string{"Key", "Start", "End", "Title", "Location", "Description", "Updated", "Sequence"}
	PDFColumns         = []string{"Key", "Year", "Start", "End", "Title", "Day Type", "Tags", "Marker", "PDF"}
	EventColumns       = []string{"Event ID", "Start", "End", "Title", "Location", "Description", "Tags", "Day Type", "Keywords", "Added By", "Added", "Source", "Sharing", "Status", "Image"}
	EnrichmentColumns  = []string{"Event ID", "Tags", "Day Type", "Keywords", "Input Hash", "Model", "Enriched"}
	OverrideColumns    = []string{"Event ID", "Title", "Start", "End", "Location", "Description", "Tags", "Day Type", "Keywords", "Hidden", "Note", "Address", "Image"}
	DayTypeColumns     = []string{"Day Type", "Dropoff Start", "Dropoff End", "School Start", "School End", "Pickup Start", "Pickup End", "Aftercare Start", "Aftercare End"}
	DayOverrideColumns = []string{"Date", "Classrooms", "Day Type", "Note"}
	TagColumns         = []string{"Tag", "Description", "Group", "Default", "Image", store.OrderColumn}
	AdminColumns       = []string{"Email"}
	FeedColumns        = []string{"Token", "Email", "Name", "Classrooms", "Tags", "Created", "Emoji", store.OrderColumn}
	SettingColumns     = []string{"Email", "Classrooms", "Categories", "Saved", "Home Name", "Home Emoji", "Home Position", "Feed Token"}
	RSVPColumns        = []string{"Email", "Event ID", "Answer", "Answered", "Answered By", "Via"}
	InvitationColumns  = []string{"Event ID", "Hosts", "Audience", "Guests", "Message", "Created By", "Created", "Sent", "Title", "Start", "End", "Location", "Description", "Flyer", "Notify", "Stepped Down", "Hide Hosts", "Public Guest List"}
	InviteColumns      = []string{"Event ID", "Email", "Name", "Guest Of", "Via", "Added By", "Added", "Sent", "Token", "Household", "Opened"}
	InviteGroupColumns = append([]string{"Event ID", "Group ID", "Auto", "Added By", "Added", "Sent", "Removed"}, filter.RuleColumns...)
	BounceColumns      = []string{"Email", "When", "Reason"}
)

var emailForm = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

const maxFeedNameLength = 80

var Blocks = []string{"Dropoff", "School", "Pickup", "Aftercare"}

var Location = mustLocation("America/Los_Angeles")

func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		logging.Fatal("load time zone", "name", name, "error", err)
	}
	return loc
}

type Classroom struct {
	Name   string   `json:"name"`
	Band   string   `json:"band,omitempty"`
	Grades []string `json:"grades"`
	Crews  []string `json:"crews,omitempty"`
}

type Roster struct {
	Classrooms []Classroom
	Households map[string][]string
	Parents    map[string][]string
}

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

type Event struct {
	ID           string     `json:"id"`
	Source       string     `json:"source"`
	Title        string     `json:"title"`
	Start        string     `json:"start"`
	End          string     `json:"end"`
	AllDay       bool       `json:"allDay"`
	Dates        []string   `json:"dates"`
	Location     string     `json:"location,omitempty"`
	Description  string     `json:"description,omitempty"`
	Tags         []string   `json:"tags"`
	Classrooms   []string   `json:"classrooms"`
	DayType      string     `json:"dayType,omitempty"`
	Keywords     []string   `json:"keywords,omitempty"`
	Marker       string     `json:"marker,omitempty"`
	Updated      string     `json:"updated,omitempty"`
	SourceURL    string     `json:"sourceUrl,omitempty"`
	SourceTitle  string     `json:"sourceTitle,omitempty"`
	SourceNote   string     `json:"sourceNote,omitempty"`
	Year         string     `json:"year,omitempty"`
	AddedBy      string     `json:"addedBy,omitempty"`
	Added        string     `json:"added,omitempty"`
	Address      string     `json:"address,omitempty"`
	PosterLeft   bool       `json:"posterLeft,omitempty"`
	Link         string     `json:"link,omitempty"`
	Availability string     `json:"availability,omitempty"`
	Mine         string     `json:"mine,omitempty"`
	Hosts        []string   `json:"-"`
	HostNames    []string   `json:"hostNames,omitempty"`
	LinkedID     string     `json:"linkedId,omitempty"`
	MineWho      []string   `json:"mineWho,omitempty"`
	MinePeople   []Standing `json:"minePeople,omitempty"`
	MineWords    string     `json:"mineWords,omitempty"`
	Call         string     `json:"call,omitempty"`
	Image        string     `json:"image,omitempty"`
	Sharing      string     `json:"sharing,omitempty"`
	Status       string     `json:"status,omitempty"`
	Pending      bool       `json:"pending,omitempty"`
	Declined     bool       `json:"declined,omitempty"`
	Cancelled    bool       `json:"cancelled,omitempty"`
	Invitation   bool       `json:"invitation,omitempty"`
	Invited      bool       `json:"invited,omitempty"`
	Hosted       bool       `json:"hosted,omitempty"`
	Hidden       bool       `json:"-"`
	duplicate    bool
	start, end   time.Time
}

func (e *Event) written() []string {
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

const (
	SchoolCalendarID   = "heliosns.org_cidjj9plktli1gdm2hrkj7gqks@group.calendar.google.com"
	SchoolFeedURL      = "https://calendar.google.com/calendar/ical/heliosns.org_cidjj9plktli1gdm2hrkj7gqks%40group.calendar.google.com/public/basic.ics"
	SchoolCalendarPage = "https://www.heliosschool.org/school-calendar"
)

type Tag struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Group       string `json:"group"`
	Default     bool   `json:"default"`
	BuiltIn     bool   `json:"builtIn,omitempty"`
	Image       string `json:"image,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
	order       string
}

type ImageChecker interface {
	Has(key string) (bool, error)
	Prefetch(names []string) error
}

type Provenance struct {
	Model     string   `json:"model,omitempty"`
	Enriched  string   `json:"enriched,omitempty"`
	Corrected []string `json:"corrected,omitempty"`
	Note      string   `json:"note,omitempty"`
}

func (m *Model) provenance(id string) *Provenance {
	p := m.Provenance[id]
	if p == nil {
		p = &Provenance{}
		m.Provenance[id] = p
	}
	return p
}

func GoogleEventURL(key string) string {
	uid, instance, _ := strings.Cut(key, "/")
	eventID, ok := strings.CutSuffix(uid, "@google.com")
	if !ok {
		return ""
	}
	if instance != "" {
		if len(instance) == 8 {
			eventID += "_" + instance
		} else if t, err := time.ParseInLocation("20060102T150405", instance, Location); err == nil {
			eventID += "_" + t.UTC().Format("20060102T150405Z")
		} else {
			return ""
		}
	}
	feed, err := url.Parse(SchoolFeedURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(feed.Path, "/")
	if len(parts) < 4 {
		return ""
	}
	calendarID := parts[3]
	eid := base64.RawStdEncoding.EncodeToString([]byte(eventID + " " + calendarID))
	return "https://www.google.com/calendar/event?eid=" + eid
}

type Setting struct {
	Classrooms   []string `json:"classrooms"`
	Tags         []string `json:"tags"`
	HomeName     string   `json:"homeName,omitempty"`
	HomeEmoji    string   `json:"homeEmoji,omitempty"`
	HomePosition int      `json:"homePosition"`
	// FeedToken is the secret in My Heliosian's feed address, minted the
	// first time its owner asks to subscribe; blank until then.
	FeedToken string `json:"-"`
}

const (
	SharingPublic  = "Public"
	SharingLink    = "Link"
	SharingInvited = "Invite Only"
)

var sharingWords = []string{SharingPublic, SharingLink, SharingInvited}

const (
	StatusPending   = "Pending"
	StatusApproved  = "Approved"
	StatusDeclined  = "Declined"
	StatusCancelled = "Cancelled"
)

const (
	MyHeliosianToken = "my-heliosian"
	MyHeliosianName  = "My Heliosian"
	MyHeliosianEmoji = ""
)

// myHeliosianFeed is the owner of a My Heliosian feed address by its
// token, or "" when no one's is.
func (m *Model) myHeliosianFeed(token string) string {
	if token == "" {
		return ""
	}
	for email, s := range m.Settings {
		if s.FeedToken == token {
			return email
		}
	}
	return ""
}

func (m *Model) MyHeliosian(email string) Feed {
	setting := m.Settings[normalizeEmail(email)]
	f := Feed{Token: MyHeliosianToken, Email: normalizeEmail(email), Name: MyHeliosianName, Emoji: MyHeliosianEmoji, Classrooms: []string{}, Tags: []string{}, Locked: true, Position: setting.HomePosition}
	if setting.HomeName != "" {
		f.Name = setting.HomeName
	}
	if setting.HomeEmoji != "" {
		f.Emoji = setting.HomeEmoji
	}
	return f
}

func tagDefault(cell string) bool {
	return !strings.EqualFold(strings.TrimSpace(cell), "No")
}

type Feed struct {
	Token      string   `json:"token"`
	Email      string   `json:"email"`
	Name       string   `json:"name"`
	Classrooms []string `json:"classrooms"`
	Tags       []string `json:"tags"`
	Created    string   `json:"created"`
	Emoji      string   `json:"emoji,omitempty"`
	Locked     bool     `json:"locked,omitempty"`
	Position   int      `json:"position,omitempty"`
	order      string
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

type Model struct {
	Events      []*Event
	Pending     []*Event
	DayTypes    []DayType
	Tags        []Tag
	Settings    map[string]Setting
	Answers     map[string]map[string]string
	Answered    map[string]map[string]Answered
	Invitations map[string]*Invitation
	Invites     map[string][]Invite
	Groups      map[string][]InviteGroup
	Bounced     map[string]Bounce
	invited     map[string]map[string]bool
	listed      map[string]map[string]bool
	byInvite    map[string]Invite
	Days        map[string]map[string]string
	Years       []Year
	Roster      Roster
	Feeds       []Feed
	Hidden      int
	Duplicates  int
	Skipped     map[string]int
	Provenance  map[string]*Provenance
	admins      []string
	imports     map[string]*Event
	byID        map[string]*Event
	byAddress   map[string]*Event
	byToken     map[string]*Feed
	tags        map[string]bool
}

func (e *Event) imported() bool {
	return e.Source == SourceGoogle || e.Source == SourcePDF
}

func (m *Model) Event(id string) *Event {
	if e := m.byID[id]; e != nil {
		return e
	}
	return m.byAddress[id]
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

func (m *Model) Plan(date, classroom string) (DayType, bool) {
	name, ok := m.Days[date][classroom]
	if !ok {
		return DayType{}, false
	}
	return *m.DayType(name), true
}

func SchoolYear(t time.Time) string {
	start := t.Year()
	if t.Month() < time.July {
		start--
	}
	return fmt.Sprintf("%d-%d", start, start+1)
}

func (m *Model) unusedFeedName(email, name string) string {
	if name == "" {
		return name
	}
	taken := map[string]bool{}
	for _, f := range m.Feeds {
		if normalizeEmail(f.Email) == normalizeEmail(email) {
			taken[strings.ToLower(f.Name)] = true
		}
	}
	if !taken[strings.ToLower(name)] {
		return name
	}
	for n := 2; ; n++ {
		numbered := fmt.Sprintf("%s %d", name, n)
		if !taken[strings.ToLower(numbered)] {
			return numbered
		}
	}
}

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

func parseDayTypes(rows []store.Row) ([]DayType, error) {
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

func parseTags(rows []store.Row) ([]Tag, error) {
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
		order := strings.TrimSpace(row[store.OrderColumn])
		if err := store.CheckKey(order); err != nil {
			return nil, fmt.Errorf("tag %q: %w", name, err)
		}
		out = append(out, Tag{Name: name, Description: row["Description"], Group: strings.TrimSpace(row["Group"]), Default: tagDefault(row["Default"]), Image: strings.TrimSpace(row["Image"]), order: order})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s has no rows", TagsTab)
	}
	slices.SortStableFunc(out, func(a, b Tag) int { return store.CompareKeys(a.order, b.order) })
	return out, nil
}

type builder struct {
	model  *Model
	school map[string]bool
	err    error
}

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

func (b *builder) event(source, id string, row store.Row) (*Event, error) {
	e := &Event{ID: id, Source: source, Title: row["Title"], SourceTitle: row["Title"], Location: row["Location"], Description: row["Description"], Sharing: SharingPublic}
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

func (b *builder) applyEnrichment(rows []store.Row) error {
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
		p := b.model.provenance(id)
		p.Model, p.Enriched = strings.TrimSpace(row["Model"]), strings.TrimSpace(row["Enriched"])
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

func (b *builder) keepImports() {
	for _, e := range b.model.Events {
		if !e.imported() {
			continue
		}
		c := *e
		c.Tags, c.Keywords = slices.Clone(e.Tags), slices.Clone(e.Keywords)
		b.model.imports[e.ID] = &c
	}
}

func (b *builder) applyOverrides(rows []store.Row) error {
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
		if address := strings.ToLower(strings.TrimSpace(row["Address"])); address != "" && address != Clear {
			if !eventIDForm.MatchString(address) {
				return fail(fmt.Errorf("the address %q is not letters, digits and dashes, 3 to 40 of them", address))
			}
			if other := b.model.byID[address]; other != nil || b.model.byAddress[address] != nil {
				return fail(fmt.Errorf("the address %q is already another event's", address))
			}
			e.Address = address
			b.model.byAddress[address] = e
		}
		switch image := strings.Trim(strings.TrimSpace(row["Image"]), "/"); image {
		case "":
		case Clear:
			e.Image = ""
		default:
			e.Image = "/" + image
		}
		p := b.model.provenance(id)
		for _, column := range []string{"Title", "Start", "End", "Location", "Description", "Tags", "Day Type", "Keywords", "Hidden", "Address", "Image"} {
			if strings.TrimSpace(row[column]) != "" {
				p.Corrected = append(p.Corrected, column)
			}
		}
		p.Note = strings.TrimSpace(row["Note"])
	}
	return nil
}

func (b *builder) settleEvent(e *Event) {
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
	if !e.Hidden && len(e.Tags) == len(e.Classrooms) && len(e.Tags) > 0 {
		e.Tags = append(e.Tags, TagMisc)
	}
}

func (b *builder) settle() {
	for _, e := range b.model.Events {
		if !e.Hidden && len(e.Tags) == 0 && e.Sharing == SharingPublic && !e.Cancelled {
			b.refuse("%s %q (%s) has no tags, so it matches nobody", e.Source, e.Title, e.ID)
		}
		b.settleEvent(e)
		if !e.Hidden && e.DayType != "" && !e.AllDay {
			b.refuse("%s %q (%s) is timed but carries the day type %q, which only an all-day event can", e.Source, e.Title, e.ID, e.DayType)
		}
	}
	for _, e := range b.model.imports {
		b.settleEvent(e)
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
		if !b.model.tags[t] && !slices.ContainsFunc(builtinTags, func(bt Tag) bool { return bt.Name == t }) {
			return fmt.Errorf("%q is not in %s", t, TagsTab)
		}
	}
	return nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (b *builder) settings(rows []store.Row) {
	for _, row := range rows {
		email := normalizeEmail(row["Email"])
		if email == "" {
			continue
		}
		setting := Setting{Classrooms: []string{}, Tags: []string{}}
		for _, c := range SplitList(row["Classrooms"]) {
			if b.model.Roster.has(c) {
				setting.Classrooms = append(setting.Classrooms, c)
			}
		}
		for _, t := range SplitList(row["Categories"]) {
			if b.model.tags[t] || slices.ContainsFunc(builtinTags, func(bt Tag) bool { return bt.Name == t }) {
				setting.Tags = append(setting.Tags, t)
			}
		}
		setting.HomeName = strings.TrimSpace(row["Home Name"])
		setting.HomeEmoji = strings.TrimSpace(row["Home Emoji"])
		setting.HomePosition, _ = strconv.Atoi(strings.TrimSpace(row["Home Position"]))
		setting.FeedToken = strings.TrimSpace(row["Feed Token"])
		b.model.Settings[email] = setting
	}
}

type Answered struct {
	Answer string `json:"answer"`
	By     string `json:"by,omitempty"`
	At     string `json:"at,omitempty"`
	Via    string `json:"via,omitempty"`
}

const (
	ViaPage     = "page"
	ViaCalendar = "calendar"
)

func isAnswer(word string) bool {
	return word == AnswerYes || word == AnswerNo || word == AnswerMaybe || word == AnswerHidden
}

func (b *builder) answers(rows []store.Row) {
	for _, row := range rows {
		email, id, answer := normalizeEmail(row["Email"]), strings.TrimSpace(row["Event ID"]), strings.ToLower(strings.TrimSpace(row["Answer"]))
		if email == "" || id == "" || !isAnswer(answer) {
			continue
		}
		if b.model.Answers[email] == nil {
			b.model.Answers[email] = map[string]string{}
			b.model.Answered[email] = map[string]Answered{}
		}
		b.model.Answers[email][id] = answer
		b.model.Answered[email][id] = Answered{Answer: answer, By: normalizeEmail(row["Answered By"]), At: strings.TrimSpace(row["Answered"]), Via: strings.ToLower(strings.TrimSpace(row["Via"]))}
	}
}

func (m *Model) AnswerOf(email, id string) string {
	return m.Answers[normalizeEmail(email)][id]
}

func (b *builder) feeds(rows []store.Row) error {
	seen := map[string]bool{}
	for _, row := range rows {
		f := Feed{
			Token: strings.TrimSpace(row["Token"]), Email: strings.TrimSpace(row["Email"]), Name: strings.TrimSpace(row["Name"]),
			Classrooms: SplitList(row["Classrooms"]), Tags: SplitList(row["Tags"]), Created: row["Created"], Emoji: strings.TrimSpace(row["Emoji"]),
			order: strings.TrimSpace(row[store.OrderColumn]),
		}
		if f.Token == "" {
			return fmt.Errorf("%s has a row with no token", FeedsTab)
		}
		if seen[f.Token] {
			return fmt.Errorf("feed token %q is listed twice", f.Token)
		}
		seen[f.Token] = true
		if err := store.CheckKey(f.order); err != nil {
			return fmt.Errorf("feed %q: %w", f.Token, err)
		}
		if err := b.checkFeed(f); err != nil {
			return fmt.Errorf("feed %q: %w", f.Token, err)
		}
		b.model.Feeds = append(b.model.Feeds, f)
	}
	slices.SortStableFunc(b.model.Feeds, func(x, y Feed) int { return store.CompareKeys(x.order, y.order) })
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
	return len(e.written()) > len(other.written())
}

func normalTitle(title string) string {
	return strings.ToLower(strings.Join(strings.Fields(title), " "))
}

func claims(e *Event) []string {
	if !e.AllDay {
		return nil
	}
	what := e.DayType
	if what == "" {
		what = "title:" + normalTitle(e.Title)
	}
	out := []string{}
	for _, date := range e.Dates {
		if day, _ := parseDate(date); day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		for _, c := range e.Classrooms {
			out = append(out, date+"|"+c+"|"+what)
		}
	}
	return out
}

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
		said := claims(e)
		if len(said) == 0 {
			tags := slices.Clone(e.Tags)
			slices.Sort(tags)
			key := e.Start + "|" + e.End + "|" + e.DayType + "|" + strings.Join(tags, ",") + "|" + normalTitle(e.Title)
			if seen[key] {
				e.Hidden, e.duplicate = true, true
			}
			seen[key] = true
			continue
		}
		fresh := false
		for _, claim := range said {
			if !claimed[claim] {
				fresh = true
			}
		}
		if !fresh {
			e.Hidden, e.duplicate = true, true
			continue
		}
		for _, claim := range said {
			claimed[claim] = true
		}
	}
}

func (b *builder) years(pdfRows []store.Row) error {
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
		case name == RegularDayType:
		case current == RegularDayType:
			layer[date][c] = name
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

func (b *builder) occupies(dayOverrides []store.Row) {
	b.school = map[string]bool{}
	for _, y := range b.model.Years {
		first, _ := parseDate(y.FirstDay)
		last, _ := parseDate(y.LastDay)
		for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
			if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
				continue
			}
			b.school[d.Format(DateFormat)] = true
		}
	}
	closed := map[string]bool{}
	for _, e := range b.model.Events {
		if e.Hidden || !e.AllDay || e.DayType != NoSchoolDayType {
			continue
		}
		for _, date := range e.written() {
			closed[date] = true
		}
	}
	for _, row := range dayOverrides {
		if row["Day Type"] == NoSchoolDayType {
			closed[row["Date"]] = true
		}
	}
	for _, e := range b.model.Events {
		e.Dates = e.written()
		if !e.AllDay || e.DayType == "" || len(e.Dates) == 1 {
			continue
		}
		first, last := e.Dates[0], e.Dates[len(e.Dates)-1]
		if !b.school[first] || closed[first] || !b.school[last] || closed[last] {
			continue
		}
		days := []string{}
		for _, date := range e.Dates {
			if b.school[date] {
				days = append(days, date)
			}
		}
		e.Dates = days
	}
}

func (b *builder) days(dayOverrides []store.Row) error {
	m := b.model
	everyone := m.Roster.Names()
	for date := range b.school {
		m.Days[date] = map[string]string{}
		for _, c := range everyone {
			m.Days[date][c] = RegularDayType
		}
	}
	for _, source := range []string{SourcePDF, SourceGoogle, SourceSheet} {
		layer := map[string]map[string]string{}
		for _, e := range m.Events {
			if e.Source != source || e.Hidden || e.DayType == "" || !e.AllDay {
				continue
			}
			for _, date := range e.Dates {
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

func BuildModel(tables store.Tables, roster Roster) (*Model, error) {
	dayTypes, err := parseDayTypes(tables[DayTypesTab])
	if err != nil {
		return nil, err
	}
	tags, err := parseTags(tables[TagsTab])
	if err != nil {
		return nil, err
	}
	m := &Model{
		Events: []*Event{}, DayTypes: dayTypes, Tags: tags, Days: map[string]map[string]string{}, Years: []Year{}, Provenance: map[string]*Provenance{},
		Roster: roster, Feeds: []Feed{}, Settings: map[string]Setting{}, Answers: map[string]map[string]string{}, Answered: map[string]map[string]Answered{},
		Invitations: map[string]*Invitation{}, Invites: map[string][]Invite{}, Groups: map[string][]InviteGroup{}, Bounced: map[string]Bounce{}, invited: map[string]map[string]bool{}, listed: map[string]map[string]bool{}, byInvite: map[string]Invite{},
		Skipped: map[string]int{}, imports: map[string]*Event{}, byID: map[string]*Event{}, byAddress: map[string]*Event{}, byToken: map[string]*Feed{}, tags: map[string]bool{},
	}
	for _, t := range tags {
		m.tags[t.Name] = true
	}
	admins := []string{}
	for _, row := range tables[AdminsTab] {
		admins = append(admins, row["Email"])
	}
	m.admins = config.NormalizeEmails(admins)
	b := &builder{model: m}
	for _, row := range tables[GoogleTab] {
		e, err := b.event(SourceGoogle, row["Key"], row)
		if err != nil {
			return nil, err
		}
		e.Updated = row["Updated"]
		e.SourceURL = GoogleEventURL(row["Key"])
		if err := b.add(e); err != nil {
			return nil, err
		}
	}
	for _, row := range tables[PDFTab] {
		e, err := b.event(SourcePDF, row["Key"], row)
		if err != nil {
			return nil, err
		}
		if want := SchoolYear(e.start); row["Year"] != want {
			return nil, fmt.Errorf("%s row %q: year %q should be %s", PDFTab, e.ID, row["Year"], want)
		}
		e.Year, e.SourceURL = row["Year"], SchoolCalendarPage
		e.Marker = row["Marker"]
		if e.Marker != "" && e.Marker != MarkerFirstDay && e.Marker != MarkerLastDay {
			return nil, fmt.Errorf("%s row %q: marker %q is not blank, %s, or %s", PDFTab, e.ID, e.Marker, MarkerFirstDay, MarkerLastDay)
		}
		if err := b.add(e); err != nil {
			return nil, err
		}
	}
	for _, row := range tables[EventsTab] {
		e, err := b.event(SourceSheet, row["Event ID"], row)
		if err != nil {
			return nil, err
		}
		e.AddedBy, e.Added = strings.TrimSpace(row["Added By"]), strings.TrimSpace(row["Added"])
		e.Status = strings.TrimSpace(row["Status"])
		e.Pending = strings.EqualFold(e.Status, StatusPending)
		e.Declined = strings.EqualFold(e.Status, StatusDeclined)
		e.Cancelled = strings.EqualFold(e.Status, StatusCancelled)
		if sharing := strings.TrimSpace(row["Sharing"]); sharing != "" {
			i := slices.IndexFunc(sharingWords, func(w string) bool { return strings.EqualFold(w, sharing) })
			if i < 0 {
				return nil, fmt.Errorf("%s row %q: sharing %q is not %s", EventsTab, e.ID, sharing, strings.Join(sharingWords, ", "))
			}
			e.Sharing = sharingWords[i]
		}
		if image := strings.Trim(strings.TrimSpace(row["Image"]), "/"); image != "" {
			e.Image = "/" + image
		}
		if proof := strings.TrimSpace(row["Source"]); proof != "" {
			if u, err := url.Parse(proof); err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
				e.SourceURL = proof
			} else {
				e.SourceNote = proof
			}
		}
		if err := b.add(e); err != nil {
			return nil, err
		}
	}
	if err := b.applyEnrichment(tables[EnrichmentTab]); err != nil {
		return nil, err
	}
	b.keepImports()
	if err := b.applyOverrides(tables[OverridesTab]); err != nil {
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
	if err := b.years(tables[PDFTab]); err != nil {
		return nil, err
	}
	b.occupies(tables[DayOverridesTab])
	b.dedupe()
	if err := b.days(tables[DayOverridesTab]); err != nil {
		return nil, err
	}
	b.settings(tables[SettingsTab])
	b.answers(tables[RSVPsTab])
	b.invitations(tables[InvitationsTab], tables[InvitesTab])
	for _, e := range m.Events {
		if inv := m.Invitations[e.ID]; inv != nil && e.AddedBy != "" && inv.SteppedDown == normalizeEmail(e.AddedBy) {
			e.PosterLeft = true
		}
	}
	b.groups(tables[InviteGroupsTab])
	b.bounces(tables[BouncesTab])
	if err := b.feeds(tables[FeedsTab]); err != nil {
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
			delete(m.byAddress, e.Address)
			continue
		}
		if e.Pending || e.Declined || e.Cancelled || e.Sharing != SharingPublic {
			m.Pending = append(m.Pending, e)
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
