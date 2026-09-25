package celebrate

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/imagesearch"
	"heliosian/internal/logging"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	imageFolder           = "party-images"
	maxImageSize          = 8 << 20
	shell                 = "web/celebrate/index.html"
	maxTicketsPerPurchase = 20
)

var pages = []string{
	"/{$}", "/parties/{id}", "/p/{pretty}", "/celebrations/{code}", "/my", "/my/{email}", "/hosting", "/admin",
}

var local = mustLocation("America/Los_Angeles")

func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		logging.Fatal("load time zone", "name", name, "error", err)
	}
	return loc
}

type ImageSearch = imagesearch.Search

type app struct {
	cache       *Cache
	store       *blob.Store
	directory   Directory
	superAdmins func() []string
	search      ImageSearch
	mailer      mail.Sender
	from        string
	rsvps       RSVPLookup
}

func Register(mux *http.ServeMux, cache *Cache, store *blob.Store, directory Directory, superAdmins func() []string, search ImageSearch, mailer mail.Sender, from string, rsvps RSVPLookup) {
	if search.UserAgent == "" {
		search.UserAgent = "Helios Celebrate image search (+https://celebrate.heliosian.com)"
	}
	a := app{cache: cache, store: store, directory: directory, superAdmins: superAdmins, search: search, mailer: mailer, from: from, rsvps: rsvps}
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.HandleFunc("GET /api/celebrate/model", a.model)
	mux.HandleFunc("GET /api/celebrate/people", a.people)
	mux.HandleFunc("GET /api/celebrate/images/search", a.search.ServeSearch)
	mux.HandleFunc("GET /api/celebrate/images/thumb", a.search.ServeThumb)
	mux.HandleFunc("POST /api/celebrate/images/import", a.importImage)
	mux.HandleFunc("POST /api/celebrate/image", a.uploadImage)
	mux.HandleFunc("GET /open/share/upcoming.png", a.shareUpcoming)
	mux.HandleFunc("GET /open/share/{id}", a.shareCard)
	mux.HandleFunc("POST /api/celebrate/tickets", a.buyTickets)
	mux.HandleFunc("POST /api/celebrate/waitlist", a.joinWaitlist)
	mux.HandleFunc("POST /api/celebrate/waitlist/offer", a.offerTickets)
	mux.HandleFunc("POST /api/celebrate/ticket", a.editTicket)
	mux.HandleFunc("DELETE /api/celebrate/ticket", a.removeTicket)
	mux.HandleFunc("POST /api/celebrate/ticket/reassign", a.reassignTicket)
	mux.HandleFunc("POST /api/celebrate/party", a.saveParty)
	mux.HandleFunc("DELETE /api/celebrate/party", a.deleteParty)
	mux.HandleFunc("POST /api/celebrate/party/flags", a.setFlags)
	mux.HandleFunc("POST /api/celebrate/party/status", a.setStatus)
	mux.HandleFunc("POST /api/celebrate/celebration", a.saveCelebration)
	mux.HandleFunc("DELETE /api/celebrate/celebration", a.deleteCelebration)
	mux.HandleFunc("POST /api/celebrate/category", a.saveCategory)
	mux.HandleFunc("DELETE /api/celebrate/category", a.deleteCategory)
	mux.HandleFunc("POST /api/celebrate/categories/order", a.reorderCategories)
	mux.HandleFunc("POST /api/celebrate/settings", a.saveSettings)
	mux.HandleFunc("GET /api/celebrate/invoices.csv", a.invoicesCSV)
	mux.HandleFunc("GET /api/admin/state", a.adminState)
	mux.HandleFunc("POST /api/admin/admins", a.setAdmins)
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

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
	view := RenderWith(a.cache.Model(), a.directory, a.rsvps, email, admin, now())
	view.ImageSearch = a.search.On()
	view.User.IsSuperAdmin = a.cache.IsSuperAdmin(email)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode celebrate model", "error", err)
	}
}

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

func (a app) commit(w http.ResponseWriter, r *http.Request, actor string, ops ...store.Op) bool {
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func (a app) invoiceRow(p *Party, cells store.Row) store.Row {
	price, _ := ParsePrice(cells["Price"])
	if cells["Status"] != TicketSold || price <= 0 {
		return nil
	}
	return store.Row{
		"Date": today(), "Party Title": p.Title, "Event Code": p.Celebration, "Purchaser Email": cells["Purchaser"],
		"Guest Name": a.ticketName(cells), "Action": "ADD", "Quantity": "1", "Cost": PriceCell(price),
	}
}

func (a app) ticketOps(p *Party, added []store.Row) []store.Op {
	ops := []store.Op{}
	for _, cells := range added {
		ops = append(ops, store.Insert(ticketsTab, cells))
		if row := a.invoiceRow(p, cells); row != nil {
			ops = append(ops, store.Insert(invoicingTab, row))
		}
	}
	return ops
}

func cleanEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

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

func (a app) editor(p *Party, actor string, admin bool) bool {
	return admin || p.Hosted(actor)
}

func (a app) nameOf(email string) string {
	if p, ok := a.directory.Person(a.directory.Resolve(email)); ok {
		return p.Name
	}
	return DisplayName(email)
}

func (a app) ticketName(t map[string]string) string {
	if t["Email"] != "" {
		if p, ok := a.directory.Person(a.directory.Resolve(t["Email"])); ok && p.Name != "" {
			return p.Name
		}
	}
	if t["Name"] != "" {
		return t["Name"]
	}
	return DisplayName(t["Email"])
}

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

func (a app) buyTickets(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
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
	purchaser := cleanEmail(body.Purchaser)
	if purchaser == "" {
		purchaser = actor
	}
	if err := checkEmail(purchaser); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
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
	remaining := p.Remaining()
	sold, waitlisted := 0, 0
	price := PriceCell(p.Price)
	if body.Free {
		price = "0"
	}
	added := []store.Row{}
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
		added = append(added, store.Row{
			"Ticket ID": NewID(), "Party ID": p.ID, "Email": rw.email, "Name": rw.name, "Purchaser": rw.purchaser,
			"Status": TicketSold, "Quantity": "1", "Price": price, "Note": strings.TrimSpace(body.Note), "Added By": actor, "Added": stamp(),
		})
	}
	if waitlisted > 0 {
		added = append(added, waitlistRequest(p, purchaser, waitlisted, strings.TrimSpace(body.Note), actor))
	}
	ops := []store.Op{}
	if body.Free && body.RaiseCapacity && p.Capacity > 0 && sold > 0 {
		ops = append(ops, store.Update(partiesTab, store.Row{"Party ID": p.ID}, store.Row{"Capacity": countCell(p.Capacity + sold)}))
	}
	if !a.commit(w, r, actor, append(ops, a.ticketOps(p, added)...)...) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: tickets taken", "actor", actor, "party", p.Title, "purchaser", purchaser, "sold", sold, "waitlisted", waitlisted)
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

func waitlistRequest(p *Party, purchaser string, quantity int, note, actor string) store.Row {
	return store.Row{
		"Ticket ID": NewID(), "Party ID": p.ID, "Email": purchaser, "Name": "", "Purchaser": purchaser,
		"Status": TicketWaitlist, "Quantity": strconv.Itoa(quantity), "Price": PriceCell(p.Price), "Note": note, "Added By": actor, "Added": stamp(),
	}
}

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
	for _, t := range p.Tickets {
		if t.Status == TicketWaitlist && t.Purchaser == purchaser {
			cells := store.Row{"Quantity": strconv.Itoa(body.Quantity), "Note": strings.TrimSpace(body.Note)}
			if !a.commit(w, r, actor, store.Update(ticketsTab, store.Row{"Ticket ID": t.ID}, cells)) {
				return
			}
			slog.InfoContext(r.Context(), "celebrate: waitlist request changed", "actor", actor, "party", p.Title, "purchaser", purchaser, "quantity", body.Quantity)
			full := map[string]string{"Email": t.Email, "Purchaser": t.Purchaser, "Status": TicketWaitlist, "Quantity": cells["Quantity"], "Note": cells["Note"]}
			a.mailTickets(r, p, purchaser, []map[string]string{full}, actor)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]int{"sold": 0, "waitlisted": body.Quantity})
			return
		}
	}
	cells := waitlistRequest(p, purchaser, body.Quantity, strings.TrimSpace(body.Note), actor)
	if !a.commit(w, r, actor, store.Insert(ticketsTab, cells)) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: joined waitlist", "actor", actor, "party", p.Title, "purchaser", purchaser, "quantity", body.Quantity)
	a.mailTickets(r, p, purchaser, []map[string]string{cells}, actor)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"sold": 0, "waitlisted": body.Quantity})
}

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
	selfTicket := known && p.Admits(holder, true)
	for _, other := range p.Tickets {
		if other.Status == TicketSold && other.Email == t.Purchaser {
			selfTicket = false
		}
	}
	added := []store.Row{}
	for i := 0; i < n; i++ {
		cells := store.Row{
			"Ticket ID": NewID(), "Party ID": p.ID, "Purchaser": t.Purchaser, "Status": TicketSold, "Quantity": "1",
			"Price": PriceCell(t.Price), "Note": t.Note, "Added By": actor, "Added": stamp(),
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
	ops := a.ticketOps(p, added)
	if left > 0 {
		ops = append(ops, store.Update(ticketsTab, match, store.Row{"Quantity": strconv.Itoa(left)}))
	} else {
		ops = append(ops, store.Delete(ticketsTab, match))
	}
	if !a.commit(w, r, actor, ops...) {
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

func (a app) kid(actor string) bool {
	person, known := a.directory.Person(actor)
	return known && person.IsStudent && !person.IsParent && !person.IsStaff
}

func (a app) owns(t *Ticket, actor string) bool {
	return InHousehold(a.directory, actor, t.Purchaser) || (t.Email != "" && InHousehold(a.directory, actor, t.Email))
}

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
	who := t.Email
	if who == "" {
		who = t.Name
	}
	status := t.Status
	if !a.commit(w, r, actor, store.Delete(ticketsTab, store.Row{"Ticket ID": t.ID})) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: ticket removed", "actor", actor, "party", p.Title, "who", who, "was", status)
	w.WriteHeader(http.StatusNoContent)
}

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
	cells := store.Row{}
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
	who := t.Email
	if who == "" {
		who = t.Name
	}
	if !a.commit(w, r, actor, store.Update(ticketsTab, store.Row{"Ticket ID": t.ID}, cells)) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: ticket edited", "actor", actor, "party", p.Title, "who", who, "details", details)
	w.WriteHeader(http.StatusNoContent)
}

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
	if !a.commit(w, r, actor, store.Update(ticketsTab, store.Row{"Ticket ID": t.ID}, store.Row{"Email": email, "Name": name})) {
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
	if !adding && !admin && !slices.Contains(hosts, actor) {
		http.Error(w, "you can't remove yourself as a host; ask another host or an admin", http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(body.ID)
	if adding {
		id = NewID()
	}
	pretty := NormalizePretty(body.PrettyID)
	if CheckPretty(pretty) != nil {
		http.Error(w, fmt.Sprintf("the friendly address can be only lower-case letters, digits and hyphens, at most %d", maxPrettyLength), http.StatusBadRequest)
		return
	}
	if other := model.ByPretty(pretty); pretty != "" && other != nil && other.ID != id {
		http.Error(w, fmt.Sprintf("%q is already the address of %s", pretty, other.Title), http.StatusBadRequest)
		return
	}
	cells := store.Row{
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
	ops := []store.Op{}
	was := []string{}
	if adding {
		cells["Party ID"] = id
		cells["Added By"] = actor
		cells["Added"] = today()
		ops = append(ops, store.Insert(partiesTab, cells))
	} else {
		was = current.HostEmails
		ops = append(ops, store.Update(partiesTab, store.Row{"Party ID": id}, cells))
	}
	for _, h := range was {
		if !slices.Contains(hosts, h) {
			ops = append(ops, store.Delete(hostsTab, store.Row{"Party ID": id, "Email": h}))
		}
	}
	for _, h := range hosts {
		if !slices.Contains(was, h) {
			ops = append(ops, store.Insert(hostsTab, store.Row{"Party ID": id, "Email": h}))
		}
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	action := map[bool]string{true: "add", false: "edit"}[adding]
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
	if !a.commit(w, r, actor, store.Delete(partiesTab, store.Row{"Party ID": p.ID})) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: removed party", "actor", actor, "party", p.Title)
	w.WriteHeader(http.StatusNoContent)
}

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
	cells := store.Row{
		"Tickets": map[bool]string{true: "Open", false: "Closed"}[body.TicketsOpen], "Waitlist": YesNo(body.Waitlist),
		"Adults": YesNo(body.Adults), "Students": YesNo(body.Students),
		"Drop-Off": YesNo(body.DropOff), "Parent Ticket Required": YesNo(body.ParentTicket),
	}
	if !a.commit(w, r, actor, store.Update(partiesTab, store.Row{"Party ID": p.ID}, cells)) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: set party flags", "actor", actor, "party", p.Title, "tickets", cells["Tickets"])
	w.WriteHeader(http.StatusNoContent)
}

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
	if !a.commit(w, r, actor, store.Update(partiesTab, store.Row{"Party ID": p.ID}, store.Row{"Status": body.Status})) {
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
	cells := store.Row{
		"Code": code, "Title": strings.TrimSpace(body.Title), "Subtitle": strings.TrimSpace(body.Subtitle),
		"Start": strings.TrimSpace(body.Start), "End": strings.TrimSpace(body.End), "Location": strings.TrimSpace(body.Location),
		"Address": strings.TrimSpace(body.Address), "Description": strings.TrimSpace(body.Description), "Image": strings.TrimSpace(body.Image),
		"Button Text": strings.TrimSpace(body.ButtonText), "Button URL": strings.TrimSpace(body.ButtonURL), "Current": YesNo(body.Current), "Banner": YesNo(body.Banner),
	}
	ops := []store.Op{}
	for _, c := range model.Celebrations {
		if c.Code == body.Original {
			continue
		}
		unmark := store.Row{}
		if body.Current && c.Current {
			unmark["Current"] = "No"
		}
		if body.Banner && c.Banner {
			unmark["Banner"] = "No"
		}
		if len(unmark) > 0 {
			ops = append(ops, store.Update(celebrationsTab, store.Row{"Code": c.Code}, unmark))
		}
	}
	if adding {
		ops = append(ops, store.Insert(celebrationsTab, cells))
	} else {
		ops = append(ops, store.Update(celebrationsTab, store.Row{"Code": body.Original}, cells))
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	action := map[bool]string{true: "add", false: "edit"}[adding]
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
	if a.cache.Model().Celebration(body.Code) == nil {
		http.Error(w, "no such celebration", http.StatusNotFound)
		return
	}
	if a.cache.Count(partiesTab, store.Row{"Celebration": body.Code}) > 0 {
		http.Error(w, "parties belong to this celebration; move or remove them first", http.StatusBadRequest)
		return
	}
	if !a.commit(w, r, actor, store.Delete(celebrationsTab, store.Row{"Code": body.Code})) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: removed celebration", "actor", actor, "code", body.Code)
	w.WriteHeader(http.StatusNoContent)
}

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
	op := store.Insert(categoriesTab, store.Row{"Title": title})
	if !adding {
		op = store.Update(categoriesTab, store.Row{"Title": body.Original}, store.Row{"Title": title})
	}
	if !a.commit(w, r, actor, op) {
		return
	}
	action := map[bool]string{true: "add", false: "edit"}[adding]
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
	if a.cache.Count(partiesTab, store.Row{"Category": body.Title}) > 0 {
		http.Error(w, "parties are filed under this category; move them first", http.StatusBadRequest)
		return
	}
	if !a.commit(w, r, actor, store.Delete(categoriesTab, store.Row{"Title": body.Title})) {
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
	model := a.cache.Model()
	if len(body.Titles) != len(model.Categories) {
		http.Error(w, "the order must name every category once", http.StatusBadRequest)
		return
	}
	titles, current := []string{}, []string{}
	for _, title := range body.Titles {
		if !slices.Contains(model.Categories, title) || slices.Contains(titles, title) {
			http.Error(w, "the order must name every category once", http.StatusBadRequest)
			return
		}
		titles, current = append(titles, title), append(current, model.categoryOrder[title])
	}
	keys := store.Order(current)
	ops := []store.Op{}
	for i, title := range titles {
		if keys[i] != current[i] {
			ops = append(ops, store.Update(categoriesTab, store.Row{"Title": title}, store.Row{store.OrderColumn: keys[i]}))
		}
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: reordered categories", "actor", actor, "changed", len(ops))
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
	ops := []store.Op{}
	for _, key := range settingKeys {
		ops = append(ops, store.Set(settingsTab, store.Row{"Key": key}, store.Row{"Value": values[key]}))
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: changed the settings", "actor", actor)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) importImage(w http.ResponseWriter, r *http.Request) {
	a.search.ServeImport(w, r, imageFolder, maxImageSize)
}

func (a app) uploadImage(w http.ResponseWriter, r *http.Request) {
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
	current := a.cache.Model().admins
	ops := []store.Op{}
	for _, e := range current {
		if !slices.Contains(admins, e) {
			ops = append(ops, store.Delete(adminsTab, store.Row{"Email": e}))
		}
	}
	for _, e := range admins {
		if !slices.Contains(current, e) {
			ops = append(ops, store.Insert(adminsTab, store.Row{"Email": e}))
		}
	}
	if !a.commit(w, r, actor, ops...) {
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
