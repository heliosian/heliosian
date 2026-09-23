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
	"heliosian/internal/filter"
)

const refreshInterval = 5 * time.Minute

type Enqueuer interface {
	Add(func())
}

type Cache struct {
	source      data.Source
	images      ImageChecker
	superAdmins func() []string
	queue       Enqueuer
	directory   Directory
	mu          sync.RWMutex
	model       *Model
	tables      *Tables
}

func (c *Cache) includes(rules []filter.Rule, email string) bool {
	return c.directory != nil && len(rules) > 0 && filter.OnList(filter.List{Rules: rules, Editors: c.Admins()}, c.directory.Sources(), email)
}

func NewCache(source data.Source, images ImageChecker, superAdmins func() []string, queue Enqueuer) (*Cache, error) {
	c := &Cache{source: source, images: images, superAdmins: superAdmins, queue: queue}
	if err := c.refresh(); err != nil {
		return nil, err
	}
	go c.refreshLoop()
	return c, nil
}

func (c *Cache) refreshLoop() {
	for range time.Tick(refreshInterval) {
		c.Refresh()
	}
}

func (c *Cache) Refresh() {
	c.queue.Add(func() {
		if err := c.refresh(); err != nil {
			slog.Error("apps model refresh", "error", err)
		}
	})
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

type Person struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type AppVisibility struct {
	App
	Visibility string        `json:"visibility"`
	Emails     []string      `json:"emails"`
	Rules      []filter.Rule `json:"rules"`
}

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

func (c *Cache) AppVisibilities() []AppVisibility {
	model := c.Model()
	out := make([]AppVisibility, 0, len(Apps))
	for _, app := range orderedApps(model) {
		v := visibilityOf(model, app)
		app.Name, app.Tagline, app.Mark = v.Name, v.Tagline, markVersion(app.Key)
		out = append(out, AppVisibility{App: app, Visibility: v.Mode, Emails: v.Emails, Rules: v.Rules})
	}
	return out
}

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

func (c *Cache) HiddenApps(email string) []string {
	email = strings.ToLower(strings.TrimSpace(email))
	hidden := []string{}
	model := c.Model()
	for _, app := range Apps {
		v := visibilityOf(model, app)
		if v.Mode == VisibleToList && !slices.Contains(v.Emails, email) && !c.includes(v.Rules, email) {
			hidden = append(hidden, app.Key)
		}
	}
	return hidden
}

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

func (c *Cache) IsSuperAdmin(email string) bool {
	return slices.Contains(normalizeEmails(c.superAdmins()), strings.ToLower(strings.TrimSpace(email)))
}

func (c *Cache) IsAdmin(email string) bool {
	return slices.Contains(c.Admins(), strings.ToLower(strings.TrimSpace(email)))
}

func (c *Cache) Admins() []string {
	admins := normalizeEmails(append(c.tabAdmins(), c.superAdmins()...))
	sort.Strings(admins)
	return admins
}

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
