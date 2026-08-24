package directory

import (
	"log"
	"sync"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/geocode"
)

const refreshInterval = 5 * time.Minute

type Geocoder interface {
	Lookup(address string) (geocode.Point, error)
}

type Cache struct {
	source   data.Source
	geocoder Geocoder
	blobs    BlobChecker
	static   BlobChecker
	build    sync.Mutex
	mu       sync.RWMutex
	model    *Model
	tables   *Tables
}

func NewCache(source data.Source, geocoder Geocoder, blobs, static BlobChecker) (*Cache, error) {
	c := &Cache{source: source, geocoder: geocoder, blobs: blobs, static: static}
	if err := c.refresh(); err != nil {
		return nil, err
	}
	go c.refreshLoop()
	return c, nil
}

func (c *Cache) Refresh() error {
	return c.refresh()
}

// Rebuild reruns the model over the tables already in memory, for changes that
// alter no sheet cell.
func (c *Cache) Rebuild() error {
	start := time.Now()
	c.build.Lock()
	defer c.build.Unlock()
	return c.rebuild(c.currentTables(), start)
}

// ApplyOverride folds a just-written Overrides change into the cached tables and
// reruns the model, so a write costs no sheet read.
func (c *Cache) ApplyOverride(email string, cells map[string]string) error {
	start := time.Now()
	c.build.Lock()
	defer c.build.Unlock()
	return c.rebuild(c.currentTables().withOverride(email, cells), start)
}

func (c *Cache) Model() *Model {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.model
}

func (c *Cache) currentTables() *Tables {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tables
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
	c.build.Lock()
	defer c.build.Unlock()
	tables, err := ReadTables(c.source)
	if err != nil {
		return err
	}
	return c.rebuild(tables, start)
}

func (c *Cache) rebuild(tables *Tables, start time.Time) error {
	model, err := BuildModel(tables, c.blobs, c.static)
	if err != nil {
		return err
	}
	c.geocodeFamilies(model)
	c.mu.Lock()
	c.model = model
	c.tables = tables
	c.mu.Unlock()
	log.Printf("loaded directory model: %d people, %d families, %d classrooms, %d crews in %s",
		len(model.People), len(model.Families), len(model.Classrooms), len(model.Crews),
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
