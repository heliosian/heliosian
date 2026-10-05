package store

import (
	"context"

	"heliosian/internal/access"
)

type Tx struct {
	ctx    context.Context
	actor  access.Actor
	staged []*stage
	after  []func()
}

type stage struct {
	owner  any
	tables map[string]Tables
	model  any
	plans  map[string]Plan
	order  []string
	swap   func()
	write  func() <-chan struct{}
}

func (tx *Tx) Actor() access.Actor {
	return tx.actor
}

func (tx *Tx) After(run func()) {
	tx.after = append(tx.after, run)
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
		st.swap()
	}
	q.afterSwap()
	var done <-chan struct{}
	for _, st := range tx.staged {
		done = st.write()
	}
	q.interrupt()
	return done, tx.after, nil
}
