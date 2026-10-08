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
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

func uploadMail(t *testing.T, s *Store, pics *Pictures, eml string) string {
	t.Helper()
	rec := postMail(t, s, pics.queue, pics, "bearer:"+testImportKey, []byte(eml))
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

func TestUnlistedMailIsSentToWhomClaudeReads(t *testing.T) {
	var mu sync.Mutex
	asked := []string{}
	intercept.Install(intercept.ClaudeHost, testkit.ClaudeReplying(func(request string) string {
		mu.Lock()
		defer mu.Unlock()
		switch {
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
	s, queue := sampleWithQueue(t)
	if err := commit(s, GroupsSheet, store.Insert("GROUP", store.Row{"id": "grp00000000033", "kind": "group", "status": "open", "slug": "grade-3-parents", "name": "Grade 3 Parents", "visible_to": "grp00000000004"})); err != nil {
		t.Fatal(err)
	}
	bucket := blob.NewMemoryBucket()
	pics := NewPictures(s, queue, bucket)
	NewExtractor(s, queue, bucket, "test")
	StartClassifier(s, queue, bucket, "test")
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
		if len(sentTo(s, trip)) > 0 && len(sentTo(s, play)) > 0 && len(sentTo(s, picnic)) > 0 && !gone {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not classified after two seconds: trip %v, play %v, picnic %v, lit circle still there %v", sentTo(s, trip), sentTo(s, play), sentTo(s, picnic), gone)
		}
		time.Sleep(10 * time.Millisecond)
	}
	for document, want := range map[string]string{trip: "grp00000000030", play: "grp00000000033", picnic: "grp00000000004", listed: "grp00000000002 grp00000000003"} {
		if got := strings.Join(sentTo(s, document), " "); got != want {
			t.Errorf("%s was sent to %s, want %s", document, got, want)
		}
	}
	for _, row := range s.Model().Table("DOCUMENT").All() {
		if row["parent"] == circle {
			t.Fatalf("a part of the lit circle's email is left: %v", row)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if slices.Contains(asked, "Staff lunch") {
		t.Fatal("mail through a list was sent to Claude")
	}
}
