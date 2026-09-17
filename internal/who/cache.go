package who

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sort"
	"strconv"
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
	writer   data.Writer
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
}

// store is the concrete blob store (nil in sample mode), needed to replace a classroom
// or grade image in place, which needs more than the existence check BlobChecker exposes.
func NewCache(source data.Source, writer data.Writer, geocoder Geocoder, blobs, static BlobChecker, store *blob.Store, queue *Queue, superAdmins func() []string) (*Cache, error) {
	c := &Cache{
		source: source, writer: writer, geocoder: geocoder, blobs: blobs, static: static, store: store, queue: queue,
		superAdmins: superAdmins, superEdit: map[string]bool{},
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
	return TagsOf(c.tables.Tags, c.model, owner)
}

// TagsOf is Tags over the Tags tab's rows and a model, for a tool holding
// both without a cache.
func TagsOf(rows []map[string]string, model *Model, owner string) map[string][]string {
	tags := map[string][]string{}
	for _, row := range rows {
		if !strings.EqualFold(row[tagOwner], owner) {
			continue
		}
		person := strings.ToLower(row[tagPerson])
		if model.Person(person) == nil {
			continue
		}
		tags[row[tagName]] = append(tags[row[tagName]], person)
	}
	for _, people := range tags {
		sort.Strings(people)
	}
	return tags
}

// A SharedTag is someone else's tag this person may manage, as the model
// hands it to the page: whose it is, its people, and everyone managing it.
type SharedTag struct {
	Owner     string   `json:"owner"`
	OwnerName string   `json:"ownerName"`
	Name      string   `json:"name"`
	People    []string `json:"people"`
	Managers  []string `json:"managers"`
}

// TagManagers returns, for each of an owner's tags that has any, who else
// manages it. A manager no longer in the directory is skipped, as Tags skips
// a tagged person who has left.
func (c *Cache) TagManagers(owner string) map[string][]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := map[string][]string{}
	for _, row := range c.tables.Managers {
		if !strings.EqualFold(row[tagOwner], owner) {
			continue
		}
		manager := strings.ToLower(row[managerEmail])
		if c.model.Person(manager) == nil {
			continue
		}
		out[row[tagName]] = append(out[row[tagName]], manager)
	}
	for _, managers := range out {
		sort.Strings(managers)
	}
	return out
}

// SharedTags returns the tags other people have let this person manage,
// owner by owner, each with its people and its managers - a tag whose owner
// has left the directory, or that has no rows left, is skipped.
func (c *Cache) SharedTags(email string) []SharedTag {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return SharedTagsOf(c.tables.Tags, c.tables.Managers, c.model, email)
}

// SharedTagsOf is SharedTags over the Tags and Tag Managers tabs' rows and a
// model, for a tool holding them without a cache.
func SharedTagsOf(tagRows, managerRows []map[string]string, model *Model, email string) []SharedTag {
	out := []SharedTag{}
	for _, row := range managerRows {
		if !strings.EqualFold(row[managerEmail], email) {
			continue
		}
		owner := strings.ToLower(row[tagOwner])
		if model.Person(owner) == nil {
			continue
		}
		tag := row[tagName]
		shared := SharedTag{Owner: owner, OwnerName: model.DisplayName(owner), Name: tag, People: []string{}, Managers: []string{}}
		for _, t := range tagRows {
			if strings.EqualFold(t[tagOwner], owner) && t[tagName] == tag {
				person := strings.ToLower(t[tagPerson])
				if model.Person(person) != nil {
					shared.People = append(shared.People, person)
				}
			}
		}
		if len(shared.People) == 0 {
			continue
		}
		for _, m := range managerRows {
			if strings.EqualFold(m[tagOwner], owner) && m[tagName] == tag {
				manager := strings.ToLower(m[managerEmail])
				if model.Person(manager) != nil {
					shared.Managers = append(shared.Managers, manager)
				}
			}
		}
		sort.Strings(shared.People)
		sort.Strings(shared.Managers)
		out = append(out, shared)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Owner < out[j].Owner
	})
	return out
}

// canManage says whether email may change one of owner's tags: the owner
// always, and anyone the owner has made a manager of that tag.
func (c *Cache) canManage(email, owner, tag string) bool {
	if strings.EqualFold(email, owner) {
		return true
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, row := range c.tables.Managers {
		if strings.EqualFold(row[tagOwner], owner) && row[tagName] == tag && strings.EqualFold(row[managerEmail], email) {
			return true
		}
	}
	return false
}

func (c *Cache) applyManager(owner, tag, manager string, on bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rows := []map[string]string{}
	for _, row := range c.tables.Managers {
		if strings.EqualFold(row[tagOwner], owner) && row[tagName] == tag && strings.EqualFold(row[managerEmail], manager) {
			continue
		}
		rows = append(rows, row)
	}
	if on {
		rows = append(rows, map[string]string{tagOwner: owner, tagName: tag, managerEmail: manager})
	}
	next := *c.tables
	next.Managers = rows
	c.tables = &next
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

// renameTag gives one owner's tag a new name across its rows and its
// managers', returning how many people it has - zero when there was no such
// tag to rename.
func (c *Cache) renameTag(owner, from, to string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	rename := func(rows []map[string]string) ([]map[string]string, int) {
		next := make([]map[string]string, len(rows))
		n := 0
		for i, row := range rows {
			if strings.EqualFold(row[tagOwner], owner) && row[tagName] == from {
				clone := maps.Clone(row)
				clone[tagName] = to
				next[i] = clone
				n++
			} else {
				next[i] = row
			}
		}
		return next, n
	}
	tags, people := rename(c.tables.Tags)
	if people == 0 {
		return 0
	}
	managers, _ := rename(c.tables.Managers)
	next := *c.tables
	next.Tags = tags
	next.Managers = managers
	c.tables = &next
	return people
}

// copyTag gives someone a new tag of their own with the same people as a
// tag of fromOwner's - their own, or one shared with them - returning the
// rows it added. The copy is theirs alone: the original's managers aren't
// carried over.
func (c *Cache) copyTag(fromOwner, from, owner, to string) [][]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	rows := slices.Clone(c.tables.Tags)
	added := [][]string{}
	for _, row := range c.tables.Tags {
		if strings.EqualFold(row[tagOwner], fromOwner) && row[tagName] == from {
			rows = append(rows, map[string]string{tagOwner: owner, tagName: to, tagPerson: row[tagPerson]})
			added = append(added, []string{owner, to, row[tagPerson]})
		}
	}
	if len(added) == 0 {
		return nil
	}
	next := *c.tables
	next.Tags = rows
	c.tables = &next
	return added
}

// dropTag forgets every row of one owner's tag, and whoever managed it,
// returning how many people it had - zero means the tag wasn't theirs to
// begin with (or was already gone).
func (c *Cache) dropTag(owner, tag string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	rows := []map[string]string{}
	dropped := 0
	for _, row := range c.tables.Tags {
		if strings.EqualFold(row[tagOwner], owner) && row[tagName] == tag {
			dropped++
			continue
		}
		rows = append(rows, row)
	}
	if dropped == 0 {
		return 0
	}
	managers := []map[string]string{}
	for _, row := range c.tables.Managers {
		if strings.EqualFold(row[tagOwner], owner) && row[tagName] == tag {
			continue
		}
		managers = append(managers, row)
	}
	next := *c.tables
	next.Tags = rows
	next.Managers = managers
	c.tables = &next
	return dropped
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
	if err := c.geocodeFamilies(model, tables); err != nil {
		return err
	}
	c.mu.Lock()
	c.model = model
	c.tables = tables
	c.mu.Unlock()
	slog.Info("loaded directory model", "people", len(model.People), "families", len(model.Families),
		"classrooms", len(model.Classrooms), "crews", len(model.Crews), "took", time.Since(start).Round(time.Millisecond))
	return nil
}

func (c *Cache) geocodeFamilies(model *Model, tables *Tables) error {
	start := time.Now()
	known := map[string]geocode.Point{}
	for _, row := range tables.Geocode {
		address := row[geocodeAddress]
		if address == "" {
			return fmt.Errorf("geocode row %v has no address", row)
		}
		lat, err := strconv.ParseFloat(row[geocodeLat], 64)
		if err != nil {
			return fmt.Errorf("geocode row %q has invalid %s %q", address, geocodeLat, row[geocodeLat])
		}
		lng, err := strconv.ParseFloat(row[geocodeLng], 64)
		if err != nil {
			return fmt.Errorf("geocode row %q has invalid %s %q", address, geocodeLng, row[geocodeLng])
		}
		known[address] = geocode.Point{Lat: lat, Lng: lng}
	}

	missing := []string{}
	for _, family := range model.Families {
		if family.Address == "" {
			continue
		}
		if _, ok := known[family.Address]; ok {
			continue
		}
		if !slices.Contains(missing, family.Address) {
			missing = append(missing, family.Address)
		}
	}
	sort.Strings(missing)

	jobs := make(chan string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	found := map[string]geocode.Point{}
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for address := range jobs {
				point, err := c.geocoder.Lookup(address)
				if err != nil {
					slog.Error("geocode", "error", err)
					continue
				}
				mu.Lock()
				found[address] = point
				mu.Unlock()
			}
		}()
	}
	for _, address := range missing {
		jobs <- address
	}
	close(jobs)
	wg.Wait()

	rows := [][]string{}
	folded := make([]map[string]string, 0, len(tables.Geocode)+len(found))
	folded = append(folded, tables.Geocode...)
	for _, address := range missing {
		point, ok := found[address]
		if !ok {
			continue
		}
		known[address] = point
		lat, lng := strconv.FormatFloat(point.Lat, 'f', -1, 64), strconv.FormatFloat(point.Lng, 'f', -1, 64)
		rows = append(rows, []string{address, lat, lng})
		folded = append(folded, map[string]string{geocodeAddress: address, geocodeLat: lat, geocodeLng: lng})
	}
	if len(rows) > 0 {
		if err := c.writer.AppendAll(appName, geocodeTable, rows); err != nil {
			slog.Error("[ERROR] geocode cache write", "error", err)
		} else {
			tables.Geocode = folded
		}
	}

	located, withAddress := 0, 0
	for key, family := range model.Families {
		if family.Address == "" {
			continue
		}
		withAddress++
		point, ok := known[family.Address]
		if !ok {
			continue
		}
		family.Lat = point.Lat
		family.Lng = point.Lng
		model.Families[key] = family
		located++
	}
	slog.Info("geocoded family addresses", "located", located, "of", withAddress, "cached", len(known)-len(rows),
		"looked up", len(missing), "took", time.Since(start).Round(time.Millisecond))
	return nil
}
