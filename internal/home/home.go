// Package home serves the community's link portal, Heliosian.
package home

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/imagesearch"
	"heliosian/internal/serve"
)

const (
	imageFolder  = "link-images"
	maxImageSize = 8 << 20
)

type app struct {
	cache       *Cache
	writer      data.Writer
	queue       Enqueuer
	store       *blob.Store
	superAdmins func() []string
	heroPhoto   func(string) string
	people      func() []Person
	alerts      func(string) (int, bool)
	upcoming    func(email, token string) Upcoming
	makeDefault func(ctx context.Context, email, token string) error
	month       func(email, month string) Month
	search      imagesearch.Search
	// answer records a person's word on a calendar event - yes, no, hidden
	// - with the calendar, whose lists follow it.
	answer func(ctx context.Context, email, id, answer string) error
}

// Event is a Helios Calendar event as the front page's Upcoming Events lists
// it: the calendar reckons which are ahead for this viewer, and the page
// links across to it. Image is its picture as a path on ImageApp's host,
// and for an event another app runs, Link is its page on LinkApp with Call
// the way in as the calendar words it, Mine and Availability behind that.
type Event struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Path         string `json:"path"`
	Start        string `json:"start"`
	When         string `json:"when"`
	StartAt      string `json:"startAt"`
	EndAt        string `json:"endAt,omitempty"`
	Location     string `json:"location,omitempty"`
	Description  string `json:"description,omitempty"`
	Image        string `json:"image,omitempty"`
	ImageApp     string `json:"imageApp,omitempty"`
	Link         string `json:"link,omitempty"`
	LinkApp      string `json:"linkApp,omitempty"`
	Call         string `json:"call,omitempty"`
	Mine         string `json:"mine,omitempty"`
	Availability string `json:"availability,omitempty"`
	// Answer is the viewer's word on it: yes, no, or nothing yet.
	Answer string `json:"answer,omitempty"`
	// People is everyone in the household with a part in it: a ticket, a
	// waitlist place, a role.
	People []Standing `json:"people,omitempty"`
}

type Standing struct {
	Name string `json:"name"`
	Note string `json:"note,omitempty"`
}

// Upcoming is the front page's Upcoming Events for a person: the events,
// the saved calendar they are read under by token, and every saved
// calendar of theirs - the first their default on Helios Calendar - for
// the picker beside the heading.
type Upcoming struct {
	Events    []Event         `json:"events"`
	Calendar  string          `json:"calendar,omitempty"`
	Calendars []SavedCalendar `json:"calendars,omitempty"`
}

// SavedCalendar is one of a person's saved calendars: its token, name and
// mark.
type SavedCalendar struct {
	Token string `json:"token"`
	Name  string `json:"name"`
	Emoji string `json:"emoji,omitempty"`
}

// Month is a month as the rail's calendar shows it, from Helios Calendar:
// today, the school days in it with what kind of day each is for the
// viewer's classrooms, and the viewer's events that touch it.
type Month struct {
	Month  string         `json:"month"`
	Today  string         `json:"today"`
	Days   map[string]Day `json:"days"`
	Events []Event        `json:"events"`
}

// Day is one school day: the day types in force other than Regular.
type Day struct {
	Kinds []Kind `json:"kinds"`
}

// Kind is one day type in force: its name, and the words for it - "Early
// Dismissal · Hummingbirds" when it is only some classrooms' day.
type Kind struct {
	Name  string `json:"name"`
	Words string `json:"words"`
}

// alerts is what the toolbar's badges say, reckoned by the directory: things
// to update for the new year, and a privacy mismatch.
type alerts struct {
	Stale   int  `json:"stale"`
	Privacy bool `json:"privacy"`
}

// Register wires the portal: the two pages, the model, and the admin writes.
// Every route already sits behind sign-in; the writes additionally require an
// admin. superAdmins reads the platform list out of the directory's settings.
// heroPhoto resolves the signed-in person's own directory photo; it comes from
// the directory cache, which this app does not otherwise depend on.
// search finds pictures for links on the web, as HCA-Team's editors do.
// alerts is the directory's reckoning of the toolbar badges for a person;
// upcoming is the calendar's list of what is ahead for a person and month
// its reckoning of one month of theirs; people is the directory as the
// admin page's pickers list it.
func Register(mux *http.ServeMux, cache *Cache, writer data.Writer, queue Enqueuer, store *blob.Store, superAdmins func() []string, heroPhoto func(string) string, people func() []Person, alerts func(string) (int, bool), upcoming func(email, token string) Upcoming, month func(email, month string) Month, search imagesearch.Search, answer func(ctx context.Context, email, id, answer string) error, makeDefault func(ctx context.Context, email, token string) error) {
	if search.UserAgent == "" {
		search.UserAgent = "Heliosian image search (+https://heliosian.com)"
	}
	a := app{cache: cache, writer: writer, queue: queue, store: store, superAdmins: superAdmins, heroPhoto: heroPhoto, people: people, alerts: alerts, upcoming: upcoming, month: month, search: search, answer: answer, makeDefault: makeDefault}
	mux.HandleFunc("GET /{$}", a.page)
	mux.HandleFunc("GET /admin", a.adminPage)
	mux.HandleFunc("GET /dl/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /api/apps/model", a.model)
	mux.HandleFunc("GET /api/apps/calendar", a.calendar)
	mux.HandleFunc("GET /api/apps/upcoming", a.upcomingUnder)
	mux.HandleFunc("POST /api/apps/calendar/default", a.setDefault)
	mux.HandleFunc("POST /api/apps/link", a.saveLink)
	mux.HandleFunc("POST /api/apps/rsvp", a.rsvp)
	mux.HandleFunc("DELETE /api/apps/link", a.deleteLink)
	mux.HandleFunc("POST /api/apps/category", a.saveCategory)
	mux.HandleFunc("DELETE /api/apps/category", a.deleteCategory)
	mux.HandleFunc("POST /api/apps/categories/order", a.reorderCategories)
	mux.HandleFunc("POST /api/apps/image", a.uploadImage)
	mux.HandleFunc("GET /api/apps/images/search", a.requireAdminFunc(a.search.ServeSearch))
	mux.HandleFunc("POST /api/apps/images/import", a.requireAdminFunc(a.importImage))
	mux.HandleFunc("GET /api/admin/state", a.adminState)
	mux.HandleFunc("POST /api/admin/admins", a.setAdmins)
	mux.HandleFunc("POST /api/admin/visibility", a.setVisibility)
	mux.HandleFunc("POST /api/admin/visibility/order", a.setAppOrder)
	RegisterSwitch(mux, cache)
	a.discoverApps()
}

// discoverApps writes the Visibility tab a row for every app of the registry
// it has none for - a new app, at its first start - as a list with nobody on
// it, so an app is out of sight until an admin lets people in, and the row
// is there in the sheet to edit. Nothing is written when every app has one.
func (a app) discoverApps() {
	missing := a.cache.MissingVisibility()
	if len(missing) == 0 {
		return
	}
	tables := a.cache.Tables()
	for _, app := range missing {
		tables = tables.withVisibility(app.Key, Visibility{Mode: VisibleToList, Tagline: app.Tagline, Name: app.Name})
	}
	model, err := BuildModel(tables, a.cache.images)
	if err != nil {
		slog.Error("apps: discover apps", "error", err)
		return
	}
	a.queue.Add(func() {
		a.cache.set(tables, model)
		for _, app := range missing {
			if err := a.writer.AppendCells(appName, visibilityTab, map[string]string{"App": app.Key, "Visibility": VisibleToList, "Tagline": app.Tagline, "Name": app.Name}); err != nil {
				slog.Error("apps: write a new app's visibility row", "app", app.Key, "error", err)
				return
			}
			slog.Info("apps: found a new app, listed for nobody yet", "app", app.Key)
		}
	})
}

// RegisterSwitch serves the app switch: every app in the order the switch
// lists them, Home first, and which of them the Visibility tab keeps off
// the signed-in person's. The shared toolbar asks whichever app's origin it
// is on, so every app's mux gets this route, and only the front page's the
// rest of the portal.
func RegisterSwitch(mux *http.ServeMux, cache *Cache) {
	mux.HandleFunc("GET /api/apps/switch", func(w http.ResponseWriter, r *http.Request) {
		view := struct {
			Apps   []App    `json:"apps"`
			Hidden []string `json:"hidden"`
		}{Apps: append([]App{homeApp()}, cache.AppList()...), Hidden: cache.HiddenApps(auth.Email(r))}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(view); err != nil {
			slog.ErrorContext(r.Context(), "encode app switch", "error", err)
		}
	})
}

// homeApp is Home with its mark's fingerprint on.
func homeApp() App {
	app := Home
	app.Mark = markVersion(app.Key)
	return app
}

// visibleApps is the community apps as the front page's apps section lists
// them for a person: every app less those the Visibility tab keeps from them.
func (a app) visibleApps(email string) []App {
	hidden := a.cache.HiddenApps(email)
	out := []App{}
	for _, app := range a.cache.AppList() {
		if !slices.Contains(hidden, app.Key) {
			out = append(out, app)
		}
	}
	return out
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, "web/home/index.html")
}

func (a app) adminPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	serve.File(w, r, "web/home/admin.html")
}

func (a app) requireAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	email := strings.ToLower(auth.Email(r))
	if !a.cache.IsAdmin(email) {
		http.Error(w, "admin access required", http.StatusForbidden)
		return "", false
	}
	return email, true
}

// requireAdminFunc guards a handler that has no admin-only body of its own.
func (a app) requireAdminFunc(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := a.requireAdmin(w, r); ok {
			next(w, r)
		}
	}
}

type user struct {
	Email    string `json:"email"`
	Initial  string `json:"initial"`
	PhotoURL string `json:"photoUrl,omitempty"`
	IsAdmin  bool   `json:"isAdmin"`
}

// model serves the portal. Hidden links reach only admins, who see them
// greyed out; everyone else gets the visible ones. A link into a community
// app narrowed to a list this person is not on is left out altogether,
// admin or not - the same rows the toolbar leaves off their app switch.
// calendar serves another month for the rail's calendar as it pages:
// /api/apps/calendar?month=2026-10.
func (a app) calendar(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(auth.Email(r))
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(a.month(email, r.URL.Query().Get("month"))); err != nil {
		slog.ErrorContext(r.Context(), "encode apps calendar", "error", err)
	}
}

// rsvp is the viewer's word on an event from a card: passed to the
// calendar, which keeps it and sends the invite for a yes.
func (a app) rsvp(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(auth.Email(r))
	var body struct {
		ID     string `json:"id"`
		Answer string `json:"answer"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	if a.answer == nil {
		http.Error(w, "the calendar is not set up", http.StatusBadRequest)
		return
	}
	if err := a.answer(r.Context(), email, body.ID, body.Answer); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// upcomingUnder answers /api/apps/upcoming?calendar=<token>: Upcoming
// Events read under one of the person's saved calendars, for the picker.
func (a app) upcomingUnder(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(auth.Email(r))
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(a.upcoming(email, r.URL.Query().Get("calendar"))); err != nil {
		slog.ErrorContext(r.Context(), "encode upcoming", "error", err)
	}
}

// setDefault makes one of the person's saved calendars their default on
// Helios Calendar - the one Upcoming Events and the rail's month read.
func (a app) setDefault(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(auth.Email(r))
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	if a.makeDefault == nil {
		http.Error(w, "the calendar is not set up", http.StatusBadRequest)
		return
	}
	if err := a.makeDefault(r.Context(), email, body.Token); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(auth.Email(r))
	admin := a.cache.IsAdmin(email)
	hidden := hiddenHosts(r.Host, a.cache.HiddenApps(email))
	full := a.cache.Model()
	categories := make([]Category, 0, len(full.Categories))
	for _, category := range full.Categories {
		shown := Category{Title: category.Title, Emoji: category.Emoji, Style: category.Style, Max: category.Max, Links: []Link{}, Virtual: category.Virtual}
		for _, link := range category.Links {
			if (link.Visible || admin) && !linksInto(hidden, link.URL) {
				shown.Links = append(shown.Links, link)
			}
		}
		categories = append(categories, shown)
	}
	view := struct {
		Categories []Category `json:"categories"`
		User       user       `json:"user"`
		// ImageSources lists where the link editor's picture search can
		// look, first first.
		ImageSources []string `json:"imageSources"`
		Alerts       alerts   `json:"alerts"`
		Upcoming     []Event  `json:"upcoming"`
		// UpcomingCalendar is the saved calendar Upcoming is read under, and
		// every saved calendar of theirs for the picker; absent with none.
		UpcomingCalendar *Upcoming `json:"upcomingCalendar,omitempty"`
		// Calendar fills the rail's month and day card: the month now is in.
		Calendar Month `json:"calendar"`
		// Apps fills the apps section: the community apps this person sees.
		Apps []App `json:"apps"`
	}{
		Categories:   categories,
		User:         user{Email: email, Initial: strings.ToUpper(email[:1]), PhotoURL: a.heroPhoto(email), IsAdmin: admin},
		ImageSources: a.search.Sources(),
		Calendar:     a.month(email, ""),
		Apps:         a.visibleApps(email),
	}
	ahead := a.upcoming(email, "")
	view.Upcoming = ahead.Events
	if ahead.Calendar != "" {
		view.UpcomingCalendar = &Upcoming{Calendar: ahead.Calendar, Calendars: ahead.Calendars}
	}
	view.Alerts.Stale, view.Alerts.Privacy = a.alerts(email)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode apps model", "error", err)
	}
}

// hiddenHosts is the set of hosts a link into one of the apps has on
// the requesting page's own tier, mirroring appOrigin in web/common/toolbar.js:
// from home.lab.heliosian.com the directory is who.lab.heliosian.com, from
// heliosian.com (or www) it is who.heliosian.com, and a local port carries
// over. hca.<tier> is the volunteer portal's older name and counts as team's.
func hiddenHosts(pageHost string, apps []string) map[string]bool {
	if len(apps) == 0 {
		return nil
	}
	host, port, _ := strings.Cut(strings.ToLower(pageHost), ":")
	labels := strings.Split(host, ".")
	if len(labels) > 2 {
		labels = labels[1:]
	}
	tier := strings.Join(labels, ".")
	if port != "" {
		tier += ":" + port
	}
	hidden := map[string]bool{}
	for _, app := range apps {
		hidden[app+"."+tier] = true
		if app == "team" {
			hidden["hca."+tier] = true
		}
	}
	return hidden
}

// linksInto says whether a link's URL opens one of the hosts.
func linksInto(hosts map[string]bool, link string) bool {
	if len(hosts) == 0 {
		return false
	}
	u, err := url.Parse(link)
	if err != nil {
		return false
	}
	return hosts[strings.ToLower(u.Host)]
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(into); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return false
	}
	return true
}

func visibleCell(visible bool) string {
	if visible {
		return "Yes"
	}
	return "No"
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
			slog.ErrorContext(ctx, "apps write", "error", err)
		}
	})
	<-applied
	return true
}

func (a app) logChange(actor, action, kind string, cells map[string]string) error {
	return a.writer.Append(appName, changeLogTab, []string{
		time.Now().Format(time.RFC3339), actor, action, kind,
		cells["Title"], cells["Description"], cells["URL"], cells["Image"], cells["Category"], cells["Visible"], cells["Style"],
	})
}

// importImage stores a picked search result the way an upload is stored.
func (a app) importImage(w http.ResponseWriter, r *http.Request) {
	a.search.ServeImport(w, r, a.store, imageFolder, maxImageSize)
}

func (a app) saveLink(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Original    string `json:"original"`
		Title       string `json:"title"`
		Description string `json:"description"`
		URL         string `json:"url"`
		Image       string `json:"image"`
		Category    string `json:"category"`
		Visible     bool   `json:"visible"`
	}
	if !decode(w, r, &body) {
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" || len(title) > maxTitleLength || len(body.Description) > maxDescLength {
		http.Error(w, "title is required and fields must be short", http.StatusBadRequest)
		return
	}
	cells := map[string]string{
		"Title": title, "Description": strings.TrimSpace(body.Description), "URL": strings.TrimSpace(body.URL),
		"Image": strings.TrimSpace(body.Image), "Category": strings.TrimSpace(body.Category), "Visible": visibleCell(body.Visible),
	}
	action := "edit"
	if body.Original == "" {
		action = "add"
		cells["Added By"] = actor
		cells["Added"] = time.Now().Format(addedFormat)
	}
	tables := a.cache.Tables().withRow(linksTab, body.Original, cells)
	if !a.commit(r.Context(), w, tables, func() error {
		if body.Original == "" {
			if err := a.writer.Append(appName, linksTab, []string{cells["Title"], cells["Description"], cells["URL"], cells["Image"], cells["Category"], cells["Visible"], cells["Added By"], cells["Added"]}); err != nil {
				return err
			}
		} else if err := a.writer.Upsert(appName, linksTab, "Title", body.Original, cells); err != nil {
			return err
		}
		return a.logChange(actor, action, "link", cells)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "apps: saved link", "action", action, "title", title)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteLink(w http.ResponseWriter, r *http.Request) {
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
	tables := a.cache.Tables().withoutRow(linksTab, body.Title)
	if !a.commit(r.Context(), w, tables, func() error {
		if err := a.writer.Delete(appName, linksTab, map[string]string{"Title": body.Title}); err != nil {
			return err
		}
		return a.logChange(actor, "delete", "link", map[string]string{"Title": body.Title})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "apps: deleted link", "title", body.Title)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveCategory(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Original string `json:"original"`
		Title    string `json:"title"`
		Emoji    string `json:"emoji"`
		Style    string `json:"style"`
		// Max is a count, or blank for no limit; it arrives as text since that
		// is what the sheet holds and what an empty field sends.
		Max string `json:"max"`
	}
	if !decode(w, r, &body) {
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" || len(title) > maxTitleLength {
		http.Error(w, "title is required and must be short", http.StatusBadRequest)
		return
	}
	style, err := checkStyle(strings.TrimSpace(body.Style))
	if err != nil {
		http.Error(w, "style "+err.Error(), http.StatusBadRequest)
		return
	}
	emoji := strings.TrimSpace(body.Emoji)
	if err := checkEmoji(emoji); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// The events section keeps its style: it is the one section the portal
	// fills, and can only be one thing. Edited while it is still synthesized,
	// it gets its row now - first, where the page has been showing it.
	virtual := a.virtualEvents(body.Original)
	if virtual {
		style = StyleEvents
	} else if body.Original != "" && a.styleOf(body.Original) == StyleEvents {
		style = StyleEvents
	} else if style == StyleEvents {
		http.Error(w, "the events section is the one the page already has", http.StatusBadRequest)
		return
	}
	// The apps section holds the community apps, not links, and there is
	// only one: a category becomes it once its links are gone, and no other
	// can while it stands.
	if style == StyleApps {
		if other := a.titleOf(StyleApps); other != "" && other != body.Original {
			http.Error(w, fmt.Sprintf("%q is already the community apps section", other), http.StatusBadRequest)
			return
		}
		if body.Original != "" && a.styleOf(body.Original) != StyleApps {
			for _, row := range a.cache.Tables().Links {
				if row["Category"] == body.Original {
					http.Error(w, "move or delete its links first: the community apps section holds no links", http.StatusBadRequest)
					return
				}
			}
		}
	}
	if _, err := checkMax(body.Max); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cells := map[string]string{"Title": title, "Emoji": emoji, "Style": style, "Max": strings.TrimSpace(body.Max)}
	var tables *Tables
	if virtual {
		tables = a.cache.Tables().withRow(categoriesTab, "", cells)
		tables.Categories = append([]map[string]string{tables.Categories[len(tables.Categories)-1]}, tables.Categories[:len(tables.Categories)-1]...)
	} else {
		tables = a.cache.Tables().withRow(categoriesTab, body.Original, cells)
	}
	// A rename carries every link along, since links name their category by title.
	if body.Original != "" && body.Original != title {
		links := cloneRows(tables.Links)
		for _, row := range links {
			if row["Category"] == body.Original {
				row["Category"] = title
			}
		}
		tables.Links = links
	}
	action := "edit"
	if body.Original == "" {
		action = "add"
	}
	if !a.commit(r.Context(), w, tables, func() error {
		if body.Original == "" || virtual {
			if err := a.writer.Append(appName, categoriesTab, []string{title, cells["Emoji"], cells["Style"], cells["Max"]}); err != nil {
				return err
			}
			if virtual {
				if err := a.writer.Reorder(appName, categoriesTab, "Title", rowTitles(tables.Categories)); err != nil {
					return err
				}
			}
		} else {
			if err := a.writer.Upsert(appName, categoriesTab, "Title", body.Original, cells); err != nil {
				return err
			}
			if body.Original != title {
				for _, row := range tables.Links {
					if row["Category"] != title {
						continue
					}
					if err := a.writer.Upsert(appName, linksTab, "Title", row["Title"], map[string]string{"Category": title}); err != nil {
						return err
					}
				}
			}
		}
		return a.logChange(actor, action, "category", cells)
	}) {
		return
	}
	slog.InfoContext(r.Context(), "apps: saved category", "action", action, "title", title)
	w.WriteHeader(http.StatusNoContent)
}

// reorderCategories moves rows rather than rewriting them: the sheet's row
// order is the display order, so this is the only way to reorder from the app.
func (a app) reorderCategories(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Titles []string `json:"titles"`
	}
	if !decode(w, r, &body) {
		return
	}
	tables := a.cache.Tables()
	// Moving the events section while it is still synthesized is what writes
	// its row: appended with its standing name and mark, then ordered with
	// the rest.
	var materialized []string
	for _, title := range body.Titles {
		if a.virtualEvents(title) {
			tables = tables.withRow(categoriesTab, "", map[string]string{"Title": title, "Emoji": EventsEmoji, "Style": StyleEvents})
			materialized = []string{title, EventsEmoji, StyleEvents}
		}
	}
	if len(body.Titles) != len(tables.Categories) {
		http.Error(w, "the order must name every category exactly once", http.StatusBadRequest)
		return
	}
	byTitle := map[string]map[string]string{}
	for _, row := range tables.Categories {
		byTitle[row["Title"]] = row
	}
	ordered := make([]map[string]string, 0, len(body.Titles))
	for _, title := range body.Titles {
		row, ok := byTitle[title]
		if !ok {
			http.Error(w, "unknown category "+title, http.StatusBadRequest)
			return
		}
		delete(byTitle, title)
		ordered = append(ordered, row)
	}
	next := *tables
	next.Categories = ordered
	if !a.commit(r.Context(), w, &next, func() error {
		if materialized != nil {
			if err := a.writer.Append(appName, categoriesTab, materialized); err != nil {
				return err
			}
		}
		if err := a.writer.Reorder(appName, categoriesTab, "Title", body.Titles); err != nil {
			return err
		}
		return a.logChange(actor, "reorder", "category", map[string]string{"Title": strings.Join(body.Titles, ", ")})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "apps: reordered categories", "count", len(body.Titles))
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
	if a.styleOf(body.Title) == StyleEvents {
		http.Error(w, "the events section can be renamed or moved, not deleted", http.StatusBadRequest)
		return
	}
	for _, row := range a.cache.Tables().Links {
		if row["Category"] == body.Title {
			http.Error(w, "move or delete its links first", http.StatusBadRequest)
			return
		}
	}
	tables := a.cache.Tables().withoutRow(categoriesTab, body.Title)
	if !a.commit(r.Context(), w, tables, func() error {
		if err := a.writer.Delete(appName, categoriesTab, map[string]string{"Title": body.Title}); err != nil {
			return err
		}
		return a.logChange(actor, "delete", "category", map[string]string{"Title": body.Title})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "apps: deleted category", "title", body.Title)
	w.WriteHeader(http.StatusNoContent)
}

// styleOf is a category's style as the page has it, "" for no such category
// - the synthesized events section included.
func (a app) styleOf(title string) string {
	for _, c := range a.cache.Model().Categories {
		if c.Title == title {
			return c.Style
		}
	}
	return ""
}

// titleOf is the title of the category with a style, "" for none.
func (a app) titleOf(style string) string {
	for _, c := range a.cache.Model().Categories {
		if c.Style == style {
			return c.Title
		}
	}
	return ""
}

// virtualEvents says whether title names the events section while it is
// still synthesized, with no row of its own yet.
func (a app) virtualEvents(title string) bool {
	for _, c := range a.cache.Model().Categories {
		if c.Title == title {
			return c.Virtual
		}
	}
	return false
}

func rowTitles(rows []map[string]string) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row["Title"])
	}
	return out
}

// uploadImage stores a content-addressed image and returns the name the sheet
// should record; the link or category save that follows references it.
func (a app) uploadImage(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
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
		slog.ErrorContext(r.Context(), "store link image", "error", err)
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
		// Apps are the community apps the App Visibility panel has a card
		// for, each with its mode and list; People is who its pickers offer.
		Apps   []AppVisibility `json:"apps"`
		People []Person        `json:"people"`
	}{Email: email, HasStore: a.store != nil, Admins: a.cache.Admins(a.superAdmins()), Apps: a.cache.AppVisibilities(), People: a.people()}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode apps admin state", "error", err)
	}
}

func (a app) setAdmins(w http.ResponseWriter, r *http.Request) {
	_, ok := a.requireAdmin(w, r)
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
	tables := a.cache.Tables().withAdmins(admins)
	if !a.commit(r.Context(), w, tables, func() error {
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
	slog.InfoContext(r.Context(), "apps: set the admin list", "admins", admins)
	w.WriteHeader(http.StatusNoContent)
}

// setVisibility sets one app's name, tagline, mode and list together, the
// way the admin page edits them: the switch, either field and every add or
// remove save at once. The list is written whichever the mode, so a list
// drawn up while the app is everyone's is there when the switch flips. The
// row's place in the order is kept (setAppOrder moves it).
func (a app) setVisibility(w http.ResponseWriter, r *http.Request) {
	_, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		App        string   `json:"app"`
		Visibility string   `json:"visibility"`
		Emails     []string `json:"emails"`
		Tagline    string   `json:"tagline"`
		Name       string   `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	key := strings.ToLower(strings.TrimSpace(body.App))
	if !appKnown(key) {
		http.Error(w, "app must be one of "+strings.Join(appKeys(), ", "), http.StatusBadRequest)
		return
	}
	if body.Visibility != VisibleToEveryone && body.Visibility != VisibleToList {
		http.Error(w, "visibility must be "+VisibleToEveryone+" or "+VisibleToList, http.StatusBadRequest)
		return
	}
	tagline := strings.TrimSpace(body.Tagline)
	if tagline == "" || len(tagline) > maxDescLength {
		http.Error(w, "a short tagline is required", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len(name) > maxTitleLength {
		http.Error(w, "a short name is required", http.StatusBadRequest)
		return
	}
	app, _ := appByKey(key)
	v := Visibility{Mode: body.Visibility, Emails: normalizeEmails(body.Emails), Tagline: tagline, Name: name, Order: visibilityOf(a.cache.Model(), app).Order}
	tables := a.cache.Tables().withVisibility(key, v)
	if !a.commit(r.Context(), w, tables, func() error {
		return a.writer.Upsert(appName, visibilityTab, "App", key, v.cells())
	}) {
		return
	}
	slog.InfoContext(r.Context(), "apps: set an app's visibility", "app", key, "visibility", v.Mode, "emails", len(v.Emails), "name", v.Name, "tagline", v.Tagline)
	w.WriteHeader(http.StatusNoContent)
}

// setAppOrder takes the whole order at once - every app's key, as the admin
// page has them after a move - and numbers each row from one, so the switch
// and the front page follow.
func (a app) setAppOrder(w http.ResponseWriter, r *http.Request) {
	_, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Apps []string `json:"apps"`
	}
	if !decode(w, r, &body) {
		return
	}
	keys := make([]string, 0, len(body.Apps))
	for _, key := range body.Apps {
		keys = append(keys, strings.ToLower(strings.TrimSpace(key)))
	}
	slices.Sort(keys)
	want := appKeys()
	slices.Sort(want)
	if !slices.Equal(keys, want) {
		http.Error(w, "the order must list every app once: "+strings.Join(appKeys(), ", "), http.StatusBadRequest)
		return
	}
	model := a.cache.Model()
	tables := a.cache.Tables()
	rows := map[string]Visibility{}
	for i, key := range body.Apps {
		key = strings.ToLower(strings.TrimSpace(key))
		app, _ := appByKey(key)
		v := visibilityOf(model, app)
		v.Order = i + 1
		rows[key] = v
		tables = tables.withVisibility(key, v)
	}
	if !a.commit(r.Context(), w, tables, func() error {
		for key, v := range rows {
			if err := a.writer.Upsert(appName, visibilityTab, "App", key, v.cells()); err != nil {
				return err
			}
		}
		return nil
	}) {
		return
	}
	slog.InfoContext(r.Context(), "apps: set the apps' order", "apps", body.Apps)
	w.WriteHeader(http.StatusNoContent)
}
