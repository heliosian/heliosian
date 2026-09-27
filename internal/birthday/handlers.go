package birthday

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/claude"
	"heliosian/internal/describe"
	"heliosian/internal/logging"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
)

const shell = "web/birthday/index.html"

var pages = []string{
	"/{$}", "/jobs", "/process", "/calendar", "/charities", "/charities/{name}", "/newsletters", "/newsletters/{date}", "/skipped", "/unassigned", "/admin", "/staff/{handle}",
}

var local = mustLocation("America/Los_Angeles")

func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		logging.Fatal("load time zone", "name", name, "error", err)
	}
	return loc
}

type app struct {
	cache       *Cache
	directory   Directory
	superAdmins func() []string
	describer   Describer
	mailer      mail.Sender
	from        string
	base        string
	joinHome    func(ctx context.Context, email string) error
}

type Describer interface {
	Charity(ctx context.Context, actor, name, link string) (describe.Info, error)
}

func Register(mux *http.ServeMux, cache *Cache, directory Directory, superAdmins func() []string, describer Describer, mailer mail.Sender, from, base string, joinHome func(ctx context.Context, email string) error, about *sharecard.About) {
	a := app{cache: cache, directory: directory, superAdmins: superAdmins, describer: describer, mailer: mailer, from: from, base: base, joinHome: joinHome}
	if mailer != nil {
		go a.remindLoop()
	}
	go a.exportLoop()
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.Handle("GET /open/share/about.png", about)
	mux.HandleFunc("GET /api/birthday/model", a.model)
	mux.HandleFunc("POST /api/birthday/assign", a.assign)
	mux.HandleFunc("DELETE /api/birthday/assign", a.unassign)
	mux.HandleFunc("POST /api/birthday/outreach", a.outreach)
	mux.HandleFunc("POST /api/birthday/donation", a.saveDonation)
	mux.HandleFunc("DELETE /api/birthday/donation", a.deleteDonation)
	mux.HandleFunc("POST /api/birthday/used", a.used)
	mux.HandleFunc("POST /api/birthday/birthday", a.saveBirthday)
	mux.HandleFunc("DELETE /api/birthday/birthday", a.deleteBirthday)
	mux.HandleFunc("POST /api/birthday/participation", a.saveParticipation)
	mux.HandleFunc("DELETE /api/birthday/participation", a.deleteParticipation)
	mux.HandleFunc("POST /api/birthday/note", a.addNote)
	mux.HandleFunc("DELETE /api/birthday/note", a.deleteNote)
	mux.HandleFunc("POST /api/birthday/charity", a.saveCharity)
	mux.HandleFunc("DELETE /api/birthday/charity", a.deleteCharity)
	mux.HandleFunc("POST /api/birthday/charity/describe", a.describeCharity)
	mux.HandleFunc("POST /api/birthday/newsletter-date", a.addNewsletterDate)
	mux.HandleFunc("PUT /api/birthday/newsletter-date", a.changeNewsletterDate)
	mux.HandleFunc("DELETE /api/birthday/newsletter-date", a.deleteNewsletterDate)
	mux.HandleFunc("POST /api/birthday/newsletter-dates/clear-future", a.clearFutureNewsletterDates)
	mux.HandleFunc("POST /api/birthday/newsletter-dates/create", a.createNewsletterDates)
	mux.HandleFunc("POST /api/birthday/newsletter/share", a.shareIssue)
	mux.HandleFunc("POST /api/birthday/settings", a.saveSettings)
	mux.HandleFunc("GET /api/admin/state", a.adminState)
	mux.HandleFunc("POST /api/admin/admins", a.setAdmins)
	mux.HandleFunc("POST /api/admin/resend-invites", a.resendInvites)
	mux.HandleFunc("POST /api/birthday/team/join", a.joinTeam)
	mux.HandleFunc("POST /api/admin/team", a.addTeamMember)
	mux.HandleFunc("DELETE /api/admin/team", a.removeTeamMember)
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

func (a app) actor(r *http.Request) access.Actor {
	email := a.directory.Resolve(strings.ToLower(auth.Email(r)))
	return access.Actor{Email: email, Admin: a.cache.IsAdmin(email)}
}

func refuse(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), access.Status(err))
}

func (a app) requireAdmin(w http.ResponseWriter, r *http.Request) (access.Actor, bool) {
	actor := a.actor(r)
	if err := requireAdmin(actor); err != nil {
		refuse(w, err)
		return access.Actor{}, false
	}
	return actor, true
}

func (a app) requireTeam(w http.ResponseWriter, r *http.Request) (access.Actor, bool) {
	actor := a.actor(r)
	if err := a.cache.Model().requireTeam(actor); err != nil {
		refuse(w, err)
		return access.Actor{}, false
	}
	return actor, true
}

var now = func() time.Time {
	return time.Now().In(local)
}

func today() string {
	return now().Format(DateFormat)
}

func (m *Model) year() string {
	month, day, _ := ParseMonthDay(m.Settings.YearStart)
	return YearContaining(now(), month, day).Label
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	view := Render(a.cache.Model(), a.directory, actor, now())
	view.User.IsSuperAdmin = a.cache.IsSuperAdmin(actor.Email)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode birthday model", "error", err)
	}
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(into); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return false
	}
	return true
}

func (a app) write(w http.ResponseWriter, r *http.Request, actor access.Actor, ops []store.Op, err error) bool {
	if err != nil {
		refuse(w, err)
		return false
	}
	before := a.askDays(a.cache.Model())
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		refuse(w, err)
		return false
	}
	a.mailMovedAskDays(r, before, a.askDays(a.cache.Model()))
	return true
}

func cleanEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func (a app) assign(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email      string `json:"email"`
		AssignedTo string `json:"assignedTo"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, t, err := a.cache.assign(actor, body.Email, body.AssignedTo)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: assigned", "actor", actor.Email, "email", t.email, "to", t.to, "year", t.year)
	a.mailAssignment(r, t.email, t.to)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) unassign(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, t, err := a.cache.Model().unassign(actor, body.Email)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: unassigned", "actor", actor.Email, "email", t.email, "year", t.year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) outreach(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email     string `json:"email"`
		Contacted bool   `json:"contacted"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, t, err := a.cache.Model().outreach(actor, body.Email, body.Contacted)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	if !body.Contacted {
		slog.InfoContext(r.Context(), "birthday: outreach undone", "actor", actor.Email, "email", t.email, "year", t.year)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	slog.InfoContext(r.Context(), "birthday: contacted", "actor", actor.Email, "email", t.email, "year", t.year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveDonation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email   string `json:"email"`
		Charity string `json:"charity"`
		Note    string `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, t, err := a.cache.Model().saveDonation(actor, body.Email, body.Charity, body.Note)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: saved donation", "actor", actor.Email, "email", t.email, "charity", t.charity, "year", t.year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteDonation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, t, err := a.cache.Model().deleteDonation(actor, body.Email)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed donation", "actor", actor.Email, "email", t.email, "year", t.year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) used(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
		Used  bool   `json:"used"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, t, err := a.cache.Model().markUsed(actor, body.Email, body.Used)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: marked donation", "actor", actor.Email, "used", body.Used, "email", t.email, "year", t.year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveBirthday(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Birthday string `json:"birthday"`
		Override string `json:"override"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, email, err := a.cache.Model().saveBirthday(actor, body.Email, body.Birthday, body.Override)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: saved birthday", "actor", actor.Email, "email", email, "birthday", strings.TrimSpace(body.Birthday), "override", strings.TrimSpace(body.Override))
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteBirthday(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, email, err := a.cache.deleteBirthday(actor, body.Email)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed birthday", "actor", actor.Email, "email", email)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveParticipation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
		Level string `json:"level"`
		Note  string `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, email, err := a.cache.Model().saveParticipation(actor, body.Email, body.Level, body.Note)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: saved participation", "actor", actor.Email, "email", email, "level", body.Level)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteParticipation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, email, err := a.cache.Model().deleteParticipation(actor, body.Email)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed participation", "actor", actor.Email, "email", email)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) addNote(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
		Note  string `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, email, err := a.cache.Model().addNote(actor, body.Email, body.Note)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: added note", "actor", actor.Email, "email", email)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteNote(w http.ResponseWriter, r *http.Request) {
	var body Note
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, email, err := a.cache.Model().deleteNote(actor, body)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed note", "actor", actor.Email, "email", email)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveCharity(w http.ResponseWriter, r *http.Request) {
	var body charityEdit
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, saved, adding, err := a.cache.Model().saveCharity(actor, body)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: saved charity", "actor", actor.Email, "adding", adding, "charity", saved.Name, "allowed", saved.Allowed)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) describeCharity(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireTeam(w, r)
	if !ok {
		return
	}
	var body struct {
		Name         string `json:"name"`
		DonationLink string `json:"donationLink"`
	}
	if !decode(w, r, &body) {
		return
	}
	name, link := strings.TrimSpace(body.Name), strings.TrimSpace(body.DonationLink)
	if name == "" {
		http.Error(w, "give the charity's name first", http.StatusBadRequest)
		return
	}
	if a.describer == nil {
		http.Error(w, "suggesting a sentence is not set up on this server", http.StatusServiceUnavailable)
		return
	}
	info, err := a.describer.Charity(r.Context(), actor.Email, name, link)
	if errors.Is(err, claude.ErrTooMany) {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	if errors.Is(err, describe.ErrTooLong) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "birthday: describe charity", "actor", actor.Email, "name", name, "error", err)
		http.Error(w, "could not look this charity up right now", http.StatusBadGateway)
		return
	}
	if info.Sentence == "" {
		http.Error(w, "could not find this charity; please enter its information", http.StatusNotFound)
		return
	}
	slog.InfoContext(r.Context(), "birthday: described charity", "actor", actor.Email, "name", name, "link", info.DonationLink)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(info); err != nil {
		slog.ErrorContext(r.Context(), "encode charity sentence", "error", err)
	}
}

func (a app) deleteCharity(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, err := a.cache.deleteCharity(actor, body.Name)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed charity", "actor", actor.Email, "charity", body.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) addNewsletterDate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Date string `json:"date"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, date, err := a.cache.Model().addNewsletterDate(actor, body.Date)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: added newsletter date", "actor", actor.Email, "date", date)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) changeNewsletterDate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Original string `json:"original"`
		Date     string `json:"date"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, err := a.cache.Model().changeNewsletterDate(actor, body.Original, body.Date)
	if err == nil && len(ops) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: moved newsletter date", "actor", actor.Email, "from", strings.TrimSpace(body.Original), "to", strings.TrimSpace(body.Date))
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteNewsletterDate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Date string `json:"date"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, date, err := deleteNewsletterDate(actor, body.Date)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed newsletter date", "actor", actor.Email, "date", date)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) createNewsletterDates(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Weekday int    `json:"weekday"`
		From    string `json:"from"`
		To      string `json:"to"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, added, err := a.cache.Model().createNewsletterDates(actor, body.Weekday, body.From, body.To)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: created newsletter dates", "actor", actor.Email, "count", len(added), "from", added[0], "to", added[len(added)-1])
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]int{"added": len(added)}); err != nil {
		slog.ErrorContext(r.Context(), "encode created dates", "error", err)
	}
}

func (a app) clearFutureNewsletterDates(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	day := today()
	ops, err := a.cache.Model().clearFutureNewsletterDates(actor, day)
	if err == nil && len(ops) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: cleared future newsletter dates", "actor", actor.Email, "count", len(ops), "from", day)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveSettings(w http.ResponseWriter, r *http.Request) {
	var body Settings
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, err := saveSettings(actor, body)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: changed the settings", "actor", actor.Email)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) adminState(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	model := a.cache.Model()
	team := []TeamView{}
	v := viewer{directory: a.directory}
	for _, m := range model.Team {
		p, _ := v.person(m.Email)
		team = append(team, TeamView{Email: m.Email, Name: p.Name, Role: m.Role})
	}
	view := struct {
		Email  string     `json:"email"`
		Admins []string   `json:"admins"`
		Team   []TeamView `json:"team"`
		Roles  []string   `json:"roles"`
		People []Person   `json:"people"`
	}{Email: actor.Email, Admins: a.cache.Admins(a.superAdmins()), Team: team, Roles: Roles, People: a.directory.People()}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode birthday admin state", "error", err)
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
	ops, admins, err := a.cache.Model().setAdmins(actor, body.Admins, a.superAdmins())
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: set the admin list", "actor", actor.Email, "admins", admins)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) resendInvites(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	if a.mailer == nil {
		http.Error(w, "mail is not set up on this server", http.StatusServiceUnavailable)
		return
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
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]int{"sent": n}); err != nil {
		slog.ErrorContext(r.Context(), "encode resent invites", "error", err)
	}
}

func (a app) joinTeam(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	ops, err := a.cache.joinTeam(actor)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	if a.joinHome != nil {
		if err := a.joinHome(r.Context(), actor.Email); err != nil {
			slog.ErrorContext(r.Context(), "birthday: put a joiner on the app's list", "error", err, "email", actor.Email)
		}
	}
	slog.InfoContext(r.Context(), "birthday: joined the team", "email", actor.Email)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) addTeamMember(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, member, err := a.cache.addTeamMember(actor, body.Email, body.Role)
	if err == nil && len(ops) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: added team member", "actor", actor.Email, "email", member.Email, "role", member.Role)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) removeTeamMember(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, member, err := removeTeamMember(actor, body.Email, body.Role)
	if !a.write(w, r, actor, ops, err) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed team member", "actor", actor.Email, "email", member.Email, "role", member.Role)
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
