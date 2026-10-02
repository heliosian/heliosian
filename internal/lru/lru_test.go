package lru

import "testing"

func one(int) int {
	return 1
}

func TestEvictsLeastRecentlyUsed(t *testing.T) {
	c := New[string, int](2, one)
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
	c := New[string, int](2, one)
	c.Put("a", 1)
	c.Put("a", 2)
	c.Put("b", 3)
	if v, ok := c.Get("a"); !ok || v != 2 {
		t.Errorf("a = %d, %v", v, ok)
	}
}

func TestEvictsByWeight(t *testing.T) {
	c := New[string, []byte](10, func(b []byte) int { return len(b) })
	c.Put("a", make([]byte, 4))
	c.Put("b", make([]byte, 4))
	c.Put("c", make([]byte, 4))
	if _, ok := c.Get("a"); ok {
		t.Error("a kept past the budget")
	}
	if _, ok := c.Get("b"); !ok {
		t.Error("b dropped though it fits")
	}
	c.Put("huge", make([]byte, 50))
	if _, ok := c.Get("huge"); !ok {
		t.Error("an entry over the whole budget was not kept on its own")
	}
	if _, ok := c.Get("c"); ok {
		t.Error("c kept beside an entry over the budget")
	}
}
