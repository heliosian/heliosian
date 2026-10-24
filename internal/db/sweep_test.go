package db

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/store"
)

func TestUnreferencedContentIsSwept(t *testing.T) {
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	ctx := context.Background()
	for _, name := range []string{"content/h1", "content/h2"} {
		if _, err := bucket.Put(ctx, name, "text/html", []byte("<p>hi</p>"), 0); err != nil {
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
		if shared || alone {
			continue
		}
		x := &sweeper{s: s, queue: queue, bucket: bucket}
		if _, err := x.sweepObjects(ctx, time.Now().Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if held, err := bucket.Exists(ctx, "content/h2"); err != nil || held {
			t.Fatalf("the swept content's object is still held: %v", err)
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

func TestBytesARowHoldsAreNotRewritten(t *testing.T) {
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	ctx := context.Background()
	body := []byte("<p>held</p>")
	hash := blob.Hash(body)
	first, err := bucket.Put(ctx, "content/"+hash, "text/html", body, 0)
	if err != nil {
		t.Fatal(err)
	}
	apply(t, s, queue, []Edit{{Insert: "CONTENT", Row: map[string]any{"hash": hash, "blob": "content/" + hash, "mime": "text/html", "size": "11"}}})
	if _, err := keepBytes(ctx, s.Model(), bucket, "text/html", body); err != nil {
		t.Fatal(err)
	}
	if now, err := bucket.Generation(ctx, "content/"+hash); err != nil || now != first {
		t.Fatalf("held bytes were written again: generation %d, was %d: %v", now, first, err)
	}
}

func TestStoringBytesNoRowHoldsBeatsASweepThatListedThem(t *testing.T) {
	s, _ := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	ctx := context.Background()
	body := []byte("<p>left behind</p>")
	name := "content/" + blob.Hash(body)
	if _, err := bucket.Put(ctx, name, "text/html", body, 0); err != nil {
		t.Fatal(err)
	}
	listed, err := bucket.Older(ctx, "content/", time.Now().Add(time.Second))
	if err != nil || len(listed) != 1 {
		t.Fatalf("the sweep lists %v: %v", listed, err)
	}
	if _, err := keepBytes(ctx, s.Model(), bucket, "text/html", body); err != nil {
		t.Fatal(err)
	}
	if err := bucket.Remove(ctx, listed[0].Name, listed[0].Generation); !errors.Is(err, blob.ErrChanged) {
		t.Fatalf("the sweep's delete at the listed generation: %v", err)
	}
	if held, err := bucket.Exists(ctx, name); err != nil || !held {
		t.Fatalf("the stored bytes are gone: %v", err)
	}
}

func TestARowSweptWhileItsBytesAreReusedIsNotMadeAgain(t *testing.T) {
	s, queue := sampleWithQueue(t)
	bucket := blob.NewMemoryBucket()
	ctx := context.Background()
	body := []byte("<p>reused</p>")
	hash := blob.Hash(body)
	if _, err := bucket.Put(ctx, "content/"+hash, "text/html", body, 0); err != nil {
		t.Fatal(err)
	}
	ids := apply(t, s, queue, []Edit{{Insert: "CONTENT", Row: map[string]any{"hash": hash, "blob": "content/" + hash, "mime": "text/html", "size": "13"}}})
	kept, err := keepBytes(ctx, s.Model(), bucket, "text/html", body)
	if err != nil || kept.written {
		t.Fatalf("bytes a row holds were written again: %v %v", kept, err)
	}
	apply(t, s, queue, []Edit{{Delete: ids[0]}})
	stage := func(b madeBytes) error {
		_, err := queue.Transact(ctx, access.System("test"), func(tx *store.Tx) error {
			_, err := stageContent(s.In(tx), &pendingWrites{}, map[string]string{}, b)
			return err
		})
		return err
	}
	if err := stage(kept); err == nil {
		t.Fatal("a swept row was made again over bytes the sweep may take")
	}
	x := &sweeper{s: s, queue: queue, bucket: bucket}
	if removed, err := x.sweepObjects(ctx, time.Now().Add(time.Second)); err != nil || removed != 1 {
		t.Fatalf("the sweep removed %d objects: %v", removed, err)
	}
	again, err := keepBytes(ctx, s.Model(), bucket, "text/html", body)
	if err != nil || !again.written {
		t.Fatalf("storing again after the sweep wrote nothing: %v %v", again, err)
	}
	if err := stage(again); err != nil {
		t.Fatal(err)
	}
	if held, err := bucket.Exists(ctx, "content/"+hash); err != nil || !held {
		t.Fatalf("the bytes stored again are gone: %v", err)
	}
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
