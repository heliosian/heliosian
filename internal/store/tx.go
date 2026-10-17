package store

import (
	"context"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/trace"
)

type Tx struct {
	ctx     context.Context
	actor   access.Actor
	staged  []*stage
	swapped []func()
	after   []func()
	values  map[any]any
}

type stage struct {
	owner  any
	start  any
	tables map[string]Tables
	model  any
	dirty  map[string]bool
	err    error
	plans  map[string]Plan
	order  []string
	settle func() error
	swap   func()
	write  func() <-chan struct{}
}

func (tx *Tx) Actor() access.Actor {
	return tx.actor
}

func (tx *Tx) Context() context.Context {
	return tx.ctx
}

func (tx *Tx) After(run func()) {
	tx.after = append(tx.after, run)
}

func (tx *Tx) Swapped(run func()) {
	tx.swapped = append(tx.swapped, run)
}

func (tx *Tx) Value(key any, make func() any) any {
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

func (tx *Tx) stageOf(owner any) *stage {
	for _, st := range tx.staged {
		if st.owner == owner {
			return st
		}
	}
	return nil
}

func (q *Queue) Transact(ctx context.Context, actor access.Actor, run func(tx *Tx) error) (<-chan struct{}, error) {
	done, after, err := q.transact(ctx, actor, run)
	if err != nil {
		return nil, err
	}
	for _, f := range after {
		f()
	}
	return done, nil
}

func (q *Queue) transact(ctx context.Context, actor access.Actor, run func(tx *Tx) error) (<-chan struct{}, []func(), error) {
	q.commits.Lock()
	defer q.commits.Unlock()
	tx := &Tx{ctx: ctx, actor: actor}
	if err := run(tx); err != nil {
		return nil, nil, err
	}
	if len(tx.staged) == 0 {
		return nil, tx.after, nil
	}
	for _, st := range tx.staged {
		if err := st.settle(); err != nil {
			return nil, nil, err
		}
	}
	for _, st := range tx.staged {
		st.swap()
	}
	delivering := time.Now()
	for _, f := range tx.swapped {
		f()
	}
	trace.From(ctx).Tally("swapped").Add(time.Since(delivering))
	start := time.Now()
	q.afterSwap()
	trace.From(ctx).Tally("swap hooks").Add(time.Since(start))
	var done <-chan struct{}
	for _, st := range tx.staged {
		done = st.write()
	}
	return done, tx.after, nil
}
