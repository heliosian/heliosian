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
	tables Tables
	model  any
	plan   Plan
	swap   func()
	write  func(Plan) <-chan struct{}
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
	q.interrupt()
	for _, st := range tx.staged {
		st.swap()
	}
	q.afterSwap()
	var done <-chan struct{}
	for _, st := range tx.staged {
		done = st.write(st.plan)
	}
	return done, tx.after, nil
}

func (s *Store[M]) Stage(tx *Tx, ops ...Op) error {
	st := tx.stageOf(s)
	tables := Tables(nil)
	if st != nil {
		tables = st.tables
	} else {
		s.mu.RLock()
		tables = s.tables
		s.mu.RUnlock()
	}
	plan, err := s.book.Plan(tx.ctx, tables, tx.actor.Email, ops)
	if err != nil || plan.Empty() {
		return err
	}
	model, err := s.spec.Build(tx.ctx, plan.Tables)
	if err != nil {
		return access.Invalid("%v", err)
	}
	if st == nil {
		st = &stage{owner: s, write: s.book.Write}
		tx.staged = append(tx.staged, st)
	}
	st.tables, st.model = plan.Tables, model
	st.plan = Plan{Tables: plan.Tables, writes: append(st.plan.writes, plan.writes...), log: append(st.plan.log, plan.log...)}
	st.swap = func() {
		s.mu.Lock()
		s.tables, s.model = plan.Tables, model
		s.mu.Unlock()
	}
	return nil
}

func (s *Store[M]) In(tx *Tx) M {
	if tx != nil {
		if st := tx.stageOf(s); st != nil {
			return st.model.(M)
		}
	}
	return s.Model()
}
