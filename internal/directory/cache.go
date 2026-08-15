package directory

import (
	"log"
	"sync"
	"time"

	"heliosian/internal/data"
)

const refreshInterval = 5 * time.Minute

type Cache struct {
	source data.Source
	mu     sync.RWMutex
	model  *Model
}

func NewCache(source data.Source) (*Cache, error) {
	c := &Cache{source: source}
	if err := c.refresh(); err != nil {
		return nil, err
	}
	go c.refreshLoop()
	return c, nil
}

func (c *Cache) Model() *Model {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.model
}

func (c *Cache) refreshLoop() {
	for range time.Tick(refreshInterval) {
		if err := c.refresh(); err != nil {
			log.Printf("[ERROR] directory model refresh: %v", err)
		}
	}
}

func (c *Cache) refresh() error {
	start := time.Now()
	model, err := LoadModel(c.source)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.model = model
	c.mu.Unlock()
	log.Printf("loaded directory model: %d people, %d families, %d classrooms, %d sections in %s",
		len(model.People), len(model.Families), len(model.Classrooms), len(model.Sections),
		time.Since(start).Round(time.Millisecond))
	return nil
}
