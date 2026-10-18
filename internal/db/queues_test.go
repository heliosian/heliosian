package db

import (
	"testing"
	"time"

	"heliosian/internal/cells"
)

func TestEachQueuesQueryListsWhatItCounts(t *testing.T) {
	s, queue := sampleWithQueue(t)
	undigested := 0
	for _, q := range s.Model().queueCounts() {
		if q.Name == "digest" {
			undigested = q.Pending
		}
	}
	apply(t, s, queue, []Edit{
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
	for name, want := range map[string]int{"extraction": 2, "extraction message/rfc822": 1, "extraction text/html": 1, "fetching": 1, "fetching image": 1, "fetching linked": 0, "fetching signed in": 1, "fetching signed in image": 0, "fetching signed in linked": 1, "classifying": 4, "digest": undigested + 1,"content sweep": 1, "content sweep image/png": 1} {
		if counted[name] != want {
			t.Errorf("%s pending %d, want %d", name, counted[name], want)
		}
	}
	if !m.superAdmin(staff) || m.superAdmin(parent) || m.superAdmin("") {
		t.Fatal("only super admins may read the queues")
	}
}
