// Package calendar holds the school calendar: imported events, the day plan, and the rules that layer them.
package calendar

import (
	"encoding/base64"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/filter"
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
	SettingsTab     = "Settings"
	RSVPsTab        = "RSVPs"
	InvitationsTab  = "Invitations"
	InvitesTab      = "Invites"
	InviteGroupsTab = "Invite Groups"
	// BouncesTab holds the addresses the mail provider could not deliver
	// to, one row per bounce, so a host is warned before sending again.
	BouncesTab = "Bounces"
	// ThemeTab holds the admin's colouring of the page (internal/theme),
	// Key/Value; Settings is each person's own, so it lives apart.
	ThemeTab     = "Theme"
	ChangeLogTab = "Change Log"
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
	// A person's answer to an event: going, not going, or hidden from
	// their lists - still on the month, in gray, and in the search.
	AnswerYes      = "yes"
	AnswerNo       = "no"
	AnswerMaybe    = "maybe"
	AnswerHidden   = "hidden"
	MarkerFirstDay = "First Day"
	MarkerLastDay  = "Last Day"
	maxTitleLength = 200
	maxTextLength  = 6000
)

var (
	GoogleColumns      = []string{"Key", "Start", "End", "Title", "Location", "Description", "Updated", "Sequence"}
	PDFColumns         = []string{"Key", "Year", "Start", "End", "Title", "Day Type", "Tags", "Marker", "PDF"}
	EventColumns       = []string{"Event ID", "Start", "End", "Title", "Location", "Description", "Tags", "Day Type", "Keywords", "Added By", "Added", "Source", "Status", "Image"}
	EnrichmentColumns  = []string{"Event ID", "Tags", "Day Type", "Keywords", "Input Hash", "Model", "Enriched"}
	OverrideColumns    = []string{"Event ID", "Title", "Start", "End", "Location", "Description", "Tags", "Day Type", "Keywords", "Hidden", "Note"}
	DayTypeColumns     = []string{"Day Type", "Dropoff Start", "Dropoff End", "School Start", "School End", "Pickup Start", "Pickup End", "Aftercare Start", "Aftercare End"}
	DayOverrideColumns = []string{"Date", "Classrooms", "Day Type", "Note"}
	TagColumns         = []string{"Tag", "Description", "Group", "Default", "Image"}
	AdminColumns       = []string{"Email"}
	FeedColumns        = []string{"Token", "Email", "Name", "Classrooms", "Tags", "Created", "Emoji"}
	SettingColumns     = []string{"Email", "Classrooms", "Categories", "Saved", "Home Name", "Home Emoji", "Home Position"}
	RSVPColumns        = []string{"Email", "Event ID", "Answer", "Answered", "Answered By", "Via"}
	InvitationColumns  = []string{"Event ID", "Hosts", "Audience", "Guests", "Guest List", "Message", "Created By", "Created", "Sent", "Title", "Start", "End", "Location", "Description", "Flyer", "Notify"}
	InviteColumns      = []string{"Event ID", "Email", "Name", "Guest Of", "Via", "Added By", "Added", "Sent", "Token", "Household", "Opened"}
	InviteGroupColumns = append([]string{"Event ID", "Group ID", "Auto", "Added By", "Added", "Sent", "Removed"}, filter.RuleColumns...)
	BounceColumns      = []string{"Email", "When", "Reason"}
	ChangeLogColumns   = []string{"Timestamp", "Actor", "Action", "Tab", "Key", "Column", "From", "To"}
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

// Classroom is one homeroom as the directory knows it: its band, the grades
// its students are in, and its crews when it has them.
type Classroom struct {
	Name   string   `json:"name"`
	Band   string   `json:"band,omitempty"`
	Grades []string `json:"grades"`
	Crews  []string `json:"crews,omitempty"`
}

// A Roster also carries the households the directory lists, each address
// to the others in its families, so an invitation sent to one member of a
// household is on every member's calendar.
type Roster struct {
	Classrooms []Classroom
	Households map[string][]string
	// Parents are a student's parents, by the student's address: the
	// adults an invitation to the student reaches as well.
	Parents map[string][]string
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
	ID     string `json:"id"`
	Source string `json:"source"`
	Title  string `json:"title"`
	Start  string `json:"start"`
	End    string `json:"end"`
	AllDay bool   `json:"allDay"`
	// Dates are the days the event sits on: the days it is shown under, and
	// the days its day type stamps. A day type describes school days, so an
	// all-day event carrying one and written from one school day to another
	// is the school days between them - a conference week written across a
	// weekend says nothing about the Saturday and sits on nobody's. Anything
	// else is taken at its word: a single day, a span that starts or ends on
	// a day school is out, a trip or a book fair week with no day type. Start
	// and End stay as written, so a span is still one entry in a calendar app.
	Dates       []string `json:"dates"`
	Location    string   `json:"location,omitempty"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags"`
	Classrooms  []string `json:"classrooms"`
	DayType     string   `json:"dayType,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
	Marker      string   `json:"marker,omitempty"`
	Updated     string   `json:"updated,omitempty"`
	// Where the event's dates came from, for the page to say and link:
	// SourceURL is the original - the feed's event in Google Calendar, the
	// school's calendar page for the year calendar - SourceTitle the title
	// as the source had it, Year the school year a year-calendar row was
	// read from, and AddedBy and Added who put a hand-added event in and when.
	SourceURL   string `json:"sourceUrl,omitempty"`
	SourceTitle string `json:"sourceTitle,omitempty"`
	// SourceNote is a hand-added event's proof when it is not an address:
	// the Events tab's Source cell as written.
	SourceNote string `json:"sourceNote,omitempty"`
	Year       string `json:"year,omitempty"`
	AddedBy    string `json:"addedBy,omitempty"`
	Added      string `json:"added,omitempty"`
	// Link is the page of an event another app runs, as a path on that site;
	// Availability is what a reader can do there now; Mine is where the
	// viewer's household already stands with it; Image is its picture
	// there, as a path on that site too.
	Link         string `json:"link,omitempty"`
	Availability string `json:"availability,omitempty"`
	Mine         string `json:"mine,omitempty"`
	// Hosts are whoever runs a linked event on its own app - an HCA
	// event's chairs - by address; they run its guest list here.
	Hosts []string `json:"-"`
	// HostNames names them for the page, before any guest list exists.
	HostNames []string `json:"hostNames,omitempty"`
	// LinkedID is the id the other app knows the event by, when it is not
	// the event's own here - a school listing folded with the HCA event
	// keeps the school's id and carries the HCA event's here.
	LinkedID string `json:"linkedId,omitempty"`
	// MineWho names the household members the standing belongs to, when
	// the viewer is not among them - "Sam is going" rather than "You're
	// going".
	MineWho []string `json:"mineWho,omitempty"`
	// MinePeople is everyone in the household with a part in a linked
	// event - a ticket, a waitlist place, a role - for the lists to name.
	MinePeople []Standing `json:"minePeople,omitempty"`
	// MineWords says the standing - "Sam and Alex are waitlisted" - and Call
	// what the row and the page offer: that standing first, else the way in,
	// nothing once the event has passed or closed. Written here rather than
	// worked out again by each reader, so the calendar's rows and pages,
	// Heliosian's cards and its rail all say the same words.
	MineWords string `json:"mineWords,omitempty"`
	Call      string `json:"call,omitempty"`
	Image     string `json:"image,omitempty"`
	// Status is the Events tab's word on a hand-added event: Pending while
	// someone other than an admin's waits for approval, Declined once an
	// admin turned it away - either on the calendar for them and the
	// admins alone - Private for one found by its link or by invitation
	// alone, on the calendar of whoever has answered or been invited to
	// it, and Approved (or blank) for one that is on for everyone.
	// Pending, Declined, InviteOnly and Cancelled say which, for the page.
	Status     string `json:"status,omitempty"`
	Pending    bool   `json:"pending,omitempty"`
	Declined   bool   `json:"declined,omitempty"`
	InviteOnly bool   `json:"inviteOnly,omitempty"`
	Cancelled  bool   `json:"cancelled,omitempty"`
	// Invitation says a guest list is kept for the event (invites.go), for
	// the page to fetch it; Invited that the viewer's household is on it,
	// sent - the event on their calendar whatever its classrooms.
	Invitation bool `json:"invitation,omitempty"`
	Invited    bool `json:"invited,omitempty"`
	// Hosted marks an event the viewer runs: one they shared, a party they
	// host, or one they co-host the guest list of - never by being an
	// admin.
	Hosted     bool `json:"hosted,omitempty"`
	Hidden     bool `json:"-"`
	duplicate  bool
	start, end time.Time
}

// written is every date between the event's ends, the days it is written
// across.
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

// Tag is one word of the vocabulary events are filed under, with the
// description the classifier is given for it.
// Where the school's own calendar lives: the Google Calendar feed the
// import mirrors and the page that links the year calendar's PDF - the
// places an event's source line links to.
const (
	SchoolCalendarID   = "heliosns.org_cidjj9plktli1gdm2hrkj7gqks@group.calendar.google.com"
	SchoolFeedURL      = "https://calendar.google.com/calendar/ical/heliosns.org_cidjj9plktli1gdm2hrkj7gqks%40group.calendar.google.com/public/basic.ics"
	SchoolCalendarPage = "https://www.heliosschool.org/school-calendar"
)

// A Tag is one category events are filed under. Group is the line of the
// filters it sits on, blank for the plain line at the end; Default is
// whether it starts on for someone who has not chosen - on unless the
// tab's Default column says No; BuiltIn marks the tags the app supplies
// rather than the sheet, whose group is fixed.
type Tag struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Group       string `json:"group"`
	Default     bool   `json:"default"`
	BuiltIn     bool   `json:"builtIn,omitempty"`
	// Image is the picture an event under this tag wears when it has none
	// of its own, as the sheet names it - an uploaded object or a bundled
	// file - and ImageURL is where the page fetches it, blank when the name
	// resolves to nothing.
	Image    string `json:"image,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
}

// ImageChecker says whether a name the sheet records is an image the app
// can serve, and fetches many ahead of time.
type ImageChecker interface {
	Has(key string) (bool, error)
	Prefetch(names []string) error
}

// Provenance is the admins' side of an event's story: the classifier's
// filing (which model, when) and the correction the Overrides tab made -
// the columns it touched and its note.
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

// GoogleEventURL is where a feed event opens in Google Calendar: the
// feed's calendar and the event's id, as Google encodes the pair. A
// repeating event's instance carries its start in UTC after an underscore,
// which is how the import's key - the instance's wall-clock start - reads
// once turned back. Anything that is not the school's feed gives nothing.
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

// A Setting is one person's saved view: the classrooms and the categories
// they chose and kept, in place of the calendar's own defaults.
type Setting struct {
	Classrooms []string `json:"classrooms"`
	Tags       []string `json:"tags"`
	// HomeName and HomeEmoji are what the person calls My Heliosian and
	// marks it with, blank for the given ones; HomePosition where it sits
	// among their saved calendars, 0 first - the first being their default
	// calendar.
	HomeName     string `json:"homeName,omitempty"`
	HomeEmoji    string `json:"homeEmoji,omitempty"`
	HomePosition int    `json:"homePosition"`
}

// The Events tab's Status words for a hand-added event.
const (
	StatusPending  = "Pending"
	StatusApproved = "Approved"
	StatusDeclined = "Declined"
	StatusPrivate  = "Private"
	// Cancelled is a host's own word on an event they called off: it
	// leaves everyone's calendar and lists, its page saying so.
	StatusCancelled = "Cancelled"
	// Two older words for Private, still read as it.
	StatusInviteOnly = "Direct Link Only"
	StatusRSVP       = "RSVP Invite"
)

// MyHeliosian is the calendar everyone has: the calendar's own defaults -
// the person's classrooms and the categories on by default - which nobody
// changes or removes, though each person may rename it, mark it and place
// it among their saved calendars - first to start, and whichever calendar
// is first is the person's default. MyHeliosianToken names it where a
// saved calendar's token would.
const (
	MyHeliosianToken = "my-heliosian"
	MyHeliosianName  = "My Heliosian"
	MyHeliosianEmoji = ""
)

// MyHeliosian is the calendar as one person has it: their name and mark
// for it, else the given ones, Locked, at their position.
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

// tagDefault reads the Default column: anything but No is on, so a column
// left blank starts everything on.
func tagDefault(cell string) bool {
	return !strings.EqualFold(strings.TrimSpace(cell), "No")
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
	// Emoji is the mark the owner gave it, shown before its name in the
	// rail; blank for the calendar icon.
	Emoji string `json:"emoji,omitempty"`
	// Locked marks My Heliosian: its filters are the calendar's own and it
	// cannot be removed; Position is where it sits among the person's
	// saved calendars.
	Locked   bool `json:"locked,omitempty"`
	Position int  `json:"position,omitempty"`
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
	Events []*Event
	// Pending are the hand-added events that are not on the calendar for
	// everyone - waiting for an admin, declined, or invite only: off the
	// feeds and the front page, on the page for the person who shared
	// each and for the admins, and an invite-only one on the calendar of
	// whoever has answered it.
	Pending  []*Event
	DayTypes []DayType
	Tags     []Tag
	// Settings is each person's saved view, by address: the classrooms and
	// categories the calendar opens to for them, and what Heliosian's
	// Upcoming Events are read under.
	Settings map[string]Setting
	// Answers is each person's word on each event, by address then event
	// id: yes, no, maybe, or hidden; Answered is the same with who gave it
	// and when.
	Answers  map[string]map[string]string
	Answered map[string]map[string]Answered
	// Invitations is each event's guest list settings, by event id, and
	// Invites its guest list in the sheet's order; invited is every
	// address with a sent invite on an event, or in the household of one.
	Invitations map[string]*Invitation
	Invites     map[string][]Invite
	// Groups is each event's invite groups (groups.go), by event id, in
	// the sheet's order.
	Groups map[string][]InviteGroup
	// Bounced is every address the mail provider has reported undeliverable,
	// with the latest reason.
	Bounced  map[string]Bounce
	invited  map[string]map[string]bool
	byInvite map[string]Invite
	Days     map[string]map[string]string
	Years    []Year
	Roster   Roster
	Feeds    []Feed
	Hidden   int
	// Duplicates counts the events folded into another that says the same
	// thing: the same days, day type, and tags from a second source.
	Duplicates int
	Skipped    map[string]int
	// Provenance is what the admins' tabs did to each event - the
	// classifier's filing and any correction - for the admins' eyes.
	Provenance map[string]*Provenance
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
	Settings     []map[string]string
	RSVPs        []map[string]string
	Invitations  []map[string]string
	Invites      []map[string]string
	InviteGroups []map[string]string
	Bounces      []map[string]string
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
	settings := &table{name: SettingsTab, want: SettingColumns}
	rsvps := &table{name: RSVPsTab, want: RSVPColumns}
	invitations := &table{name: InvitationsTab, want: InvitationColumns}
	invites := &table{name: InvitesTab, want: InviteColumns}
	groups := &table{name: InviteGroupsTab, want: InviteGroupColumns}
	bounces := &table{name: BouncesTab, want: BounceColumns}
	changeLog := &table{name: ChangeLogTab, want: ChangeLogColumns}
	read := []*table{google, pdf, events, enrichment, overrides, dayTypes, dayOverrides, tags, admins, feeds, settings, rsvps, invitations, invites, groups, bounces}
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
		Overrides: overrides.rows, DayTypes: dayTypes.rows, DayOverrides: dayOverrides.rows, Tags: tags.rows, Admins: admins.rows, Feeds: feeds.rows, Settings: settings.rows, RSVPs: rsvps.rows,
		Invitations: invitations.rows, Invites: invites.rows, InviteGroups: groups.rows, Bounces: bounces.rows,
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

// WithTags is the tables with the Tags tab's rows replaced, in their order.
func (t *Tables) WithTags(rows []map[string]string) *Tables {
	out := *t
	out.Tags = cloneRows(rows)
	return &out
}

// WithSetting is the tables with one person's saved view set - their row
// replaced, or added.
func (t *Tables) WithSetting(email string, cells map[string]string) *Tables {
	out := *t
	out.Settings = []map[string]string{}
	found := false
	for _, row := range t.Settings {
		if normalizeEmail(row["Email"]) != email {
			out.Settings = append(out.Settings, maps.Clone(row))
			continue
		}
		// The person's row keeps what the cells do not name.
		merged := maps.Clone(row)
		maps.Copy(merged, cells)
		out.Settings = append(out.Settings, merged)
		found = true
	}
	if !found {
		out.Settings = append(out.Settings, maps.Clone(cells))
	}
	return &out
}

// WithOverride is the tables with an event's Overrides row given cells - the
// row it has with those cells set, or a new one.
func (t *Tables) WithOverride(id string, cells map[string]string) *Tables {
	out := *t
	out.Overrides = cloneRows(t.Overrides)
	for _, row := range out.Overrides {
		if row["Event ID"] == id {
			maps.Copy(row, cells)
			return &out
		}
	}
	row := map[string]string{"Event ID": id}
	maps.Copy(row, cells)
	out.Overrides = append(out.Overrides, row)
	return &out
}

// WithEvents is the tables with rows added to the Events tab.
func (t *Tables) WithEvents(rows []map[string]string) *Tables {
	out := *t
	out.Events = append(cloneRows(t.Events), cloneRows(rows)...)
	return &out
}

// WithEventWhen is the tables with one Events tab row's Start and End set.
func (t *Tables) WithEventWhen(id, start, end string) *Tables {
	out := *t
	out.Events = cloneRows(t.Events)
	for _, row := range out.Events {
		if row["Event ID"] == id {
			row["Start"], row["End"] = start, end
		}
	}
	return &out
}

// WithEventCells is the tables with cells set on one Events row.
func (t *Tables) WithEventCells(id string, cells map[string]string) *Tables {
	out := *t
	out.Events = cloneRows(t.Events)
	for _, row := range out.Events {
		if row["Event ID"] == id {
			maps.Copy(row, cells)
		}
	}
	return &out
}

// WithoutInvitation is the tables with everything of one event's guest
// list dropped: its Invitations row, its Invites, its groups and every
// answer to it.
func (t *Tables) WithoutInvitation(id string) *Tables {
	out := *t
	keep := func(rows []map[string]string) []map[string]string {
		kept := []map[string]string{}
		for _, row := range rows {
			if row["Event ID"] != id {
				kept = append(kept, maps.Clone(row))
			}
		}
		return kept
	}
	out.Invitations, out.Invites, out.InviteGroups, out.RSVPs = keep(t.Invitations), keep(t.Invites), keep(t.InviteGroups), keep(t.RSVPs)
	return &out
}

// WithoutEvent is the tables with one Events row dropped.
func (t *Tables) WithoutEvent(id string) *Tables {
	out := *t
	out.Events = []map[string]string{}
	for _, row := range t.Events {
		if row["Event ID"] != id {
			out.Events = append(out.Events, maps.Clone(row))
		}
	}
	return &out
}

// WithAnswer is the tables with one person's answer to one event set - the
// row for the pair replaced or added - or, for a blank answer, dropped.
func (t *Tables) WithAnswer(email, id, answer string, cells map[string]string) *Tables {
	out := *t
	out.RSVPs = []map[string]string{}
	for _, row := range t.RSVPs {
		if normalizeEmail(row["Email"]) != email || row["Event ID"] != id {
			out.RSVPs = append(out.RSVPs, maps.Clone(row))
		}
	}
	if answer != "" {
		out.RSVPs = append(out.RSVPs, maps.Clone(cells))
	}
	return &out
}

// WithInvitation is the tables with one event's Invitations row given
// cells - the row it has with those set, or a new one.
func (t *Tables) WithInvitation(id string, cells map[string]string) *Tables {
	out := *t
	out.Invitations = cloneRows(t.Invitations)
	for _, row := range out.Invitations {
		if row["Event ID"] == id {
			maps.Copy(row, cells)
			return &out
		}
	}
	row := map[string]string{"Event ID": id}
	maps.Copy(row, cells)
	out.Invitations = append(out.Invitations, row)
	return &out
}

// WithInvites is the tables with rows added to the Invites tab.
func (t *Tables) WithInvites(rows []map[string]string) *Tables {
	out := *t
	out.Invites = append(cloneRows(t.Invites), cloneRows(rows)...)
	return &out
}

// WithInviteCells is the tables with cells set on the Invites rows of one
// event whose addresses are named - every row when none are.
func (t *Tables) WithInviteCells(id string, emails []string, cells map[string]string) *Tables {
	out := *t
	out.Invites = cloneRows(t.Invites)
	for _, row := range out.Invites {
		if row["Event ID"] == id && (emails == nil || slices.Contains(emails, normalizeEmail(row["Email"]))) {
			maps.Copy(row, cells)
		}
	}
	return &out
}

// WithGroup is the tables with a row added to the Invite Groups tab.
func (t *Tables) WithGroup(row map[string]string) *Tables {
	out := *t
	out.InviteGroups = append(cloneRows(t.InviteGroups), maps.Clone(row))
	return &out
}

// WithGroupCells is the tables with cells set on one group's row.
func (t *Tables) WithGroupCells(id, group string, cells map[string]string) *Tables {
	out := *t
	out.InviteGroups = cloneRows(t.InviteGroups)
	for _, row := range out.InviteGroups {
		if row["Event ID"] == id && row["Group ID"] == group {
			maps.Copy(row, cells)
		}
	}
	return &out
}

// groupUnsent says whether anyone a group put on an event's list is still
// to be sent their invite.
func (t *Tables) groupUnsent(id, group string) bool {
	for _, row := range t.Invites {
		if row["Event ID"] == id && row["Via"] == ViaGroup+group && strings.TrimSpace(row["Sent"]) == "" {
			return true
		}
	}
	return false
}

// WithoutGroup is the tables with one group's row dropped, and with it
// the Invites rows it added that have not been sent, their answers too.
func (t *Tables) WithoutGroup(id, group string) *Tables {
	out := *t
	out.InviteGroups = []map[string]string{}
	for _, row := range t.InviteGroups {
		if row["Event ID"] != id || row["Group ID"] != group {
			out.InviteGroups = append(out.InviteGroups, maps.Clone(row))
		}
	}
	dropped := map[string]bool{}
	out.Invites = []map[string]string{}
	for _, row := range t.Invites {
		if row["Event ID"] == id && row["Via"] == ViaGroup+group && strings.TrimSpace(row["Sent"]) == "" {
			dropped[normalizeEmail(row["Email"])] = true
			continue
		}
		out.Invites = append(out.Invites, maps.Clone(row))
	}
	out.RSVPs = []map[string]string{}
	for _, row := range t.RSVPs {
		if row["Event ID"] != id || !dropped[normalizeEmail(row["Email"])] {
			out.RSVPs = append(out.RSVPs, maps.Clone(row))
		}
	}
	return &out
}

// WithoutInvite is the tables with one person's Invites row on one event
// dropped, and their answer to it with it.
func (t *Tables) WithoutInvite(id, email string) *Tables {
	out := *t
	out.Invites = []map[string]string{}
	for _, row := range t.Invites {
		if row["Event ID"] != id || normalizeEmail(row["Email"]) != email {
			out.Invites = append(out.Invites, maps.Clone(row))
		}
	}
	out.RSVPs = []map[string]string{}
	for _, row := range t.RSVPs {
		if row["Event ID"] != id || normalizeEmail(row["Email"]) != email {
			out.RSVPs = append(out.RSVPs, maps.Clone(row))
		}
	}
	return &out
}

// WithInviteEmail is the tables with one person's Invites row on one event
// keyed by a new address - the row kept whole, its token with it - and
// their answer moved with it.
func (t *Tables) WithInviteEmail(id, email, to string) *Tables {
	out := *t
	out.Invites = cloneRows(t.Invites)
	for _, row := range out.Invites {
		if row["Event ID"] == id && normalizeEmail(row["Email"]) == email {
			row["Email"] = to
		}
	}
	out.RSVPs = cloneRows(t.RSVPs)
	for _, row := range out.RSVPs {
		if row["Event ID"] == id && normalizeEmail(row["Email"]) == email {
			row["Email"] = to
		}
	}
	return &out
}

// WithBounce is the tables with a bounce noted.
func (t *Tables) WithBounce(row map[string]string) *Tables {
	out := *t
	out.Bounces = append(cloneRows(t.Bounces), maps.Clone(row))
	return &out
}

// WithoutSetting is the tables with one person's saved view forgotten.
func (t *Tables) WithoutSetting(email string) *Tables {
	out := *t
	out.Settings = []map[string]string{}
	for _, row := range t.Settings {
		if normalizeEmail(row["Email"]) != email {
			out.Settings = append(out.Settings, maps.Clone(row))
		}
	}
	return &out
}

// unusedFeedName is a name none of one person's feeds has: the one given,
// or it with the first free number after it - "… 2", "… 3" - so two saved
// calendars are told apart in the rail and in a calendar app. A blank
// name stays blank for the check that refuses it.
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

// WithFeedOrder is the tables with the Feeds rows in the order the tokens
// give - every token once, as Reorder wants.
func (t *Tables) WithFeedOrder(tokens []string) *Tables {
	out := *t
	byToken := map[string]map[string]string{}
	for _, row := range t.Feeds {
		byToken[row["Token"]] = maps.Clone(row)
	}
	out.Feeds = []map[string]string{}
	for _, token := range tokens {
		if row := byToken[token]; row != nil {
			out.Feeds = append(out.Feeds, row)
		}
	}
	return &out
}

// WithFeedChanged is the tables with one feed's cells changed - its name
// and filter - on the row its token names.
func (t *Tables) WithFeedChanged(token string, cells map[string]string) *Tables {
	out := *t
	out.Feeds = cloneRows(t.Feeds)
	for _, row := range out.Feeds {
		if row["Token"] == token {
			maps.Copy(row, cells)
		}
	}
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
		out = append(out, Tag{Name: name, Description: row["Description"], Group: strings.TrimSpace(row["Group"]), Default: tagDefault(row["Default"]), Image: strings.TrimSpace(row["Image"])})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s has no rows", TagsTab)
	}
	return out, nil
}

type builder struct {
	model *Model
	// school is every weekday inside a school year, the days the day plan
	// covers and the days a day type's span keeps to.
	school map[string]bool
	err    error
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
	e := &Event{ID: id, Source: source, Title: row["Title"], SourceTitle: row["Title"], Location: row["Location"], Description: row["Description"]}
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
		p := b.model.provenance(id)
		for _, column := range []string{"Title", "Start", "End", "Location", "Description", "Tags", "Day Type", "Keywords", "Hidden"} {
			if strings.TrimSpace(row[column]) != "" {
				p.Corrected = append(p.Corrected, column)
			}
		}
		p.Note = strings.TrimSpace(row["Note"])
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
		// An event found by its link or by invitation alone is for whoever
		// is sent it, so it may carry no tags at all - and then no Misc;
		// a cancelled one is for nobody.
		if len(e.Tags) == 0 && !e.InviteOnly && !e.Cancelled {
			b.refuse("%s %q (%s) has no tags, so it matches nobody", e.Source, e.Title, e.ID)
		}
		if len(e.Tags) == len(e.Classrooms) && len(e.Tags) > 0 {
			e.Tags = append(e.Tags, TagMisc)
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
	// A feed carries the other apps' events too, so the built-in tags are
	// as good as the tab's.
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

// settings reads each person's saved view, keeping the classrooms and
// categories the sheet still has - a renamed one drops out rather than
// hiding everything - and the last row for an address when there are two.
func (b *builder) settings(rows []map[string]string) {
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
		b.model.Settings[email] = setting
	}
}

// An Answered is one person's word on one event with who gave it - the
// person themselves, a parent for a child, a host - when, and how: on a
// page here, or by the Accept or Decline in their calendar app.
type Answered struct {
	Answer string `json:"answer"`
	By     string `json:"by,omitempty"`
	At     string `json:"at,omitempty"`
	Via    string `json:"via,omitempty"`
}

// How an answer came: ViaPage from a page - the event's here, an outside
// person's own, Heliosian's cards - and ViaCalendar from the reply a
// calendar app sent to the invite.
const (
	ViaPage     = "page"
	ViaCalendar = "calendar"
)

// isAnswer says a word is one the app takes: yes, no, maybe, or hidden.
func isAnswer(word string) bool {
	return word == AnswerYes || word == AnswerNo || word == AnswerMaybe || word == AnswerHidden
}

// answers reads each person's word on each event; an answer the app does
// not know is dropped, and the last row for a pair wins.
func (b *builder) answers(rows []map[string]string) {
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

// AnswerOf is one person's word on one event, or nothing.
func (m *Model) AnswerOf(email, id string) string {
	return m.Answers[normalizeEmail(email)][id]
}

func (b *builder) feeds(rows []map[string]string) error {
	for _, row := range rows {
		f := Feed{
			Token: strings.TrimSpace(row["Token"]), Email: strings.TrimSpace(row["Email"]), Name: strings.TrimSpace(row["Name"]),
			Classrooms: SplitList(row["Classrooms"]), Tags: SplitList(row["Tags"]), Created: row["Created"], Emoji: strings.TrimSpace(row["Emoji"]),
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
	return len(e.written()) > len(other.written())
}

func normalTitle(title string) string {
	return strings.ToLower(strings.Join(strings.Fields(title), " "))
}

// claims is what an all-day event says, one claim per weekday per classroom:
// the day type it imposes, or its title when it imposes none. It reads the
// days the event sits on (Event.Dates, settled by occupies) rather than
// working them out again, and drops the weekends among them, since a weekend
// carries nothing to say about school - which is why two feed weeks cover a
// PDF span written across the weekend between them. The test is the weekday
// and not the school year, because the feed carries years the PDF has never
// named - last year, and the summer camps - and their entries fold too.
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

// dedupe folds events that say the same thing from two sources. Events are
// taken in order of preference - one carrying a year marker first, then a
// hand-added row over the feed over the PDF, then the longer span - and each
// is kept unless it adds nothing. An all-day event is a set of claims, one
// per school day per classroom, and adds nothing when events already kept
// state every one of them: four one-day feed entries cover a four-day PDF
// entry, and two feed weeks cover a PDF span written across the weekend
// between them. A timed event adds nothing when one already kept has its
// start, end, tags, and title. The rest are hidden and counted, so the day
// plan, the lists, and the feeds all see one.
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
// School, which wins - a break the feed also marks as a no-aftercare day or
// a half day is still a break - or one of them is Regular, which yields: a
// first day of school the enricher files as a regular day is still the
// half day the feed says it is for kindergarten.
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

// occupies settles the days every event sits on (Event.Dates) once the school
// years are known, so the day plan, the pages, the front page's rail and
// Helios Ask all read one answer.
func (b *builder) occupies(dayOverrides []map[string]string) {
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

func (b *builder) days(dayOverrides []map[string]string) error {
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

// BuildModel validates every row and refuses the whole set on the first
// rule broken, the stance every app here takes. Events layer import, then
// enrichment joining it, then overrides winning over both; the school years
// come from the PDF's markers, which settles the days each event sits on
// (occupies) and so what each one claims for the folding; the day plan is
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
		Events: []*Event{}, DayTypes: dayTypes, Tags: tags, Days: map[string]map[string]string{}, Years: []Year{}, Provenance: map[string]*Provenance{},
		Roster: roster, Feeds: []Feed{}, Settings: map[string]Setting{}, Answers: map[string]map[string]string{}, Answered: map[string]map[string]Answered{},
		Invitations: map[string]*Invitation{}, Invites: map[string][]Invite{}, Groups: map[string][]InviteGroup{}, Bounced: map[string]Bounce{}, invited: map[string]map[string]bool{}, byInvite: map[string]Invite{},
		Skipped: map[string]int{}, byID: map[string]*Event{}, byToken: map[string]*Feed{}, tags: map[string]bool{},
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
		e.SourceURL = GoogleEventURL(row["Key"])
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
		e.Year, e.SourceURL = row["Year"], SchoolCalendarPage
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
		e.AddedBy, e.Added = strings.TrimSpace(row["Added By"]), strings.TrimSpace(row["Added"])
		e.Status = strings.TrimSpace(row["Status"])
		e.Pending = strings.EqualFold(e.Status, StatusPending)
		e.Declined = strings.EqualFold(e.Status, StatusDeclined)
		e.Cancelled = strings.EqualFold(e.Status, StatusCancelled)
		e.InviteOnly = strings.EqualFold(e.Status, StatusPrivate) || strings.EqualFold(e.Status, StatusInviteOnly) || strings.EqualFold(e.Status, StatusRSVP)
		// The Image cell names an upload in the shared blob store, the way
		// a category's does; the page fetches it by that path.
		if image := strings.Trim(strings.TrimSpace(row["Image"]), "/"); image != "" {
			e.Image = "/" + image
		}
		// The Source cell is the row's proof: a web address links, anything
		// else is said as written.
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
	if err := b.years(tables.PDF); err != nil {
		return nil, err
	}
	b.occupies(tables.DayOverrides)
	b.dedupe()
	if err := b.days(tables.DayOverrides); err != nil {
		return nil, err
	}
	b.settings(tables.Settings)
	b.answers(tables.RSVPs)
	b.invitations(tables.Invitations, tables.Invites)
	b.groups(tables.InviteGroups)
	b.bounces(tables.Bounces)
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
		// A pending, declined, invite-only or cancelled event waits apart
		// from the calendar, still found by id.
		if e.Pending || e.Declined || e.InviteOnly || e.Cancelled {
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
