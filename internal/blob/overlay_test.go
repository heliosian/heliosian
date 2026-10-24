package blob

import (
	"errors"
	"slices"
	"testing"
)

func TestOverlayReadsThroughAndNeverWritesTheBase(t *testing.T) {
	ctx := t.Context()
	base := NewMemoryBucket()
	for _, name := range []string{"content/a", "content/b"} {
		if _, err := base.Put(ctx, name, "text/plain", []byte("base "+name), 0); err != nil {
			t.Fatal(err)
		}
	}
	o := Overlay(base)
	if got, _, err := o.Get(ctx, "content/a"); err != nil || string(got) != "base content/a" {
		t.Fatalf("a read through: %q %v", got, err)
	}
	if _, err := o.Put(ctx, "content/a", "text/plain", []byte("mine"), 0); !errors.Is(err, ErrChanged) {
		t.Fatalf("a write over the base's a as new: %v", err)
	}
	a, err := o.Generation(ctx, "content/a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.Put(ctx, "content/a", "text/plain", []byte("mine"), a); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Put(ctx, "content/c", "text/plain", []byte("new"), 0); err != nil {
		t.Fatal(err)
	}
	b, err := o.Generation(ctx, "content/b")
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Remove(ctx, "content/b", b); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := o.Get(ctx, "content/a"); string(got) != "mine" {
		t.Errorf("the overlay's own write reads %q", got)
	}
	if _, _, err := o.Get(ctx, "content/b"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a removed object still reads: %v", err)
	}
	if held, _ := o.Exists(ctx, "content/b"); held {
		t.Error("a removed object still exists")
	}
	listed, _ := o.List(ctx, "content/")
	names := []string{}
	for _, v := range listed {
		names = append(names, v.Name)
	}
	if !slices.Equal(names, []string{"content/a", "content/c"}) {
		t.Errorf("the overlay lists %v", names)
	}
	if got, _, _ := base.Get(ctx, "content/a"); string(got) != "base content/a" {
		t.Errorf("the base's a became %q", got)
	}
	if held, _ := base.Exists(ctx, "content/b"); !held {
		t.Error("the base lost b")
	}
	if held, _ := base.Exists(ctx, "content/c"); held {
		t.Error("the base gained c")
	}
	if _, err := o.Put(ctx, "content/b", "text/plain", []byte("back"), 0); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := o.Get(ctx, "content/b"); string(got) != "back" {
		t.Errorf("b written again reads %q", got)
	}
}

func TestWritesAndDeletesNeedTheGenerationTheyExpect(t *testing.T) {
	ctx := t.Context()
	b := NewMemoryBucket()
	first, err := b.Put(ctx, "content/a", "text/plain", []byte("one"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Put(ctx, "content/a", "text/plain", []byte("two"), 0); !errors.Is(err, ErrChanged) {
		t.Errorf("a create over an existing object: %v", err)
	}
	second, err := b.Put(ctx, "content/a", "text/plain", []byte("two"), first)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Remove(ctx, "content/a", first); !errors.Is(err, ErrChanged) {
		t.Errorf("a delete at a generation since rewritten: %v", err)
	}
	if err := b.Remove(ctx, "content/a", second); err != nil {
		t.Fatal(err)
	}
	if err := b.Remove(ctx, "content/a", second); !errors.Is(err, ErrNotFound) {
		t.Errorf("a delete of a missing object: %v", err)
	}
}
