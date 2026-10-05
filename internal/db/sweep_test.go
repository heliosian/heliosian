package db

import (
	"context"
	"testing"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/store"
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
	if err := commit(s, DocumentsSheet,
		store.Insert("CONTENT", store.Row{"id": "cnt00000000001", "hash": "h1", "blob": "content/h1", "mime": "text/html", "size": "9"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000010", "kind": "page", "content": "cnt00000000001"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000002", "hash": "h1", "blob": "content/h1", "mime": "text/plain", "size": "9"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000003", "hash": "h2", "blob": "content/h2", "mime": "text/html", "size": "9"}),
	); err != nil {
		t.Fatal(err)
	}
	StartSweeper(s, queue, bucket)
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		contents := s.Model().Table("CONTENT")
		_, shared := contents.Get("cnt00000000002")
		_, alone := contents.Get("cnt00000000003")
		held, err := bucket.Exists(ctx, "content/h2")
		if err != nil {
			t.Fatal(err)
		}
		if shared || alone || held {
			continue
		}
		if _, ok := contents.Get("cnt00000000001"); !ok {
			t.Fatal("the referenced content was swept")
		}
		if held, err := bucket.Exists(ctx, "content/h1"); err != nil || !held {
			t.Fatalf("the referenced content's object was removed: %v", err)
		}
		return
	}
	t.Fatal("the unreferenced content is still held")
}
