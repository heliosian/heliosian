package celebrate

import (
	"crypto/rand"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/imagesearch"
	"heliosian/internal/logging"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
)

const (
	imageFolder           = "party-images"
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
	moved       AddressMoved
	style       *sharecard.Style
}

func Register(mux *http.ServeMux, cache *Cache, store *blob.Store, directory Directory, superAdmins func() []string, search ImageSearch, mailer mail.Sender, from string, rsvps RSVPLookup, moved AddressMoved, style *sharecard.Style) {
	if search.UserAgent == "" {
		search.UserAgent = "Helios Celebrate image search (+https://celebrate.heliosian.com)"
	}
	a := app{cache: cache, store: store, directory: directory, superAdmins: superAdmins, search: search, mailer: mailer, from: from, rsvps: rsvps, moved: moved, style: style}
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.HandleFunc("GET /api/celebrate/model", a.model)
	mux.HandleFunc("GET /api/celebrate/people", a.people)
	a.search.Register(mux, "/api/celebrate", imageFolder, imagesearch.Members)
	mux.HandleFunc("GET /open/share/upcoming.png", a.shareUpcoming)
	mux.HandleFunc("GET /open/share/{id}", a.shareCard)
	mux.HandleFunc("POST /api/celebrate/tickets", a.buyTickets)
	mux.HandleFunc("POST /api/celebrate/waitlist", a.joinWaitlist)
	mux.HandleFunc("POST /api/celebrate/waitlist/offer", a.offerTickets)
	mux.HandleFunc("POST /api/celebrate/ticket", a.editTicket)
	mux.HandleFunc("DELETE /api/celebrate/ticket", a.removeTicket)
	mux.HandleFunc("POST /api/celebrate/ticket/reassign", a.reassignTicket)
	mux.HandleFunc("POST /api/celebrate/address", a.moveAddress)
	mux.HandleFunc("GET /api/celebrate/addresses", a.addresses)
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

func (a app) actor(r *http.Request) access.Actor {
	email := a.directory.Resolve(strings.ToLower(auth.Email(r)))
	return access.Actor{Email: email, Admin: a.cache.IsAdmin(email), Household: a.directory.Family(email)}
}

func (a app) requireAdmin(w http.ResponseWriter, r *http.Request) (access.Actor, bool) {
	actor := a.actor(r)
	if err := requireAdmin(actor); err != nil {
		http.Error(w, err.Error(), access.Status(err))
		return actor, false
	}
	return actor, true
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
	actor := a.actor(r)
	view := RenderWith(a.cache.Model(), a.directory, a.rsvps, actor, now())
	view.ImageSearch = a.search.On()
	view.User.IsSuperAdmin = a.cache.IsSuperAdmin(actor.Email)
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

func refuse(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), access.Status(err))
}

func (a app) commit(w http.ResponseWriter, r *http.Request, actor access.Actor, ops ...store.Op) bool {
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		refuse(w, err)
		return false
	}
	return true
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

func nameOf(directory Directory, email string) string {
	if p, ok := directory.Person(directory.Resolve(email)); ok {
		return p.Name
	}
	return DisplayName(email)
}

func ticketName(directory Directory, t map[string]string) string {
	if t["Email"] != "" {
		if p, ok := directory.Person(directory.Resolve(t["Email"])); ok && p.Name != "" {
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
	var body ticketOrder
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	got, err := a.cache.Model().takeTickets(actor, a.directory, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, got.ops...) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: tickets taken", "actor", actor.Email, "party", got.party.Title, "purchaser", got.purchaser, "sold", got.sold, "waitlisted", got.waitlisted)
	byPurchaser := map[string][]map[string]string{}
	order := []string{}
	for _, cells := range got.added {
		if _, seen := byPurchaser[cells["Purchaser"]]; !seen {
			order = append(order, cells["Purchaser"])
		}
		byPurchaser[cells["Purchaser"]] = append(byPurchaser[cells["Purchaser"]], cells)
	}
	for _, who := range order {
		a.mailTickets(r, got.party, who, byPurchaser[who], actor.Email)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"sold": got.sold, "waitlisted": got.waitlisted})
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func (a app) joinWaitlist(w http.ResponseWriter, r *http.Request) {
	var body waitlistOrder
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	got, err := a.cache.Model().joinWaitlist(actor, a.directory, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, got.ops...) {
		return
	}
	event := "celebrate: joined waitlist"
	if got.changed {
		event = "celebrate: waitlist request changed"
	}
	slog.InfoContext(r.Context(), event, "actor", actor.Email, "party", got.party.Title, "purchaser", got.purchaser, "quantity", body.Quantity)
	a.mailTickets(r, got.party, got.purchaser, []map[string]string{got.cells}, actor.Email)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"sold": 0, "waitlisted": body.Quantity})
}

func (a app) offerTickets(w http.ResponseWriter, r *http.Request) {
	var body offer
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	got, err := a.cache.Model().offerTickets(actor, a.directory, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, got.ops...) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: offered tickets", "actor", actor.Email, "party", got.party.Title, "purchaser", got.ticket.Purchaser, "offered", got.offered, "left", got.left)
	a.mailOffered(r, got.party, got.ticket.Purchaser, got.added, actor.Email)
	w.WriteHeader(http.StatusNoContent)
}

func ticketHolder(t *Ticket) string {
	if t.Email == "" {
		return t.Name
	}
	return t.Email
}

func (a app) removeTicket(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TicketID string `json:"ticketId"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	t, p, ops, err := a.cache.Model().removeTicket(actor, a.directory, body.TicketID)
	if err != nil {
		refuse(w, err)
		return
	}
	who, status := ticketHolder(t), t.Status
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: ticket removed", "actor", actor.Email, "party", p.Title, "who", who, "was", status)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) editTicket(w http.ResponseWriter, r *http.Request) {
	var body ticketEdit
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	t, p, ops, details, err := a.cache.Model().editTicket(actor, a.directory, body)
	if err != nil {
		refuse(w, err)
		return
	}
	who := ticketHolder(t)
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: ticket edited", "actor", actor.Email, "party", p.Title, "who", who, "details", details)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) reassignTicket(w http.ResponseWriter, r *http.Request) {
	var body reassignment
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	t, p, ops, who, err := a.cache.Model().reassignTicket(actor, a.directory, body)
	if err != nil {
		refuse(w, err)
		return
	}
	was := ticketHolder(t)
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: ticket reassigned", "actor", actor.Email, "party", p.Title, "from", was, "to", who)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveParty(w http.ResponseWriter, r *http.Request) {
	var body partyBody
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	saved, err := a.cache.Model().saveParty(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, saved.ops...) {
		return
	}
	action := map[bool]string{true: "add", false: "edit"}[saved.adding]
	slog.InfoContext(r.Context(), "celebrate: saved party", "actor", actor.Email, "action", action, "party", saved.title, "status", saved.status)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": saved.id})
}

func (a app) deleteParty(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	p, ops, err := a.cache.Model().deleteParty(actor, body.ID)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: removed party", "actor", actor.Email, "party", p.Title)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) setFlags(w http.ResponseWriter, r *http.Request) {
	var body partyFlags
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	p, ops, err := a.cache.Model().setFlags(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: set party flags", "actor", actor.Email, "party", p.Title, "tickets", ticketsCell(body.TicketsOpen))
	w.WriteHeader(http.StatusNoContent)
}

func (a app) setStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	p, ops, err := a.cache.Model().setStatus(actor, body.ID, body.Status)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: set party status", "actor", actor.Email, "party", p.Title, "status", body.Status)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveCelebration(w http.ResponseWriter, r *http.Request) {
	var body celebrationForm
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, adding, err := a.cache.Model().saveCelebration(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	action := map[bool]string{true: "add", false: "edit"}[adding]
	slog.InfoContext(r.Context(), "celebrate: saved celebration", "actor", actor.Email, "action", action, "code", strings.TrimSpace(body.Code))
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteCelebration(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, err := a.cache.deleteCelebration(actor, body.Code)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: removed celebration", "actor", actor.Email, "code", body.Code)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveCategory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Original string `json:"original"`
		Title    string `json:"title"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, adding, err := a.cache.Model().saveCategory(actor, body.Original, body.Title)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	action := map[bool]string{true: "add", false: "edit"}[adding]
	slog.InfoContext(r.Context(), "celebrate: saved category", "actor", actor.Email, "action", action, "category", strings.TrimSpace(body.Title))
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteCategory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, err := a.cache.deleteCategory(actor, body.Title)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: removed category", "actor", actor.Email, "category", body.Title)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) reorderCategories(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Titles []string `json:"titles"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, err := a.cache.Model().reorderCategories(actor, body.Titles)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: reordered categories", "actor", actor.Email, "changed", len(ops))
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveSettings(w http.ResponseWriter, r *http.Request) {
	var body Settings
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, err := a.cache.Model().saveSettings(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: changed the settings", "actor", actor.Email)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) invoicesCSV(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	model := a.cache.Model()
	code := r.URL.Query().Get("celebration")
	if code != "" && model.Celebration(code) == nil {
		http.Error(w, "no such celebration", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"invoicing-%s.csv\"", code))
	out := csv.NewWriter(w)
	out.Write(InvoicingColumns)
	for _, l := range model.Invoicing {
		if code != "" && l.Code != code {
			continue
		}
		row := []string{l.Date, l.Party, l.Code, l.Purchaser, l.Guest, l.Action, strconv.Itoa(l.Quantity), PriceCell(l.Cost), l.Invoice, l.InvoiceTo}
		for i, cell := range row {
			row[i] = csvCell(cell)
		}
		out.Write(row)
	}
	out.Flush()
}

func csvCell(s string) string {
	if s == "" || !strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return s
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return s
	}
	return "'" + s
}

func (a app) adminState(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	view := struct {
		Email  string   `json:"email"`
		Admins []string `json:"admins"`
	}{Email: actor.Email, Admins: a.cache.Admins(a.superAdmins())}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode celebrate admin state", "error", err)
	}
}

func (a app) setAdmins(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Admins []string `json:"admins"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, admins, err := a.cache.Model().setAdmins(actor, a.superAdmins(), body.Admins)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "celebrate: set the admin list", "actor", actor.Email, "admins", admins)
	w.WriteHeader(http.StatusNoContent)
}
