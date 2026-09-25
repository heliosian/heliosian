package lru

import (
	"container/list"
	"sync"
)

type entry[K comparable, V any] struct {
	key   K
	value V
}

type Cache[K comparable, V any] struct {
	size  int
	mu    sync.Mutex
	order *list.List
	items map[K]*list.Element
}

func New[K comparable, V any](size int) *Cache[K, V] {
	return &Cache[K, V]{size: size, order: list.New(), items: map[K]*list.Element{}}
}

func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	c.order.MoveToFront(e)
	return e.Value.(*entry[K, V]).value, true
}

func (c *Cache[K, V]) Put(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		e.Value.(*entry[K, V]).value = value
		c.order.MoveToFront(e)
		return
	}
	c.items[key] = c.order.PushFront(&entry[K, V]{key: key, value: value})
	if c.order.Len() > c.size {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.items, oldest.Value.(*entry[K, V]).key)
	}
}
