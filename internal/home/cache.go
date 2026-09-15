package home

import (
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"heliosian/internal/data"
)

const refreshInterval = 5 * time.Minute

// Enqueuer serializes sheet writes; the directory's write queue is shared here.
type Enqueuer interface {
	Add(func())
}

type Cache struct {
	source     data.Source
	images     ImageChecker
	superAdmin func(email string) bool
	queue      Enqueuer
	mu         sync.RWMutex
	model      *Model
	tables     *Tables
}

func NewCache(source data.Source, images ImageChecker, superAdmin func(string) bool, queue Enqueuer) (*Cache, error) {
	c := &Cache{source: source, images: images, superAdmin: superAdmin, queue: queue}
	if err := c.refresh(); err != nil {
		return nil, err
	}
	go c.refreshLoop()
	return c, nil
}

func (c *Cache) refreshLoop() {
	for range time.Tick(refreshInterval) {
		c.queue.Add(func() {
			if err := c.refresh(); err != nil {
				slog.Error("apps model refresh", "error", err)
			}
		})
	}
}

func (c *Cache) refresh() error {
	start := time.Now()
	tables, err := ReadTables(c.source)
	if err != nil {
		return err
	}
	model, err := BuildModel(tables, c.images)
	if err != nil {
		return err
	}
	c.set(tables, model)
	links := 0
	for _, category := range model.Categories {
		links += len(category.Links)
	}
	slog.Info("loaded apps model", "categories", len(model.Categories), "links", links, "took", time.Since(start).Round(time.Millisecond))
	return nil
}

func (c *Cache) set(tables *Tables, model *Model) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tables = tables
	c.model = model
}

func (c *Cache) Model() *Model {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.model
}

func (c *Cache) Tables() *Tables {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tables
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

// Person is someone the admin page can list an app for: the directory as a
// picker sees it.
type Person struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// AppVisibility is one app as the admin page shows it: its name and
// tagline, its mode, and the list, whether or not the mode is using it.
type AppVisibility struct {
	App
	Visibility string   `json:"visibility"`
	Emails     []string `json:"emails"`
}

// visibilityOf is an app's row as the page reads it: the sheet's, or for an
// app the sheet has no row for yet, the new-app default - a list with nobody
// on it, and the registry's name and tagline.
func visibilityOf(model *Model, app App) Visibility {
	v, ok := model.Visibility[app.Key]
	if !ok {
		v = Visibility{Mode: VisibleToList}
	}
	if v.Emails == nil {
		v.Emails = []string{}
	}
	if v.Tagline == "" {
		v.Tagline = app.Tagline
	}
	if v.Name == "" {
		v.Name = app.Name
	}
	return v
}

// orderedApps is the registry in the order the sheet gives it: every app
// with an Order cell first, by it, then the rest in the registry's order.
func orderedApps(model *Model) []App {
	out := slices.Clone(Apps)
	place := func(app App) int {
		if v, ok := model.Visibility[app.Key]; ok && v.Order > 0 {
			return v.Order
		}
		return len(Apps) + 1 + slices.Index(Apps, app)
	}
	slices.SortStableFunc(out, func(a, b App) int { return place(a) - place(b) })
	return out
}

// AppVisibilities is every app's visibility for the admin page.
func (c *Cache) AppVisibilities() []AppVisibility {
	model := c.Model()
	out := make([]AppVisibility, 0, len(Apps))
	for _, app := range orderedApps(model) {
		v := visibilityOf(model, app)
		app.Name, app.Tagline, app.Mark = v.Name, v.Tagline, markVersion(app.Key)
		out = append(out, AppVisibility{App: app, Visibility: v.Mode, Emails: v.Emails})
	}
	return out
}

// AppList is the registry as the switch and the front page show it: each
// app with the name and tagline its row gives it, in the row's order.
func (c *Cache) AppList() []App {
	model := c.Model()
	out := make([]App, 0, len(Apps))
	for _, app := range orderedApps(model) {
		v := visibilityOf(model, app)
		app.Name, app.Tagline, app.Mark = v.Name, v.Tagline, markVersion(app.Key)
		out = append(out, app)
	}
	return out
}

// HiddenApps is which apps are narrowed to a list this person is not on -
// the rows the toolbar leaves off their switch and the links the front page
// leaves out. A new app, with no row yet, is on nobody's list.
func (c *Cache) HiddenApps(email string) []string {
	email = strings.ToLower(strings.TrimSpace(email))
	hidden := []string{}
	model := c.Model()
	for _, app := range Apps {
		v := visibilityOf(model, app)
		if v.Mode == VisibleToList && !slices.Contains(v.Emails, email) {
			hidden = append(hidden, app.Key)
		}
	}
	return hidden
}

// MissingVisibility is the registry's apps the sheet has no row for yet.
func (c *Cache) MissingVisibility() []App {
	model := c.Model()
	out := []App{}
	for _, app := range Apps {
		if _, ok := model.Visibility[app.Key]; !ok {
			out = append(out, app)
		}
	}
	return out
}

func (c *Cache) tabAdmins() []string {
	tables := c.Tables()
	emails := make([]string, 0, len(tables.Admins))
	for _, row := range tables.Admins {
		emails = append(emails, row["Email"])
	}
	return normalizeEmails(emails)
}

// IsSuperAdmin reports whether email is one of the platform's super admins
// (docs/config.md) - the tier that colours the app in Appearance.
func (c *Cache) IsSuperAdmin(email string) bool {
	return c.superAdmin(strings.ToLower(strings.TrimSpace(email)))
}

// IsAdmin reports whether email may edit: a row in the Admins tab, or a
// platform super admin.
func (c *Cache) IsAdmin(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, admin := range c.tabAdmins() {
		if admin == email {
			return true
		}
	}
	return c.superAdmin(email)
}

// Admins is every admin as the admin page lists them: the tab plus the super
// admins, indistinguishable, sorted together.
func (c *Cache) Admins(superAdmins []string) []string {
	admins := normalizeEmails(append(c.tabAdmins(), superAdmins...))
	sort.Strings(admins)
	return admins
}

// Grant puts someone on an app's list, so the app shows on their home when
// the app is only its list's - for an app that takes people on itself, the
// way Staff Birthdays takes a volunteer. The list is written whichever the
// mode, as the admin page writes it; someone already on it is left be.
func Grant(cache *Cache, writer data.Writer, queue Enqueuer, appKey, email string) error {
	app, ok := appByKey(appKey)
	if !ok {
		return fmt.Errorf("no app %q", appKey)
	}
	email = strings.ToLower(strings.TrimSpace(email))
	v := visibilityOf(cache.Model(), app)
	if slices.Contains(v.Emails, email) {
		return nil
	}
	v.Emails = normalizeEmails(append(v.Emails, email))
	tables := cache.Tables().withVisibility(app.Key, v)
	model, err := BuildModel(tables, cache.images)
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	queue.Add(func() {
		cache.set(tables, model)
		done <- writer.Upsert(appName, visibilityTab, "App", app.Key, v.cells())
	})
	return <-done
}
