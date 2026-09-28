package team

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"heliosian/internal/id"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

func TestOldIDsReachTheirThings(t *testing.T) {
	cache, mux := newServer(t)
	m := cache.Model()
	if a := m.Activity("E001"); a == nil || a.ID != "act0000000001" || m.Activity("e001") != a {
		t.Fatalf("old activity id: %+v", a)
	}
	if c := m.Category("C08"); c == nil || c.ID != "tcg0000000008" {
		t.Fatalf("old category id: %+v", c)
	}
	if m.Resolve("/activities/E001/E020") != m.Activity("act0000000020") || m.Resolve("/v/international-night/E020") != m.Activity("act0000000020") {
		t.Fatalf("an old id in a path did not reach its activity")
	}
	if rec := testkit.Call(t, mux, parent, "POST", "/api/team/volunteer", map[string]any{"id": "E017", "position": PositionVolunteer}); rec.Code != http.StatusNoContent {
		t.Fatalf("sign up by an old id: %d %s", rec.Code, rec.Body)
	}
	if cache.Count(volunteersTab, store.Row{"Event ID": "act0000000017", "Email": parent}) != 1 || cache.Count(volunteersTab, store.Row{"Event ID": "E017"}) != 0 {
		t.Fatalf("a sign-up by an old id was not stored under the activity's id")
	}
	if rec := testkit.Call(t, mux, parent, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000018", "position": PositionVolunteer, "from": "E017"}); rec.Code != http.StatusNoContent {
		t.Fatalf("move from an old id: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Activity("act0000000017").volunteer(parent) != nil || cache.Model().Activity("act0000000018").volunteer(parent) == nil {
		t.Fatalf("a move from an old id did not move")
	}
	if rec := testkit.Call(t, mux, admin, "POST", "/api/team/category", map[string]any{"id": "C02", "title": "Gatherings", "allowAdding": AddingNo}); rec.Code != http.StatusNoContent {
		t.Fatalf("edit a category by an old id: %d %s", rec.Code, rec.Body)
	}
	if cache.Count(categoriesTab, store.Row{"Category ID": "tcg0000000002", "Title": "Gatherings"}) != 1 || cache.Count(categoriesTab, store.Row{"Category ID": "C02"}) != 0 {
		t.Fatalf("a category edit by an old id missed its row")
	}
	rec := testkit.Call(t, mux, parent, "POST", "/api/team/activity", map[string]any{"year": "2026 - 2027", "title": "Sweden", "parent": "E001", "category": "C08"})
	if rec.Code != http.StatusOK {
		t.Fatalf("add under old ids: %d %s", rec.Code, rec.Body)
	}
	if cache.Count(activitiesTab, store.Row{"Title": "Sweden", "Parent": "act0000000001", "Category": "tcg0000000008"}) != 1 {
		t.Fatalf("an add naming old ids stored them")
	}
	order := []string{"E025", "act0000000023", "E024"}
	if rec := testkit.Call(t, mux, chair, "POST", "/api/team/order", map[string]any{"parent": "E002", "ids": order}); rec.Code != http.StatusNoContent {
		t.Fatalf("reorder by old ids: %d %s", rec.Code, rec.Body)
	}
	if first := cache.Model().Activity("act0000000002").Children[0]; first.ID != "act0000000025" {
		t.Fatalf("reorder by old ids put %s first", first.Title)
	}
}

func TestOldPagePathsRedirect(t *testing.T) {
	cache, mux := newServer(t)
	if rec := testkit.Call(t, mux, admin, "POST", "/api/team/redirect", map[string]string{"old": "/old-booth", "new": "/activities/E001/E020"}); rec.Code != http.StatusNoContent {
		t.Fatalf("a redirect naming old ids: %d %s", rec.Code, rec.Body)
	}
	handler := Redirected(cache, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	for _, c := range []struct {
		path   string
		status int
		to     string
	}{
		{"/activities/E001", http.StatusMovedPermanently, "https://example.com/v/international-night"},
		{"/activities/E001?x=1", http.StatusMovedPermanently, "https://example.com/v/international-night?x=1"},
		{"/activities/E003/E026", http.StatusMovedPermanently, "https://example.com/activities/act0000000003/act0000000026"},
		{"/activities/act0000000003/E026", http.StatusMovedPermanently, "https://example.com/activities/act0000000003/act0000000026"},
		{"/v/international-night/E020", http.StatusMovedPermanently, "https://example.com/v/international-night/act0000000020"},
		{"/old-booth", http.StatusFound, "https://example.com/v/international-night/act0000000020"},
		{"/activities/act0000000003/act0000000026", http.StatusTeapot, ""},
		{"/activities/act0000000001", http.StatusTeapot, ""},
		{"/activities/E999", http.StatusTeapot, ""},
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", c.path, nil))
		if rec.Code != c.status || rec.Header().Get("Location") != c.to {
			t.Errorf("%s: %d %q, want %d %q", c.path, rec.Code, rec.Header().Get("Location"), c.status, c.to)
		}
	}
}

func TestMintedIDs(t *testing.T) {
	cache, mux := newServer(t)
	rec := testkit.Call(t, mux, admin, "POST", "/api/team/activity", map[string]any{"year": "2026 - 2027", "title": "Bake Sale", "category": "tcg0000000002", "status": StatusOpen})
	var saved activityRef
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &saved) != nil {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	if _, ok := id.Parse(saved.ID); !ok || cache.Model().Activity(saved.ID) == nil || cache.Model().Activity(saved.ID).Title != "Bake Sale" {
		t.Fatalf("the add answered %q", saved.ID)
	}
	if rec := testkit.Call(t, mux, admin, "POST", "/api/team/category", map[string]any{"title": "Fundraisers", "allowAdding": AddingNo}); rec.Code != http.StatusNoContent {
		t.Fatalf("add category: %d %s", rec.Code, rec.Body)
	}
	for _, c := range cache.Model().Categories {
		if _, ok := id.Parse(c.ID); !ok && !c.BuiltIn {
			t.Errorf("category %q has id %q", c.Title, c.ID)
		}
	}
	if rec := testkit.Call(t, mux, admin, "POST", "/api/team/link", map[string]any{"id": saved.ID, "title": "Menu", "url": "https://example.org/menu"}); rec.Code != http.StatusNoContent {
		t.Fatalf("add link: %d %s", rec.Code, rec.Body)
	}
	if links := cache.Model().Activity(saved.ID).Links; len(links) != 1 {
		t.Fatalf("links: %+v", links)
	} else if _, ok := id.Parse(links[0].ID); !ok {
		t.Fatalf("link id %q", links[0].ID)
	}
	if rec := testkit.Call(t, mux, admin, "POST", "/api/team/copy", map[string]string{"id": "act0000000001"}); rec.Code != http.StatusNoContent {
		t.Fatalf("copy: %d %s", rec.Code, rec.Body)
	}
	m := cache.Model()
	next := byTitle(m, "2027 - 2028", "International Night")
	seen := map[string]bool{}
	for _, a := range append([]*Activity{next}, next.Descendants()...) {
		ids := []string{a.ID}
		for _, l := range a.Links {
			ids = append(ids, l.ID)
		}
		for _, c := range a.Categories {
			ids = append(ids, c.ID)
		}
		for _, key := range ids {
			if _, ok := id.Parse(key); !ok || seen[key] {
				t.Errorf("copied %q has id %q, or it is used twice", a.Title, key)
			}
			seen[key] = true
		}
	}
	for _, l := range m.Activity("act0000000001").Links {
		if seen[l.ID] {
			t.Errorf("the copy reused link id %s", l.ID)
		}
	}
	if len(seen) != 1+6+2+3 {
		t.Errorf("copied ids: %d", len(seen))
	}
}

func TestLinksByID(t *testing.T) {
	cache, mux := newServer(t)
	const booth = "act0000000019"
	for _, url := range []string{"https://example.org/one", "https://example.org/two"} {
		if rec := testkit.Call(t, mux, admin, "POST", "/api/team/link", map[string]any{"id": booth, "title": "Plan", "url": url}); rec.Code != http.StatusNoContent {
			t.Fatalf("add link: %d %s", rec.Code, rec.Body)
		}
	}
	links := cache.Model().Activity(booth).Links
	if len(links) != 2 || links[0].ID == links[1].ID {
		t.Fatalf("two links of one title: %+v", links)
	}
	one, two := links[0], links[1]
	if rec := testkit.Call(t, mux, parent, "POST", "/api/team/link", map[string]any{"link": one.ID, "title": "Mine", "url": "https://example.org/x"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent edited a link: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, admin, "POST", "/api/team/link", map[string]any{"link": one.ID, "title": "Plan", "url": "https://example.org/uno"}); rec.Code != http.StatusNoContent {
		t.Fatalf("edit link: %d %s", rec.Code, rec.Body)
	}
	links = cache.Model().Activity(booth).Links
	if len(links) != 2 || links[0].ID != one.ID || links[0].URL != "https://example.org/uno" || links[1] != two {
		t.Fatalf("after editing one: %+v", links)
	}
	if rec := testkit.Call(t, mux, admin, "DELETE", "/api/team/link", map[string]any{"link": two.ID}); rec.Code != http.StatusNoContent {
		t.Fatalf("delete link: %d %s", rec.Code, rec.Body)
	}
	links = cache.Model().Activity(booth).Links
	if len(links) != 1 || links[0].ID != one.ID {
		t.Fatalf("after deleting two: %+v", links)
	}
	if rec := testkit.Call(t, mux, admin, "DELETE", "/api/team/link", map[string]any{"link": two.ID}); rec.Code != http.StatusNotFound {
		t.Fatalf("deleted a deleted link: %d", rec.Code)
	}
	next := tables(t)
	next[linksTab] = append(next[linksTab], store.Row{"Link ID": one.ID, "Event ID": booth, "Title": "Again", "URL": "https://example.org/again"})
	if _, err := BuildModel(t.Context(), next, bundled); err == nil {
		t.Fatal("a link id used twice loaded")
	}
	next = tables(t)
	next[linksTab] = append(next[linksTab], store.Row{"Link ID": "", "Event ID": booth, "Title": "Bare", "URL": "https://example.org/bare"})
	if _, err := BuildModel(t.Context(), next, bundled); err == nil {
		t.Fatal("a link without an id loaded")
	}
}
