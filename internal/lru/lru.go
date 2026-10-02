package lru

import (
	"container/list"
	"sync"
)

type entry[K comparable, V any] struct {
	key    K
	value  V
	weight int
}

type Cache[K comparable, V any] struct {
	budget int
	weigh  func(V) int
	mu     sync.Mutex
	used   int
	order  *list.List
	items  map[K]*list.Element
}

func New[K comparable, V any](budget int, weigh func(V) int) *Cache[K, V] {
	return &Cache[K, V]{budget: budget, weigh: weigh, order: list.New(), items: map[K]*list.Element{}}
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
		c.remove(e)
	}
	weight := c.weigh(value)
	c.items[key] = c.order.PushFront(&entry[K, V]{key: key, value: value, weight: weight})
	c.used += weight
	for c.used > c.budget && c.order.Len() > 1 {
		c.remove(c.order.Back())
	}
}

func (c *Cache[K, V]) remove(e *list.Element) {
	it := e.Value.(*entry[K, V])
	c.order.Remove(e)
	delete(c.items, it.key)
	c.used -= it.weight
}
