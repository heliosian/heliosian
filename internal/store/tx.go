package store

import (
	"context"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/trace"
)

type Tx[M any] struct {
	ctx     context.Context
	actor   access.Actor
	stage   *stage[M]
	swapped []func()
	after   []func()
	values  map[any]any
}

type stage[M any] struct {
	start  *M
	tables map[string]Tables
	model  *M
	dirty  map[string]bool
	err    error
	plans  map[string]Plan
	order  []string
}

func (tx *Tx[M]) Actor() access.Actor {
	return tx.actor
}

func (tx *Tx[M]) Context() context.Context {
	return tx.ctx
}

func (tx *Tx[M]) After(run func()) {
	tx.after = append(tx.after, run)
}

func (tx *Tx[M]) Swapped(run func()) {
	tx.swapped = append(tx.swapped, run)
}

func (tx *Tx[M]) Value(key any, make func() any) any {
	if v, ok := tx.values[key]; ok {
		return v
	}
	if tx.values == nil {
		tx.values = map[any]any{}
	}
	v := make()
	tx.values[key] = v
	return v
}

func (s *Store[M]) Transact(ctx context.Context, actor access.Actor, run func(tx *Tx[M]) error) (<-chan struct{}, error) {
	done, after, err := s.transact(ctx, actor, run)
	if err != nil {
		return nil, err
	}
	for _, f := range after {
		f()
	}
	return done, nil
}

func (s *Store[M]) transact(ctx context.Context, actor access.Actor, run func(tx *Tx[M]) error) (<-chan struct{}, []func(), error) {
	s.commits.Lock()
	defer s.commits.Unlock()
	tx := &Tx[M]{ctx: ctx, actor: actor}
	if err := run(tx); err != nil {
		return nil, nil, err
	}
	st := tx.stage
	if st == nil {
		return nil, tx.after, nil
	}
	final, err := s.Built(tx)
	if err != nil {
		return nil, nil, err
	}
	if err := s.check(ctx, st.start, final); err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	s.tables, s.model = st.tables, st.model
	s.mu.Unlock()
	delivering := time.Now()
	for _, f := range tx.swapped {
		f()
	}
	trace.From(ctx).Tally("swapped").Add(time.Since(delivering))
	start := time.Now()
	s.afterSwap()
	trace.From(ctx).Tally("swap hooks").Add(time.Since(start))
	var done <-chan struct{}
	for _, app := range st.order {
		done = s.write(s.books[app], st.plans[app])
	}
	return done, tx.after, nil
}
