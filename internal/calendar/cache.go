package calendar

import (
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"heliosian/internal/config"
	"heliosian/internal/data"
)

const refreshInterval = 5 * time.Minute

type Enqueuer interface {
	Add(func())
}

type Cache struct {
	source     data.Source
	roster     func() Roster
	superAdmin func(email string) bool
	queue      Enqueuer
	mu         sync.RWMutex
	model      *Model
	tables     *Tables
	edits      int
	// images says which image names can be served; nil in a test.
	images ImageChecker
}

func NewCache(source data.Source, roster func() Roster, images ImageChecker, superAdmin func(string) bool, queue Enqueuer) (*Cache, error) {
	c := &Cache{source: source, roster: roster, images: images, superAdmin: superAdmin, queue: queue}
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

// Refresh reloads the model from the sheet ahead of the next tick, behind whatever writes are queued.
func (c *Cache) Refresh() {
	c.queue.Add(func() {
		if err := c.refresh(); err != nil {
			slog.Error("calendar model refresh", "error", err)
		}
	})
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
	model, err := BuildModel(tables, c.roster())
	if err != nil {
		return err
	}
	c.resolveImages(model)
	c.mu.Lock()
	if c.edits == before {
		c.tables, c.model = tables, model
	} else {
		slog.Info("calendar model refresh skipped: edited while reading")
	}
	c.mu.Unlock()
	slog.Info("loaded calendar model", "events", len(model.Events), "hidden", model.Hidden, "days", len(model.Days),
		"feeds", len(model.Feeds), "skipped", model.Skipped, "took", time.Since(start).Round(time.Millisecond))
	return nil
}

// resolveImages asks the store for every picture the model names - each
// tag's, each hand-added event's, each invitation's flyer - once per model
// rather than per request, so the store holds them for serving, and turns
// each tag's image name into the address the page fetches it from. A name
// that resolves to nothing is logged and left: a missing picture is never
// a reason to refuse the calendar.
func (c *Cache) resolveImages(model *Model) {
	if c.images == nil {
		return
	}
	names := []string{}
	for _, t := range model.Tags {
		if t.Image != "" {
			names = append(names, t.Image)
		}
	}
	pictures := map[string]string{}
	for _, e := range slices.Concat(model.Events, model.Pending) {
		if e.Source == SourceSheet && e.Image != "" {
			pictures["event "+e.ID] = e.Image
		}
	}
	for id, inv := range model.Invitations {
		if inv.Flyer != "" {
			pictures["flyer "+id] = inv.Flyer
		}
	}
	for _, name := range pictures {
		names = append(names, name)
	}
	if err := c.images.Prefetch(names); err != nil {
		slog.Error("calendar: prefetch images", "error", err)
	}
	for i := range model.Tags {
		t := &model.Tags[i]
		if t.Image == "" {
			continue
		}
		found, err := c.images.Has(t.Image)
		if err != nil {
			slog.Error("calendar: tag image", "tag", t.Name, "image", t.Image, "error", err)
		} else if !found {
			slog.Warn("calendar: tag image does not exist", "tag", t.Name, "image", t.Image)
		} else {
			t.ImageURL = "/" + t.Image
		}
	}
	for what, name := range pictures {
		found, err := c.images.Has(name)
		if err != nil {
			slog.Error("calendar: picture", "of", what, "image", name, "error", err)
		} else if !found {
			slog.Warn("calendar: picture does not exist", "of", what, "image", name)
		}
	}
}

func (c *Cache) set(tables *Tables, model *Model) {
	c.resolveImages(model)
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

func (c *Cache) IsAdmin(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, admin := range c.tabAdmins() {
		if admin == email {
			return true
		}
	}
	return c.superAdmin(email)
}

func (c *Cache) Admins(superAdmins []string) []string {
	admins := config.NormalizeEmails(append(c.tabAdmins(), superAdmins...))
	sort.Strings(admins)
	return admins
}
