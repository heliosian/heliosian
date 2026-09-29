package model

import (
	"context"
	"encoding/csv"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/imagesearch"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
)

const (
	partiesShell          = "web/celebrate/index.html"
	maxTicketsPerPurchase = 20
)

var partiesPages = []string{
	"/{$}", "/p/{pretty}", "/celebrations/{code}", "/my", "/my/{email}", "/hosting", "/admin",
}

type partiesApp struct {
	store    *Store
	images   blob.Images
	calendar calendarApp
	search   imagesearch.Search
	mailer   *mail.Mailgun
	style    *sharecard.Style
}

type PartiesDeps struct {
	Store    *Store
	Images   blob.Images
	Calendar CalendarHooks
	Search   imagesearch.Search
	Mailer   *mail.Mailgun
	Style    *sharecard.Style
}

func RegisterParties(mux *http.ServeMux, d PartiesDeps) {
	d.Search.UserAgent = "Helios Celebrate image search (+https://celebrate.heliosian.com)"
	a := partiesApp{store: d.Store, images: d.Images, calendar: d.Calendar.app, search: d.Search, mailer: d.Mailer, style: d.Style}
	for _, page := range partiesPages {
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
	RegisterAdmins(mux, a.store, "celebrate", noAdminState)
}

func (a partiesApp) parties() *Parties {
	return a.store.Model().Parties
}

func (a partiesApp) directory() *Directory {
	return a.store.Model().Directory
}

func (a partiesApp) commit(ctx context.Context, actor access.Actor, ops ...store.Op) error {
	return a.store.Commit(ctx, actor, partiesAppName, ops...)
}

func (a partiesApp) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, partiesShell)
}

func (a partiesApp) partyPage(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("id")
	if p := a.parties().Party(key); p != nil && p.ID != key {
		http.Redirect(w, r, "/parties/"+url.PathEscape(p.ID), http.StatusMovedPermanently)
		return
	}
	serve.File(w, r, partiesShell)
}

func (a partiesApp) actor(r *http.Request) access.Actor {
	return a.store.Model().actor(r, "celebrate")
}

func stamp() string {
	return now().Format(DateTimeFormat)
}

func (a partiesApp) model(r *http.Request, _ serve.None) (PartiesView, error) {
	m := a.store.Model()
	actor := m.actor(r, "celebrate")
	view := RenderParties(m.Parties, m.Directory, a.calendar.at(m).linkedRSVPs, actor, now())
	view.ImageSearch = a.search.On()
	return view, nil
}

func nameOf(directory *Directory, email string) string {
	if p := directory.Person(directory.Resolve(email)); p != nil {
		return p.FullName
	}
	return cells.DisplayName(email)
}

func ticketName(directory *Directory, t map[string]string) string {
	if t["Email"] != "" {
		if p := directory.Person(directory.Resolve(t["Email"])); p != nil && p.FullName != "" {
			return p.FullName
		}
	}
	if t["Name"] != "" {
		return t["Name"]
	}
	return cells.DisplayName(t["Email"])
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

func (a partiesApp) buyTickets(r *http.Request, body ticketOrder) (map[string]int, error) {
	actor := a.actor(r)
	got, err := a.parties().takeTickets(actor, a.directory(), body)
	if err != nil {
		return nil, err
	}
	if err := a.commit(r.Context(), actor, got.ops...); err != nil {
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

func (a partiesApp) joinWaitlist(r *http.Request, body waitlistOrder) (map[string]int, error) {
	actor := a.actor(r)
	got, err := a.parties().joinWaitlist(actor, a.directory(), body)
	if err != nil {
		return nil, err
	}
	if err := a.commit(r.Context(), actor, got.ops...); err != nil {
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

func (a partiesApp) offerTickets(r *http.Request, body offer) (serve.None, error) {
	actor := a.actor(r)
	got, err := a.parties().offerTickets(actor, a.directory(), body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r.Context(), actor, got.ops...); err != nil {
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

func (a partiesApp) removeTicket(r *http.Request, body ticketRef) (serve.None, error) {
	actor := a.actor(r)
	t, p, ops, err := a.parties().removeTicket(actor, body.TicketID)
	if err != nil {
		return serve.None{}, err
	}
	who, status := ticketHolder(t), t.Status
	if err := a.commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: ticket removed", "actor", actor.Email, "party", p.Title, "who", who, "was", status)
	return serve.None{}, nil
}

func (a partiesApp) editTicket(r *http.Request, body ticketEdit) (serve.None, error) {
	actor := a.actor(r)
	t, p, ops, details, err := a.parties().editTicket(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	who := ticketHolder(t)
	if err := a.commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: ticket edited", "actor", actor.Email, "party", p.Title, "who", who, "details", details)
	return serve.None{}, nil
}

func (a partiesApp) reassignTicket(r *http.Request, body reassignment) (serve.None, error) {
	actor := a.actor(r)
	t, p, ops, who, err := a.parties().reassignTicket(actor, a.directory(), body)
	if err != nil {
		return serve.None{}, err
	}
	was := ticketHolder(t)
	if err := a.commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: ticket reassigned", "actor", actor.Email, "party", p.Title, "from", was, "to", who)
	return serve.None{}, nil
}

func (a partiesApp) saveParty(r *http.Request, body partyBody) (map[string]string, error) {
	actor := a.actor(r)
	saved, err := a.parties().saveParty(actor, body)
	if err != nil {
		return nil, err
	}
	if err := a.commit(r.Context(), actor, saved.ops...); err != nil {
		return nil, err
	}
	action := map[bool]string{true: "add", false: "edit"}[saved.adding]
	slog.InfoContext(r.Context(), "celebrate: saved party", "actor", actor.Email, "action", action, "party", saved.title, "status", saved.status)
	return map[string]string{"id": saved.id}, nil
}

type partyRef struct {
	ID string `json:"id"`
}

func (a partiesApp) deleteParty(r *http.Request, body partyRef) (serve.None, error) {
	actor := a.actor(r)
	p, ops, err := a.parties().deleteParty(actor, body.ID)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: removed party", "actor", actor.Email, "party", p.Title)
	return serve.None{}, nil
}

func (a partiesApp) setFlags(r *http.Request, body partyFlags) (serve.None, error) {
	actor := a.actor(r)
	p, ops, err := a.parties().setFlags(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: set party flags", "actor", actor.Email, "party", p.Title, "tickets", ticketsCell(body.TicketsOpen))
	return serve.None{}, nil
}

type statusChange struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func (a partiesApp) setStatus(r *http.Request, body statusChange) (serve.None, error) {
	actor := a.actor(r)
	p, ops, err := a.parties().setStatus(actor, body.ID, body.Status)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: set party status", "actor", actor.Email, "party", p.Title, "status", body.Status)
	return serve.None{}, nil
}

func (a partiesApp) saveCelebration(r *http.Request, body celebrationForm) (serve.None, error) {
	actor := a.actor(r)
	ops, adding, err := a.parties().saveCelebration(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	action := map[bool]string{true: "add", false: "edit"}[adding]
	slog.InfoContext(r.Context(), "celebrate: saved celebration", "actor", actor.Email, "action", action, "code", strings.TrimSpace(body.Code))
	return serve.None{}, nil
}

type celebrationRef struct {
	ID string `json:"id"`
}

func (a partiesApp) deleteCelebration(r *http.Request, body celebrationRef) (serve.None, error) {
	actor := a.actor(r)
	c, ops, err := a.store.deleteCelebration(actor, body.ID)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: removed celebration", "actor", actor.Email, "code", c.Code)
	return serve.None{}, nil
}

type categoryForm struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func (a partiesApp) saveCategory(r *http.Request, body categoryForm) (serve.None, error) {
	actor := a.actor(r)
	ops, adding, err := a.parties().saveCategory(actor, body.ID, body.Title)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	action := map[bool]string{true: "add", false: "edit"}[adding]
	slog.InfoContext(r.Context(), "celebrate: saved category", "actor", actor.Email, "action", action, "category", strings.TrimSpace(body.Title))
	return serve.None{}, nil
}

type categoryRef struct {
	ID string `json:"id"`
}

func (a partiesApp) deleteCategory(r *http.Request, body categoryRef) (serve.None, error) {
	actor := a.actor(r)
	c, ops, err := a.store.deletePartyCategory(actor, body.ID)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: removed category", "actor", actor.Email, "category", c.Title)
	return serve.None{}, nil
}

type categoryIDs struct {
	IDs []string `json:"ids"`
}

func (a partiesApp) reorderCategories(r *http.Request, body categoryIDs) (serve.None, error) {
	actor := a.actor(r)
	ops, err := a.parties().reorderCategories(actor, body.IDs)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: reordered categories", "actor", actor.Email, "changed", len(ops))
	return serve.None{}, nil
}

func (a partiesApp) saveSettings(r *http.Request, body PartiesSettings) (serve.None, error) {
	actor := a.actor(r)
	ops, err := a.parties().saveSettings(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "celebrate: changed the settings", "actor", actor.Email)
	return serve.None{}, nil
}

func (a partiesApp) invoicesCSV(w http.ResponseWriter, r *http.Request) {
	if err := require(a.actor(r), SeeAllParties); err != nil {
		serve.Error(w, r, err)
		return
	}
	m := a.parties()
	key := r.URL.Query().Get("celebration")
	code := ""
	if key != "" {
		c := m.CelebrationByID(key)
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
	for _, l := range m.Invoicing {
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
