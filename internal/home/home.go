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
	alerts      func(string) (int, bool)
	upcoming    func() []Event
	search      imagesearch.Search
}

// Event is an HCA-Team event as the front page's Upcoming Events lists it:
// the portal reckons which are ahead, and the page links across to it.
type Event struct {
	Title       string `json:"title"`
	Path        string `json:"path"`
	Start       string `json:"start"`
	When        string `json:"when"`
	StartAt     string `json:"startAt"`
	EndAt       string `json:"endAt,omitempty"`
	Location    string `json:"location,omitempty"`
	Description string `json:"description,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
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
// upcoming is the volunteer portal's list of what is ahead.
func Register(mux *http.ServeMux, cache *Cache, writer data.Writer, queue Enqueuer, store *blob.Store, superAdmins func() []string, heroPhoto func(string) string, alerts func(string) (int, bool), upcoming func() []Event, search imagesearch.Search) {
	if search.UserAgent == "" {
		search.UserAgent = "Heliosian image search (+https://heliosian.com)"
	}
	a := app{cache: cache, writer: writer, queue: queue, store: store, superAdmins: superAdmins, heroPhoto: heroPhoto, alerts: alerts, upcoming: upcoming, search: search}
	mux.HandleFunc("GET /{$}", a.page)
	mux.HandleFunc("GET /admin", a.adminPage)
	mux.HandleFunc("GET /dl/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /api/apps/model", a.model)
	mux.HandleFunc("POST /api/apps/link", a.saveLink)
	mux.HandleFunc("DELETE /api/apps/link", a.deleteLink)
	mux.HandleFunc("POST /api/apps/category", a.saveCategory)
	mux.HandleFunc("DELETE /api/apps/category", a.deleteCategory)
	mux.HandleFunc("POST /api/apps/categories/order", a.reorderCategories)
	mux.HandleFunc("POST /api/apps/image", a.uploadImage)
	mux.HandleFunc("GET /api/apps/images/search", a.requireAdminFunc(a.search.ServeSearch))
	mux.HandleFunc("POST /api/apps/images/import", a.requireAdminFunc(a.importImage))
	mux.HandleFunc("GET /api/admin/state", a.adminState)
	mux.HandleFunc("POST /api/admin/admins", a.setAdmins)
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
// greyed out; everyone else gets the visible ones.
func (a app) model(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(auth.Email(r))
	admin := a.cache.IsAdmin(email)
	full := a.cache.Model()
	categories := make([]Category, 0, len(full.Categories))
	for _, category := range full.Categories {
		shown := Category{Title: category.Title, Emoji: category.Emoji, Style: category.Style, Max: category.Max, Links: []Link{}, Virtual: category.Virtual}
		for _, link := range category.Links {
			if link.Visible || admin {
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
	}{
		Categories:   categories,
		User:         user{Email: email, Initial: strings.ToUpper(email[:1]), PhotoURL: a.heroPhoto(email), IsAdmin: admin},
		ImageSources: a.search.Sources(),
		Upcoming:     a.upcoming(),
	}
	view.Alerts.Stale, view.Alerts.Privacy = a.alerts(email)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode apps model", "error", err)
	}
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
	}{Email: email, HasStore: a.store != nil, Admins: a.cache.Admins(a.superAdmins())}
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
