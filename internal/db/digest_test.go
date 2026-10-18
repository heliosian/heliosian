package db

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/intercept"
	"heliosian/internal/testkit"
)

func TestTheDigestReadsMailIntoToDosThatKeepTheirMarks(t *testing.T) {
	var mu sync.Mutex
	answers := map[string]string{
		"Field trip": `{"summary": "The Jays visit the tide pools and need slips.", "points": ["Return the slip by Fri, Oct 2.", "Pack a lunch."], "asks": "Return a slip.", "todos": [{"title": "Return the tide pool slip.", "summary": "Signed slip for the trip", "details": "Sign and return it by Friday.", "link": "https://example.org/slip", "due": "2026-10-02", "point": 1}, {"title": "Pay the fee", "summary": "Trip fee", "details": "Pay online.", "link": "https://example.org/made-up", "due": "Friday", "point": 7}], "repeats": []}`,
		"Reminder":   `{"summary": "A reminder about the slips.", "points": ["Slips are due Friday."], "asks": "Return a slip.", "todos": [{"title": "Return the slip again", "summary": "Same slip", "details": "Return it.", "link": "", "due": "", "point": 1}], "repeats": [{"point": 1, "listed": LISTED}]}`,
	}
	listedSlip := regexp.MustCompile(`(\d+)\. Return the tide pool slip \(`)
	intercept.Install(intercept.ClaudeHost, testkit.ClaudeReplying(func(request string) string {
		mu.Lock()
		defer mu.Unlock()
		for subject, answer := range answers {
			if strings.Contains(request, "Subject: "+subject) {
				if found := listedSlip.FindStringSubmatch(request); found != nil {
					answer = strings.ReplaceAll(answer, "LISTED", found[1])
				}
				return answer
			}
		}
		t.Errorf("claude was asked about something else: %s", request)
		return `{}`
	}))
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	pics := NewPictures(s, queue, bucket)
	NewExtractor(s, queue, bucket, "test")
	StartClassifier(s, queue, bucket, "test")
	StartDigest(s, queue, bucket, "test", func() time.Time { return testNow })
	mail := func(subject string) string {
		return "From: Maya Lindqvist <maya.lindqvist@example.org>\r\nDate: Tue, 29 Sep 2026 15:00:00 +0000\r\nSubject: " + subject + "\r\nList-Id: <parentsandstaff.heliosschool.org>\r\n" +
			"Content-Type: text/html; charset=utf-8\r\n\r\n<p>" + subject + ": sign the slip at https://example.org/slip.</p>\r\n"
	}
	trip := uploadMail(t, s, pics, mail("Field trip"))
	waitFor := func(what string, done func() bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for !done() {
			if time.Now().After(deadline) {
				t.Fatalf("%s: not after three seconds", what)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	read := func(id string) func() bool {
		return func() bool {
			row, _ := s.Model().Table("DOCUMENT").Get(id)
			return row["digest"] == DigestRevision
		}
	}
	waitFor("the field trip read", read(trip))
	doc, _ := s.Model().Table("DOCUMENT").Get(trip)
	if doc["summary"] != "The Jays visit the tide pools and need slips." || doc["key_points"] != "Return the slip by Fri, Oct 2.\nPack a lunch." {
		t.Errorf("the reading: summary %q, points %q", doc["summary"], doc["key_points"])
	}
	toDos := s.Model().Table("TODO").Referencing("document", trip)
	if len(toDos) != 2 {
		t.Fatalf("the field trip has %d to-dos: %v", len(toDos), toDos)
	}
	slip := ""
	for _, td := range toDos {
		switch td["title"] {
		case "Return the tide pool slip":
			slip = td["id"]
			if td["link"] != "https://example.org/slip" || td["due"] != "2026-10-02" || td["point"] != "1" {
				t.Errorf("the slip's to-do: %v", td)
			}
		case "Pay the fee":
			if td["link"] != "" || td["due"] != "" || td["point"] != "" {
				t.Errorf("a link the email lacks, a day that is no date or a point past the points was kept: %v", td)
			}
		default:
			t.Errorf("an unexpected to-do: %v", td)
		}
	}
	mark := func(viewer string, edits []Edit) error {
		_, err := Write(context.Background(), s, queue, access.Actor{Email: "rowan.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
		return err
	}
	if err := mark(parent, []Edit{{Insert: "TODO_MARK", Row: map[string]any{"person": parent, "todo": slip, "state": "done"}}}); err != nil {
		t.Fatalf("a parent marks their to-do done: %v", err)
	}
	if err := mark(parent, []Edit{{Insert: "TODO_MARK", Row: map[string]any{"person": staff, "todo": slip, "state": "done"}}}); err == nil {
		t.Error("a parent marked a to-do for someone else")
	}
	if got := len(as(t, s, student, "(from TODO_MARK)")); got != 0 {
		t.Errorf("their child read %d of a parent's marks", got)
	}
	if got := len(as(t, s, parent, "(from TODO_MARK)")); got != 1 {
		t.Errorf("the parent reads %d of their own marks", got)
	}
	reminder := uploadMail(t, s, pics, mail("Reminder"))
	waitFor("the reminder read", read(reminder))
	doc, _ = s.Model().Table("DOCUMENT").Get(reminder)
	if doc["repeats"] != "1: "+slip {
		t.Errorf("the reminder's repeats: %q, want its point 1 on %s", doc["repeats"], slip)
	}
	if n := len(s.Model().Table("TODO").Referencing("document", reminder)); n != 0 {
		t.Errorf("the reminder kept %d to-dos on a point that repeats the trip's", n)
	}
	answers["Field trip"] = `{"summary": "Again.", "points": ["Return the slip."], "asks": "A slip.", "todos": [{"title": "Return the tide pool slip", "summary": "Signed slip", "details": "Return it.", "link": "", "due": "2026-10-02", "point": 1}], "repeats": []}`
	apply(t, s, queue, []Edit{{Set: trip, Cells: map[string]any{"digest": "2026-01-01"}}})
	waitFor("the field trip read again", func() bool {
		row, _ := s.Model().Table("DOCUMENT").Get(trip)
		return row["digest"] == DigestRevision && row["summary"] == "Again."
	})
	toDos = s.Model().Table("TODO").Referencing("document", trip)
	if len(toDos) != 1 || toDos[0]["id"] != slip || toDos[0]["summary"] != "Signed slip" {
		t.Errorf("after the re-read the trip's to-dos are %v, want the slip's own row, rewritten", toDos)
	}
	if _, ok := s.Model().Table("TODO_MARK").Find(parent, slip); !ok {
		t.Error("the re-read took the parent's mark")
	}
	apply(t, s, queue, []Edit{{Delete: slip}})
	if n := s.Model().Table("TODO_MARK").Len(); n != 0 {
		t.Errorf("%d marks outlived their to-do", n)
	}
}
