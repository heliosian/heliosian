package store

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"heliosian/internal/workload"
)

func (s *Store[M]) Add(task func()) {
	s.work.Lock()
	defer s.work.Unlock()
	s.pending = append(s.pending, task)
	s.cond.Signal()
}

func (s *Store[M]) Hold() {
	s.work.Lock()
	defer s.work.Unlock()
	s.holds++
}

func (s *Store[M]) Release() {
	s.work.Lock()
	defer s.work.Unlock()
	s.holds--
	s.cond.Signal()
}

func (s *Store[M]) Flush() {
	s.work.Lock()
	refresh := s.refreshDone
	s.work.Unlock()
	done := make(chan struct{})
	s.Add(func() { close(done) })
	<-done
	if refresh != nil {
		<-refresh
	}
}

func (s *Store[M]) OnDrained(f func()) {
	s.work.Lock()
	defer s.work.Unlock()
	s.drained = append(s.drained, f)
}

func (s *Store[M]) Drain() <-chan struct{} {
	s.work.Lock()
	s.draining = true
	s.cond.Signal()
	s.work.Unlock()
	return s.done
}

func (s *Store[M]) run() {
	for {
		s.work.Lock()
		for len(s.pending) == 0 && (!s.draining || s.holds > 0) {
			s.cond.Wait()
		}
		if len(s.pending) == 0 {
			drained := s.drained
			s.work.Unlock()
			for _, f := range drained {
				f()
			}
			close(s.done)
			return
		}
		task := s.pending[0]
		s.pending = s.pending[1:]
		s.work.Unlock()
		task()
	}
}

func (s *Store[M]) OnSwap(swapped func()) {
	s.commits.Lock()
	defer s.commits.Unlock()
	s.swapped = append(s.swapped, swapped)
	swapped()
}

func (s *Store[M]) afterSwap() {
	for _, swapped := range s.swapped {
		swapped()
	}
}

func (s *Store[M]) OnTick(tick func()) {
	s.commits.Lock()
	defer s.commits.Unlock()
	s.ticked = append(s.ticked, tick)
}

func (s *Store[M]) Tick() {
	refresh, tick := time.Tick(refreshInterval), time.Tick(tickInterval)
	for {
		select {
		case <-refresh:
			s.Refresh()
		case <-tick:
			s.commits.Lock()
			ticked := slices.Clone(s.ticked)
			s.commits.Unlock()
			for _, f := range ticked {
				f()
			}
		}
	}
}

func (s *Store[M]) Refresh() {
	s.work.Lock()
	defer s.work.Unlock()
	if s.refreshDone != nil {
		s.refreshAgain = true
		return
	}
	s.refreshDone = make(chan struct{})
	s.holds++
	s.pending = append(s.pending, s.refresh)
	s.cond.Signal()
}

func (s *Store[M]) Refreshed() <-chan struct{} {
	return s.first
}

type Status struct {
	Pending     int       `json:"pending"`
	Held        int       `json:"held"`
	LastRefresh time.Time `json:"lastRefresh"`
}

func (s *Store[M]) LastRead() time.Time {
	s.work.Lock()
	defer s.work.Unlock()
	return s.lastRead
}

func (s *Store[M]) Status() Status {
	s.work.Lock()
	defer s.work.Unlock()
	return Status{Pending: len(s.pending), Held: s.holds, LastRefresh: s.lastRefresh}
}

func (s *Store[M]) refresh() {
	start := time.Now()
	ctx := context.Background()
	held, base := s.current()
	tables, took, err := s.read(ctx)
	if err != nil {
		slog.Error("refresh", "error", err)
		s.finishRefresh()
		return
	}
	for _, book := range s.books {
		book.retain()
	}
	go func() {
		defer s.finishRefresh()
		settle, err := workload.Run(ctx, s.Work, func() (Settle, error) {
			return s.prepare(ctx, held, base, tables, took)
		})
		if err != nil {
			slog.Error("refresh", "error", err)
			return
		}
		if err := s.swapRefresh(ctx, start, settle); err != nil {
			slog.Error("refresh", "error", err)
		}
	}()
}

func (s *Store[M]) finishRefresh() {
	for _, book := range s.books {
		book.releaseRetained()
	}
	s.work.Lock()
	defer s.work.Unlock()
	if s.refreshAgain {
		s.refreshAgain = false
		s.pending = append(s.pending, s.refresh)
		s.cond.Signal()
		return
	}
	close(s.refreshDone)
	s.refreshDone = nil
	s.holds--
	s.cond.Signal()
}

func (s *Store[M]) swapRefresh(ctx context.Context, start time.Time, settle Settle) error {
	release, err := s.Work.Commit(ctx, true)
	if err != nil {
		return err
	}
	defer release()
	s.commits.Lock()
	defer s.commits.Unlock()
	settled := time.Now()
	swap, err := settle()
	if err != nil {
		return err
	}
	swap()
	s.afterSwap()
	s.work.Lock()
	s.lastRefresh, s.lastRead = time.Now(), start
	s.work.Unlock()
	s.firstOnce.Do(func() { close(s.first) })
	slog.Info("refreshed", "took", time.Since(start).Round(time.Millisecond), "locked", time.Since(settled).Round(time.Millisecond))
	return nil
}
