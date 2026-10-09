package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/api"
	"heliosian/internal/auth"
	"heliosian/internal/data"
	"heliosian/internal/geocode"
	"heliosian/internal/intercept"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

type countingGeocoder struct {
	mu     sync.Mutex
	calls  int
	answer http.Handler
}

func (g *countingGeocoder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	g.calls++
	g.mu.Unlock()
	g.answer.ServeHTTP(w, r)
}

func (g *countingGeocoder) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

type server struct {
	dir   *data.Dir
	queue *store.Queue
	store *Store
	mux   *http.ServeMux
}

func newServer(t *testing.T) server {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	queue := store.NewQueue()
	s := sampleDirectory(t, dir, queue)
	mux := http.NewServeMux()
	typedRegistry(s, queue, DirectoryResources(), []api.Type[*Model]{adminListResources(s)}).Register(mux)
	return server{dir: dir, queue: queue, store: s, mux: mux}
}

func (s server) directory() *Directory {
	return s.store.Model().Directory
}

func (s server) personID(email string) string {
	return s.directory().Person(email).ID
}

func (s server) call(t *testing.T, as, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	auth.Fixed(as, s.mux).ServeHTTP(rec, req)
	return rec
}

func (s server) ok(t *testing.T, as, method, path, body string) {
	t.Helper()
	if rec := s.call(t, as, method, path, body); rec.Code != http.StatusNoContent {
		t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body)
	}
}

func (s server) rows(t *testing.T, tab string) []map[string]string {
	t.Helper()
	return testkit.Rows(t, s.dir, s.queue, DirectoryApp, tab)
}

func (s server) count(t *testing.T, tab string, match store.Row) int {
	t.Helper()
	n := 0
	for _, row := range s.rows(t, tab) {
		if !slices.ContainsFunc(sortedKeys(match), func(column string) bool { return !strings.EqualFold(row[column], match[column]) }) {
			n++
		}
	}
	return n
}

func sortedKeys(row store.Row) []string {
	keys := []string{}
	for k := range row {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func (s server) changeLog(t *testing.T) []string {
	t.Helper()
	return testkit.ChangeLines(t, s.dir, s.queue, DirectoryApp)
}

func logged(t *testing.T, log []string, want ...string) {
	t.Helper()
	for _, line := range want {
		if !slices.Contains(log, line) {
			t.Errorf("the change log lacks %q:\n%s", line, strings.Join(log, "\n"))
		}
	}
}

func TestTheWhoAdminListIsACommit(t *testing.T) {
	s := newServer(t)
	s.ok(t, jordan, "POST", "/api/admin-lists/who/edit", `{"admins":["`+abena+`"]}`)
	if !s.store.Model().AdminList("who").IsAdmin(abena) || s.count(t, adminsTabName, store.Row{"Email": abena}) != 1 {
		t.Fatal("the admin list did not take")
	}
	logged(t, s.changeLog(t), jordan+"|insert|Admins|Email="+abena+"||")
}

func TestRemovingATagsLastMemberRemovesTheTag(t *testing.T) {
	s := newServer(t)
	for _, person := range []string{"daniel.park@heliosschool.org", "elena.torres@heliosschool.org", "anders.lindqvist@heliosschool.org"} {
		if err := s.store.Commit(context.Background(), access.Actor{Email: abena}, DirectoryApp, store.Delete(tagsTable, store.Row{tagID: bookClub, tagPerson: person})); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := s.directory().Tag(bookClub); ok {
		t.Fatal("the emptied tag is still in memory")
	}
	if shared := s.directory().SharedTags(jordan); len(shared) != 0 {
		t.Fatalf("the emptied tag is still shared: %+v", shared)
	}
	for _, tab := range []string{tagListTable, tagsTable, managersTable} {
		if n := s.count(t, tab, store.Row{tagID: bookClub}); n != 0 {
			t.Errorf("the emptied tag left %d rows in %s", n, tab)
		}
	}
	logged(t, s.changeLog(t),
		abena+"|delete|Tag List|Tag ID="+bookClub+"|Tag|Book Club",
		abena+"|delete|Tag Managers|Tag ID="+bookClub+"; Manager Email="+jordan+"|Manager Email|"+jordan,
	)
}

func TestTagRowsMustHangTogether(t *testing.T) {
	if _, err := BuildDirectory(context.Background(), sampleTables(t), nil, testkit.None, testKey); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]struct {
		tab string
		row store.Row
	}{
		"a bad tag id":          {tagListTable, store.Row{tagID: "Band", tagOwner: jordan, tagName: "Band"}},
		"a repeated tag id":     {tagListTable, store.Row{tagID: carpool, tagOwner: abena, tagName: "Other"}},
		"a repeated name":       {tagListTable, store.Row{tagID: band, tagOwner: jordan, tagName: "CARPOOL"}},
		"a tag with nobody":     {tagListTable, store.Row{tagID: band, tagOwner: jordan, tagName: "Band"}},
		"an unknown member":     {tagsTable, store.Row{tagID: band, tagPerson: noa}},
		"an unknown manager":    {managersTable, store.Row{tagID: band, managerEmail: noa}},
		"a member tagged twice": {tagsTable, store.Row{tagID: carpool, tagPerson: "Abena.Osei@heliosschool.org"}},
	} {
		tb := sampleTables(t)
		tb[bad.tab] = append(slices.Clone(tb[bad.tab]), bad.row)
		if _, err := BuildDirectory(context.Background(), tb, nil, testkit.None, testKey); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
}

func TestDeletingAnImportedPersonsOverridesKeepsTheirRows(t *testing.T) {
	s := newServer(t)
	if err := s.store.Commit(context.Background(), access.System("test"), DirectoryApp, store.Delete(overridesTab, store.Row{"Email": jordan})); err != nil {
		t.Fatal(err)
	}
	if s.count(t, tagListTable, store.Row{tagOwner: jordan}) == 0 {
		t.Fatal("an imported person's tags went with their overrides row")
	}
}

func TestGeocoderRecordsWhatItFinds(t *testing.T) {
	s := newServer(t)
	unlocated := len(s.directory().unlocated)
	if unlocated == 0 {
		t.Fatal("the sample has no addresses to locate")
	}
	if len(s.store.unlocated) != 1 {
		t.Fatal("the load did not wake the geocoder")
	}
	counted := &countingGeocoder{answer: intercept.Geocode()}
	intercept.Install(intercept.GeocodeHost, counted)
	geocoder := geocode.New("test")
	s.store.geocode(geocoder)
	if counted.count() != unlocated || len(s.rows(t, geocodeTable)) != unlocated || len(s.directory().unlocated) != 0 {
		t.Fatalf("%d lookups, %d rows, %d still unlocated, of %d", counted.count(), len(s.rows(t, geocodeTable)), len(s.directory().unlocated), unlocated)
	}
	located := 0
	for _, family := range s.directory().Families {
		if family.Address != "" && family.Lat != 0 {
			located++
		}
	}
	if located == 0 {
		t.Fatal("no family has coordinates")
	}
	s.store.geocode(geocoder)
	if counted.count() != unlocated {
		t.Fatalf("a second pass looked up %d more", counted.count()-unlocated)
	}
	for _, line := range s.changeLog(t) {
		if strings.Contains(line, "|Geocode|") {
			t.Fatalf("the append-only geocode tab was logged: %s", line)
		}
	}
}
