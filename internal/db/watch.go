package db

import (
	"slices"
	"sync"

	"heliosian/internal/store"
)

type watcher func(tx *store.Tx, c Change, env Env)

type watchers struct {
	mu   sync.Mutex
	list []watcher
}

func watch(s *Store, w watcher) {
	ws := s.Model().watchers
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.list = append(ws.list, w)
}

func (m *Model) watched(tx *store.Tx, c Change, env Env) {
	m.watchers.mu.Lock()
	list := slices.Clone(m.watchers.list)
	m.watchers.mu.Unlock()
	for _, w := range list {
		w(tx, c, env)
	}
}
