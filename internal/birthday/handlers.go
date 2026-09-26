package birthday

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"heliosian/internal/blob"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/claude"
	"heliosian/internal/describe"
	"heliosian/internal/logging"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
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

func Register(mux *http.ServeMux, cache *Cache, media *blob.Store, directory Directory, superAdmins func() []string, describer Describer, mailer mail.Sender, from, base string, joinHome func(ctx context.Context, email string) error) {
	a := app{cache: cache, directory: directory, superAdmins: superAdmins, describer: describer, mailer: mailer, from: from, base: base, joinHome: joinHome}
	if mailer != nil {
		go a.remindLoop()
	}
	go a.exportLoop()
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.HandleFunc("GET /open/share/about.png", a.shareCard)
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

func (a app) requireTeam(w http.ResponseWriter, r *http.Request) (string, bool, bool) {
	email, admin := a.who(r)
	if !admin && !a.cache.Model().OnTeam(email) {
		http.Error(w, "team membership required", http.StatusForbidden)
		return "", false, false
	}
	return email, admin, true
}

var now = func() time.Time {
	return time.Now().In(local)
}

func today() string {
	return now().Format(DateFormat)
}

func (a app) year() string {
	month, day, _ := ParseMonthDay(a.cache.Model().Settings.YearStart)
	return YearContaining(now(), month, day).Label
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	email, admin := a.who(r)
	view := Render(a.cache.Model(), a.directory, email, admin, now())
	view.User.IsSuperAdmin = a.cache.IsSuperAdmin(email)
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

func (a app) commit(w http.ResponseWriter, r *http.Request, actor string, ops ...store.Op) bool {
	before := a.askDays(a.cache.Model())
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	a.mailMovedAskDays(r, before, a.askDays(a.cache.Model()))
	return true
}

func cleanEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func (a app) findStaff(w http.ResponseWriter, raw string) (string, bool) {
	email := cleanEmail(raw)
	model := a.cache.Model()
	if model.Skipped(email) {
		http.Error(w, fmt.Sprintf("%s asked to be left out", email), http.StatusBadRequest)
		return "", false
	}
	if !model.InPipeline(email) {
		http.Error(w, fmt.Sprintf("%s has no birthday on file", email), http.StatusNotFound)
		return "", false
	}
	return email, true
}

func (a app) assign(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireTeam(w, r)
	if !ok {
		return
	}
	var body struct {
		Email      string `json:"email"`
		AssignedTo string `json:"assignedTo"`
	}
	if !decode(w, r, &body) {
		return
	}
	email, ok := a.findStaff(w, body.Email)
	if !ok {
		return
	}
	to := cleanEmail(body.AssignedTo)
	if to == "" {
		to = actor
	}
	if err := checkEmail(to); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if to != actor && !a.cache.Model().OnTeam(to) && !a.cache.IsAdmin(to) {
		http.Error(w, fmt.Sprintf("%s is not on the birthday team", to), http.StatusBadRequest)
		return
	}
	year := a.year()
	if !a.commit(w, r, actor, store.Set(assignmentsTab, store.Row{"Email": email, "Year": year}, store.Row{"Assigned To": to, "Assigned On": today()})) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: assigned", "actor", actor, "email", email, "to", to, "year", year)
	a.mailAssignment(r, email, to)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) unassign(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireTeam(w, r)
	if !ok {
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &body) {
		return
	}
	email, ok := a.findStaff(w, body.Email)
	if !ok {
		return
	}
	year := a.year()
	if !a.commit(w, r, actor, store.Delete(assignmentsTab, store.Row{"Email": email, "Year": year})) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: unassigned", "actor", actor, "email", email, "year", year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) outreach(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireTeam(w, r)
	if !ok {
		return
	}
	var body struct {
		Email     string `json:"email"`
		Contacted bool   `json:"contacted"`
	}
	if !decode(w, r, &body) {
		return
	}
	email, ok := a.findStaff(w, body.Email)
	if !ok {
		return
	}
	year := a.year()
	match := store.Row{"Email": email, "Year": year}
	if !body.Contacted {
		if !a.commit(w, r, actor, store.Delete(outreachTab, match)) {
			return
		}
		slog.InfoContext(r.Context(), "birthday: outreach undone", "actor", actor, "email", email, "year", year)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.commit(w, r, actor, store.Set(outreachTab, match, store.Row{"Contacted On": today(), "Contacted By": actor})) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: contacted", "actor", actor, "email", email, "year", year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveDonation(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireTeam(w, r)
	if !ok {
		return
	}
	var body struct {
		Email   string `json:"email"`
		Charity string `json:"charity"`
		Note    string `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	email, ok := a.findStaff(w, body.Email)
	if !ok {
		return
	}
	charity := a.cache.Model().Charity(strings.TrimSpace(body.Charity))
	if charity == nil {
		http.Error(w, "pick a charity from the list", http.StatusBadRequest)
		return
	}
	if !charity.Allowed {
		http.Error(w, fmt.Sprintf("%s is not an allowed charity: %s", charity.Name, charity.WhyNotAllowed), http.StatusBadRequest)
		return
	}
	if len(body.Note) > maxTextLength {
		http.Error(w, "the note is too long", http.StatusBadRequest)
		return
	}
	year := a.year()
	cells := store.Row{"Charity": charity.Name, "Note": strings.TrimSpace(body.Note), "Recorded On": today(), "Recorded By": actor}
	if !a.commit(w, r, actor, store.Set(donationsTab, store.Row{"Email": email, "Year": year}, cells)) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: saved donation", "actor", actor, "email", email, "charity", charity.Name, "year", year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteDonation(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireTeam(w, r)
	if !ok {
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &body) {
		return
	}
	email, ok := a.findStaff(w, body.Email)
	if !ok {
		return
	}
	year := a.year()
	if !a.commit(w, r, actor, store.Delete(donationsTab, store.Row{"Email": email, "Year": year})) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed donation", "actor", actor, "email", email, "year", year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) used(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireTeam(w, r)
	if !ok {
		return
	}
	var body struct {
		Email string `json:"email"`
		Used  bool   `json:"used"`
	}
	if !decode(w, r, &body) {
		return
	}
	email, ok := a.findStaff(w, body.Email)
	if !ok {
		return
	}
	year := a.year()
	if _, ok := a.cache.Model().Donation(email, year); !ok {
		http.Error(w, "record a donation first", http.StatusBadRequest)
		return
	}
	cells := store.Row{"Used On": "", "Used By": ""}
	if body.Used {
		cells = store.Row{"Used On": today(), "Used By": actor}
	}
	if !a.commit(w, r, actor, store.Update(donationsTab, store.Row{"Email": email, "Year": year}, cells)) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: marked donation", "actor", actor, "used", body.Used, "email", email, "year", year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveBirthday(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireTeam(w, r)
	if !ok {
		return
	}
	var body struct {
		Email    string `json:"email"`
		Birthday string `json:"birthday"`
		Override string `json:"override"`
	}
	if !decode(w, r, &body) {
		return
	}
	email := cleanEmail(body.Email)
	if err := checkEmail(email); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	birthday, override := strings.TrimSpace(body.Birthday), strings.TrimSpace(body.Override)
	if !a.commit(w, r, actor, store.Set(birthdaysTab, store.Row{"Email": email}, store.Row{"Birthday": birthday, "Newsletter Override": override})) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: saved birthday", "actor", actor, "email", email, "birthday", birthday, "override", override)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteBirthday(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &body) {
		return
	}
	email := cleanEmail(body.Email)
	match := store.Row{"Email": email}
	if a.cache.Count(birthdaysTab, match) == 0 {
		http.Error(w, "no such birthday", http.StatusNotFound)
		return
	}
	for _, tab := range []string{assignmentsTab, outreachTab, donationsTab, notesTab} {
		if a.cache.Count(tab, match) > 0 {
			http.Error(w, "remove their assignments, outreach, donations, and notes first", http.StatusBadRequest)
			return
		}
	}
	if !a.commit(w, r, actor, store.Delete(birthdaysTab, match)) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed birthday", "actor", actor, "email", email)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveParticipation(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireTeam(w, r)
	if !ok {
		return
	}
	var body struct {
		Email string `json:"email"`
		Level string `json:"level"`
		Note  string `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	email := cleanEmail(body.Email)
	if err := checkEmail(email); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !slices.Contains(Levels, body.Level) {
		http.Error(w, "level must be one of "+strings.Join(Levels, ", "), http.StatusBadRequest)
		return
	}
	if len(body.Note) > maxTextLength {
		http.Error(w, "the note is too long", http.StatusBadRequest)
		return
	}
	if !a.commit(w, r, actor, store.Set(birthdaysTab, store.Row{"Email": email}, store.Row{"Participation": body.Level, "Note": strings.TrimSpace(body.Note)})) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: saved participation", "actor", actor, "email", email, "level", body.Level)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteParticipation(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireTeam(w, r)
	if !ok {
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &body) {
		return
	}
	email := cleanEmail(body.Email)
	b := a.cache.Model().Birthday(email)
	if b == nil || b.Level == "" {
		http.Error(w, "no preference is recorded", http.StatusNotFound)
		return
	}
	match := store.Row{"Email": email}
	op := store.Update(birthdaysTab, match, store.Row{"Participation": "", "Note": ""})
	if b.Birthday == "" {
		op = store.Delete(birthdaysTab, match)
	}
	if !a.commit(w, r, actor, op) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed participation", "actor", actor, "email", email)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) addNote(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireTeam(w, r)
	if !ok {
		return
	}
	var body struct {
		Email string `json:"email"`
		Note  string `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	email := cleanEmail(body.Email)
	if b := a.cache.Model().Birthday(email); b == nil || b.Birthday == "" {
		http.Error(w, fmt.Sprintf("%s has no birthday on file", email), http.StatusNotFound)
		return
	}
	note := strings.TrimSpace(body.Note)
	if note == "" || len(note) > maxTextLength {
		http.Error(w, "the note is empty or too long", http.StatusBadRequest)
		return
	}
	if !a.commit(w, r, actor, store.Insert(notesTab, store.Row{"Email": email, "Note": note, "Added By": actor, "Added": today()})) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: added note", "actor", actor, "email", email)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteNote(w http.ResponseWriter, r *http.Request) {
	actor, admin, ok := a.requireTeam(w, r)
	if !ok {
		return
	}
	var body struct {
		Email   string `json:"email"`
		Note    string `json:"note"`
		AddedBy string `json:"addedBy"`
		Added   string `json:"added"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !admin && cleanEmail(body.AddedBy) != actor {
		http.Error(w, "only the note's author or an admin can remove it", http.StatusForbidden)
		return
	}
	match := store.Row{"Email": cleanEmail(body.Email), "Note": body.Note, "Added By": cleanEmail(body.AddedBy), "Added": body.Added}
	if !a.commit(w, r, actor, store.Delete(notesTab, match)) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed note", "actor", actor, "email", match["Email"])
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveCharity(w http.ResponseWriter, r *http.Request) {
	actor, admin, ok := a.requireTeam(w, r)
	if !ok {
		return
	}
	var body struct {
		Original      string `json:"original"`
		Name          string `json:"name"`
		DonationLink  string `json:"donationLink"`
		About         string `json:"about"`
		EIN           string `json:"ein"`
		Allowed       bool   `json:"allowed"`
		WhyNotAllowed string `json:"whyNotAllowed"`
	}
	if !decode(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	model := a.cache.Model()
	adding := body.Original == ""
	var current *Charity
	if !adding {
		current = model.Charity(body.Original)
		if current == nil {
			http.Error(w, "no such charity", http.StatusNotFound)
			return
		}
	}
	allowed, why := true, ""
	switch {
	case admin:
		allowed, why = body.Allowed, strings.TrimSpace(body.WhyNotAllowed)
	case !adding:
		allowed, why = current.Allowed, current.WhyNotAllowed
	}
	if allowed {
		why = ""
	}
	cells := store.Row{
		"Name": name, "Donation Link": strings.TrimSpace(body.DonationLink), "About": strings.TrimSpace(body.About),
		"EIN": strings.TrimSpace(body.EIN), "Allowed": YesNo(allowed), "Why Not Allowed": why,
	}
	renamed := !adding && body.Original != name
	if (adding || renamed) && model.Charity(name) != nil {
		http.Error(w, fmt.Sprintf("%q is already on the list", name), http.StatusBadRequest)
		return
	}
	op := store.Update(charitiesTab, store.Row{"Name": body.Original}, cells)
	if adding {
		cells["Added On"] = today()
		op = store.Insert(charitiesTab, cells)
	}
	if !a.commit(w, r, actor, op) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: saved charity", "actor", actor, "adding", adding, "charity", name, "allowed", allowed)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) describeCharity(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireTeam(w, r)
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
	info, err := a.describer.Charity(r.Context(), actor, name, link)
	if errors.Is(err, claude.ErrTooMany) {
		http.Error(w, err.Error(), http.StatusTooManyRequests)
		return
	}
	if errors.Is(err, describe.ErrTooLong) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "birthday: describe charity", "actor", actor, "name", name, "error", err)
		http.Error(w, "could not look this charity up right now", http.StatusBadGateway)
		return
	}
	if info.Sentence == "" {
		http.Error(w, "could not find this charity; please enter its information", http.StatusNotFound)
		return
	}
	slog.InfoContext(r.Context(), "birthday: described charity", "actor", actor, "name", name, "link", info.DonationLink)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(info); err != nil {
		slog.ErrorContext(r.Context(), "encode charity sentence", "error", err)
	}
}

func (a app) deleteCharity(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	model := a.cache.Model()
	if model.Charity(body.Name) == nil {
		http.Error(w, "no such charity", http.StatusNotFound)
		return
	}
	if model.Settings.DefaultCharity == body.Name {
		http.Error(w, "pick another default charity first", http.StatusBadRequest)
		return
	}
	if a.cache.Count(donationsTab, store.Row{"Charity": body.Name}) > 0 {
		http.Error(w, "donations name this charity; it can be marked not allowed instead", http.StatusBadRequest)
		return
	}
	if !a.commit(w, r, actor, store.Delete(charitiesTab, store.Row{"Name": body.Name})) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed charity", "actor", actor, "charity", body.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) addNewsletterDate(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Date string `json:"date"`
	}
	if !decode(w, r, &body) {
		return
	}
	date := strings.TrimSpace(body.Date)
	if _, err := ParseDate(date); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if slices.Contains(a.cache.Model().NewsletterDates, date) {
		http.Error(w, "that date is already on the list", http.StatusBadRequest)
		return
	}
	if !a.commit(w, r, actor, store.Insert(newsletterDatesTab, store.Row{"Date": date})) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: added newsletter date", "actor", actor, "date", date)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) changeNewsletterDate(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Original string `json:"original"`
		Date     string `json:"date"`
	}
	if !decode(w, r, &body) {
		return
	}
	original, date := strings.TrimSpace(body.Original), strings.TrimSpace(body.Date)
	if _, err := ParseDate(date); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dates := a.cache.Model().NewsletterDates
	if !slices.Contains(dates, original) {
		http.Error(w, "no such newsletter date", http.StatusNotFound)
		return
	}
	if date == original {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if slices.Contains(dates, date) {
		http.Error(w, "that date is already on the list", http.StatusBadRequest)
		return
	}
	if !a.commit(w, r, actor, store.Update(newsletterDatesTab, store.Row{"Date": original}, store.Row{"Date": date})) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: moved newsletter date", "actor", actor, "from", original, "to", date)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteNewsletterDate(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Date string `json:"date"`
	}
	if !decode(w, r, &body) {
		return
	}
	match := store.Row{"Date": strings.TrimSpace(body.Date)}
	if !a.commit(w, r, actor, store.Delete(newsletterDatesTab, match)) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed newsletter date", "actor", actor, "date", match["Date"])
	w.WriteHeader(http.StatusNoContent)
}

func (a app) createNewsletterDates(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Weekday int    `json:"weekday"`
		From    string `json:"from"`
		To      string `json:"to"`
	}
	if !decode(w, r, &body) {
		return
	}
	from, err := ParseDate(strings.TrimSpace(body.From))
	if err != nil {
		http.Error(w, "from: "+err.Error(), http.StatusBadRequest)
		return
	}
	to, err := ParseDate(strings.TrimSpace(body.To))
	if err != nil {
		http.Error(w, "to: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Weekday < 0 || body.Weekday > 6 {
		http.Error(w, "weekday must be 0 (Sunday) to 6 (Saturday)", http.StatusBadRequest)
		return
	}
	if to.Before(from) {
		http.Error(w, "the final date is before the first", http.StatusBadRequest)
		return
	}
	if to.Sub(from) > 366*24*time.Hour {
		http.Error(w, "at most a year at a time", http.StatusBadRequest)
		return
	}
	day := from.AddDate(0, 0, (body.Weekday-int(from.Weekday())+7)%7)
	have := a.cache.Model().NewsletterDates
	added := []string{}
	ops := []store.Op{}
	for ; !day.After(to); day = day.AddDate(0, 0, 7) {
		cell := day.Format(DateFormat)
		if slices.Contains(have, cell) {
			continue
		}
		added = append(added, cell)
		ops = append(ops, store.Insert(newsletterDatesTab, store.Row{"Date": cell}))
	}
	if len(added) == 0 {
		http.Error(w, "every one of those dates is already on the list", http.StatusBadRequest)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: created newsletter dates", "actor", actor, "count", len(added), "from", added[0], "to", added[len(added)-1])
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]int{"added": len(added)}); err != nil {
		slog.ErrorContext(r.Context(), "encode created dates", "error", err)
	}
}

func (a app) clearFutureNewsletterDates(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	day := today()
	removed := []string{}
	ops := []store.Op{}
	for _, date := range a.cache.Model().NewsletterDates {
		if date >= day {
			removed = append(removed, date)
			ops = append(ops, store.Delete(newsletterDatesTab, store.Row{"Date": date}))
		}
	}
	if len(removed) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: cleared future newsletter dates", "actor", actor, "count", len(removed), "from", day)
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
	values := map[string]string{
		DefaultCharityKey: strings.TrimSpace(body.DefaultCharity), YearStartKey: strings.TrimSpace(body.YearStart),
		EmailSubjectKey: strings.TrimSpace(body.EmailSubject), EmailBodyKey: strings.TrimSpace(body.EmailBody),
		NoNewsletterNoteKey: strings.TrimSpace(body.NoNewsletterNote), OutreachCCKey: strings.TrimSpace(body.OutreachCC),
		RequestLeadKey: strconv.Itoa(body.RequestLeadDays),
	}
	ops := []store.Op{}
	for _, key := range settingKeys {
		ops = append(ops, store.Set(settingsTab, store.Row{"Key": key}, store.Row{"Value": values[key]}))
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: changed the settings", "actor", actor)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) adminState(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	model := a.cache.Model()
	team := make([]TeamView, 0, len(model.Team))
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
	}{Email: email, Admins: a.cache.Admins(a.superAdmins()), Team: team, Roles: Roles, People: a.directory.People()}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode birthday admin state", "error", err)
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
	current := a.cache.Model().Admins
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
	slog.InfoContext(r.Context(), "birthday: set the admin list", "actor", actor, "admins", admins)
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
	slog.InfoContext(r.Context(), "birthday: resent invites", "actor", actor, "count", n)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]int{"sent": n}); err != nil {
		slog.ErrorContext(r.Context(), "encode resent invites", "error", err)
	}
}

func (a app) joinTeam(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
	if err := checkEmail(actor); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	row := store.Row{"Email": actor, "Role": RoleVolunteer}
	if a.cache.Count(teamTab, row) == 0 && !a.commit(w, r, actor, store.Insert(teamTab, row)) {
		return
	}
	if a.joinHome != nil {
		if err := a.joinHome(r.Context(), actor); err != nil {
			slog.ErrorContext(r.Context(), "birthday: put a joiner on the app's list", "error", err, "email", actor)
		}
	}
	slog.InfoContext(r.Context(), "birthday: joined the team", "email", actor)
	w.WriteHeader(http.StatusNoContent)
}

func teamRow(w http.ResponseWriter, r *http.Request) (map[string]string, bool) {
	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !decode(w, r, &body) {
		return nil, false
	}
	email := cleanEmail(body.Email)
	if err := checkEmail(email); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, false
	}
	role := strings.TrimSpace(body.Role)
	if !slices.Contains(Roles, role) {
		http.Error(w, fmt.Sprintf("%q is not a role", role), http.StatusBadRequest)
		return nil, false
	}
	return map[string]string{"Email": email, "Role": role}, true
}

func (a app) addTeamMember(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	row, ok := teamRow(w, r)
	if !ok {
		return
	}
	if a.cache.Count(teamTab, row) > 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.commit(w, r, actor, store.Insert(teamTab, row)) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: added team member", "actor", actor, "email", row["Email"], "role", row["Role"])
	w.WriteHeader(http.StatusNoContent)
}

func (a app) removeTeamMember(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	row, ok := teamRow(w, r)
	if !ok {
		return
	}
	if !a.commit(w, r, actor, store.Delete(teamTab, row)) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed team member", "actor", actor, "email", row["Email"], "role", row["Role"])
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
