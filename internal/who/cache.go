package who

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/admins"
	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/data"
	"heliosian/internal/geocode"
	"heliosian/internal/store"
)

type Cache struct {
	*store.Store[*Model]
	admins.List
	unlocated chan struct{}
}

var Tabs = []store.Tab{
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
	admins.Spec,
	{Name: geocodeTable, Columns: geocodeColumns, Key: []string{geocodeAddress}, AppendOnly: true},
}

func modelTabs() []store.Tab {
	return append(slices.Clone(Tabs),
		store.Tab{App: preferencesApp, Name: preferencesTab, Columns: []string{preferenceTimestamp, preferenceEmail, preferenceStatus, preferencePermission}, Key: []string{preferenceTimestamp, preferenceEmail}},
	)
}

func NewBook(source data.Source, writer data.Writer, queue *store.Queue) (*store.Book, error) {
	return store.NewBook(appName, modelTabs(), source, writer, queue)
}

func spec(blobs, static blob.Checker, idKey []byte, loaded func()) store.Spec[*Model] {
	return store.Spec[*Model]{
		App:  appName,
		Tabs: modelTabs(),
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

func carryPerson(_ store.Tables, before, after store.Row) []store.Op {
	if before == nil {
		return nil
	}
	if added, _ := cells.YesNo(before["Added"], false); !added {
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

func Open(source data.Source, writer data.Writer, blobs, static blob.Checker, idKey []byte, queue *store.Queue) (*store.Store[*Model], error) {
	return store.New(spec(blobs, static, idKey, func() {}), source, writer, queue)
}

func LoadModel(source data.Source, blobs, static blob.Checker, idKey []byte) (*Model, error) {
	s, err := Open(source, nil, blobs, static, idKey, store.NewQueue())
	if err != nil {
		return nil, err
	}
	return s.Model(), nil
}

func NewCache(source data.Source, writer data.Writer, blobs, static blob.Checker, queue *store.Queue, idKey []byte, superAdmins func() []string) (*Cache, error) {
	c := &Cache{unlocated: make(chan struct{}, 1)}
	s, err := store.New(spec(blobs, static, idKey, c.locate), source, writer, queue)
	if err != nil {
		return nil, err
	}
	c.Store = s
	c.List = admins.New("who", AdminAllowances, superAdmins, func() []string { return s.Model().admins }, s.Commit)
	return c, nil
}

func (c *Cache) Actor(r *http.Request, held func(email string) []access.Allowance) access.Actor {
	return c.Model().Actor(r, held)
}

func (c *Cache) commit(ctx context.Context, actor access.Actor, ops ...store.Op) error {
	if err := c.Commit(ctx, actor, ops...); err != nil {
		return err
	}
	c.locate()
	return nil
}

func (c *Cache) locate() {
	select {
	case c.unlocated <- struct{}{}:
	default:
	}
}

func (c *Cache) Locate(geocoder *geocode.Client) {
	for range c.unlocated {
		c.geocode(geocoder)
	}
}

func (c *Cache) geocode(geocoder *geocode.Client) {
	model := c.Model()
	missing := model.unlocated
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
					slog.Error("geocode", "error", err)
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
	actor := access.System("geocoder")
	ops := model.locate(actor, found)
	if err := c.Commit(context.Background(), actor, ops...); err != nil {
		slog.Error("record geocoded addresses", "error", err)
	}
	slog.Info("geocoded family addresses", "looked up", len(missing), "found", len(ops), "took", time.Since(start).Round(time.Millisecond))
}
