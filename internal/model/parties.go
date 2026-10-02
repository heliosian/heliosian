package model

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

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

const (
	partiesAppName     = "celebrate"
	celebrationsTab    = "Celebrations"
	partyCategoriesTab = "Categories"
	partiesTab         = "Parties"
	hostsTab           = "Hosts"
	ticketsTab         = "Tickets"
	partySettingsTab   = "Settings"
	partyRedirectsTab  = "Redirects"
	invoicingTab       = "INVOICING"
	formerTab          = "Former Addresses"
)

const (
	maxPartyTitleLength = 120
	maxGuestNameLength  = 120
)

const (
	StatusOpen   = "Open"
	StatusHidden = "Hidden"
)

var PartyStatuses = []string{StatusPending, StatusOpen, StatusHidden}

const (
	TicketSold     = "Ticket"
	TicketWaitlist = "Waitlist"
	TicketOffered  = "Offered"
)

var TicketStatuses = []string{TicketSold, TicketWaitlist, TicketOffered}

const (
	PartiesIntroKey = "Parties Intro"
	TicketNoteKey   = "Ticket Note"
	ButtonCalendar  = "calendar"
	HostingOpenKey  = "Hosting Open"
)

var partySettingKeys = []string{PartiesIntroKey, TicketNoteKey, HostingOpenKey}

var (
	CelebrationColumns   = []string{"Celebration ID", "Code", "Title", "Subtitle", "Start", "End", "Location", "Address", "Description", "Image", "Button Text", "Button URL", "Current", "Banner"}
	PartyCategoryColumns = []string{"Category ID", "Title", store.OrderColumn}
	PartyColumns         = []string{"Party ID", "Celebration", "Title", "Subtitle", "Summary", "Description", "Need To Know", "Note Emoji", "Note Title", "Hosts", "Category", "Audience", "Ticket Unit", "Price", "Capacity", "Minimum", "Start", "End", "Location", "Address", "Image", "Flyer Image", "Pretty ID", "Status", "Tickets", "Waitlist", "Adults", "Students", "Drop-Off", "Parent Ticket Required", "Added By", "Added"}
	HostColumns          = []string{"Party ID", "Email"}
	TicketColumns        = []string{"Ticket ID", "Party ID", "Email", "Name", "Purchaser", "Status", "Quantity", "Price", "Note", "Added By", "Added"}
	KeyValueColumns      = []string{"Key", "Value"}
	RedirectColumns      = []string{"Type", "Old", "New", "Date"}
	InvoicingColumns     = []string{"Date", "Party ID", "Party Title", "Celebration", "Purchaser Email", "Guest Name", "Action", "Quantity", "Cost", "Invoice", "Invoice To"}
	FormerColumns        = []string{"Old", "New", "Name", "Changed"}
)

type PartyRedirect struct {
	Type string `json:"type"`
	Old  string `json:"old"`
	New  string `json:"new"`
	Date string `json:"date,omitempty"`
}

const RedirectParty = "Party"

func partyRedirectPath(cell string) string {
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

type Celebration struct {
	ID          string `json:"id"`
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

type PartyCategory struct {
	ID    string `json:"id"`
	Title string `json:"title"`
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

type PartiesSettings struct {
	PartiesIntro string `json:"partiesIntro"`
	TicketNote   string `json:"ticketNote"`
	HostingOpen  bool   `json:"hostingOpen"`
}

type Parties struct {
	Celebrations  []*Celebration
	Categories    []PartyCategory
	Parties       []*Party
	Settings      PartiesSettings
	Redirects     []PartyRedirect
	Invoicing     []InvoiceLine
	Skipped       map[string]int
	admins        []string
	aliases       id.Aliases
	categoryOrder map[string]string
	byCode        map[string]*Celebration
	byCelebration map[string]*Celebration
	byParty       map[string]*Party
	byTicket      map[string]*Ticket
	ticketParties map[string]*Party
	pretty        map[string]*Party
	former        map[string]Former
}

type Former struct {
	New     string
	Name    string
	Changed string
}

func (m *Parties) CurrentAddress(email string) string {
	if f, ok := m.former[email]; ok {
		return f.New
	}
	return email
}

type InvoiceLine struct {
	Date        string  `json:"date"`
	PartyID     string  `json:"partyId"`
	Party       string  `json:"party"`
	Celebration string  `json:"celebration"`
	Code        string  `json:"code"`
	Purchaser   string  `json:"purchaser"`
	Guest       string  `json:"guest"`
	Action      string  `json:"action"`
	Quantity    int     `json:"quantity"`
	Cost        float64 `json:"cost"`
	Invoice     string  `json:"invoice"`
	InvoiceTo   string  `json:"invoiceTo"`
}

func (m *Parties) ByPretty(pretty string) *Party {
	return m.pretty[cells.NormalizePretty(pretty)]
}

func (m *Parties) PathOf(p *Party) string {
	return partyPath(p.ID, p.PrettyID)
}

func partyPath(id, pretty string) string {
	if pretty != "" {
		return "/p/" + pretty
	}
	return "/parties/" + id
}

func (m *Parties) walk(path string) *Party {
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

func (m *Parties) Resolve(path string) *Party {
	at := partyRedirectPath(path)
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

func (m *Parties) Celebration(code string) *Celebration {
	return m.byCode[code]
}

func (m *Parties) CelebrationByID(key string) *Celebration {
	return m.byCelebration[key]
}

func (m *Parties) Category(key string) *PartyCategory {
	for i := range m.Categories {
		if m.Categories[i].ID == key {
			return &m.Categories[i]
		}
	}
	return nil
}

func (m *Parties) categoryTitled(title string) *PartyCategory {
	for i := range m.Categories {
		if m.Categories[i].Title == title {
			return &m.Categories[i]
		}
	}
	return nil
}

func (m *Parties) taken(key string) bool {
	_, alias := m.aliases[key]
	_, ticket := m.byTicket[key]
	return alias || ticket || m.byParty[key] != nil || m.byCelebration[key] != nil || m.Category(key) != nil
}

func (m *Parties) minter() func() string {
	minted := map[string]bool{}
	return func() string {
		s := id.New(func(key string) bool { return minted[key] || m.taken(key) })
		minted[s] = true
		return s
	}
}

func (m *Parties) Current() *Celebration {
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

func (m *Parties) Banner() *Celebration {
	for _, c := range m.Celebrations {
		if c.Banner {
			return c
		}
	}
	return m.Current()
}

func (m *Parties) Party(key string) *Party {
	return m.byParty[m.aliases.Resolve(key)]
}

func (m *Parties) TicketByID(key string) (*Ticket, *Party) {
	key = m.aliases.Resolve(key)
	return m.byTicket[key], m.ticketParties[key]
}

func (p *Party) Hosted(email string) bool {
	return slices.Contains(p.HostEmails, email)
}

func (p *Party) Edits(v access.Actor) bool {
	return v.May(ActAsPartyHost) || p.Hosted(v.Email)
}

func (p *Party) Sees(v access.Actor) bool {
	return v.May(SeeAllParties) || p.Hosted(v.Email)
}

func (p *Party) VisibleTo(v access.Actor) bool {
	return p.Status == StatusOpen || p.Sees(v)
}

func (p *Party) For(v access.Actor, directory *Directory) *Party {
	if !p.VisibleTo(v) {
		return nil
	}
	c := *p
	c.Tickets = []Ticket{}
	for _, t := range p.Tickets {
		if !p.Sees(v) && !v.Mine(t.Purchaser) && !v.Mine(t.Email) {
			if t.Email != "" && (t.Email == t.Purchaser || directory.Member(directory.Resolve(t.Email))) {
				t.Purchaser = ""
			}
			t.Price, t.Note, t.AddedBy = 0, "", ""
		}
		c.Tickets = append(c.Tickets, t)
	}
	return &c
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

func (p *Party) OffersTo(directory *Directory, email string) []Ticket {
	family := Billable(directory, email)
	out := []Ticket{}
	for _, t := range p.Tickets {
		if t.Status == TicketOffered && slices.Contains(family, t.Purchaser) {
			out = append(out, t)
		}
	}
	return out
}

func (p *Party) OfferedTo(directory *Directory, email string) int {
	n := 0
	for _, t := range p.OffersTo(directory, email) {
		n += t.Quantity
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
	t, _ := cells.When(p.Start)
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
	t, err := cells.When(cell)
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

func checkEmail(email string) error {
	if !emailForm.MatchString(email) {
		return fmt.Errorf("%q is not an email address", email)
	}
	if email != strings.ToLower(email) {
		return fmt.Errorf("email %q is not lowercase", email)
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

func parsePartySettings(rows []store.Row) (PartiesSettings, error) {
	values, err := store.ParseSettings(rows, partySettingKeys, partySettingKeys)
	if err != nil {
		return PartiesSettings{}, err
	}
	hosting, err := cells.YesNo(values[HostingOpenKey], true)
	if err != nil {
		return PartiesSettings{}, fmt.Errorf("%s %s: %w", partySettingsTab, HostingOpenKey, err)
	}
	return PartiesSettings{PartiesIntro: values[PartiesIntroKey], TicketNote: values[TicketNoteKey], HostingOpen: hosting}, nil
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

func BuildParties(ctx context.Context, tables store.Tables, images blob.Checker) (*Parties, error) {
	settings, err := parsePartySettings(tables[partySettingsTab])
	if err != nil {
		return nil, err
	}
	if err := images.Prefetch(ctx, cells.ImageNames([]string{"Image", "Flyer Image"}, tables[celebrationsTab], tables[partiesTab])); err != nil {
		return nil, fmt.Errorf("prefetch images: %w", err)
	}
	aliases, err := id.ParseAliases(tables[id.AliasesTab])
	if err != nil {
		return nil, err
	}
	m := &Parties{
		Celebrations: []*Celebration{}, Categories: []PartyCategory{}, Parties: []*Party{}, Settings: settings, aliases: aliases,
		Skipped: map[string]int{}, categoryOrder: map[string]string{}, byCode: map[string]*Celebration{}, byCelebration: map[string]*Celebration{}, byParty: map[string]*Party{},
		byTicket: map[string]*Ticket{}, ticketParties: map[string]*Party{}, pretty: map[string]*Party{}, Redirects: []PartyRedirect{}, Invoicing: []InvoiceLine{},
		former: map[string]Former{},
	}
	m.admins = ReadAdmins(tables)
	if err := m.readCelebrations(tables[celebrationsTab], images); err != nil {
		return nil, err
	}
	if err := m.readCategories(tables[partyCategoriesTab]); err != nil {
		return nil, err
	}
	if err := m.readParties(tables[partiesTab], images); err != nil {
		return nil, err
	}
	m.readRedirects(tables[partyRedirectsTab])
	if err := m.readInvoicing(tables[invoicingTab]); err != nil {
		return nil, err
	}
	if err := m.readHosts(tables[hostsTab]); err != nil {
		return nil, err
	}
	if err := m.readFormer(tables[formerTab]); err != nil {
		return nil, err
	}
	if err := m.readTickets(tables[ticketsTab]); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Parties) readCelebrations(rows []store.Row, images blob.Checker) error {
	for _, row := range rows {
		code := strings.TrimSpace(row["Code"])
		if code == "" {
			m.Skipped["celebrations without a code"]++
			continue
		}
		c, err := m.parseCelebration(row, code, images)
		if err != nil {
			return fmt.Errorf("celebration %s: %w", code, err)
		}
		m.Celebrations = append(m.Celebrations, c)
		m.byCode[code] = c
		m.byCelebration[c.ID] = c
	}
	slices.SortStableFunc(m.Celebrations, func(a, b *Celebration) int { return strings.Compare(b.Start, a.Start) })
	current, banner := 0, 0
	for _, c := range m.Celebrations {
		if c.Current {
			current++
		}
		if c.Banner {
			banner++
		}
	}
	if current > 1 {
		return fmt.Errorf("%d celebrations are marked Current; only one may be", current)
	}
	if banner > 1 {
		return fmt.Errorf("%d celebrations are marked Banner; only one may be", banner)
	}
	return nil
}

func (m *Parties) parseCelebration(row store.Row, code string, images blob.Checker) (*Celebration, error) {
	if !codeForm.MatchString(code) {
		return nil, fmt.Errorf("code is not letters, digits and hyphens")
	}
	if _, ok := id.Parse(code); ok {
		return nil, fmt.Errorf("code reads as an id")
	}
	if m.Celebration(code) != nil {
		return nil, fmt.Errorf("is listed twice")
	}
	key, ok := id.Parse(row["Celebration ID"])
	if !ok {
		return nil, fmt.Errorf("celebration id %q is not an id", row["Celebration ID"])
	}
	if m.byCelebration[key] != nil {
		return nil, fmt.Errorf("celebration id %s is used twice", key)
	}
	if err := cells.Title("celebration", row["Title"], maxPartyTitleLength); err != nil {
		return nil, err
	}
	if err := cells.Span(row["Start"], row["End"]); err != nil {
		return nil, err
	}
	if err := checkText("description", row["Description"]); err != nil {
		return nil, err
	}
	if link := strings.TrimSpace(row["Button URL"]); link != ButtonCalendar {
		if err := cells.URL(link, true); err != nil {
			return nil, err
		}
	} else if strings.TrimSpace(row["Start"]) == "" {
		return nil, fmt.Errorf("a calendar button needs a start date")
	}
	if (strings.TrimSpace(row["Button URL"]) == "") != (strings.TrimSpace(row["Button Text"]) == "") {
		return nil, fmt.Errorf("the button needs both its text and its link, or neither")
	}
	current, err := cells.YesNo(row["Current"], false)
	if err != nil {
		return nil, fmt.Errorf("current %w", err)
	}
	banner, err := cells.YesNo(row["Banner"], false)
	if err != nil {
		return nil, fmt.Errorf("banner %w", err)
	}
	image := strings.TrimSpace(row["Image"])
	url, err := cells.ImageURL(images, image)
	if err != nil {
		return nil, err
	}
	return &Celebration{
		ID: key, Code: code, Title: strings.TrimSpace(row["Title"]), Subtitle: strings.TrimSpace(row["Subtitle"]),
		Start: row["Start"], End: row["End"], Location: strings.TrimSpace(row["Location"]), Address: strings.TrimSpace(row["Address"]),
		Description: row["Description"], Image: image, ImageURL: url, ButtonText: strings.TrimSpace(row["Button Text"]), ButtonURL: strings.TrimSpace(row["Button URL"]), Current: current, Banner: banner,
	}, nil
}

func (m *Parties) readCategories(rows []store.Row) error {
	for _, row := range rows {
		title := strings.TrimSpace(row["Title"])
		if title == "" {
			m.Skipped["categories without a title"]++
			continue
		}
		if m.categoryTitled(title) != nil {
			return fmt.Errorf("category %q is listed twice", title)
		}
		key, ok := id.Parse(row["Category ID"])
		if !ok {
			return fmt.Errorf("category %q: category id %q is not an id", title, row["Category ID"])
		}
		if m.Category(key) != nil {
			return fmt.Errorf("category id %s is used twice", key)
		}
		order := strings.TrimSpace(row[store.OrderColumn])
		if err := store.CheckKey(order); err != nil {
			return fmt.Errorf("category %q: %w", title, err)
		}
		m.Categories = append(m.Categories, PartyCategory{ID: key, Title: title})
		m.categoryOrder[key] = order
	}
	slices.SortStableFunc(m.Categories, func(a, b PartyCategory) int {
		return store.CompareKeys(m.categoryOrder[a.ID], m.categoryOrder[b.ID])
	})
	return nil
}

func (m *Parties) readParties(rows []store.Row, images blob.Checker) error {
	for _, row := range rows {
		id := strings.TrimSpace(row["Party ID"])
		if id == "" {
			m.Skipped["parties without an id"]++
			continue
		}
		p, err := parseParty(row, m, images)
		if err != nil {
			return fmt.Errorf("party %s (%s): %w", id, strings.TrimSpace(row["Title"]), err)
		}
		if m.byParty[id] != nil {
			return fmt.Errorf("party id %s is used twice", id)
		}
		if p.PrettyID != "" {
			if other := m.ByPretty(p.PrettyID); other != nil {
				return fmt.Errorf("party %s (%s) and %s (%s) both have the friendly address %q", other.ID, other.Title, id, p.Title, p.PrettyID)
			}
			m.pretty[p.PrettyID] = p
		}
		m.Parties = append(m.Parties, p)
		m.byParty[id] = p
	}
	return nil
}

func (m *Parties) readRedirects(rows []store.Row) {
	for _, row := range rows {
		old, to := partyRedirectPath(row["Old"]), partyRedirectPath(row["New"])
		if old == "" || to == "" {
			m.Skipped["redirects without both ends"]++
			continue
		}
		m.Redirects = append(m.Redirects, PartyRedirect{Type: strings.TrimSpace(row["Type"]), Old: old, New: to, Date: row["Date"]})
	}
}

func (m *Parties) readInvoicing(rows []store.Row) error {
	for _, row := range rows {
		partyID, item, purchaser := strings.TrimSpace(row["Party ID"]), strings.TrimSpace(row["Party Title"]), mail.Normalize(row["Purchaser Email"])
		if (partyID == "" && item == "") || purchaser == "" {
			m.Skipped["invoicing rows naming no party or purchaser"]++
			continue
		}
		if partyID != "" && item != "" {
			return fmt.Errorf("%s row of %s for %s names both party %s and %q", invoicingTab, row["Date"], purchaser, partyID, item)
		}
		title := item
		if partyID != "" {
			title = partyID
		}
		if p := m.Party(partyID); p != nil {
			title = p.Title
		}
		celebration := strings.TrimSpace(row["Celebration"])
		code := celebration
		if c := m.CelebrationByID(celebration); c != nil {
			code = c.Code
		}
		quantity, _ := strconv.Atoi(strings.TrimSpace(row["Quantity"]))
		cost, _ := ParsePrice(row["Cost"])
		m.Invoicing = append(m.Invoicing, InvoiceLine{
			Date: strings.TrimSpace(row["Date"]), PartyID: partyID, Party: title, Celebration: celebration, Code: code, Purchaser: purchaser,
			Guest: strings.TrimSpace(row["Guest Name"]), Action: strings.TrimSpace(row["Action"]), Quantity: quantity, Cost: cost,
			Invoice: strings.TrimSpace(row["Invoice"]), InvoiceTo: strings.TrimSpace(row["Invoice To"]),
		})
	}
	return nil
}

func (m *Parties) readHosts(rows []store.Row) error {
	for _, row := range rows {
		p := m.byParty[strings.TrimSpace(row["Party ID"])]
		if p == nil {
			m.Skipped["hosts of no party"]++
			continue
		}
		email := strings.ToLower(strings.TrimSpace(row["Email"]))
		if err := checkEmail(email); err != nil {
			return fmt.Errorf("host of %s: %w", p.Title, err)
		}
		if !slices.Contains(p.HostEmails, email) {
			p.HostEmails = append(p.HostEmails, email)
		}
	}
	return nil
}

func (m *Parties) readFormer(rows []store.Row) error {
	for _, row := range rows {
		old, to := mail.Normalize(row["Old"]), mail.Normalize(row["New"])
		if old == "" && to == "" {
			continue
		}
		f, err := m.parseFormer(row, old, to)
		if err != nil {
			return fmt.Errorf("former address %s: %w", old, err)
		}
		m.former[old] = f
	}
	for old, f := range m.former {
		if _, ok := m.former[f.New]; ok {
			return fmt.Errorf("former address %s moves to %s, which has itself moved: point it at where that went", old, f.New)
		}
	}
	return nil
}

func (m *Parties) parseFormer(row store.Row, old, to string) (Former, error) {
	if err := checkEmail(old); err != nil {
		return Former{}, err
	}
	if err := checkEmail(to); err != nil {
		return Former{}, fmt.Errorf("new %w", err)
	}
	if old == to {
		return Former{}, fmt.Errorf("moves to itself")
	}
	if _, dup := m.former[old]; dup {
		return Former{}, fmt.Errorf("is listed twice")
	}
	name := strings.TrimSpace(row["Name"])
	if len(name) > maxGuestNameLength {
		return Former{}, fmt.Errorf("name is too long")
	}
	return Former{New: to, Name: name, Changed: strings.TrimSpace(row["Changed"])}, nil
}

func (m *Parties) readTickets(rows []store.Row) error {
	for _, row := range rows {
		id := strings.TrimSpace(row["Ticket ID"])
		if id == "" {
			m.Skipped["tickets without an id"]++
			continue
		}
		p := m.byParty[strings.TrimSpace(row["Party ID"])]
		if p == nil {
			m.Skipped["tickets for no party"]++
			continue
		}
		t, err := parseTicket(row, id)
		if err != nil {
			return fmt.Errorf("ticket %s on %s: %w", id, p.Title, err)
		}
		if f, ok := m.former[t.Email]; ok {
			t.Email = f.New
			if t.Name == "" {
				t.Name = f.Name
			}
		}
		t.Purchaser = m.CurrentAddress(t.Purchaser)
		if _, dup := m.byTicket[id]; dup {
			return fmt.Errorf("ticket id %s is used twice", id)
		}
		p.Tickets = append(p.Tickets, *t)
		m.byTicket[id] = t
		m.ticketParties[id] = p
	}
	for _, p := range m.Parties {
		slices.SortStableFunc(p.Tickets, compareAdded)
		for i := range p.Tickets {
			m.byTicket[p.Tickets[i].ID] = &p.Tickets[i]
		}
	}
	return nil
}

func parseParty(row store.Row, m *Parties, images blob.Checker) (*Party, error) {
	celebration := strings.TrimSpace(row["Celebration"])
	if m.CelebrationByID(celebration) == nil {
		return nil, fmt.Errorf("names celebration %q, which is not on the Celebrations tab", celebration)
	}
	if err := cells.Title("party", row["Title"], maxPartyTitleLength); err != nil {
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
	if category != "" && m.Category(category) == nil {
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
	if err := cells.Span(row["Start"], row["End"]); err != nil {
		return nil, err
	}
	status := strings.TrimSpace(row["Status"])
	if !slices.Contains(PartyStatuses, status) {
		return nil, fmt.Errorf("status %q is not one of %s", status, strings.Join(PartyStatuses, ", "))
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
		v, err := cells.YesNo(row[f.column], f.blank)
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
	if err := cells.Added(row["Added"]); err != nil {
		return nil, err
	}
	image := strings.TrimSpace(row["Image"])
	url, err := cells.ImageURL(images, image)
	if err != nil {
		return nil, err
	}
	flyer := strings.TrimSpace(row["Flyer Image"])
	flyerURL, err := cells.ImageURL(images, flyer)
	if err != nil {
		return nil, fmt.Errorf("flyer %w", err)
	}
	pretty := cells.NormalizePretty(row["Pretty ID"])
	if err := cells.CheckPretty(pretty); err != nil {
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
	if len(name) > maxGuestNameLength {
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
	if err := cells.Added(row["Added"]); err != nil {
		return nil, err
	}
	return &Ticket{
		ID: id, PartyID: strings.TrimSpace(row["Party ID"]), Email: email, Name: name, Purchaser: purchaser,
		Status: status, Quantity: quantity, Price: price, Note: strings.TrimSpace(row["Note"]),
		AddedBy: strings.ToLower(strings.TrimSpace(row["Added By"])), Added: row["Added"],
	}, nil
}

func todayLocal() string {
	return now().Format(DateFormat)
}

func (m *Parties) SortedParties(celebration string) []*Party {
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
