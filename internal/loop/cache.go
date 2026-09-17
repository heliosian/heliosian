package loop

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
	superAdmin func(email string) bool
	queue      Enqueuer
	mu         sync.RWMutex
	model      *Model
	tables     *Tables
	// edits counts every change applied from a request, so a refresh that
	// read the sheet before one landed knows not to put the older sheet back.
	edits int
}

func NewCache(source data.Source, superAdmin func(string) bool, queue Enqueuer) (*Cache, error) {
	c := &Cache{source: source, superAdmin: superAdmin, queue: queue}
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
				slog.Error("groups model refresh", "error", err)
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
	if err != nil {
		return err
	}
	model, err := BuildModel(tables)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if c.edits == before {
		c.tables, c.model = tables, model
	} else {
		slog.Info("groups model refresh skipped: edited while reading")
	}
	c.mu.Unlock()
	rules := 0
	for _, g := range model.Groups {
		rules += len(g.Rules)
	}
	slog.Info("loaded groups model", "groups", len(model.Groups), "rules", rules, "took", time.Since(start).Round(time.Millisecond))
	return nil
}

func (c *Cache) set(tables *Tables, model *Model) {
	c.mu.Lock()
	c.tables = tables
	c.model = model
	c.edits++
	c.mu.Unlock()
}

func (c *Cache) edit(fn func(*Tables) *Tables) {
	c.mu.Lock()
	c.tables = fn(c.tables)
	c.edits++
	c.mu.Unlock()
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

func (c *Cache) tabAdmins() []string {
	tables := c.Tables()
	emails := make([]string, 0, len(tables.Admins))
	for _, row := range tables.Admins {
		emails = append(emails, row["Email"])
	}
	return config.NormalizeEmails(emails)
}

// IsSuperAdmin reports whether email is one of the platform's super admins
// (docs/config.md) - the tier that colours the app in Appearance.
func (c *Cache) IsSuperAdmin(email string) bool {
	return c.superAdmin(strings.ToLower(strings.TrimSpace(email)))
}

// IsAdmin reports whether email runs the app - sees and edits every group:
// a row in the Admins tab, or a platform super admin.
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
