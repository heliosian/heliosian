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
		if err := base.Put(ctx, name, "text/plain", []byte("base "+name)); err != nil {
			t.Fatal(err)
		}
	}
	o := Overlay(base)
	if got, _, err := o.Get(ctx, "content/a"); err != nil || string(got) != "base content/a" {
		t.Fatalf("a read through: %q %v", got, err)
	}
	if err := o.Put(ctx, "content/a", "text/plain", []byte("mine")); err != nil {
		t.Fatal(err)
	}
	if err := o.Put(ctx, "content/c", "text/plain", []byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := o.Remove(ctx, "content/b"); err != nil {
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
	if listed, _ := o.List(ctx, "content/"); !slices.Equal(listed, []string{"content/a", "content/c"}) {
		t.Errorf("the overlay lists %v", listed)
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
	if err := o.Put(ctx, "content/b", "text/plain", []byte("back")); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := o.Get(ctx, "content/b"); string(got) != "back" {
		t.Errorf("b written again reads %q", got)
	}
}
