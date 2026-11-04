package tools

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/db"
	"heliosian/internal/intercept"
)

func TestReadDocumentFetchesOnlyTheRequestedPage(t *testing.T) {
	dir := &data.Dir{Root: "../../sampledata"}
	s, err := db.NewStore(dir, dir, db.NewSearchIndex(), func(app string) string { return "https://" + app + ".heliosian.com" })
	if err != nil {
		t.Fatal(err)
	}
	intercept.GoogleLogin(t.TempDir())
	objects := map[string]string{}
	var reads, transferred atomic.Int64
	intercept.Install("storage.googleapis.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, name, ok := strings.Cut(r.URL.Path, "/o/")
		text, exists := objects[name]
		if r.Method != http.MethodGet || !ok || !exists {
			http.NotFound(w, r)
			return
		}
		reads.Add(1)
		transferred.Add(int64(len(text)))
		w.Header().Set("x-goog-generation", "1")
		w.Header().Set("Content-Type", "text/markdown")
		fmt.Fprint(w, text)
	}))
	bucket, err := blob.Open("document-test")
	if err != nil {
		t.Fatal(err)
	}
	edits := []db.Edit{{Insert: "DOCUMENT", As: "root", Row: map[string]any{"kind": "file", "name": "Long deck"}}}
	textBodies := []string{}
	for i, size := range []int{59463, 65898, 66011, 42368} {
		text := strings.Repeat(fmt.Sprintf("part %d 週\n", i+1), size/11) + strings.Repeat("x", size%11)
		textBodies = append(textBodies, text)
		hash := blob.Hash([]byte(text))
		name := "content/" + hash
		objects[name] = text
		edits = append(edits,
			db.Edit{Insert: "CONTENT", As: fmt.Sprintf("text%d", i), Row: map[string]any{"hash": hash, "blob": name, "mime": "text/markdown", "size": fmt.Sprint(len(text))}},
			db.Edit{Insert: "DOCUMENT", Row: map[string]any{"parent": "@root", "relation": "extract", "order": string(rune('a' + i)), "content": fmt.Sprintf("@text%d", i)}},
		)
	}
	ids := write(t, s, edits...)
	id := ids[0]
	set := New(Deps{Bucket: bucket, Now: func() time.Time { return sampleNow }})
	var complete strings.Builder
	offset := 0
	for page := 0; ; page++ {
		if page > 20 {
			t.Fatal("pagination did not finish")
		}
		before := reads.Load()
		answer, err := run(t, set, s, "helios_read_document", fmt.Sprintf(`{"id":%q,"offset":%d}`, id, offset))
		if err != nil {
			t.Fatal(err)
		}
		if page == 0 {
			t.Logf("first page: %d text downloads, %d bytes", reads.Load()-before, transferred.Load())
			if reads.Load()-before != 1 {
				t.Fatalf("the first page fetched %d text parts, want 1", reads.Load()-before)
			}
			for _, id := range []string{ids[2], ids[4], ids[6], ids[8]} {
				if !strings.Contains(answer.Text, "- "+id+" extract") {
					t.Fatalf("the first page's outline omitted %s", id)
				}
			}
		}
		if len(answer.Text) > documentText || !utf8.ValidString(answer.Text) {
			t.Fatalf("invalid page: %d bytes", len(answer.Text))
		}
		text, continuation, more := strings.Cut(answer.Text, "\n(more text remains;")
		complete.WriteString(text)
		if !more {
			break
		}
		next := offset + utf8.RuneCountInString(text)
		if next <= offset || !strings.Contains(continuation, fmt.Sprintf("offset %d to continue", next)) {
			t.Fatal("invalid continuation offset")
		}
		offset = next
	}
	wantText := ""
	for i, text := range textBodies {
		wantText += fmt.Sprintf("\n## extract (%s)\n\n%s\n", ids[2+2*i], text)
	}
	if !strings.HasSuffix(complete.String(), wantText) {
		t.Fatal("paginated reading lost, repeated or reordered document text")
	}
	_, err = run(t, set, s, "helios_read_document", fmt.Sprintf(`{"id":%q,"offset":%d}`, id, utf8.RuneCountInString(complete.String())+1))
	if err == nil || !strings.Contains(err.Error(), "offset exceeds") {
		t.Fatalf("an offset beyond the end was not refused: %v", err)
	}
	write(t, s, db.Edit{Set: id, Cells: map[string]any{"name": strings.Repeat("長", documentText)}})
	before := reads.Load()
	answer, err := run(t, set, s, "helios_read_document", fmt.Sprintf(`{"id":%q}`, id))
	if err != nil {
		t.Fatal(err)
	}
	if reads.Load() != before {
		t.Fatal("a page filled by the outline fetched body text")
	}
	if !utf8.ValidString(answer.Text) || !strings.Contains(answer.Text, "more text remains;") {
		t.Fatal("the long outline lost its continuation")
	}
}
