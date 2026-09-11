package events

import (
	"context"
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
	"heliosian/internal/data"
	"heliosian/internal/logging"
	"heliosian/internal/serve"
)

const (
	imageFolder  = "activity-images"
	maxImageSize = 8 << 20
	shell        = "web/hca/index.html"
)

var pages = []string{
	"/{$}", "/my", "/calendar", "/approvals", "/admin", "/years/{year}", "/activities/{path...}", "/v/{path...}",
}

// local is the school's clock: the sheet's dates are wall-clock there, and a
// sign-up made late one evening is dated that evening.
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
	store       *blob.Store
	directory   Directory
	superAdmins func() []string
}

// Register wires the portal: one shell for every page, the model, and the
// writes. Every route already sits behind sign-in.
func Register(mux *http.ServeMux, cache *Cache, writer data.Writer, queue Enqueuer, store *blob.Store, directory Directory, superAdmins func() []string) {
	a := app{cache: cache, writer: writer, queue: queue, store: store, directory: directory, superAdmins: superAdmins}
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.ready(a.page))
	}
	mux.HandleFunc("GET /api/events/model", a.ready(a.model))
	// Public, past sign-in (auth.Public): the image a chat app shows for a link.
	mux.HandleFunc("GET /share/{id}", a.ready(a.shareCard))
	mux.HandleFunc("GET /api/events/people", a.ready(a.people))
	mux.HandleFunc("POST /api/events/volunteer", a.ready(a.saveVolunteer))
	mux.HandleFunc("DELETE /api/events/volunteer", a.ready(a.removeVolunteer))
	mux.HandleFunc("POST /api/events/activity", a.ready(a.saveActivity))
	mux.HandleFunc("DELETE /api/events/activity", a.ready(a.deleteActivity))
	mux.HandleFunc("POST /api/events/link", a.ready(a.saveLink))
	mux.HandleFunc("DELETE /api/events/link", a.ready(a.deleteLink))
	mux.HandleFunc("POST /api/events/category", a.ready(a.saveCategory))
	mux.HandleFunc("DELETE /api/events/category", a.ready(a.deleteCategory))
	mux.HandleFunc("POST /api/events/categories/order", a.ready(a.reorderCategories))
	mux.HandleFunc("POST /api/events/copy", a.ready(a.copyActivity))
	mux.HandleFunc("POST /api/events/settings", a.ready(a.saveSettings))
	mux.HandleFunc("POST /api/events/image", a.ready(a.uploadImage))
	mux.HandleFunc("GET /api/admin/state", a.ready(a.adminState))
	mux.HandleFunc("POST /api/admin/admins", a.ready(a.setAdmins))
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

// ready holds every route until the sheet has loaded once. Before then the
// portal answers 503 with the reason, so a sheet that needs fixing says what is
// wrong instead of taking the server down with the directory on it.
func (a app) ready(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cache.Model() == nil {
			reason := "the Events sheet has not loaded yet"
			if err := a.cache.Err(); err != nil {
				reason = err.Error()
			}
			http.Error(w, "HCA-Team cannot load its data: "+reason, http.StatusServiceUnavailable)
			return
		}
		next(w, r)
	}
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

func (a app) logChange(actor, action, kind string, cells map[string]string) error {
	return a.writer.AppendCells(appName, changeLogTab, map[string]string{
		"Timestamp": time.Now().Format(time.RFC3339), "Actor": actor, "Action": action, "Kind": kind,
		"Year": cells["Year"], "Activity": cells["Activity"], "Title": cells["Title"], "Email": cells["Email"], "Details": cells["Details"],
	})
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

// newID mints a key for a row the app creates. Eight characters from a 32-symbol
// alphabet is 40 bits - collisions are not a practical concern at this scale,
// and the load refuses a duplicate anyway rather than letting one through.
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

// people is the directory for the sign-up picker. Anyone signed in may see it -
// it is the same list the school directory shows every member.
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
	}
	if !decode(w, r, &body) {
		return
	}
	act, ok := a.findActivity(w, body.ID)
	if !ok {
		return
	}
	editor := admin || a.cache.Model().Runs(act, actor)
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if email == "" {
		email = actor
	}
	if !emailForm.MatchString(email) {
		http.Error(w, "that is not an email address", http.StatusBadRequest)
		return
	}
	status, spots := act.Status, act.Spots
	// A root that does not take sign-ups itself sends people to the things under
	// it; a child always takes them.
	if act.Parent == "" && !act.DirectSignUp && !editor {
		http.Error(w, "sign up for one of the things under it instead", http.StatusBadRequest)
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
	match := map[string]string{"Event ID": act.ID, "Email": email}
	tables := a.cache.Tables()
	existing := tables.count(volunteersTab, match) > 0
	if existing && !editor && email != actor {
		http.Error(w, "only a co-chair or admin can change someone else's sign-up", http.StatusForbidden)
		return
	}
	if !existing && !editor && spots > 0 {
		taken := tables.count(volunteersTab, map[string]string{"Event ID": act.ID})
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
			"Year": act.Year, "Activity": act.Title, "Email": email, "Details": body.Position,
		})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved volunteer", "actor", actor, "action", action, "email", email, "activity", act.Title, "year", act.Year)
	w.WriteHeader(http.StatusNoContent)
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
	if email != actor && !admin && !a.cache.Model().Runs(act, actor) {
		http.Error(w, "only a co-chair or admin can remove someone else", http.StatusForbidden)
		return
	}
	match := map[string]string{"Event ID": act.ID, "Email": email}
	if !a.commit(r.Context(), w, a.cache.Tables().without(volunteersTab, match), func() error {
		if err := a.writer.Delete(appName, volunteersTab, match); err != nil {
			return err
		}
		return a.logChange(actor, "remove", "volunteer", map[string]string{
			"Year": act.Year, "Activity": act.Title, "Email": email,
		})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: removed volunteer", "actor", actor, "email", email, "activity", act.Title, "year", act.Year)
	w.WriteHeader(http.StatusNoContent)
}

type activityBody struct {
	ID               string `json:"id"`
	Year             string `json:"year"`
	Title            string `json:"title"`
	Parent           string `json:"parent"`
	Category         string `json:"category"`
	Status           string `json:"status"`
	Description      string `json:"description"`
	Image            string `json:"image"`
	Timing           string `json:"timing"`
	Start            string `json:"start"`
	End              string `json:"end"`
	Location         string `json:"location"`
	Spots            int    `json:"spots"`
	CoLeaderNeeded   bool   `json:"coLeaderNeeded"`
	VolunteersHidden bool   `json:"volunteersHidden"`
	DirectSignUp     bool   `json:"directSignUp"`
	CoChair          bool   `json:"coChair"`
	PrettyID         string `json:"prettyId"`
	AllowAdding      string `json:"allowAdding"`
	// TakeOver says the sender has agreed to rename a prior year's activity
	// that holds the same Pretty ID - see prettyConflict.
	TakeOver bool `json:"takeOver"`
}

// prettyConflict is the answer when a Pretty ID is already someone else's: who
// has it, and whether they are in a prior year - in which case the sender may
// ask again with takeOver, and the old one is renamed to Renamed.
type prettyConflict struct {
	Error   string `json:"error"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Year    string `json:"year"`
	Prior   bool   `json:"prior"`
	Renamed string `json:"renamed,omitempty"`
}

// renamedPretty is the address a prior year's activity moves to when a newer
// one takes its Pretty ID: the same word with the year it belonged to, or the
// first free numbered variant of that.
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
		// Refined below by the adding policy; a co-chair's addition is live.
		status = StatusOpen
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
	// A parent is an id in the same year, and not the row itself or anything
	// under it - the loader would refuse the loop, so refuse it here first.
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
	// Uncategorized is the model's name for a root with no category; the sheet
	// stores that as a blank.
	category := strings.TrimSpace(body.Category)
	if category == UncategorizedID {
		category = ""
	}
	editor := admin
	if parent != "" {
		editor = editor || model.Runs(model.Activity(parent), actor)
	}
	// What someone who does not run the thing may add is the policy of the
	// category they add into, or of the parent when there is no category; a
	// new event with no category has nowhere to take a policy from.
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
	// Allow Adding gates additions from people who do not run the thing, and
	// says whether what they add waits for approval; whoever runs it adds
	// freely and their additions are live at once.
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
	// A root's Pretty ID is one address across every year. Another root holding
	// it in this year or a later one is simply a clash; one in a prior year can
	// be renamed out of the way, once the sender has agreed to that. A child's
	// is one address among its siblings, under the parent's path.
	pretty := NormalizePretty(body.PrettyID)
	if err := CheckPretty(pretty); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Only whoever runs it sets what others may add under it; a proposal
	// leaves the cell blank to inherit.
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
	cells := map[string]string{
		"Event ID": id, "Year": year, "Title": title, "Parent": parent,
		"Category": category, "Status": status,
		"Description": strings.TrimSpace(body.Description), "Image": strings.TrimSpace(body.Image),
		"Timing": strings.TrimSpace(body.Timing), "Start": strings.TrimSpace(body.Start), "End": strings.TrimSpace(body.End),
		"Location": strings.TrimSpace(body.Location), "Spots": spotsCell(body.Spots),
		"Co-Leader Needed": YesNo(body.CoLeaderNeeded), "Volunteers Hidden": YesNo(body.VolunteersHidden),
		"Direct Sign-Up": YesNo(body.DirectSignUp), "Pretty ID": pretty, "Allow Adding": allowAdding,
	}
	tables := a.cache.Tables()
	if displaced != nil {
		tables = tables.with(activitiesTab, map[string]string{"Event ID": displaced.ID}, map[string]string{"Pretty ID": renamed})
	}
	// An address that changes - a new or removed friendly name, or a move under
	// another parent - leaves a redirect from the old path to the new, so a
	// link someone kept still lands here. A root's redirect carries everything
	// under it along, so its children need none of their own.
	var redirect map[string]string
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
			redirect = map[string]string{"Type": RedirectActivity, "Old": was, "New": now, "Date": today()}
			tables = tables.with(redirectsTab, nil, redirect)
		}
	}
	action := "edit"
	match := map[string]string{"Event ID": id}
	// Moving a root to another year takes its whole tree along, since a parent
	// and child in different years is something the loader refuses.
	var moved []*Activity
	if adding {
		action = "add"
		cells["Added By"] = actor
		cells["Added"] = today()
		tables = tables.with(activitiesTab, nil, cells)
		if body.CoChair {
			tables = tables.with(volunteersTab, nil, map[string]string{
				"Event ID": id, "Email": actor, "Position": PositionOpen, "Added By": actor, "Added": today(),
			})
		}
	} else {
		tables = tables.with(activitiesTab, match, cells)
		if current.Year != year {
			moved = current.Descendants()
			for _, d := range moved {
				tables = tables.with(activitiesTab, map[string]string{"Event ID": d.ID}, map[string]string{"Year": year})
			}
		}
	}
	if !a.commit(r.Context(), w, tables, func() error {
		if adding {
			if err := a.writer.AppendCells(appName, activitiesTab, cells); err != nil {
				return err
			}
			if body.CoChair {
				if err := a.writer.AppendCells(appName, volunteersTab, map[string]string{
					"Event ID": id, "Email": actor, "Position": PositionOpen, "Added By": actor, "Added": today(),
				}); err != nil {
					return err
				}
			}
		} else {
			if err := a.writer.Set(appName, activitiesTab, match, cells); err != nil {
				return err
			}
			for _, d := range moved {
				if err := a.writer.Set(appName, activitiesTab, map[string]string{"Event ID": d.ID}, map[string]string{"Year": year}); err != nil {
					return err
				}
			}
		}
		if redirect != nil {
			if err := a.writer.AppendCells(appName, redirectsTab, redirect); err != nil {
				return err
			}
		}
		if displaced != nil {
			if err := a.writer.Set(appName, activitiesTab, map[string]string{"Event ID": displaced.ID}, map[string]string{"Pretty ID": renamed}); err != nil {
				return err
			}
			if err := a.logChange(actor, "rename", "activity", map[string]string{"Year": displaced.Year, "Activity": displaced.Title, "Details": "pretty id " + pretty + " -> " + renamed + " " + displaced.ID}); err != nil {
				return err
			}
		}
		return a.logChange(actor, action, "activity", map[string]string{"Year": year, "Activity": title, "Details": status + " " + id})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved activity", "actor", actor, "action", action, "activity", title, "id", id, "year", year, "status", status)
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
	match := map[string]string{"Event ID": act.ID}
	tables := a.cache.Tables()
	if tables.count(volunteersTab, match) > 0 {
		http.Error(w, "remove its volunteers first", http.StatusBadRequest)
		return
	}
	// Deleting a parent would orphan whatever hangs off it, and the next load
	// would refuse the whole sheet over the dangling Parent.
	if len(act.Children) > 0 {
		http.Error(w, "delete the things under it first", http.StatusBadRequest)
		return
	}
	tables = tables.without(linksTab, match).without(activitiesTab, match)
	if !a.commit(r.Context(), w, tables, func() error {
		if err := a.writer.Delete(appName, linksTab, match); err != nil {
			return err
		}
		if err := a.writer.Delete(appName, activitiesTab, match); err != nil {
			return err
		}
		return a.logChange(actor, "delete", "activity", map[string]string{"Year": act.Year, "Activity": act.Title})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: deleted activity", "actor", actor, "activity", act.Title, "year", act.Year)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveLink(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		ID       string `json:"id"`
		Original string `json:"original"`
		Title    string `json:"title"`
		URL      string `json:"url"`
		Image    string `json:"image"`
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
	cells := map[string]string{
		"Event ID": act.ID, "Title": title,
		"URL": strings.TrimSpace(body.URL), "Image": strings.TrimSpace(body.Image),
	}
	tables := a.cache.Tables()
	action := "edit"
	var match map[string]string
	if body.Original == "" {
		action = "add"
		tables = tables.with(linksTab, nil, cells)
	} else {
		match = map[string]string{"Event ID": act.ID, "Title": body.Original}
		tables = tables.with(linksTab, match, cells)
	}
	if !a.commit(r.Context(), w, tables, func() error {
		if body.Original == "" {
			if err := a.writer.AppendCells(appName, linksTab, cells); err != nil {
				return err
			}
		} else if err := a.writer.Set(appName, linksTab, match, cells); err != nil {
			return err
		}
		return a.logChange(actor, action, "link", map[string]string{"Year": act.Year, "Activity": act.Title, "Title": title, "Details": cells["URL"]})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved link", "actor", actor, "action", action, "link", title, "activity", act.Title, "year", act.Year)
	w.WriteHeader(http.StatusNoContent)
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
	match := map[string]string{"Event ID": act.ID, "Title": body.Title}
	if !a.commit(r.Context(), w, a.cache.Tables().without(linksTab, match), func() error {
		if err := a.writer.Delete(appName, linksTab, match); err != nil {
			return err
		}
		return a.logChange(actor, "delete", "link", map[string]string{"Year": act.Year, "Activity": act.Title, "Title": body.Title})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: deleted link", "actor", actor, "link", body.Title, "activity", act.Title, "year", act.Year)
	w.WriteHeader(http.StatusNoContent)
}

// categoryEditor decides who may change a category: an admin for a page
// heading, and for an event's own category, anyone who runs that event.
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
		// A category stays where it was made: moving one between events, or
		// between an event and the page, would strand whatever names it.
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
	cells := map[string]string{
		"Category ID":  id,
		"Event ID":     eventID,
		"Title":        title,
		"Description":  strings.TrimSpace(body.Description),
		"Image":        strings.TrimSpace(body.Image),
		"Allow Adding": allowAdding,
		// Only a page heading can be kept off the page; an event's own
		// categories are always shown on the event, and left blank here.
		"Show On Main Page": "",
	}
	if eventID == "" {
		cells["Show On Main Page"] = YesNo(body.ShowOnMain == nil || *body.ShowOnMain)
	}
	tables := a.cache.Tables()
	action := "edit"
	match := map[string]string{"Category ID": id}
	if adding {
		action = "add"
		tables = tables.with(categoriesTab, nil, cells)
	} else {
		tables = tables.with(categoriesTab, match, cells)
	}
	if !a.commit(r.Context(), w, tables, func() error {
		if adding {
			if err := a.writer.AppendCells(appName, categoriesTab, cells); err != nil {
				return err
			}
		} else if err := a.writer.Set(appName, categoriesTab, match, cells); err != nil {
			return err
		}
		return a.logChange(actor, action, "category", map[string]string{"Title": title, "Details": id + " " + eventID})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: saved category", "actor", actor, "action", action, "category", title, "id", id, "event", eventID)
	w.WriteHeader(http.StatusNoContent)
}

// reorderCategories puts one scope's categories - the page's, or one event's -
// into the given order. The writer's Reorder wants every row of the tab exactly
// once, so the other scopes' rows are threaded through untouched, each in the
// slot it already had.
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
	tables := a.cache.Tables()
	inScope := map[string]map[string]string{}
	for _, row := range tables.Categories {
		if strings.TrimSpace(row["Event ID"]) == eventID {
			inScope[row["Category ID"]] = row
		}
	}
	if len(body.IDs) != len(inScope) {
		http.Error(w, "the order must name every category exactly once", http.StatusBadRequest)
		return
	}
	for _, id := range body.IDs {
		if _, ok := inScope[id]; !ok {
			http.Error(w, "the order must name every category exactly once", http.StatusBadRequest)
			return
		}
		delete(inScope, id)
	}
	ordered := make([]map[string]string, 0, len(tables.Categories))
	keys := make([]string, 0, len(tables.Categories))
	next := 0
	byID := map[string]map[string]string{}
	for _, row := range tables.Categories {
		byID[row["Category ID"]] = row
	}
	for _, row := range tables.Categories {
		if strings.TrimSpace(row["Event ID"]) == eventID {
			row = byID[body.IDs[next]]
			next++
		}
		ordered = append(ordered, row)
		keys = append(keys, row["Category ID"])
	}
	nextTables := *tables
	nextTables.Categories = ordered
	if !a.commit(r.Context(), w, &nextTables, func() error {
		if err := a.writer.Reorder(appName, categoriesTab, "Category ID", keys); err != nil {
			return err
		}
		return a.logChange(actor, "reorder", "category", map[string]string{"Details": eventID + ": " + strings.Join(body.IDs, ", ")})
	}) {
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
	tables := a.cache.Tables()
	if tables.count(activitiesTab, map[string]string{"Category": cat.ID}) > 0 {
		http.Error(w, "move or delete its activities first", http.StatusBadRequest)
		return
	}
	match := map[string]string{"Category ID": cat.ID}
	if !a.commit(r.Context(), w, tables.without(categoriesTab, match), func() error {
		if err := a.writer.Delete(appName, categoriesTab, match); err != nil {
			return err
		}
		return a.logChange(actor, "delete", "category", map[string]string{"Title": cat.Title, "Details": cat.ID})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "events: deleted category", "actor", actor, "category", cat.Title, "id", cat.ID)
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
	act, ok := a.findActivity(w, body.ID)
	if !ok {
		return
	}
	if act.Parent != "" {
		http.Error(w, "copy the whole activity it sits under instead", http.StatusBadRequest)
		return
	}
	year := ShiftYear(act.Year, 1)
	// Titles are not keys, so "already copied" has to be judged the way a person
	// would: something of the same name already sits at the top of next year.
	for _, other := range a.cache.Model().Activities {
		if other.Year == year && other.Title == act.Title {
			http.Error(w, fmt.Sprintf("%q already exists in %s", act.Title, year), http.StatusBadRequest)
			return
		}
	}
	// Every copied row gets a fresh id, and the old-to-new map rewires each
	// child's Parent onto its copied parent. A pending one is somebody's
	// unapproved suggestion for the old year, so it does not travel.
	fresh := map[string]string{act.ID: newID()}
	// The event's own categories come along under new ids, so the copied
	// children can name them; page headings are shared and keep their id.
	catRows := []map[string]string{}
	for _, c := range act.Categories {
		fresh[c.ID] = newID()
		catRows = append(catRows, map[string]string{
			"Category ID": fresh[c.ID], "Event ID": fresh[act.ID], "Title": c.Title, "Description": c.Description,
			"Image": c.Image, "Allow Adding": c.AllowAdding, "Show On Main Page": "",
		})
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
	rowFor := func(c *Activity, parent string) map[string]string {
		return map[string]string{
			"Event ID": fresh[c.ID], "Year": year, "Title": c.Title, "Parent": parent, "Category": remap(c.Category),
			"Status": c.Status, "Description": c.Description, "Image": c.Image, "Timing": c.Timing,
			"Location": c.Location, "Spots": spotsCell(c.Spots),
			"Co-Leader Needed": YesNo(c.CoLeaderNeeded), "Volunteers Hidden": YesNo(c.VolunteersHidden),
			// The address stays with the original: two years cannot share one.
			"Direct Sign-Up": YesNo(c.DirectSignUp), "Pretty ID": "", "Allow Adding": c.AllowAdding, "Added By": actor, "Added": today(),
		}
	}
	rows := []map[string]string{rowFor(act, "")}
	rows[0]["Status"] = StatusOpen
	copied := []*Activity{act}
	for _, c := range act.Descendants() {
		if c.Status == StatusPending {
			continue
		}
		if _, ok := fresh[c.Parent]; !ok {
			continue // under a pending one that did not travel
		}
		fresh[c.ID] = newID()
		rows = append(rows, rowFor(c, fresh[c.Parent]))
		copied = append(copied, c)
	}
	links := []map[string]string{}
	for _, node := range copied {
		for _, l := range node.Links {
			links = append(links, map[string]string{"Event ID": fresh[node.ID], "Title": l.Title, "URL": l.URL, "Image": l.Image})
		}
	}
	tables := a.cache.Tables()
	for _, row := range catRows {
		tables = tables.with(categoriesTab, nil, row)
	}
	for _, row := range rows {
		tables = tables.with(activitiesTab, nil, row)
	}
	for _, row := range links {
		tables = tables.with(linksTab, nil, row)
	}
	if !a.commit(r.Context(), w, tables, func() error {
		for _, row := range catRows {
			if err := a.writer.AppendCells(appName, categoriesTab, row); err != nil {
				return err
			}
		}
		for _, row := range rows {
			if err := a.writer.AppendCells(appName, activitiesTab, row); err != nil {
				return err
			}
		}
		for _, row := range links {
			if err := a.writer.AppendCells(appName, linksTab, row); err != nil {
				return err
			}
		}
		return a.logChange(actor, "copy", "activity", map[string]string{"Year": year, "Activity": act.Title, "Details": "from " + act.Year + " as " + fresh[act.ID]})
	}) {
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
				if err := a.writer.AppendCells(appName, adminsTab, map[string]string{"Email": e}); err != nil {
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
