package lru

import "testing"

func TestEvictsLeastRecentlyUsed(t *testing.T) {
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("a = %d, %v", v, ok)
	}
	c.Put("c", 3)
	if _, ok := c.Get("b"); ok {
		t.Error("b kept past the size")
	}
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Errorf("a = %d, %v after c", v, ok)
	}
	if v, ok := c.Get("c"); !ok || v != 3 {
		t.Errorf("c = %d, %v", v, ok)
	}
}

func TestPutReplaces(t *testing.T) {
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("a", 2)
	c.Put("b", 3)
	if v, ok := c.Get("a"); !ok || v != 2 {
		t.Errorf("a = %d, %v", v, ok)
	}
}
