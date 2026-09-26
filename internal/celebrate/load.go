package celebrate

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"heliosian/internal/config"
	"heliosian/internal/store"
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
	invoicingTab    = "INVOICING"
	formerTab       = "Former Addresses"
)

const (
	DateFormat     = "2006-01-02"
	DateTimeFormat = "2006-01-02 15:04"
	maxTitleLength = 120
	maxTextLength  = 6000
	maxURLLength   = 1000
	maxNameLength  = 120
)

const (
	StatusPending = "Pending"
	StatusOpen    = "Open"
	StatusHidden  = "Hidden"
)

var Statuses = []string{StatusPending, StatusOpen, StatusHidden}

const (
	TicketSold     = "Ticket"
	TicketWaitlist = "Waitlist"
)

var TicketStatuses = []string{TicketSold, TicketWaitlist}

const (
	PartiesIntroKey = "Parties Intro"
	TicketNoteKey   = "Ticket Note"
	ButtonCalendar  = "calendar"
	HostingOpenKey  = "Hosting Open"
)

var settingKeys = []string{PartiesIntroKey, TicketNoteKey, HostingOpenKey}

var legacyThemeKeys = []string{"Sidebar Color", "Sidebar Color 2", "Sidebar Text Color", "Page Color", "Page Color 2", "Logo", "Sidebar Image"}

var (
	CelebrationColumns = []string{"Code", "Title", "Subtitle", "Start", "End", "Location", "Address", "Description", "Image", "Button Text", "Button URL", "Current", "Banner"}
	CategoryColumns    = []string{"Title", store.OrderColumn}
	PartyColumns       = []string{"Party ID", "Celebration", "Title", "Subtitle", "Summary", "Description", "Need To Know", "Note Emoji", "Note Title", "Hosts", "Category", "Audience", "Ticket Unit", "Price", "Capacity", "Minimum", "Start", "End", "Location", "Address", "Image", "Flyer Image", "Pretty ID", "Status", "Tickets", "Waitlist", "Adults", "Students", "Drop-Off", "Parent Ticket Required", "Added By", "Added"}
	HostColumns        = []string{"Party ID", "Email"}
	TicketColumns      = []string{"Ticket ID", "Party ID", "Email", "Name", "Purchaser", "Status", "Quantity", "Price", "Note", "Added By", "Added"}
	SettingColumns     = []string{"Key", "Value"}
	AdminColumns       = []string{"Email"}
	RedirectColumns    = []string{"Type", "Old", "New", "Date"}
	InvoicingColumns   = []string{"Date", "Party Title", "Event Code", "Purchaser Email", "Guest Name", "Action", "Quantity", "Cost", "Invoice", "Invoice To"}
	FormerColumns      = []string{"Old", "New", "Name", "Changed"}
)

var emailForm = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

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

var codeForm = regexp.MustCompile(`^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$`)

type ImageChecker interface {
	Has(key string) (bool, error)
	Prefetch(ctx context.Context, names []string) error
}

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
	ButtonText  string `json:"buttonText,omitempty"`
	ButtonURL   string `json:"buttonUrl,omitempty"`
	Current     bool   `json:"current"`
	Banner      bool   `json:"banner"`
}

type Party struct {
	ID           string   `json:"id"`
	Celebration  string   `json:"celebration"`
	Title        string   `json:"title"`
	Subtitle     string   `json:"subtitle,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	Description  string   `json:"description,omitempty"`
	NeedToKnow   string   `json:"needToKnow,omitempty"`
	NoteEmoji    string   `json:"noteEmoji,omitempty"`
	NoteTitle    string   `json:"noteTitle,omitempty"`
	Hosts        string   `json:"hosts,omitempty"`
	Category     string   `json:"category,omitempty"`
	Audience     string   `json:"audience,omitempty"`
	Unit         string   `json:"unit,omitempty"`
	Price        float64  `json:"price"`
	Capacity     int      `json:"capacity,omitempty"`
	Minimum      int      `json:"minimum,omitempty"`
	Start        string   `json:"start,omitempty"`
	End          string   `json:"end,omitempty"`
	Location     string   `json:"location,omitempty"`
	Address      string   `json:"address,omitempty"`
	Image        string   `json:"image,omitempty"`
	ImageURL     string   `json:"imageUrl,omitempty"`
	Flyer        string   `json:"flyer,omitempty"`
	FlyerURL     string   `json:"flyerUrl,omitempty"`
	PrettyID     string   `json:"prettyId,omitempty"`
	Status       string   `json:"status"`
	TicketsOpen  bool     `json:"ticketsOpen"`
	Waitlist     bool     `json:"waitlist"`
	Adults       bool     `json:"adults"`
	Students     bool     `json:"students"`
	DropOff      bool     `json:"dropOff"`
	ParentTicket bool     `json:"parentTicket"`
	AddedBy      string   `json:"addedBy,omitempty"`
	Added        string   `json:"added,omitempty"`
	HostEmails   []string `json:"hostEmails"`
	Tickets      []Ticket `json:"-"`
}

type Ticket struct {
	ID        string  `json:"ticketId"`
	PartyID   string  `json:"partyId"`
	Email     string  `json:"email,omitempty"`
	Name      string  `json:"name,omitempty"`
	Purchaser string  `json:"purchaser"`
	Status    string  `json:"status"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
	Note      string  `json:"note,omitempty"`
	AddedBy   string  `json:"addedBy,omitempty"`
	Added     string  `json:"added,omitempty"`
}

type Settings struct {
	PartiesIntro string `json:"partiesIntro"`
	TicketNote   string `json:"ticketNote"`
	HostingOpen  bool   `json:"hostingOpen"`
}

type Model struct {
	Celebrations  []*Celebration
	Categories    []string
	Parties       []*Party
	Settings      Settings
	Redirects     []Redirect
	Invoicing     []InvoiceLine
	Skipped       map[string]int
	admins        []string
	categoryOrder map[string]string
	byCode        map[string]*Celebration
	byParty       map[string]*Party
	byTicket      map[string]*Ticket
	ticketParties map[string]*Party
	pretty        map[string]*Party
	former        map[string]Former
}

// Former is where an address nobody reads any more - an alum's closed school
// account - has moved to, and the name to call its owner by, since the
// directory no longer holds them.
type Former struct {
	New     string
	Name    string
	Changed string
}

// CurrentAddress is the address to use for email: the one it moved to, when
// it is a former address, and itself otherwise.
func (m *Model) CurrentAddress(email string) string {
	if f, ok := m.former[email]; ok {
		return f.New
	}
	return email
}

type InvoiceLine struct {
	Date      string  `json:"date"`
	Party     string  `json:"party"`
	Code      string  `json:"code"`
	Purchaser string  `json:"purchaser"`
	Guest     string  `json:"guest"`
	Action    string  `json:"action"`
	Quantity  int     `json:"quantity"`
	Cost      float64 `json:"cost"`
	Invoice   string  `json:"invoice"`
	InvoiceTo string  `json:"invoiceTo"`
}

func (m *Model) ByPretty(pretty string) *Party {
	return m.pretty[NormalizePretty(pretty)]
}

func (m *Model) PathOf(p *Party) string {
	return partyPath(p.ID, p.PrettyID)
}

func partyPath(id, pretty string) string {
	if pretty != "" {
		return "/p/" + pretty
	}
	return "/parties/" + id
}

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

func (m *Model) Banner() *Celebration {
	for _, c := range m.Celebrations {
		if c.Banner {
			return c
		}
	}
	return m.Current()
}

func (m *Model) Party(id string) *Party {
	return m.byParty[id]
}

func (m *Model) TicketByID(id string) (*Ticket, *Party) {
	return m.byTicket[id], m.ticketParties[id]
}

func (p *Party) Hosted(email string) bool {
	return slices.Contains(p.HostEmails, email)
}

func (p *Party) VisibleTo(email string, admin bool) bool {
	return p.Status == StatusOpen || admin || p.Hosted(email)
}

func (p *Party) Sold() int {
	n := 0
	for _, t := range p.Tickets {
		if t.Status == TicketSold {
			n++
		}
	}
	return n
}

func (p *Party) Raised() float64 {
	sum := 0.0
	for _, t := range p.Tickets {
		if t.Status == TicketSold {
			sum += t.Price
		}
	}
	return sum
}

func (p *Party) Waiting() int {
	n := 0
	for _, t := range p.Tickets {
		if t.Status == TicketWaitlist {
			n += t.Quantity
		}
	}
	return n
}

func (p *Party) Remaining() int {
	if p.Capacity == 0 {
		return -1
	}
	return max(0, p.Capacity-p.Sold())
}

func (p *Party) Full() bool {
	return p.Capacity > 0 && p.Sold() >= p.Capacity
}

func (p *Party) StartTime() time.Time {
	t, _ := ParseWhen(p.Start)
	return t
}

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

func checkNote(emoji, title string) error {
	if n := utf8.RuneCountInString(strings.TrimSpace(emoji)); n > 8 {
		return fmt.Errorf("note emoji %q is too long", emoji)
	}
	if len(strings.TrimSpace(title)) > 60 {
		return fmt.Errorf("note title %q is too long", title)
	}
	return nil
}

func checkText(what, text string) error {
	if len(text) > maxTextLength {
		return fmt.Errorf("%s is too long", what)
	}
	return nil
}

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

func parseSettings(rows []store.Row) (Settings, error) {
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
	hosting, err := yesNo(values[HostingOpenKey], true)
	if err != nil {
		return Settings{}, fmt.Errorf("%s %s: %w", settingsTab, HostingOpenKey, err)
	}
	return Settings{PartiesIntro: values[PartiesIntroKey], TicketNote: values[TicketNoteKey], HostingOpen: hosting}, nil
}

func imageNames(rows ...[]store.Row) []string {
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

func compareAdded(a, b Ticket) int {
	switch {
	case a.Added == b.Added:
		return 0
	case a.Added == "":
		return 1
	case b.Added == "":
		return -1
	}
	return strings.Compare(a.Added, b.Added)
}

func BuildModel(ctx context.Context, tables store.Tables, images ImageChecker) (*Model, error) {
	settings, err := parseSettings(tables[settingsTab])
	if err != nil {
		return nil, err
	}
	if err := images.Prefetch(ctx, imageNames(tables[celebrationsTab], tables[partiesTab])); err != nil {
		return nil, fmt.Errorf("prefetch images: %w", err)
	}
	model := &Model{
		Celebrations: []*Celebration{}, Categories: []string{}, Parties: []*Party{}, Settings: settings,
		Skipped: map[string]int{}, categoryOrder: map[string]string{}, byCode: map[string]*Celebration{}, byParty: map[string]*Party{},
		byTicket: map[string]*Ticket{}, ticketParties: map[string]*Party{}, pretty: map[string]*Party{}, Redirects: []Redirect{}, Invoicing: []InvoiceLine{},
		former: map[string]Former{},
	}
	admins := []string{}
	for _, row := range tables[adminsTab] {
		admins = append(admins, row["Email"])
	}
	model.admins = config.NormalizeEmails(admins)
	for _, row := range tables[celebrationsTab] {
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
		if link := strings.TrimSpace(row["Button URL"]); link != ButtonCalendar {
			if err := checkURL(link); err != nil {
				return fail(err)
			}
		} else if strings.TrimSpace(row["Start"]) == "" {
			return fail(fmt.Errorf("a calendar button needs a start date"))
		}
		if (strings.TrimSpace(row["Button URL"]) == "") != (strings.TrimSpace(row["Button Text"]) == "") {
			return fail(fmt.Errorf("the button needs both its text and its link, or neither"))
		}
		current, err := yesNo(row["Current"], false)
		if err != nil {
			return fail(fmt.Errorf("current %w", err))
		}
		banner, err := yesNo(row["Banner"], false)
		if err != nil {
			return fail(fmt.Errorf("banner %w", err))
		}
		image := strings.TrimSpace(row["Image"])
		url, err := imageURL(images, image)
		if err != nil {
			return fail(err)
		}
		c := &Celebration{
			Code: code, Title: strings.TrimSpace(row["Title"]), Subtitle: strings.TrimSpace(row["Subtitle"]),
			Start: row["Start"], End: row["End"], Location: strings.TrimSpace(row["Location"]), Address: strings.TrimSpace(row["Address"]),
			Description: row["Description"], Image: image, ImageURL: url, ButtonText: strings.TrimSpace(row["Button Text"]), ButtonURL: strings.TrimSpace(row["Button URL"]), Current: current, Banner: banner,
		}
		model.Celebrations = append(model.Celebrations, c)
		model.byCode[code] = c
	}
	slices.SortStableFunc(model.Celebrations, func(a, b *Celebration) int { return strings.Compare(b.Start, a.Start) })
	current, banner := 0, 0
	for _, c := range model.Celebrations {
		if c.Current {
			current++
		}
		if c.Banner {
			banner++
		}
	}
	if current > 1 {
		return nil, fmt.Errorf("%d celebrations are marked Current; only one may be", current)
	}
	if banner > 1 {
		return nil, fmt.Errorf("%d celebrations are marked Banner; only one may be", banner)
	}

	for _, row := range tables[categoriesTab] {
		title := strings.TrimSpace(row["Title"])
		if title == "" {
			model.Skipped["categories without a title"]++
			continue
		}
		if slices.Contains(model.Categories, title) {
			return nil, fmt.Errorf("category %q is listed twice", title)
		}
		order := strings.TrimSpace(row[store.OrderColumn])
		if err := store.CheckKey(order); err != nil {
			return nil, fmt.Errorf("category %q: %w", title, err)
		}
		model.Categories = append(model.Categories, title)
		model.categoryOrder[title] = order
	}
	slices.SortStableFunc(model.Categories, func(a, b string) int {
		return store.CompareKeys(model.categoryOrder[a], model.categoryOrder[b])
	})

	for _, row := range tables[partiesTab] {
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

	for _, row := range tables[redirectsTab] {
		old, to := redirectPath(row["Old"]), redirectPath(row["New"])
		if old == "" || to == "" {
			model.Skipped["redirects without both ends"]++
			continue
		}
		model.Redirects = append(model.Redirects, Redirect{Type: strings.TrimSpace(row["Type"]), Old: old, New: to, Date: row["Date"]})
	}

	for _, row := range tables[invoicingTab] {
		title, purchaser := strings.TrimSpace(row["Party Title"]), cleanEmail(row["Purchaser Email"])
		if title == "" || purchaser == "" {
			model.Skipped["invoicing rows naming no party or purchaser"]++
			continue
		}
		quantity, _ := strconv.Atoi(strings.TrimSpace(row["Quantity"]))
		cost, _ := ParsePrice(row["Cost"])
		model.Invoicing = append(model.Invoicing, InvoiceLine{
			Date: strings.TrimSpace(row["Date"]), Party: title, Code: strings.TrimSpace(row["Event Code"]), Purchaser: purchaser,
			Guest: strings.TrimSpace(row["Guest Name"]), Action: strings.TrimSpace(row["Action"]), Quantity: quantity, Cost: cost,
			Invoice: strings.TrimSpace(row["Invoice"]), InvoiceTo: strings.TrimSpace(row["Invoice To"]),
		})
	}

	for _, row := range tables[hostsTab] {
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

	for _, row := range tables[formerTab] {
		old, to := cleanEmail(row["Old"]), cleanEmail(row["New"])
		if old == "" && to == "" {
			continue
		}
		fail := func(err error) (*Model, error) {
			return nil, fmt.Errorf("former address %s: %w", old, err)
		}
		if err := checkEmail(old); err != nil {
			return fail(err)
		}
		if err := checkEmail(to); err != nil {
			return fail(fmt.Errorf("new %w", err))
		}
		if old == to {
			return fail(fmt.Errorf("moves to itself"))
		}
		if _, dup := model.former[old]; dup {
			return fail(fmt.Errorf("is listed twice"))
		}
		name := strings.TrimSpace(row["Name"])
		if len(name) > maxNameLength {
			return fail(fmt.Errorf("name is too long"))
		}
		model.former[old] = Former{New: to, Name: name, Changed: strings.TrimSpace(row["Changed"])}
	}
	for old, f := range model.former {
		if _, ok := model.former[f.New]; ok {
			return nil, fmt.Errorf("former address %s moves to %s, which has itself moved: point it at where that went", old, f.New)
		}
	}

	for _, row := range tables[ticketsTab] {
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
		// A ticket still naming a former address is read as naming where it
		// went, so a row pasted into Former Addresses by hand takes at once.
		if f, ok := model.former[t.Email]; ok {
			t.Email = f.New
			if t.Name == "" {
				t.Name = f.Name
			}
		}
		t.Purchaser = model.CurrentAddress(t.Purchaser)
		if _, dup := model.byTicket[id]; dup {
			return nil, fmt.Errorf("ticket id %s is used twice", id)
		}
		p.Tickets = append(p.Tickets, *t)
		model.byTicket[id] = t
		model.ticketParties[id] = p
	}
	for _, p := range model.Parties {
		slices.SortStableFunc(p.Tickets, compareAdded)
		for i := range p.Tickets {
			model.byTicket[p.Tickets[i].ID] = &p.Tickets[i]
		}
	}
	return model, nil
}

func parseParty(row store.Row, model *Model, images ImageChecker) (*Party, error) {
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
	if err := checkNote(row["Note Emoji"], row["Note Title"]); err != nil {
		return nil, err
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
	}{{"Waitlist", true}, {"Adults", true}, {"Students", false}, {"Drop-Off", false}, {"Parent Ticket Required", false}} {
		v, err := yesNo(row[f.column], f.blank)
		if err != nil {
			return nil, fmt.Errorf("%s %w", strings.ToLower(f.column), err)
		}
		flags[f.column] = v
	}
	if !flags["Adults"] && !flags["Students"] {
		return nil, fmt.Errorf("allows nobody: adults and students are both No")
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
		NeedToKnow: strings.TrimSpace(row["Need To Know"]), NoteEmoji: strings.TrimSpace(row["Note Emoji"]), NoteTitle: strings.TrimSpace(row["Note Title"]),
		Hosts: strings.TrimSpace(row["Hosts"]), Category: category,
		Audience: strings.TrimSpace(row["Audience"]), Unit: strings.TrimSpace(row["Ticket Unit"]), Price: price,
		Capacity: capacity, Minimum: minimum, Start: row["Start"], End: row["End"], Location: strings.TrimSpace(row["Location"]), Address: strings.TrimSpace(row["Address"]),
		Image: image, ImageURL: url, Flyer: flyer, FlyerURL: flyerURL, PrettyID: pretty, Status: status, TicketsOpen: ticketsOpen, Waitlist: flags["Waitlist"],
		Adults: flags["Adults"], Students: flags["Students"], DropOff: flags["Drop-Off"],
		ParentTicket: flags["Parent Ticket Required"], AddedBy: strings.ToLower(strings.TrimSpace(row["Added By"])), Added: row["Added"],
		HostEmails: []string{}, Tickets: []Ticket{},
	}, nil
}

func parseTicket(row store.Row, id string) (*Ticket, error) {
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
	quantity, err := parseCount("quantity", row["Quantity"])
	if err != nil {
		return nil, err
	}
	if quantity == 0 || status == TicketSold {
		quantity = 1
	}
	if err := checkText("note", row["Note"]); err != nil {
		return nil, err
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
		Status: status, Quantity: quantity, Price: price, Note: strings.TrimSpace(row["Note"]),
		AddedBy: strings.ToLower(strings.TrimSpace(row["Added By"])), Added: row["Added"],
	}, nil
}

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
