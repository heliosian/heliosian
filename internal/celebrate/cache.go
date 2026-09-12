package celebrate

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
	// edits counts every change applied from a request, so a refresh that
	// read the sheet before one landed knows not to put the older sheet back.
	edits int
}

// NewCache loads the sheet and keeps reloading it. A sheet that will not load
// is returned as the error, but the cache is still usable and keeps trying on
// the refresh interval: Model and Tables are nil until a load succeeds, and
// Err says why. The sheet is one admins edit by hand, so a broken one stalls
// this app rather than the server it shares with the directory.
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
				slog.Error("celebrate model refresh", "error", err)
			}
		})
	}
}

func (c *Cache) refresh() error {
	start := time.Now()
	c.mu.RLock()
	before := c.edits
	c.mu.RUnlock()
	tables, err := ReadTables(c.source)
	if err == nil {
		var model *Model
		if model, err = BuildModel(tables, c.images); err == nil {
			c.mu.Lock()
			if c.edits == before {
				c.tables, c.model = tables, model
			} else {
				slog.Info("celebrate model refresh skipped: edited while reading")
			}
			c.mu.Unlock()
		}
	}
	c.mu.Lock()
	c.err = err
	c.mu.Unlock()
	if err != nil {
		return err
	}
	model := c.Model()
	tickets := 0
	for _, p := range model.Parties {
		tickets += len(p.Tickets)
	}
	slog.Info("loaded celebrate model", "celebrations", len(model.Celebrations), "parties", len(model.Parties),
		"tickets", tickets, "skipped", model.Skipped, "took", time.Since(start).Round(time.Millisecond))
	return nil
}

// set applies a request's change. It counts as an edit, which a refresh in
// flight defers to.
func (c *Cache) set(tables *Tables, model *Model) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tables = tables
	c.model = model
	c.edits++
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

// IsAdmin reports whether email runs the app: a row in the Admins tab, or a
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
