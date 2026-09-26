package who

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"heliosian/internal/config"
	"heliosian/internal/data"
	"heliosian/internal/geocode"
	"heliosian/internal/store"
)

type Geocoder interface {
	Lookup(address string) (geocode.Point, error)
}

type Cache struct {
	*store.Store[*Model]
	superAdmins func() []string
	unlocated   chan struct{}
}

func spec(blobs, static BlobChecker, idKey []byte, loaded func()) store.Spec[*Model] {
	return store.Spec[*Model]{
		App: appName,
		Tabs: []store.Tab{
			{Name: AliasesTable, Columns: AliasColumns, Key: []string{AliasColumn}},
			{Name: studentsTab, Columns: importColumns, Key: []string{"entry_sort_name"}},
			{Name: staffTab, Columns: staffImportColumns, Key: []string{"entry_sort_name"}},
			{Name: namesTab, Columns: []string{"Name", "Email"}, Key: []string{"Name"}},
			{Name: overridesTab, Columns: overrideColumns, Key: []string{"Email"}, Cascade: carryPerson},
			{Name: familiesTab, Columns: familyColumns, Key: []string{"Email"}},
			{Name: WebsiteTable, Columns: WebsiteColumns, Key: []string{WebsiteID}},
			{Name: tagsTable, Columns: tagColumns, Key: tagColumns},
			{Name: managersTable, Columns: managerColumns, Key: managerColumns},
			{Name: photosTab, Columns: photoColumns, Key: []string{"Email", "Photo Name"}},
			{Name: imagesTab, Columns: imageColumns, Key: []string{imageKind, imageName}},
			{Name: adminsTable, Columns: []string{"Email"}, Key: []string{"Email"}},
			{Name: geocodeTable, Columns: geocodeColumns, Key: []string{geocodeAddress}, AppendOnly: true},
			{App: preferencesApp, Name: preferencesTab, Columns: []string{preferenceTimestamp, preferenceEmail, preferenceStatus, preferencePermission}, Key: []string{preferenceTimestamp, preferenceEmail}},
		},
		Build: func(ctx context.Context, tables store.Tables) (*Model, error) {
			return BuildModel(ctx, tables, blobs, static, idKey)
		},
		Loaded: func(model *Model, took time.Duration) {
			slog.Info("loaded directory model", "people", len(model.People), "families", len(model.Families),
				"classrooms", len(model.Classrooms), "crews", len(model.Crews), "unlocated", len(model.unlocated), "took", took.Round(time.Millisecond))
			loaded()
		},
	}
}

func carryPerson(before, after store.Row) []store.Op {
	if before == nil || before["Added"] != "TRUE" {
		return nil
	}
	was := before["Email"]
	if after == nil {
		return []store.Op{
			store.Delete(tagsTable, store.Row{tagOwner: was}),
			store.Delete(tagsTable, store.Row{tagPerson: was}),
			store.Delete(managersTable, store.Row{tagOwner: was}),
			store.Delete(managersTable, store.Row{managerEmail: was}),
			store.Delete(photosTab, store.Row{"Email": was}),
		}
	}
	now := after["Email"]
	if strings.EqualFold(strings.TrimSpace(now), strings.TrimSpace(was)) {
		return nil
	}
	return []store.Op{
		store.Update(tagsTable, store.Row{tagOwner: was}, store.Row{tagOwner: now}),
		store.Update(tagsTable, store.Row{tagPerson: was}, store.Row{tagPerson: now}),
		store.Update(managersTable, store.Row{tagOwner: was}, store.Row{tagOwner: now}),
		store.Update(managersTable, store.Row{managerEmail: was}, store.Row{managerEmail: now}),
		store.Update(photosTab, store.Row{"Email": was}, store.Row{"Email": now}),
	}
}

func Open(source data.Source, writer data.Writer, blobs, static BlobChecker, idKey []byte, queue *store.Queue) (*store.Store[*Model], error) {
	return store.New(spec(blobs, static, idKey, func() {}), source, writer, queue)
}

func LoadModel(source data.Source, blobs, static BlobChecker, idKey []byte) (*Model, error) {
	s, err := Open(source, nil, blobs, static, idKey, store.NewQueue())
	if err != nil {
		return nil, err
	}
	return s.Model(), nil
}

func NewCache(source data.Source, writer data.Writer, blobs, static BlobChecker, queue *store.Queue, idKey []byte, superAdmins func() []string) (*Cache, error) {
	c := &Cache{superAdmins: superAdmins, unlocated: make(chan struct{}, 1)}
	s, err := store.New(spec(blobs, static, idKey, c.locate), source, writer, queue)
	if err != nil {
		return nil, err
	}
	c.Store = s
	return c, nil
}

func (c *Cache) commit(w http.ResponseWriter, r *http.Request, actor string, ops ...store.Op) bool {
	if err := c.Commit(r.Context(), actor, ops...); err != nil {
		serverError(w, r, err)
		return false
	}
	c.locate()
	return true
}

func (c *Cache) locate() {
	select {
	case c.unlocated <- struct{}{}:
	default:
	}
}

func (c *Cache) Locate(geocoder Geocoder) {
	for range c.unlocated {
		c.geocode(geocoder)
	}
}

func (c *Cache) geocode(geocoder Geocoder) {
	missing := c.Model().unlocated
	if len(missing) == 0 {
		return
	}
	start := time.Now()
	jobs := make(chan string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	found := map[string]geocode.Point{}
	for range 8 {
		wg.Go(func() {
			for address := range jobs {
				point, err := geocoder.Lookup(address)
				if err != nil {
					slog.Error("[ERROR] geocode", "error", err)
					continue
				}
				mu.Lock()
				found[address] = point
				mu.Unlock()
			}
		})
	}
	for _, address := range missing {
		jobs <- address
	}
	close(jobs)
	wg.Wait()
	ops := []store.Op{}
	for _, address := range missing {
		point, ok := found[address]
		if !ok {
			continue
		}
		ops = append(ops, store.Insert(geocodeTable, store.Row{
			geocodeAddress: address,
			geocodeLat:     strconv.FormatFloat(point.Lat, 'f', -1, 64),
			geocodeLng:     strconv.FormatFloat(point.Lng, 'f', -1, 64),
		}))
	}
	if err := c.Commit(context.Background(), "geocoder", ops...); err != nil {
		slog.Error("[ERROR] record geocoded addresses", "error", err)
	}
	slog.Info("geocoded family addresses", "looked up", len(missing), "found", len(ops), "took", time.Since(start).Round(time.Millisecond))
}

func (c *Cache) IsAdmin(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	return c.IsSuperAdmin(email) || slices.Contains(c.Model().admins, email)
}

func (c *Cache) Admins() []string {
	admins := config.NormalizeEmails(append(slices.Clone(c.Model().admins), c.superAdmins()...))
	sort.Strings(admins)
	return admins
}

func (c *Cache) IsSuperAdmin(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	return slices.Contains(c.superAdmins(), email)
}

func (c *Cache) HeroPhoto(email string) string { return c.Model().HeroPhoto(email) }

func (c *Cache) Tags(owner string) map[string][]string { return c.Model().Tags(owner) }

func (c *Cache) TagManagers(owner string) map[string][]string { return c.Model().TagManagers(owner) }

func (c *Cache) SharedTags(email string) []SharedTag { return c.Model().SharedTags(email) }
