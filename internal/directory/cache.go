package directory

import (
	"errors"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/geocode"
)

const refreshInterval = 5 * time.Minute

var errNoStore = errors.New("image uploads require real-data mode")

type Geocoder interface {
	Lookup(address string) (geocode.Point, error)
}

type Cache struct {
	source   data.Source
	geocoder Geocoder
	blobs    BlobChecker
	static   BlobChecker
	store    *blob.Store
	queue    *Queue
	mu       sync.RWMutex
	model    *Model
	tables   *Tables
	settings Settings

	// superEdit tracks, per admin, whether they've turned on the switch that lets them
	// edit anyone's record rather than just their own family's. Deliberately
	// in-memory only: it resets on every restart rather than staying on forever.
	superEdit map[string]bool

	// spoof tracks, per admin, which other person they're currently viewing the
	// directory as. View-only by design — it changes what an admin sees, never who a
	// write is attributed to, so it never touches upload.go's identity resolution.
	spoof map[string]string
}

// store is the concrete blob store (nil in sample mode), needed for admin operations
// — reading and writing settings, and replacing a classroom or grade image in place —
// which need more than the existence check BlobChecker exposes.
func NewCache(source data.Source, geocoder Geocoder, blobs, static BlobChecker, store *blob.Store, queue *Queue) (*Cache, error) {
	c := &Cache{
		source: source, geocoder: geocoder, blobs: blobs, static: static, store: store, queue: queue,
		settings: loadSettings(store), superEdit: map[string]bool{}, spoof: map[string]string{},
	}
	start := time.Now()
	tables, err := ReadTables(source)
	if err != nil {
		return nil, err
	}
	if err := c.rebuild(tables, start); err != nil {
		return nil, err
	}
	go c.refreshLoop()
	return c, nil
}

// rebuildCurrent reruns the model over the tables already in memory, for changes
// that alter no sheet cell.
func (c *Cache) rebuildCurrent() error {
	return c.rebuild(c.currentTables(), time.Now())
}

// applyOverride folds an Overrides change into the cached tables and reruns the
// model, so a write costs no sheet read.
func (c *Cache) applyOverride(email string, cells map[string]string) error {
	return c.rebuild(c.currentTables().withOverride(email, cells), time.Now())
}

// applyPhotos folds a Photos-sheet rewrite for one person - an upload, a reorder, or
// a delete, all just "this person's photo list is now exactly names" - into the
// cached tables and reruns the model, mirroring applyOverride's trick so a write is
// visible on the very next model build rather than only after the next sheet read.
// cells, if non-nil, folds in an Overrides change (e.g. retiring the legacy primary
// pointer) as part of the same rebuild.
func (c *Cache) applyPhotos(email string, names []string, cells map[string]string) error {
	tables := c.currentTables().withPhotos(email, names)
	if len(cells) > 0 {
		tables = tables.withOverride(email, cells)
	}
	return c.rebuild(tables, time.Now())
}

// HasStore reports whether a real blob store is configured, which the admin page uses
// to explain why image uploads are unavailable in sample mode.
func (c *Cache) HasStore() bool {
	return c.store != nil
}

func (c *Cache) Settings() Settings {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.settings
}

// IsAdmin reports whether email may use the admin tools at all — either tier.
func (c *Cache) IsAdmin(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, admin := range c.settings.Admins {
		if admin == email {
			return true
		}
	}
	for _, admin := range c.settings.SuperAdmins {
		if admin == email {
			return true
		}
	}
	return false
}

// IsSuperAdmin reports whether email is on the super admin list specifically — the
// tier that can spoof another user and manage who else is a super admin. Regular
// admins never see anything gated on this, including that it exists.
func (c *Cache) IsSuperAdmin(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, admin := range c.settings.SuperAdmins {
		if admin == email {
			return true
		}
	}
	return false
}

// SuperEditEnabled reports whether this admin has switched on editing anyone's
// record. Meaningless (and never checked) for a non-admin.
func (c *Cache) SuperEditEnabled(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.superEdit[email]
}

func (c *Cache) SetSuperEdit(email string, enabled bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	c.mu.Lock()
	defer c.mu.Unlock()
	if enabled {
		c.superEdit[email] = true
	} else {
		delete(c.superEdit, email)
	}
}

// SpoofTarget returns who this admin is currently viewing the directory as, or "" if
// they're viewing as themselves.
func (c *Cache) SpoofTarget(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.spoof[email]
}

func (c *Cache) SetSpoof(email, target string) {
	email = strings.ToLower(strings.TrimSpace(email))
	target = strings.ToLower(strings.TrimSpace(target))
	c.mu.Lock()
	defer c.mu.Unlock()
	if target == "" {
		delete(c.spoof, email)
	} else {
		c.spoof[email] = target
	}
}

// UpdateSettings saves the new settings (when a store is configured) and rebuilds the
// model so a changed threshold or a replaced image is reflected immediately, not on
// the next five-minute tick.
func (c *Cache) UpdateSettings(settings Settings) error {
	if err := saveSettings(c.store, settings); err != nil {
		return err
	}
	c.mu.Lock()
	c.settings = settings
	c.mu.Unlock()
	return c.rebuildCurrent()
}

// PutImage replaces a classroom or grade image in the bucket and rebuilds the model so
// the new URL (the object's generation changes, so its content isn't cached under the
// old one) shows up right away. Real-data mode only: there is no bucket in sample mode.
func (c *Cache) PutImage(folder, name, mimeType string, content []byte) error {
	if c.store == nil {
		return errNoStore
	}
	if err := c.store.PutNamed(folder, name, mimeType, content); err != nil {
		return err
	}
	return c.rebuildCurrent()
}

func (c *Cache) Model() *Model {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.model
}

// Tags returns one owner's tags. A tag naming somebody no longer in the directory
// is skipped rather than fatal, since people leave and their rows go with them.
func (c *Cache) Tags(owner string) map[string][]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	tags := map[string][]string{}
	for _, row := range c.tables.Tags {
		if !strings.EqualFold(row[tagOwner], owner) {
			continue
		}
		person := strings.ToLower(row[tagPerson])
		if c.model.Person(person) == nil {
			continue
		}
		tags[row[tagName]] = append(tags[row[tagName]], person)
	}
	for _, people := range tags {
		sort.Strings(people)
	}
	return tags
}

func (c *Cache) tagged(owner, tag, person string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, row := range c.tables.Tags {
		if strings.EqualFold(row[tagOwner], owner) && row[tagName] == tag && strings.EqualFold(row[tagPerson], person) {
			return true
		}
	}
	return false
}

func (c *Cache) applyTag(owner, tag, person string, on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rows := []map[string]string{}
	for _, row := range c.tables.Tags {
		if strings.EqualFold(row[tagOwner], owner) && row[tagName] == tag && strings.EqualFold(row[tagPerson], person) {
			continue
		}
		rows = append(rows, row)
	}
	if on {
		rows = append(rows, map[string]string{tagOwner: owner, tagName: tag, tagPerson: person})
	}
	next := *c.tables
	next.Tags = rows
	c.tables = &next
}

func (c *Cache) currentTables() *Tables {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tables
}

func (c *Cache) refreshLoop() {
	for range time.Tick(refreshInterval) {
		c.queue.Add(func() {
			if err := c.refresh(); err != nil {
				log.Printf("[ERROR] directory model refresh: %v", err)
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
	return c.rebuild(tables, start)
}

func (c *Cache) rebuild(tables *Tables, start time.Time) error {
	model, err := BuildModel(tables, c.blobs, c.static)
	if err != nil {
		return err
	}
	c.geocodeFamilies(model)
	c.mu.Lock()
	model.StaleYears = c.settings.StaleYears
	model.PrivacyLinks = c.settings.PrivacyLinks
	model.StaffColor = c.settings.StaffColor
	for i := range model.Classrooms {
		model.Classrooms[i].Color = c.settings.ClassroomColors[model.Classrooms[i].Name]
	}
	for i := range model.Grades {
		model.Grades[i].Color = c.settings.GradeColors[model.Grades[i].Name]
	}
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
