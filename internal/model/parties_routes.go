package model

import (
	"net/http"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/imagesearch"
	"heliosian/internal/mail"
)

const maxTicketsPerPurchase = 20

type partiesApp struct {
	store    *Store
	images   blob.Images
	calendar calendarApp
	search   imagesearch.Search
	mailer   *mail.Mailgun
}

type PartiesDeps struct {
	Store    *Store
	Images   blob.Images
	Calendar CalendarHooks
	Search   imagesearch.Search
	Mailer   *mail.Mailgun
}

type PartiesHooks struct {
	app partiesApp
}

func RegisterParties(mux *http.ServeMux, d PartiesDeps) PartiesHooks {
	d.Search.UserAgent = "Helios Celebrate image search (+https://celebrate.heliosian.com)"
	a := partiesApp{store: d.Store, images: d.Images, calendar: d.Calendar.app, search: d.Search, mailer: d.Mailer}
	a.search.Register(mux, "/api/celebrate", a.images.Folder(), imagesearch.Members)
	return PartiesHooks{app: a}
}

func (a partiesApp) parties() *Parties {
	return a.store.Model().Parties
}

func (a partiesApp) directory() *Directory {
	return a.store.Model().Directory
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

func (p *Party) PurchaserName(directory *Directory, email string) string {
	if directory.Person(directory.Resolve(email)) == nil {
		for _, t := range p.Tickets {
			if t.Email == email && t.Name != "" {
				return t.Name
			}
		}
	}
	return nameOf(directory, email)
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
