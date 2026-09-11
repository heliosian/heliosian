package events

import (
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"heliosian/internal/config"
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
	err        error
}

// NewCache loads the sheet and keeps reloading it. A sheet that will not load
// is returned as the error, but the cache is still usable and keeps trying on
// the refresh interval: Model and Tables are nil until a load succeeds, and Err
// says why. That way a broken Events sheet stalls the portal rather than the
// server it shares with the directory, and comes back once the sheet is fixed
// without a restart.
func NewCache(source data.Source, images ImageChecker, superAdmin func(string) bool, queue Enqueuer) (*Cache, error) {
	c := &Cache{source: source, images: images, superAdmin: superAdmin, queue: queue}
	err := c.refresh()
	go c.refreshLoop()
	return c, err
}

func (c *Cache) refreshLoop() {
	for range time.Tick(refreshInterval) {
		c.queue.Add(func() {
			if err := c.refresh(); err != nil {
				slog.Error("events model refresh", "error", err)
			}
		})
	}
}

func (c *Cache) refresh() error {
	start := time.Now()
	tables, err := ReadTables(c.source)
	if err == nil {
		var model *Model
		if model, err = BuildModel(tables, c.images); err == nil {
			c.set(tables, model)
		}
	}
	c.mu.Lock()
	c.err = err
	c.mu.Unlock()
	if err != nil {
		return err
	}
	model := c.Model()
	children, volunteers := 0, len(tables.Volunteers)
	for _, a := range model.Activities {
		children += len(a.Descendants())
	}
	slog.Info("loaded events model", "categories", len(model.Categories), "roots", len(model.Activities),
		"children", children, "volunteers", volunteers, "skipped", model.Skipped, "took", time.Since(start).Round(time.Millisecond))
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

// Err is why the last load failed, or nil. A failed refresh keeps the previous
// model serving, so Err can be set while Model is still usable.
func (c *Cache) Err() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.err
}

func (c *Cache) tabAdmins() []string {
	tables := c.Tables()
	if tables == nil {
		return nil
	}
	emails := make([]string, 0, len(tables.Admins))
	for _, row := range tables.Admins {
		emails = append(emails, row["Email"])
	}
	return config.NormalizeEmails(emails)
}

// IsAdmin reports whether email runs the portal: a row in the Admins tab, or a
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
	admins := config.NormalizeEmails(append(c.tabAdmins(), superAdmins...))
	sort.Strings(admins)
	return admins
}
