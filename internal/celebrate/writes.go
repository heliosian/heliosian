package celebrate

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/config"
	"heliosian/internal/serve"
	"heliosian/internal/store"
	"heliosian/internal/who"
)

func requireAdmin(actor access.Actor) error {
	if !actor.Admin {
		return access.Forbidden("admin access required")
	}
	return nil
}

func isKid(directory *who.Model, email string) bool {
	person := directory.Person(email)
	return person != nil && person.IsStudent && !person.IsParent && !person.IsStaff
}

func owns(t *Ticket, actor access.Actor) bool {
	return actor.Mine(t.Purchaser) || actor.Mine(t.Email)
}

func (m *Model) findParty(id string) (*Party, error) {
	p := m.Party(strings.TrimSpace(id))
	if p == nil {
		return nil, access.Missing("no party with id %q", id)
	}
	return p, nil
}

func (m *Model) findTicket(id string) (*Ticket, *Party, error) {
	t, p := m.TicketByID(strings.TrimSpace(id))
	if t == nil {
		return nil, nil, access.Missing("no such ticket")
	}
	return t, p, nil
}

func current(m *Model, directory *who.Model, email string) string {
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

func invoiceRow(directory *who.Model, p *Party, cells store.Row) store.Row {
	price, _ := ParsePrice(cells["Price"])
	if cells["Status"] != TicketSold || price <= 0 {
		return nil
	}
	return store.Row{
		"Date": today(), "Party Title": p.Title, "Event Code": p.Celebration, "Purchaser Email": cells["Purchaser"],
		"Guest Name": ticketName(directory, cells), "Action": "ADD", "Quantity": "1", "Cost": PriceCell(price),
	}
}

func ticketOps(directory *who.Model, p *Party, added []store.Row) []store.Op {
	ops := []store.Op{}
	for _, cells := range added {
		ops = append(ops, store.Insert(ticketsTab, cells))
		if row := invoiceRow(directory, p, cells); row != nil {
			ops = append(ops, store.Insert(invoicingTab, row))
		}
	}
	return ops
}

func waitlistRequest(p *Party, purchaser string, quantity int, note, actor string) store.Row {
	return store.Row{
		"Ticket ID": serve.ID(8), "Party ID": p.ID, "Email": purchaser, "Name": "", "Purchaser": purchaser,
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

func (m *Model) takeTickets(actor access.Actor, directory *who.Model, order ticketOrder) (taken, error) {
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
	if !editor {
		switch p.Availability(now()) {
		case Past:
			return taken{}, access.Invalid("this party has already happened")
		case Closed:
			return taken{}, access.Invalid("tickets are closed for this party")
		case SoldOut:
			return taken{}, access.Invalid("this party is sold out")
		case Waitlist:
			return taken{}, access.Invalid("this party is full; join the waitlist instead")
		}
	}
	purchaser := config.NormalizeEmail(order.Purchaser)
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
		email, name := config.NormalizeEmail(att.Email), strings.TrimSpace(att.Name)
		if email == "" && name == "" {
			return taken{}, access.Invalid("each ticket needs a person or a guest's name")
		}
		if len(name) > maxNameLength {
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
	out := taken{party: p, purchaser: purchaser, added: []store.Row{}, ops: []store.Op{}}
	price := PriceCell(p.Price)
	if order.Free {
		price = "0"
	}
	for _, rw := range rows {
		switch {
		case editor || remaining < 0:
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
			"Ticket ID": serve.ID(8), "Party ID": p.ID, "Email": rw.email, "Name": rw.name, "Purchaser": rw.purchaser,
			"Status": TicketSold, "Quantity": "1", "Price": price, "Note": strings.TrimSpace(order.Note), "Added By": actor.Email, "Added": stamp(),
		})
	}
	if out.waitlisted > 0 {
		out.added = append(out.added, waitlistRequest(p, purchaser, out.waitlisted, strings.TrimSpace(order.Note), actor.Email))
	}
	if order.Free && order.RaiseCapacity && p.Capacity > 0 && out.sold > 0 {
		out.ops = append(out.ops, store.Update(partiesTab, store.Row{"Party ID": p.ID}, store.Row{"Capacity": countCell(p.Capacity + out.sold)}))
	}
	out.ops = append(out.ops, ticketOps(directory, p, out.added)...)
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

func (m *Model) joinWaitlist(actor access.Actor, directory *who.Model, order waitlistOrder) (joined, error) {
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
	purchaser := config.NormalizeEmail(order.Purchaser)
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
	cells := waitlistRequest(p, purchaser, order.Quantity, note, actor.Email)
	return joined{party: p, purchaser: purchaser, cells: cells, ops: []store.Op{store.Insert(ticketsTab, cells)}}, nil
}

type offer struct {
	TicketID string `json:"ticketId"`
	Quantity int    `json:"quantity"`
}

type offered struct {
	party   *Party
	ticket  *Ticket
	added   []store.Row
	ops     []store.Op
	offered int
	left    int
}

func (m *Model) offerTickets(actor access.Actor, directory *who.Model, o offer) (offered, error) {
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
	holder := directory.Person(directory.Resolve(t.Purchaser))
	holderName := nameOf(directory, t.Purchaser)
	selfTicket := holder != nil && p.Admits(holder)
	for _, other := range p.Tickets {
		if other.Status == TicketSold && other.Email == t.Purchaser {
			selfTicket = false
		}
	}
	added := []store.Row{}
	for i := 0; i < n; i++ {
		cells := store.Row{
			"Ticket ID": serve.ID(8), "Party ID": p.ID, "Purchaser": t.Purchaser, "Status": TicketSold, "Quantity": "1",
			"Price": PriceCell(t.Price), "Note": t.Note, "Added By": actor.Email, "Added": stamp(),
		}
		if i == 0 && selfTicket {
			cells["Email"] = t.Purchaser
		} else {
			cells["Name"] = holderName + "'s guest (to be named)"
		}
		added = append(added, cells)
	}
	match := store.Row{"Ticket ID": t.ID}
	left := t.Quantity - n
	ops := ticketOps(directory, p, added)
	if left > 0 {
		ops = append(ops, store.Update(ticketsTab, match, store.Row{"Quantity": strconv.Itoa(left)}))
	} else {
		ops = append(ops, store.Delete(ticketsTab, match))
	}
	return offered{party: p, ticket: t, added: added, ops: ops, offered: n, left: left}, nil
}

func (m *Model) removeTicket(actor access.Actor, id string) (*Ticket, *Party, []store.Op, error) {
	t, p, err := m.findTicket(id)
	if err != nil {
		return nil, nil, nil, err
	}
	editor := p.Edits(actor)
	if !editor && t.Status == TicketSold {
		return nil, nil, nil, access.Forbidden("tickets can't be given back; ask the party's host, or resell it to another family")
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

func (m *Model) editTicket(actor access.Actor, edit ticketEdit) (*Ticket, *Party, []store.Op, []string, error) {
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

func (m *Model) reassignTicket(actor access.Actor, directory *who.Model, re reassignment) (*Ticket, *Party, []store.Op, string, error) {
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
	email, name := config.NormalizeEmail(re.Email), strings.TrimSpace(re.Name)
	if email == "" && name == "" {
		return nil, nil, nil, "", access.Invalid("pick someone, or name a guest")
	}
	if len(name) > maxNameLength {
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

type savedParty struct {
	id     string
	title  string
	status string
	adding bool
	ops    []store.Op
}

func (m *Model) saveParty(actor access.Actor, body partyBody) (savedParty, error) {
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
	case adding && actor.Admin:
		if status == "" {
			status = StatusOpen
		}
	case adding:
		if !m.Settings.HostingOpen {
			return savedParty{}, access.Forbidden("hosting is closed for now - an admin can open it in Settings")
		}
		status, category = StatusPending, ""
		if c := m.Current(); c != nil {
			celebration = c.Code
		}
	case actor.Admin:
		if status == "" {
			status = was.Status
		}
	default:
		status, celebration, category = was.Status, was.Celebration, was.Category
	}
	if !slices.Contains(Statuses, status) {
		return savedParty{}, access.Invalid("status must be one of %s", strings.Join(Statuses, ", "))
	}
	if celebration == "" {
		if c := m.Current(); c != nil {
			celebration = c.Code
		}
	}
	if body.Price < 0 || body.Capacity < 0 || body.Minimum < 0 {
		return savedParty{}, access.Invalid("price, capacity, and minimum can't be negative")
	}
	if !body.Adults && !body.Students {
		return savedParty{}, access.Invalid("let adults, students, or both hold a ticket")
	}
	hosts := config.NormalizeEmails(body.HostEmails)
	if adding && !actor.Admin && !slices.Contains(hosts, actor.Email) {
		hosts = append(hosts, actor.Email)
	}
	for _, h := range hosts {
		if err := checkEmail(h); err != nil {
			return savedParty{}, access.Invalid("%s", err)
		}
	}
	if !adding && !actor.Admin && !slices.Contains(hosts, actor.Email) {
		return savedParty{}, access.Invalid("you can't remove yourself as a host; ask another host or an admin")
	}
	id := strings.TrimSpace(body.ID)
	if adding {
		id = serve.ID(8)
	}
	pretty := cells.NormalizePretty(body.PrettyID)
	if cells.CheckPretty(pretty) != nil {
		return savedParty{}, access.Invalid("the friendly address can be only lower-case letters, digits and hyphens, at most %d", cells.MaxPrettyLength)
	}
	if other := m.ByPretty(pretty); pretty != "" && other != nil && other.ID != id {
		return savedParty{}, access.Invalid("%q is already the address of %s", pretty, other.Title)
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
		row["Party ID"] = id
		row["Added By"] = actor.Email
		row["Added"] = today()
		ops = append(ops, store.Insert(partiesTab, row))
	} else {
		before = was.HostEmails
		ops = append(ops, store.Update(partiesTab, store.Row{"Party ID": id}, row))
	}
	for _, h := range before {
		if !slices.Contains(hosts, h) {
			ops = append(ops, store.Delete(hostsTab, store.Row{"Party ID": id, "Email": h}))
		}
	}
	for _, h := range hosts {
		if !slices.Contains(before, h) {
			ops = append(ops, store.Insert(hostsTab, store.Row{"Party ID": id, "Email": h}))
		}
	}
	return savedParty{id: id, title: row["Title"], status: status, adding: adding, ops: ops}, nil
}

func (m *Model) deleteParty(actor access.Actor, id string) (*Party, []store.Op, error) {
	if err := requireAdmin(actor); err != nil {
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

type partyFlags struct {
	ID           string `json:"id"`
	TicketsOpen  bool   `json:"ticketsOpen"`
	Waitlist     bool   `json:"waitlist"`
	Adults       bool   `json:"adults"`
	Students     bool   `json:"students"`
	DropOff      bool   `json:"dropOff"`
	ParentTicket bool   `json:"parentTicket"`
}

func (m *Model) setFlags(actor access.Actor, flags partyFlags) (*Party, []store.Op, error) {
	p, err := m.findParty(flags.ID)
	if err != nil {
		return nil, nil, err
	}
	if !p.Edits(actor) {
		return nil, nil, access.Forbidden("only a host or admin can change this")
	}
	if !flags.Adults && !flags.Students {
		return nil, nil, access.Invalid("let adults, students, or both hold a ticket")
	}
	row := store.Row{
		"Tickets": ticketsCell(flags.TicketsOpen), "Waitlist": cells.YesNoCell(flags.Waitlist),
		"Adults": cells.YesNoCell(flags.Adults), "Students": cells.YesNoCell(flags.Students),
		"Drop-Off": cells.YesNoCell(flags.DropOff), "Parent Ticket Required": cells.YesNoCell(flags.ParentTicket),
	}
	return p, []store.Op{store.Update(partiesTab, store.Row{"Party ID": p.ID}, row)}, nil
}

func (m *Model) setStatus(actor access.Actor, id, status string) (*Party, []store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, nil, err
	}
	p, err := m.findParty(id)
	if err != nil {
		return nil, nil, err
	}
	if !slices.Contains(Statuses, status) {
		return nil, nil, access.Invalid("status must be one of %s", strings.Join(Statuses, ", "))
	}
	return p, []store.Op{store.Update(partiesTab, store.Row{"Party ID": p.ID}, store.Row{"Status": status})}, nil
}

type celebrationForm struct {
	Original    string `json:"original"`
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

func (m *Model) saveCelebration(actor access.Actor, form celebrationForm) ([]store.Op, bool, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, false, err
	}
	code := strings.TrimSpace(form.Code)
	adding := strings.TrimSpace(form.Original) == ""
	if !adding && m.Celebration(form.Original) == nil {
		return nil, false, access.Missing("no such celebration")
	}
	renamed := !adding && form.Original != code
	if (adding || renamed) && m.Celebration(code) != nil {
		return nil, false, access.Invalid("%q is already a celebration", code)
	}
	row := store.Row{
		"Code": code, "Title": strings.TrimSpace(form.Title), "Subtitle": strings.TrimSpace(form.Subtitle),
		"Start": strings.TrimSpace(form.Start), "End": strings.TrimSpace(form.End), "Location": strings.TrimSpace(form.Location),
		"Address": strings.TrimSpace(form.Address), "Description": strings.TrimSpace(form.Description), "Image": strings.TrimSpace(form.Image),
		"Button Text": strings.TrimSpace(form.ButtonText), "Button URL": strings.TrimSpace(form.ButtonURL), "Current": cells.YesNoCell(form.Current), "Banner": cells.YesNoCell(form.Banner),
	}
	ops := []store.Op{}
	for _, c := range m.Celebrations {
		if c.Code == form.Original {
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
			ops = append(ops, store.Update(celebrationsTab, store.Row{"Code": c.Code}, unmark))
		}
	}
	if adding {
		ops = append(ops, store.Insert(celebrationsTab, row))
	} else {
		ops = append(ops, store.Update(celebrationsTab, store.Row{"Code": form.Original}, row))
	}
	return ops, adding, nil
}

func (c *Cache) deleteCelebration(actor access.Actor, code string) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	if c.Model().Celebration(code) == nil {
		return nil, access.Missing("no such celebration")
	}
	if c.Count(partiesTab, store.Row{"Celebration": code}) > 0 {
		return nil, access.Invalid("parties belong to this celebration; move or remove them first")
	}
	return []store.Op{store.Delete(celebrationsTab, store.Row{"Code": code})}, nil
}

func (m *Model) saveCategory(actor access.Actor, original, title string) ([]store.Op, bool, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, false, err
	}
	title = strings.TrimSpace(title)
	if err := cells.Title("category", title, maxTitleLength); err != nil {
		return nil, false, access.Invalid("%s", err)
	}
	adding := original == ""
	if !adding && !slices.Contains(m.Categories, original) {
		return nil, false, access.Missing("no such category")
	}
	renamed := !adding && original != title
	if (adding || renamed) && slices.Contains(m.Categories, title) {
		return nil, false, access.Invalid("%q is already a category", title)
	}
	if adding {
		return []store.Op{store.Insert(categoriesTab, store.Row{"Title": title})}, true, nil
	}
	return []store.Op{store.Update(categoriesTab, store.Row{"Title": original}, store.Row{"Title": title})}, false, nil
}

func (c *Cache) deleteCategory(actor access.Actor, title string) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	if !slices.Contains(c.Model().Categories, title) {
		return nil, access.Missing("no such category")
	}
	if c.Count(partiesTab, store.Row{"Category": title}) > 0 {
		return nil, access.Invalid("parties are filed under this category; move them first")
	}
	return []store.Op{store.Delete(categoriesTab, store.Row{"Title": title})}, nil
}

func (m *Model) reorderCategories(actor access.Actor, order []string) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	if len(order) != len(m.Categories) {
		return nil, access.Invalid("the order must name every category once")
	}
	titles, keys := []string{}, []string{}
	for _, title := range order {
		if !slices.Contains(m.Categories, title) || slices.Contains(titles, title) {
			return nil, access.Invalid("the order must name every category once")
		}
		titles, keys = append(titles, title), append(keys, m.categoryOrder[title])
	}
	placed := store.Order(keys)
	ops := []store.Op{}
	for i, title := range titles {
		if placed[i] != keys[i] {
			ops = append(ops, store.Update(categoriesTab, store.Row{"Title": title}, store.Row{store.OrderColumn: placed[i]}))
		}
	}
	return ops, nil
}

func (m *Model) saveSettings(actor access.Actor, s Settings) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	values := map[string]string{PartiesIntroKey: strings.TrimSpace(s.PartiesIntro), TicketNoteKey: strings.TrimSpace(s.TicketNote), HostingOpenKey: cells.YesNoCell(s.HostingOpen)}
	ops := []store.Op{}
	for _, key := range settingKeys {
		ops = append(ops, store.Upsert(settingsTab, store.Row{"Key": key}, store.Row{"Value": values[key]}))
	}
	return ops, nil
}

func (m *Model) setAdmins(actor access.Actor, superAdmins, requested []string) ([]store.Op, []string, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, nil, err
	}
	super := map[string]bool{}
	for _, e := range superAdmins {
		super[e] = true
	}
	admins := []string{}
	for _, e := range config.NormalizeEmails(requested) {
		if !super[e] {
			admins = append(admins, e)
		}
	}
	ops := []store.Op{}
	for _, e := range m.admins {
		if !slices.Contains(admins, e) {
			ops = append(ops, store.Delete(adminsTab, store.Row{"Email": e}))
		}
	}
	for _, e := range admins {
		if !slices.Contains(m.admins, e) {
			ops = append(ops, store.Insert(adminsTab, store.Row{"Email": e}))
		}
	}
	return ops, admins, nil
}

func (m *Model) moveUnlisted(actor access.Actor, directory *who.Model, old, to, name string) ([]store.Op, int, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, 0, err
	}
	if directory.Member(directory.Resolve(config.NormalizeEmail(old))) {
		return nil, 0, access.Invalid("their address is the directory's to change")
	}
	return m.moveAddress(actor, old, to, name)
}

func (m *Model) moveAddress(actor access.Actor, old, to, name string) ([]store.Op, int, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, 0, err
	}
	old, to, name = config.NormalizeEmail(old), config.NormalizeEmail(to), strings.TrimSpace(name)
	if err := checkEmail(old); err != nil {
		return nil, 0, access.Invalid("%s", err)
	}
	if err := checkEmail(to); err != nil {
		return nil, 0, access.Invalid("%s", err)
	}
	if old == to {
		return nil, 0, access.Invalid("that is the address it has already")
	}
	if len(name) > maxNameLength {
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
		store.Insert(formerTab, store.Row{"Old": old, "New": to, "Name": name, "Changed": today()}),
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
