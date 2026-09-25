package who

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"heliosian/internal/auth"
	"heliosian/internal/data"
)

func TestEveryQueuedWriteReleasesItsCount(t *testing.T) {
	dir := &data.Dir{Root: "../../sampledata"}
	queue := NewQueue()
	cache, err := NewCache(dir, dir, &countingGeocoder{}, noBlobs{}, noBlobs{}, nil, queue, testKey, func() []string { return nil })
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterTags(mux, cache, dir, queue, nil)
	RegisterAdmin(mux, cache, dir, queue)
	const owner, other = "jordan.whitfield@heliosschool.org", "abena.osei@heliosschool.org"
	post := func(as, path, contentType, body string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", contentType)
		rec := httptest.NewRecorder()
		auth.Fixed(as, mux).ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body)
		}
	}
	form := func(as, path string, values url.Values) {
		t.Helper()
		post(as, path, "application/x-www-form-urlencoded", values.Encode())
	}
	form(owner, "/api/directory/tag", url.Values{"tag": {"Carpool"}, "person": {"daniel.park@heliosschool.org"}, "on": {"0"}})
	form(owner, "/api/directory/tag-rename", url.Values{"tag": {"Soccer Team"}, "name": {"Football"}})
	form(owner, "/api/directory/tag-copy", url.Values{"tag": {"Football"}, "name": {"Kicks"}})
	form(owner, "/api/directory/tag-share", url.Values{"tag": {"Kicks"}, "manager": {other}, "on": {"1"}})
	form(other, "/api/directory/tag-leave", url.Values{"tag": {"Kicks"}, "owner": {owner}})
	form(owner, "/api/directory/tag-delete", url.Values{"tag": {"Kicks"}})
	form(owner, "/api/directory/tag-delete", url.Values{"tag": {"Nothing Here"}})
	post(owner, "/api/admin/admins", "application/json", `{"admins":["`+owner+`","`+other+`"]}`)
	done := make(chan struct{})
	queue.Add(func() { close(done) })
	<-done
	cache.mu.RLock()
	pending := cache.pending
	cache.mu.RUnlock()
	if pending != 0 {
		t.Fatalf("%d writes still counted after the queue ran", pending)
	}
	if err := cache.refresh(); err != nil {
		t.Fatal(err)
	}
	if len(cache.Tags(owner)["Football"]) == 0 || len(cache.Tags(owner)["Kicks"]) != 0 || !cache.IsAdmin(other) {
		t.Fatal("the refresh after the writes did not read them back")
	}
}
