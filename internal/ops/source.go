package ops

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"heliosian/internal/vitals"
)

const fetchTimeout = 2 * time.Minute

type Source[T any] struct {
	name    string
	fetch   func(context.Context) (T, error)
	mu      sync.Mutex
	value   T
	fetched time.Time
	err     string
	running bool
	again   bool
}

type Reading[T any] struct {
	Value   T         `json:"value"`
	Fetched time.Time `json:"fetched"`
	Error   string    `json:"error,omitempty"`
}

func newSource[T any](name string, empty T, fetch func(context.Context) (T, error)) *Source[T] {
	return &Source[T]{name: name, fetch: fetch, value: empty}
}

func (s *Source[T]) Refresh() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		s.again = true
		return
	}
	s.running = true
	go s.run()
}

func (s *Source[T]) run() {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	start := time.Now()
	value, err := s.fetch(ctx)
	cancel()
	if err != nil {
		slog.Error("ops: fetch", "source", s.name, "error", err)
	} else {
		slog.Info("ops: fetched", "source", s.name, "took", time.Since(start))
	}
	s.mu.Lock()
	s.fetched = time.Now()
	s.err = ""
	if err != nil {
		s.err = err.Error()
	} else {
		s.value = value
	}
	again := s.again
	s.again = false
	s.running = again
	s.mu.Unlock()
	vitals.Poke()
	if again {
		s.run()
	}
}

func (s *Source[T]) freshen(age time.Duration) {
	s.mu.Lock()
	stale := !s.running && (s.err != "" || time.Since(s.fetched) > age)
	s.mu.Unlock()
	if stale {
		s.Refresh()
	}
}

func (s *Source[T]) retry() {
	s.mu.Lock()
	failed := !s.running && s.err != ""
	s.mu.Unlock()
	if failed {
		s.Refresh()
	}
}

func (s *Source[T]) Read() Reading[T] {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Reading[T]{Value: s.value, Fetched: s.fetched, Error: s.err}
}
