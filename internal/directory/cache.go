package directory

import (
	"log"
	"sync"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/geocode"
)

const refreshInterval = 5 * time.Minute

type Cache struct {
	source   data.Source
	geocoder *geocode.Client
	mu       sync.RWMutex
	model    *Model
}

func NewCache(source data.Source, geocoder *geocode.Client) (*Cache, error) {
	c := &Cache{source: source, geocoder: geocoder}
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
	c.geocodeFamilies(model)
	c.mu.Lock()
	c.model = model
	c.mu.Unlock()
	log.Printf("loaded directory model: %d people, %d families, %d classrooms, %d sections in %s",
		len(model.People), len(model.Families), len(model.Classrooms), len(model.Sections),
		time.Since(start).Round(time.Millisecond))
	return nil
}

func (c *Cache) geocodeFamilies(model *Model) {
	start := time.Now()
	type job struct {
		key     string
		address string
	}
	pending := []job{}
	for key, family := range model.Families {
		if family.Address != "" {
			pending = append(pending, job{key: key, address: family.Address})
		}
	}
	jobs := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	located := 0
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				point, err := c.geocoder.Lookup(j.address)
				if err != nil {
					log.Printf("[ERROR] %v", err)
					continue
				}
				mu.Lock()
				family := model.Families[j.key]
				family.Lat = point.Lat
				family.Lng = point.Lng
				model.Families[j.key] = family
				located++
				mu.Unlock()
			}
		}()
	}
	for _, j := range pending {
		jobs <- j
	}
	close(jobs)
	wg.Wait()
	log.Printf("geocoded %d of %d family addresses in %s", located, len(pending), time.Since(start).Round(time.Millisecond))
}
