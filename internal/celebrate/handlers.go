package celebrate

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"heliosian/internal/theme"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/imagesearch"
	"heliosian/internal/logging"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
)

const (
	imageFolder  = "party-images"
	maxImageSize = 8 << 20
	shell        = "web/celebrate/index.html"
	// maxTicketsPerPurchase bounds one form submission; a family buys a
	// handful, a host adding a whole guest list does it in a few goes.
	maxTicketsPerPurchase = 20
)

var pages = []string{
	"/{$}", "/parties/{id}", "/p/{pretty}", "/celebrations/{code}", "/my", "/my/{email}", "/hosting", "/admin",
}

// local is the school's clock: the sheet's dates are wall-clock there, and a
// ticket taken late one evening is stamped that evening.
var local = mustLocation("America/Los_Angeles")

func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		logging.Fatal("load time zone", "name", name, "error", err)
	}
	return loc
}

// ImageSearch is the picture search shared with HCA-Team and Heliosian.
type ImageSearch = imagesearch.Search

type app struct {
	cache       *Cache
	writer      data.Writer
	queue       Enqueuer
	store       *blob.Store
	directory   Directory
	superAdmins func() []string
	search      ImageSearch
	// mailer sends the site's email, from the address in from; nil sends
	// nothing.
	mailer mail.Sender
	from   string
}

// Register wires the app: one shell for every page, the model, and the
// writes. Every route already sits behind sign-in. A nil mailer sends
// nothing.
func Register(mux *http.ServeMux, cache *Cache, writer data.Writer, queue Enqueuer, store *blob.Store, directory Directory, superAdmins func() []string, search ImageSearch, mailer mail.Sender, from string) {
	if search.UserAgent == "" {
		search.UserAgent = "Helios Celebrate image search (+https://celebrate.heliosian.com)"
	}
	a := app{cache: cache, writer: writer, queue: queue, store: store, directory: directory, superAdmins: superAdmins, search: search, mailer: mailer, from: from}
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.ready(a.page))
	}
	mux.HandleFunc("GET /api/celebrate/model", a.ready(a.model))
	mux.HandleFunc("GET /api/celebrate/people", a.ready(a.people))
	mux.HandleFunc("GET /api/celebrate/images/search", a.ready(a.search.ServeSearch))
	mux.HandleFunc("POST /api/celebrate/images/import", a.ready(a.importImage))
	mux.HandleFunc("POST /api/celebrate/image", a.ready(a.uploadImage))
	// Public, past sign-in (auth.Public): the image a chat app shows for a
	// link - one party's, or the site's own.
	mux.HandleFunc("GET /share/upcoming.png", a.ready(a.shareUpcoming))
	mux.HandleFunc("GET /share/{id}", a.ready(a.shareCard))
	mux.HandleFunc("POST /api/celebrate/tickets", a.ready(a.buyTickets))
	mux.HandleFunc("POST /api/celebrate/waitlist", a.ready(a.joinWaitlist))
	mux.HandleFunc("POST /api/celebrate/waitlist/offer", a.ready(a.offerTickets))
	mux.HandleFunc("POST /api/celebrate/ticket", a.ready(a.editTicket))
	mux.HandleFunc("DELETE /api/celebrate/ticket", a.ready(a.removeTicket))
	mux.HandleFunc("POST /api/celebrate/ticket/reassign", a.ready(a.reassignTicket))
	mux.HandleFunc("POST /api/celebrate/party", a.ready(a.saveParty))
	mux.HandleFunc("DELETE /api/celebrate/party", a.ready(a.deleteParty))
	mux.HandleFunc("POST /api/celebrate/party/flags", a.ready(a.setFlags))
	mux.HandleFunc("POST /api/celebrate/party/status", a.ready(a.setStatus))
	mux.HandleFunc("POST /api/celebrate/celebration", a.ready(a.saveCelebration))
	mux.HandleFunc("DELETE /api/celebrate/celebration", a.ready(a.deleteCelebration))
	mux.HandleFunc("POST /api/celebrate/category", a.ready(a.saveCategory))
	mux.HandleFunc("DELETE /api/celebrate/category", a.ready(a.deleteCategory))
	mux.HandleFunc("POST /api/celebrate/categories/order", a.ready(a.reorderCategories))
	mux.HandleFunc("POST /api/celebrate/settings", a.ready(a.saveSettings))
	mux.HandleFunc("POST /api/celebrate/theme", a.ready(a.saveTheme))
	mux.HandleFunc("POST /api/celebrate/theme/picture", a.ready(theme.Upload(store, func(w http.ResponseWriter, r *http.Request) bool {
		_, ok := a.requireAdmin(w, r)
		return ok
	})))
	mux.HandleFunc("GET /api/celebrate/invoices.csv", a.ready(a.invoicesCSV))
	mux.HandleFunc("GET /api/admin/state", a.ready(a.adminState))
	mux.HandleFunc("POST /api/admin/admins", a.ready(a.setAdmins))
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

// ready holds every route until the sheet has loaded once. Before then the
// app answers 503 with the reason, so a sheet that needs fixing says what is
// wrong instead of taking the server down with the directory on it.
func (a app) ready(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cache.Model() == nil {
			reason := "the Celebrate sheet has not loaded yet"
			if err := a.cache.Err(); err != nil {
				reason = err.Error()
			}
			http.Error(w, "Helios Celebrate cannot load its data: "+reason, http.StatusServiceUnavailable)
			return
		}
		next(w, r)
	}
}

// who is the signed-in person as the app keys them: the address Google
// vouched for, resolved through the directory's aliases.
func (a app) who(r *http.Request) (string, bool) {
	email := a.directory.Resolve(strings.ToLower(auth.Email(r)))
	return email, a.cache.IsAdmin(email)
}

func (a app) requireAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	email, admin := a.who(r)
	if !admin {
		http.Error(w, "admin access required", http.StatusForbidden)
		return "", false
	}
	return email, true
}

var now = func() time.Time {
	return time.Now().In(local)
}

func today() string {
	return now().Format(DateFormat)
}

func stamp() string {
	return now().Format(DateTimeFormat)
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	email, admin := a.who(r)
	view := Render(a.cache.Model(), a.directory, email, admin, now())
	view.ImageSearch = true
	view.ImageSources = a.search.Sources()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode celebrate model", "error", err)
	}
}

// people is the directory for the pickers. Anyone signed in may see it - it
// is the same list the school directory shows every member.
func (a app) people(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(a.directory.People()); err != nil {
		slog.ErrorContext(r.Context(), "celebrate: encode people", "error", err)
	}
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(into); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return false
	}
	return true
}

// commit rebuilds the model over the proposed tables first, so a change the
// sheet rules reject never reaches the sheet, then applies it in memory at
// once and queues the writes behind every earlier one. The page reads the
// memory, so the change shows the moment the request returns, however long
// the sheet takes.
func (a app) commit(ctx context.Context, w http.ResponseWriter, tables *Tables, flush func() error) bool {
	model, err := BuildModel(tables, a.cache.images)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	a.cache.set(tables, model)
	a.queue.Add(func() {
		if err := flush(); err != nil {
			slog.ErrorContext(ctx, "celebrate write", "error", err)
		}
	})
	return true
}

func (a app) logChange(actor, action, kind string, cells map[string]string) error {
	return a.writer.AppendCells(appName, changeLogTab, map[string]string{
		"Timestamp": time.Now().Format(time.RFC3339), "Actor": actor, "Action": action, "Kind": kind,
		"Celebration": cells["Celebration"], "Party": cells["Party"], "Title": cells["Title"], "Email": cells["Email"], "Details": cells["Details"],
	})
}

// logInvoice writes a sold ticket to the INVOICING ledger for accounting:
// the date, the party and its celebration's code, who is billed, who the
// ticket is for, ADD, one, and the cost. A free ticket bills nobody and is
// left out.
func (a app) invoiceRow(p *Party, cells map[string]string) map[string]string {
	price, _ := ParsePrice(cells["Price"])
	if cells["Status"] != TicketSold || price <= 0 {
		return nil
	}
	name := cells["Name"]
	if cells["Email"] != "" {
		name = a.nameOf(cells["Email"])
	}
	return map[string]string{
		"Date": today(), "Party Title": p.Title, "Event Code": p.Celebration, "Purchaser Email": cells["Purchaser"],
		"Guest Name": name, "Action": "ADD", "Quantity": "1", "Cost": PriceCell(price),
	}
}

func (a app) logInvoice(p *Party, cells map[string]string) error {
	row := a.invoiceRow(p, cells)
	if row == nil {
		return nil
	}
	return a.writer.AppendCells(appName, invoicingTab, row)
}

func cleanEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// NewID mints a key for a row the app creates. Eight characters from a
// 31-symbol alphabet is enough that collisions are not a practical concern
// at this scale, and the load refuses a duplicate anyway.
func NewID() string {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	out := make([]byte, len(raw))
	for i, b := range raw {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out)
}

func (a app) findParty(w http.ResponseWriter, id string) (*Party, bool) {
	p := a.cache.Model().Party(strings.TrimSpace(id))
	if p == nil {
		http.Error(w, fmt.Sprintf("no party with id %q", id), http.StatusNotFound)
		return nil, false
	}
	return p, true
}

// editor says whether the actor runs the party: an admin, or one of its hosts.
func (a app) editor(p *Party, actor string, admin bool) bool {
	return admin || p.Hosted(actor)
}

func (a app) nameOf(email string) string {
	if p, ok := a.directory.Person(a.directory.Resolve(email)); ok {
		return p.Name
	}
	return displayName(email)
}

// audienceWords is the party's audience rule in words, for a refusal.
func audienceWords(p *Party) string {
	words := []string{}
	if p.Adults {
		words = append(words, "adults")
	}
	if p.Students {
		words = append(words, "students")
	}
	return strings.Join(words, " and ")
}

// buyTickets takes tickets for one or more people at once: the viewer, their
// household, or a guest by name - billed to an adult of the household. Each
// ticket is sold while there is room, then goes to the waitlist when the
// party takes one; a full party without a waitlist refuses. Whoever runs the
// party may add anyone, billed to anyone, and is never refused for room.
func (a app) buyTickets(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		PartyID   string `json:"partyId"`
		Purchaser string `json:"purchaser"`
		Note      string `json:"note"`
		// Free is a host's gift: the tickets are minted at $0, so nothing
		// lands on an invoice. Only whoever runs the party may give one.
		// RaiseCapacity grows the cap by as many, so the gift takes no
		// paid place.
		Free          bool `json:"free"`
		RaiseCapacity bool `json:"raiseCapacity"`
		Attendees     []struct {
			Email string `json:"email"`
			Name  string `json:"name"`
		} `json:"attendees"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, ok := a.findParty(w, body.PartyID)
	if !ok {
		return
	}
	editor := a.editor(p, actor, admin)
	if !editor && a.kid(actor) {
		http.Error(w, "tickets are taken by a parent - ask yours to sign in", http.StatusForbidden)
		return
	}
	if body.Free && !editor {
		http.Error(w, "only the hosts can give a free ticket", http.StatusForbidden)
		return
	}
	if len(body.Attendees) == 0 {
		http.Error(w, "pick at least one person", http.StatusBadRequest)
		return
	}
	if len(body.Attendees) > maxTicketsPerPurchase {
		http.Error(w, fmt.Sprintf("at most %d tickets at a time", maxTicketsPerPurchase), http.StatusBadRequest)
		return
	}
	if err := checkText("note", body.Note); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	availability := p.Availability(now())
	if !editor {
		switch availability {
		case Past:
			http.Error(w, "this party has already happened", http.StatusBadRequest)
			return
		case Closed:
			http.Error(w, "tickets are closed for this party", http.StatusBadRequest)
			return
		case SoldOut:
			http.Error(w, "this party is sold out", http.StatusBadRequest)
			return
		case Waitlist:
			http.Error(w, "this party is full; join the waitlist instead", http.StatusBadRequest)
			return
		}
	}
	// Whoever is billed is an adult of the buyer's own family - hosts and
	// admins included: nobody bills a stranger. When a host adds someone
	// outside their family, that ticket is billed to the person themselves,
	// or a student's parent.
	purchaser := cleanEmail(body.Purchaser)
	if purchaser == "" {
		purchaser = actor
	}
	if err := checkEmail(purchaser); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// A host's gift may name whose guest the holder is: an adult in the
	// directory, who holds the ticket for them - it sits with their family
	// to pass on, and the note goes to them. Nothing is billed, so the
	// family rule does not apply.
	gift := body.Free && purchaser != actor
	if gift {
		if host, known := a.directory.Person(purchaser); !known || host.IsStudent {
			http.Error(w, "a guest's host is an adult in the directory", http.StatusBadRequest)
			return
		}
	} else if !slices.Contains(Billable(a.directory, actor), purchaser) {
		http.Error(w, "tickets are billed to you or another adult in your family", http.StatusForbidden)
		return
	}
	type row struct {
		email, name, purchaser string
	}
	rows := []row{}
	seen := map[string]bool{}
	for _, att := range body.Attendees {
		email, name := cleanEmail(att.Email), strings.TrimSpace(att.Name)
		if email == "" && name == "" {
			http.Error(w, "each ticket needs a person or a guest's name", http.StatusBadRequest)
			return
		}
		if len(name) > maxNameLength {
			http.Error(w, "a guest's name is too long", http.StatusBadRequest)
			return
		}
		if email == "" {
			rows = append(rows, row{"", name, purchaser})
			continue
		}
		if err := checkEmail(email); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		email = a.directory.Resolve(email)
		if seen[email] {
			continue
		}
		seen[email] = true
		person, known := a.directory.Person(email)
		// A name with an address the directory does not know is a guest with
		// a way to reach them: anyone may bring one, billed to their family.
		if !known && name != "" {
			rows = append(rows, row{email, name, purchaser})
			continue
		}
		bill := purchaser
		if !InHousehold(a.directory, actor, email) && !gift {
			if !editor {
				http.Error(w, "you can take tickets for yourself and your family; a friend goes in as a guest by name", http.StatusForbidden)
				return
			}
			// Someone outside the host's family pays their own way: the
			// person, or for a student the first parent the directory lists.
			switch {
			case known && person.IsStudent && len(person.ParentEmails) > 0:
				bill = a.directory.Resolve(person.ParentEmails[0])
			case known && person.IsStudent:
				http.Error(w, fmt.Sprintf("%s has no parent on file to bill", person.Name), http.StatusBadRequest)
				return
			default:
				bill = email
			}
		}
		if known && !p.Admits(person, known) {
			http.Error(w, fmt.Sprintf("%s can't hold a ticket: this party is for %s", person.Name, audienceWords(p)), http.StatusBadRequest)
			return
		}
		for _, t := range p.Tickets {
			if t.Email == email {
				if t.Status == TicketSold {
					http.Error(w, fmt.Sprintf("%s already has a ticket", a.nameOf(email)), http.StatusBadRequest)
					return
				}
			}
		}
		rows = append(rows, row{email, "", bill})
	}
	// Everyone gets a ticket while there is room. Once the room runs out the
	// rest of the family go on the waitlist as one request for that many
	// tickets - the names are theirs to give once a host offers places - or
	// the whole purchase is refused when the party takes no waitlist.
	remaining := p.Remaining()
	sold, waitlisted := 0, 0
	price := PriceCell(p.Price)
	if body.Free {
		price = "0"
	}
	tables := a.cache.Tables()
	added := []map[string]string{}
	for _, rw := range rows {
		switch {
		case editor || remaining < 0:
		case remaining > 0:
			remaining--
		case p.Waitlist:
			waitlisted++
			continue
		default:
			http.Error(w, fmt.Sprintf("only %d %s left", p.Remaining(), plural(p.Remaining(), "ticket")), http.StatusBadRequest)
			return
		}
		sold++
		cells := map[string]string{
			"Ticket ID": NewID(), "Party ID": p.ID, "Email": rw.email, "Name": rw.name, "Purchaser": rw.purchaser,
			"Status": TicketSold, "Quantity": "1", "Price": price, "Note": strings.TrimSpace(body.Note), "Added By": actor, "Added": stamp(),
		}
		added = append(added, cells)
		tables = tables.with(ticketsTab, nil, cells)
	}
	if waitlisted > 0 {
		cells := waitlistRequest(p, purchaser, waitlisted, strings.TrimSpace(body.Note), actor)
		added = append(added, cells)
		tables = tables.with(ticketsTab, nil, cells)
	}
	for _, cells := range added {
		if row := a.invoiceRow(p, cells); row != nil {
			tables = tables.with(invoicingTab, nil, row)
		}
	}
	capacity := map[string]string{}
	if body.Free && body.RaiseCapacity && p.Capacity > 0 && sold > 0 {
		capacity["Capacity"] = countCell(p.Capacity + sold)
		tables = tables.with(partiesTab, map[string]string{"Party ID": p.ID}, capacity)
	}
	if !a.commit(r.Context(), w, tables, func() error {
		if len(capacity) > 0 {
			if err := a.writer.Set(appName, partiesTab, map[string]string{"Party ID": p.ID}, capacity); err != nil {
				return err
			}
			if err := a.logChange(actor, "edit", "party", map[string]string{"Celebration": p.Celebration, "Party": p.ID, "Title": p.Title, "Details": "Capacity " + capacity["Capacity"] + " (free ticket)"}); err != nil {
				return err
			}
		}
		for _, cells := range added {
			if err := a.writer.AppendCells(appName, ticketsTab, cells); err != nil {
				return err
			}
			if err := a.logInvoice(p, cells); err != nil {
				return err
			}
			who := cells["Email"]
			if who == "" {
				who = cells["Name"]
			}
			if err := a.logChange(actor, "add", "ticket", map[string]string{
				"Celebration": p.Celebration, "Party": p.ID, "Title": p.Title, "Email": who,
				"Details": fmt.Sprintf("%s ×%s, billed to %s, $%s", cells["Status"], cells["Quantity"], cells["Purchaser"], cells["Price"]),
			}); err != nil {
				return err
			}
		}
		return nil
	}) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: tickets taken", "actor", actor, "party", p.Title, "purchaser", purchaser, "sold", sold, "waitlisted", waitlisted)
	// One note per family billed: the buyer's own, and each family a host
	// added someone from.
	byPurchaser := map[string][]map[string]string{}
	order := []string{}
	for _, cells := range added {
		if _, seen := byPurchaser[cells["Purchaser"]]; !seen {
			order = append(order, cells["Purchaser"])
		}
		byPurchaser[cells["Purchaser"]] = append(byPurchaser[cells["Purchaser"]], cells)
	}
	for _, who := range order {
		a.mailTickets(r, p, who, byPurchaser[who], actor)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"sold": sold, "waitlisted": waitlisted})
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// waitlistRequest is the row for a family asking for tickets when places
// open up: held by whoever asked, for that many.
func waitlistRequest(p *Party, purchaser string, quantity int, note, actor string) map[string]string {
	return map[string]string{
		"Ticket ID": NewID(), "Party ID": p.ID, "Email": purchaser, "Name": "", "Purchaser": purchaser,
		"Status": TicketWaitlist, "Quantity": strconv.Itoa(quantity), "Price": PriceCell(p.Price), "Note": note, "Added By": actor, "Added": stamp(),
	}
}

// joinWaitlist puts a family on a full party's waitlist: how many tickets
// they want, and a note. It is a request, not a purchase - nothing is billed
// until a host offers the places - so it is for the family itself, billed
// to an adult of it. A second request from the same family changes the
// first rather than joining the line again.
func (a app) joinWaitlist(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		PartyID   string `json:"partyId"`
		Purchaser string `json:"purchaser"`
		Quantity  int    `json:"quantity"`
		Note      string `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, ok := a.findParty(w, body.PartyID)
	if !ok {
		return
	}
	editor := a.editor(p, actor, admin)
	if !editor && p.Availability(now()) != Waitlist {
		http.Error(w, "this party is not taking a waitlist right now", http.StatusBadRequest)
		return
	}
	if body.Quantity < 1 || body.Quantity > maxTicketsPerPurchase {
		http.Error(w, fmt.Sprintf("ask for between 1 and %d tickets", maxTicketsPerPurchase), http.StatusBadRequest)
		return
	}
	if err := checkText("note", body.Note); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	purchaser := cleanEmail(body.Purchaser)
	if purchaser == "" {
		purchaser = actor
	}
	if a.kid(actor) {
		http.Error(w, "the waitlist is joined by a parent - ask yours to sign in", http.StatusForbidden)
		return
	}
	if !slices.Contains(Billable(a.directory, actor), purchaser) {
		http.Error(w, "the waitlist is for your own family, billed to an adult in it", http.StatusForbidden)
		return
	}
	tables := a.cache.Tables()
	for _, t := range p.Tickets {
		if t.Status == TicketWaitlist && t.Purchaser == purchaser {
			// Already in line: the request changes, the place in line stays.
			match := map[string]string{"Ticket ID": t.ID}
			cells := map[string]string{"Quantity": strconv.Itoa(body.Quantity), "Note": strings.TrimSpace(body.Note)}
			if !a.commit(r.Context(), w, tables.with(ticketsTab, match, cells), func() error {
				if err := a.writer.Set(appName, ticketsTab, match, cells); err != nil {
					return err
				}
				return a.logChange(actor, "edit", "waitlist", map[string]string{"Celebration": p.Celebration, "Party": p.ID, "Title": p.Title, "Email": purchaser, "Details": fmt.Sprintf("×%d", body.Quantity)})
			}) {
				return
			}
			slog.InfoContext(r.Context(), "celebrate: waitlist request changed", "actor", actor, "party", p.Title, "purchaser", purchaser, "quantity", body.Quantity)
			// A changed request is told the same way a new one is: the family
			// hears where they stand, the hosts hear what is wanted now.
			full := map[string]string{"Email": t.Email, "Purchaser": t.Purchaser, "Status": TicketWaitlist, "Quantity": cells["Quantity"], "Note": cells["Note"]}
			a.mailTickets(r, p, purchaser, []map[string]string{full}, actor)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]int{"sold": 0, "waitlisted": body.Quantity})
			return
		}
	}
	cells := waitlistRequest(p, purchaser, body.Quantity, strings.TrimSpace(body.Note), actor)
	if !a.commit(r.Context(), w, tables.with(ticketsTab, nil, cells), func() error {
		if err := a.writer.AppendCells(appName, ticketsTab, cells); err != nil {
			return err
		}
		return a.logChange(actor, "add", "waitlist", map[string]string{"Celebration": p.Celebration, "Party": p.ID, "Title": p.Title, "Email": purchaser, "Details": fmt.Sprintf("×%d", body.Quantity)})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: joined waitlist", "actor", actor, "party", p.Title, "purchaser", purchaser, "quantity", body.Quantity)
	a.mailTickets(r, p, purchaser, []map[string]string{cells}, actor)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"sold": 0, "waitlisted": body.Quantity})
}

// offerTickets is a host's answer to a waitlist request: the family gets
// that many tickets (or fewer, when the host says how many) at the price
// they were asked for - the first for whoever asked, when the party admits
// them, the rest as guests of theirs to be named with Reassign - and the
// request comes off the list, or shrinks by what was offered.
func (a app) offerTickets(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		TicketID string `json:"ticketId"`
		Quantity int    `json:"quantity"`
	}
	if !decode(w, r, &body) {
		return
	}
	t, p, ok := a.findTicket(w, body.TicketID)
	if !ok {
		return
	}
	if !a.editor(p, actor, admin) {
		http.Error(w, "only a host or admin can offer tickets", http.StatusForbidden)
		return
	}
	if t.Status != TicketWaitlist {
		http.Error(w, "this is not a waitlist request", http.StatusBadRequest)
		return
	}
	n := body.Quantity
	if n <= 0 || n > t.Quantity {
		n = t.Quantity
	}
	holder, known := a.directory.Person(a.directory.Resolve(t.Purchaser))
	holderName := a.nameOf(t.Purchaser)
	// Whoever asked holds a ticket themselves only if the party admits them
	// and they have none already.
	selfTicket := known && p.Admits(holder, true)
	for _, other := range p.Tickets {
		if other.Status == TicketSold && other.Email == t.Purchaser {
			selfTicket = false
		}
	}
	tables := a.cache.Tables()
	added := []map[string]string{}
	for i := 0; i < n; i++ {
		cells := map[string]string{
			"Ticket ID": NewID(), "Party ID": p.ID, "Purchaser": t.Purchaser, "Status": TicketSold, "Quantity": "1",
			"Price": PriceCell(t.Price), "Note": t.Note, "Added By": actor, "Added": stamp(),
		}
		if i == 0 && selfTicket {
			cells["Email"] = t.Purchaser
		} else {
			cells["Name"] = holderName + "'s guest (to be named)"
		}
		added = append(added, cells)
		tables = tables.with(ticketsTab, nil, cells)
	}
	match := map[string]string{"Ticket ID": t.ID}
	left := t.Quantity - n
	if left > 0 {
		tables = tables.with(ticketsTab, match, map[string]string{"Quantity": strconv.Itoa(left)})
	} else {
		tables = tables.without(ticketsTab, match)
	}
	for _, cells := range added {
		if row := a.invoiceRow(p, cells); row != nil {
			tables = tables.with(invoicingTab, nil, row)
		}
	}
	if !a.commit(r.Context(), w, tables, func() error {
		for _, cells := range added {
			if err := a.writer.AppendCells(appName, ticketsTab, cells); err != nil {
				return err
			}
			if err := a.logInvoice(p, cells); err != nil {
				return err
			}
		}
		if left > 0 {
			if err := a.writer.Set(appName, ticketsTab, match, map[string]string{"Quantity": strconv.Itoa(left)}); err != nil {
				return err
			}
		} else if err := a.writer.Delete(appName, ticketsTab, match); err != nil {
			return err
		}
		return a.logChange(actor, "offer", "waitlist", map[string]string{
			"Celebration": p.Celebration, "Party": p.ID, "Title": p.Title, "Email": t.Purchaser, "Details": fmt.Sprintf("%d of %d, $%s", n, t.Quantity, PriceCell(t.Price)),
		})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: offered tickets", "actor", actor, "party", p.Title, "purchaser", t.Purchaser, "offered", n, "left", left)
	a.mailOffered(r, p, t.Purchaser, added, actor)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) findTicket(w http.ResponseWriter, id string) (*Ticket, *Party, bool) {
	t, p := a.cache.Model().TicketByID(strings.TrimSpace(id))
	if t == nil {
		http.Error(w, "no such ticket", http.StatusNotFound)
		return nil, nil, false
	}
	return t, p, true
}

// owns says whether the actor may act on a ticket as its holder: they bought
// it, it is for them, or either is someone in their household.
// kid says the actor is a student and nothing else: tickets are a family's
// business, so a student neither takes them nor passes them on - a parent
// does, and a host or admin may on anyone's behalf.
func (a app) kid(actor string) bool {
	person, known := a.directory.Person(actor)
	return known && person.IsStudent && !person.IsParent && !person.IsStaff
}

func (a app) owns(t *Ticket, actor string) bool {
	return InHousehold(a.directory, actor, t.Purchaser) || (t.Email != "" && InHousehold(a.directory, actor, t.Email))
}

// removeTicket takes a sold ticket off a party, or a name off the waitlist.
// A sold ticket is the party's money: it is a fundraiser, so only whoever
// runs the party gives one back. A place on the waitlist costs nothing, so
// the family that took it may leave it.
func (a app) removeTicket(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		TicketID string `json:"ticketId"`
	}
	if !decode(w, r, &body) {
		return
	}
	t, p, ok := a.findTicket(w, body.TicketID)
	if !ok {
		return
	}
	editor := a.editor(p, actor, admin)
	if !editor && t.Status == TicketSold {
		http.Error(w, "tickets can't be given back; ask the party's host, or resell it to another family", http.StatusForbidden)
		return
	}
	if !editor && !a.owns(t, actor) {
		http.Error(w, "only the family that asked, a host, or an admin can remove this", http.StatusForbidden)
		return
	}
	if !editor && p.Past(now()) {
		http.Error(w, "this party has already happened", http.StatusBadRequest)
		return
	}
	match := map[string]string{"Ticket ID": t.ID}
	who := t.Email
	if who == "" {
		who = t.Name
	}
	status := t.Status
	if !a.commit(r.Context(), w, a.cache.Tables().without(ticketsTab, match), func() error {
		if err := a.writer.Delete(appName, ticketsTab, match); err != nil {
			return err
		}
		return a.logChange(actor, "remove", "ticket", map[string]string{
			"Celebration": p.Celebration, "Party": p.ID, "Title": p.Title, "Email": who, "Details": status,
		})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: ticket removed", "actor", actor, "party", p.Title, "who", who, "was", status)
	w.WriteHeader(http.StatusNoContent)
}

// editTicket changes a waitlist request's quantity (the family, a host or
// an admin) or a ticket's note (the holder's household).
func (a app) editTicket(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		TicketID string  `json:"ticketId"`
		Quantity *int    `json:"quantity"`
		Note     *string `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	t, p, ok := a.findTicket(w, body.TicketID)
	if !ok {
		return
	}
	editor := a.editor(p, actor, admin)
	cells := map[string]string{}
	details := []string{}
	if body.Quantity != nil {
		if !editor && !a.owns(t, actor) {
			http.Error(w, "only the family that asked, a host, or an admin can change a request", http.StatusForbidden)
			return
		}
		if t.Status != TicketWaitlist {
			http.Error(w, "only a waitlist request has a quantity", http.StatusBadRequest)
			return
		}
		if *body.Quantity < 1 || *body.Quantity > maxTicketsPerPurchase {
			http.Error(w, fmt.Sprintf("ask for between 1 and %d tickets", maxTicketsPerPurchase), http.StatusBadRequest)
			return
		}
		cells["Quantity"] = strconv.Itoa(*body.Quantity)
		details = append(details, fmt.Sprintf("×%d", *body.Quantity))
	}
	if body.Note != nil {
		if !editor && !a.owns(t, actor) {
			http.Error(w, "only the ticket's family, a host, or an admin can change the note", http.StatusForbidden)
			return
		}
		if err := checkText("note", *body.Note); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		cells["Note"] = strings.TrimSpace(*body.Note)
		details = append(details, "note")
	}
	if len(cells) == 0 {
		http.Error(w, "nothing to change", http.StatusBadRequest)
		return
	}
	match := map[string]string{"Ticket ID": t.ID}
	who := t.Email
	if who == "" {
		who = t.Name
	}
	if !a.commit(r.Context(), w, a.cache.Tables().with(ticketsTab, match, cells), func() error {
		if err := a.writer.Set(appName, ticketsTab, match, cells); err != nil {
			return err
		}
		return a.logChange(actor, "edit", "ticket", map[string]string{
			"Celebration": p.Celebration, "Party": p.ID, "Title": p.Title, "Email": who, "Details": strings.Join(details, ", "),
		})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: ticket edited", "actor", actor, "party", p.Title, "who", who, "details", details)
	w.WriteHeader(http.StatusNoContent)
}

// reassignTicket moves a sold ticket to someone else: the family passing it
// to a sibling or reselling it to another family, or a host doing the same
// for them. The row keeps its id, price, place in line and who is billed -
// a resale is settled between the two families themselves; only who holds
// the ticket changes, to a directory person the party admits, or a guest by
// name.
func (a app) reassignTicket(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		TicketID string `json:"ticketId"`
		Email    string `json:"email"`
		Name     string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	t, p, ok := a.findTicket(w, body.TicketID)
	if !ok {
		return
	}
	editor := a.editor(p, actor, admin)
	if !editor && (!a.owns(t, actor) || a.kid(actor)) {
		http.Error(w, "only a parent in the family that holds this ticket, a host, or an admin can reassign it", http.StatusForbidden)
		return
	}
	if t.Status != TicketSold {
		http.Error(w, "only a sold ticket can be reassigned", http.StatusBadRequest)
		return
	}
	if !editor && p.Past(now()) {
		http.Error(w, "this party has already happened", http.StatusBadRequest)
		return
	}
	email, name := cleanEmail(body.Email), strings.TrimSpace(body.Name)
	if email == "" && name == "" {
		http.Error(w, "pick someone, or name a guest", http.StatusBadRequest)
		return
	}
	if len(name) > maxNameLength {
		http.Error(w, "the name is too long", http.StatusBadRequest)
		return
	}
	if email != "" {
		if err := checkEmail(email); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		email = a.directory.Resolve(email)
		person, known := a.directory.Person(email)
		if known && !p.Admits(person, known) {
			http.Error(w, fmt.Sprintf("%s can't hold a ticket: this party is for %s", person.Name, audienceWords(p)), http.StatusBadRequest)
			return
		}
		if known {
			// Someone in the directory is named by their address alone.
			name = ""
		}
		for _, other := range p.Tickets {
			if other.ID != t.ID && other.Email == email && other.Status == TicketSold {
				http.Error(w, fmt.Sprintf("%s already has a ticket", a.nameOf(email)), http.StatusBadRequest)
				return
			}
		}
	}
	was := t.Email
	if was == "" {
		was = t.Name
	}
	who := email
	if who == "" {
		who = name
	}
	match := map[string]string{"Ticket ID": t.ID}
	cells := map[string]string{"Email": email, "Name": name}
	if !a.commit(r.Context(), w, a.cache.Tables().with(ticketsTab, match, cells), func() error {
		if err := a.writer.Set(appName, ticketsTab, match, cells); err != nil {
			return err
		}
		return a.logChange(actor, "reassign", "ticket", map[string]string{
			"Celebration": p.Celebration, "Party": p.ID, "Title": p.Title, "Email": who, "Details": "from " + was,
		})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: ticket reassigned", "actor", actor, "party", p.Title, "from", was, "to", who)
	w.WriteHeader(http.StatusNoContent)
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

func countCell(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// saveParty adds a party or edits one. Anyone may post one - it waits as
// Pending for an admin, with the poster as its host - and a host edits their
// own; only an admin sets the status or moves it to another celebration.
func (a app) saveParty(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body partyBody
	if !decode(w, r, &body) {
		return
	}
	model := a.cache.Model()
	adding := strings.TrimSpace(body.ID) == ""
	var current *Party
	if !adding {
		var ok bool
		if current, ok = a.findParty(w, body.ID); !ok {
			return
		}
		if !a.editor(current, actor, admin) {
			http.Error(w, "only a host or admin can edit this party", http.StatusForbidden)
			return
		}
	}
	celebration := strings.TrimSpace(body.Celebration)
	status := strings.TrimSpace(body.Status)
	// The status, the celebration and the category are an admin's to set;
	// a host's edit keeps them as they are, and a new party from a host
	// starts pending, under the current celebration, with no category.
	category := strings.TrimSpace(body.Category)
	switch {
	case adding && admin:
		if status == "" {
			status = StatusOpen
		}
	case adding:
		if !model.Settings.HostingOpen {
			http.Error(w, "hosting is closed for now - an admin can open it in Settings", http.StatusForbidden)
			return
		}
		status, category = StatusPending, ""
		if c := model.Current(); c != nil {
			celebration = c.Code
		}
	case admin:
		if status == "" {
			status = current.Status
		}
	default:
		status, celebration, category = current.Status, current.Celebration, current.Category
	}
	if !slices.Contains(Statuses, status) {
		http.Error(w, "status must be one of "+strings.Join(Statuses, ", "), http.StatusBadRequest)
		return
	}
	if celebration == "" {
		if c := model.Current(); c != nil {
			celebration = c.Code
		}
	}
	if body.Price < 0 || body.Capacity < 0 || body.Minimum < 0 {
		http.Error(w, "price, capacity, and minimum can't be negative", http.StatusBadRequest)
		return
	}
	if !body.Adults && !body.Students {
		http.Error(w, "let adults, students, or both hold a ticket", http.StatusBadRequest)
		return
	}
	hosts := normalizeEmails(body.HostEmails)
	if adding && !admin && !slices.Contains(hosts, actor) {
		hosts = append(hosts, actor)
	}
	for _, h := range hosts {
		if err := checkEmail(h); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	// A host may not drop themselves from a party they are editing, so a
	// party never ends up run by nobody while someone is in the middle of it;
	// an admin may leave it hostless.
	if !adding && !admin && !slices.Contains(hosts, actor) {
		http.Error(w, "you can't remove yourself as a host; ask another host or an admin", http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(body.ID)
	if adding {
		id = NewID()
	}
	// The friendly address: lower-case letters, digits and hyphens, one party
	// per address across every celebration.
	pretty := NormalizePretty(body.PrettyID)
	if err := CheckPretty(pretty); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if other := model.ByPretty(pretty); pretty != "" && other != nil && other.ID != id {
		http.Error(w, fmt.Sprintf("%q is already the address of %s", pretty, other.Title), http.StatusBadRequest)
		return
	}
	// Changing or dropping a friendly address leaves a redirect behind, so a
	// link someone kept still works.
	var redirect map[string]string
	if current != nil {
		was, now := model.PathOf(current), "/parties/"+id
		if pretty != "" {
			now = "/p/" + pretty
		}
		if was != now {
			redirect = map[string]string{"Type": RedirectParty, "Old": was, "New": now, "Date": today()}
		}
	}
	cells := map[string]string{
		"Celebration": celebration, "Title": strings.TrimSpace(body.Title), "Subtitle": strings.TrimSpace(body.Subtitle),
		"Summary": strings.TrimSpace(body.Summary), "Description": strings.TrimSpace(body.Description), "Need To Know": strings.TrimSpace(body.NeedToKnow),
		"Note Emoji": strings.TrimSpace(body.NoteEmoji), "Note Title": strings.TrimSpace(body.NoteTitle),
		"Hosts": strings.TrimSpace(body.Hosts), "Category": category, "Audience": strings.TrimSpace(body.Audience),
		"Ticket Unit": strings.TrimSpace(body.Unit), "Price": PriceCell(body.Price), "Capacity": countCell(body.Capacity), "Minimum": countCell(body.Minimum),
		"Start": strings.TrimSpace(body.Start), "End": strings.TrimSpace(body.End), "Location": strings.TrimSpace(body.Location),
		"Address": strings.TrimSpace(body.Address), "Pretty ID": pretty, "Image": strings.TrimSpace(body.Image), "Flyer Image": strings.TrimSpace(body.Flyer), "Status": status, "Tickets": map[bool]string{true: "Open", false: "Closed"}[body.TicketsOpen],
		"Waitlist": YesNo(body.Waitlist), "Adults": YesNo(body.Adults), "Students": YesNo(body.Students),
		"Drop-Off": YesNo(body.DropOff), "Parent Ticket Required": YesNo(body.ParentTicket),
	}
	if adding {
		cells["Party ID"] = id
		cells["Added By"] = actor
		cells["Added"] = today()
	}
	match := map[string]string{"Party ID": id}
	tables := a.cache.Tables()
	if adding {
		tables = tables.with(partiesTab, nil, cells)
	} else {
		tables = tables.with(partiesTab, match, cells)
	}
	// The Hosts tab holds exactly the list given: rows gone are deleted, rows
	// new are appended.
	was := []string{}
	if current != nil {
		was = current.HostEmails
	}
	tables = tables.without(hostsTab, match)
	for _, h := range hosts {
		tables = tables.with(hostsTab, nil, map[string]string{"Party ID": id, "Email": h})
	}
	if redirect != nil {
		tables = tables.with(redirectsTab, nil, redirect)
	}
	action := map[bool]string{true: "add", false: "edit"}[adding]
	if !a.commit(r.Context(), w, tables, func() error {
		if adding {
			if err := a.writer.AppendCells(appName, partiesTab, cells); err != nil {
				return err
			}
		} else if err := a.writer.Set(appName, partiesTab, match, cells); err != nil {
			return err
		}
		for _, h := range was {
			if !slices.Contains(hosts, h) {
				if err := a.writer.Delete(appName, hostsTab, map[string]string{"Party ID": id, "Email": h}); err != nil {
					return err
				}
			}
		}
		for _, h := range hosts {
			if !slices.Contains(was, h) {
				if err := a.writer.AppendCells(appName, hostsTab, map[string]string{"Party ID": id, "Email": h}); err != nil {
					return err
				}
			}
		}
		if redirect != nil {
			if err := a.writer.AppendCells(appName, redirectsTab, redirect); err != nil {
				return err
			}
		}
		return a.logChange(actor, action, "party", map[string]string{
			"Celebration": celebration, "Party": id, "Title": cells["Title"], "Details": status + ", hosts " + strings.Join(hosts, " "),
		})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: saved party", "actor", actor, "action", action, "party", cells["Title"], "status", status)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func (a app) deleteParty(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, ok := a.findParty(w, body.ID)
	if !ok {
		return
	}
	if len(p.Tickets) > 0 {
		http.Error(w, "remove its tickets and waitlist first, or hide it instead", http.StatusBadRequest)
		return
	}
	match := map[string]string{"Party ID": p.ID}
	tables := a.cache.Tables().without(partiesTab, match).without(hostsTab, match)
	if !a.commit(r.Context(), w, tables, func() error {
		if err := a.writer.Delete(appName, partiesTab, match); err != nil {
			return err
		}
		if err := a.writer.Delete(appName, hostsTab, match); err != nil {
			return err
		}
		return a.logChange(actor, "remove", "party", map[string]string{"Celebration": p.Celebration, "Party": p.ID, "Title": p.Title})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: removed party", "actor", actor, "party", p.Title)
	w.WriteHeader(http.StatusNoContent)
}

// setFlags is the party page's row of switches for whoever runs it: whether
// tickets are on sale, whether a full party takes a waitlist, who may come,
// drop-off, and whether a staying parent needs a ticket.
func (a app) setFlags(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		ID           string `json:"id"`
		TicketsOpen  bool   `json:"ticketsOpen"`
		Waitlist     bool   `json:"waitlist"`
		Adults       bool   `json:"adults"`
		Students     bool   `json:"students"`
		DropOff      bool   `json:"dropOff"`
		ParentTicket bool   `json:"parentTicket"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, ok := a.findParty(w, body.ID)
	if !ok {
		return
	}
	if !a.editor(p, actor, admin) {
		http.Error(w, "only a host or admin can change this", http.StatusForbidden)
		return
	}
	if !body.Adults && !body.Students {
		http.Error(w, "let adults, students, or both hold a ticket", http.StatusBadRequest)
		return
	}
	match := map[string]string{"Party ID": p.ID}
	cells := map[string]string{
		"Tickets": map[bool]string{true: "Open", false: "Closed"}[body.TicketsOpen], "Waitlist": YesNo(body.Waitlist),
		"Adults": YesNo(body.Adults), "Students": YesNo(body.Students),
		"Drop-Off": YesNo(body.DropOff), "Parent Ticket Required": YesNo(body.ParentTicket),
	}
	if !a.commit(r.Context(), w, a.cache.Tables().with(partiesTab, match, cells), func() error {
		if err := a.writer.Set(appName, partiesTab, match, cells); err != nil {
			return err
		}
		return a.logChange(actor, "edit", "party", map[string]string{
			"Celebration": p.Celebration, "Party": p.ID, "Title": p.Title, "Details": fmt.Sprintf("tickets %s, waitlist %s, adults %s, students %s, drop-off %s, parent ticket %s",
				cells["Tickets"], cells["Waitlist"], cells["Adults"], cells["Students"], cells["Drop-Off"], cells["Parent Ticket Required"]),
		})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: set party flags", "actor", actor, "party", p.Title, "tickets", cells["Tickets"])
	w.WriteHeader(http.StatusNoContent)
}

// setStatus is an admin's approve, hide, or unhide.
func (a app) setStatus(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, ok := a.findParty(w, body.ID)
	if !ok {
		return
	}
	if !slices.Contains(Statuses, body.Status) {
		http.Error(w, "status must be one of "+strings.Join(Statuses, ", "), http.StatusBadRequest)
		return
	}
	match := map[string]string{"Party ID": p.ID}
	cells := map[string]string{"Status": body.Status}
	if !a.commit(r.Context(), w, a.cache.Tables().with(partiesTab, match, cells), func() error {
		if err := a.writer.Set(appName, partiesTab, match, cells); err != nil {
			return err
		}
		return a.logChange(actor, "status", "party", map[string]string{"Celebration": p.Celebration, "Party": p.ID, "Title": p.Title, "Details": body.Status})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: set party status", "actor", actor, "party", p.Title, "status", body.Status)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveCelebration(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
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
	if !decode(w, r, &body) {
		return
	}
	code := strings.TrimSpace(body.Code)
	model := a.cache.Model()
	adding := strings.TrimSpace(body.Original) == ""
	if !adding && model.Celebration(body.Original) == nil {
		http.Error(w, "no such celebration", http.StatusNotFound)
		return
	}
	renamed := !adding && body.Original != code
	if (adding || renamed) && model.Celebration(code) != nil {
		http.Error(w, fmt.Sprintf("%q is already a celebration", code), http.StatusBadRequest)
		return
	}
	cells := map[string]string{
		"Code": code, "Title": strings.TrimSpace(body.Title), "Subtitle": strings.TrimSpace(body.Subtitle),
		"Start": strings.TrimSpace(body.Start), "End": strings.TrimSpace(body.End), "Location": strings.TrimSpace(body.Location),
		"Address": strings.TrimSpace(body.Address), "Description": strings.TrimSpace(body.Description), "Image": strings.TrimSpace(body.Image),
		"Button Text": strings.TrimSpace(body.ButtonText), "Button URL": strings.TrimSpace(body.ButtonURL), "Current": YesNo(body.Current), "Banner": YesNo(body.Banner),
	}
	tables := a.cache.Tables()
	// One celebration is current and one is the banner: marking this one
	// unmarks the rest, flag by flag.
	others := map[string]map[string]string{}
	for _, c := range model.Celebrations {
		if c.Code == body.Original {
			continue
		}
		unmark := map[string]string{}
		if body.Current && c.Current {
			unmark["Current"] = "No"
		}
		if body.Banner && c.Banner {
			unmark["Banner"] = "No"
		}
		if len(unmark) > 0 {
			others[c.Code] = unmark
			tables = tables.with(celebrationsTab, map[string]string{"Code": c.Code}, unmark)
		}
	}
	var match map[string]string
	if adding {
		tables = tables.with(celebrationsTab, nil, cells)
	} else {
		match = map[string]string{"Code": body.Original}
		tables = tables.with(celebrationsTab, match, cells)
		if renamed {
			tables = tables.with(partiesTab, map[string]string{"Celebration": body.Original}, map[string]string{"Celebration": code})
		}
	}
	hadParties := !adding && tables.count(partiesTab, map[string]string{"Celebration": code}) > 0
	action := map[bool]string{true: "add", false: "edit"}[adding]
	if !a.commit(r.Context(), w, tables, func() error {
		for other, unmark := range others {
			if err := a.writer.Set(appName, celebrationsTab, map[string]string{"Code": other}, unmark); err != nil {
				return err
			}
		}
		if adding {
			if err := a.writer.AppendCells(appName, celebrationsTab, cells); err != nil {
				return err
			}
		} else {
			if err := a.writer.Set(appName, celebrationsTab, match, cells); err != nil {
				return err
			}
			if renamed && hadParties {
				if err := a.writer.Set(appName, partiesTab, map[string]string{"Celebration": body.Original}, map[string]string{"Celebration": code}); err != nil {
					return err
				}
			}
		}
		return a.logChange(actor, action, "celebration", map[string]string{"Celebration": code, "Title": cells["Title"]})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: saved celebration", "actor", actor, "action", action, "code", code)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteCelebration(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &body) {
		return
	}
	model := a.cache.Model()
	if model.Celebration(body.Code) == nil {
		http.Error(w, "no such celebration", http.StatusNotFound)
		return
	}
	tables := a.cache.Tables()
	if tables.count(partiesTab, map[string]string{"Celebration": body.Code}) > 0 {
		http.Error(w, "parties belong to this celebration; move or remove them first", http.StatusBadRequest)
		return
	}
	match := map[string]string{"Code": body.Code}
	if !a.commit(r.Context(), w, tables.without(celebrationsTab, match), func() error {
		if err := a.writer.Delete(appName, celebrationsTab, match); err != nil {
			return err
		}
		return a.logChange(actor, "remove", "celebration", map[string]string{"Celebration": body.Code})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: removed celebration", "actor", actor, "code", body.Code)
	w.WriteHeader(http.StatusNoContent)
}

// saveCategory adds or renames a category; a rename carries every party
// naming it along.
func (a app) saveCategory(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Original string `json:"original"`
		Title    string `json:"title"`
	}
	if !decode(w, r, &body) {
		return
	}
	title := strings.TrimSpace(body.Title)
	if err := checkTitle("category", title); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	model := a.cache.Model()
	adding := body.Original == ""
	if !adding && !slices.Contains(model.Categories, body.Original) {
		http.Error(w, "no such category", http.StatusNotFound)
		return
	}
	renamed := !adding && body.Original != title
	if (adding || renamed) && slices.Contains(model.Categories, title) {
		http.Error(w, fmt.Sprintf("%q is already a category", title), http.StatusBadRequest)
		return
	}
	tables := a.cache.Tables()
	cells := map[string]string{"Title": title}
	var match map[string]string
	if adding {
		tables = tables.with(categoriesTab, nil, cells)
	} else {
		match = map[string]string{"Title": body.Original}
		tables = tables.with(categoriesTab, match, cells)
		if renamed {
			tables = tables.with(partiesTab, map[string]string{"Category": body.Original}, map[string]string{"Category": title})
		}
	}
	used := !adding && a.cache.Tables().count(partiesTab, map[string]string{"Category": body.Original}) > 0
	action := map[bool]string{true: "add", false: "edit"}[adding]
	if !a.commit(r.Context(), w, tables, func() error {
		if adding {
			if err := a.writer.AppendCells(appName, categoriesTab, cells); err != nil {
				return err
			}
		} else {
			if err := a.writer.Set(appName, categoriesTab, match, cells); err != nil {
				return err
			}
			if renamed && used {
				if err := a.writer.Set(appName, partiesTab, map[string]string{"Category": body.Original}, map[string]string{"Category": title}); err != nil {
					return err
				}
			}
		}
		return a.logChange(actor, action, "category", map[string]string{"Title": title, "Details": body.Original})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: saved category", "actor", actor, "action", action, "category", title)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteCategory(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !slices.Contains(a.cache.Model().Categories, body.Title) {
		http.Error(w, "no such category", http.StatusNotFound)
		return
	}
	tables := a.cache.Tables()
	if tables.count(partiesTab, map[string]string{"Category": body.Title}) > 0 {
		http.Error(w, "parties are filed under this category; move them first", http.StatusBadRequest)
		return
	}
	match := map[string]string{"Title": body.Title}
	if !a.commit(r.Context(), w, tables.without(categoriesTab, match), func() error {
		if err := a.writer.Delete(appName, categoriesTab, match); err != nil {
			return err
		}
		return a.logChange(actor, "remove", "category", map[string]string{"Title": body.Title})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: removed category", "actor", actor, "category", body.Title)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) reorderCategories(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Titles []string `json:"titles"`
	}
	if !decode(w, r, &body) {
		return
	}
	current := a.cache.Model().Categories
	if len(body.Titles) != len(current) {
		http.Error(w, "the order must name every category once", http.StatusBadRequest)
		return
	}
	rows := []map[string]string{}
	for _, title := range body.Titles {
		if !slices.Contains(current, title) || slices.ContainsFunc(rows, func(row map[string]string) bool { return row["Title"] == title }) {
			http.Error(w, "the order must name every category once", http.StatusBadRequest)
			return
		}
		rows = append(rows, map[string]string{"Title": title})
	}
	tables := *a.cache.Tables()
	tables.Categories = rows
	if !a.commit(r.Context(), w, &tables, func() error {
		if err := a.writer.Reorder(appName, categoriesTab, "Title", body.Titles); err != nil {
			return err
		}
		return a.logChange(actor, "reorder", "category", map[string]string{"Details": strings.Join(body.Titles, ", ")})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: reordered categories", "actor", actor)
	w.WriteHeader(http.StatusNoContent)
}

// saveTheme takes the page's colours at once (internal/theme) and writes
// their rows of the Settings tab, adding any the tab has not got yet.
func (a app) saveTheme(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body theme.Theme
	if !decode(w, r, &body) {
		return
	}
	t, err := theme.Of(body.Values())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	values := t.Values()
	tables := a.cache.Tables()
	for _, key := range theme.Keys {
		tables = tables.with(settingsTab, map[string]string{"Key": key}, map[string]string{"Value": values[key]})
	}
	if !a.commit(r.Context(), w, tables, func() error {
		for _, key := range theme.Keys {
			if err := a.writer.Set(appName, settingsTab, map[string]string{"Key": key}, map[string]string{"Value": values[key]}); err != nil {
				return err
			}
		}
		return a.logChange(actor, "edit", "settings", nil)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: changed the theme", "actor", actor, "theme", t)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveSettings(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body Settings
	if !decode(w, r, &body) {
		return
	}
	values := map[string]string{PartiesIntroKey: strings.TrimSpace(body.PartiesIntro), TicketNoteKey: strings.TrimSpace(body.TicketNote), HostingOpenKey: YesNo(body.HostingOpen)}
	tables := a.cache.Tables()
	// A key the tab has not got yet is appended rather than set, so a
	// setting added after the sheet was made needs no hand edit.
	present := map[string]bool{}
	for _, row := range tables.Settings {
		present[row["Key"]] = true
	}
	for key, value := range values {
		if present[key] {
			tables = tables.with(settingsTab, map[string]string{"Key": key}, map[string]string{"Value": value})
		} else {
			tables = tables.with(settingsTab, nil, map[string]string{"Key": key, "Value": value})
		}
	}
	if !a.commit(r.Context(), w, tables, func() error {
		for key, value := range values {
			if present[key] {
				if err := a.writer.Set(appName, settingsTab, map[string]string{"Key": key}, map[string]string{"Value": value}); err != nil {
					return err
				}
			} else if err := a.writer.AppendCells(appName, settingsTab, map[string]string{"Key": key, "Value": value}); err != nil {
				return err
			}
		}
		return a.logChange(actor, "edit", "settings", nil)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: changed the settings", "actor", actor)
	w.WriteHeader(http.StatusNoContent)
}

// importImage stores a picked search result under the app's own folder.
func (a app) importImage(w http.ResponseWriter, r *http.Request) {
	a.search.ServeImport(w, r, a.store, imageFolder, maxImageSize)
}

// uploadImage stores a content-addressed image and returns the name the sheet
// should record; the save that follows references it.
func (a app) uploadImage(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		http.Error(w, "image uploads require real-data mode", http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImageSize)
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "an image file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "could not read the image", http.StatusBadRequest)
		return
	}
	mimeType := http.DetectContentType(content)
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp"}[mimeType]
	if ext == "" {
		http.Error(w, fmt.Sprintf("%s is not a supported image", header.Filename), http.StatusBadRequest)
		return
	}
	sum := sha256.Sum256(content)
	name := hex.EncodeToString(sum[:]) + ext
	if err := a.store.Put(imageFolder, name, mimeType, content); err != nil {
		slog.ErrorContext(r.Context(), "celebrate: store image", "error", err)
		http.Error(w, "could not store the image", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"name": imageFolder + "/" + name})
}

// invoicesCSV is the INVOICING ledger for one celebration as a file, in the
// ledger's own columns, for whoever wants it outside the sheet.
func (a app) invoicesCSV(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	model := a.cache.Model()
	code := r.URL.Query().Get("celebration")
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"invoicing-%s.csv\"", code))
	out := csv.NewWriter(w)
	out.Write(InvoicingColumns)
	for _, l := range model.Invoicing {
		if code != "" && l.Code != code {
			continue
		}
		out.Write([]string{l.Date, l.Party, l.Code, l.Purchaser, l.Guest, l.Action, strconv.Itoa(l.Quantity), PriceCell(l.Cost), l.Invoice, l.InvoiceTo})
	}
	out.Flush()
}

func (a app) adminState(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	view := struct {
		Email  string   `json:"email"`
		Admins []string `json:"admins"`
	}{Email: email, Admins: a.cache.Admins(a.superAdmins())}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode celebrate admin state", "error", err)
	}
}

func (a app) setAdmins(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Admins []string `json:"admins"`
	}
	if !decode(w, r, &body) {
		return
	}
	// Super admins show in the merged list but never round-trip into the tab.
	super := map[string]bool{}
	for _, e := range a.superAdmins() {
		super[e] = true
	}
	admins := []string{}
	for _, e := range normalizeEmails(body.Admins) {
		if !super[e] {
			admins = append(admins, e)
		}
	}
	current := a.cache.tabAdmins()
	was := map[string]bool{}
	for _, e := range current {
		was[e] = true
	}
	is := map[string]bool{}
	for _, e := range admins {
		is[e] = true
	}
	if !a.commit(r.Context(), w, a.cache.Tables().withAdmins(admins), func() error {
		for _, e := range current {
			if !is[e] {
				if err := a.writer.Delete(appName, adminsTab, map[string]string{"Email": e}); err != nil {
					return err
				}
			}
		}
		for _, e := range admins {
			if !was[e] {
				if err := a.writer.Append(appName, adminsTab, []string{e}); err != nil {
					return err
				}
			}
		}
		return nil
	}) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: set the admin list", "actor", actor, "admins", admins)
	w.WriteHeader(http.StatusNoContent)
}

func normalizeEmails(emails []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, e := range emails {
		e = cleanEmail(e)
		if e == "" || !strings.Contains(e, "@") || seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	return out
}
