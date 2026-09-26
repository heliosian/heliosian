package store

import (
	"context"
	"fmt"
	"sync"
	"time"

	"heliosian/internal/data"
)

const refreshInterval = 5 * time.Minute

type Spec[M any] struct {
	App    string
	Tabs   []Tab
	Build  func(context.Context, Tables) (M, error)
	Loaded func(model M, took time.Duration)
}

type Store[M any] struct {
	spec   Spec[M]
	book   *Book
	queue  *Queue
	mu     sync.RWMutex
	tables Tables
	model  M
}

func New[M any](spec Spec[M], source data.Source, writer data.Writer, queue *Queue) (*Store[M], error) {
	book, err := NewBook(spec.App, spec.Tabs, source, writer, queue)
	if err != nil {
		return nil, err
	}
	s := &Store[M]{spec: spec, book: book, queue: queue}
	swap, err := s.load(context.Background())
	if err != nil {
		return nil, err
	}
	swap()
	queue.Register(s.load)
	return s, nil
}

func (s *Store[M]) load(ctx context.Context) (func(), error) {
	start := time.Now()
	tables, model, err := s.read(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.spec.App, err)
	}
	took := time.Since(start)
	return func() {
		s.mu.Lock()
		s.tables, s.model = tables, model
		s.mu.Unlock()
		s.spec.Loaded(model, took)
	}, nil
}

func (s *Store[M]) read(ctx context.Context) (Tables, M, error) {
	var none M
	tables, err := s.book.Read(ctx)
	if err != nil {
		return nil, none, err
	}
	model, err := s.spec.Build(ctx, tables)
	if err != nil {
		return nil, none, err
	}
	return tables, model, nil
}

func (s *Store[M]) Model() M {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.model
}

func (s *Store[M]) Count(tab string, match Row) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, row := range s.tables[tab] {
		if matches(row, match) {
			n++
		}
	}
	return n
}

func (s *Store[M]) Commit(ctx context.Context, actor string, ops ...Op) error {
	_, err := s.commit(ctx, actor, ops)
	return err
}

func (s *Store[M]) CommitAndWait(ctx context.Context, actor string, ops ...Op) error {
	done, err := s.commit(ctx, actor, ops)
	if err != nil || done == nil {
		return err
	}
	<-done
	return nil
}

func (s *Store[M]) commit(ctx context.Context, actor string, ops []Op) (<-chan struct{}, error) {
	s.queue.commits.Lock()
	defer s.queue.commits.Unlock()
	s.mu.RLock()
	tables := s.tables
	s.mu.RUnlock()
	plan, err := s.book.Plan(ctx, tables, actor, ops)
	if err != nil || plan.Empty() {
		return nil, err
	}
	model, err := s.spec.Build(ctx, plan.Tables)
	if err != nil {
		return nil, err
	}
	s.queue.interrupt()
	s.mu.Lock()
	s.tables, s.model = plan.Tables, model
	s.mu.Unlock()
	return s.book.Write(plan), nil
}
