package events

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/serve"
)

const (
	imageFolder  = "activity-images"
	maxImageSize = 8 << 20
	shell        = "web/hca/index.html"
)

var pages = []string{
	"/{$}", "/my", "/calendar", "/all", "/people", "/people/{email}", "/admin", "/years/{year}",
	"/activities/{year}/{activity}", "/activities/{year}/{activity}/roles/{role}",
}

// local is the school's clock: the sheet's dates are wall-clock there, and a
// sign-up made late one evening is dated that evening.
var local = mustLocation("America/Los_Angeles")

func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		log.Fatalf("[ERROR] load time zone %s: %v", name, err)
	}
	return loc
}

type app struct {
	cache       *Cache
	writer      data.Writer
	queue       Enqueuer
	store       *blob.Store
	directory   Directory
	superAdmins func() []string
}

// Register wires the portal: one shell for every page, the model, and the
// writes. Every route already sits behind sign-in.
func Register(mux *http.ServeMux, cache *Cache, writer data.Writer, queue Enqueuer, store *blob.Store, directory Directory, superAdmins func() []string) {
	a := app{cache: cache, writer: writer, queue: queue, store: store, directory: directory, superAdmins: superAdmins}
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.HandleFunc("GET /api/events/model", a.model)
	mux.HandleFunc("POST /api/events/volunteer", a.saveVolunteer)
	mux.HandleFunc("DELETE /api/events/volunteer", a.removeVolunteer)
	mux.HandleFunc("POST /api/events/activity", a.saveActivity)
	mux.HandleFunc("DELETE /api/events/activity", a.deleteActivity)
	mux.HandleFunc("POST /api/events/role", a.saveRole)
	mux.HandleFunc("DELETE /api/events/role", a.deleteRole)
	mux.HandleFunc("POST /api/events/link", a.saveLink)
	mux.HandleFunc("DELETE /api/events/link", a.deleteLink)
	mux.HandleFunc("POST /api/events/category", a.saveCategory)
	mux.HandleFunc("DELETE /api/events/category", a.deleteCategory)
	mux.HandleFunc("POST /api/events/copy", a.copyActivity)
	mux.HandleFunc("POST /api/events/settings", a.saveSettings)
	mux.HandleFunc("POST /api/events/image", a.uploadImage)
	mux.HandleFunc("GET /api/admin/state", a.adminState)
	mux.HandleFunc("POST /api/admin/admins", a.setAdmins)
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

// who is the signed-in person as the portal keys them: the address Google
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

func today() string {
	return time.Now().In(local).Format(DateFormat)
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	email, admin := a.who(r)
	view := Render(a.cache.Model(), a.directory, email, admin, time.Now().In(local))
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode events model", "error", err)
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
func (a app) commit(ctx context.Context, w http.ResponseWriter, tables *Tables, flush func() error) bool {
	model, err := BuildModel(tables, a.cache.images)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	applied := make(chan struct{})
	a.queue.Add(func() {
		a.cache.set(tables, model)
		close(applied)
		if err := flush(); err != nil {
			slog.ErrorContext(ctx, "events write", "error", err)
		}
	})
	<-applied
	return true
}

func rowOf(columns []string, cells map[string]string) []string {
	row := make([]string, len(columns))
	for i, column := range columns {
		row[i] = cells[column]
	}
	return row
}

func (a app) logChange(actor, action, kind string, cells map[string]string) error {
	return a.writer.Append(appName, changeLogTab, []string{
		time.Now().Format(time.RFC3339), actor, action, kind,
		cells["Year"], cells["Activity"], cells["Role"], cells["Title"], cells["Email"], cells["Details"],
	})
}

type activityRef struct {
	Year  string `json:"year"`
	Title string `json:"title"`
}

func (a app) findActivity(w http.ResponseWriter, year, title string) (*Activity, bool) {
	act := a.cache.Model().Activity(year, title)
	if act == nil {
		http.Error(w, fmt.Sprintf("no activity %q in %s", title, year), http.StatusNotFound)
		return nil, false
	}
	return act, true
}

func (a app) saveVolunteer(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		Year     string `json:"year"`
		Activity string `json:"activity"`
		Role     string `json:"role"`
		Email    string `json:"email"`
		Position string `json:"position"`
		Note     string `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	act, ok := a.findActivity(w, body.Year, body.Activity)
	if !ok {
		return
	}
	editor := admin || act.IsCoChair(actor)
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if email == "" {
		email = actor
	}
	if !emailForm.MatchString(email) {
		http.Error(w, "that is not an email address", http.StatusBadRequest)
		return
	}
	status, spots := act.Status, act.Spots
	if body.Role != "" {
		role := act.Role(body.Role)
		if role == nil {
			http.Error(w, fmt.Sprintf("no role %q on %s", body.Role, act.Title), http.StatusNotFound)
			return
		}
		status, spots = role.Status, role.Spots
	} else if !act.DirectSignUp && !editor {
		http.Error(w, "sign up for one of its roles instead", http.StatusBadRequest)
		return
	}
	if !editor && status != StatusOpen {
		http.Error(w, "this is not open for sign-ups", http.StatusBadRequest)
		return
	}
	if !slices.Contains(Positions, body.Position) {
		http.Error(w, "position must be one of "+strings.Join(Positions, ", "), http.StatusBadRequest)
		return
	}
	if !editor && body.Position == PositionCoChair {
		http.Error(w, "only a co-chair or admin can name a co-chair", http.StatusForbidden)
		return
	}
	if len(body.Note) > maxTextLength {
		http.Error(w, "the note is too long", http.StatusBadRequest)
		return
	}
	match := map[string]string{"Year": act.Year, "Activity": act.Title, "Role": body.Role, "Email": email}
	tables := a.cache.Tables()
	existing := tables.count(volunteersTab, match) > 0
	if existing && !editor && email != actor {
		http.Error(w, "only a co-chair or admin can change someone else's sign-up", http.StatusForbidden)
		return
	}
	if !existing && !editor && spots > 0 {
		taken := tables.count(volunteersTab, map[string]string{"Year": act.Year, "Activity": act.Title, "Role": body.Role})
		if taken >= spots {
			http.Error(w, "every spot is taken", http.StatusBadRequest)
			return
		}
	}
	cells := map[string]string{"Position": body.Position, "Note": strings.TrimSpace(body.Note)}
	action := "edit"
	if !existing {
		action = "add"
		cells["Added By"] = actor
		cells["Added"] = today()
	}
	if !a.commit(r.Context(), w, tables.with(volunteersTab, match, cells), func() error {
		if err := a.writer.Set(appName, volunteersTab, match, cells); err != nil {
			return err
		}
		return a.logChange(actor, action, "volunteer", map[string]string{
			"Year": act.Year, "Activity": act.Title, "Role": body.Role, "Email": email, "Details": body.Position,
		})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved volunteer", "actor", actor, "action", action, "email", email, "activity", act.Title, "role", body.Role, "year", act.Year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) removeVolunteer(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		Year     string `json:"year"`
		Activity string `json:"activity"`
		Role     string `json:"role"`
		Email    string `json:"email"`
	}
	if !decode(w, r, &body) {
		return
	}
	act, ok := a.findActivity(w, body.Year, body.Activity)
	if !ok {
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if email != actor && !admin && !act.IsCoChair(actor) {
		http.Error(w, "only a co-chair or admin can remove someone else", http.StatusForbidden)
		return
	}
	match := map[string]string{"Year": act.Year, "Activity": act.Title, "Role": body.Role, "Email": email}
	if !a.commit(r.Context(), w, a.cache.Tables().without(volunteersTab, match), func() error {
		if err := a.writer.Delete(appName, volunteersTab, match); err != nil {
			return err
		}
		return a.logChange(actor, "remove", "volunteer", map[string]string{
			"Year": act.Year, "Activity": act.Title, "Role": body.Role, "Email": email,
		})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: removed volunteer", "actor", actor, "email", email, "activity", act.Title, "role", body.Role, "year", act.Year)
	w.WriteHeader(http.StatusNoContent)
}

type activityBody struct {
	Original         activityRef `json:"original"`
	Year             string      `json:"year"`
	Title            string      `json:"title"`
	Category         string      `json:"category"`
	Status           string      `json:"status"`
	Description      string      `json:"description"`
	Image            string      `json:"image"`
	Timing           string      `json:"timing"`
	Start            string      `json:"start"`
	End              string      `json:"end"`
	Location         string      `json:"location"`
	Spots            int         `json:"spots"`
	CoLeaderNeeded   bool        `json:"coLeaderNeeded"`
	VolunteersHidden bool        `json:"volunteersHidden"`
	DirectSignUp     bool        `json:"directSignUp"`
	CoChair          bool        `json:"coChair"`
}

func spotsCell(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// saveActivity adds or edits an activity. Anyone may propose one, which lands
// as Pending until an admin opens it; only an admin or one of its co-chairs may
// change an existing one, and only an admin may approve, hide, or move it
// between years.
func (a app) saveActivity(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body activityBody
	if !decode(w, r, &body) {
		return
	}
	title := strings.TrimSpace(body.Title)
	year := strings.TrimSpace(body.Year)
	adding := body.Original.Title == ""
	var current *Activity
	if !adding {
		act, ok := a.findActivity(w, body.Original.Year, body.Original.Title)
		if !ok {
			return
		}
		current = act
		if !admin && !act.IsCoChair(actor) {
			http.Error(w, "only a co-chair or admin can edit this", http.StatusForbidden)
			return
		}
	}
	status := body.Status
	switch {
	case adding && !admin:
		status = StatusPending
	case adding && status == "":
		status = StatusOpen
	case !adding && !admin:
		if current.Status == StatusPending {
			status = StatusPending
		} else if status != StatusOpen && status != StatusDone {
			http.Error(w, "a co-chair may only mark this open or done", http.StatusBadRequest)
			return
		}
		year = current.Year
	}
	cells := map[string]string{
		"Year": year, "Title": title, "Category": strings.TrimSpace(body.Category), "Status": status,
		"Description": strings.TrimSpace(body.Description), "Image": strings.TrimSpace(body.Image),
		"Timing": strings.TrimSpace(body.Timing), "Start": strings.TrimSpace(body.Start), "End": strings.TrimSpace(body.End),
		"Location": strings.TrimSpace(body.Location), "Spots": spotsCell(body.Spots),
		"Co-Leader Needed": YesNo(body.CoLeaderNeeded), "Volunteers Hidden": YesNo(body.VolunteersHidden),
		"Direct Sign-Up": YesNo(body.DirectSignUp),
	}
	tables := a.cache.Tables()
	action := "edit"
	var match map[string]string
	if adding {
		action = "add"
		cells["Added By"] = actor
		cells["Added"] = today()
		if a.cache.Model().Activity(year, title) != nil {
			http.Error(w, fmt.Sprintf("%q already exists in %s", title, year), http.StatusBadRequest)
			return
		}
		tables = tables.with(activitiesTab, nil, cells)
		if body.CoChair {
			tables = tables.with(volunteersTab, nil, map[string]string{
				"Year": year, "Activity": title, "Email": actor, "Position": PositionOpen, "Added By": actor, "Added": today(),
			})
		}
	} else {
		match = map[string]string{"Year": current.Year, "Title": current.Title}
		tables = tables.with(activitiesTab, match, cells)
		if current.Year != year || current.Title != title {
			if a.cache.Model().Activity(year, title) != nil {
				http.Error(w, fmt.Sprintf("%q already exists in %s", title, year), http.StatusBadRequest)
				return
			}
			tables = tables.renameActivity(current.Year, current.Title, year, title)
		}
	}
	if !a.commit(r.Context(), w, tables, func() error {
		if adding {
			if err := a.writer.Append(appName, activitiesTab, rowOf(ActivityColumns, cells)); err != nil {
				return err
			}
			if body.CoChair {
				if err := a.writer.Append(appName, volunteersTab, rowOf(VolunteerColumns, map[string]string{
					"Year": year, "Activity": title, "Email": actor, "Position": PositionOpen, "Added By": actor, "Added": today(),
				})); err != nil {
					return err
				}
			}
		} else {
			if err := a.writer.Set(appName, activitiesTab, match, cells); err != nil {
				return err
			}
			if current.Year != year || current.Title != title {
				if err := a.flushRename(current.Year, current.Title, year, title); err != nil {
					return err
				}
			}
		}
		return a.logChange(actor, action, "activity", map[string]string{"Year": year, "Activity": title, "Details": status})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved activity", "actor", actor, "action", action, "activity", title, "year", year, "status", status)
	w.WriteHeader(http.StatusNoContent)
}

// renameActivity carries every role, volunteer, and link along with a renamed
// or moved activity, since they all name it by year and title.
func (t *Tables) renameActivity(oldYear, oldTitle, year, title string) *Tables {
	match := map[string]string{"Year": oldYear, "Activity": oldTitle}
	cells := map[string]string{"Year": year, "Activity": title}
	out := t
	for _, tab := range []string{rolesTab, volunteersTab, linksTab} {
		if out.count(tab, match) > 0 {
			out = out.with(tab, match, cells)
		}
	}
	return out
}

func (a app) flushRename(oldYear, oldTitle, year, title string) error {
	match := map[string]string{"Year": oldYear, "Activity": oldTitle}
	cells := map[string]string{"Year": year, "Activity": title}
	for _, tab := range []string{rolesTab, volunteersTab, linksTab} {
		if a.cache.Tables().count(tab, map[string]string{"Year": year, "Activity": title}) == 0 {
			continue
		}
		if err := a.writer.Set(appName, tab, match, cells); err != nil {
			return err
		}
	}
	return nil
}

func (a app) deleteActivity(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body activityRef
	if !decode(w, r, &body) {
		return
	}
	act, ok := a.findActivity(w, body.Year, body.Title)
	if !ok {
		return
	}
	match := map[string]string{"Year": act.Year, "Activity": act.Title}
	tables := a.cache.Tables()
	if tables.count(volunteersTab, match) > 0 {
		http.Error(w, "remove its volunteers first", http.StatusBadRequest)
		return
	}
	self := map[string]string{"Year": act.Year, "Title": act.Title}
	tables = tables.without(rolesTab, match).without(linksTab, match).without(activitiesTab, self)
	if !a.commit(r.Context(), w, tables, func() error {
		for _, tab := range []string{rolesTab, linksTab} {
			if err := a.writer.Delete(appName, tab, match); err != nil {
				return err
			}
		}
		if err := a.writer.Delete(appName, activitiesTab, self); err != nil {
			return err
		}
		return a.logChange(actor, "delete", "activity", map[string]string{"Year": act.Year, "Activity": act.Title})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: deleted activity", "actor", actor, "activity", act.Title, "year", act.Year)
	w.WriteHeader(http.StatusNoContent)
}

type roleBody struct {
	Year             string `json:"year"`
	Activity         string `json:"activity"`
	Original         string `json:"original"`
	Title            string `json:"title"`
	Parent           string `json:"parent"`
	Group            string `json:"group"`
	Status           string `json:"status"`
	Description      string `json:"description"`
	Image            string `json:"image"`
	Start            string `json:"start"`
	End              string `json:"end"`
	Spots            int    `json:"spots"`
	CoLeaderNeeded   bool   `json:"coLeaderNeeded"`
	VolunteersHidden bool   `json:"volunteersHidden"`
	CoChair          bool   `json:"coChair"`
}

// saveRole adds or edits a role under an activity. Anyone may suggest one on an
// open activity, which lands as Pending; the activity's editors add them open
// and change them.
func (a app) saveRole(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body roleBody
	if !decode(w, r, &body) {
		return
	}
	act, ok := a.findActivity(w, body.Year, body.Activity)
	if !ok {
		return
	}
	editor := admin || act.IsCoChair(actor)
	title := strings.TrimSpace(body.Title)
	adding := body.Original == ""
	if !adding && !editor {
		http.Error(w, "only a co-chair or admin can edit this", http.StatusForbidden)
		return
	}
	if adding && !editor && act.Status != StatusOpen {
		http.Error(w, "this activity is not open for suggestions", http.StatusBadRequest)
		return
	}
	status := body.Status
	if !editor {
		status = StatusPending
	} else if status == "" {
		status = StatusOpen
	}
	cells := map[string]string{
		"Year": act.Year, "Activity": act.Title, "Parent": strings.TrimSpace(body.Parent), "Title": title,
		"Group": strings.TrimSpace(body.Group), "Status": status, "Description": strings.TrimSpace(body.Description),
		"Image": strings.TrimSpace(body.Image), "Start": strings.TrimSpace(body.Start), "End": strings.TrimSpace(body.End),
		"Spots": spotsCell(body.Spots), "Co-Leader Needed": YesNo(body.CoLeaderNeeded), "Volunteers Hidden": YesNo(body.VolunteersHidden),
	}
	tables := a.cache.Tables()
	action := "edit"
	var match map[string]string
	renamed := !adding && body.Original != title
	if adding || renamed {
		if act.Role(title) != nil {
			http.Error(w, fmt.Sprintf("%q already has a role called %q", act.Title, title), http.StatusBadRequest)
			return
		}
	}
	volunteer := map[string]string{
		"Year": act.Year, "Activity": act.Title, "Role": title, "Email": actor, "Position": PositionOpen, "Added By": actor, "Added": today(),
	}
	if adding {
		action = "add"
		cells["Added By"] = actor
		cells["Added"] = today()
		tables = tables.with(rolesTab, nil, cells)
		if body.CoChair {
			tables = tables.with(volunteersTab, nil, volunteer)
		}
	} else {
		match = map[string]string{"Year": act.Year, "Activity": act.Title, "Title": body.Original}
		tables = tables.with(rolesTab, match, cells)
		if renamed {
			tables = tables.renameRole(act.Year, act.Title, body.Original, title)
		}
	}
	if !a.commit(r.Context(), w, tables, func() error {
		if adding {
			if err := a.writer.Append(appName, rolesTab, rowOf(RoleColumns, cells)); err != nil {
				return err
			}
			if body.CoChair {
				if err := a.writer.Append(appName, volunteersTab, rowOf(VolunteerColumns, volunteer)); err != nil {
					return err
				}
			}
		} else {
			if err := a.writer.Set(appName, rolesTab, match, cells); err != nil {
				return err
			}
			if renamed {
				if err := a.flushRoleRename(act.Year, act.Title, body.Original, title); err != nil {
					return err
				}
			}
		}
		return a.logChange(actor, action, "role", map[string]string{"Year": act.Year, "Activity": act.Title, "Role": title, "Details": status})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved role", "actor", actor, "action", action, "role", title, "activity", act.Title, "year", act.Year, "status", status)
	w.WriteHeader(http.StatusNoContent)
}

// renameRole carries sub-roles, volunteers, and links along with a renamed role.
func (t *Tables) renameRole(year, activity, oldTitle, title string) *Tables {
	base := map[string]string{"Year": year, "Activity": activity}
	out := t
	for _, step := range []struct{ tab, column string }{{rolesTab, "Parent"}, {volunteersTab, "Role"}, {linksTab, "Role"}} {
		match := map[string]string{"Year": base["Year"], "Activity": base["Activity"], step.column: oldTitle}
		if out.count(step.tab, match) > 0 {
			out = out.with(step.tab, match, map[string]string{step.column: title})
		}
	}
	return out
}

func (a app) flushRoleRename(year, activity, oldTitle, title string) error {
	for _, step := range []struct{ tab, column string }{{rolesTab, "Parent"}, {volunteersTab, "Role"}, {linksTab, "Role"}} {
		if a.cache.Tables().count(step.tab, map[string]string{"Year": year, "Activity": activity, step.column: title}) == 0 {
			continue
		}
		match := map[string]string{"Year": year, "Activity": activity, step.column: oldTitle}
		if err := a.writer.Set(appName, step.tab, match, map[string]string{step.column: title}); err != nil {
			return err
		}
	}
	return nil
}

func (a app) deleteRole(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		Year     string `json:"year"`
		Activity string `json:"activity"`
		Title    string `json:"title"`
	}
	if !decode(w, r, &body) {
		return
	}
	act, ok := a.findActivity(w, body.Year, body.Activity)
	if !ok {
		return
	}
	if !admin && !act.IsCoChair(actor) {
		http.Error(w, "only a co-chair or admin can delete this", http.StatusForbidden)
		return
	}
	role := act.Role(body.Title)
	if role == nil {
		http.Error(w, "no such role", http.StatusNotFound)
		return
	}
	tables := a.cache.Tables()
	if len(role.Roles) > 0 {
		http.Error(w, "delete the roles under it first", http.StatusBadRequest)
		return
	}
	if tables.count(volunteersTab, map[string]string{"Year": act.Year, "Activity": act.Title, "Role": role.Title}) > 0 {
		http.Error(w, "remove its volunteers first", http.StatusBadRequest)
		return
	}
	self := map[string]string{"Year": act.Year, "Activity": act.Title, "Title": role.Title}
	links := map[string]string{"Year": act.Year, "Activity": act.Title, "Role": role.Title}
	tables = tables.without(linksTab, links).without(rolesTab, self)
	if !a.commit(r.Context(), w, tables, func() error {
		if err := a.writer.Delete(appName, linksTab, links); err != nil {
			return err
		}
		if err := a.writer.Delete(appName, rolesTab, self); err != nil {
			return err
		}
		return a.logChange(actor, "delete", "role", map[string]string{"Year": act.Year, "Activity": act.Title, "Role": role.Title})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: deleted role", "actor", actor, "role", role.Title, "activity", act.Title, "year", act.Year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveLink(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		Year     string `json:"year"`
		Activity string `json:"activity"`
		Role     string `json:"role"`
		Original string `json:"original"`
		Title    string `json:"title"`
		URL      string `json:"url"`
		Image    string `json:"image"`
	}
	if !decode(w, r, &body) {
		return
	}
	act, ok := a.findActivity(w, body.Year, body.Activity)
	if !ok {
		return
	}
	if !admin && !act.IsCoChair(actor) {
		http.Error(w, "only a co-chair or admin can add links", http.StatusForbidden)
		return
	}
	title := strings.TrimSpace(body.Title)
	cells := map[string]string{
		"Year": act.Year, "Activity": act.Title, "Role": body.Role, "Title": title,
		"URL": strings.TrimSpace(body.URL), "Image": strings.TrimSpace(body.Image),
	}
	tables := a.cache.Tables()
	action := "edit"
	var match map[string]string
	if body.Original == "" {
		action = "add"
		tables = tables.with(linksTab, nil, cells)
	} else {
		match = map[string]string{"Year": act.Year, "Activity": act.Title, "Role": body.Role, "Title": body.Original}
		tables = tables.with(linksTab, match, cells)
	}
	if !a.commit(r.Context(), w, tables, func() error {
		if body.Original == "" {
			if err := a.writer.Append(appName, linksTab, rowOf(LinkColumns, cells)); err != nil {
				return err
			}
		} else if err := a.writer.Set(appName, linksTab, match, cells); err != nil {
			return err
		}
		return a.logChange(actor, action, "link", map[string]string{"Year": act.Year, "Activity": act.Title, "Role": body.Role, "Title": title, "Details": cells["URL"]})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved link", "actor", actor, "action", action, "link", title, "activity", act.Title, "year", act.Year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteLink(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		Year     string `json:"year"`
		Activity string `json:"activity"`
		Role     string `json:"role"`
		Title    string `json:"title"`
	}
	if !decode(w, r, &body) {
		return
	}
	act, ok := a.findActivity(w, body.Year, body.Activity)
	if !ok {
		return
	}
	if !admin && !act.IsCoChair(actor) {
		http.Error(w, "only a co-chair or admin can remove links", http.StatusForbidden)
		return
	}
	match := map[string]string{"Year": act.Year, "Activity": act.Title, "Role": body.Role, "Title": body.Title}
	if !a.commit(r.Context(), w, a.cache.Tables().without(linksTab, match), func() error {
		if err := a.writer.Delete(appName, linksTab, match); err != nil {
			return err
		}
		return a.logChange(actor, "delete", "link", map[string]string{"Year": act.Year, "Activity": act.Title, "Role": body.Role, "Title": body.Title})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: deleted link", "actor", actor, "link", body.Title, "activity", act.Title, "year", act.Year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveCategory(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Original    string `json:"original"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if !decode(w, r, &body) {
		return
	}
	title := strings.TrimSpace(body.Title)
	cells := map[string]string{"Title": title, "Description": strings.TrimSpace(body.Description)}
	tables := a.cache.Tables()
	action := "edit"
	var match map[string]string
	renamed := body.Original != "" && body.Original != title
	if body.Original == "" {
		action = "add"
		tables = tables.with(categoriesTab, nil, cells)
	} else {
		match = map[string]string{"Title": body.Original}
		tables = tables.with(categoriesTab, match, cells)
		if renamed {
			tables = tables.with(activitiesTab, map[string]string{"Category": body.Original}, map[string]string{"Category": title})
		}
	}
	if !a.commit(r.Context(), w, tables, func() error {
		if body.Original == "" {
			if err := a.writer.Append(appName, categoriesTab, rowOf(CategoryColumns, cells)); err != nil {
				return err
			}
		} else {
			if err := a.writer.Set(appName, categoriesTab, match, cells); err != nil {
				return err
			}
			if renamed && a.cache.Tables().count(activitiesTab, map[string]string{"Category": title}) > 0 {
				if err := a.writer.Set(appName, activitiesTab, map[string]string{"Category": body.Original}, map[string]string{"Category": title}); err != nil {
					return err
				}
			}
		}
		return a.logChange(actor, action, "category", map[string]string{"Title": title})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved category", "actor", actor, "action", action, "category", title)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteCategory(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	if !decode(w, r, &body) {
		return
	}
	tables := a.cache.Tables()
	if tables.count(activitiesTab, map[string]string{"Category": body.Title}) > 0 {
		http.Error(w, "move or delete its activities first", http.StatusBadRequest)
		return
	}
	match := map[string]string{"Title": body.Title}
	if !a.commit(r.Context(), w, tables.without(categoriesTab, match), func() error {
		if err := a.writer.Delete(appName, categoriesTab, match); err != nil {
			return err
		}
		return a.logChange(actor, "delete", "category", map[string]string{"Title": body.Title})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: deleted category", "actor", actor, "category", body.Title)
	w.WriteHeader(http.StatusNoContent)
}

// copyActivity carries an activity, its roles, and its links into the next
// school year, open and undated, so a recurring event starts from last year's
// shape rather than from nothing. Volunteers are not copied: sign-ups are the
// point of the new year.
func (a app) copyActivity(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body activityRef
	if !decode(w, r, &body) {
		return
	}
	act, ok := a.findActivity(w, body.Year, body.Title)
	if !ok {
		return
	}
	year := ShiftYear(act.Year, 1)
	if a.cache.Model().Activity(year, act.Title) != nil {
		http.Error(w, fmt.Sprintf("%q already exists in %s", act.Title, year), http.StatusBadRequest)
		return
	}
	activity := map[string]string{
		"Year": year, "Title": act.Title, "Category": act.Category, "Status": StatusOpen,
		"Description": act.Description, "Image": act.Image, "Timing": act.Timing, "Location": act.Location,
		"Spots": spotsCell(act.Spots), "Co-Leader Needed": YesNo(act.CoLeaderNeeded),
		"Volunteers Hidden": YesNo(act.VolunteersHidden), "Direct Sign-Up": YesNo(act.DirectSignUp),
		"Added By": actor, "Added": today(),
	}
	roles := []map[string]string{}
	for _, role := range act.AllRoles() {
		if role.Status == StatusPending {
			continue
		}
		roles = append(roles, map[string]string{
			"Year": year, "Activity": act.Title, "Parent": role.Parent, "Title": role.Title, "Group": role.Group,
			"Status": role.Status, "Description": role.Description, "Image": role.Image, "Spots": spotsCell(role.Spots),
			"Co-Leader Needed": YesNo(role.CoLeaderNeeded), "Volunteers Hidden": YesNo(role.VolunteersHidden),
			"Added By": actor, "Added": today(),
		})
	}
	links := []map[string]string{}
	for _, l := range act.Links {
		links = append(links, map[string]string{"Year": year, "Activity": act.Title, "Title": l.Title, "URL": l.URL, "Image": l.Image})
	}
	for _, role := range act.AllRoles() {
		for _, l := range role.Links {
			links = append(links, map[string]string{"Year": year, "Activity": act.Title, "Role": role.Title, "Title": l.Title, "URL": l.URL, "Image": l.Image})
		}
	}
	tables := a.cache.Tables().with(activitiesTab, nil, activity)
	for _, row := range roles {
		tables = tables.with(rolesTab, nil, row)
	}
	for _, row := range links {
		tables = tables.with(linksTab, nil, row)
	}
	if !a.commit(r.Context(), w, tables, func() error {
		if err := a.writer.Append(appName, activitiesTab, rowOf(ActivityColumns, activity)); err != nil {
			return err
		}
		for _, row := range roles {
			if err := a.writer.Append(appName, rolesTab, rowOf(RoleColumns, row)); err != nil {
				return err
			}
		}
		for _, row := range links {
			if err := a.writer.Append(appName, linksTab, rowOf(LinkColumns, row)); err != nil {
				return err
			}
		}
		return a.logChange(actor, "copy", "activity", map[string]string{"Year": year, "Activity": act.Title, "Details": "from " + act.Year})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: copied activity", "actor", actor, "activity", act.Title, "from", act.Year, "to", year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveSettings(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		ExpenseFormURL string `json:"expenseFormUrl"`
		Intro          string `json:"intro"`
	}
	if !decode(w, r, &body) {
		return
	}
	values := map[string]string{ExpenseFormKey: strings.TrimSpace(body.ExpenseFormURL), IntroKey: strings.TrimSpace(body.Intro)}
	tables := a.cache.Tables()
	for key, value := range values {
		tables = tables.with(settingsTab, map[string]string{"Key": key}, map[string]string{"Value": value})
	}
	if !a.commit(r.Context(), w, tables, func() error {
		for key, value := range values {
			if err := a.writer.Set(appName, settingsTab, map[string]string{"Key": key}, map[string]string{"Value": value}); err != nil {
				return err
			}
		}
		return a.logChange(actor, "edit", "settings", nil)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: changed the settings", "actor", actor)
	w.WriteHeader(http.StatusNoContent)
}

// uploadImage stores a content-addressed image and returns the name the sheet
// should record; the save that follows references it.
func (a app) uploadImage(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		http.Error(w, "image uploads require real-data mode", http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImageSize)
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "an image file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "could not read the image", http.StatusBadRequest)
		return
	}
	mimeType := http.DetectContentType(content)
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp"}[mimeType]
	if ext == "" {
		http.Error(w, fmt.Sprintf("%s is not a supported image", header.Filename), http.StatusBadRequest)
		return
	}
	sum := sha256.Sum256(content)
	name := hex.EncodeToString(sum[:]) + ext
	if err := a.store.Put(imageFolder, name, mimeType, content); err != nil {
		slog.ErrorContext(r.Context(), "store activity image", "error", err)
		http.Error(w, "could not store the image", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"name": imageFolder + "/" + name}); err != nil {
		slog.ErrorContext(r.Context(), "encode image name", "error", err)
	}
}

func (a app) adminState(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	view := struct {
		Email    string   `json:"email"`
		HasStore bool     `json:"hasStore"`
		Admins   []string `json:"admins"`
	}{Email: email, HasStore: a.store != nil, Admins: a.cache.Admins(a.superAdmins())}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode events admin state", "error", err)
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
	if !a.commit(r.Context(), w, a.cache.Tables().withAdmins(admins), func() error {
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
	slog.InfoContext(r.Context(), "events: set the admin list", "actor", actor, "admins", admins)
	w.WriteHeader(http.StatusNoContent)
}

func normalizeEmails(emails []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, e := range emails {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" || !strings.Contains(e, "@") || seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	return out
}
