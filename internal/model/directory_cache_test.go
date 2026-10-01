package model

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/api"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/geocode"
	"heliosian/internal/id"
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
	media := blob.New(blob.NewMemoryBucket())
	mux := http.NewServeMux()
	RegisterDirectoryMedia(mux, media)
	typedRegistry(s, queue, DirectoryResources(s), WhoResources(s, ""), []api.Type[*Model]{adminListResources(s)}).Register(mux)
	return server{dir: dir, queue: queue, store: s, mux: mux}
}

func (s server) directory() *Directory {
	return s.store.Model().Directory
}

func (s server) personID(email string) string {
	return s.directory().Person(email).ID
}

func (s server) record(email string) string {
	return derived(s.store.Model(), kindPersonRecord, email)
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

func (s server) made(t *testing.T, as, path, body string) string {
	t.Helper()
	rec := s.call(t, as, http.MethodPost, path, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", path, rec.Code, rec.Body)
	}
	return created(t, rec)
}

func (s server) person(email string) string {
	return `{"person":"` + s.personID(email) + `"}`
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

func tagNamed(tags []Tag, name string) (Tag, bool) {
	i := slices.IndexFunc(tags, func(t Tag) bool { return t.Name == name })
	if i < 0 {
		return Tag{}, false
	}
	return tags[i], true
}

func TestTagChangesReachMemoryTheSheetAndTheLog(t *testing.T) {
	s := newServer(t)
	s.ok(t, jordan, "POST", "/api/tags/"+strings.ToUpper(carpool)+"/remove", s.person("daniel.park@heliosschool.org"))
	s.ok(t, jordan, "POST", "/api/tags/"+soccerTeam+"/rename", `{"name":"Football"}`)
	kicks := s.made(t, jordan, "/api/tags/"+soccerTeam+"/copy", `{"name":"Kicks"}`)
	if parsed, ok := id.Parse(kicks); !ok || parsed != kicks || kicks == soccerTeam {
		t.Fatalf("minted tag id %q", kicks)
	}
	s.ok(t, jordan, "POST", "/api/tags/"+kicks+"/share", s.person(abena))
	if shared := s.directory().SharedTags(abena); len(shared) != 1 || shared[0].ID != kicks || shared[0].Name != "Kicks" || len(shared[0].People) != 3 {
		t.Fatalf("shared tags of %s: %+v", abena, shared)
	}
	s.ok(t, abena, "POST", "/api/tags/"+kicks+"/leave", "")
	s.ok(t, jordan, "DELETE", "/api/tags/"+kicks, "")
	if rec := s.call(t, jordan, "DELETE", "/api/tags/"+kicks, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("deleting a deleted tag: %d %s", rec.Code, rec.Body)
	}
	s.ok(t, jordan, "POST", "/api/admin-lists/who/edit", `{"admins":["`+abena+`"]}`)

	tags := s.directory().Tags(jordan)
	names := []string{}
	for _, tag := range tags {
		names = append(names, fmt.Sprintf("%s %s %d", tag.ID, tag.Name, len(tag.People)))
	}
	if want := []string{carpool + " Carpool 1", soccerTeam + " Football 3"}; !slices.Equal(names, want) {
		t.Fatalf("tags in memory: %v, want %v", names, want)
	}
	if s.count(t, tagListTable, store.Row{tagID: soccerTeam, tagOwner: jordan, tagName: "Football"}) != 1 || s.count(t, tagsTable, store.Row{tagID: soccerTeam}) != 3 {
		t.Fatalf("the renamed tag in the sheet: %v, %v", s.rows(t, tagListTable), s.rows(t, tagsTable))
	}
	for _, tab := range []string{tagListTable, tagsTable, managersTable} {
		if n := s.count(t, tab, store.Row{tagID: kicks}); n != 0 {
			t.Errorf("the deleted tag left %d rows in %s", n, tab)
		}
	}
	if s.count(t, managersTable, store.Row{tagID: soccerTeam, managerEmail: asha}) != 1 {
		t.Fatalf("managers in the sheet: %v", s.rows(t, managersTable))
	}
	if !s.store.Model().AdminList("who").IsAdmin(abena) || s.count(t, adminsTabName, store.Row{"Email": abena}) != 1 {
		t.Fatal("the admin list did not take")
	}
	logged(t, s.changeLog(t),
		jordan+"|delete|Tags|Tag ID="+carpool+"; Person Email=daniel.park@heliosschool.org|Person Email|daniel.park@heliosschool.org",
		jordan+"|set|Tag List|Tag ID="+soccerTeam+"|Tag|Soccer Team",
		jordan+"|insert|Tag List|Tag ID="+kicks+"||",
		jordan+"|insert|Tag Managers|Tag ID="+kicks+"; Manager Email="+abena+"||",
		abena+"|delete|Tag Managers|Tag ID="+kicks+"; Manager Email="+abena+"|Manager Email|"+abena,
		jordan+"|delete|Tag List|Tag ID="+kicks+"|Tag|Kicks",
		jordan+"|delete|Tags|Tag ID="+kicks+"; Person Email="+asha+"|Person Email|"+asha,
		jordan+"|insert|Admins|Email="+abena+"||",
	)
	for _, line := range s.changeLog(t) {
		if strings.Contains(line, "|set|Tags|") || strings.Contains(line, "|set|Tag Managers|") {
			t.Errorf("the rename reached past the tag list: %s", line)
		}
	}
}

func TestTagsAreKeptByTheirOwnersAndManagers(t *testing.T) {
	s := newServer(t)
	for _, c := range []struct {
		as, path, body string
		want           int
	}{
		{asha, "/api/tags/" + soccerTeam + "/rename", `{"name":"Mine Now"}`, http.StatusForbidden},
		{asha, "/api/tags/" + soccerTeam + "/delete", "", http.StatusForbidden},
		{asha, "/api/tags/" + soccerTeam + "/share", s.person(abena), http.StatusForbidden},
		{abena, "/api/tags/" + soccerTeam + "/add", s.person(noa), http.StatusNotFound},
		{abena, "/api/tags/" + soccerTeam + "/copy", `{"name":"Kicks"}`, http.StatusNotFound},
		{jordan, "/api/tags/" + soccerTeam + "/rename", `{"name":"carpool"}`, http.StatusConflict},
		{jordan, "/api/tags/" + bookClub + "/copy", `{"name":"Carpool"}`, http.StatusConflict},
		{jordan, "/api/tags/Soccer%20Team/rename", `{"name":"Football"}`, http.StatusNotFound},
		{jordan, "/api/tags/dtg0000000099/add", s.person(noa), http.StatusNotFound},
	} {
		if rec := s.call(t, c.as, http.MethodPost, c.path, c.body); rec.Code != c.want {
			t.Errorf("%s %s %s: %d %s, want %d", c.as, c.path, c.body, rec.Code, rec.Body, c.want)
		}
	}
	s.ok(t, asha, "POST", "/api/tags/"+soccerTeam+"/add", s.person(noa))
	if tag, _ := s.directory().Tag(soccerTeam); !slices.Contains(tag.People, noa) {
		t.Fatalf("a manager's tagging did not take: %+v", tag)
	}
	copied := s.made(t, jordan, "/api/tags/"+bookClub+"/copy", `{"name":"Book Club"}`)
	if tag, ok := s.directory().Tag(copied); !ok || tag.Owner != jordan || tag.Name != "Book Club" || len(tag.People) != 3 {
		t.Fatalf("a manager's copy of the owner's tag: %+v", tag)
	}
}

func TestANewTagNameMintsATagAndAnOldOneReusesIt(t *testing.T) {
	s := newServer(t)
	made := s.made(t, jordan, "/api/tags", `{"name":"Chess","person":"`+s.personID(noa)+`"}`)
	if parsed, ok := id.Parse(made); !ok || parsed != made || made == carpool || made == soccerTeam || made == bookClub {
		t.Fatalf("minted tag id %q", made)
	}
	if again := s.made(t, jordan, "/api/tags", `{"name":"chess","person":"`+s.personID(abena)+`"}`); again != made {
		t.Fatalf("a second tagging by name made %q, want %q", again, made)
	}
	if tag, ok := tagNamed(s.directory().Tags(jordan), "Chess"); !ok || tag.ID != made || !slices.Equal(tag.People, []string{abena, noa}) {
		t.Fatalf("the new tag: %+v", tag)
	}
	if s.count(t, tagListTable, store.Row{tagID: made, tagOwner: jordan, tagName: "Chess"}) != 1 || s.count(t, tagsTable, store.Row{tagID: made}) != 2 {
		t.Fatalf("the new tag in the sheet: %v, %v", s.rows(t, tagListTable), s.rows(t, tagsTable))
	}
}

func TestRemovingATagsLastMemberRemovesTheTag(t *testing.T) {
	s := newServer(t)
	for _, person := range []string{"daniel.park@heliosschool.org", "elena.torres@heliosschool.org", "anders.lindqvist@heliosschool.org"} {
		s.ok(t, abena, "POST", "/api/tags/"+bookClub+"/remove", s.person(person))
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

func seedAddedPerson(t *testing.T, s server) {
	t.Helper()
	err := s.store.Commit(context.Background(), access.System("test"), DirectoryApp,
		store.Insert(tagListTable, store.Row{tagID: band, tagOwner: jordan, tagName: "Band"}),
		store.Insert(tagsTable, store.Row{tagID: band, tagPerson: noa}),
		store.Insert(tagListTable, store.Row{tagID: choir, tagOwner: noa, tagName: "Choir"}),
		store.Insert(tagsTable, store.Row{tagID: choir, tagPerson: jordan}),
		store.Insert(managersTable, store.Row{tagID: choir, managerEmail: abena}),
		store.Insert(managersTable, store.Row{tagID: carpool, managerEmail: noa}),
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
	s.ok(t, jordan, "POST", "/api/person-records/"+s.record(noa)+"/edit", `{"email":"`+renamed+`","fullName":"Noa Adler","isStudent":false,"isParent":false,"isStaff":true}`)
	if s.directory().Person(noa) != nil || s.directory().Person(renamed) == nil {
		t.Fatal("the rename did not reach memory")
	}
	bandTag, _ := s.directory().Tag(band)
	choirTag, _ := s.directory().Tag(choir)
	carpoolTag, _ := s.directory().Tag(carpool)
	if !slices.Equal(bandTag.People, []string{renamed}) || choirTag.Owner != renamed || !slices.Contains(carpoolTag.Managers, renamed) || len(s.directory().Person(renamed).Photos) != 1 {
		t.Fatalf("the rename stranded tags or photos in memory: %+v %+v %+v", bandTag, choirTag, carpoolTag)
	}
	for _, tab := range []string{tagListTable, tagsTable, managersTable, photosTab} {
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
		jordan+"|set|Tags|Tag ID="+band+"; Person Email="+renamed+"|Person Email|"+noa,
		jordan+"|set|Tag List|Tag ID="+choir+"|Owner Email|"+noa,
		jordan+"|set|Tag Managers|Tag ID="+carpool+"; Manager Email="+renamed+"|Manager Email|"+noa,
	)
}

func TestDeletingAnAddedPersonTakesTheirRows(t *testing.T) {
	s := newServer(t)
	seedAddedPerson(t, s)
	s.ok(t, jordan, "DELETE", "/api/person-records/"+s.record(noa), "")
	if s.directory().Person(noa) != nil {
		t.Fatal("the person is still in memory")
	}
	for _, tab := range []string{overridesTab, tagListTable, tagsTable, managersTable, photosTab} {
		for _, row := range s.rows(t, tab) {
			for _, value := range row {
				if strings.EqualFold(value, noa) {
					t.Errorf("%s keeps %v", tab, row)
				}
			}
		}
	}
	for _, key := range []string{band, choir} {
		for _, tab := range []string{tagListTable, tagsTable, managersTable} {
			if n := s.count(t, tab, store.Row{tagID: key}); n != 0 {
				t.Errorf("%s keeps %d rows of tag %s, whose only member or owner was deleted", tab, n, key)
			}
		}
	}
	if _, ok := s.directory().Tag(carpool); !ok {
		t.Error("a tag the deleted person only managed went with them")
	}
	logged(t, s.changeLog(t), jordan+"|delete|Photos|Email="+noa+"; Photo Name=noa.jpg|Photo Name|noa.jpg")
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

func TestPhotoOrderIsSortKeys(t *testing.T) {
	s := newServer(t)
	const elena = "elena.torres@heliosschool.org"
	after := []photoRef{{Name: "a.jpg"}, {Name: "b.jpg"}, {Name: "c.jpg"}}
	if err := s.store.Commit(context.Background(), access.Actor{Email: elena}, DirectoryApp, photoOps(elena, nil, after)...); err != nil {
		t.Fatal(err)
	}
	names := func() []string {
		out := []string{}
		for _, p := range s.directory().Person(elena).Photos {
			out = append(out, p.Name)
		}
		return out
	}
	if got := names(); !slices.Equal(got, []string{"a.jpg", "b.jpg", "c.jpg"}) {
		t.Fatalf("photos %v", got)
	}
	s.ok(t, elena, "POST", "/api/people/"+s.personID(elena)+"/order-photos", `{"names":["c.jpg","a.jpg"]}`)
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

func (s server) upload(t *testing.T, as, kind, file string, content []byte) string {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	form.WriteField("kind", kind)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+file+`"`)
	header.Set("Content-Type", mime.TypeByExtension(filepath.Ext(file)))
	part, err := form.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(content)
	form.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/directory/media", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	auth.Fixed(as, s.mux).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}
	var stored storedMedia
	if err := json.NewDecoder(rec.Body).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	return stored.Name
}

func TestClassroomImageIsACommit(t *testing.T) {
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	s := newServer(t)
	name := s.upload(t, jordan, "photo", "condors.png", picture.Bytes())
	if want := fmt.Sprintf("%x.png", sha256.Sum256(picture.Bytes())); name != want {
		t.Fatalf("stored as %q, want %q", name, want)
	}
	i := slices.IndexFunc(s.directory().Classrooms, func(c Classroom) bool { return c.Name == "Condors" })
	condors := s.directory().Classrooms[i].ID
	if rec := s.call(t, asha, "POST", "/api/classrooms/"+condors+"/image", `{"name":"`+name+`"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent replacing a classroom image: %d %s", rec.Code, rec.Body)
	}
	s.ok(t, jordan, "POST", "/api/classrooms/"+condors+"/image", `{"name":"`+name+`"}`)
	if got := s.directory().Classrooms[i].ImageURL; got != "/photos/"+name {
		t.Fatalf("classroom image %q, want the uploaded object", got)
	}
	if s.count(t, imagesTab, store.Row{imageKind: "classroom", imageName: "Condors", imageImage: name}) != 1 {
		t.Fatalf("images tab %v", s.rows(t, imagesTab))
	}
	logged(t, s.changeLog(t), jordan+"|insert|Images|Kind=classroom; Name=Condors||")
	if rec := s.call(t, jordan, "POST", "/api/classrooms/"+condors+"/image", `{"name":"https://example.org/x.png"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("an image that was never uploaded: %d %s", rec.Code, rec.Body)
	}
}

func TestGreetingsAreTheirCreatorsToChange(t *testing.T) {
	s := newServer(t)
	dir := s.dir
	key := s.made(t, asha, "/api/greetings", `{"format":"Dear Enders","grouped":true}`)
	if parsed, ok := id.Parse(key); !ok || parsed != key {
		t.Fatalf("minted greeting id %q", key)
	}
	s.ok(t, asha, "POST", "/api/greetings/"+strings.ToUpper(key)+"/edit", `{"format":"Hello Enders","grouped":true}`)
	for _, c := range []struct {
		as, method, path string
		want             int
	}{
		{jordan, http.MethodPost, "/api/greetings/" + key + "/edit", http.StatusNotFound},
		{asha, http.MethodPost, "/api/greetings/" + builtinGreetings.Kids + "/edit", http.StatusForbidden},
		{asha, http.MethodPost, "/api/greetings/Hello%20Enders/edit", http.StatusNotFound},
		{jordan, http.MethodDelete, "/api/greetings/" + key, http.StatusNotFound},
	} {
		if rec := s.call(t, c.as, c.method, c.path, `{"format":"Hi"}`); rec.Code != c.want {
			t.Fatalf("%s %s %s: %d, want %d", c.as, c.method, c.path, rec.Code, c.want)
		}
	}
	if g, ok := s.store.Model().Invites.greeting(key); !ok || g.Name != "Hello Enders" || g.Format != "Hello Enders" {
		t.Fatalf("the edit did not take: %+v", g)
	}
	s.ok(t, asha, "DELETE", "/api/greetings/"+key, "")
	if _, ok := s.store.Model().Invites.greeting(key); ok {
		t.Fatal("the delete did not take")
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
	if len(log) == 0 || log[0]["Action"] != "insert" || log[0]["Key"] != "Greeting ID="+key {
		t.Fatalf("change log %v", log)
	}
}
