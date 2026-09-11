package events

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"heliosian/internal/auth"
	"heliosian/internal/data"
)

const (
	parent = "robin.whitfield@heliosschool.org"
	admin  = "jordan.whitfield@heliosschool.org"
	chair  = "mina.park@heliosschool.org"
)

type syncQueue struct{}

func (syncQueue) Add(f func()) { f() }

type fakeDirectory struct{}

func (fakeDirectory) Resolve(email string) string { return email }

func (fakeDirectory) People() []DirectoryPerson { return nil }

func (fakeDirectory) Person(email string) (string, string, bool) {
	if email == parent {
		return "Robin Whitfield", "", true
	}
	return "", "", false
}

type bundled struct{}

func (bundled) Has(key string) (bool, error) { return strings.HasPrefix(key, "brand/"), nil }

func (bundled) Prefetch([]string) error { return nil }

func newServer(t *testing.T) (*Cache, *http.ServeMux) {
	t.Helper()
	t.Chdir("../..")
	dir := &data.Dir{Root: "sampledata"}
	cache, err := NewCache(dir, bundled{}, func(e string) bool { return e == admin }, syncQueue{})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, cache, dir, syncQueue{}, nil, fakeDirectory{}, func() []string { return []string{admin} })
	return cache, mux
}

func call(t *testing.T, mux *http.ServeMux, as, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	auth.Fixed(as, mux).ServeHTTP(rec, req)
	return rec
}

// byTitle finds the one activity of that title in a year - tests read better by
// name, and ids are minted at save time so a freshly added row has no known id.
func byTitle(m *Model, year, title string) *Activity {
	for _, root := range m.Activities {
		for _, node := range append([]*Activity{root}, root.Descendants()...) {
			if node.Year == year && node.Title == title {
				return node
			}
		}
	}
	return nil
}

func TestSampleLoads(t *testing.T) {
	cache, _ := newServer(t)
	m := cache.Model()
	if len(m.Categories) != 6 || len(m.Activities) != 15 {
		t.Fatalf("got %d categories, %d root activities", len(m.Categories), len(m.Activities))
	}
	night := m.Activity("E001")
	// Everything under an activity is an activity too, keyed by its own id, so a
	// grandchild resolves without walking the tree.
	perf := byTitle(m, "2026 - 2027", "India Performance")
	if night == nil || len(night.Children) != 6 || perf == nil || perf.Parent != "E020" {
		t.Fatalf("international night did not load as expected: %+v", night)
	}
	if perf.Category != "" {
		t.Fatalf("a child should inherit its root's category, got %q", perf.Category)
	}
	if len(night.CoChairs()) != 2 {
		t.Fatalf("co-chairs: %v", night.CoChairs())
	}
}

func TestRenderHidesWhatItShould(t *testing.T) {
	cache, _ := newServer(t)
	view := Render(cache.Model(), fakeDirectory{}, parent, false, now())
	for _, a := range view.Activities {
		if a.Status == StatusHidden || a.Status == StatusPending {
			t.Errorf("%s reached a parent as %s", a.Title, a.Status)
		}
		if a.Title == "Room Parents" {
			for _, v := range a.Volunteers {
				if v.Position != PositionCoChair {
					t.Errorf("room parents list leaked %s", v.Email)
				}
			}
			if a.Taken != 1 {
				t.Errorf("room parents taken %d", a.Taken)
			}
		}
	}
	if view.User.Name != "Robin Whitfield" || view.People != nil {
		t.Errorf("user %+v, people %v", view.User, view.People)
	}
	suggester := Render(cache.Model(), fakeDirectory{}, "elena.torres@heliosschool.org", false, now())
	found := false
	for _, a := range suggester.Activities {
		if a.Title == "Family Escape Room Night" {
			found = true
		}
	}
	if !found {
		t.Error("a suggester cannot see their own pending suggestion")
	}
	if got := Render(cache.Model(), fakeDirectory{}, "someone.new@heliosschool.org", false, now()).User.Name; got != "Someone New" {
		t.Errorf("display name %q", got)
	}
}

func TestSignUpAndRemove(t *testing.T) {
	cache, mux := newServer(t)
	body := map[string]any{"id": "E017", "position": PositionOpen, "note": "happy to help"}
	if rec := call(t, mux, parent, "POST", "/api/events/volunteer", body); rec.Code != http.StatusNoContent {
		t.Fatalf("sign up: %d %s", rec.Code, rec.Body)
	}
	role := cache.Model().Activity("E017")
	if len(role.Volunteers) != 1 || role.Volunteers[0].Email != parent || role.Volunteers[0].AddedBy != parent {
		t.Fatalf("volunteers after sign up: %+v", role.Volunteers)
	}
	body["position"] = PositionCoChair
	if rec := call(t, mux, parent, "POST", "/api/events/volunteer", body); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent named themselves co-chair: %d", rec.Code)
	}
	if rec := call(t, mux, chair, "POST", "/api/events/volunteer", map[string]any{"id": "E017", "email": parent, "position": PositionCoChair}); rec.Code != http.StatusNoContent {
		t.Fatalf("a co-chair could not promote: %d %s", rec.Code, rec.Body)
	}
	full := map[string]any{"id": "E026", "position": PositionVolunteer}
	if rec := call(t, mux, parent, "POST", "/api/events/volunteer", full); rec.Code != http.StatusBadRequest {
		t.Fatalf("a full role took a sign-up: %d", rec.Code)
	}
	direct := map[string]any{"id": "E001", "position": PositionVolunteer}
	if rec := call(t, mux, parent, "POST", "/api/events/volunteer", direct); rec.Code != http.StatusBadRequest {
		t.Fatalf("an activity without direct sign-up took one: %d", rec.Code)
	}
	if rec := call(t, mux, "someone.else@heliosschool.org", "DELETE", "/api/events/volunteer", map[string]any{"id": "E017", "email": parent}); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger removed someone: %d", rec.Code)
	}
	if rec := call(t, mux, parent, "DELETE", "/api/events/volunteer", map[string]any{"id": "E017", "email": parent}); rec.Code != http.StatusNoContent {
		t.Fatalf("remove self: %d %s", rec.Code, rec.Body)
	}
	if n := len(cache.Model().Activity("E017").Volunteers); n != 0 {
		t.Fatalf("%d volunteers left after removal", n)
	}
}

func TestSuggestApproveRenameDelete(t *testing.T) {
	cache, mux := newServer(t)
	suggestion := map[string]any{"year": "2026 - 2027", "title": "Kite Day", "category": "C06", "status": StatusOpen, "description": "Fly kites", "coChair": true, "directSignUp": true}
	if rec := call(t, mux, parent, "POST", "/api/events/activity", suggestion); rec.Code != http.StatusNoContent {
		t.Fatalf("suggest: %d %s", rec.Code, rec.Body)
	}
	kite := byTitle(cache.Model(), "2026 - 2027", "Kite Day")
	if kite == nil || kite.Status != StatusPending || kite.AddedBy != parent || len(kite.Volunteers) != 1 || kite.Volunteers[0].Position != PositionOpen {
		t.Fatalf("suggestion landed as %+v", kite)
	}
	edit := map[string]any{"id": kite.ID, "year": "2026 - 2027", "title": "Kite Festival", "category": "C02", "status": StatusOpen, "directSignUp": true}
	if rec := call(t, mux, parent, "POST", "/api/events/activity", edit); rec.Code != http.StatusForbidden {
		t.Fatalf("a non-chair edited an activity: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/events/activity", edit); rec.Code != http.StatusNoContent {
		t.Fatalf("approve and rename: %d %s", rec.Code, rec.Body)
	}
	if byTitle(cache.Model(), "2026 - 2027", "Kite Day") != nil {
		t.Fatal("the old title survived the rename")
	}
	festival := cache.Model().Activity(kite.ID)
	if festival == nil || festival.Status != StatusOpen || len(festival.Volunteers) != 1 {
		t.Fatalf("renamed activity: %+v", festival)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/events/activity", map[string]string{"id": kite.ID}); rec.Code != http.StatusBadRequest {
		t.Fatalf("delete with a volunteer on it: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/events/volunteer", map[string]any{"id": kite.ID, "email": parent}); rec.Code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/events/activity", map[string]string{"id": kite.ID}); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Activity(kite.ID) != nil {
		t.Fatal("the activity survived deletion")
	}
}

// A title is not a key, so renaming touches one row and everything that hung
// off the old name is still there under the new one.
func TestRenameKeepsTheTree(t *testing.T) {
	cache, mux := newServer(t)
	edit := map[string]any{
		"id": "E020", "year": "2026 - 2027", "title": "India Booth", "parent": "E001",
		"category": "C08", "status": StatusOpen, "coLeaderNeeded": true, "directSignUp": true,
	}
	if rec := call(t, mux, chair, "POST", "/api/events/activity", edit); rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	booth := cache.Model().Activity("E020")
	if booth == nil || booth.Title != "India Booth" || len(booth.Volunteers) != 1 || len(booth.Links) != 1 || len(booth.Children) != 1 || booth.Children[0].Parent != "E020" {
		t.Fatalf("renamed: %+v", booth)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/events/activity", map[string]string{"id": "E020"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("deleted something with children and volunteers: %d", rec.Code)
	}
	// Putting something inside its own descendant is refused before the loader
	// would have to.
	loop := map[string]any{"id": "E001", "year": "2026 - 2027", "title": "International Night", "parent": "E020", "category": "", "status": StatusOpen}
	if rec := call(t, mux, admin, "POST", "/api/events/activity", loop); rec.Code != http.StatusBadRequest {
		t.Fatalf("a parent loop was accepted: %d", rec.Code)
	}
}

func TestCopyToNextYear(t *testing.T) {
	cache, mux := newServer(t)
	if rec := call(t, mux, chair, "POST", "/api/events/copy", map[string]string{"id": "E001"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a co-chair copied: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/events/copy", map[string]string{"id": "E001"}); rec.Code != http.StatusNoContent {
		t.Fatalf("copy: %d %s", rec.Code, rec.Body)
	}
	next := byTitle(cache.Model(), "2027 - 2028", "International Night")
	if next == nil || next.ID == "E001" || next.Status != StatusOpen || next.Start != "" || len(next.Volunteers) != 0 || len(next.Descendants()) != 6 || byTitle(cache.Model(), "2027 - 2028", "Cybertron") != nil || len(next.Links) != 2 {
		t.Fatalf("copied activity: %+v", next)
	}
	// Each copied child hangs off its copied parent, not the original.
	for _, c := range next.Descendants() {
		if p := cache.Model().Activity(c.Parent); p == nil || p.Year != "2027 - 2028" {
			t.Fatalf("copied child %q points at parent %q in the wrong year", c.Title, c.Parent)
		}
	}
	if rec := call(t, mux, admin, "POST", "/api/events/copy", map[string]string{"id": "E001"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("copied twice: %d", rec.Code)
	}
}

func TestYears(t *testing.T) {
	if got := SchoolYear(mustTime("2026-09-09")); got != "2026 - 2027" {
		t.Errorf("september: %s", got)
	}
	if got := SchoolYear(mustTime("2027-03-01")); got != "2026 - 2027" {
		t.Errorf("march: %s", got)
	}
	if got := ShiftYear("2026 - 2027", -1); got != "2025 - 2026" {
		t.Errorf("shift: %s", got)
	}
	if err := CheckYear("2026 - 2028"); err == nil {
		t.Error("a two-year span passed")
	}
}

// An event's categories are its own: a child may only name one of its root's, a
// root may only name a page heading, and Allow Adding gates proposals from
// people who do not run the event.
func TestEventCategories(t *testing.T) {
	cache, mux := newServer(t)
	m := cache.Model()
	night := m.Activity("E001")
	if len(night.Categories) != 2 || night.Categories[1].ID != "C08" || !night.Categories[1].AllowAdding {
		t.Fatalf("international night's categories: %+v", night.Categories)
	}
	if len(m.Categories) != 6 {
		t.Fatalf("the page should see only the six headings, got %d", len(m.Categories))
	}
	propose := func(who, category string) int {
		return call(t, mux, who, "POST", "/api/events/activity", map[string]any{
			"year": "2026 - 2027", "title": "Sweden", "parent": "E001", "category": category, "status": StatusOpen, "directSignUp": true,
		}).Code
	}
	if code := propose(parent, "C08"); code != http.StatusNoContent {
		t.Fatalf("a booth under an open category was refused: %d", code)
	}
	if code := propose(parent, "C07"); code != http.StatusBadRequest {
		t.Fatalf("a proposal into a closed category went through: %d", code)
	}
	if code := propose(chair, "C07"); code != http.StatusNoContent {
		t.Fatalf("the co-chair could not add into a closed category: %d", code)
	}
	if code := propose(chair, "C01"); code != http.StatusBadRequest {
		t.Fatalf("a child took a page heading as its category: %d", code)
	}
	// Only the event's own editors manage its categories; the page's need an admin.
	own := map[string]any{"eventId": "E001", "title": "Performances", "allowAdding": true}
	if rec := call(t, mux, parent, "POST", "/api/events/category", own); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent made an event category: %d", rec.Code)
	}
	if rec := call(t, mux, chair, "POST", "/api/events/category", own); rec.Code != http.StatusNoContent {
		t.Fatalf("the co-chair could not add an event category: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, chair, "POST", "/api/events/category", map[string]any{"title": "New Heading"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a co-chair made a page heading: %d", rec.Code)
	}
	if n := len(cache.Model().Activity("E001").Categories); n != 3 {
		t.Fatalf("event categories after adding: %d", n)
	}
	// Reordering one event's categories leaves the page's and other events' alone.
	ids := []string{}
	for _, c := range cache.Model().Activity("E001").Categories {
		ids = append(ids, c.ID)
	}
	ids[0], ids[1] = ids[1], ids[0]
	if rec := call(t, mux, chair, "POST", "/api/events/categories/order", map[string]any{"eventId": "E001", "ids": ids}); rec.Code != http.StatusNoContent {
		t.Fatalf("reorder: %d %s", rec.Code, rec.Body)
	}
	after := cache.Model()
	if after.Activity("E001").Categories[0].ID != ids[0] || after.Categories[0].ID != "C01" || after.Activity("E013").Categories[0].ID != "C09" {
		t.Fatalf("reorder leaked out of its scope")
	}
	// Copying an event copies its categories under new ids and repoints the children.
	if rec := call(t, mux, admin, "POST", "/api/events/copy", map[string]string{"id": "E001"}); rec.Code != http.StatusNoContent {
		t.Fatalf("copy: %d %s", rec.Code, rec.Body)
	}
	next := byTitle(cache.Model(), "2027 - 2028", "International Night")
	if next == nil || len(next.Categories) != 3 || next.Categories[0].ID == ids[0] {
		t.Fatalf("copied categories: %+v", next.Categories)
	}
	norway := byTitle(cache.Model(), "2027 - 2028", "Norway")
	if norway == nil || cache.Model().Category(norway.Category).EventID != next.ID {
		t.Fatalf("copied child still names the old event's category: %+v", norway)
	}
}

func TestUncategorizedFallback(t *testing.T) {
	cache, mux := newServer(t)
	tables := cache.Tables()
	next := *tables
	next.Activities = append([]map[string]string{}, tables.Activities...)
	blank := map[string]string{"Event ID": "E900", "Year": "2026 - 2027", "Title": "Blank", "Status": StatusOpen}
	unknown := map[string]string{"Event ID": "E901", "Year": "2026 - 2027", "Title": "Unknown", "Category": "nope", "Status": StatusOpen}
	borrowed := map[string]string{"Event ID": "E902", "Year": "2026 - 2027", "Title": "Borrowed", "Category": "C07", "Status": StatusOpen}
	child := map[string]string{"Event ID": "E903", "Year": "2026 - 2027", "Title": "Child", "Parent": "E001", "Category": "C09", "Status": StatusOpen}
	next.Activities = append(next.Activities, blank, unknown, borrowed, child)
	m, err := BuildModel(&next, bundled{})
	if err != nil {
		t.Fatal(err)
	}
	// A root with a blank, unknown or another event's category lands under the
	// built-in Uncategorized heading, which then appears last on the page.
	for _, id := range []string{"E900", "E901", "E902"} {
		if got := m.Activity(id).Category; got != UncategorizedID {
			t.Fatalf("%s: category %q", id, got)
		}
	}
	if last := m.Categories[len(m.Categories)-1]; last.ID != UncategorizedID || !last.BuiltIn || len(m.Categories) != 7 {
		t.Fatalf("categories: %+v", m.Categories)
	}
	// A child naming a category of some other event simply has none.
	if got := m.Activity("E903").Category; got != "" {
		t.Fatalf("child category %q", got)
	}
	// Nothing needs the heading in the sample data, so it is not there.
	if len(cache.Model().Categories) != 6 {
		t.Fatalf("the heading appeared without anything in it")
	}
	// Saving a root as Uncategorized stores a blank, and only editors may do it;
	// the heading itself cannot be edited or deleted.
	add := map[string]any{"year": "2026 - 2027", "title": "Loose End", "category": UncategorizedID, "status": StatusOpen}
	if rec := call(t, mux, parent, "POST", "/api/events/activity", add); rec.Code != http.StatusBadRequest {
		t.Fatalf("a proposal without a category went through: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/events/activity", add); rec.Code != http.StatusNoContent {
		t.Fatalf("admin add: %d %s", rec.Code, rec.Body)
	}
	loose := byTitle(cache.Model(), "2026 - 2027", "Loose End")
	if loose == nil || loose.Category != UncategorizedID || cache.Tables().count(activitiesTab, map[string]string{"Title": "Loose End", "Category": ""}) != 1 {
		t.Fatalf("loose end: %+v", loose)
	}
	if rec := call(t, mux, admin, "POST", "/api/events/category", map[string]any{"id": UncategorizedID, "title": "Misc"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("edited the built-in heading: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/events/category", map[string]any{"id": UncategorizedID}); rec.Code != http.StatusBadRequest {
		t.Fatalf("deleted the built-in heading: %d", rec.Code)
	}
}

func TestBrokenSheetStallsThePortalOnly(t *testing.T) {
	t.Chdir("../..")
	// A Volunteers tab still in the old shape: the load fails, but the cache
	// exists, every route says why, and the first good refresh brings it back.
	broken := t.TempDir()
	if err := os.MkdirAll(filepath.Join(broken, "events"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Categories", "Activities", "Links", "Settings", "Admins"} {
		raw, err := os.ReadFile(filepath.Join("sampledata", "events", name+".csv"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(broken, "events", name+".csv"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(broken, "events", "Volunteers.csv"), []byte("Year,Activity,Role,Email,Position,Note,Added By,Added\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cache, err := NewCache(&data.Dir{Root: broken}, bundled{}, func(string) bool { return false }, syncQueue{})
	if err == nil || cache == nil || cache.Model() != nil {
		t.Fatalf("a broken sheet should give a cache without a model and an error, got %v %v", cache, err)
	}
	mux := http.NewServeMux()
	Register(mux, cache, &data.Dir{Root: broken}, syncQueue{}, nil, fakeDirectory{}, func() []string { return nil })
	rec := call(t, mux, parent, "GET", "/api/events/model", nil)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `missing column "Event ID"`) {
		t.Fatalf("before the sheet loads: %d %s", rec.Code, rec.Body)
	}
	if cache.IsAdmin(admin) {
		t.Fatalf("nobody is a tab admin before the tab has loaded")
	}
	cache.source = &data.Dir{Root: "sampledata"}
	if err := cache.refresh(); err != nil {
		t.Fatal(err)
	}
	if rec := call(t, mux, parent, "GET", "/api/events/model", nil); rec.Code != http.StatusOK || cache.Err() != nil {
		t.Fatalf("after the sheet loads: %d %v", rec.Code, cache.Err())
	}
}

func TestHandWrittenRows(t *testing.T) {
	cache, _ := newServer(t)
	tables := cache.Tables()
	next := *tables
	next.Activities = append(cloneRows(tables.Activities),
		// A child may leave Year blank and takes its root's; blank switches
		// default to co-leader wanted, direct sign-up on, volunteers shown.
		map[string]string{"Event ID": "E900", "Title": "Bare Child", "Parent": "E020", "Status": StatusOpen},
		// No Event ID is a deleted row: not loaded, however broken the rest is.
		map[string]string{"Event ID": "", "Title": "Gone", "Year": "nonsense", "Status": "Active", "Parent": "nope"},
	)
	next.Volunteers = append(cloneRows(tables.Volunteers), map[string]string{"Event ID": "", "Email": "not an email", "Position": "Boss"})
	next.Links = append(cloneRows(tables.Links), map[string]string{"Event ID": "", "Title": "Old", "URL": "https://example.com"})
	m, err := BuildModel(&next, bundled{})
	if err != nil {
		t.Fatal(err)
	}
	bare := m.Activity("E900")
	if bare == nil || bare.Year != "2026 - 2027" || !bare.CoLeaderNeeded || !bare.DirectSignUp || bare.VolunteersHidden {
		t.Fatalf("bare child: %+v", bare)
	}
	if byTitle(m, "2026 - 2027", "Gone") != nil {
		t.Fatalf("a row without an id was loaded")
	}
	// Whatever pointed at a deleted row is deleted with it, all the way down;
	// a person listed twice on one thing counts once.
	next.Activities = append(next.Activities,
		map[string]string{"Event ID": "E910", "Title": "Orphan", "Parent": "gone-id", "Status": StatusOpen},
		map[string]string{"Event ID": "E911", "Title": "Orphan's Child", "Parent": "E910", "Status": StatusOpen},
	)
	next.Volunteers = append(next.Volunteers,
		map[string]string{"Event ID": "gone-id", "Email": parent, "Position": PositionVolunteer},
		map[string]string{"Event ID": "E001", "Email": chair, "Position": PositionVolunteer},
	)
	next.Links = append(next.Links, map[string]string{"Event ID": "E910", "Title": "Lost", "URL": "https://example.com"})
	m, err = BuildModel(&next, bundled{})
	if err != nil {
		t.Fatal(err)
	}
	if m.Activity("E910") != nil || m.Activity("E911") != nil {
		t.Fatalf("orphans were loaded")
	}
	want := Skipped{Deleted: 1, Orphans: 2, Volunteers: 2, Links: 2, Duplicates: 1}
	if m.Skipped != want {
		t.Fatalf("skipped %+v, want %+v", m.Skipped, want)
	}
	if n := len(m.Activity("E001").Volunteers); n != len(cache.Model().Activity("E001").Volunteers) {
		t.Fatalf("the duplicate sign-up was added: %d volunteers", n)
	}
	// A child whose Year disagrees with its root is still refused.
	next.Activities = append(next.Activities, map[string]string{"Event ID": "E901", "Title": "Lost", "Year": "2025 - 2026", "Parent": "E020", "Status": StatusOpen})
	if _, err := BuildModel(&next, bundled{}); err == nil || !strings.Contains(err.Error(), "has its parent") {
		t.Fatalf("a child in another year loaded: %v", err)
	}
}

func TestShowOnMainPage(t *testing.T) {
	cache, mux := newServer(t)
	if c := cache.Model().Category("C01"); !c.ShowOnMain {
		t.Fatalf("a heading defaults to being shown: %+v", c)
	}
	// A blank cell means shown; an event's own category is always shown.
	tables := cache.Tables()
	next := *tables
	next.Categories = cloneRows(tables.Categories)
	next.Categories[0]["Show On Main Page"] = ""
	next.Categories[6]["Show On Main Page"] = "No"
	m, err := BuildModel(&next, bundled{})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Category("C01").ShowOnMain || !m.Category("C07").ShowOnMain {
		t.Fatalf("blank or event-scoped categories should be shown")
	}
	off := false
	edit := map[string]any{"id": "C02", "title": "Activities", "allowAdding": false, "showOnMain": off}
	if rec := call(t, mux, admin, "POST", "/api/events/category", edit); rec.Code != http.StatusNoContent {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Category("C02").ShowOnMain || cache.Tables().count(categoriesTab, map[string]string{"Category ID": "C02", "Show On Main Page": "No"}) != 1 {
		t.Fatalf("the heading was not taken off the page")
	}
	// The event's own categories never carry the flag in the sheet.
	own := map[string]any{"eventId": "E001", "title": "Shifts", "allowAdding": true, "showOnMain": off}
	if rec := call(t, mux, chair, "POST", "/api/events/category", own); rec.Code != http.StatusNoContent {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	if cache.Tables().count(categoriesTab, map[string]string{"Title": "Shifts", "Show On Main Page": ""}) != 1 {
		t.Fatalf("an event category carried the page flag")
	}
}

func TestPrettyIDs(t *testing.T) {
	cache, mux := newServer(t)
	m := cache.Model()
	if m.ByPretty("International-Night") != m.Activity("E001") || m.ByPretty("nope") != nil {
		t.Fatalf("pretty lookup")
	}
	edit := func(id, pretty string, takeOver bool) *httptest.ResponseRecorder {
		act := cache.Model().Activity(id)
		return call(t, mux, admin, "POST", "/api/events/activity", map[string]any{
			"id": id, "year": act.Year, "title": act.Title, "category": act.Category, "status": act.Status,
			"directSignUp": true, "prettyId": pretty, "takeOver": takeOver,
		})
	}
	if rec := edit("E002", "Bad Address!", false); rec.Code != http.StatusBadRequest {
		t.Fatalf("a pretty id with spaces went through: %d", rec.Code)
	}
	// The same year's address is a plain clash.
	rec := edit("E002", "international-night", false)
	var conflict prettyConflict
	if rec.Code != http.StatusConflict || json.Unmarshal(rec.Body.Bytes(), &conflict) != nil || conflict.Prior || conflict.ID != "E001" {
		t.Fatalf("same-year clash: %d %s", rec.Code, rec.Body)
	}
	if rec := edit("E002", "international-night", true); rec.Code != http.StatusConflict {
		t.Fatalf("take-over of a current address went through: %d", rec.Code)
	}
	// A prior year's address is offered for renaming, and taken once agreed.
	last := byTitle(cache.Model(), "2025 - 2026", "International Night")
	rec = edit("E002", "international-night-2025", false)
	if rec.Code != http.StatusConflict || json.Unmarshal(rec.Body.Bytes(), &conflict) != nil || !conflict.Prior || conflict.ID != last.ID || conflict.Renamed != "international-night-2025-2025" {
		t.Fatalf("prior-year clash: %d %s", rec.Code, rec.Body)
	}
	if rec := edit("E002", "international-night-2025", true); rec.Code != http.StatusNoContent {
		t.Fatalf("take-over: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model()
	if m.Activity("E002").PrettyID != "international-night-2025" || m.Activity(last.ID).PrettyID != "international-night-2025-2025" {
		t.Fatalf("after take-over: %q %q", m.Activity("E002").PrettyID, m.Activity(last.ID).PrettyID)
	}
	// Every change of address leaves a redirect from the old path to the new:
	// the old address still finds the thing, through a chain of renames, and
	// a removed address sends people to the row's plain path. A bare word in
	// the sheet means /v/{word}. The sample sheet starts with one redirect.
	if m.Resolve("intl-night") != m.Activity("E001") || m.Resolve("/v/intl-night") != m.Activity("E001") || m.Resolve("nope") != nil {
		t.Fatalf("sample redirect")
	}
	if rec := edit("E002", "spring-party", false); rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	if rec := edit("E002", "", false); rec.Code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model()
	for _, old := range []string{"/v/spring-celebration", "/v/international-night-2025", "/v/spring-party", "/activities/E002"} {
		if m.Resolve(old) != m.Activity("E002") {
			t.Fatalf("%s did not reach the event: %+v", old, m.Redirects)
		}
	}
	if n := cache.Tables().count(redirectsTab, map[string]string{"Type": RedirectActivity, "Old": "/v/spring-party", "New": "/activities/E002"}); n != 1 {
		t.Fatalf("a removed address should redirect to the row: %+v", m.Redirects)
	}
	if rec := edit("E002", "international-night-2025", false); rec.Code != http.StatusNoContent {
		t.Fatalf("restore: %d %s", rec.Code, rec.Body)
	}
	// A child is addressed under its parent, by friendly name or id, and its
	// friendly name only has to be unique among its siblings. Renaming the
	// event carries every path under it along.
	m = cache.Model()
	norway, india := byTitle(m, "2026 - 2027", "Norway"), byTitle(m, "2026 - 2027", "India")
	if m.PathOf(norway) != "/v/international-night/"+norway.ID {
		t.Fatalf("child path %q", m.PathOf(norway))
	}
	child := func(node *Activity, pretty string) *httptest.ResponseRecorder {
		return call(t, mux, admin, "POST", "/api/events/activity", map[string]any{
			"id": node.ID, "year": node.Year, "title": node.Title, "parent": node.Parent, "category": node.Category, "status": node.Status,
			"directSignUp": true, "prettyId": pretty,
		})
	}
	if rec := child(norway, "norway"); rec.Code != http.StatusNoContent {
		t.Fatalf("child pretty: %d %s", rec.Code, rec.Body)
	}
	if rec := child(india, "norway"); rec.Code != http.StatusConflict {
		t.Fatalf("two siblings took one address: %d", rec.Code)
	}
	m = cache.Model()
	if m.Resolve("/v/international-night/norway") != m.Activity(norway.ID) || m.Resolve("/v/international-night/"+norway.ID) != m.Activity(norway.ID) || m.Resolve("/v/intl-night/norway") != m.Activity(norway.ID) {
		t.Fatalf("child by path: %q", m.PathOf(m.Activity(norway.ID)))
	}
	if rec := edit("E001", "inight", false); rec.Code != http.StatusNoContent {
		t.Fatalf("rename event: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model()
	if m.PathOf(m.Activity(norway.ID)) != "/v/inight/norway" || m.Resolve("/v/international-night/norway") != m.Activity(norway.ID) || m.Resolve("/v/intl-night/norway") != m.Activity(norway.ID) {
		t.Fatalf("event rename did not carry the booth: %q", m.PathOf(m.Activity(norway.ID)))
	}
	// Keeping one's own address is not a clash; a hand-edited duplicate loads
	// with the latest year keeping it.
	if rec := edit("E002", "International-Night-2025", false); rec.Code != http.StatusNoContent {
		t.Fatalf("re-saving own address: %d %s", rec.Code, rec.Body)
	}
	tables := cache.Tables()
	next := *tables
	next.Activities = cloneRows(tables.Activities)
	for _, row := range next.Activities {
		if row["Event ID"] == last.ID {
			row["Pretty ID"] = "inight"
		}
	}
	dup, err := BuildModel(&next, bundled{})
	if err != nil {
		t.Fatal(err)
	}
	if dup.ByPretty("inight") != dup.Activity("E001") || dup.Activity(last.ID).PrettyID != "" || dup.Skipped.PrettyIDs != 1 {
		t.Fatalf("duplicate in the sheet: %+v", dup.Skipped)
	}
}

func TestSharePreview(t *testing.T) {
	cache, mux := newServer(t)
	head := PreviewHead(cache)
	// An open event previews with its title, its date and a line of its
	// description, and its card; a hidden one, and any other page, show nothing.
	tags := head(httptest.NewRequest("GET", "https://hca.heliosian.com/v/intl-night/", nil))
	for _, want := range []string{`og:title" content="International Night"`, `og:url" content="https://hca.heliosian.com/v/international-night"`,
		`og:image" content="https://hca.heliosian.com/share/E001.png"`, `Thursday, September 24 · 4:00–6:00 PM — We invite you`} {
		if !strings.Contains(tags, want) {
			t.Fatalf("preview lacks %s:\n%s", want, tags)
		}
	}
	// A thing under an event is titled with the event and, having no date of
	// its own, previews with the event's.
	tags = head(httptest.NewRequest("GET", "https://hca.heliosian.com/v/international-night/E020", nil))
	for _, want := range []string{`og:title" content="India · International Night"`, `Thursday, September 24 · 4:00–6:00 PM`} {
		if !strings.Contains(tags, want) {
			t.Fatalf("child preview lacks %s:\n%s", want, tags)
		}
	}
	if head(httptest.NewRequest("GET", "https://hca.heliosian.com/activities/E006", nil)) != "" {
		t.Fatalf("a hidden thing was previewed")
	}
	if head(httptest.NewRequest("GET", "https://hca.heliosian.com/my", nil)) != "" {
		t.Fatalf("a page with nothing to preview got tags")
	}
	// The card is public - the mux is called without a session - and only for
	// what previews.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/share/E001.png", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || rec.Body.Len() < 10000 {
		t.Fatalf("card: %d %s %d bytes", rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/share/E006.png", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("a hidden thing has a card: %d", rec.Code)
	}
}
