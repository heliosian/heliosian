package birthday

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/admins"
	"heliosian/internal/claude"
	"heliosian/internal/describe"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

const shell = "web/birthday/index.html"

var pages = []string{
	"/{$}", "/jobs", "/process", "/calendar", "/charities", "/charities/{name}", "/newsletters", "/newsletters/{date}", "/skipped", "/unassigned", "/admin", "/staff/{handle}",
}

type app struct {
	cache     *Cache
	directory func() *who.Model
	describer *describe.Describer
	mailer    *mail.Mailgun
	base      string
	joinHome  func(ctx context.Context, email string) error
}

type emailRef struct {
	Email string `json:"email"`
}

type assignment struct {
	Email      string `json:"email"`
	AssignedTo string `json:"assignedTo"`
}

type contact struct {
	Email     string `json:"email"`
	Contacted bool   `json:"contacted"`
}

type donationEdit struct {
	Email   string `json:"email"`
	Charity string `json:"charity"`
	Note    string `json:"note"`
}

type usedMark struct {
	Email string `json:"email"`
	Used  bool   `json:"used"`
}

type birthdayEdit struct {
	Email    string `json:"email"`
	Birthday string `json:"birthday"`
	Override string `json:"override"`
}

type participationEdit struct {
	Email string `json:"email"`
	Level string `json:"level"`
	Note  string `json:"note"`
}

type noteEdit struct {
	Email string `json:"email"`
	Note  string `json:"note"`
}

type charityLookup struct {
	Name         string `json:"name"`
	DonationLink string `json:"donationLink"`
}

type charityRef struct {
	Name string `json:"name"`
}

type dateRef struct {
	Date string `json:"date"`
}

type dateMove struct {
	Original string `json:"original"`
	Date     string `json:"date"`
}

type dateRun struct {
	Weekday int    `json:"weekday"`
	From    string `json:"from"`
	To      string `json:"to"`
}

type Deps struct {
	Cache     *Cache
	Directory func() *who.Model
	Describer *describe.Describer
	Mailer    *mail.Mailgun
	Base      string
	JoinHome  func(ctx context.Context, email string) error
	About     *sharecard.About
}

func Register(mux *http.ServeMux, d Deps) {
	a := app{cache: d.Cache, directory: d.Directory, describer: d.Describer, mailer: d.Mailer, base: d.Base, joinHome: d.JoinHome}
	go a.remindLoop()
	go a.exportLoop()
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.Handle("GET /open/share/about.png", d.About)
	mux.HandleFunc("GET /api/birthday/model", serve.JSON(a.model))
	mux.HandleFunc("POST /api/birthday/assign", serve.JSON(a.assign))
	mux.HandleFunc("DELETE /api/birthday/assign", serve.JSON(a.unassign))
	mux.HandleFunc("POST /api/birthday/outreach", serve.JSON(a.outreach))
	mux.HandleFunc("POST /api/birthday/donation", serve.JSON(a.saveDonation))
	mux.HandleFunc("DELETE /api/birthday/donation", serve.JSON(a.deleteDonation))
	mux.HandleFunc("POST /api/birthday/used", serve.JSON(a.used))
	mux.HandleFunc("POST /api/birthday/birthday", serve.JSON(a.saveBirthday))
	mux.HandleFunc("DELETE /api/birthday/birthday", serve.JSON(a.deleteBirthday))
	mux.HandleFunc("POST /api/birthday/participation", serve.JSON(a.saveParticipation))
	mux.HandleFunc("DELETE /api/birthday/participation", serve.JSON(a.deleteParticipation))
	mux.HandleFunc("POST /api/birthday/note", serve.JSON(a.addNote))
	mux.HandleFunc("DELETE /api/birthday/note", serve.JSON(a.deleteNote))
	mux.HandleFunc("POST /api/birthday/charity", serve.JSON(a.saveCharity))
	mux.HandleFunc("DELETE /api/birthday/charity", serve.JSON(a.deleteCharity))
	mux.HandleFunc("POST /api/birthday/charity/describe", serve.JSON(a.describeCharity))
	mux.HandleFunc("POST /api/birthday/newsletter-date", serve.JSON(a.addNewsletterDate))
	mux.HandleFunc("PUT /api/birthday/newsletter-date", serve.JSON(a.changeNewsletterDate))
	mux.HandleFunc("DELETE /api/birthday/newsletter-date", serve.JSON(a.deleteNewsletterDate))
	mux.HandleFunc("POST /api/birthday/newsletter-dates/clear-future", serve.JSON(a.clearFutureNewsletterDates))
	mux.HandleFunc("POST /api/birthday/newsletter-dates/create", serve.JSON(a.createNewsletterDates))
	mux.HandleFunc("POST /api/birthday/newsletter/share", serve.JSON(a.shareIssue))
	mux.HandleFunc("POST /api/birthday/settings", serve.JSON(a.saveSettings))
	admins.Register(mux, a.cache.List, a.actor, a.adminState)
	mux.HandleFunc("POST /api/admin/resend-invites", serve.JSON(a.resendInvites))
	mux.HandleFunc("POST /api/birthday/team/join", serve.JSON(a.joinTeam))
	mux.HandleFunc("POST /api/admin/team", serve.JSON(a.addTeamMember))
	mux.HandleFunc("DELETE /api/admin/team", serve.JSON(a.removeTeamMember))
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

func (a app) actor(r *http.Request) access.Actor {
	return a.directory().Actor(r, a.cache.Held)
}

func (a app) requireAdmin(r *http.Request) (access.Actor, error) {
	actor := a.actor(r)
	if err := requireAdmin(actor); err != nil {
		return access.Actor{}, err
	}
	return actor, nil
}

func (a app) requireTeam(r *http.Request) (access.Actor, error) {
	actor := a.actor(r)
	if err := a.cache.Model().requireTeam(actor); err != nil {
		return access.Actor{}, err
	}
	return actor, nil
}

var now = func() time.Time {
	return time.Now().In(when.Location)
}

func today() string {
	return now().Format(DateFormat)
}

func (m *Model) year() string {
	month, day, _ := ParseMonthDay(m.Settings.YearStart)
	return YearContaining(now(), month, day).Label
}

func (a app) model(r *http.Request, _ serve.None) (View, error) {
	actor := a.actor(r)
	return Render(a.cache.Model(), a.directory, actor, now()), nil
}

func (a app) commit(r *http.Request, actor access.Actor, ops ...store.Op) error {
	before := a.askDays(a.cache.Model())
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return err
	}
	a.mailMovedAskDays(r, before, a.askDays(a.cache.Model()))
	return nil
}

func (a app) assign(r *http.Request, body assignment) (serve.None, error) {
	actor := a.actor(r)
	ops, t, err := a.cache.assign(actor, body.Email, body.AssignedTo)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: assigned", "actor", actor.Email, "email", t.email, "to", t.to, "year", t.year)
	a.mailAssignment(r, t.email, t.to)
	return serve.None{}, nil
}

func (a app) unassign(r *http.Request, body emailRef) (serve.None, error) {
	actor := a.actor(r)
	ops, t, err := a.cache.Model().unassign(actor, body.Email)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: unassigned", "actor", actor.Email, "email", t.email, "year", t.year)
	return serve.None{}, nil
}

func (a app) outreach(r *http.Request, body contact) (serve.None, error) {
	actor := a.actor(r)
	ops, t, err := a.cache.Model().outreach(actor, body.Email, body.Contacted)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	if !body.Contacted {
		slog.InfoContext(r.Context(), "birthday: outreach undone", "actor", actor.Email, "email", t.email, "year", t.year)
		return serve.None{}, nil
	}
	slog.InfoContext(r.Context(), "birthday: contacted", "actor", actor.Email, "email", t.email, "year", t.year)
	return serve.None{}, nil
}

func (a app) saveDonation(r *http.Request, body donationEdit) (serve.None, error) {
	actor := a.actor(r)
	ops, t, err := a.cache.Model().saveDonation(actor, body.Email, body.Charity, body.Note)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: saved donation", "actor", actor.Email, "email", t.email, "charity", t.charity, "year", t.year)
	return serve.None{}, nil
}

func (a app) deleteDonation(r *http.Request, body emailRef) (serve.None, error) {
	actor := a.actor(r)
	ops, t, err := a.cache.Model().deleteDonation(actor, body.Email)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: removed donation", "actor", actor.Email, "email", t.email, "year", t.year)
	return serve.None{}, nil
}

func (a app) used(r *http.Request, body usedMark) (serve.None, error) {
	actor := a.actor(r)
	ops, t, err := a.cache.Model().markUsed(actor, body.Email, body.Used)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: marked donation", "actor", actor.Email, "used", body.Used, "email", t.email, "year", t.year)
	return serve.None{}, nil
}

func (a app) saveBirthday(r *http.Request, body birthdayEdit) (serve.None, error) {
	actor := a.actor(r)
	ops, email, err := a.cache.Model().saveBirthday(actor, body.Email, body.Birthday, body.Override)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: saved birthday", "actor", actor.Email, "email", email, "birthday", strings.TrimSpace(body.Birthday), "override", strings.TrimSpace(body.Override))
	return serve.None{}, nil
}

func (a app) deleteBirthday(r *http.Request, body emailRef) (serve.None, error) {
	actor := a.actor(r)
	ops, email, err := a.cache.deleteBirthday(actor, body.Email)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: removed birthday", "actor", actor.Email, "email", email)
	return serve.None{}, nil
}

func (a app) saveParticipation(r *http.Request, body participationEdit) (serve.None, error) {
	actor := a.actor(r)
	ops, email, err := a.cache.Model().saveParticipation(actor, body.Email, body.Level, body.Note)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: saved participation", "actor", actor.Email, "email", email, "level", body.Level)
	return serve.None{}, nil
}

func (a app) deleteParticipation(r *http.Request, body emailRef) (serve.None, error) {
	actor := a.actor(r)
	ops, email, err := a.cache.Model().deleteParticipation(actor, body.Email)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: removed participation", "actor", actor.Email, "email", email)
	return serve.None{}, nil
}

func (a app) addNote(r *http.Request, body noteEdit) (serve.None, error) {
	actor := a.actor(r)
	ops, email, err := a.cache.Model().addNote(actor, body.Email, body.Note)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: added note", "actor", actor.Email, "email", email)
	return serve.None{}, nil
}

func (a app) deleteNote(r *http.Request, body Note) (serve.None, error) {
	actor := a.actor(r)
	ops, email, err := a.cache.Model().deleteNote(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: removed note", "actor", actor.Email, "email", email)
	return serve.None{}, nil
}

func (a app) saveCharity(r *http.Request, body charityEdit) (serve.None, error) {
	actor := a.actor(r)
	ops, saved, adding, err := a.cache.Model().saveCharity(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: saved charity", "actor", actor.Email, "adding", adding, "charity", saved.Name, "allowed", saved.Allowed)
	return serve.None{}, nil
}

func (a app) describeCharity(r *http.Request, body charityLookup) (describe.Info, error) {
	actor, err := a.requireTeam(r)
	if err != nil {
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

func (a app) deleteCharity(r *http.Request, body charityRef) (serve.None, error) {
	actor := a.actor(r)
	ops, err := a.cache.deleteCharity(actor, body.Name)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: removed charity", "actor", actor.Email, "charity", body.Name)
	return serve.None{}, nil
}

func (a app) addNewsletterDate(r *http.Request, body dateRef) (serve.None, error) {
	actor := a.actor(r)
	ops, date, err := a.cache.Model().addNewsletterDate(actor, body.Date)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: added newsletter date", "actor", actor.Email, "date", date)
	return serve.None{}, nil
}

func (a app) changeNewsletterDate(r *http.Request, body dateMove) (serve.None, error) {
	actor := a.actor(r)
	ops, err := a.cache.Model().changeNewsletterDate(actor, body.Original, body.Date)
	if err != nil {
		return serve.None{}, err
	}
	if len(ops) == 0 {
		return serve.None{}, nil
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: moved newsletter date", "actor", actor.Email, "from", strings.TrimSpace(body.Original), "to", strings.TrimSpace(body.Date))
	return serve.None{}, nil
}

func (a app) deleteNewsletterDate(r *http.Request, body dateRef) (serve.None, error) {
	actor := a.actor(r)
	ops, date, err := deleteNewsletterDate(actor, body.Date)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: removed newsletter date", "actor", actor.Email, "date", date)
	return serve.None{}, nil
}

func (a app) createNewsletterDates(r *http.Request, body dateRun) (map[string]int, error) {
	actor := a.actor(r)
	ops, added, err := a.cache.Model().createNewsletterDates(actor, body.Weekday, body.From, body.To)
	if err != nil {
		return nil, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return nil, err
	}
	slog.InfoContext(r.Context(), "birthday: created newsletter dates", "actor", actor.Email, "count", len(added), "from", added[0], "to", added[len(added)-1])
	return map[string]int{"added": len(added)}, nil
}

func (a app) clearFutureNewsletterDates(r *http.Request, _ serve.None) (serve.None, error) {
	actor := a.actor(r)
	day := today()
	ops, err := a.cache.Model().clearFutureNewsletterDates(actor, day)
	if err != nil {
		return serve.None{}, err
	}
	if len(ops) == 0 {
		return serve.None{}, nil
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: cleared future newsletter dates", "actor", actor.Email, "count", len(ops), "from", day)
	return serve.None{}, nil
}

func (a app) saveSettings(r *http.Request, body Settings) (serve.None, error) {
	actor := a.actor(r)
	ops, err := saveSettings(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: changed the settings", "actor", actor.Email)
	return serve.None{}, nil
}

func (a app) adminState(*http.Request, access.Actor) map[string]any {
	model := a.cache.Model()
	team := []TeamView{}
	directory := a.directory()
	v := viewer{directory: directory}
	for _, m := range model.Team {
		p, _ := v.person(m.Email)
		team = append(team, TeamView{Email: m.Email, Name: p.Name, Role: m.Role})
	}
	people := []Person{}
	for _, p := range directory.Listed() {
		people = append(people, personView(p))
	}
	return map[string]any{"team": team, "roles": Roles, "people": people}
}

func (a app) resendInvites(r *http.Request, _ serve.None) (map[string]int, error) {
	actor, err := a.requireAdmin(r)
	if err != nil {
		return nil, err
	}
	model := a.cache.Model()
	n := 0
	for i := range model.Birthdays {
		sv, ok := a.staffView(model, model.Birthdays[i].Email)
		if !ok || sv.AssignedTo == "" || sv.RequestBy == "" || sv.Stage == StageComplete {
			continue
		}
		a.sendInvite(r, sv, sv.AssignedTo, "")
		n++
	}
	slog.InfoContext(r.Context(), "birthday: resent invites", "actor", actor.Email, "count", n)
	return map[string]int{"sent": n}, nil
}

func (a app) joinTeam(r *http.Request, _ serve.None) (serve.None, error) {
	actor := a.actor(r)
	ops, err := a.cache.joinTeam(actor)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	if err := a.joinHome(r.Context(), actor.Email); err != nil {
		slog.ErrorContext(r.Context(), "birthday: put a joiner on the app's list", "error", err, "email", actor.Email)
	}
	slog.InfoContext(r.Context(), "birthday: joined the team", "email", actor.Email)
	return serve.None{}, nil
}

func (a app) addTeamMember(r *http.Request, body TeamMember) (serve.None, error) {
	actor := a.actor(r)
	ops, member, err := a.cache.addTeamMember(actor, body.Email, body.Role)
	if err != nil {
		return serve.None{}, err
	}
	if len(ops) == 0 {
		return serve.None{}, nil
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: added team member", "actor", actor.Email, "email", member.Email, "role", member.Role)
	return serve.None{}, nil
}

func (a app) removeTeamMember(r *http.Request, body TeamMember) (serve.None, error) {
	actor := a.actor(r)
	ops, member, err := removeTeamMember(actor, body.Email, body.Role)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "birthday: removed team member", "actor", actor.Email, "email", member.Email, "role", member.Role)
	return serve.None{}, nil
}
