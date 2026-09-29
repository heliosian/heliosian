package model

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/claude"
	"heliosian/internal/describe"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
)

const birthdaysShell = "web/birthday/index.html"

var birthdaysPages = []string{
	"/{$}", "/jobs", "/process", "/calendar", "/charities", "/charities/{name}", "/newsletters", "/newsletters/{date}", "/skipped", "/unassigned", "/admin", "/staff/{handle}",
}

type birthdaysApp struct {
	store     *Store
	queue     *store.Queue
	describer *describe.Describer
	mailer    *mail.Mailgun
	base      string
	taken     func(string) bool
}

type charityLookup struct {
	Name         string `json:"name"`
	DonationLink string `json:"donationLink"`
}

type BirthdaysDeps struct {
	Store     *Store
	Queue     *store.Queue
	Describer *describe.Describer
	Mailer    *mail.Mailgun
	Base      string
	About     *sharecard.About
	Taken     func(string) bool
}

func RegisterBirthdays(mux *http.ServeMux, d BirthdaysDeps) {
	a := birthdaysApp{store: d.Store, queue: d.Queue, describer: d.Describer, mailer: d.Mailer, base: d.Base, taken: d.Taken}
	kick := make(chan struct{}, 1)
	d.Queue.OnSwap(func() {
		select {
		case kick <- struct{}{}:
		default:
		}
	})
	go a.inviteLoop(kick)
	go a.remindLoop()
	go a.exportLoop()
	for _, page := range birthdaysPages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.Handle("GET /open/share/about.png", d.About)
	mux.HandleFunc("POST /api/birthday/charity/describe", serve.JSON(a.describeCharity))
	RegisterAdmins(mux, a.store, "birthday", noAdminState)
}

func (a birthdaysApp) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, birthdaysShell)
}

func (a birthdaysApp) describeCharity(r *http.Request, body charityLookup) (describe.Info, error) {
	m := a.store.Model()
	actor := m.actor(r, "birthday")
	if err := m.Birthdays.requireTeam(actor); err != nil {
		return describe.Info{}, err
	}
	name, link := strings.TrimSpace(body.Name), strings.TrimSpace(body.DonationLink)
	if name == "" {
		return describe.Info{}, access.Invalid("give the charity's name first")
	}
	info, err := a.describer.Charity(r.Context(), actor.Email, name, link)
	if errors.Is(err, claude.ErrTooMany) {
		return describe.Info{}, access.Refuse(http.StatusTooManyRequests, "%v", err)
	}
	if errors.Is(err, describe.ErrTooLong) {
		return describe.Info{}, access.Invalid("%v", err)
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "birthday: describe charity", "actor", actor.Email, "name", name, "error", err)
		return describe.Info{}, access.Refuse(http.StatusBadGateway, "could not look this charity up right now")
	}
	if info.Sentence == "" {
		return describe.Info{}, access.Missing("could not find this charity; please enter its information")
	}
	slog.InfoContext(r.Context(), "birthday: described charity", "actor", actor.Email, "name", name, "link", info.DonationLink)
	return info, nil
}
