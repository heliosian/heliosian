package db

import (
	"testing"

	"heliosian/internal/store"
)

func TestEachQueuesQueryListsWhatItCounts(t *testing.T) {
	s := sample(t)
	if err := commit(s, DocumentsSheet,
		store.Insert("CONTENT", store.Row{"id": "cnt00000000001", "hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000002", "hash": "b2", "blob": "content/b2", "mime": "text/html; charset=utf-8", "size": "200"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000003", "hash": "c3", "blob": "content/c3", "mime": "image/png", "size": "300"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000010", "kind": "mail", "content": "cnt00000000001", "name": "Not yet read"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000011", "relation": "part", "parent": "doc00000000010", "content": "cnt00000000002"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000012", "relation": "image", "parent": "doc00000000011", "url": "https://example.org/a.png"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000013", "relation": "linked", "parent": "doc00000000011", "url": "https://example.org/page", "fetch": "gone"}),
	); err != nil {
		t.Fatal(err)
	}
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
	for name, want := range map[string]int{"extraction": 2, "extraction message/rfc822": 1, "extraction text/html": 1, "fetching": 1, "fetching image": 1, "fetching linked": 0, "classifying": 1, "content sweep": 1, "content sweep image/png": 1} {
		if counted[name] != want {
			t.Errorf("%s pending %d, want %d", name, counted[name], want)
		}
	}
	if !m.superAdmin(staff) || m.superAdmin(parent) || m.superAdmin("") {
		t.Fatal("only super admins may read the queues")
	}
}
