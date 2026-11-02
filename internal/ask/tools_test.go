package ask

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/db"
	"heliosian/internal/tools"
)

func TestReadLongUnicodeDocumentThroughAsk(t *testing.T) {
	dir := &data.Dir{Root: "../../sampledata"}
	s, err := db.NewStore(dir, dir, db.NewSearchIndex(), origin)
	if err != nil {
		t.Fatal(err)
	}
	bucket := blob.NewMemoryBucket()
	text := strings.Repeat("週", 30000) + "\n# Last slide\n[Camping](https://example.org/camping)"
	hash := blob.Hash([]byte(text))
	object := "content/" + hash
	if _, err := bucket.Put(t.Context(), object, "text/markdown", []byte(text), 0); err != nil {
		t.Fatal(err)
	}
	env := db.Env{System: "test", Now: sampleNow}
	ids, err := db.Write(t.Context(), s, access.System("test"), env, db.Batch{Batch: []db.Edit{
		{Insert: "CONTENT", As: "text", Row: map[string]any{"hash": hash, "blob": object, "mime": "text/markdown", "size": fmt.Sprint(len(text))}},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "file", "name": strings.Repeat("更新", 10000), "url": "https://docs.google.com/presentation/d/long/edit", "content": "@text"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	id := ids[1]
	set := tools.New(tools.Deps{Bucket: bucket, Now: func() time.Time { return sampleNow }})
	tr := &turn{m: s.Model(), env: db.Env{Viewer: "per00000000002", Now: sampleNow}, sources: Sources{Tools: set}, found: newFound()}
	var complete strings.Builder
	offset := 0
	for pageNumber := 0; ; pageNumber++ {
		if pageNumber > 20 {
			t.Fatal("pagination did not finish")
		}
		input := json.RawMessage(fmt.Sprintf("{\"id\":%q,\"offset\":%d}", id, offset))
		page, err := tr.run(t.Context(), "helios_read_document", input)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) > maxToolOutput || !utf8.ValidString(page) {
			t.Fatalf("invalid page: %d bytes", len(page))
		}
		body, continuation, more := strings.Cut(page, "\n(more text remains;")
		complete.WriteString(body)
		if !more {
			break
		}
		next := offset + utf8.RuneCountInString(body)
		if next <= offset || !strings.Contains(continuation, fmt.Sprintf("offset %d to continue", next)) {
			t.Fatal("invalid continuation offset")
		}
		offset = next
	}
	if !strings.Contains(complete.String(), text) {
		t.Fatal("Ask did not receive the complete Markdown")
	}
}
