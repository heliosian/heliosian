package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"heliosian/internal/blob"
)

func TestUnreferencedContentIsSwept(t *testing.T) {
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	ctx := context.Background()
	for _, name := range []string{"content/h1", "content/h2"} {
		if err := bucket.Put(ctx, name, "text/html", []byte("<p>hi</p>")); err != nil {
			t.Fatal(err)
		}
	}
	ids := apply(t, s, queue, []Edit{
		{Insert: "CONTENT", As: "referenced", Row: map[string]any{"hash": "h1", "blob": "content/h1", "mime": "text/html", "size": "9"}},
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "file", "content": "@referenced"}},
		{Insert: "CONTENT", Row: map[string]any{"hash": "h1", "blob": "content/h1", "mime": "text/plain", "size": "9"}},
		{Insert: "CONTENT", Row: map[string]any{"hash": "h2", "blob": "content/h2", "mime": "text/html", "size": "9"}},
	})
	referenced, sharing, unshared := ids[0], ids[2], ids[3]
	StartSweeper(s, queue, bucket)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		contents := s.Model().Table("CONTENT")
		_, shared := contents.Get(sharing)
		_, alone := contents.Get(unshared)
		held, err := bucket.Exists(ctx, "content/h2")
		if err != nil {
			t.Fatal(err)
		}
		if shared || alone || held {
			continue
		}
		if _, ok := contents.Get(referenced); !ok {
			t.Fatal("the referenced content was swept")
		}
		if held, err := bucket.Exists(ctx, "content/h1"); err != nil || !held {
			t.Fatalf("the referenced content's object was removed: %v", err)
		}
		return
	}
	t.Fatal("the unreferenced content is still held")
}

func TestContentIsSweptInBatches(t *testing.T) {
	s, queue := sampleWithQueue(t)
	held := s.Model().Table("CONTENT").Len()
	edits := []Edit{}
	for i := range sweepBatch*2 + 3 {
		hash := fmt.Sprintf("h%d", i)
		edits = append(edits, Edit{Insert: "CONTENT", Row: map[string]any{"hash": hash, "blob": "content/" + hash, "mime": "text/html", "size": "9"}})
	}
	apply(t, s, queue, edits)
	StartSweeper(s, queue, blob.NewMemoryBucket())
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if s.Model().Table("CONTENT").Len() == held {
			return
		}
	}
	t.Fatalf("%d content rows are left, want the %d the sample's documents hold", s.Model().Table("CONTENT").Len(), held)
}
