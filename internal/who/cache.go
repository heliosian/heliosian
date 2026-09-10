package who

import (
	"errors"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/config"
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
	// superAdmins reads the platform list out of the Config sheet's own cache.
	superAdmins func() []string

	mu     sync.RWMutex
	model  *Model
	tables *Tables

	// superEdit tracks, per admin, whether they've turned on the switch that lets them
	// edit anyone's record rather than just their own family's. Deliberately
	// in-memory only: it resets on every restart rather than staying on forever.
	superEdit map[string]bool

	// spoof tracks, per admin, which other person they're currently viewing the
	// directory as. View-only by design — it changes what an admin sees, never who a
	// write is attributed to, so it never touches upload.go's identity resolution.
	spoof map[string]string
}

// store is the concrete blob store (nil in sample mode), needed to replace a classroom
// or grade image in place, which needs more than the existence check BlobChecker exposes.
func NewCache(source data.Source, geocoder Geocoder, blobs, static BlobChecker, store *blob.Store, queue *Queue, superAdmins func() []string) (*Cache, error) {
	c := &Cache{
		source: source, geocoder: geocoder, blobs: blobs, static: static, store: store, queue: queue,
		superAdmins: superAdmins, superEdit: map[string]bool{}, spoof: map[string]string{},
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

// applyFamily is applyOverride for the Families tab, keyed by the family key.
func (c *Cache) applyFamily(key string, cells map[string]string) error {
	return c.rebuild(c.currentTables().withFamily(key, cells), time.Now())
}

// applyEmailRename folds an added-only person's email change (plus any other Overrides
// cells changing in the same save) into the cached tables and reruns the model, the
// same "reject before persist" trick as applyOverride - see Tables.withEmailRenamed.
func (c *Cache) applyEmailRename(oldEmail, newEmail string, cells map[string]string) error {
	return c.rebuild(c.currentTables().withEmailRenamed(oldEmail, newEmail, cells), time.Now())
}

// applyDeletePerson folds removing an added-only person's Overrides/Tags/Photos rows
// into the cached tables and reruns the model, the same "reject before persist" trick
// as applyOverride - see Tables.withoutPerson.
func (c *Cache) applyDeletePerson(email string) error {
	return c.rebuild(c.currentTables().withoutPerson(email), time.Now())
}

// applyPhotos folds a Photos-sheet rewrite for one person - an upload, a reorder, a
// delete, or a crop, all just "this person's photo list is now exactly refs" - into
// the cached tables and reruns the model, mirroring applyOverride's trick so a write
// is visible on the very next model build rather than only after the next sheet
// read. cells, if non-nil, folds in an Overrides change (e.g. retiring the legacy
// primary pointer) as part of the same rebuild.
func (c *Cache) applyPhotos(email string, refs []photoRef, cells map[string]string) error {
	tables := c.currentTables().withPhotos(email, refs)
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

// IsAdmin reports whether email may use the admin tools at all — either tier:
// a row in the Admins tab, or the platform super admin list.
func (c *Cache) IsAdmin(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if c.IsSuperAdmin(email) {
		return true
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, row := range c.tables.Admins {
		if strings.EqualFold(strings.TrimSpace(row["Email"]), email) {
			return true
		}
	}
	return false
}

// tabAdmins is the Admins tab as written, normalized.
func (c *Cache) tabAdmins() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	emails := make([]string, 0, len(c.tables.Admins))
	for _, row := range c.tables.Admins {
		emails = append(emails, row["Email"])
	}
	return config.NormalizeEmails(emails)
}

// Admins is what the Admins tab of the admin page shows: every admin, either
// tier, indistinguishable, sorted together. A regular admin's client never
// learns which names came from which list, because there's only one list here.
func (c *Cache) Admins() []string {
	admins := config.NormalizeEmails(append(c.tabAdmins(), c.superAdmins()...))
	sort.Strings(admins)
	return admins
}

func (c *Cache) applyAdmins(emails []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rows := make([]map[string]string, 0, len(emails))
	for _, email := range emails {
		rows = append(rows, map[string]string{"Email": email})
	}
	next := *c.tables
	next.Admins = rows
	c.tables = &next
}

// IsSuperAdmin reports whether email is on the super admin list specifically — the
// tier that can spoof another user and manage who else is a super admin. Regular
// admins never see anything gated on this, including that it exists.
func (c *Cache) HeroPhoto(email string) string { return c.Model().HeroPhoto(email) }

func (c *Cache) IsSuperAdmin(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	return slices.Contains(c.superAdmins(), email)
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
				slog.Error("directory model refresh", "error", err)
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
	c.model = model
	c.tables = tables
	c.mu.Unlock()
	slog.Info("loaded directory model", "people", len(model.People), "families", len(model.Families),
		"classrooms", len(model.Classrooms), "crews", len(model.Crews), "took", time.Since(start).Round(time.Millisecond))
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
					slog.Error("geocode", "error", err)
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
	slog.Info("geocoded family addresses", "located", located, "of", len(pending), "took", time.Since(start).Round(time.Millisecond))
}
