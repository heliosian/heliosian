package birthday

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/data"
	"heliosian/internal/describe"
	"heliosian/internal/logging"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
)

const shell = "web/birthday/index.html"

var pages = []string{
	"/{$}", "/jobs", "/process", "/calendar", "/charities", "/charities/{name}", "/newsletters", "/newsletters/{date}", "/skipped", "/unassigned", "/admin", "/staff/{handle}",
}

// local is the school's clock: a birthday is a day there, and a step taken late
// one evening is dated that evening.
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
	writer      data.Writer
	queue       Enqueuer
	directory   Directory
	superAdmins func() []string
	describer   Describer
	// mailer sends the assignee their invite; nil sends nothing. from is the
	// address it comes from.
	mailer mail.Sender
	from   string
	// base is the app's address for the links in mail sent off a request.
	base string
}

// Describer finds where a charity takes donations and writes the newsletter's
// sentence about it; nil leaves the charity form's Generate Info saying it is
// not set up.
type Describer interface {
	Charity(ctx context.Context, name, link string) (describe.Info, error)
}

// Register wires the app: one shell for every page, the model, and the writes.
// Every route already sits behind sign-in.
func Register(mux *http.ServeMux, cache *Cache, writer data.Writer, queue Enqueuer, directory Directory, superAdmins func() []string, describer Describer, mailer mail.Sender, from, base string) {
	a := app{cache: cache, writer: writer, queue: queue, directory: directory, superAdmins: superAdmins, describer: describer, mailer: mailer, from: from, base: base}
	if mailer != nil {
		go a.remindLoop()
	}
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
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
	mux.HandleFunc("POST /api/birthday/settings", a.saveSettings)
	mux.HandleFunc("GET /api/admin/state", a.adminState)
	mux.HandleFunc("POST /api/admin/admins", a.setAdmins)
	mux.HandleFunc("POST /api/admin/team", a.addTeamMember)
	mux.HandleFunc("DELETE /api/admin/team", a.removeTeamMember)
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

// who is the signed-in person as the app keys them: the address Google
// vouched for, resolved through the directory's aliases.
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

var now = func() time.Time {
	return time.Now().In(local)
}

func today() string {
	return now().Format(DateFormat)
}

// year is the birthday year the app is working in right now.
func (a app) year() string {
	month, day, _ := ParseMonthDay(a.cache.Model().Settings.YearStart)
	return YearContaining(now(), month, day).Label
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	email, admin := a.who(r)
	view := Render(a.cache.Model(), a.directory, email, admin, now())
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

// commit rebuilds the model over the proposed tables first, so a change the
// sheet rules reject never reaches the sheet, then applies it in memory and
// queues the writes behind every earlier one.
func (a app) commit(r *http.Request, w http.ResponseWriter, tables *Tables, flush func() error) bool {
	ctx := r.Context()
	model, err := BuildModel(tables)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	before := a.askDays(a.cache.Model())
	applied := make(chan struct{})
	a.queue.Add(func() {
		a.cache.set(tables, model)
		close(applied)
		if err := flush(); err != nil {
			slog.ErrorContext(ctx, "birthday write", "error", err)
		}
	})
	<-applied
	a.mailMovedAskDays(r, before, a.askDays(model))
	return true
}

func rowOf(columns []string, cells map[string]string) []string {
	row := make([]string, len(columns))
	for i, column := range columns {
		row[i] = cells[column]
	}
	return row
}

func (a app) logChange(actor, action, kind, email, year, details string) error {
	return a.writer.Append(appName, changeLogTab, []string{
		time.Now().Format(time.RFC3339), actor, action, kind, email, year, details,
	})
}

func cleanEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// findStaff resolves a request's email to someone in the pipeline: they have a
// birthday on file and have not asked to be left out.
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
	actor, _ := a.who(r)
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
	year := a.year()
	match := map[string]string{"Email": email, "Year": year}
	cells := map[string]string{"Assigned To": to, "Assigned On": today()}
	tables := a.cache.Tables()
	action := "edit"
	if tables.count(assignmentsTab, match) == 0 {
		action = "add"
	}
	if !a.commit(r, w, tables.with(assignmentsTab, match, cells), func() error {
		if err := a.writer.Set(appName, assignmentsTab, match, cells); err != nil {
			return err
		}
		return a.logChange(actor, action, "assignment", email, year, to)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: assigned", "actor", actor, "email", email, "to", to, "year", year)
	a.mailAssignment(r, email, to)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) unassign(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
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
	match := map[string]string{"Email": email, "Year": year}
	if !a.commit(r, w, a.cache.Tables().without(assignmentsTab, match), func() error {
		if err := a.writer.Delete(appName, assignmentsTab, match); err != nil {
			return err
		}
		return a.logChange(actor, "remove", "assignment", email, year, "")
	}) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: unassigned", "actor", actor, "email", email, "year", year)
	w.WriteHeader(http.StatusNoContent)
}

// outreach records that someone has been contacted this year, or takes that
// back.
func (a app) outreach(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
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
	match := map[string]string{"Email": email, "Year": year}
	tables := a.cache.Tables()
	if !body.Contacted {
		if !a.commit(r, w, tables.without(outreachTab, match), func() error {
			if err := a.writer.Delete(appName, outreachTab, match); err != nil {
				return err
			}
			return a.logChange(actor, "remove", "outreach", email, year, "")
		}) {
			return
		}
		slog.InfoContext(r.Context(), "birthday: outreach undone", "actor", actor, "email", email, "year", year)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	cells := map[string]string{"Contacted On": today(), "Contacted By": actor}
	if !a.commit(r, w, tables.with(outreachTab, match, cells), func() error {
		if err := a.writer.Set(appName, outreachTab, match, cells); err != nil {
			return err
		}
		return a.logChange(actor, "add", "outreach", email, year, "")
	}) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: contacted", "actor", actor, "email", email, "year", year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveDonation(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
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
	match := map[string]string{"Email": email, "Year": year}
	cells := map[string]string{"Charity": charity.Name, "Note": strings.TrimSpace(body.Note), "Recorded On": today(), "Recorded By": actor}
	tables := a.cache.Tables()
	action := "edit"
	if tables.count(donationsTab, match) == 0 {
		action = "add"
	}
	if !a.commit(r, w, tables.with(donationsTab, match, cells), func() error {
		if err := a.writer.Set(appName, donationsTab, match, cells); err != nil {
			return err
		}
		return a.logChange(actor, action, "donation", email, year, charity.Name)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: saved donation", "actor", actor, "action", action, "email", email, "charity", charity.Name, "year", year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteDonation(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
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
	match := map[string]string{"Email": email, "Year": year}
	if !a.commit(r, w, a.cache.Tables().without(donationsTab, match), func() error {
		if err := a.writer.Delete(appName, donationsTab, match); err != nil {
			return err
		}
		return a.logChange(actor, "remove", "donation", email, year, "")
	}) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed donation", "actor", actor, "email", email, "year", year)
	w.WriteHeader(http.StatusNoContent)
}

// used marks this year's donation as carried in the newsletter, or takes that
// back.
func (a app) used(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
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
	match := map[string]string{"Email": email, "Year": year}
	cells := map[string]string{"Used On": "", "Used By": ""}
	action := "unused"
	if body.Used {
		cells = map[string]string{"Used On": today(), "Used By": actor}
		action = "used"
	}
	if !a.commit(r, w, a.cache.Tables().with(donationsTab, match, cells), func() error {
		if err := a.writer.Set(appName, donationsTab, match, cells); err != nil {
			return err
		}
		return a.logChange(actor, action, "donation", email, year, "")
	}) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: marked donation", "actor", actor, "action", action, "email", email, "year", year)
	w.WriteHeader(http.StatusNoContent)
}

// saveBirthday records or corrects a birthday, and the newsletter it should
// land in when the usual pick is wrong.
func (a app) saveBirthday(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
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
	match := map[string]string{"Email": email}
	cells := map[string]string{"Birthday": birthday, "Newsletter Override": override}
	tables := a.cache.Tables()
	action := "edit"
	if tables.count(birthdaysTab, match) == 0 {
		action = "add"
	}
	if !a.commit(r, w, tables.with(birthdaysTab, match, cells), func() error {
		if err := a.writer.Set(appName, birthdaysTab, match, cells); err != nil {
			return err
		}
		return a.logChange(actor, action, "birthday", email, "", birthday+" "+override)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: saved birthday", "actor", actor, "action", action, "email", email, "birthday", birthday, "override", override)
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
	match := map[string]string{"Email": email}
	tables := a.cache.Tables()
	if tables.count(birthdaysTab, match) == 0 {
		http.Error(w, "no such birthday", http.StatusNotFound)
		return
	}
	for _, tab := range []string{assignmentsTab, outreachTab, donationsTab, notesTab} {
		if tables.count(tab, match) > 0 {
			http.Error(w, "remove their assignments, outreach, donations, and notes first", http.StatusBadRequest)
			return
		}
	}
	if !a.commit(r, w, tables.without(birthdaysTab, match), func() error {
		if err := a.writer.Delete(appName, birthdaysTab, match); err != nil {
			return err
		}
		return a.logChange(actor, "remove", "birthday", email, "", "")
	}) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed birthday", "actor", actor, "email", email)
	w.WriteHeader(http.StatusNoContent)
}

// saveParticipation records that someone wants to be left out, or asked and
// donated for but kept out of the newsletter, on their Birthdays row, adding
// one without a birthday for someone opting out who has none on file.
func (a app) saveParticipation(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
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
	match := map[string]string{"Email": email}
	cells := map[string]string{"Participation": body.Level, "Note": strings.TrimSpace(body.Note)}
	tables := a.cache.Tables()
	action := "edit"
	if tables.count(birthdaysTab, match) == 0 {
		action = "add"
	}
	if !a.commit(r, w, tables.with(birthdaysTab, match, cells), func() error {
		if err := a.writer.Set(appName, birthdaysTab, match, cells); err != nil {
			return err
		}
		return a.logChange(actor, action, "participation", email, "", body.Level)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: saved participation", "actor", actor, "action", action, "email", email, "level", body.Level)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteParticipation(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
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
	match := map[string]string{"Email": email}
	cells := map[string]string{"Participation": "", "Note": ""}
	tables := a.cache.Tables()
	// A row that held only the preference has nothing left to say.
	dropRow := b.Birthday == ""
	if dropRow {
		tables = tables.without(birthdaysTab, match)
	} else {
		tables = tables.with(birthdaysTab, match, cells)
	}
	if !a.commit(r, w, tables, func() error {
		if dropRow {
			if err := a.writer.Delete(appName, birthdaysTab, match); err != nil {
				return err
			}
		} else if err := a.writer.Set(appName, birthdaysTab, match, cells); err != nil {
			return err
		}
		return a.logChange(actor, "remove", "participation", email, "", "")
	}) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed participation", "actor", actor, "email", email)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) addNote(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
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
	cells := map[string]string{"Email": email, "Note": note, "Added By": actor, "Added": today()}
	if !a.commit(r, w, a.cache.Tables().with(notesTab, nil, cells), func() error {
		if err := a.writer.Append(appName, notesTab, rowOf(NoteColumns, cells)); err != nil {
			return err
		}
		return a.logChange(actor, "add", "note", email, "", note)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: added note", "actor", actor, "email", email)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteNote(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
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
	match := map[string]string{"Email": cleanEmail(body.Email), "Note": body.Note, "Added By": cleanEmail(body.AddedBy), "Added": body.Added}
	if !a.commit(r, w, a.cache.Tables().without(notesTab, match), func() error {
		if err := a.writer.Delete(appName, notesTab, match); err != nil {
			return err
		}
		return a.logChange(actor, "remove", "note", match["Email"], "", body.Note)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed note", "actor", actor, "email", match["Email"])
	w.WriteHeader(http.StatusNoContent)
}

// saveCharity adds or edits a charity. Anyone may add one, since a staff
// member's pick often is not on the list yet; only an admin decides whether it
// is allowed.
func (a app) saveCharity(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
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
	cells := map[string]string{
		"Name": name, "Donation Link": strings.TrimSpace(body.DonationLink), "About": strings.TrimSpace(body.About),
		"EIN": strings.TrimSpace(body.EIN), "Allowed": YesNo(allowed), "Why Not Allowed": why,
	}
	tables := a.cache.Tables()
	action := "edit"
	var match map[string]string
	renamed := !adding && body.Original != name
	if (adding || renamed) && model.Charity(name) != nil {
		http.Error(w, fmt.Sprintf("%q is already on the list", name), http.StatusBadRequest)
		return
	}
	if adding {
		action = "add"
		cells["Added On"] = today()
		tables = tables.with(charitiesTab, nil, cells)
	} else {
		match = map[string]string{"Name": body.Original}
		tables = tables.with(charitiesTab, match, cells)
		if renamed {
			tables = tables.renameCharity(body.Original, name)
		}
	}
	if !a.commit(r, w, tables, func() error {
		if adding {
			if err := a.writer.Append(appName, charitiesTab, rowOf(CharityColumns, cells)); err != nil {
				return err
			}
		} else {
			if err := a.writer.Set(appName, charitiesTab, match, cells); err != nil {
				return err
			}
			if renamed {
				if err := a.flushCharityRename(body.Original, name); err != nil {
					return err
				}
			}
		}
		return a.logChange(actor, action, "charity", "", "", name)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: saved charity", "actor", actor, "action", action, "charity", name, "allowed", allowed)
	w.WriteHeader(http.StatusNoContent)
}

// renameCharity carries every donation and the default-charity setting along
// with a renamed charity, since they name it by name.
func (t *Tables) renameCharity(oldName, name string) *Tables {
	out := t
	if out.count(donationsTab, map[string]string{"Charity": oldName}) > 0 {
		out = out.with(donationsTab, map[string]string{"Charity": oldName}, map[string]string{"Charity": name})
	}
	if out.count(settingsTab, map[string]string{"Key": DefaultCharityKey, "Value": oldName}) > 0 {
		out = out.with(settingsTab, map[string]string{"Key": DefaultCharityKey}, map[string]string{"Value": name})
	}
	return out
}

func (a app) flushCharityRename(oldName, name string) error {
	tables := a.cache.Tables()
	if tables.count(donationsTab, map[string]string{"Charity": name}) > 0 {
		if err := a.writer.Set(appName, donationsTab, map[string]string{"Charity": oldName}, map[string]string{"Charity": name}); err != nil {
			return err
		}
	}
	if tables.count(settingsTab, map[string]string{"Key": DefaultCharityKey, "Value": name}) > 0 {
		if err := a.writer.Set(appName, settingsTab, map[string]string{"Key": DefaultCharityKey}, map[string]string{"Value": name}); err != nil {
			return err
		}
	}
	return nil
}

// describeCharity asks Claude for the newsletter's sentence about a charity,
// for the form to offer; nothing is saved until the person saves the form.
func (a app) describeCharity(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
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
	info, err := a.describer.Charity(r.Context(), name, link)
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
	tables := a.cache.Tables()
	if tables.count(donationsTab, map[string]string{"Charity": body.Name}) > 0 {
		http.Error(w, "donations name this charity; it can be marked not allowed instead", http.StatusBadRequest)
		return
	}
	match := map[string]string{"Name": body.Name}
	if !a.commit(r, w, tables.without(charitiesTab, match), func() error {
		if err := a.writer.Delete(appName, charitiesTab, match); err != nil {
			return err
		}
		return a.logChange(actor, "remove", "charity", "", "", body.Name)
	}) {
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
	cells := map[string]string{"Date": date}
	if !a.commit(r, w, a.cache.Tables().with(newsletterDatesTab, nil, cells), func() error {
		if err := a.writer.Append(appName, newsletterDatesTab, rowOf(NewsletterDateColumns, cells)); err != nil {
			return err
		}
		return a.logChange(actor, "add", "newsletter date", "", "", date)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: added newsletter date", "actor", actor, "date", date)
	w.WriteHeader(http.StatusNoContent)
}

// changeNewsletterDate moves one issue to another day. A birthday pinned to the
// old day by its override moves with it, so nobody is left pointing at an issue
// that no longer exists.
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
	match, cells := map[string]string{"Date": original}, map[string]string{"Date": date}
	pinned, moved := map[string]string{"Newsletter Override": original}, map[string]string{"Newsletter Override": date}
	tables := a.cache.Tables().with(newsletterDatesTab, match, cells)
	overrides := tables.count(birthdaysTab, pinned)
	if overrides > 0 {
		tables = tables.with(birthdaysTab, pinned, moved)
	}
	if !a.commit(r, w, tables, func() error {
		if err := a.writer.Set(appName, newsletterDatesTab, match, cells); err != nil {
			return err
		}
		if overrides > 0 {
			if err := a.writer.Set(appName, birthdaysTab, pinned, moved); err != nil {
				return err
			}
		}
		return a.logChange(actor, "edit", "newsletter date", "", "", original+" to "+date)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: moved newsletter date", "actor", actor, "from", original, "to", date, "overrides", overrides)
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
	match := map[string]string{"Date": strings.TrimSpace(body.Date)}
	if !a.commit(r, w, a.cache.Tables().without(newsletterDatesTab, match), func() error {
		if err := a.writer.Delete(appName, newsletterDatesTab, match); err != nil {
			return err
		}
		return a.logChange(actor, "remove", "newsletter date", "", "", match["Date"])
	}) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: removed newsletter date", "actor", actor, "date", match["Date"])
	w.WriteHeader(http.StatusNoContent)
}

// createNewsletterDates lays out a weekly run of issues: every given weekday
// from one date to another, skipping any already on the list.
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
	// The first of the weekday on or after from, then every week to the end.
	day := from.AddDate(0, 0, (body.Weekday-int(from.Weekday())+7)%7)
	have := a.cache.Model().NewsletterDates
	added := []string{}
	tables := a.cache.Tables()
	for ; !day.After(to); day = day.AddDate(0, 0, 7) {
		cell := day.Format(DateFormat)
		if slices.Contains(have, cell) {
			continue
		}
		added = append(added, cell)
		tables = tables.with(newsletterDatesTab, nil, map[string]string{"Date": cell})
	}
	if len(added) == 0 {
		http.Error(w, "every one of those dates is already on the list", http.StatusBadRequest)
		return
	}
	if !a.commit(r, w, tables, func() error {
		rows := make([][]string, 0, len(added))
		for _, cell := range added {
			rows = append(rows, []string{cell})
		}
		if err := a.writer.AppendAll(appName, newsletterDatesTab, rows); err != nil {
			return err
		}
		return a.logChange(actor, "add", "newsletter dates", "", "", fmt.Sprintf("%d, %s to %s", len(added), added[0], added[len(added)-1]))
	}) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: created newsletter dates", "actor", actor, "count", len(added), "from", added[0], "to", added[len(added)-1])
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]int{"added": len(added)}); err != nil {
		slog.ErrorContext(r.Context(), "encode created dates", "error", err)
	}
}

// clearFutureNewsletterDates removes every issue from today on, for starting
// a year's list over; the ones already out stay, since birthdays landed in them.
func (a app) clearFutureNewsletterDates(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	day := today()
	tables := a.cache.Tables()
	removed := []string{}
	for _, date := range a.cache.Model().NewsletterDates {
		if date >= day {
			removed = append(removed, date)
			tables = tables.without(newsletterDatesTab, map[string]string{"Date": date})
		}
	}
	if len(removed) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.commit(r, w, tables, func() error {
		for _, date := range removed {
			if err := a.writer.Delete(appName, newsletterDatesTab, map[string]string{"Date": date}); err != nil {
				return err
			}
		}
		return a.logChange(actor, "remove", "newsletter dates", "", "", fmt.Sprintf("%d from %s on", len(removed), day))
	}) {
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
	}
	tables := a.cache.Tables()
	for key, value := range values {
		tables = tables.with(settingsTab, map[string]string{"Key": key}, map[string]string{"Value": value})
	}
	if !a.commit(r, w, tables, func() error {
		for key, value := range values {
			if err := a.writer.Set(appName, settingsTab, map[string]string{"Key": key}, map[string]string{"Value": value}); err != nil {
				return err
			}
		}
		return a.logChange(actor, "edit", "settings", "", "", "")
	}) {
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
	// Super admins show in the merged list but never round-trip into the tab.
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
	current := a.cache.tabAdmins()
	was := map[string]bool{}
	for _, e := range current {
		was[e] = true
	}
	is := map[string]bool{}
	for _, e := range admins {
		is[e] = true
	}
	if !a.commit(r, w, a.cache.Tables().withAdmins(admins), func() error {
		for _, e := range current {
			if !is[e] {
				if err := a.writer.Delete(appName, adminsTab, map[string]string{"Email": e}); err != nil {
					return err
				}
			}
		}
		for _, e := range admins {
			if !was[e] {
				if err := a.writer.Append(appName, adminsTab, []string{e}); err != nil {
					return err
				}
			}
		}
		return nil
	}) {
		return
	}
	slog.InfoContext(r.Context(), "birthday: set the admin list", "actor", actor, "admins", admins)
	w.WriteHeader(http.StatusNoContent)
}

// teamRow reads the email and role an admin is adding or removing, refusing a
// role the tab does not know.
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
	if a.cache.Tables().count(teamTab, row) > 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.commit(r, w, a.cache.Tables().with(teamTab, nil, row), func() error {
		if err := a.writer.Append(appName, teamTab, rowOf(TeamColumns, row)); err != nil {
			return err
		}
		return a.logChange(actor, "add", "team member", row["Email"], "", row["Role"])
	}) {
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
	if !a.commit(r, w, a.cache.Tables().without(teamTab, row), func() error {
		if err := a.writer.Delete(appName, teamTab, row); err != nil {
			return err
		}
		return a.logChange(actor, "remove", "team member", row["Email"], "", row["Role"])
	}) {
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
