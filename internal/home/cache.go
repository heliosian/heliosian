package home

import (
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

// AppVisibility is one app as the admin page shows it: its name, its mode,
// and the list, whether or not the mode is using it.
type AppVisibility struct {
	App
	Visibility string   `json:"visibility"`
	Emails     []string `json:"emails"`
}

// AppVisibilities is every app's visibility for the admin page, an app with
// no row of its own everyone's with nobody listed.
func (c *Cache) AppVisibilities() []AppVisibility {
	model := c.Model()
	out := make([]AppVisibility, 0, len(Apps))
	for _, app := range Apps {
		v := model.Visibility[app.Key]
		if v.Mode == "" {
			v.Mode = VisibleToEveryone
		}
		if v.Emails == nil {
			v.Emails = []string{}
		}
		out = append(out, AppVisibility{App: app, Visibility: v.Mode, Emails: v.Emails})
	}
	return out
}

// HiddenApps is which apps are narrowed to a list this person is not on -
// the rows the toolbar leaves off their switch and the links the front page
// leaves out. Empty for nearly everyone: an app is everyone's until narrowed.
func (c *Cache) HiddenApps(email string) []string {
	email = strings.ToLower(strings.TrimSpace(email))
	hidden := []string{}
	for _, app := range Apps {
		v := c.Model().Visibility[app.Key]
		if v.Mode == VisibleToList && !slices.Contains(v.Emails, email) {
			hidden = append(hidden, app.Key)
		}
	}
	return hidden
}

func (c *Cache) tabAdmins() []string {
	tables := c.Tables()
	emails := make([]string, 0, len(tables.Admins))
	for _, row := range tables.Admins {
		emails = append(emails, row["Email"])
	}
	return normalizeEmails(emails)
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
