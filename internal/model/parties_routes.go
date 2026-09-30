package model

import (
	"encoding/csv"
	"fmt"
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

type PartiesHooks struct {
	app partiesApp
}

func RegisterParties(mux *http.ServeMux, d PartiesDeps) PartiesHooks {
	d.Search.UserAgent = "Helios Celebrate image search (+https://celebrate.heliosian.com)"
	a := partiesApp{store: d.Store, images: d.Images, calendar: d.Calendar.app, search: d.Search, mailer: d.Mailer, style: d.Style}
	for _, page := range partiesPages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.HandleFunc("GET /parties/{id}", a.partyPage)
	a.search.Register(mux, "/api/celebrate", a.images.Folder(), imagesearch.Members)
	mux.HandleFunc("GET /open/share/upcoming.png", a.shareUpcoming)
	mux.HandleFunc("GET /open/share/{id}", a.shareCard)
	mux.HandleFunc("GET /api/celebrate/invoices.csv", a.invoicesCSV)
	RegisterAdmins(mux, a.store, "celebrate", noAdminState)
	return PartiesHooks{app: a}
}

func (a partiesApp) parties() *Parties {
	return a.store.Model().Parties
}

func (a partiesApp) directory() *Directory {
	return a.store.Model().Directory
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

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func ticketHolder(t *Ticket) string {
	if t.Email == "" {
		return t.Name
	}
	return t.Email
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
