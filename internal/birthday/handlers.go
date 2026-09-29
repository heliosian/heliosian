package birthday

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/claude"
	"heliosian/internal/describe"
	"heliosian/internal/mail"
	"heliosian/internal/model"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
)

const shell = "web/birthday/index.html"

var pages = []string{
	"/{$}", "/jobs", "/process", "/calendar", "/charities", "/charities/{name}", "/newsletters", "/newsletters/{date}", "/skipped", "/unassigned", "/admin", "/staff/{handle}",
}

type app struct {
	cache     *Cache
	queue     *store.Queue
	directory func() *model.Directory
	describer *describe.Describer
	mailer    *mail.Mailgun
	base      string
	taken     func(string) bool
}

type charityLookup struct {
	Name         string `json:"name"`
	DonationLink string `json:"donationLink"`
}

type Deps struct {
	Cache     *Cache
	Queue     *store.Queue
	Directory func() *model.Directory
	Describer *describe.Describer
	Mailer    *mail.Mailgun
	Base      string
	About     *sharecard.About
	Taken     func(string) bool
}

func Register(mux *http.ServeMux, d Deps) {
	a := app{cache: d.Cache, queue: d.Queue, directory: d.Directory, describer: d.Describer, mailer: d.Mailer, base: d.Base, taken: d.Taken}
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
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.Handle("GET /open/share/about.png", d.About)
	mux.HandleFunc("POST /api/birthday/charity/describe", serve.JSON(a.describeCharity))
	model.RegisterAdmins(mux, a.cache.AdminList, a.actor, func(*http.Request, access.Actor) map[string]any { return map[string]any{} })
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

func (a app) actor(r *http.Request) access.Actor {
	return a.directory().Actor(r, a.cache.Held)
}

var now = func() time.Time {
	return time.Now().In(model.Location)
}

func (a app) describeCharity(r *http.Request, body charityLookup) (describe.Info, error) {
	actor := a.actor(r)
	if err := a.cache.Model().requireTeam(actor); err != nil {
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
