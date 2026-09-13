package calendar

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
	// images says which tag image names can be served; nil in a test.
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
		c.queue.Add(func() {
			if err := c.refresh(); err != nil {
				slog.Error("calendar model refresh", "error", err)
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

// resolveImages turns each tag's image name into the address the page
// fetches it from, once per model rather than per request. A name that
// resolves to nothing is logged and left blank: a missing picture is never
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
	if err := c.images.Prefetch(names); err != nil {
		slog.Error("calendar: prefetch tag images", "error", err)
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
