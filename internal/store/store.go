package store

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/data"
)

const refreshInterval = 5 * time.Minute

type Part[M any] struct {
	App    string
	Tabs   []Tab
	Reads  []string
	Build  func(ctx context.Context, tables Tables, m *M) error
	Loaded func(m *M, took time.Duration)
}

type Store[M any] struct {
	parts   []Part[M]
	consent func(m *M) error
	books   map[string]*Book
	queue   *Queue
	mu      sync.RWMutex
	tables  map[string]Tables
	model   *M
}

func New[M any](parts []Part[M], consent func(m *M) error, source data.Source, writer data.Writer, queue *Queue) (*Store[M], error) {
	ordered, err := order(parts)
	if err != nil {
		return nil, err
	}
	s := &Store[M]{parts: ordered, consent: consent, books: map[string]*Book{}, queue: queue}
	for _, p := range ordered {
		book, err := NewBook(p.App, p.Tabs, source, writer, queue)
		if err != nil {
			return nil, err
		}
		s.books[p.App] = book
	}
	swap, err := s.load(context.Background())
	if err != nil {
		return nil, err
	}
	swap()
	queue.Register(s.load)
	return s, nil
}

func order[M any](parts []Part[M]) ([]Part[M], error) {
	byApp := map[string]Part[M]{}
	for _, p := range parts {
		if _, ok := byApp[p.App]; ok {
			return nil, fmt.Errorf("part %s declared twice", p.App)
		}
		byApp[p.App] = p
	}
	out := []Part[M]{}
	placed, visiting := map[string]bool{}, map[string]bool{}
	var place func(app string) error
	place = func(app string) error {
		if placed[app] {
			return nil
		}
		if visiting[app] {
			return fmt.Errorf("part %s reads itself", app)
		}
		p, ok := byApp[app]
		if !ok {
			return fmt.Errorf("no part %s", app)
		}
		visiting[app] = true
		for _, read := range p.Reads {
			if err := place(read); err != nil {
				return fmt.Errorf("%s: %w", app, err)
			}
		}
		placed[app] = true
		out = append(out, p)
		return nil
	}
	for _, p := range parts {
		if err := place(p.App); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store[M]) load(ctx context.Context) (func(), error) {
	tables := map[string]Tables{}
	took := map[string]time.Duration{}
	for _, p := range s.parts {
		start := time.Now()
		read, err := s.books[p.App].Read(ctx)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.App, err)
		}
		tables[p.App] = read
		took[p.App] = time.Since(start)
	}
	model, err := s.build(ctx, tables, new(M), nil, took)
	if err != nil {
		return nil, err
	}
	return func() {
		s.mu.Lock()
		s.tables, s.model = tables, model
		s.mu.Unlock()
		for _, p := range s.parts {
			p.Loaded(model, took[p.App])
		}
	}, nil
}

func (s *Store[M]) build(ctx context.Context, tables map[string]Tables, base *M, changed map[string]bool, took map[string]time.Duration) (*M, error) {
	next := *base
	for _, p := range s.parts {
		if changed != nil && !changed[p.App] && !slices.ContainsFunc(p.Reads, func(read string) bool { return changed[read] }) {
			continue
		}
		start := time.Now()
		if err := p.Build(ctx, tables[p.App], &next); err != nil {
			if changed == nil {
				return nil, fmt.Errorf("%s: %w", p.App, err)
			}
			return nil, err
		}
		if took != nil {
			took[p.App] += time.Since(start)
		}
		if changed != nil {
			changed[p.App] = true
		}
	}
	if err := s.consent(&next); err != nil {
		return nil, fmt.Errorf("consent: %w", err)
	}
	return &next, nil
}

func (s *Store[M]) Model() *M {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.model
}

func (s *Store[M]) Count(app, tab string, match Row) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, row := range s.tables[app][tab] {
		if data.Matches(row, match) {
			n++
		}
	}
	return n
}

func (s *Store[M]) Commit(ctx context.Context, actor access.Actor, app string, ops ...Op) error {
	_, err := s.commit(ctx, actor, app, ops)
	return err
}

func (s *Store[M]) CommitAndWait(ctx context.Context, actor access.Actor, app string, ops ...Op) error {
	done, err := s.commit(ctx, actor, app, ops)
	if err != nil || done == nil {
		return err
	}
	<-done
	return nil
}

func (s *Store[M]) commit(ctx context.Context, actor access.Actor, app string, ops []Op) (<-chan struct{}, error) {
	return s.queue.Transact(ctx, actor, func(tx *Tx) error { return s.Stage(tx, app, ops...) })
}

func (s *Store[M]) current() (map[string]Tables, *M) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tables, s.model
}

func (s *Store[M]) Stage(tx *Tx, app string, ops ...Op) error {
	book, ok := s.books[app]
	if !ok {
		return fmt.Errorf("no sheet %s", app)
	}
	st := tx.stageOf(s)
	tables, base := map[string]Tables(nil), (*M)(nil)
	if st != nil {
		tables, base = st.tables, st.model.(*M)
	} else {
		tables, base = s.current()
	}
	plan, err := book.Plan(tx.ctx, tables[app], tx.actor.Email, ops)
	if err != nil || plan.Empty() {
		return err
	}
	next := maps.Clone(tables)
	next[app] = plan.Tables
	model, err := s.build(tx.ctx, next, base, map[string]bool{app: true}, nil)
	if err != nil {
		return access.Invalid("%v", err)
	}
	if st == nil {
		st = &stage{owner: s, plans: map[string]Plan{}}
		tx.staged = append(tx.staged, st)
	}
	if _, ok := st.plans[app]; !ok {
		st.order = append(st.order, app)
	}
	was := st.plans[app]
	st.plans[app] = Plan{Tables: plan.Tables, writes: append(was.writes, plan.writes...), log: append(was.log, plan.log...)}
	st.tables, st.model = next, model
	st.swap = func() {
		s.mu.Lock()
		s.tables, s.model = next, model
		s.mu.Unlock()
	}
	st.write = func() <-chan struct{} {
		var done <-chan struct{}
		for _, app := range st.order {
			done = s.books[app].Write(st.plans[app])
		}
		return done
	}
	return nil
}

func (s *Store[M]) In(tx *Tx) *M {
	if tx != nil {
		if st := tx.stageOf(s); st != nil {
			return st.model.(*M)
		}
	}
	return s.Model()
}
