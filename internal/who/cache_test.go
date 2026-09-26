package who

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/geocode"
	"heliosian/internal/store"
)

type allBlobs struct{}

func (allBlobs) Has(string) (bool, error) { return true, nil }

func (allBlobs) Prefetch(context.Context, []string) error { return nil }

type countingGeocoder struct {
	mu    sync.Mutex
	calls int
}

func (g *countingGeocoder) Lookup(address string) (geocode.Point, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	return geocode.Fake{}.Lookup(address)
}

type server struct {
	dir   *data.Dir
	queue *store.Queue
	cache *Cache
	mux   *http.ServeMux
}

func newServer(t *testing.T) server {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	queue := store.NewQueue()
	cache, err := NewCache(dir, dir, allBlobs{}, noBlobs{}, queue, testKey, func() []string { return []string{jordan} })
	if err != nil {
		t.Fatal(err)
	}
	media := blob.NewMemory()
	mux := http.NewServeMux()
	RegisterTags(mux, cache)
	RegisterAdmin(mux, cache, media)
	RegisterUpload(mux, cache, media)
	return server{dir: dir, queue: queue, cache: cache, mux: mux}
}

func (s server) post(t *testing.T, as, path, contentType string, body []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	auth.Fixed(as, s.mux).ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("%s: %d %s", path, rec.Code, rec.Body)
	}
}

func (s server) form(t *testing.T, as, path string, values url.Values) {
	t.Helper()
	s.post(t, as, path, "application/x-www-form-urlencoded", []byte(values.Encode()))
}

func (s server) rows(t *testing.T, tab string) []map[string]string {
	t.Helper()
	s.queue.Flush()
	_, rows, err := s.dir.Table(appName, tab)
	if err != nil {
		t.Fatal(err)
	}
	return rows
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
	out := []string{}
	for _, row := range s.rows(t, store.ChangeLogTab) {
		out = append(out, row["Actor"]+"|"+row["Action"]+"|"+row["Tab"]+"|"+row["Key"]+"|"+row["Column"]+"|"+row["Previous"])
	}
	return out
}

func logged(t *testing.T, log []string, want ...string) {
	t.Helper()
	for _, line := range want {
		if !slices.Contains(log, line) {
			t.Errorf("the change log lacks %q:\n%s", line, strings.Join(log, "\n"))
		}
	}
}

const (
	abena = "abena.osei@heliosschool.org"
	noa   = "noa.adler@heliosschool.org"
)

func TestTagChangesReachMemoryTheSheetAndTheLog(t *testing.T) {
	s := newServer(t)
	s.form(t, jordan, "/api/directory/tag", url.Values{"tag": {"Carpool"}, "person": {"daniel.park@heliosschool.org"}, "on": {"0"}})
	s.form(t, jordan, "/api/directory/tag-rename", url.Values{"tag": {"Soccer Team"}, "name": {"Football"}})
	s.form(t, jordan, "/api/directory/tag-copy", url.Values{"tag": {"Football"}, "name": {"Kicks"}})
	s.form(t, jordan, "/api/directory/tag-share", url.Values{"tag": {"Kicks"}, "manager": {abena}, "on": {"1"}})
	if len(s.cache.SharedTags(abena)) != 1 {
		t.Fatalf("shared tags of %s: %+v", abena, s.cache.SharedTags(abena))
	}
	s.form(t, abena, "/api/directory/tag-leave", url.Values{"tag": {"Kicks"}, "owner": {jordan}})
	s.form(t, jordan, "/api/directory/tag-delete", url.Values{"tag": {"Kicks"}})
	s.form(t, jordan, "/api/directory/tag-delete", url.Values{"tag": {"Nothing Here"}})
	s.post(t, jordan, "/api/admin/admins", "application/json", []byte(`{"admins":["`+abena+`"]}`))

	tags := s.cache.Tags(jordan)
	if len(tags["Carpool"]) != 1 || len(tags["Football"]) != 3 || len(tags["Kicks"]) != 0 || len(tags["Soccer Team"]) != 0 {
		t.Fatalf("tags in memory: %v", tags)
	}
	if s.count(t, tagsTable, store.Row{tagOwner: jordan, tagName: "Football"}) != 3 || s.count(t, tagsTable, store.Row{tagName: "Kicks"}) != 0 {
		t.Fatalf("tags in the sheet: %v", s.rows(t, tagsTable))
	}
	if s.count(t, managersTable, store.Row{tagOwner: jordan, tagName: "Football", managerEmail: "asha.chandra@heliosschool.org"}) != 1 || s.count(t, managersTable, store.Row{tagName: "Kicks"}) != 0 {
		t.Fatalf("managers in the sheet: %v", s.rows(t, managersTable))
	}
	if !s.cache.IsAdmin(abena) || s.count(t, adminsTable, store.Row{"Email": abena}) != 1 {
		t.Fatal("the admin list did not take")
	}
	logged(t, s.changeLog(t),
		jordan+"|delete|Tags|Owner Email="+jordan+"; Tag=Carpool; Person Email=daniel.park@heliosschool.org|Person Email|daniel.park@heliosschool.org",
		jordan+"|set|Tag Managers|Owner Email="+jordan+"; Tag=Football; Manager Email=asha.chandra@heliosschool.org|Tag|Soccer Team",
		jordan+"|insert|Tag Managers|Owner Email="+jordan+"; Tag=Kicks; Manager Email="+abena+"||",
		abena+"|delete|Tag Managers|Owner Email="+jordan+"; Tag=Kicks; Manager Email="+abena+"|Manager Email|"+abena,
		jordan+"|insert|Admins|Email="+abena+"||",
	)
}

func seedAddedPerson(t *testing.T, s server) {
	t.Helper()
	err := s.cache.Commit(context.Background(), "test",
		store.Insert(tagsTable, store.Row{tagOwner: jordan, tagName: "Band", tagPerson: noa}),
		store.Insert(tagsTable, store.Row{tagOwner: noa, tagName: "Choir", tagPerson: jordan}),
		store.Insert(managersTable, store.Row{tagOwner: noa, tagName: "Choir", managerEmail: abena}),
		store.Insert(photosTab, store.Row{"Email": noa, "Photo Name": "noa.jpg", store.OrderColumn: "i"}),
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRenamingAnAddedPersonCarriesTheirRows(t *testing.T) {
	s := newServer(t)
	seedAddedPerson(t, s)
	const renamed = "noa.a@heliosschool.org"
	s.post(t, jordan, "/api/admin/added-fields", "application/json", []byte(`{"email":"`+noa+`","newEmail":"`+renamed+`","fullName":"Noa Adler","isStaff":true}`))
	if s.cache.Model().Person(noa) != nil || s.cache.Model().Person(renamed) == nil {
		t.Fatal("the rename did not reach memory")
	}
	if len(s.cache.Tags(jordan)["Band"]) != 1 || len(s.cache.Tags(renamed)["Choir"]) != 1 || len(s.cache.Model().Person(renamed).Photos) != 1 {
		t.Fatal("the rename stranded tags or photos in memory")
	}
	for _, tab := range []string{tagsTable, managersTable, photosTab} {
		for _, row := range s.rows(t, tab) {
			for column, value := range row {
				if strings.EqualFold(value, noa) {
					t.Errorf("%s keeps %s under %s: %v", tab, noa, column, row)
				}
			}
		}
	}
	logged(t, s.changeLog(t),
		jordan+"|set|Overrides|Email="+renamed+"|Email|"+noa,
		jordan+"|set|Photos|Email="+renamed+"; Photo Name=noa.jpg|Email|"+noa,
		jordan+"|set|Tags|Owner Email="+jordan+"; Tag=Band; Person Email="+renamed+"|Person Email|"+noa,
	)
}

func TestDeletingAnAddedPersonTakesTheirRows(t *testing.T) {
	s := newServer(t)
	seedAddedPerson(t, s)
	s.post(t, jordan, "/api/admin/delete-person", "application/json", []byte(`{"email":"`+noa+`"}`))
	if s.cache.Model().Person(noa) != nil {
		t.Fatal("the person is still in memory")
	}
	for _, tab := range []string{overridesTab, tagsTable, managersTable, photosTab} {
		for _, row := range s.rows(t, tab) {
			for _, value := range row {
				if strings.EqualFold(value, noa) {
					t.Errorf("%s keeps %v", tab, row)
				}
			}
		}
	}
	logged(t, s.changeLog(t), jordan+"|delete|Photos|Email="+noa+"; Photo Name=noa.jpg|Photo Name|noa.jpg")
}

func TestDeletingAnImportedPersonsOverridesKeepsTheirRows(t *testing.T) {
	s := newServer(t)
	if err := s.cache.Commit(context.Background(), "test", store.Delete(overridesTab, store.Row{"Email": jordan})); err != nil {
		t.Fatal(err)
	}
	if s.count(t, tagsTable, store.Row{tagOwner: jordan}) == 0 {
		t.Fatal("an imported person's tags went with their overrides row")
	}
}

func TestPhotoOrderIsSortKeys(t *testing.T) {
	s := newServer(t)
	const elena = "elena.torres@heliosschool.org"
	after := []photoRef{{Name: "a.jpg"}, {Name: "b.jpg"}, {Name: "c.jpg"}}
	if err := s.cache.Commit(context.Background(), elena, photoOps(elena, nil, after)...); err != nil {
		t.Fatal(err)
	}
	names := func() []string {
		out := []string{}
		for _, p := range s.cache.Model().Person(elena).Photos {
			out = append(out, p.Name)
		}
		return out
	}
	if got := names(); !slices.Equal(got, []string{"a.jpg", "b.jpg", "c.jpg"}) {
		t.Fatalf("photos %v", got)
	}
	s.form(t, elena, "/api/directory/reorder-photos", url.Values{"key": {elena}, "order": {"c.jpg,a.jpg"}})
	if got := names(); !slices.Equal(got, []string{"c.jpg", "a.jpg"}) {
		t.Fatalf("photos after the reorder %v", got)
	}
	orders := map[string]string{}
	for _, row := range s.rows(t, photosTab) {
		orders[row["Photo Name"]] = row[store.OrderColumn]
	}
	if len(orders) != 2 || store.CompareKeys(orders["c.jpg"], orders["a.jpg"]) >= 0 {
		t.Fatalf("sheet orders %v", orders)
	}
	logged(t, s.changeLog(t),
		elena+"|delete|Photos|Email="+elena+"; Photo Name=b.jpg|Photo Name|b.jpg",
		elena+"|set|Photos|Email="+elena+"; Photo Name=a.jpg|Order|9",
	)
}

func TestGeocoderRecordsWhatItFinds(t *testing.T) {
	s := newServer(t)
	unlocated := len(s.cache.Model().unlocated)
	if unlocated == 0 {
		t.Fatal("the sample has no addresses to locate")
	}
	if len(s.cache.unlocated) != 1 {
		t.Fatal("the load did not wake the geocoder")
	}
	geocoder := &countingGeocoder{}
	s.cache.geocode(geocoder)
	if geocoder.calls != unlocated || len(s.rows(t, geocodeTable)) != unlocated || len(s.cache.Model().unlocated) != 0 {
		t.Fatalf("%d lookups, %d rows, %d still unlocated, of %d", geocoder.calls, len(s.rows(t, geocodeTable)), len(s.cache.Model().unlocated), unlocated)
	}
	located := 0
	for _, family := range s.cache.Model().Families {
		if family.Address != "" && family.Lat != 0 {
			located++
		}
	}
	if located == 0 {
		t.Fatal("no family has coordinates")
	}
	s.cache.geocode(geocoder)
	if geocoder.calls != unlocated {
		t.Fatalf("a second pass looked up %d more", geocoder.calls-unlocated)
	}
	for _, line := range s.changeLog(t) {
		if strings.Contains(line, "|Geocode|") {
			t.Fatalf("the append-only geocode tab was logged: %s", line)
		}
	}
}

func TestClassroomImageIsACommit(t *testing.T) {
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	form.WriteField("kind", "classroom")
	form.WriteField("name", "Condors")
	part, err := form.CreateFormFile("file", "condors.png")
	if err != nil {
		t.Fatal(err)
	}
	part.Write(picture.Bytes())
	form.Close()
	s := newServer(t)
	s.post(t, jordan, "/api/admin/images", form.FormDataContentType(), body.Bytes())
	name := fmt.Sprintf("%x.png", sha256.Sum256(picture.Bytes()))
	for _, c := range s.cache.Model().Classrooms {
		if c.Name == "Condors" && c.ImageURL != "/photos/"+name {
			t.Fatalf("classroom image %q, want the uploaded object", c.ImageURL)
		}
	}
	if s.count(t, imagesTab, store.Row{imageKind: "classroom", imageName: "Condors", imageImage: name}) != 1 {
		t.Fatalf("images tab %v", s.rows(t, imagesTab))
	}
	logged(t, s.changeLog(t), jordan+"|insert|Images|Kind=classroom; Name=Condors||")
}

func TestGreetingsAreTheirCreatorsToChange(t *testing.T) {
	dir := &data.Dir{Root: "../../sampledata"}
	s := newServer(t)
	invites, err := NewInvites(dir, dir, s.queue)
	if err != nil {
		t.Fatal(err)
	}
	RegisterInvites(s.mux, s.cache, invites)
	const asha = "asha.chandra@heliosschool.org"
	s.form(t, asha, "/api/directory/greetings", url.Values{"format": {"Dear Enders"}, "grouped": {"1"}})
	s.form(t, asha, "/api/directory/greetings", url.Values{"format": {"Hello Enders"}, "original": {"Dear Enders"}, "grouped": {"1"}})
	for _, c := range []struct {
		as, method, query, body string
	}{
		{jordan, http.MethodPost, "", url.Values{"format": {"Hi"}, "original": {"Hello Enders"}}.Encode()},
		{asha, http.MethodPost, "", url.Values{"format": {"Hi"}, "original": {"Kids only"}}.Encode()},
		{jordan, http.MethodDelete, "?" + url.Values{"name": {"Hello Enders"}}.Encode(), ""},
	} {
		req := httptest.NewRequest(c.method, "/api/directory/greetings"+c.query, strings.NewReader(c.body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		auth.Fixed(c.as, s.mux).ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s %s: %d", c.as, c.method, c.body, rec.Code)
		}
	}
	if _, ok := invites.greeting("Hello Enders"); !ok {
		t.Fatal("the edit did not take")
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/directory/greetings?"+url.Values{"name": {"Hello Enders"}}.Encode(), nil)
	rec := httptest.NewRecorder()
	auth.Fixed(asha, s.mux).ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	s.queue.Flush()
	_, rows, err := dir.Table(invitesApp, greetingsTab)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 7 {
		t.Fatalf("greetings in the sheet: %v", rows)
	}
	_, log, err := dir.Table(invitesApp, store.ChangeLogTab)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) == 0 || log[0]["Action"] != "insert" || log[0]["Key"] != "Name=Dear Enders" {
		t.Fatalf("change log %v", log)
	}
}
