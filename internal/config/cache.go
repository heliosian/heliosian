package config

import (
	"fmt"
	"log"
	"slices"
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
	queue    Enqueuer
	mu       sync.RWMutex
	tables   *Tables
	settings *Settings
}

func NewCache(source data.Source, queue Enqueuer) (*Cache, error) {
	c := &Cache{source: source, queue: queue}
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
				log.Printf("[ERROR] config refresh: %v", err)
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
	settings, err := Parse(tables)
	if err != nil {
		return err
	}
	c.set(tables, settings)
	log.Printf("loaded config: %d super admins, %d grade colors, %d classroom colors in %s",
		len(settings.SuperAdmins), len(settings.GradeColors), len(settings.ClassroomColors),
		time.Since(start).Round(time.Millisecond))
	return nil
}

func (c *Cache) set(tables *Tables, settings *Settings) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tables = tables
	c.settings = settings
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

// update mirrors a write into the tables and parses the result first, so a change
// the sheet rules reject never reaches the sheet - otherwise the sheet ends up
// holding a value no future load can read, including the next server start - then
// applies it in memory and persists it behind every earlier write.
func (c *Cache) update(action string, mirror func(*Tables) *Tables, persist func() error) error {
	applied := make(chan error, 1)
	c.queue.Add(func() {
		tables := mirror(c.tablesNow())
		settings, err := Parse(tables)
		applied <- err
		if err != nil {
			return
		}
		c.set(tables, settings)
		if err := persist(); err != nil {
			log.Printf("[ERROR] %s: %v", action, err)
		}
	})
	if err := <-applied; err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	return nil
}
