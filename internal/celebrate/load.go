// Package celebrate serves Helios Celebrate: the community's
// fun(d)raiser parties, posted by their hosts, with tickets that families take
// and are invoiced for later, and a waitlist once a party is full.
package celebrate

import (
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"heliosian/internal/data"
)

const (
	appName         = "celebrate"
	celebrationsTab = "Celebrations"
	categoriesTab   = "Categories"
	partiesTab      = "Parties"
	hostsTab        = "Hosts"
	ticketsTab      = "Tickets"
	settingsTab     = "Settings"
	adminsTab       = "Admins"
	redirectsTab    = "Redirects"
	changeLogTab    = "Change Log"
)

const (
	DateFormat     = "2006-01-02"
	DateTimeFormat = "2006-01-02 15:04"
	maxTitleLength = 120
	maxTextLength  = 6000
	maxURLLength   = 1000
	maxNameLength  = 120
)

// A party is Pending while it waits for an admin, Open once it is listed, and
// Hidden when parked; whether it sells tickets is a separate switch.
const (
	StatusPending = "Pending"
	StatusOpen    = "Open"
	StatusHidden  = "Hidden"
)

var Statuses = []string{StatusPending, StatusOpen, StatusHidden}

// A ticket row is a sold ticket, or a place on the waitlist.
const (
	TicketSold     = "Ticket"
	TicketWaitlist = "Waitlist"
)

var TicketStatuses = []string{TicketSold, TicketWaitlist}

// Invoice is where the money stands: nothing yet, Sent, or Paid.
const (
	InvoiceSent = "Sent"
	InvoicePaid = "Paid"
)

var InvoiceStatuses = []string{"", InvoiceSent, InvoicePaid}

const (
	PartiesIntroKey = "Parties Intro"
	TicketNoteKey   = "Ticket Note"
)

var settingKeys = []string{PartiesIntroKey, TicketNoteKey}

var (
	CelebrationColumns = []string{"Code", "Title", "Subtitle", "Start", "End", "Location", "Address", "Description", "Image", "Button Text", "Button URL", "Current"}
	CategoryColumns    = []string{"Title"}
	PartyColumns       = []string{"Party ID", "Celebration", "Title", "Subtitle", "Summary", "Description", "Need To Know", "Hosts", "Category", "Audience", "Ticket Unit", "Price", "Capacity", "Minimum", "Start", "End", "Location", "Address", "Image", "Flyer Image", "Pretty ID", "Status", "Tickets", "Waitlist", "Parents", "Students", "Staff", "Drop-Off", "Parent Ticket Required", "Added By", "Added"}
	HostColumns        = []string{"Party ID", "Email"}
	TicketColumns      = []string{"Ticket ID", "Party ID", "Email", "Name", "Purchaser", "Status", "Price", "Note", "Invoice", "Added By", "Added"}
	SettingColumns     = []string{"Key", "Value"}
	AdminColumns       = []string{"Email"}
	RedirectColumns    = []string{"Type", "Old", "New", "Date"}
	ChangeLogColumns   = []string{"Timestamp", "Actor", "Action", "Kind", "Celebration", "Party", "Title", "Email", "Details"}
)

var emailForm = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// prettyForm is what a Pretty ID may look like: the tail of
// celebrate.heliosian.com/p/..., lower case letters, digits and hyphens, so
// it types easily and reads aloud.
var prettyForm = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const maxPrettyLength = 40

// NormalizePretty is the Pretty ID as stored: trimmed and lower-cased, so
// "Fondue" and "fondue" are the same address.
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

// A Redirect keeps an old address working after it changed: someone holding
// celebrate.heliosian.com/p/fondue still lands on the party after it was
// renamed. Old and New are paths as the site serves them - /p/fondue, or
// /parties/{id}; a bare word is taken as /p/{word}. A live address always
// wins over a redirect of the same name, and a chain of renames is followed
// to its end.
type Redirect struct {
	Type string `json:"type"`
	Old  string `json:"old"`
	New  string `json:"new"`
	Date string `json:"date,omitempty"`
}

const RedirectParty = "Party"

func redirectPath(cell string) string {
	path := strings.TrimSpace(cell)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/p/" + path
	}
	return strings.TrimRight(path, "/")
}

// codeForm is what a celebration's code may look like: SC-2026, the way the
// old site named its editions - letters, digits and hyphens.
var codeForm = regexp.MustCompile(`^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$`)

type ImageChecker interface {
	Has(key string) (bool, error)
	Prefetch(names []string) error
}

// Celebration is one year's Spring Celebration: the gala the parties raise
// money around. The parties page leads with the current one.
type Celebration struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	Subtitle    string `json:"subtitle,omitempty"`
	Start       string `json:"start,omitempty"`
	End         string `json:"end,omitempty"`
	Location    string `json:"location,omitempty"`
	Address     string `json:"address,omitempty"`
	Description string `json:"description,omitempty"`
	Image       string `json:"image,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
	// ButtonText and ButtonURL are the banner's button - "Learn More" to the
	// celebration's own site, say. Both or neither.
	ButtonText string `json:"buttonText,omitempty"`
	ButtonURL  string `json:"buttonUrl,omitempty"`
	Current    bool   `json:"current"`
}

// Party is one fun(d)raiser party: what it is, when and where, what a ticket
// costs and covers, how many there are, who may come, and its switches.
type Party struct {
	ID          string  `json:"id"`
	Celebration string  `json:"celebration"`
	Title       string  `json:"title"`
	Subtitle    string  `json:"subtitle,omitempty"`
	Summary     string  `json:"summary,omitempty"`
	Description string  `json:"description,omitempty"`
	NeedToKnow  string  `json:"needToKnow,omitempty"`
	Hosts       string  `json:"hosts,omitempty"`
	Category    string  `json:"category,omitempty"`
	Audience    string  `json:"audience,omitempty"`
	Unit        string  `json:"unit,omitempty"`
	Price       float64 `json:"price"`
	Capacity    int     `json:"capacity,omitempty"`
	Minimum     int     `json:"minimum,omitempty"`
	Start       string  `json:"start,omitempty"`
	End         string  `json:"end,omitempty"`
	// Location is the place in words for everyone - "The Parks' House in Los
	// Altos"; Address is the street address, for signed-in members only, so
	// it never travels in anything shown before sign-in.
	Location string `json:"location,omitempty"`
	Address  string `json:"address,omitempty"`
	Image    string `json:"image,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
	// Flyer is the party's poster, shown whole in the page's rail; Image is
	// the wide banner across the page and the card.
	Flyer    string `json:"flyer,omitempty"`
	FlyerURL string `json:"flyerUrl,omitempty"`
	// PrettyID is the party's friendly address: fondue puts it at /p/fondue.
	PrettyID string `json:"prettyId,omitempty"`
	Status   string `json:"status"`
	// TicketsOpen is the host's switch: off, the party is listed but sells
	// nothing. Waitlist says whether a full party takes a waitlist.
	TicketsOpen bool `json:"ticketsOpen"`
	Waitlist    bool `json:"waitlist"`
	// Who a ticket may be for.
	Parents  bool `json:"parents"`
	Students bool `json:"students"`
	Staff    bool `json:"staff"`
	// DropOff says a child may come without a parent; ParentTicket says a
	// parent who stays needs a ticket of their own.
	DropOff      bool     `json:"dropOff"`
	ParentTicket bool     `json:"parentTicket"`
	AddedBy      string   `json:"addedBy,omitempty"`
	Added        string   `json:"added,omitempty"`
	HostEmails   []string `json:"hostEmails"`
	Tickets      []Ticket `json:"-"`
}

// Ticket is one person on a party: a sold ticket or a place on the waitlist.
// Email names someone in the directory; a guest who is not in it has a Name
// instead. Purchaser is who is invoiced. Price is the party's price when the
// ticket was taken, so a later change does not reprice it.
type Ticket struct {
	ID        string  `json:"ticketId"`
	PartyID   string  `json:"partyId"`
	Email     string  `json:"email,omitempty"`
	Name      string  `json:"name,omitempty"`
	Purchaser string  `json:"purchaser"`
	Status    string  `json:"status"`
	Price     float64 `json:"price"`
	Note      string  `json:"note,omitempty"`
	Invoice   string  `json:"invoice,omitempty"`
	AddedBy   string  `json:"addedBy,omitempty"`
	Added     string  `json:"added,omitempty"`
}

type Settings struct {
	PartiesIntro string `json:"partiesIntro"`
	TicketNote   string `json:"ticketNote"`
}

// Model is the sheet organized: celebrations and parties in row order, each
// party carrying its hosts and its tickets in the order they were taken.
type Model struct {
	Celebrations []*Celebration
	Categories   []string
	Parties      []*Party
	Settings     Settings
	Redirects    []Redirect
	// Skipped counts the rows a load left out and why, so a mistyped id shows
	// up as a number in the log rather than a silently missing row.
	Skipped       map[string]int
	byCode        map[string]*Celebration
	byParty       map[string]*Party
	byTicket      map[string]*Ticket
	ticketParties map[string]*Party
	pretty        map[string]*Party
}

// ByPretty finds the party whose friendly address this is now.
func (m *Model) ByPretty(pretty string) *Party {
	return m.pretty[NormalizePretty(pretty)]
}

// PathOf is the address a party is served at: /p/{pretty} with a friendly
// address, /parties/{id} without.
func (m *Model) PathOf(p *Party) string {
	if p.PrettyID != "" {
		return "/p/" + p.PrettyID
	}
	return "/parties/" + p.ID
}

// walk follows one path to a party, or nil.
func (m *Model) walk(path string) *Party {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	if len(segs) != 2 {
		return nil
	}
	switch segs[0] {
	case "p":
		return m.ByPretty(segs[1])
	case "parties":
		return m.Party(segs[1])
	}
	return nil
}

// Resolve finds the party at a path: the one there now, or the one an old
// address has been redirected to, through any chain of renames.
func (m *Model) Resolve(path string) *Party {
	at := redirectPath(path)
	for hops := 0; hops < 20 && at != ""; hops++ {
		if p := m.walk(at); p != nil {
			return p
		}
		moved := ""
		for _, r := range m.Redirects {
			if strings.EqualFold(r.Old, at) {
				moved = r.New
			}
		}
		at = moved
	}
	return nil
}

func (m *Model) Celebration(code string) *Celebration {
	return m.byCode[code]
}

// Current is the celebration the site leads with: the one marked Current,
// else the latest by start.
func (m *Model) Current() *Celebration {
	var latest *Celebration
	for _, c := range m.Celebrations {
		if c.Current {
			return c
		}
		if latest == nil || c.Start > latest.Start {
			latest = c
		}
	}
	return latest
}

func (m *Model) Party(id string) *Party {
	return m.byParty[id]
}

// TicketByID finds a ticket and the party it is on.
func (m *Model) TicketByID(id string) (*Ticket, *Party) {
	return m.byTicket[id], m.ticketParties[id]
}

// Hosted reports whether email runs the party.
func (p *Party) Hosted(email string) bool {
	return slices.Contains(p.HostEmails, email)
}

// Sold counts the tickets sold; Waiting counts the waitlist.
func (p *Party) Sold() int {
	n := 0
	for _, t := range p.Tickets {
		if t.Status == TicketSold {
			n++
		}
	}
	return n
}

func (p *Party) Waiting() int {
	return len(p.Tickets) - p.Sold()
}

// Remaining is how many tickets are left, or -1 for a party with no cap.
func (p *Party) Remaining() int {
	if p.Capacity == 0 {
		return -1
	}
	return max(0, p.Capacity-p.Sold())
}

// Full says every ticket is taken.
func (p *Party) Full() bool {
	return p.Capacity > 0 && p.Sold() >= p.Capacity
}

// StartTime is when the party begins, or the zero time for one with no date.
func (p *Party) StartTime() time.Time {
	t, _ := ParseWhen(p.Start)
	return t
}

// Past says the party has happened: its end, or its start when it has no
// end, is behind now (wall-clock at the school).
func (p *Party) Past(now time.Time) bool {
	cell := p.End
	if cell == "" {
		cell = p.Start
	}
	if cell == "" {
		return false
	}
	t, err := ParseWhen(cell)
	if err != nil {
		return false
	}
	if len(cell) == len(DateFormat) {
		t = t.AddDate(0, 0, 1)
	}
	return !t.After(time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0, time.UTC))
}

// Availability is what the party's ticket button says: available, waitlist
// (full, taking names), sold-out (full, no waitlist), closed (the host has
// stopped sales, or the party is not open), or past.
const (
	Available = "available"
	Waitlist  = "waitlist"
	SoldOut   = "sold-out"
	Closed    = "closed"
	Past      = "past"
)

func (p *Party) Availability(now time.Time) string {
	switch {
	case p.Past(now):
		return Past
	case p.Status != StatusOpen || !p.TicketsOpen:
		return Closed
	case !p.Full():
		return Available
	case p.Waitlist:
		return Waitlist
	}
	return SoldOut
}

type Tables struct {
	Celebrations []map[string]string
	Categories   []map[string]string
	Parties      []map[string]string
	Hosts        []map[string]string
	Tickets      []map[string]string
	Settings     []map[string]string
	Admins       []map[string]string
	Redirects    []map[string]string
}

func ReadTables(source data.Source) (*Tables, error) {
	type table struct {
		name   string
		want   []string
		header []string
		rows   []map[string]string
		err    error
	}
	celebrations := &table{name: celebrationsTab, want: CelebrationColumns}
	categories := &table{name: categoriesTab, want: CategoryColumns}
	parties := &table{name: partiesTab, want: PartyColumns}
	hosts := &table{name: hostsTab, want: HostColumns}
	tickets := &table{name: ticketsTab, want: TicketColumns}
	settings := &table{name: settingsTab, want: SettingColumns}
	admins := &table{name: adminsTab, want: AdminColumns}
	redirects := &table{name: redirectsTab, want: RedirectColumns}
	changeLog := &table{name: changeLogTab, want: ChangeLogColumns}
	read := []*table{celebrations, categories, parties, hosts, tickets, settings, admins, redirects}
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
		Celebrations: celebrations.rows, Categories: categories.rows, Parties: parties.rows,
		Hosts: hosts.rows, Tickets: tickets.rows, Settings: settings.rows, Admins: admins.rows, Redirects: redirects.rows,
	}, nil
}

// yesNo reads a Yes/No cell; a blank takes the column's default, which is what
// a row someone typed by hand means by leaving it out.
func yesNo(cell string, blank bool) (bool, error) {
	switch strings.TrimSpace(cell) {
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
	if _, err := ParseWhen(cell); err != nil {
		return fmt.Errorf("added %w", err)
	}
	return nil
}

func checkURL(raw string) error {
	if raw == "" {
		return nil
	}
	if len(raw) > maxURLLength {
		return fmt.Errorf("url is too long")
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return fmt.Errorf("url %q must start with http(s)://", raw)
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

func checkTitle(kind, title string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("%s has no title", kind)
	}
	if len(title) > maxTitleLength {
		return fmt.Errorf("%s title %q is too long", kind, title)
	}
	return nil
}

func checkText(what, text string) error {
	if len(text) > maxTextLength {
		return fmt.Errorf("%s is too long", what)
	}
	return nil
}

// ParsePrice reads a dollar amount: 65, 65.00, or $65 as typed by hand.
func ParsePrice(cell string) (float64, error) {
	cell = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(cell), "$"))
	if cell == "" {
		return 0, nil
	}
	n, err := strconv.ParseFloat(cell, 64)
	if err != nil || n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("price %q is not a dollar amount", cell)
	}
	return math.Round(n*100) / 100, nil
}

// PriceCell writes a price back the way people type it: 65, or 12.50.
func PriceCell(n float64) string {
	if n == math.Trunc(n) {
		return strconv.Itoa(int(n))
	}
	return strconv.FormatFloat(n, 'f', 2, 64)
}

func parseCount(what, cell string) (int, error) {
	cell = strings.TrimSpace(cell)
	if cell == "" {
		return 0, nil
	}
	// A hand-typed 70.0 from the old sheet is 70.
	n, err := strconv.ParseFloat(cell, 64)
	if err != nil || n <= 0 || n != math.Trunc(n) {
		return 0, fmt.Errorf("%s %q must be a positive whole number or blank", what, cell)
	}
	return int(n), nil
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
	return Settings{PartiesIntro: values[PartiesIntroKey], TicketNote: values[TicketNoteKey]}, nil
}

func imageNames(rows ...[]map[string]string) []string {
	names := []string{}
	for _, list := range rows {
		for _, row := range list {
			for _, column := range []string{"Image", "Flyer Image"} {
				if name := strings.TrimSpace(row[column]); name != "" {
					names = append(names, name)
				}
			}
		}
	}
	return names
}

// BuildModel validates every row and refuses the whole set on the first
// problem, the stance every app here takes: a sheet edit that breaks a rule
// surfaces as a refused load, never as a page quietly missing a party. A row
// with a blank id is a deleted thing left in place and is skipped, as is a
// ticket or host row naming a party no live row has.
func BuildModel(tables *Tables, images ImageChecker) (*Model, error) {
	settings, err := parseSettings(tables.Settings)
	if err != nil {
		return nil, err
	}
	if err := images.Prefetch(imageNames(tables.Celebrations, tables.Parties)); err != nil {
		return nil, fmt.Errorf("prefetch images: %w", err)
	}
	model := &Model{
		Celebrations: []*Celebration{}, Categories: []string{}, Parties: []*Party{}, Settings: settings,
		Skipped: map[string]int{}, byCode: map[string]*Celebration{}, byParty: map[string]*Party{},
		byTicket: map[string]*Ticket{}, ticketParties: map[string]*Party{}, pretty: map[string]*Party{}, Redirects: []Redirect{},
	}
	for _, row := range tables.Celebrations {
		code := strings.TrimSpace(row["Code"])
		if code == "" {
			model.Skipped["celebrations without a code"]++
			continue
		}
		fail := func(err error) (*Model, error) {
			return nil, fmt.Errorf("celebration %s: %w", code, err)
		}
		if !codeForm.MatchString(code) {
			return fail(fmt.Errorf("code is not letters, digits and hyphens"))
		}
		if model.Celebration(code) != nil {
			return fail(fmt.Errorf("is listed twice"))
		}
		if err := checkTitle("celebration", row["Title"]); err != nil {
			return fail(err)
		}
		if err := checkSpan(row["Start"], row["End"]); err != nil {
			return fail(err)
		}
		if err := checkText("description", row["Description"]); err != nil {
			return fail(err)
		}
		if err := checkURL(row["Button URL"]); err != nil {
			return fail(err)
		}
		if (strings.TrimSpace(row["Button URL"]) == "") != (strings.TrimSpace(row["Button Text"]) == "") {
			return fail(fmt.Errorf("the button needs both its text and its link, or neither"))
		}
		current, err := yesNo(row["Current"], false)
		if err != nil {
			return fail(fmt.Errorf("current %w", err))
		}
		image := strings.TrimSpace(row["Image"])
		url, err := imageURL(images, image)
		if err != nil {
			return fail(err)
		}
		c := &Celebration{
			Code: code, Title: strings.TrimSpace(row["Title"]), Subtitle: strings.TrimSpace(row["Subtitle"]),
			Start: row["Start"], End: row["End"], Location: strings.TrimSpace(row["Location"]), Address: strings.TrimSpace(row["Address"]),
			Description: row["Description"], Image: image, ImageURL: url, ButtonText: strings.TrimSpace(row["Button Text"]), ButtonURL: strings.TrimSpace(row["Button URL"]), Current: current,
		}
		model.Celebrations = append(model.Celebrations, c)
		model.byCode[code] = c
	}
	current := 0
	for _, c := range model.Celebrations {
		if c.Current {
			current++
		}
	}
	if current > 1 {
		return nil, fmt.Errorf("%d celebrations are marked Current; only one may be", current)
	}

	for _, row := range tables.Categories {
		title := strings.TrimSpace(row["Title"])
		if title == "" {
			model.Skipped["categories without a title"]++
			continue
		}
		if slices.Contains(model.Categories, title) {
			return nil, fmt.Errorf("category %q is listed twice", title)
		}
		model.Categories = append(model.Categories, title)
	}

	for _, row := range tables.Parties {
		id := strings.TrimSpace(row["Party ID"])
		if id == "" {
			model.Skipped["parties without an id"]++
			continue
		}
		p, err := parseParty(row, model, images)
		if err != nil {
			return nil, fmt.Errorf("party %s (%s): %w", id, strings.TrimSpace(row["Title"]), err)
		}
		if model.Party(id) != nil {
			return nil, fmt.Errorf("party id %s is used twice", id)
		}
		if p.PrettyID != "" {
			if other := model.ByPretty(p.PrettyID); other != nil {
				return nil, fmt.Errorf("party %s (%s) and %s (%s) both have the friendly address %q", other.ID, other.Title, id, p.Title, p.PrettyID)
			}
			model.pretty[p.PrettyID] = p
		}
		model.Parties = append(model.Parties, p)
		model.byParty[id] = p
	}

	for _, row := range tables.Redirects {
		old, to := redirectPath(row["Old"]), redirectPath(row["New"])
		if old == "" || to == "" {
			model.Skipped["redirects without both ends"]++
			continue
		}
		model.Redirects = append(model.Redirects, Redirect{Type: strings.TrimSpace(row["Type"]), Old: old, New: to, Date: row["Date"]})
	}

	for _, row := range tables.Hosts {
		p := model.Party(strings.TrimSpace(row["Party ID"]))
		if p == nil {
			model.Skipped["hosts of no party"]++
			continue
		}
		email := strings.ToLower(strings.TrimSpace(row["Email"]))
		if err := checkEmail(email); err != nil {
			return nil, fmt.Errorf("host of %s: %w", p.Title, err)
		}
		if !slices.Contains(p.HostEmails, email) {
			p.HostEmails = append(p.HostEmails, email)
		}
	}

	for _, row := range tables.Tickets {
		id := strings.TrimSpace(row["Ticket ID"])
		if id == "" {
			model.Skipped["tickets without an id"]++
			continue
		}
		p := model.Party(strings.TrimSpace(row["Party ID"]))
		if p == nil {
			model.Skipped["tickets for no party"]++
			continue
		}
		t, err := parseTicket(row, id)
		if err != nil {
			return nil, fmt.Errorf("ticket %s on %s: %w", id, p.Title, err)
		}
		if _, dup := model.byTicket[id]; dup {
			return nil, fmt.Errorf("ticket id %s is used twice", id)
		}
		p.Tickets = append(p.Tickets, *t)
		model.byTicket[id] = &p.Tickets[len(p.Tickets)-1]
		model.ticketParties[id] = p
	}
	// Tickets were appended one at a time, which can move the slice; index
	// the final addresses.
	for _, p := range model.Parties {
		for i := range p.Tickets {
			model.byTicket[p.Tickets[i].ID] = &p.Tickets[i]
		}
	}
	return model, nil
}

func parseParty(row map[string]string, model *Model, images ImageChecker) (*Party, error) {
	celebration := strings.TrimSpace(row["Celebration"])
	if model.Celebration(celebration) == nil {
		return nil, fmt.Errorf("names celebration %q, which is not on the Celebrations tab", celebration)
	}
	if err := checkTitle("party", row["Title"]); err != nil {
		return nil, err
	}
	for _, what := range []string{"Summary", "Description", "Need To Know"} {
		if err := checkText(strings.ToLower(what), row[what]); err != nil {
			return nil, err
		}
	}
	category := strings.TrimSpace(row["Category"])
	if category != "" && !slices.Contains(model.Categories, category) {
		return nil, fmt.Errorf("names category %q, which is not on the Categories tab", category)
	}
	price, err := ParsePrice(row["Price"])
	if err != nil {
		return nil, err
	}
	capacity, err := parseCount("capacity", row["Capacity"])
	if err != nil {
		return nil, err
	}
	minimum, err := parseCount("minimum", row["Minimum"])
	if err != nil {
		return nil, err
	}
	if err := checkSpan(row["Start"], row["End"]); err != nil {
		return nil, err
	}
	status := strings.TrimSpace(row["Status"])
	if !slices.Contains(Statuses, status) {
		return nil, fmt.Errorf("status %q is not one of %s", status, strings.Join(Statuses, ", "))
	}
	ticketsOpen := true
	switch strings.TrimSpace(row["Tickets"]) {
	case "", "Open":
	case "Closed":
		ticketsOpen = false
	default:
		return nil, fmt.Errorf("tickets %q is not Open, Closed, or blank", row["Tickets"])
	}
	flags := map[string]bool{}
	for _, f := range []struct {
		column string
		blank  bool
	}{{"Waitlist", true}, {"Parents", true}, {"Students", false}, {"Staff", true}, {"Drop-Off", false}, {"Parent Ticket Required", false}} {
		v, err := yesNo(row[f.column], f.blank)
		if err != nil {
			return nil, fmt.Errorf("%s %w", strings.ToLower(f.column), err)
		}
		flags[f.column] = v
	}
	if !flags["Parents"] && !flags["Students"] && !flags["Staff"] {
		return nil, fmt.Errorf("allows nobody: parents, students, and staff are all No")
	}
	if row["Added By"] != "" {
		if err := checkEmail(strings.ToLower(strings.TrimSpace(row["Added By"]))); err != nil {
			return nil, fmt.Errorf("added by %w", err)
		}
	}
	if err := checkAdded(row["Added"]); err != nil {
		return nil, err
	}
	image := strings.TrimSpace(row["Image"])
	url, err := imageURL(images, image)
	if err != nil {
		return nil, err
	}
	flyer := strings.TrimSpace(row["Flyer Image"])
	flyerURL, err := imageURL(images, flyer)
	if err != nil {
		return nil, fmt.Errorf("flyer %w", err)
	}
	pretty := NormalizePretty(row["Pretty ID"])
	if err := CheckPretty(pretty); err != nil {
		return nil, err
	}
	return &Party{
		ID: strings.TrimSpace(row["Party ID"]), Celebration: celebration, Title: strings.TrimSpace(row["Title"]),
		Subtitle: strings.TrimSpace(row["Subtitle"]), Summary: strings.TrimSpace(row["Summary"]), Description: row["Description"],
		NeedToKnow: strings.TrimSpace(row["Need To Know"]), Hosts: strings.TrimSpace(row["Hosts"]), Category: category,
		Audience: strings.TrimSpace(row["Audience"]), Unit: strings.TrimSpace(row["Ticket Unit"]), Price: price,
		Capacity: capacity, Minimum: minimum, Start: row["Start"], End: row["End"], Location: strings.TrimSpace(row["Location"]), Address: strings.TrimSpace(row["Address"]),
		Image: image, ImageURL: url, Flyer: flyer, FlyerURL: flyerURL, PrettyID: pretty, Status: status, TicketsOpen: ticketsOpen, Waitlist: flags["Waitlist"],
		Parents: flags["Parents"], Students: flags["Students"], Staff: flags["Staff"], DropOff: flags["Drop-Off"],
		ParentTicket: flags["Parent Ticket Required"], AddedBy: strings.ToLower(strings.TrimSpace(row["Added By"])), Added: row["Added"],
		HostEmails: []string{}, Tickets: []Ticket{},
	}, nil
}

func parseTicket(row map[string]string, id string) (*Ticket, error) {
	email := strings.ToLower(strings.TrimSpace(row["Email"]))
	name := strings.TrimSpace(row["Name"])
	if email == "" && name == "" {
		return nil, fmt.Errorf("names nobody: it needs an email or a guest's name")
	}
	if email != "" {
		if err := checkEmail(email); err != nil {
			return nil, err
		}
	}
	if len(name) > maxNameLength {
		return nil, fmt.Errorf("name is too long")
	}
	purchaser := strings.ToLower(strings.TrimSpace(row["Purchaser"]))
	if err := checkEmail(purchaser); err != nil {
		return nil, fmt.Errorf("purchaser %w", err)
	}
	status := strings.TrimSpace(row["Status"])
	if status == "" {
		status = TicketSold
	}
	if !slices.Contains(TicketStatuses, status) {
		return nil, fmt.Errorf("status %q is not one of %s", status, strings.Join(TicketStatuses, ", "))
	}
	price, err := ParsePrice(row["Price"])
	if err != nil {
		return nil, err
	}
	if err := checkText("note", row["Note"]); err != nil {
		return nil, err
	}
	invoice := strings.TrimSpace(row["Invoice"])
	if !slices.Contains(InvoiceStatuses, invoice) {
		return nil, fmt.Errorf("invoice %q is not Sent, Paid, or blank", invoice)
	}
	if row["Added By"] != "" {
		if err := checkEmail(strings.ToLower(strings.TrimSpace(row["Added By"]))); err != nil {
			return nil, fmt.Errorf("added by %w", err)
		}
	}
	if err := checkAdded(row["Added"]); err != nil {
		return nil, err
	}
	return &Ticket{
		ID: id, PartyID: strings.TrimSpace(row["Party ID"]), Email: email, Name: name, Purchaser: purchaser,
		Status: status, Price: price, Note: strings.TrimSpace(row["Note"]), Invoice: invoice,
		AddedBy: strings.ToLower(strings.TrimSpace(row["Added By"])), Added: row["Added"],
	}, nil
}

// SortedParties is the parties of one celebration in date order, undated ones
// last, each group by title.
func (m *Model) SortedParties(celebration string) []*Party {
	out := []*Party{}
	for _, p := range m.Parties {
		if celebration == "" || p.Celebration == celebration {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.Start == "") != (b.Start == "") {
			return a.Start != ""
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.Title < b.Title
	})
	return out
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
	case celebrationsTab:
		return t.Celebrations
	case categoriesTab:
		return t.Categories
	case partiesTab:
		return t.Parties
	case hostsTab:
		return t.Hosts
	case ticketsTab:
		return t.Tickets
	case settingsTab:
		return t.Settings
	case redirectsTab:
		return t.Redirects
	}
	return t.Admins
}

func (t *Tables) setTab(name string, rows []map[string]string) {
	switch name {
	case celebrationsTab:
		t.Celebrations = rows
	case categoriesTab:
		t.Categories = rows
	case partiesTab:
		t.Parties = rows
	case hostsTab:
		t.Hosts = rows
	case ticketsTab:
		t.Tickets = rows
	case settingsTab:
		t.Settings = rows
	case redirectsTab:
		t.Redirects = rows
	default:
		t.Admins = rows
	}
}

// with mirrors what data.Writer.Set is about to do to one tab, so the model
// can be rebuilt and checked before anything is persisted. A nil match appends.
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
