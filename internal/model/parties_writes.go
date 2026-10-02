package model

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

var (
	SeeAllParties    = access.Named("celebrate.see-all")
	CurateParties    = access.Named("celebrate.curate")
	ConfigureParties = access.Named("celebrate.configure")
	MoveAddresses    = access.Named("celebrate.move-addresses")
	ActAsPartyHost   = access.Named("celebrate.act-as-host")
)

var PartiesAdminAllowances = []access.Allowance{SeeAllParties, CurateParties, ConfigureParties, MoveAddresses, ActAsPartyHost}

func isKid(directory *Directory, email string) bool {
	person := directory.Person(email)
	return person != nil && person.IsStudent && !person.IsParent && !person.IsStaff
}

func owns(t *Ticket, actor access.Actor) bool {
	return actor.Mine(t.Purchaser) || actor.Mine(t.Email)
}

func (m *Parties) findParty(id string) (*Party, error) {
	p := m.Party(strings.TrimSpace(id))
	if p == nil {
		return nil, access.Missing("no party with id %q", id)
	}
	return p, nil
}

func (m *Parties) findTicket(id string) (*Ticket, *Party, error) {
	t, p := m.TicketByID(strings.TrimSpace(id))
	if t == nil {
		return nil, nil, access.Missing("no such ticket")
	}
	return t, p, nil
}

func current(m *Parties, directory *Directory, email string) string {
	return m.CurrentAddress(directory.Resolve(email))
}

func ticketsCell(open bool) string {
	return map[bool]string{true: "Open", false: "Closed"}[open]
}

func countCell(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func invoiceRow(directory *Directory, p *Party, cells store.Row) store.Row {
	price, _ := ParsePrice(cells["Price"])
	if cells["Status"] != TicketSold || price <= 0 {
		return nil
	}
	return store.Row{
		"Date": todayLocal(), "Party ID": p.ID, "Celebration": p.Celebration, "Purchaser Email": cells["Purchaser"],
		"Guest Name": ticketName(directory, cells), "Action": "ADD", "Quantity": "1", "Cost": PriceCell(price),
	}
}

func ticketOps(directory *Directory, p *Party, added []store.Row) []store.Op {
	ops := []store.Op{}
	for _, cells := range added {
		ops = append(ops, store.Insert(ticketsTab, cells))
		if row := invoiceRow(directory, p, cells); row != nil {
			ops = append(ops, store.Insert(invoicingTab, row))
		}
	}
	return ops
}

func waitlistRequest(ticketID string, p *Party, purchaser string, quantity int, note, actor string) store.Row {
	return store.Row{
		"Ticket ID": ticketID, "Party ID": p.ID, "Email": purchaser, "Name": "", "Purchaser": purchaser,
		"Status": TicketWaitlist, "Quantity": strconv.Itoa(quantity), "Price": PriceCell(p.Price), "Note": note, "Added By": actor, "Added": stamp(),
	}
}

type ticketOrder struct {
	PartyID       string `json:"partyId"`
	Purchaser     string `json:"purchaser"`
	Note          string `json:"note"`
	Free          bool   `json:"free"`
	RaiseCapacity bool   `json:"raiseCapacity"`
	Attendees     []struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	} `json:"attendees"`
}

type taken struct {
	party      *Party
	purchaser  string
	added      []store.Row
	ops        []store.Op
	sold       int
	waitlisted int
}

func (m *Parties) takeTickets(actor access.Actor, directory *Directory, order ticketOrder) (taken, error) {
	p, err := m.findParty(order.PartyID)
	if err != nil {
		return taken{}, err
	}
	editor := p.Edits(actor)
	if !editor && isKid(directory, actor.Email) {
		return taken{}, access.Forbidden("tickets are taken by a parent - ask yours to sign in")
	}
	if order.Free && !editor {
		return taken{}, access.Forbidden("only the hosts can give a free ticket")
	}
	if len(order.Attendees) == 0 {
		return taken{}, access.Invalid("pick at least one person")
	}
	if len(order.Attendees) > maxTicketsPerPurchase {
		return taken{}, access.Invalid("at most %d tickets at a time", maxTicketsPerPurchase)
	}
	if err := checkText("note", order.Note); err != nil {
		return taken{}, access.Invalid("%s", err)
	}
	availability := p.Availability(now())
	offers := p.OffersTo(directory, actor.Email)
	offered := p.OfferedTo(directory, actor.Email)
	if !editor && availability == Past {
		return taken{}, access.Invalid("this party has already happened")
	}
	if !editor && offered == 0 {
		switch availability {
		case Closed:
			return taken{}, access.Invalid("tickets are closed for this party")
		case SoldOut:
			return taken{}, access.Invalid("this party is sold out")
		case Waitlist:
			return taken{}, access.Invalid("this party is full; join the waitlist instead")
		}
	}
	purchaser := mail.Normalize(order.Purchaser)
	if purchaser == "" {
		purchaser = actor.Email
	}
	if err := checkEmail(purchaser); err != nil {
		return taken{}, access.Invalid("%s", err)
	}
	gift := order.Free && purchaser != actor.Email
	if gift {
		if host := directory.Person(purchaser); host == nil || host.IsStudent {
			return taken{}, access.Invalid("a guest's host is an adult in the directory")
		}
	} else if !slices.Contains(Billable(directory, actor.Email), purchaser) {
		return taken{}, access.Forbidden("tickets are billed to you or another adult in your family")
	}
	type row struct {
		email, name, purchaser string
	}
	rows := []row{}
	seen := map[string]bool{}
	for _, att := range order.Attendees {
		email, name := mail.Normalize(att.Email), strings.TrimSpace(att.Name)
		if email == "" && name == "" {
			return taken{}, access.Invalid("each ticket needs a person or a guest's name")
		}
		if len(name) > maxGuestNameLength {
			return taken{}, access.Invalid("a guest's name is too long")
		}
		if email == "" {
			rows = append(rows, row{"", name, purchaser})
			continue
		}
		if err := checkEmail(email); err != nil {
			return taken{}, access.Invalid("%s", err)
		}
		email = current(m, directory, email)
		if seen[email] {
			continue
		}
		seen[email] = true
		person := directory.Person(email)
		if person == nil && name != "" {
			rows = append(rows, row{email, name, purchaser})
			continue
		}
		bill := purchaser
		if !actor.Mine(email) && !gift {
			if !editor {
				return taken{}, access.Forbidden("you can take tickets for yourself and your family; a friend goes in as a guest by name")
			}
			switch {
			case person != nil && person.IsStudent && len(person.ParentContactEmails) > 0:
				bill = directory.Resolve(person.ParentContactEmails[0])
			case person != nil && person.IsStudent:
				return taken{}, access.Invalid("%s has no parent on file to bill", person.FullName)
			default:
				bill = email
			}
		}
		if !p.Admits(person) {
			return taken{}, access.Invalid("%s can't hold a ticket: this party is for %s", person.FullName, audienceWords(p))
		}
		for _, t := range p.Tickets {
			if t.Email == email && t.Status == TicketSold {
				return taken{}, access.Invalid("%s already has a ticket", nameOf(directory, email))
			}
		}
		rows = append(rows, row{email, "", bill})
	}
	remaining := p.Remaining()
	mint := m.minter()
	out := taken{party: p, purchaser: purchaser, added: []store.Row{}, ops: []store.Op{}}
	price := PriceCell(p.Price)
	if order.Free {
		price = "0"
	}
	used := 0
	for _, rw := range rows {
		switch {
		case editor:
		case used < offered:
			used++
		case availability == Closed:
			return taken{}, access.Invalid("your family was offered %d %s", offered, plural(offered, "ticket"))
		case remaining < 0:
		case remaining > 0:
			remaining--
		case p.Waitlist:
			out.waitlisted++
			continue
		default:
			return taken{}, access.Invalid("only %d %s left", p.Remaining(), plural(p.Remaining(), "ticket"))
		}
		out.sold++
		out.added = append(out.added, store.Row{
			"Ticket ID": mint(), "Party ID": p.ID, "Email": rw.email, "Name": rw.name, "Purchaser": rw.purchaser,
			"Status": TicketSold, "Quantity": "1", "Price": price, "Note": strings.TrimSpace(order.Note), "Added By": actor.Email, "Added": stamp(),
		})
	}
	if out.waitlisted > 0 {
		out.added = append(out.added, waitlistRequest(mint(), p, purchaser, out.waitlisted, strings.TrimSpace(order.Note), actor.Email))
	}
	if order.Free && order.RaiseCapacity && p.Capacity > 0 && out.sold > 0 {
		out.ops = append(out.ops, store.Update(partiesTab, store.Row{"Party ID": p.ID}, store.Row{"Capacity": countCell(p.Capacity + out.sold)}))
	}
	out.ops = append(out.ops, ticketOps(directory, p, out.added)...)
	for _, o := range offers {
		if used == 0 {
			break
		}
		match := store.Row{"Ticket ID": o.ID}
		if used < o.Quantity {
			out.ops = append(out.ops, store.Update(ticketsTab, match, store.Row{"Quantity": strconv.Itoa(o.Quantity - used)}))
			break
		}
		used -= o.Quantity
		out.ops = append(out.ops, store.Delete(ticketsTab, match))
	}
	return out, nil
}

type waitlistOrder struct {
	PartyID   string `json:"partyId"`
	Purchaser string `json:"purchaser"`
	Quantity  int    `json:"quantity"`
	Note      string `json:"note"`
}

type joined struct {
	party     *Party
	purchaser string
	cells     map[string]string
	ops       []store.Op
	changed   bool
}

func (m *Parties) joinWaitlist(actor access.Actor, directory *Directory, order waitlistOrder) (joined, error) {
	p, err := m.findParty(order.PartyID)
	if err != nil {
		return joined{}, err
	}
	if !p.Edits(actor) && p.Availability(now()) != Waitlist {
		return joined{}, access.Invalid("this party is not taking a waitlist right now")
	}
	if order.Quantity < 1 || order.Quantity > maxTicketsPerPurchase {
		return joined{}, access.Invalid("ask for between 1 and %d tickets", maxTicketsPerPurchase)
	}
	if err := checkText("note", order.Note); err != nil {
		return joined{}, access.Invalid("%s", err)
	}
	purchaser := mail.Normalize(order.Purchaser)
	if purchaser == "" {
		purchaser = actor.Email
	}
	if isKid(directory, actor.Email) {
		return joined{}, access.Forbidden("the waitlist is joined by a parent - ask yours to sign in")
	}
	if !slices.Contains(Billable(directory, actor.Email), purchaser) {
		return joined{}, access.Forbidden("the waitlist is for your own family, billed to an adult in it")
	}
	note := strings.TrimSpace(order.Note)
	for _, t := range p.Tickets {
		if t.Status == TicketWaitlist && t.Purchaser == purchaser {
			cells := store.Row{"Quantity": strconv.Itoa(order.Quantity), "Note": note}
			full := map[string]string{"Email": t.Email, "Purchaser": t.Purchaser, "Status": TicketWaitlist, "Quantity": cells["Quantity"], "Note": cells["Note"]}
			return joined{party: p, purchaser: purchaser, cells: full, changed: true, ops: []store.Op{store.Update(ticketsTab, store.Row{"Ticket ID": t.ID}, cells)}}, nil
		}
	}
	cells := waitlistRequest(id.New(m.taken), p, purchaser, order.Quantity, note, actor.Email)
	return joined{party: p, purchaser: purchaser, cells: cells, ops: []store.Op{store.Insert(ticketsTab, cells)}}, nil
}

type offer struct {
	TicketID string `json:"ticketId"`
	Quantity int    `json:"quantity"`
}

type offered struct {
	party   *Party
	ticket  *Ticket
	ops     []store.Op
	offered int
	left    int
}

func (m *Parties) offerTickets(actor access.Actor, o offer) (offered, error) {
	t, p, err := m.findTicket(o.TicketID)
	if err != nil {
		return offered{}, err
	}
	if !p.Edits(actor) {
		return offered{}, access.Forbidden("only a host or admin can offer tickets")
	}
	if t.Status != TicketWaitlist {
		return offered{}, access.Invalid("this is not a waitlist request")
	}
	n := o.Quantity
	if n <= 0 || n > t.Quantity {
		n = t.Quantity
	}
	match := store.Row{"Ticket ID": t.ID}
	left := t.Quantity - n
	ops := []store.Op{}
	if left > 0 {
		ops = append(ops, store.Update(ticketsTab, match, store.Row{"Quantity": strconv.Itoa(left)}))
	} else {
		ops = append(ops, store.Delete(ticketsTab, match))
	}
	for _, other := range p.Tickets {
		if other.Status == TicketOffered && other.Purchaser == t.Purchaser {
			ops = append(ops, store.Update(ticketsTab, store.Row{"Ticket ID": other.ID}, store.Row{"Quantity": strconv.Itoa(other.Quantity + n)}))
			return offered{party: p, ticket: t, ops: ops, offered: n, left: left}, nil
		}
	}
	ops = append(ops, store.Insert(ticketsTab, store.Row{
		"Ticket ID": m.minter()(), "Party ID": p.ID, "Email": t.Email, "Purchaser": t.Purchaser, "Status": TicketOffered, "Quantity": strconv.Itoa(n),
		"Price": PriceCell(t.Price), "Note": t.Note, "Added By": actor.Email, "Added": stamp(),
	}))
	return offered{party: p, ticket: t, ops: ops, offered: n, left: left}, nil
}

func (m *Parties) removeTicket(actor access.Actor, id string) (*Ticket, *Party, []store.Op, error) {
	t, p, err := m.findTicket(id)
	if err != nil {
		return nil, nil, nil, err
	}
	editor := p.Edits(actor)
	if !editor && t.Status == TicketSold {
		return nil, nil, nil, access.Forbidden("tickets can't be given back; ask the party's host, or resell it to another family")
	}
	if !editor && t.Status == TicketOffered {
		return nil, nil, nil, access.Forbidden("only a host can withdraw an offer")
	}
	if !editor && !owns(t, actor) {
		return nil, nil, nil, access.Forbidden("only the family that asked, a host, or an admin can remove this")
	}
	if !editor && p.Past(now()) {
		return nil, nil, nil, access.Invalid("this party has already happened")
	}
	return t, p, []store.Op{store.Delete(ticketsTab, store.Row{"Ticket ID": t.ID})}, nil
}

type ticketEdit struct {
	TicketID string  `json:"ticketId"`
	Quantity *int    `json:"quantity"`
	Note     *string `json:"note"`
}

func (m *Parties) editTicket(actor access.Actor, edit ticketEdit) (*Ticket, *Party, []store.Op, []string, error) {
	t, p, err := m.findTicket(edit.TicketID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	editor := p.Edits(actor)
	cells := store.Row{}
	details := []string{}
	if edit.Quantity != nil {
		if !editor && !owns(t, actor) {
			return nil, nil, nil, nil, access.Forbidden("only the family that asked, a host, or an admin can change a request")
		}
		if t.Status != TicketWaitlist {
			return nil, nil, nil, nil, access.Invalid("only a waitlist request has a quantity")
		}
		if *edit.Quantity < 1 || *edit.Quantity > maxTicketsPerPurchase {
			return nil, nil, nil, nil, access.Invalid("ask for between 1 and %d tickets", maxTicketsPerPurchase)
		}
		cells["Quantity"] = strconv.Itoa(*edit.Quantity)
		details = append(details, fmt.Sprintf("×%d", *edit.Quantity))
	}
	if edit.Note != nil {
		if !editor && !owns(t, actor) {
			return nil, nil, nil, nil, access.Forbidden("only the ticket's family, a host, or an admin can change the note")
		}
		if err := checkText("note", *edit.Note); err != nil {
			return nil, nil, nil, nil, access.Invalid("%s", err)
		}
		cells["Note"] = strings.TrimSpace(*edit.Note)
		details = append(details, "note")
	}
	if len(cells) == 0 {
		return nil, nil, nil, nil, access.Invalid("nothing to change")
	}
	return t, p, []store.Op{store.Update(ticketsTab, store.Row{"Ticket ID": t.ID}, cells)}, details, nil
}

type reassignment struct {
	TicketID string `json:"ticketId"`
	Email    string `json:"email"`
	Name     string `json:"name"`
}

func (m *Parties) reassignTicket(actor access.Actor, directory *Directory, re reassignment) (*Ticket, *Party, []store.Op, string, error) {
	t, p, err := m.findTicket(re.TicketID)
	if err != nil {
		return nil, nil, nil, "", err
	}
	editor := p.Edits(actor)
	if !editor && (!owns(t, actor) || isKid(directory, actor.Email)) {
		return nil, nil, nil, "", access.Forbidden("only a parent in the family that holds this ticket, a host, or an admin can reassign it")
	}
	if t.Status != TicketSold {
		return nil, nil, nil, "", access.Invalid("only a sold ticket can be reassigned")
	}
	if !editor && p.Past(now()) {
		return nil, nil, nil, "", access.Invalid("this party has already happened")
	}
	email, name := mail.Normalize(re.Email), strings.TrimSpace(re.Name)
	if email == "" && name == "" {
		return nil, nil, nil, "", access.Invalid("pick someone, or name a guest")
	}
	if len(name) > maxGuestNameLength {
		return nil, nil, nil, "", access.Invalid("the name is too long")
	}
	if email != "" {
		if err := checkEmail(email); err != nil {
			return nil, nil, nil, "", access.Invalid("%s", err)
		}
		email = current(m, directory, email)
		person := directory.Person(email)
		if !p.Admits(person) {
			return nil, nil, nil, "", access.Invalid("%s can't hold a ticket: this party is for %s", person.FullName, audienceWords(p))
		}
		if person != nil {
			name = ""
		}
		for _, other := range p.Tickets {
			if other.ID != t.ID && other.Email == email && other.Status == TicketSold {
				return nil, nil, nil, "", access.Invalid("%s already has a ticket", nameOf(directory, email))
			}
		}
	}
	who := email
	if who == "" {
		who = name
	}
	return t, p, []store.Op{store.Update(ticketsTab, store.Row{"Ticket ID": t.ID}, store.Row{"Email": email, "Name": name})}, who, nil
}

type partyBody struct {
	ID           string   `json:"id"`
	Celebration  string   `json:"celebration"`
	Title        string   `json:"title"`
	Subtitle     string   `json:"subtitle"`
	Summary      string   `json:"summary"`
	Description  string   `json:"description"`
	NeedToKnow   string   `json:"needToKnow"`
	NoteEmoji    string   `json:"noteEmoji"`
	NoteTitle    string   `json:"noteTitle"`
	Hosts        string   `json:"hosts"`
	HostEmails   []string `json:"hostEmails"`
	Category     string   `json:"category"`
	Audience     string   `json:"audience"`
	Unit         string   `json:"unit"`
	Price        float64  `json:"price"`
	Capacity     int      `json:"capacity"`
	Minimum      int      `json:"minimum"`
	Start        string   `json:"start"`
	End          string   `json:"end"`
	Location     string   `json:"location"`
	Address      string   `json:"address"`
	Image        string   `json:"image"`
	Flyer        string   `json:"flyer"`
	PrettyID     string   `json:"prettyId"`
	Status       string   `json:"status"`
	TicketsOpen  bool     `json:"ticketsOpen"`
	Waitlist     bool     `json:"waitlist"`
	Adults       bool     `json:"adults"`
	Students     bool     `json:"students"`
	DropOff      bool     `json:"dropOff"`
	ParentTicket bool     `json:"parentTicket"`
}

type partyPrettyConflict struct {
	Message string `json:"error"`
	ID      string `json:"id"`
	Title   string `json:"title"`
}

func (c *partyPrettyConflict) refusal() error {
	return &access.Refusal{Status: http.StatusConflict, Message: c.Message, Body: c}
}

type savedParty struct {
	id     string
	title  string
	status string
	adding bool
	ops    []store.Op
}

var partyColumns = map[string][]string{
	"celebration": {"Celebration"}, "title": {"Title"}, "subtitle": {"Subtitle"}, "summary": {"Summary"}, "description": {"Description"},
	"needToKnow": {"Need To Know"}, "noteEmoji": {"Note Emoji"}, "noteTitle": {"Note Title"}, "hosts": {"Hosts"}, "category": {"Category"},
	"audience": {"Audience"}, "unit": {"Ticket Unit"}, "price": {"Price"}, "capacity": {"Capacity"}, "minimum": {"Minimum"},
	"start": {"Start"}, "end": {"End"}, "location": {"Location"}, "address": {"Address"}, "image": {"Image"}, "flyer": {"Flyer Image"},
	"prettyId": {"Pretty ID"}, "status": {"Status"}, "ticketsOpen": {"Tickets"}, "waitlist": {"Waitlist"}, "adults": {"Adults"},
	"students": {"Students"}, "dropOff": {"Drop-Off"}, "parentTicket": {"Parent Ticket Required"},
}

func partyBodyOf(p *Party) partyBody {
	return partyBody{
		ID: p.ID, Celebration: p.Celebration, Title: p.Title, Subtitle: p.Subtitle, Summary: p.Summary, Description: p.Description,
		NeedToKnow: p.NeedToKnow, NoteEmoji: p.NoteEmoji, NoteTitle: p.NoteTitle, Hosts: p.Hosts, HostEmails: slices.Clone(p.HostEmails),
		Category: p.Category, Audience: p.Audience, Unit: p.Unit, Price: p.Price, Capacity: p.Capacity, Minimum: p.Minimum,
		Start: p.Start, End: p.End, Location: p.Location, Address: p.Address, Image: p.Image, Flyer: p.Flyer, PrettyID: p.PrettyID,
		Status: p.Status, TicketsOpen: p.TicketsOpen, Waitlist: p.Waitlist, Adults: p.Adults, Students: p.Students, DropOff: p.DropOff, ParentTicket: p.ParentTicket,
	}
}

func (m *Parties) saveParty(actor access.Actor, body partyBody, sent map[string]bool) (savedParty, error) {
	adding := strings.TrimSpace(body.ID) == ""
	var was *Party
	if !adding {
		p, err := m.findParty(body.ID)
		if err != nil {
			return savedParty{}, err
		}
		if !p.Edits(actor) {
			return savedParty{}, access.Forbidden("only a host or admin can edit this party")
		}
		was = p
	}
	celebration := strings.TrimSpace(body.Celebration)
	status := strings.TrimSpace(body.Status)
	category := strings.TrimSpace(body.Category)
	switch {
	case adding && actor.May(CurateParties):
		if status == "" {
			status = StatusOpen
		}
	case adding:
		if !m.Settings.HostingOpen {
			return savedParty{}, access.Forbidden("hosting is closed for now - an admin can open it in Settings")
		}
		status, category = StatusPending, ""
		if c := m.Current(); c != nil {
			celebration = c.ID
		}
	case actor.May(CurateParties):
		if status == "" {
			status = was.Status
		}
	default:
		if (sent["status"] && status != was.Status) || (sent["celebration"] && celebration != was.Celebration) || (sent["category"] && category != was.Category) {
			return savedParty{}, access.Forbidden("only an admin can change a party's status, celebration or category")
		}
		status, celebration, category = was.Status, was.Celebration, was.Category
	}
	if !slices.Contains(PartyStatuses, status) {
		return savedParty{}, access.Invalid("status must be one of %s", strings.Join(PartyStatuses, ", "))
	}
	if celebration == "" {
		if c := m.Current(); c != nil {
			celebration = c.ID
		}
	}
	if body.Price < 0 || body.Capacity < 0 || body.Minimum < 0 {
		return savedParty{}, access.Invalid("price, capacity, and minimum can't be negative")
	}
	if !adding && body.Price != was.Price && was.Raised() > 0 {
		return savedParty{}, access.Invalid("the price can't change once tickets have been bought")
	}
	if !body.Adults && !body.Students {
		return savedParty{}, access.Invalid("let adults, students, or both hold a ticket")
	}
	hosts := mail.NormalizeAll(body.HostEmails)
	if adding && !actor.May(CurateParties) && !slices.Contains(hosts, actor.Email) {
		hosts = append(hosts, actor.Email)
	}
	for _, h := range hosts {
		if err := checkEmail(h); err != nil {
			return savedParty{}, access.Invalid("%s", err)
		}
	}
	if !adding && !actor.May(CurateParties) && !slices.Contains(hosts, actor.Email) {
		return savedParty{}, access.Invalid("you can't remove yourself as a host; ask another host or an admin")
	}
	var partyID string
	if adding {
		partyID = id.New(m.taken)
	} else {
		partyID = was.ID
	}
	pretty := cells.NormalizePretty(body.PrettyID)
	if cells.CheckPretty(pretty) != nil {
		return savedParty{}, access.Invalid("the friendly address can be only lower-case letters, digits and hyphens, at most %d", cells.MaxPrettyLength)
	}
	if other := m.ByPretty(pretty); pretty != "" && other != nil && other.ID != partyID {
		conflict := &partyPrettyConflict{ID: other.ID, Title: other.Title, Message: fmt.Sprintf("%q is already the address of %s", pretty, other.Title)}
		return savedParty{}, conflict.refusal()
	}
	row := store.Row{
		"Celebration": celebration, "Title": strings.TrimSpace(body.Title), "Subtitle": strings.TrimSpace(body.Subtitle),
		"Summary": strings.TrimSpace(body.Summary), "Description": strings.TrimSpace(body.Description), "Need To Know": strings.TrimSpace(body.NeedToKnow),
		"Note Emoji": strings.TrimSpace(body.NoteEmoji), "Note Title": strings.TrimSpace(body.NoteTitle),
		"Hosts": strings.TrimSpace(body.Hosts), "Category": category, "Audience": strings.TrimSpace(body.Audience),
		"Ticket Unit": strings.TrimSpace(body.Unit), "Price": PriceCell(body.Price), "Capacity": countCell(body.Capacity), "Minimum": countCell(body.Minimum),
		"Start": strings.TrimSpace(body.Start), "End": strings.TrimSpace(body.End), "Location": strings.TrimSpace(body.Location),
		"Address": strings.TrimSpace(body.Address), "Pretty ID": pretty, "Image": strings.TrimSpace(body.Image), "Flyer Image": strings.TrimSpace(body.Flyer), "Status": status, "Tickets": ticketsCell(body.TicketsOpen),
		"Waitlist": cells.YesNoCell(body.Waitlist), "Adults": cells.YesNoCell(body.Adults), "Students": cells.YesNoCell(body.Students),
		"Drop-Off": cells.YesNoCell(body.DropOff), "Parent Ticket Required": cells.YesNoCell(body.ParentTicket),
	}
	ops := []store.Op{}
	before := []string{}
	if adding {
		row["Party ID"] = partyID
		row["Added By"] = actor.Email
		row["Added"] = todayLocal()
		ops = append(ops, store.Insert(partiesTab, row))
	} else {
		before = was.HostEmails
		for field, columns := range partyColumns {
			if sent[field] {
				continue
			}
			for _, column := range columns {
				delete(row, column)
			}
		}
		if len(row) > 0 {
			ops = append(ops, store.Update(partiesTab, store.Row{"Party ID": partyID}, row))
		}
		if !sent["hostEmails"] {
			hosts = before
		}
	}
	for _, h := range before {
		if !slices.Contains(hosts, h) {
			ops = append(ops, store.Delete(hostsTab, store.Row{"Party ID": partyID, "Email": h}))
		}
	}
	for _, h := range hosts {
		if !slices.Contains(before, h) {
			ops = append(ops, store.Insert(hostsTab, store.Row{"Party ID": partyID, "Email": h}))
		}
	}
	return savedParty{id: partyID, title: strings.TrimSpace(body.Title), status: status, adding: adding, ops: ops}, nil
}

func (m *Parties) deleteParty(actor access.Actor, id string) (*Party, []store.Op, error) {
	if err := require(actor, CurateParties); err != nil {
		return nil, nil, err
	}
	p, err := m.findParty(id)
	if err != nil {
		return nil, nil, err
	}
	if len(p.Tickets) > 0 {
		return nil, nil, access.Invalid("remove its tickets and waitlist first, or hide it instead")
	}
	return p, []store.Op{store.Delete(partiesTab, store.Row{"Party ID": p.ID})}, nil
}

func (m *Parties) setStatus(actor access.Actor, id, status string) (*Party, []store.Op, error) {
	if err := require(actor, CurateParties); err != nil {
		return nil, nil, err
	}
	p, err := m.findParty(id)
	if err != nil {
		return nil, nil, err
	}
	if !slices.Contains(PartyStatuses, status) {
		return nil, nil, access.Invalid("status must be one of %s", strings.Join(PartyStatuses, ", "))
	}
	return p, []store.Op{store.Update(partiesTab, store.Row{"Party ID": p.ID}, store.Row{"Status": status})}, nil
}

type celebrationForm struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Title       string `json:"title"`
	Subtitle    string `json:"subtitle"`
	Start       string `json:"start"`
	End         string `json:"end"`
	Location    string `json:"location"`
	Address     string `json:"address"`
	Description string `json:"description"`
	Image       string `json:"image"`
	ButtonText  string `json:"buttonText"`
	ButtonURL   string `json:"buttonUrl"`
	Current     bool   `json:"current"`
	Banner      bool   `json:"banner"`
}

func (m *Parties) saveCelebration(actor access.Actor, form celebrationForm) ([]store.Op, string, error) {
	if err := require(actor, ConfigureParties); err != nil {
		return nil, "", err
	}
	code := strings.TrimSpace(form.Code)
	key := strings.TrimSpace(form.ID)
	adding := key == ""
	if !adding && m.CelebrationByID(key) == nil {
		return nil, "", access.Missing("no such celebration")
	}
	if adding {
		key = id.New(m.taken)
	}
	if other := m.Celebration(code); other != nil && other.ID != key {
		return nil, "", access.Invalid("%q is already a celebration", code)
	}
	row := store.Row{
		"Code": code, "Title": strings.TrimSpace(form.Title), "Subtitle": strings.TrimSpace(form.Subtitle),
		"Start": strings.TrimSpace(form.Start), "End": strings.TrimSpace(form.End), "Location": strings.TrimSpace(form.Location),
		"Address": strings.TrimSpace(form.Address), "Description": strings.TrimSpace(form.Description), "Image": strings.TrimSpace(form.Image),
		"Button Text": strings.TrimSpace(form.ButtonText), "Button URL": strings.TrimSpace(form.ButtonURL), "Current": cells.YesNoCell(form.Current), "Banner": cells.YesNoCell(form.Banner),
	}
	ops := []store.Op{}
	for _, c := range m.Celebrations {
		if c.ID == key {
			continue
		}
		unmark := store.Row{}
		if form.Current && c.Current {
			unmark["Current"] = "No"
		}
		if form.Banner && c.Banner {
			unmark["Banner"] = "No"
		}
		if len(unmark) > 0 {
			ops = append(ops, store.Update(celebrationsTab, store.Row{"Celebration ID": c.ID}, unmark))
		}
	}
	if adding {
		row["Celebration ID"] = key
		ops = append(ops, store.Insert(celebrationsTab, row))
	} else {
		ops = append(ops, store.Update(celebrationsTab, store.Row{"Celebration ID": key}, row))
	}
	return ops, key, nil
}

func (s *Store) deleteCelebration(actor access.Actor, key string) (*Celebration, []store.Op, error) {
	if err := require(actor, ConfigureParties); err != nil {
		return nil, nil, err
	}
	celebration := s.Model().Parties.CelebrationByID(strings.TrimSpace(key))
	if celebration == nil {
		return nil, nil, access.Missing("no such celebration")
	}
	if s.Count(partiesAppName, partiesTab, store.Row{"Celebration": celebration.ID}) > 0 {
		return nil, nil, access.Invalid("parties belong to this celebration; move or remove them first")
	}
	return celebration, []store.Op{store.Delete(celebrationsTab, store.Row{"Celebration ID": celebration.ID})}, nil
}

func (m *Parties) saveCategory(actor access.Actor, key, title string) ([]store.Op, string, error) {
	if err := require(actor, ConfigureParties); err != nil {
		return nil, "", err
	}
	title = strings.TrimSpace(title)
	if err := cells.Title("category", title, maxPartyTitleLength); err != nil {
		return nil, "", access.Invalid("%s", err)
	}
	key = strings.TrimSpace(key)
	adding := key == ""
	if !adding && m.Category(key) == nil {
		return nil, "", access.Missing("no such category")
	}
	if other := m.categoryTitled(title); other != nil && (adding || other.ID != key) {
		return nil, "", access.Invalid("%q is already a category", title)
	}
	if adding {
		key = id.New(m.taken)
		return []store.Op{store.Insert(partyCategoriesTab, store.Row{"Category ID": key, "Title": title})}, key, nil
	}
	return []store.Op{store.Update(partyCategoriesTab, store.Row{"Category ID": key}, store.Row{"Title": title})}, key, nil
}

func (s *Store) deletePartyCategory(actor access.Actor, key string) (*PartyCategory, []store.Op, error) {
	if err := require(actor, ConfigureParties); err != nil {
		return nil, nil, err
	}
	category := s.Model().Parties.Category(strings.TrimSpace(key))
	if category == nil {
		return nil, nil, access.Missing("no such category")
	}
	if s.Count(partiesAppName, partiesTab, store.Row{"Category": category.ID}) > 0 {
		return nil, nil, access.Invalid("parties are filed under this category; move them first")
	}
	return category, []store.Op{store.Delete(partyCategoriesTab, store.Row{"Category ID": category.ID})}, nil
}

func (m *Parties) reorderCategories(actor access.Actor, order []string) ([]store.Op, error) {
	if err := require(actor, ConfigureParties); err != nil {
		return nil, err
	}
	if len(order) != len(m.Categories) {
		return nil, access.Invalid("the order must name every category once")
	}
	named, keys := []string{}, []string{}
	for _, key := range order {
		if m.Category(key) == nil || slices.Contains(named, key) {
			return nil, access.Invalid("the order must name every category once")
		}
		named, keys = append(named, key), append(keys, m.categoryOrder[key])
	}
	placed := store.Order(keys)
	ops := []store.Op{}
	for i, key := range named {
		if placed[i] != keys[i] {
			ops = append(ops, store.Update(partyCategoriesTab, store.Row{"Category ID": key}, store.Row{store.OrderColumn: placed[i]}))
		}
	}
	return ops, nil
}

func (m *Parties) saveSettings(actor access.Actor, s PartiesSettings) ([]store.Op, error) {
	if err := require(actor, ConfigureParties); err != nil {
		return nil, err
	}
	values := map[string]string{PartiesIntroKey: strings.TrimSpace(s.PartiesIntro), TicketNoteKey: strings.TrimSpace(s.TicketNote), HostingOpenKey: cells.YesNoCell(s.HostingOpen)}
	ops := []store.Op{}
	for _, key := range partySettingKeys {
		ops = append(ops, store.Upsert(partySettingsTab, store.Row{"Key": key}, store.Row{"Value": values[key]}))
	}
	return ops, nil
}

func (m *Parties) moveUnlisted(actor access.Actor, directory *Directory, old, to, name string) ([]store.Op, int, error) {
	if err := require(actor, MoveAddresses); err != nil {
		return nil, 0, err
	}
	if directory.Member(directory.Resolve(mail.Normalize(old))) {
		return nil, 0, access.Invalid("their address is the directory's to change")
	}
	return m.moveAddress(actor, old, to, name)
}

func (m *Parties) moveAddress(actor access.Actor, old, to, name string) ([]store.Op, int, error) {
	if err := require(actor, MoveAddresses); err != nil {
		return nil, 0, err
	}
	old, to, name = mail.Normalize(old), mail.Normalize(to), strings.TrimSpace(name)
	if err := checkEmail(old); err != nil {
		return nil, 0, access.Invalid("%s", err)
	}
	if err := checkEmail(to); err != nil {
		return nil, 0, access.Invalid("%s", err)
	}
	if old == to {
		return nil, 0, access.Invalid("that is the address it has already")
	}
	if len(name) > maxGuestNameLength {
		return nil, 0, access.Invalid("the name is too long")
	}
	if f, ok := m.former[old]; ok {
		return nil, 0, access.Invalid("%s has already moved to %s", old, f.New)
	}
	ops := []store.Op{}
	if f, ok := m.former[to]; ok && f.New == old {
		ops = append(ops, store.Delete(formerTab, store.Row{"Old": to}))
	}
	ops = append(ops,
		store.Update(formerTab, store.Row{"New": old}, store.Row{"New": to}),
		store.Insert(formerTab, store.Row{"Old": old, "New": to, "Name": name, "Changed": todayLocal()}),
		store.Update(hostsTab, store.Row{"Email": old}, store.Row{"Email": to}),
	)
	moved := 0
	for _, p := range m.Parties {
		for _, t := range p.Tickets {
			cells := store.Row{}
			if t.Email == old {
				cells["Email"] = to
				if t.Name == "" && name != "" {
					cells["Name"] = name
				}
			}
			if t.Purchaser == old {
				cells["Purchaser"] = to
			}
			if len(cells) > 0 {
				ops = append(ops, store.Update(ticketsTab, store.Row{"Ticket ID": t.ID}, cells))
				moved++
			}
		}
	}
	return ops, moved, nil
}
