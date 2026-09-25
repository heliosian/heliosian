package team

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/imagesearch"
	"heliosian/internal/logging"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	imageFolder  = "activity-images"
	maxImageSize = 8 << 20
	shell        = "web/team/index.html"
)

var pages = []string{
	"/{$}", "/my", "/my/{email}", "/calendar", "/approvals", "/admin", "/years/{year}", "/activities/{path...}", "/v/{path...}",
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
	media       *blob.Store
	directory   Directory
	superAdmins func() []string
	search      ImageSearch
	mailer      mail.Sender
	from        string
	rsvps       RSVPLookup
}

type ImageSearch = imagesearch.Search

func (a app) importImage(w http.ResponseWriter, r *http.Request) {
	a.search.ServeImport(w, r, imageFolder, maxImageSize)
}

func Register(mux *http.ServeMux, cache *Cache, media *blob.Store, directory Directory, superAdmins func() []string, search ImageSearch, mailer mail.Sender, from string, rsvps RSVPLookup) {
	if search.UserAgent == "" {
		search.UserAgent = "HCA-Team image search (+https://team.heliosian.com)"
	}
	a := app{cache: cache, media: media, directory: directory, superAdmins: superAdmins, search: search, mailer: mailer, from: from, rsvps: rsvps}
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.HandleFunc("GET /api/team/model", a.model)
	mux.HandleFunc("GET /api/team/images/search", a.search.ServeSearch)
	mux.HandleFunc("GET /api/team/images/thumb", a.search.ServeThumb)
	mux.HandleFunc("POST /api/team/images/import", a.importImage)
	mux.HandleFunc("GET /open/share/upcoming.png", a.shareUpcoming)
	mux.HandleFunc("GET /open/share/{id}", a.shareCard)
	mux.HandleFunc("GET /api/team/people", a.people)
	mux.HandleFunc("POST /api/team/volunteer", a.saveVolunteer)
	mux.HandleFunc("DELETE /api/team/volunteer", a.removeVolunteer)
	mux.HandleFunc("POST /api/team/activity", a.saveActivity)
	mux.HandleFunc("POST /api/team/order", a.orderChildren)
	mux.HandleFunc("DELETE /api/team/activity", a.deleteActivity)
	mux.HandleFunc("POST /api/team/link", a.saveLink)
	mux.HandleFunc("DELETE /api/team/link", a.deleteLink)
	mux.HandleFunc("POST /api/team/category", a.saveCategory)
	mux.HandleFunc("DELETE /api/team/category", a.deleteCategory)
	mux.HandleFunc("POST /api/team/categories/order", a.reorderCategories)
	mux.HandleFunc("POST /api/team/copy", a.copyActivity)
	mux.HandleFunc("POST /api/team/settings", a.saveSettings)
	mux.HandleFunc("POST /api/team/notify", a.saveNotify)
	mux.HandleFunc("POST /api/team/image", a.uploadImage)
	mux.HandleFunc("GET /api/admin/state", a.adminState)
	mux.HandleFunc("POST /api/admin/admins", a.setAdmins)
	mux.HandleFunc("POST /api/team/redirect", a.saveRedirect)
	mux.HandleFunc("DELETE /api/team/redirect", a.deleteRedirect)
}

func Redirected(cache *Cache, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			if to := cache.Model().Destination(r.URL.Path); to != "" {
				if r.URL.RawQuery != "" && !strings.Contains(to, "?") {
					to += "?" + r.URL.RawQuery
				}
				http.Redirect(w, r, to, http.StatusFound)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
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

func today() string {
	return time.Now().In(local).Format(DateFormat)
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	email, admin := a.who(r)
	view := RenderWith(a.cache.Model(), a.directory, a.rsvps, email, admin, time.Now().In(local))
	view.ImageSearch = a.search.On()
	view.User.IsSuperAdmin = a.cache.IsSuperAdmin(email)
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

func (a app) commit(w http.ResponseWriter, r *http.Request, actor string, ops ...store.Op) bool {
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

type activityRef struct {
	ID string `json:"id"`
}

func (a app) findActivity(w http.ResponseWriter, id string) (*Activity, bool) {
	act := a.cache.Model().Activity(strings.TrimSpace(id))
	if act == nil {
		http.Error(w, fmt.Sprintf("no activity with id %q", id), http.StatusNotFound)
		return nil, false
	}
	return act, true
}

func newID() string {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	out := make([]byte, len(raw))
	for i, b := range raw {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out)
}

func (a app) people(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(a.directory.People()); err != nil {
		slog.ErrorContext(r.Context(), "events: encode people", "error", err)
	}
}

func (a app) saveVolunteer(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		ID       string `json:"id"`
		Email    string `json:"email"`
		Position string `json:"position"`
		Note     string `json:"note"`
		From     string `json:"from"`
	}
	if !decode(w, r, &body) {
		return
	}
	act, ok := a.findActivity(w, body.ID)
	if !ok {
		return
	}
	model := a.cache.Model()
	editor := admin || model.Runs(act, actor)
	var from *Activity
	if strings.TrimSpace(body.From) != "" && strings.TrimSpace(body.From) != act.ID {
		if from, ok = a.findActivity(w, body.From); !ok {
			return
		}
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if email == "" {
		email = actor
	}
	if !emailForm.MatchString(email) {
		http.Error(w, "that is not an email address", http.StatusBadRequest)
		return
	}
	offering := body.Position == PositionOpen && act.CoLeaderNeeded
	if !act.DirectSignUp && !editor && !offering {
		http.Error(w, "sign up for one of the things under it instead", http.StatusBadRequest)
		return
	}
	if !editor && act.Status != StatusOpen {
		http.Error(w, "this is not open for sign-ups", http.StatusBadRequest)
		return
	}
	if !slices.Contains(Positions, body.Position) {
		http.Error(w, "position must be one of "+strings.Join(Positions, ", "), http.StatusBadRequest)
		return
	}
	if len(body.Note) > maxTextLength {
		http.Error(w, "the note is too long", http.StatusBadRequest)
		return
	}
	ops := []store.Op{}
	current := act.volunteer(email)
	was := ""
	if current != nil {
		was = current.Position
	}
	if from != nil {
		moving := from.volunteer(email)
		if moving == nil {
			http.Error(w, fmt.Sprintf("%s is not signed up for %s", email, from.Title), http.StatusBadRequest)
			return
		}
		if email != actor && !admin && !model.Runs(from, actor) && !a.household(actor, email) {
			http.Error(w, "only a co-chair or admin can move someone else's sign-up", http.StatusForbidden)
			return
		}
		if was == "" {
			was = moving.Position
		}
		ops = append(ops, store.Delete(volunteersTab, store.Row{"Event ID": from.ID, "Email": email}))
	}
	existing := current != nil
	if !editor {
		if (body.Position == PositionCoChair) != (was == PositionCoChair) {
			http.Error(w, "only a co-chair or admin can make or unmake a co-chair", http.StatusForbidden)
			return
		}
		if body.Position == PositionOpen && was != PositionOpen && !act.CoLeaderNeeded {
			http.Error(w, "this is not looking for a co-chair", http.StatusBadRequest)
			return
		}
	}
	if existing && !editor && email != actor && !a.household(actor, email) {
		http.Error(w, "only a co-chair or admin can change someone else's sign-up", http.StatusForbidden)
		return
	}
	if !existing && !editor && act.Spots > 0 && len(act.Volunteers) >= act.Spots {
		http.Error(w, "every spot is taken", http.StatusBadRequest)
		return
	}
	cells := store.Row{"Position": body.Position, "Note": strings.TrimSpace(body.Note)}
	action := "edit"
	if !existing {
		action = "add"
		cells["Added By"] = actor
		cells["Added"] = today()
	}
	if from != nil {
		action = "move"
	}
	ops = append(ops, store.Set(volunteersTab, store.Row{"Event ID": act.ID, "Email": email}, cells))
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved volunteer", "actor", actor, "action", action, "email", email, "activity", act.Title, "year", act.Year)
	a.mailSignUp(r, act, email, body.Position, strings.TrimSpace(body.Note), actor, existing || from != nil, was)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) household(actor, email string) bool {
	adults, kids := a.directory.Household(actor)
	for _, c := range append(adults, kids...) {
		if strings.EqualFold(c.Email, email) {
			return true
		}
	}
	return false
}

func (a app) removeVolunteer(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	if !decode(w, r, &body) {
		return
	}
	act, ok := a.findActivity(w, body.ID)
	if !ok {
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if email != actor && !admin && !a.cache.Model().Runs(act, actor) && !a.household(actor, email) {
		http.Error(w, "only a co-chair or admin can remove someone else", http.StatusForbidden)
		return
	}
	if !a.commit(w, r, actor, store.Delete(volunteersTab, store.Row{"Event ID": act.ID, "Email": email})) {
		return
	}
	slog.InfoContext(r.Context(), "events: removed volunteer", "actor", actor, "email", email, "activity", act.Title, "year", act.Year)
	a.mailRemoved(r, act, email)
	w.WriteHeader(http.StatusNoContent)
}

type activityBody struct {
	ID                 string     `json:"id"`
	Year               string     `json:"year"`
	Title              string     `json:"title"`
	Parent             string     `json:"parent"`
	Category           string     `json:"category"`
	Status             string     `json:"status"`
	Description        string     `json:"description"`
	Image              string     `json:"image"`
	Flyer              string     `json:"flyer"`
	Highlight          *Highlight `json:"highlight"`
	Timing             string     `json:"timing"`
	Start              string     `json:"start"`
	End                string     `json:"end"`
	Location           string     `json:"location"`
	Spots              int        `json:"spots"`
	CoLeaderNeeded     bool       `json:"coLeaderNeeded"`
	VolunteersHidden   bool       `json:"volunteersHidden"`
	VolunteersComplete bool       `json:"volunteersComplete"`
	DirectSignUp       bool       `json:"directSignUp"`
	SignUp             string     `json:"signUp"`
	PrettyID           string     `json:"prettyId"`
	AllowAdding        string     `json:"allowAdding"`
	TakeOver           bool       `json:"takeOver"`
}

type prettyConflict struct {
	Error   string `json:"error"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Year    string `json:"year"`
	Prior   bool   `json:"prior"`
	Renamed string `json:"renamed,omitempty"`
}

func renamedPretty(model *Model, pretty, year string) string {
	base := pretty
	if m := yearForm.FindStringSubmatch(year); m != nil {
		base = pretty + "-" + m[1]
	}
	for i, candidate := 2, base; ; i++ {
		if model.ByPretty(candidate) == nil && len(candidate) <= maxPrettyLength {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
}

func spotsCell(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func (a app) saveActivity(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body activityBody
	if !decode(w, r, &body) {
		return
	}
	model := a.cache.Model()
	title := strings.TrimSpace(body.Title)
	year := strings.TrimSpace(body.Year)
	adding := body.ID == ""
	var current *Activity
	if !adding {
		act, ok := a.findActivity(w, body.ID)
		if !ok {
			return
		}
		current = act
		if !admin && !model.Runs(act, actor) {
			http.Error(w, "only a co-chair or admin can edit this", http.StatusForbidden)
			return
		}
	}
	status := body.Status
	switch {
	case adding && !admin:
		status = StatusOpen
	case adding && status == "":
		status = StatusOpen
	case !adding && !admin:
		// A co-chair opens or finishes a thing, or leaves its status be;
		// hiding is an admin's. A pending thing stays pending unless whoever
		// runs what it was suggested under opens or finishes it, which is
		// approving it.
		approver := current.Parent != "" && model.Runs(model.Activity(current.Parent), actor)
		switch {
		case status == current.Status:
		case current.Status == StatusPending && !approver:
			status = StatusPending
		case status != StatusOpen && status != StatusDone:
			http.Error(w, "a co-chair may only mark this open or done", http.StatusBadRequest)
			return
		}
		year = current.Year
	}
	parent := strings.TrimSpace(body.Parent)
	if parent != "" {
		p := model.Activity(parent)
		if p == nil {
			http.Error(w, fmt.Sprintf("no activity with id %q to sit under", parent), http.StatusBadRequest)
			return
		}
		if p.Year != year {
			http.Error(w, "a parent has to be in the same school year", http.StatusBadRequest)
			return
		}
		if current != nil {
			for node := p; node != nil; node = model.Activity(node.Parent) {
				if node.ID == current.ID {
					http.Error(w, "that would put this inside itself", http.StatusBadRequest)
					return
				}
			}
		}
	}
	// A co-chair moves a thing only under something else they run; making it
	// stand on its own, or putting it under someone else's, is an admin's.
	if current != nil && !admin && parent != current.Parent && (parent == "" || !model.Runs(model.Activity(parent), actor)) {
		http.Error(w, "only an admin can move this there", http.StatusForbidden)
		return
	}
	category := strings.TrimSpace(body.Category)
	if category == UncategorizedID {
		category = ""
	}
	editor := admin
	if parent != "" {
		editor = editor || model.Runs(model.Activity(parent), actor)
	}
	policy := AddingNo
	if parent != "" {
		policy = model.Activity(parent).Adding
	}
	if category == "" && parent == "" && adding && !editor {
		http.Error(w, "pick a category", http.StatusBadRequest)
		return
	}
	if category != "" {
		c := model.Category(category)
		if c == nil {
			http.Error(w, fmt.Sprintf("no category with id %q", category), http.StatusBadRequest)
			return
		}
		if parent == "" && c.EventID != "" {
			http.Error(w, fmt.Sprintf("%q belongs to one event, not the page", c.Title), http.StatusBadRequest)
			return
		}
		if parent != "" && c.EventID != model.Root(model.Activity(parent)).ID {
			http.Error(w, fmt.Sprintf("%q is not one of this event's categories", c.Title), http.StatusBadRequest)
			return
		}
		policy = c.Adding
	}
	if adding && !editor {
		switch policy {
		case AddingYes:
			status = StatusOpen
		case AddingApproval:
			status = StatusPending
		default:
			http.Error(w, "new things cannot be added here", http.StatusBadRequest)
			return
		}
	}
	id := body.ID
	if adding {
		id = newID()
	}
	signUp := ""
	if adding {
		switch strings.TrimSpace(body.SignUp) {
		case PositionVolunteer, PositionOpen:
			signUp = strings.TrimSpace(body.SignUp)
		case "", "none":
		default:
			http.Error(w, "sign up as a volunteer, as one open to co-chairing, or not at all", http.StatusBadRequest)
			return
		}
	}
	pretty := NormalizePretty(body.PrettyID)
	if err := CheckPretty(pretty); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	allowAdding, err := checkAdding(body.AllowAdding)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !editor {
		allowAdding = ""
		if current != nil {
			allowAdding = current.AllowAdding
		}
	}
	var displaced *Activity
	renamed := ""
	if pretty != "" && parent != "" {
		for _, sibling := range model.Activity(parent).Children {
			if sibling.ID != id && sibling.PrettyID == pretty {
				conflict := prettyConflict{ID: sibling.ID, Title: sibling.Title, Year: sibling.Year,
					Error: fmt.Sprintf("%q is already the address of %q under the same parent", pretty, sibling.Title)}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(conflict)
				return
			}
		}
	}
	if other := model.ByPretty(pretty); pretty != "" && parent == "" && other != nil && other.ID != id {
		conflict := prettyConflict{ID: other.ID, Title: other.Title, Year: other.Year, Prior: other.Year < year}
		if conflict.Prior {
			conflict.Renamed = renamedPretty(model, pretty, other.Year)
		}
		if !conflict.Prior || !body.TakeOver {
			if conflict.Prior {
				conflict.Error = fmt.Sprintf("%q is the address of %q from %s", pretty, other.Title, other.Year)
			} else {
				conflict.Error = fmt.Sprintf("%q is already the address of %q (%s)", pretty, other.Title, other.Year)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(conflict)
			return
		}
		displaced, renamed = other, conflict.Renamed
	}
	cells := store.Row{
		"Event ID": id, "Year": year, "Title": title, "Parent": parent,
		"Category": category, "Status": status,
		"Description": strings.TrimSpace(body.Description), "Image": strings.TrimSpace(body.Image), "Flyer Image": strings.TrimSpace(body.Flyer),
		"Timing": strings.TrimSpace(body.Timing), "Start": strings.TrimSpace(body.Start), "End": strings.TrimSpace(body.End),
		"Location": strings.TrimSpace(body.Location), "Spots": spotsCell(body.Spots),
		"Co-Leader Needed": YesNo(body.CoLeaderNeeded), "Volunteers Hidden": YesNo(body.VolunteersHidden),
		CompleteColumn:   YesNo(body.VolunteersComplete),
		"Direct Sign-Up": YesNo(body.DirectSignUp), "Pretty ID": pretty, "Allow Adding": allowAdding,
	}
	for k, v := range highlightCells(body.Highlight) {
		cells[k] = v
	}
	ops := []store.Op{}
	if displaced != nil {
		ops = append(ops, store.Update(activitiesTab, store.Row{"Event ID": displaced.ID}, store.Row{"Pretty ID": renamed}))
	}
	if current != nil {
		was := model.PathOf(current)
		now := "/activities/" + id
		if parent != "" {
			seg := id
			if pretty != "" {
				seg = pretty
			}
			now = model.PathOf(model.Activity(parent)) + "/" + seg
		} else if pretty != "" {
			now = "/v/" + pretty
		}
		if was != now {
			ops = append(ops, store.Insert(redirectsTab, store.Row{"Type": RedirectActivity, "Old": was, "New": now, "Date": today()}))
		}
	}
	action := "edit"
	if adding {
		action = "add"
		cells["Added By"] = actor
		cells["Added"] = today()
		ops = append(ops, store.Insert(activitiesTab, cells))
		if signUp != "" {
			ops = append(ops, store.Insert(volunteersTab, store.Row{"Event ID": id, "Email": actor, "Position": signUp, "Added By": actor, "Added": today()}))
		}
	} else {
		if current.Parent != parent {
			cells[store.OrderColumn] = ""
		}
		ops = append(ops, store.Update(activitiesTab, store.Row{"Event ID": id}, cells))
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved activity", "actor", actor, "action", action, "activity", title, "id", id, "year", year, "status", status)
	if adding {
		a.mailNewActivity(r, a.cache.Model().Activity(id), actor)
	}
	w.WriteHeader(http.StatusNoContent)
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
	act, ok := a.findActivity(w, body.ID)
	if !ok {
		return
	}
	if len(act.Volunteers) > 0 {
		http.Error(w, "remove its volunteers first", http.StatusBadRequest)
		return
	}
	if len(act.Children) > 0 {
		http.Error(w, "delete the things under it first", http.StatusBadRequest)
		return
	}
	if !a.commit(w, r, actor, store.Delete(activitiesTab, store.Row{"Event ID": act.ID})) {
		return
	}
	slog.InfoContext(r.Context(), "events: deleted activity", "actor", actor, "activity", act.Title, "year", act.Year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveLink(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		ID          string `json:"id"`
		Original    string `json:"original"`
		Title       string `json:"title"`
		URL         string `json:"url"`
		Description string `json:"description"`
		Image       string `json:"image"`
	}
	if !decode(w, r, &body) {
		return
	}
	act, ok := a.findActivity(w, body.ID)
	if !ok {
		return
	}
	if !admin && !a.cache.Model().Runs(act, actor) {
		http.Error(w, "only a co-chair or admin can add links", http.StatusForbidden)
		return
	}
	title := strings.TrimSpace(body.Title)
	cells := store.Row{
		"Event ID": act.ID, "Title": title,
		"URL": strings.TrimSpace(body.URL), "Image": strings.TrimSpace(body.Image),
		"Description": strings.TrimSpace(body.Description),
	}
	action := "add"
	op := store.Insert(linksTab, cells)
	if body.Original != "" {
		if !slices.ContainsFunc(act.Links, func(l Link) bool { return l.Title == body.Original }) {
			http.Error(w, fmt.Sprintf("%s has no link %q", act.Title, body.Original), http.StatusNotFound)
			return
		}
		action = "edit"
		op = store.Update(linksTab, store.Row{"Event ID": act.ID, "Title": body.Original}, cells)
	}
	if !a.commit(w, r, actor, op) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved link", "actor", actor, "action", action, "link", title, "activity", act.Title, "year", act.Year)
	w.WriteHeader(http.StatusNoContent)
}

func orderOps(tab, keyColumn string, ids, current []string) []store.Op {
	keys := store.Order(current)
	ops := []store.Op{}
	for i, id := range ids {
		if keys[i] != current[i] {
			ops = append(ops, store.Update(tab, store.Row{keyColumn: id}, store.Row{store.OrderColumn: keys[i]}))
		}
	}
	return ops
}

func (a app) orderChildren(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		Parent string   `json:"parent"`
		IDs    []string `json:"ids"`
	}
	if !decode(w, r, &body) {
		return
	}
	parent, ok := a.findActivity(w, body.Parent)
	if !ok {
		return
	}
	if !admin && !a.cache.Model().Runs(parent, actor) {
		http.Error(w, "only a co-chair or admin can reorder these", http.StatusForbidden)
		return
	}
	children := map[string]*Activity{}
	for _, c := range parent.Children {
		children[c.ID] = c
	}
	if len(body.IDs) != len(children) {
		http.Error(w, fmt.Sprintf("the order must name every thing under %s exactly once", parent.Title), http.StatusBadRequest)
		return
	}
	ids, current := []string{}, []string{}
	for _, id := range body.IDs {
		c := children[strings.TrimSpace(id)]
		if c == nil {
			http.Error(w, fmt.Sprintf("%q is not one of the things under %s", id, parent.Title), http.StatusBadRequest)
			return
		}
		delete(children, c.ID)
		ids, current = append(ids, c.ID), append(current, c.Order)
	}
	ops := orderOps(activitiesTab, "Event ID", ids, current)
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: reordered", "actor", actor, "parent", parent.Title, "year", parent.Year, "changed", len(ops))
	w.WriteHeader(http.StatusNoContent)
}

func highlightCells(h *Highlight) store.Row {
	cells := store.Row{"Highlight Headline": "", "Highlight Body": "", "Highlight Icon": ""}
	if h != nil && (strings.TrimSpace(h.Headline) != "" || strings.TrimSpace(h.Body) != "") {
		cells["Highlight Headline"] = strings.TrimSpace(h.Headline)
		cells["Highlight Body"] = strings.TrimSpace(h.Body)
		cells["Highlight Icon"] = strings.TrimSpace(h.Icon)
	}
	return cells
}

func (a app) deleteLink(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if !decode(w, r, &body) {
		return
	}
	act, ok := a.findActivity(w, body.ID)
	if !ok {
		return
	}
	if !admin && !a.cache.Model().Runs(act, actor) {
		http.Error(w, "only a co-chair or admin can remove links", http.StatusForbidden)
		return
	}
	if !a.commit(w, r, actor, store.Delete(linksTab, store.Row{"Event ID": act.ID, "Title": body.Title})) {
		return
	}
	slog.InfoContext(r.Context(), "events: deleted link", "actor", actor, "link", body.Title, "activity", act.Title, "year", act.Year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) categoryEditor(w http.ResponseWriter, r *http.Request, eventID string) (string, bool) {
	actor, admin := a.who(r)
	if admin {
		return actor, true
	}
	if eventID == "" {
		http.Error(w, "only an admin can change the page's categories", http.StatusForbidden)
		return "", false
	}
	event := a.cache.Model().Activity(eventID)
	if event == nil || !a.cache.Model().Runs(event, actor) {
		http.Error(w, "only a co-chair or admin can change this event's categories", http.StatusForbidden)
		return "", false
	}
	return actor, true
}

func (a app) saveCategory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID          string `json:"id"`
		EventID     string `json:"eventId"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Image       string `json:"image"`
		AllowAdding string `json:"allowAdding"`
		ShowOnMain  *bool  `json:"showOnMain"`
	}
	if !decode(w, r, &body) {
		return
	}
	model := a.cache.Model()
	title := strings.TrimSpace(body.Title)
	eventID := strings.TrimSpace(body.EventID)
	adding := body.ID == ""
	id := strings.TrimSpace(body.ID)
	if !adding {
		current := model.Category(id)
		if current == nil {
			http.Error(w, fmt.Sprintf("no category with id %q", id), http.StatusNotFound)
			return
		}
		if current.BuiltIn {
			http.Error(w, "Uncategorized is built in and cannot be changed", http.StatusBadRequest)
			return
		}
		eventID = current.EventID
	} else if eventID != "" {
		event := model.Activity(eventID)
		if event == nil || event.Parent != "" {
			http.Error(w, "an event's category has to belong to a root event", http.StatusBadRequest)
			return
		}
	}
	actor, ok := a.categoryEditor(w, r, eventID)
	if !ok {
		return
	}
	allowAdding, err := checkAdding(body.AllowAdding)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if adding {
		id = newID()
	}
	cells := store.Row{
		"Category ID":       id,
		"Event ID":          eventID,
		"Title":             title,
		"Description":       strings.TrimSpace(body.Description),
		"Image":             strings.TrimSpace(body.Image),
		"Allow Adding":      allowAdding,
		"Show On Main Page": "",
	}
	if eventID == "" {
		cells["Show On Main Page"] = YesNo(body.ShowOnMain == nil || *body.ShowOnMain)
	}
	action := "add"
	op := store.Insert(categoriesTab, cells)
	if !adding {
		action = "edit"
		op = store.Update(categoriesTab, store.Row{"Category ID": id}, cells)
	}
	if !a.commit(w, r, actor, op) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved category", "actor", actor, "action", action, "category", title, "id", id, "event", eventID)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) reorderCategories(w http.ResponseWriter, r *http.Request) {
	var body struct {
		EventID string   `json:"eventId"`
		IDs     []string `json:"ids"`
	}
	if !decode(w, r, &body) {
		return
	}
	eventID := strings.TrimSpace(body.EventID)
	actor, ok := a.categoryEditor(w, r, eventID)
	if !ok {
		return
	}
	model := a.cache.Model()
	list := model.Categories
	if eventID != "" {
		event := model.Activity(eventID)
		if event == nil {
			http.Error(w, fmt.Sprintf("no activity with id %q", eventID), http.StatusNotFound)
			return
		}
		list = event.Categories
	}
	inScope := map[string]Category{}
	for _, c := range list {
		if !c.BuiltIn {
			inScope[c.ID] = c
		}
	}
	if len(body.IDs) != len(inScope) {
		http.Error(w, "the order must name every category exactly once", http.StatusBadRequest)
		return
	}
	ids, current := []string{}, []string{}
	for _, id := range body.IDs {
		c, ok := inScope[id]
		if !ok {
			http.Error(w, "the order must name every category exactly once", http.StatusBadRequest)
			return
		}
		delete(inScope, id)
		ids, current = append(ids, c.ID), append(current, c.Order)
	}
	if !a.commit(w, r, actor, orderOps(categoriesTab, "Category ID", ids, current)...) {
		return
	}
	slog.InfoContext(r.Context(), "events: reordered categories", "actor", actor, "event", eventID, "count", len(body.IDs))
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteCategory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &body) {
		return
	}
	cat := a.cache.Model().Category(strings.TrimSpace(body.ID))
	if cat == nil {
		http.Error(w, "no such category", http.StatusNotFound)
		return
	}
	if cat.BuiltIn {
		http.Error(w, "Uncategorized is built in and cannot be deleted", http.StatusBadRequest)
		return
	}
	actor, ok := a.categoryEditor(w, r, cat.EventID)
	if !ok {
		return
	}
	if a.cache.Count(activitiesTab, store.Row{"Category": cat.ID}) > 0 {
		http.Error(w, "move or delete its activities first", http.StatusBadRequest)
		return
	}
	if !a.commit(w, r, actor, store.Delete(categoriesTab, store.Row{"Category ID": cat.ID})) {
		return
	}
	slog.InfoContext(r.Context(), "events: deleted category", "actor", actor, "category", cat.Title, "id", cat.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) copyActivity(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body activityRef
	if !decode(w, r, &body) {
		return
	}
	act, ok := a.findActivity(w, body.ID)
	if !ok {
		return
	}
	if act.Parent != "" {
		http.Error(w, "copy the whole activity it sits under instead", http.StatusBadRequest)
		return
	}
	year := ShiftYear(act.Year, 1)
	for _, other := range a.cache.Model().Activities {
		if other.Year == year && other.Title == act.Title {
			http.Error(w, fmt.Sprintf("%q already exists in %s", act.Title, year), http.StatusBadRequest)
			return
		}
	}
	fresh := map[string]string{act.ID: newID()}
	ops := []store.Op{}
	for _, c := range act.Categories {
		fresh[c.ID] = newID()
		ops = append(ops, store.Insert(categoriesTab, store.Row{
			"Category ID": fresh[c.ID], "Event ID": fresh[act.ID], "Title": c.Title, "Description": c.Description,
			"Image": c.Image, "Allow Adding": c.AllowAdding, store.OrderColumn: c.Order,
		}))
	}
	remap := func(id string) string {
		if to, ok := fresh[id]; ok {
			return to
		}
		if id == UncategorizedID {
			return ""
		}
		return id
	}
	rowFor := func(c *Activity, parent string) store.Row {
		row := store.Row{
			"Event ID": fresh[c.ID], "Year": year, "Title": c.Title, "Parent": parent, "Category": remap(c.Category),
			"Status": c.Status, "Description": c.Description, "Image": c.Image, "Flyer Image": c.Flyer, "Timing": c.Timing,
			"Location": c.Location, "Spots": spotsCell(c.Spots),
			"Co-Leader Needed": YesNo(c.CoLeaderNeeded), "Volunteers Hidden": YesNo(c.VolunteersHidden),
			"Direct Sign-Up": YesNo(c.DirectSignUp), "Allow Adding": c.AllowAdding, "Added By": actor, "Added": today(),
			store.OrderColumn: c.Order, CompleteColumn: YesNo(false),
		}
		for k, v := range highlightCells(c.Highlight) {
			row[k] = v
		}
		return row
	}
	root := rowFor(act, "")
	root["Status"] = StatusOpen
	ops = append(ops, store.Insert(activitiesTab, root))
	copied := []*Activity{act}
	for _, c := range act.Descendants() {
		if c.Status == StatusPending {
			continue
		}
		if _, ok := fresh[c.Parent]; !ok {
			continue
		}
		fresh[c.ID] = newID()
		ops = append(ops, store.Insert(activitiesTab, rowFor(c, fresh[c.Parent])))
		copied = append(copied, c)
	}
	for _, node := range copied {
		for _, l := range node.Links {
			ops = append(ops, store.Insert(linksTab, store.Row{"Event ID": fresh[node.ID], "Title": l.Title, "URL": l.URL, "Image": l.Image, "Description": l.Description}))
		}
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: copied activity", "actor", actor, "activity", act.Title, "from", act.Year, "to", year, "id", fresh[act.ID])
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
	ops := []store.Op{}
	for _, key := range settingKeys {
		ops = append(ops, store.Set(settingsTab, store.Row{"Key": key}, store.Row{"Value": values[key]}))
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "events: changed the settings", "actor", actor)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveNotify(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Kinds []string `json:"kinds"`
	}
	if !decode(w, r, &body) {
		return
	}
	kinds := []string{}
	for _, k := range NotifyKinds {
		if slices.Contains(body.Kinds, k) {
			kinds = append(kinds, k)
		}
	}
	value := strings.Join(kinds, ",")
	if !a.commit(w, r, actor, store.Set(settingsTab, store.Row{"Key": notifyPrefix + actor}, store.Row{"Value": value})) {
		return
	}
	slog.InfoContext(r.Context(), "events: set notifications", "actor", actor, "kinds", value)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) uploadImage(w http.ResponseWriter, r *http.Request) {
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
	if err := a.media.Put(imageFolder, name, mimeType, content); err != nil {
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
	prefs := a.cache.Model().notifyPrefs(email)
	notify := []string{}
	for _, k := range NotifyKinds {
		if prefs[k] {
			notify = append(notify, k)
		}
	}
	view := struct {
		Email  string   `json:"email"`
		Admins []string `json:"admins"`
		Notify []string `json:"notify"`
		Mail   bool     `json:"mail"`
	}{Email: email, Admins: a.cache.Admins(a.superAdmins()), Notify: notify, Mail: a.mailer != nil}
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
	current := a.cache.Model().admins
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
	slog.InfoContext(r.Context(), "events: set the admin list", "actor", actor, "admins", admins)
	w.WriteHeader(http.StatusNoContent)
}

var reservedPaths = map[string]bool{"": true, "my": true, "calendar": true, "approvals": true, "admin": true, "years": true, "api": true, "auth": true, "hooks": true, "open": true, "blob": true}

func (a app) saveRedirect(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Original string `json:"original"`
		Old      string `json:"old"`
		New      string `json:"new"`
	}
	if !decode(w, r, &body) {
		return
	}
	old, to := redirectPath(body.Old), redirectTo(body.New)
	if old == "" {
		http.Error(w, "say which address to redirect", http.StatusBadRequest)
		return
	}
	if to == "" {
		http.Error(w, "say where the address should go", http.StatusBadRequest)
		return
	}
	first, _, _ := strings.Cut(strings.TrimPrefix(old, "/"), "/")
	if reservedPaths[strings.ToLower(first)] {
		http.Error(w, fmt.Sprintf("%s is one of the portal's own addresses and cannot be redirected", old), http.StatusBadRequest)
		return
	}
	model := a.cache.Model()
	if act := model.walk(old); act != nil {
		http.Error(w, fmt.Sprintf("%s is the address of “%s” (%s); rename it from its page instead", old, act.Title, act.Year), http.StatusConflict)
		return
	}
	if strings.EqualFold(old, to) {
		http.Error(w, "an address cannot redirect to itself", http.StatusBadRequest)
		return
	}
	var replacing *Redirect
	kind := RedirectAdmin
	if original := redirectPath(body.Original); original != "" {
		replacing = model.redirect(original)
		if replacing == nil {
			http.Error(w, fmt.Sprintf("no redirect from %s", original), http.StatusNotFound)
			return
		}
		if replacing.Type != "" {
			kind = replacing.Type
		}
	} else if model.redirect(old) != nil {
		http.Error(w, fmt.Sprintf("%s is already redirected; edit that one", old), http.StatusConflict)
		return
	}
	if !isURL(to) && model.withRedirect(Redirect{Type: kind, Old: old, New: to}, replacing).Destination(old) == "" {
		http.Error(w, fmt.Sprintf("%s leads back to %s", to, old), http.StatusBadRequest)
		return
	}
	cells := store.Row{"Type": kind, "Old": old, "New": to, "Date": today()}
	action := "add"
	op := store.Insert(redirectsTab, cells)
	if replacing != nil {
		action = "edit"
		op = store.Update(redirectsTab, store.Row{"Old": replacing.cell}, cells)
	}
	if !a.commit(w, r, actor, op) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved redirect", "actor", actor, "action", action, "old", old, "new", to)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteRedirect(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Old string `json:"old"`
	}
	if !decode(w, r, &body) {
		return
	}
	redirect := a.cache.Model().redirect(redirectPath(body.Old))
	if redirect == nil {
		http.Error(w, fmt.Sprintf("no redirect from %s", body.Old), http.StatusNotFound)
		return
	}
	if !a.commit(w, r, actor, store.Delete(redirectsTab, store.Row{"Old": redirect.cell})) {
		return
	}
	slog.InfoContext(r.Context(), "events: deleted redirect", "actor", actor, "old", redirect.Old)
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
