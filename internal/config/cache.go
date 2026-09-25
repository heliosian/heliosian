package config

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"heliosian/internal/data"
)

const refreshInterval = 5 * time.Minute

// Enqueuer serializes sheet writes; the shared write queue is passed here.
type Enqueuer interface {
	Add(func())
}

type Cache struct {
	source   data.Source
	writer   data.Writer
	queue    Enqueuer
	mu       sync.RWMutex
	tables   *Tables
	settings *Settings
	edits    int
	commits  sync.Mutex
}

func NewCache(source data.Source, writer data.Writer, queue Enqueuer) (*Cache, error) {
	c := &Cache{source: source, writer: writer, queue: queue}
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
			slog.Error("config refresh", "error", err)
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
	settings, err := Parse(tables)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if c.edits != before {
		c.mu.Unlock()
		slog.Info("config refresh skipped: edited while reading")
		return nil
	}
	c.tables, c.settings = tables, settings
	c.mu.Unlock()
	slog.Info("loaded config", "superAdmins", len(settings.SuperAdmins), "gradeColors", len(settings.GradeColors),
		"classroomColors", len(settings.ClassroomColors), "took", time.Since(start).Round(time.Millisecond))
	return nil
}

func (c *Cache) set(tables *Tables, settings *Settings) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tables = tables
	c.settings = settings
	c.edits++
}

func (c *Cache) Settings() *Settings {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.settings
}

func (c *Cache) tablesNow() *Tables {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tables
}

func (c *Cache) SuperAdmins() []string {
	return c.Settings().SuperAdmins
}

func (c *Cache) IsSuperAdmin(email string) bool {
	return slices.Contains(c.SuperAdmins(), email)
}

func (c *Cache) SignedOut(email string) (time.Time, bool) {
	at, ok := c.Settings().SignedOut[strings.ToLower(strings.TrimSpace(email))]
	return at, ok
}

func (c *Cache) SignOut(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	at := time.Now().Truncate(time.Second)
	return c.update(ctx, "sign out", func(t *Tables) *Tables { return t.WithSignedOut(email, at) }, func() error {
		return WriteSignedOut(c.writer, email, at)
	})
}

// update mirrors a write into the tables and parses the result first, so a change
// the sheet rules reject never reaches the sheet - otherwise the sheet ends up
// holding a value no future load can read, including the next server start - then
// applies it in memory and persists it behind every earlier write.
func (c *Cache) update(ctx context.Context, action string, mirror func(*Tables) *Tables, persist func() error) error {
	c.commits.Lock()
	defer c.commits.Unlock()
	tables := mirror(c.tablesNow())
	settings, err := Parse(tables)
	if err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	c.set(tables, settings)
	c.queue.Add(func() {
		if err := persist(); err != nil {
			slog.ErrorContext(ctx, "config write", "action", action, "error", err)
		}
	})
	return nil
}
