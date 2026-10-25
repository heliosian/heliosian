package db

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/intercept"
	"heliosian/internal/testkit"
)

func uploadMail(t *testing.T, s *Store, pics *Pictures, eml string) string {
	t.Helper()
	rec := postMail(t, s, pics, importing, []byte(eml))
	if rec.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	var out stored
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Result[0]
}

func sentTo(s *Store, document string) []string {
	out := []string{}
	for _, link := range s.Model().Table("DOCUMENT_GROUP").Referencing("document", document) {
		if link["relation"] == "sent_to" {
			out = append(out, link["group"])
		}
	}
	slices.Sort(out)
	return out
}

func TestABodyIsItsExtractNotTheLinksAfterIt(t *testing.T) {
	s := sample(t)
	ids := apply(t, s, []Edit{
		{Insert: "CONTENT", As: "mail", Row: map[string]any{"hash": "m1", "blob": "content/m1", "mime": "message/rfc822", "size": "100"}},
		{Insert: "CONTENT", As: "html", Row: map[string]any{"hash": "h1", "blob": "content/h1", "mime": "text/html; charset=utf-8", "size": "90"}},
		{Insert: "CONTENT", As: "text", Row: map[string]any{"hash": "t1", "blob": "content/t1", "mime": "text/markdown", "size": "40"}},
		{Insert: "CONTENT", As: "logo", Row: map[string]any{"hash": "l1", "blob": "content/l1", "mime": "image/png", "size": "30"}},
		{Insert: "DOCUMENT", As: "update", Row: map[string]any{"kind": "mail", "content": "@mail", "name": "Weekly Update", "extracted": "2026-10-16 17:40:13"}},
		{Insert: "DOCUMENT", As: "part", Row: map[string]any{"relation": "part", "parent": "@update", "content": "@html", "order": "9", "extracted": "2026-10-16 17:40:25"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "extract", "parent": "@part", "content": "@text", "order": "4"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "image", "parent": "@part", "content": "@logo", "order": "9"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "linked", "parent": "@part", "url": "https://example.org/class-page", "fetch": "refused", "order": "d"}},
	})
	m := s.Model()
	root, _ := m.Table("DOCUMENT").Get(ids[4])
	body, ready := m.bodyMarkdown(root)
	if !ready || body != ids[2] {
		t.Fatalf("the body is %q (ready %v), want the extract's content %q", body, ready, ids[2])
	}
}

func TestTheNewestMailIsClassifiedFirst(t *testing.T) {
	s := sample(t)
	ids := apply(t, s, []Edit{
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "mail", "name": "Middle", "published": "2026-10-10 09:00:00"}},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "mail", "name": "Oldest", "published": "2019-03-01 09:00:00"}},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "mail", "name": "Newest", "published": "2027-01-05 09:00:00"}},
	})
	c := &Classifier{s: s, skipped: map[string]bool{}}
	got := []string{}
	for _, id := range c.pending() {
		if slices.Contains(ids, id) {
			got = append(got, id)
		}
	}
	if want := []string{ids[2], ids[0], ids[1]}; !slices.Equal(got, want) {
		t.Errorf("classified in the order %v, want newest first %v", got, want)
	}
}

func TestUnlistedMailIsSentToWhomClaudeReads(t *testing.T) {
	var mu sync.Mutex
	asked := []string{}
	regraded, circled := false, false
	intercept.Install(intercept.ClaudeHost, testkit.ClaudeReplying(func(request string) string {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.Contains(request, "Field trip") && circled:
			return `{"classrooms": [], "grades": [], "nobody": true}`
		case strings.Contains(request, "Field trip") && regraded:
			return `{"classrooms": [], "grades": ["Grade 3"], "nobody": false}`
		case strings.Contains(request, "Field trip"):
			asked = append(asked, "Field trip")
			return `{"classrooms": ["hummingbirds"], "grades": [], "nobody": false}`
		case strings.Contains(request, "Third grade play"):
			asked = append(asked, "Third grade play")
			return `{"classrooms": [], "grades": ["Grade 3"], "nobody": false}`
		case strings.Contains(request, "Lit circle"):
			asked = append(asked, "Lit circle")
			return `{"classrooms": [], "grades": [], "nobody": true}`
		}
		asked = append(asked, "Picnic")
		return `{"classrooms": [], "grades": [], "nobody": false}`
	}))
	s := sample(t)
	gradeParents := apply(t, s, []Edit{{Insert: "GROUP", Row: map[string]any{"posting": "members", "replying": "members", "kind": "group", "status": "open", "slug": "grade-3-parents", "name": "Grade 3 Parents", "visible_to": "grp00000000004"}}})[0]
	bucket := blob.NewMemoryBucket()
	pics := NewPictures(s, bucket)
	NewExtractor(s, bucket, "test")
	StartClassifier(s, bucket, "test")
	mail := func(subject, list string) string {
		header := "From: Maya Lindqvist <maya.lindqvist@example.org>\r\nDate: Thu, 12 Feb 2026 01:48:03 +0000\r\nSubject: " + subject + "\r\n"
		if list != "" {
			header += "List-Id: <" + list + ">\r\n"
		}
		return header + "Content-Type: text/html; charset=utf-8\r\n\r\n<p>" + subject + " news.</p>\r\n"
	}
	trip := uploadMail(t, s, pics, mail("Field trip", ""))
	play := uploadMail(t, s, pics, mail("Third grade play", ""))
	picnic := uploadMail(t, s, pics, mail("Picnic", ""))
	circle := uploadMail(t, s, pics, mail("Lit circle", ""))
	listed := uploadMail(t, s, pics, mail("Staff lunch", "parentsandstaff.heliosschool.org"))
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, gone := s.Model().Table("DOCUMENT").Get(circle)
		through, _ := s.Model().Table("DOCUMENT").Get(listed)
		if len(sentTo(s, trip)) > 0 && len(sentTo(s, play)) > 0 && len(sentTo(s, picnic)) > 0 && !gone && through["classified"] == ClassifyRevision {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not classified after two seconds: trip %v, play %v, picnic %v, lit circle still there %v", sentTo(s, trip), sentTo(s, play), sentTo(s, picnic), gone)
		}
		time.Sleep(10 * time.Millisecond)
	}
	for document, want := range map[string]string{trip: "grp00000000030", play: gradeParents, picnic: "grp00000000004", listed: "grp00000000002 grp00000000003"} {
		if got := strings.Join(sentTo(s, document), " "); got != want {
			t.Errorf("%s was sent to %s, want %s", document, got, want)
		}
	}
	for _, row := range s.Model().Table("DOCUMENT").All() {
		if row["parent"] == circle {
			t.Fatalf("a part of the lit circle's email is left: %v", row)
		}
	}
	for _, document := range []string{trip, play, picnic, listed} {
		if row, _ := s.Model().Table("DOCUMENT").Get(document); row["classified"] != ClassifyRevision {
			t.Errorf("%s is classified at %q, want %q", document, row["classified"], ClassifyRevision)
		}
	}
	mu.Lock()
	regraded = true
	mu.Unlock()
	apply(t, s, []Edit{{Set: trip, Cells: map[string]any{"classified": "2026-01-01"}}})
	deadline = time.Now().Add(2 * time.Second)
	for strings.Join(sentTo(s, trip), " ") != gradeParents {
		if time.Now().After(deadline) {
			t.Fatalf("not classified again after two seconds: trip sent to %v, want %s", sentTo(s, trip), gradeParents)
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	circled = true
	mu.Unlock()
	apply(t, s, []Edit{{Set: trip, Cells: map[string]any{"classified": "2026-01-01"}}})
	deadline = time.Now().Add(2 * time.Second)
	for {
		if _, held := s.Model().Table("DOCUMENT").Get(trip); !held {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("an email with groups, classified again as to nobody, is still there after two seconds, sent to %v", sentTo(s, trip))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := len(s.Model().Table("DOCUMENT_GROUP").Referencing("document", trip)); n != 0 {
		t.Errorf("%d links outlived the deleted email", n)
	}
	mu.Lock()
	defer mu.Unlock()
	if slices.Contains(asked, "Staff lunch") {
		t.Fatal("mail through a list was sent to Claude")
	}
}
