package db

import (
	"encoding/json"
	"heliosian/internal/blob"
	"heliosian/internal/ops"
	"heliosian/internal/testkit"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"heliosian/internal/cells"
)

func TestEachQueuesQueryListsWhatItCounts(t *testing.T) {
	s := sample(t)
	before := map[string]int{}
	for _, q := range s.Model().queueCounts() {
		before[q.Name] = q.Pending
	}
	apply(t, s, []Edit{
		{Insert: "CONTENT", As: "mail", Row: map[string]any{"hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}},
		{Insert: "CONTENT", As: "html", Row: map[string]any{"hash": "b2", "blob": "content/b2", "mime": "text/html; charset=utf-8", "size": "200"}},
		{Insert: "CONTENT", Row: map[string]any{"hash": "c3", "blob": "content/c3", "mime": "image/png", "size": "300"}},
		{Insert: "DOCUMENT", As: "unread", Row: map[string]any{"kind": "mail", "content": "@mail", "name": "Not yet read"}},
		{Insert: "DOCUMENT", As: "part", Row: map[string]any{"relation": "part", "parent": "@unread", "content": "@html"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "image", "parent": "@part", "url": "https://example.org/a.png"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "linked", "parent": "@part", "url": "https://example.org/page", "fetch": "gone"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "linked", "parent": "@part", "url": "https://docs.example.org/locked", "fetch": "sign_in"}},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "mail", "name": "Not yet digested", "published": time.Now().In(School).Format(cells.StampFormat), "digest": "2026-01-01"}},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "mail", "name": "Digested", "published": time.Now().In(School).Format(cells.StampFormat), "digest": DigestRevision}},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "mail", "name": "Too old to digest", "published": time.Now().In(School).Add(-2 * MailWindow).Format(cells.StampFormat)}},
	})
	m := s.Model()
	counted := map[string]int{}
	for _, q := range m.queueCounts() {
		counted[q.Name] = q.Pending
		for _, c := range append([]queueCount{q}, q.Parts...) {
			if c.Query == "" {
				continue
			}
			if got := len(as(t, s, staff, c.Query)); got != c.Pending {
				t.Errorf("%s %s: counts %d pending, its query lists %d:\n%s", q.Name, c.Name, c.Pending, got, c.Query)
			}
		}
		for _, p := range q.Parts {
			counted[q.Name+" "+p.Name] = p.Pending
		}
	}
	for name, want := range map[string]int{"extraction": 2, "extraction message/rfc822": 1, "extraction text/html": 1, "fetching": 1, "fetching image": 1, "fetching linked": 0, "fetching signed in": 1, "fetching signed in image": 0, "fetching signed in linked": 1, "classifying": before["classifying"] + 4, "digest": before["digest"] + 1, "content sweep": 1, "content sweep image/png": 1} {
		if counted[name] != want {
			t.Errorf("%s pending %d, want %d", name, counted[name], want)
		}
	}
	if !m.superAdmin(staff) || m.superAdmin(parent) || m.superAdmin("") {
		t.Fatal("only super admins may read the queues")
	}
}

func TestQueueStatusAcceptsOnlySuperAdminTokens(t *testing.T) {
	s := sample(t)
	mux := http.NewServeMux()
	RegisterQueues(mux, s, testTokens)
	for _, tc := range []struct {
		email  string
		status int
	}{
		{superAdmin, http.StatusOK},
		{"rowan.ashdown@example.org", http.StatusForbidden},
		{"", http.StatusUnauthorized},
	} {
		r := httptest.NewRequest(http.MethodGet, "/api/queues", nil)
		token := "invalid"
		if tc.email != "" {
			token = testTokens.Issue(tc.email, time.Now())
		}
		r.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		if rec.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.email, rec.Code, rec.Body.String())
		}
		if tc.status != http.StatusOK {
			continue
		}
		var report queueReport
		if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		found := map[string]bool{}
		for _, queue := range report.Queues {
			found[queue.Name] = true
		}
		for _, name := range []string{"thumbnails", "search indexing", "search processing", "pending writes"} {
			if !found[name] {
				t.Errorf("missing %s in queue report", name)
			}
		}
	}
}

func TestCopiedPageThumbnailsCreateNoThumbnailWork(t *testing.T) {
	s := sample(t)
	bucket := blob.NewMemoryBucket()
	pics := NewPictures(s, bucket)
	ids := apply(t, s, []Edit{
		{Insert: "CONTENT", As: "pdf", Row: storedContent(t, bucket, pdfType, pdfOfPages(t, 1))},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "file", "content": "@pdf"}},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "file", "content": "@pdf"}},
	})
	rec := postFile(t, s, pics, importing, "pdf-page", map[string]string{"document": ids[1], "content": ids[0], "page": "1", "pages": "1"}, "picture", redPNG(t, 120, 180))
	if rec.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	page := children(s, ids[1])[0]
	pending := func() int {
		for _, queue := range s.Model().queueCounts() {
			if queue.Name == "thumbnails" {
				return queue.Pending
			}
		}
		t.Fatal("missing thumbnail queue")
		return -1
	}
	before := pending()
	if before < 1 {
		t.Fatal("uploaded page was not queued for a server thumbnail")
	}
	worker := &Thumbnailer{s: s, bucket: bucket}
	if _, err := worker.make(page); err != nil {
		t.Fatal(err)
	}
	page, _ = s.Model().Table("DOCUMENT").Get(page["id"])
	if page["thumbnail"] == "" || page["thumbnailed"] == "" || pending() != before-1 {
		t.Fatalf("server thumbnail did not drain: %v", page)
	}
	apply(t, s, []Edit{{Insert: "DOCUMENT", Row: map[string]any{"parent": ids[2], "relation": "page", "content": page["content"], "thumbnail": page["thumbnail"], "thumbnailed": page["thumbnailed"], "first_page": "1", "last_page": "1", "extracted": page["extracted"]}}})
	if pending() != before-1 {
		t.Fatal("copied page created thumbnail work")
	}
}

func TestAbandonedThumbnailDrainsPendingUntilRestart(t *testing.T) {
	s := sample(t)
	m := s.Model()
	edits := []Edit{}
	for _, doc := range m.Table("DOCUMENT").All() {
		content, _ := m.Table("CONTENT").Get(doc["content"])
		if _, ok := drawers[baseType(content["mime"])]; ok && doc["thumbnailed"] == "" {
			edits = append(edits, Edit{Set: doc["id"], Cells: map[string]any{"thumbnailed": "2026-10-26 20:00:00"}})
		}
	}
	apply(t, s, edits)
	bucket := blob.NewMemoryBucket()
	ids := apply(t, s, []Edit{
		{Insert: "CONTENT", As: "broken", Row: storedContent(t, bucket, "image/png", []byte("not a png"))},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "file", "content": "@broken"}},
	})
	m = s.Model()
	if got := m.thumbnailQueue().Pending; got != 1 {
		t.Fatalf("pending before attempts: %d", got)
	}
	b := &board{store: s, ops: ops.New(testkit.OpsDeps(t, bucket))}
	b.snapshot()
	StartThumbnailer(s, bucket)
	key := [2]string{ids[1], ids[0]}
	for deadline := time.Now().Add(2 * time.Second); !m.services.thumbnails.exhausted(key); time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("thumbnail worker did not exhaust its attempts")
		}
		m.services.inbox(thumbnailInbox).wake.poke()
	}
	m.services.thumbnails.mu.Lock()
	failures := m.services.thumbnails.failures[key]
	m.services.thumbnails.mu.Unlock()
	if failures != thumbnailTries {
		t.Fatalf("failures=%d, expected %d", failures, thumbnailTries)
	}
	if s.Model() != m {
		t.Fatal("abandoning a thumbnail should not write a completion marker")
	}
	if queue := m.thumbnailQueue(); queue.Pending != 0 {
		t.Fatalf("abandoned thumbnail still pending: %+v", queue)
	}
	for _, queue := range b.snapshot().Queues {
		if queue.Name == "thumbnails" && queue.Pending != 0 {
			t.Fatal("dashboard cached the abandoned thumbnail as pending")
		}
	}
	doc, _ := m.Table("DOCUMENT").Get(ids[1])
	if doc["thumbnailed"] != "" || doc["thumbnail"] != "" {
		t.Fatalf("failed thumbnail was marked complete: %v", doc)
	}
	restarted := *m
	restarted.services = newServices(m.index)
	if got := restarted.thumbnailQueue().Pending; got != 1 {
		t.Fatalf("restart did not retry the failed thumbnail: pending=%d", got)
	}
}
