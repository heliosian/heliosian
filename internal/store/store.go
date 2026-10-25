package store

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/data"
	"heliosian/internal/trace"
)

const (
	refreshInterval = 30 * time.Minute
	tickInterval    = 5 * time.Minute
)

type Part[M any] struct {
	App       string
	Tabs      []Tab
	Changes   *ChangeLog
	Build     func(ctx context.Context, tables Tables, m *M) error
	Loaded    func(m *M, took time.Duration)
	Refreshed func(tab string, before, after Row)
}

type Store[M any] struct {
	parts  []Part[M]
	finish func(ctx context.Context, m *M) error
	check  func(ctx context.Context, base, m *M) error
	books  map[string]*Book
	queue  *Queue
	mu     sync.RWMutex
	tables map[string]Tables
	model  *M
}

func New[M any](parts []Part[M], finish func(ctx context.Context, m *M) error, check func(ctx context.Context, base, m *M) error, source data.Source, writer data.Writer, queue *Queue) (*Store[M], error) {
	s := &Store[M]{parts: parts, finish: finish, check: check, books: map[string]*Book{}, queue: queue}
	for _, p := range parts {
		book, err := NewBook(p.App, p.Tabs, p.Changes, source, writer, queue)
		if err != nil {
			return nil, err
		}
		s.books[p.App] = book
	}
	settle, err := s.load(context.Background())
	if err != nil {
		return nil, err
	}
	swap, err := settle()
	if err != nil {
		return nil, err
	}
	swap()
	queue.Register(s.load)
	return s, nil
}

func (s *Store[M]) load(ctx context.Context) (Settle, error) {
	held, base := s.current()
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
	replayed := map[string]int{}
	for app, book := range s.books {
		plans := book.unwritten()
		replayed[app] = len(plans)
		tables[app] = book.replay(tables[app], plans)
	}
	stale := s.stale(held, tables)
	model := base
	if base == nil || len(stale) == len(s.parts) {
		built, err := s.build(ctx, tables, new(M), nil, took)
		if err != nil {
			return nil, err
		}
		if err := s.check(ctx, nil, built); err != nil {
			return nil, err
		}
		model = built
	} else if len(stale) > 0 {
		built, err := s.build(ctx, tables, base, stale, took)
		if err != nil {
			return nil, err
		}
		if err := s.check(ctx, base, built); err != nil {
			return nil, err
		}
		model = built
	}
	return func() (func(), error) {
		next, final := maps.Clone(tables), model
		changed := map[string]bool{}
		for app, book := range s.books {
			plans := book.unwritten()[replayed[app]:]
			if len(plans) == 0 {
				continue
			}
			next[app] = book.replay(next[app], plans)
			changed[app] = true
		}
		if len(changed) > 0 {
			rebuilt, err := s.build(ctx, next, model, changed, nil)
			if err != nil {
				return nil, err
			}
			if err := s.check(ctx, model, rebuilt); err != nil {
				return nil, err
			}
			final = rebuilt
		}
		return func() {
			s.mu.Lock()
			before := s.tables
			s.tables, s.model = next, final
			s.mu.Unlock()
			for _, p := range s.parts {
				p.Loaded(final, took[p.App])
			}
			if before != nil {
				s.refreshed(before, next, stale)
			}
		}, nil
	}, nil
}

func (s *Store[M]) stale(held, read map[string]Tables) map[string]bool {
	out := map[string]bool{}
	for _, p := range s.parts {
		if !sameTables(held[p.App], read[p.App]) {
			out[p.App] = true
		}
	}
	return out
}

func sameTables(a, b Tables) bool {
	return maps.EqualFunc(a, b, func(x, y []Row) bool {
		return slices.EqualFunc(x, y, func(r, q Row) bool { return maps.Equal(r, q) })
	})
}

func (s *Store[M]) refreshed(before, after map[string]Tables, edited map[string]bool) {
	for _, p := range s.parts {
		if p.Refreshed == nil || !edited[p.App] {
			continue
		}
		changed := 0
		for _, tab := range p.Tabs {
			was := byKey(tab, before[p.App][tab.Name])
			is := byKey(tab, after[p.App][tab.Name])
			for _, key := range slices.Sorted(maps.Keys(was)) {
				if now, ok := is[key]; !ok || !maps.Equal(was[key], now) {
					p.Refreshed(tab.Name, was[key], now)
					changed++
				}
			}
			for _, key := range slices.Sorted(maps.Keys(is)) {
				if _, ok := was[key]; !ok {
					p.Refreshed(tab.Name, nil, is[key])
					changed++
				}
			}
		}
		if changed > 0 {
			slog.Info("refresh changed rows", "app", p.App, "rows", changed)
		}
	}
}

func byKey(tab Tab, rows []Row) map[string]Row {
	out := map[string]Row{}
	for _, row := range rows {
		parts := []string{}
		for _, k := range tab.Key {
			parts = append(parts, row[k])
		}
		out[strings.Join(parts, "\x00")] = row
	}
	return out
}

func (s *Store[M]) build(ctx context.Context, tables map[string]Tables, base *M, changed map[string]bool, took map[string]time.Duration) (*M, error) {
	next := *base
	building := trace.From(ctx).Tally("build")
	for _, p := range s.parts {
		if changed != nil && !changed[p.App] {
			continue
		}
		start := time.Now()
		part := building.Tally(p.App)
		err := p.Build(trace.With(ctx, part), tables[p.App], &next)
		part.Add(time.Since(start))
		if err != nil {
			if changed == nil {
				return nil, fmt.Errorf("%s: %w", p.App, err)
			}
			return nil, err
		}
		if took != nil {
			took[p.App] += time.Since(start)
		}
	}
	start := time.Now()
	finishing := building.Tally("finish")
	err := s.finish(trace.With(ctx, finishing), &next)
	finishing.Add(time.Since(start))
	if err != nil {
		return nil, err
	}
	return &next, nil
}

func (s *Store[M]) Model() *M {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.model
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
	if st == nil {
		tables, model := s.current()
		st = &stage{owner: s, start: model, tables: tables, model: model, dirty: map[string]bool{}, plans: map[string]Plan{}}
		st.settle = func() error {
			final, err := s.Built(tx)
			if err != nil {
				return err
			}
			return s.check(tx.ctx, st.start.(*M), final)
		}
		st.swap = func() {
			s.mu.Lock()
			s.tables, s.model = st.tables, st.model.(*M)
			s.mu.Unlock()
		}
		st.write = func() <-chan struct{} {
			var done <-chan struct{}
			for _, app := range st.order {
				done = s.books[app].Write(st.plans[app])
			}
			return done
		}
		tx.staged = append(tx.staged, st)
	}
	start := time.Now()
	plan, err := book.Plan(tx.ctx, st.tables[app], tx.actor.Email, ops)
	trace.From(tx.ctx).Tally("plan").Add(time.Since(start))
	if err != nil || plan.Empty() {
		return err
	}
	next := maps.Clone(st.tables)
	next[app] = plan.Tables
	if _, ok := st.plans[app]; !ok {
		st.order = append(st.order, app)
	}
	was := st.plans[app]
	st.plans[app] = Plan{Tables: plan.Tables, writes: append(was.writes, plan.writes...), log: append(was.log, plan.log...)}
	st.tables = next
	st.dirty[app] = true
	return nil
}

func (s *Store[M]) Built(tx *Tx) (*M, error) {
	st := tx.stageOf(s)
	if st == nil {
		return s.Model(), nil
	}
	if st.err != nil {
		return nil, st.err
	}
	if len(st.dirty) > 0 {
		model, err := s.build(tx.ctx, st.tables, st.model.(*M), st.dirty, nil)
		if err != nil {
			st.err = access.Invalid("%v", err)
			return nil, st.err
		}
		st.model, st.dirty = model, map[string]bool{}
	}
	return st.model.(*M), nil
}

func (s *Store[M]) In(tx *Tx) *M {
	if tx == nil {
		return s.Model()
	}
	if m, err := s.Built(tx); err == nil {
		return m
	}
	return tx.stageOf(s).model.(*M)
}
