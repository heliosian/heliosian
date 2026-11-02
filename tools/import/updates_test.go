package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/db"
	"heliosian/internal/tools"
)

func TestLongImportedDeckCanBeReadInFull(t *testing.T) {
	c, s, bucket := sampleServerWithBucket(t)
	importOnce(t, c, sampleExport(t, false))
	groups, _, err := c.table("GROUP")
	if err != nil {
		t.Fatal(err)
	}
	first := strings.Repeat("週", 30000)
	last := "\n# Last slide\n[Camping](https://example.org/camping)"
	updates := []classroomUpdate{{Classroom: "Hummingbirds", DeckURL: "https://docs.google.com/presentation/d/long/edit", Title: "Long updates", text: []byte(first + last)}}
	uploads, err := c.updatePlan(updates, groups)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.addUpdate(uploads[0]); err != nil {
		t.Fatal(err)
	}
	documents, _, err := c.table("DOCUMENT", in("url", updates[0].DeckURL))
	if err != nil {
		t.Fatal(err)
	}
	id := documents.order[0]
	set := tools.New(tools.Deps{Bucket: bucket, Now: time.Now})
	read := func(offset int) string {
		t.Helper()
		input := json.RawMessage(fmt.Sprintf("{\"id\":%q,\"offset\":%d}", id, offset))
		answer, err := set.Run(t.Context(), s.Model(), "per00000000002", "helios_read_document", input)
		if err != nil {
			t.Fatal(err)
		}
		return answer.Text
	}
	var complete strings.Builder
	offset := 0
	for pageNumber := 0; ; pageNumber++ {
		if pageNumber > 10 {
			t.Fatal("document pagination never finished")
		}
		page := read(offset)
		if len(page) > 30000 {
			t.Fatalf("page has %d bytes", len(page))
		}
		body, continuation, more := strings.Cut(page, "\n(more text remains;")
		complete.WriteString(body)
		if !more {
			break
		}
		next := offset + len([]rune(body))
		if next <= offset || !strings.Contains(continuation, fmt.Sprintf("offset %d to continue", next)) {
			t.Fatal("continuation offset does not match the returned characters")
		}
		offset = next
	}
	if !strings.Contains(complete.String(), first+last) {
		t.Fatal("paginated reading lost slide text or links")
	}
}

func TestClassroomDeckVersionsAndAudience(t *testing.T) {
	c, s := sampleServer(t)
	x := sampleExport(t, false)
	importOnce(t, c, x)
	groups, _, err := c.table("GROUP")
	if err != nil {
		t.Fatal(err)
	}
	updates := []classroomUpdate{{Classroom: "Hummingbirds", DeckURL: "https://docs.google.com/presentation/d/hummingbirds/edit", Title: "Hummingbirds updates", text: []byte("# Week 1\n[Camping](https://example.org/camping)")}}
	plan := func() []updateUpload {
		t.Helper()
		p, err := c.updatePlan(updates, groups)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	first := plan()
	if len(first) != 1 || len(first[0].SentTo) != 2 || first[0].Supersedes != "" {
		t.Fatalf("first import: %+v", first)
	}
	if err := c.addUpdate(first[0]); err != nil {
		t.Fatal(err)
	}
	if again := plan(); len(again) != 0 {
		t.Fatalf("unchanged import: %+v", again)
	}
	documents, _, err := c.table("DOCUMENT", in("url", updates[0].DeckURL))
	if err != nil || len(documents.order) != 1 {
		t.Fatalf("documents: %+v, %v", documents, err)
	}
	original := documents.order[0]
	d := documents.rows[original]
	if d["published"] != "" || d["name"] != updates[0].Title {
		t.Fatalf("metadata: %v", d)
	}
	input, ok := s.Model().SearchInputs(map[string]string{d["content"]: string(updates[0].text)})[original]
	if !ok || !strings.Contains(input.Input, string(updates[0].text)) || input.Href != updates[0].DeckURL {
		t.Fatalf("imported deck missing from search corpus: %+v", input)
	}
	content, _ := s.Model().Table("CONTENT").Get(d["content"])
	if content["mime"] != "text/markdown" {
		t.Fatalf("content: %v", content)
	}
	for _, group := range first[0].SentTo {
		found := false
		for _, link := range s.Model().Table("DOCUMENT_GROUP").Referencing("document", original) {
			found = found || link["group"] == group && link["relation"] == "sent_to"
		}
		if !found {
			t.Fatalf("no audience link to %s", group)
		}
	}
	updates[0].text = []byte("# Week 2\nNew update\n# Week 1\n[Camping](https://example.org/camping)")
	second := plan()
	if len(second) != 1 || second[0].Supersedes != original {
		t.Fatalf("changed import: %+v", second)
	}
	if err := c.addUpdate(second[0]); err != nil {
		t.Fatal(err)
	}
	old, _ := s.Model().Table("DOCUMENT").Get(original)
	if old["superseded_by"] == "" || old["content"] != d["content"] {
		t.Fatalf("history not retained: %v", old)
	}
	if again := plan(); len(again) != 0 {
		t.Fatalf("second import was not idempotent: %+v", again)
	}
	updates[0].text = first[0].text
	if err := c.addUpdate(plan()[0]); err != nil {
		t.Fatal(err)
	}
	restored, _ := s.Model().Table("DOCUMENT").Get(original)
	if restored["superseded_by"] != "" || len(plan()) != 0 {
		t.Fatalf("reversion: %v", restored)
	}
	other := first[0]
	other.URL = "https://docs.google.com/presentation/d/another/edit"
	if err := c.addUpdate(other); err != nil {
		t.Fatal(err)
	}
	all, _, err := c.table("DOCUMENT", in("url", other.URL))
	if err != nil || len(all.order) != 1 || all.order[0] == original {
		t.Fatalf("different source reused the original: %+v, %v", all, err)
	}
	before := len(s.Model().Table("DOCUMENT").All())
	invalid := first[0]
	invalid.URL = "https://example.org/invalid"
	invalid.SentTo = []string{"grp99999999999"}
	if err := c.addUpdate(invalid); err == nil {
		t.Fatal("unknown audience accepted")
	}
	if len(s.Model().Table("DOCUMENT").All()) != before {
		t.Fatal("refused audience left a document behind")
	}
	_, err = db.Write(context.Background(), s, access.System("test"), db.Env{System: "test"}, db.Batch{Batch: []db.Edit{{Insert: "DOCUMENT", Row: map[string]any{"kind": "file", "url": updates[0].DeckURL, "content": d["content"]}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.updatePlan(updates, groups); err == nil {
		t.Fatal("ambiguous current versions accepted")
	}
}
