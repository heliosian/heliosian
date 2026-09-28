package celebrate

import (
	"encoding/csv"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/admins"
	"heliosian/internal/blob"
	"heliosian/internal/imagesearch"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

const (
	shell                 = "web/celebrate/index.html"
	maxTicketsPerPurchase = 20
)

var pages = []string{
	"/{$}", "/p/{pretty}", "/celebrations/{code}", "/my", "/my/{email}", "/hosting", "/admin",
}

type app struct {
	cache     *Cache
	images    blob.Images
	directory func() *who.Model
	search    imagesearch.Search
	mailer    *mail.Mailgun
	rsvps     RSVPLookup
	moved     AddressMoved
	style     *sharecard.Style
}

type Deps struct {
	Cache     *Cache
	Images    blob.Images
	Directory func() *who.Model
	Search    imagesearch.Search
	Mailer    *mail.Mailgun
	RSVPs     RSVPLookup
	Moved     AddressMoved
	Style     *sharecard.Style
}

func Register(mux *http.ServeMux, d Deps) {
	d.Search.UserAgent = "Helios Celebrate image search (+https://celebrate.heliosian.com)"
	a := app{cache: d.Cache, images: d.Images, directory: d.Directory, search: d.Search, mailer: d.Mailer, rsvps: d.RSVPs, moved: d.Moved, style: d.Style}
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.HandleFunc("GET /parties/{id}", a.partyPage)
	mux.HandleFunc("GET /api/celebrate/model", serve.JSON(a.model))
	a.search.Register(mux, "/api/celebrate", a.images.Folder(), imagesearch.Members)
	mux.HandleFunc("GET /open/share/upcoming.png", a.shareUpcoming)
	mux.HandleFunc("GET /open/share/{id}", a.shareCard)
	mux.HandleFunc("POST /api/celebrate/tickets", serve.JSON(a.buyTickets))
	mux.HandleFunc("POST /api/celebrate/waitlist", serve.JSON(a.joinWaitlist))
	mux.HandleFunc("POST /api/celebrate/waitlist/offer", serve.JSON(a.offerTickets))
	mux.HandleFunc("POST /api/celebrate/ticket", serve.JSON(a.editTicket))
	mux.HandleFunc("DELETE /api/celebrate/ticket", serve.JSON(a.removeTicket))
	mux.HandleFunc("POST /api/celebrate/ticket/reassign", serve.JSON(a.reassignTicket))
	mux.HandleFunc("POST /api/celebrate/address", serve.JSON(a.moveAddress))
	mux.HandleFunc("GET /api/celebrate/addresses", serve.JSON(a.addresses))
	mux.HandleFunc("POST /api/celebrate/party", serve.JSON(a.saveParty))
	mux.HandleFunc("DELETE /api/celebrate/party", serve.JSON(a.deleteParty))
	mux.HandleFunc("POST /api/celebrate/party/flags", serve.JSON(a.setFlags))
	mux.HandleFunc("POST /api/celebrate/party/status", serve.JSON(a.setStatus))
	mux.HandleFunc("POST /api/celebrate/celebration", serve.JSON(a.saveCelebration))
	mux.HandleFunc("DELETE /api/celebrate/celebration", serve.JSON(a.deleteCelebration))
	mux.HandleFunc("POST /api/celebrate/category", serve.JSON(a.saveCategory))
	mux.HandleFunc("DELETE /api/celebrate/category", serve.JSON(a.deleteCategory))
	mux.HandleFunc("POST /api/celebrate/categories/order", serve.JSON(a.reorderCategories))
	mux.HandleFunc("POST /api/celebrate/settings", serve.JSON(a.saveSettings))
	mux.HandleFunc("GET /api/celebrate/invoices.csv", a.invoicesCSV)
	admins.Register(mux, a.cache.List, a.actor, func(*http.Request, access.Actor) map[string]any { return map[string]any{} })
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

func (a app) partyPage(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("id")
	if p := a.cache.Model().Party(key); p != nil && p.ID != key {
		http.Redirect(w, r, "/parties/"+url.PathEscape(p.ID), http.StatusMovedPermanently)
		return
	}
	serve.File(w, r, shell)
}

func (a app) actor(r *http.Request) access.Actor {
	return a.directory().Actor(r, a.cache.Held)
}

var now = func() time.Time {
	return time.Now().In(when.Location)
}

func today() string {
	return now().Format(DateFormat)
}

func stamp() string {
	return now().Format(DateTimeFormat)
}

func (a app) model(r *http.Request, _ serve.None) (View, error) {
	actor := a.actor(r)
	view := RenderWith(a.cache.Model(), a.directory(), a.rsvps, actor, now())
	view.ImageSearch = a.search.On()
	return view, nil
}

func nameOf(directory *who.Model, email string) string {
	if p := directory.Person(directory.Resolve(email)); p != nil {
		return p.FullName
	}
	return DisplayName(email)
}

func ticketName(directory *who.Model, t map[string]string) string {
	if t["Email"] != "" {
		if p := directory.Person(directory.Resolve(t["Email"])); p != nil && p.FullName != "" {
			return p.FullName
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

func (a app) buyTickets(r *http.Request, body ticketOrder) (map[string]int, error) {
	actor := a.actor(r)
	got, err := a.cache.Model().takeTickets(actor, a.directory(), body)
	if err != nil {
		return nil, err
	}
	if err := a.cache.Commit(r.Context(), actor, got.ops...); err != nil {
		return nil, err
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
	return map[string]int{"sold": got.sold, "waitlisted": got.waitlisted}, nil
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func (a app) joinWaitlist(r *http.Request, body waitlistOrder) (map[string]int, error) {
	actor := a.actor(r)
	got, err := a.cache.Model().joinWaitlist(actor, a.directory(), body)
	if err != nil {
		return nil, err
	}
	if err := a.cache.Commit(r.Context(), actor, got.ops...); err != nil {
		return nil, err
	}
	event := "celebrate: joined waitlist"
	if got.changed {
		event = "celebrate: waitlist request changed"
	}
	slog.InfoContext(r.Context(), event, "actor", actor.Email, "party", got.party.Title, "purchaser", got.purchaser, "quantity", body.Quantity)
	a.mailTickets(r, got.party, got.purchaser, []map[string]string{got.cells}, actor.Email)
	return map[string]int{"sold": 0, "waitlisted": body.Quantity}, nil
}

func (a app) offerTickets(r *http.Request, body offer) (serve.None, error) {
	actor := a.actor(r)
	got, err := a.cache.Model().offerTickets(actor, a.directory(), body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, got.ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: offered tickets", "actor", actor.Email, "party", got.party.Title, "purchaser", got.ticket.Purchaser, "offered", got.offered, "left", got.left)
	a.mailOffered(r, got.party, got.ticket.Purchaser, got.added, actor.Email)
	return serve.None{}, nil
}

func ticketHolder(t *Ticket) string {
	if t.Email == "" {
		return t.Name
	}
	return t.Email
}

type ticketRef struct {
	TicketID string `json:"ticketId"`
}

func (a app) removeTicket(r *http.Request, body ticketRef) (serve.None, error) {
	actor := a.actor(r)
	t, p, ops, err := a.cache.Model().removeTicket(actor, body.TicketID)
	if err != nil {
		return serve.None{}, err
	}
	who, status := ticketHolder(t), t.Status
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: ticket removed", "actor", actor.Email, "party", p.Title, "who", who, "was", status)
	return serve.None{}, nil
}

func (a app) editTicket(r *http.Request, body ticketEdit) (serve.None, error) {
	actor := a.actor(r)
	t, p, ops, details, err := a.cache.Model().editTicket(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	who := ticketHolder(t)
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: ticket edited", "actor", actor.Email, "party", p.Title, "who", who, "details", details)
	return serve.None{}, nil
}

func (a app) reassignTicket(r *http.Request, body reassignment) (serve.None, error) {
	actor := a.actor(r)
	t, p, ops, who, err := a.cache.Model().reassignTicket(actor, a.directory(), body)
	if err != nil {
		return serve.None{}, err
	}
	was := ticketHolder(t)
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: ticket reassigned", "actor", actor.Email, "party", p.Title, "from", was, "to", who)
	return serve.None{}, nil
}

func (a app) saveParty(r *http.Request, body partyBody) (map[string]string, error) {
	actor := a.actor(r)
	saved, err := a.cache.Model().saveParty(actor, body)
	if err != nil {
		return nil, err
	}
	if err := a.cache.Commit(r.Context(), actor, saved.ops...); err != nil {
		return nil, err
	}
	action := map[bool]string{true: "add", false: "edit"}[saved.adding]
	slog.InfoContext(r.Context(), "celebrate: saved party", "actor", actor.Email, "action", action, "party", saved.title, "status", saved.status)
	return map[string]string{"id": saved.id}, nil
}

type partyRef struct {
	ID string `json:"id"`
}

func (a app) deleteParty(r *http.Request, body partyRef) (serve.None, error) {
	actor := a.actor(r)
	p, ops, err := a.cache.Model().deleteParty(actor, body.ID)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: removed party", "actor", actor.Email, "party", p.Title)
	return serve.None{}, nil
}

func (a app) setFlags(r *http.Request, body partyFlags) (serve.None, error) {
	actor := a.actor(r)
	p, ops, err := a.cache.Model().setFlags(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: set party flags", "actor", actor.Email, "party", p.Title, "tickets", ticketsCell(body.TicketsOpen))
	return serve.None{}, nil
}

type statusChange struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func (a app) setStatus(r *http.Request, body statusChange) (serve.None, error) {
	actor := a.actor(r)
	p, ops, err := a.cache.Model().setStatus(actor, body.ID, body.Status)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: set party status", "actor", actor.Email, "party", p.Title, "status", body.Status)
	return serve.None{}, nil
}

func (a app) saveCelebration(r *http.Request, body celebrationForm) (serve.None, error) {
	actor := a.actor(r)
	ops, adding, err := a.cache.Model().saveCelebration(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	action := map[bool]string{true: "add", false: "edit"}[adding]
	slog.InfoContext(r.Context(), "celebrate: saved celebration", "actor", actor.Email, "action", action, "code", strings.TrimSpace(body.Code))
	return serve.None{}, nil
}

type celebrationRef struct {
	ID string `json:"id"`
}

func (a app) deleteCelebration(r *http.Request, body celebrationRef) (serve.None, error) {
	actor := a.actor(r)
	c, ops, err := a.cache.deleteCelebration(actor, body.ID)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: removed celebration", "actor", actor.Email, "code", c.Code)
	return serve.None{}, nil
}

type categoryForm struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func (a app) saveCategory(r *http.Request, body categoryForm) (serve.None, error) {
	actor := a.actor(r)
	ops, adding, err := a.cache.Model().saveCategory(actor, body.ID, body.Title)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	action := map[bool]string{true: "add", false: "edit"}[adding]
	slog.InfoContext(r.Context(), "celebrate: saved category", "actor", actor.Email, "action", action, "category", strings.TrimSpace(body.Title))
	return serve.None{}, nil
}

type categoryRef struct {
	ID string `json:"id"`
}

func (a app) deleteCategory(r *http.Request, body categoryRef) (serve.None, error) {
	actor := a.actor(r)
	c, ops, err := a.cache.deleteCategory(actor, body.ID)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: removed category", "actor", actor.Email, "category", c.Title)
	return serve.None{}, nil
}

type categoryIDs struct {
	IDs []string `json:"ids"`
}

func (a app) reorderCategories(r *http.Request, body categoryIDs) (serve.None, error) {
	actor := a.actor(r)
	ops, err := a.cache.Model().reorderCategories(actor, body.IDs)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: reordered categories", "actor", actor.Email, "changed", len(ops))
	return serve.None{}, nil
}

func (a app) saveSettings(r *http.Request, body Settings) (serve.None, error) {
	actor := a.actor(r)
	ops, err := a.cache.Model().saveSettings(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: changed the settings", "actor", actor.Email)
	return serve.None{}, nil
}

func (a app) invoicesCSV(w http.ResponseWriter, r *http.Request) {
	if err := require(a.actor(r), SeeAll); err != nil {
		serve.Error(w, r, err)
		return
	}
	model := a.cache.Model()
	key := r.URL.Query().Get("celebration")
	code := ""
	if key != "" {
		c := model.CelebrationByID(key)
		if c == nil {
			http.Error(w, "no such celebration", http.StatusNotFound)
			return
		}
		code = c.Code
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"invoicing-%s.csv\"", code))
	out := csv.NewWriter(w)
	out.Write(invoiceCSVColumns)
	for _, l := range model.Invoicing {
		if key != "" && l.Celebration != key {
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

var invoiceCSVColumns = []string{"Date", "Party Title", "Event Code", "Purchaser Email", "Guest Name", "Action", "Quantity", "Cost", "Invoice", "Invoice To"}

func csvCell(s string) string {
	if s == "" || !strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return s
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return s
	}
	return "'" + s
}
