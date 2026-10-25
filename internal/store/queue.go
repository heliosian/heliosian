package store

import (
	"context"
	"log/slog"
	"slices"
	"time"
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
	done := make(chan struct{})
	s.Add(func() { close(done) })
	<-done
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
			s.work.Unlock()
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
	s.Add(s.refresh)
}

func (s *Store[M]) Refreshed() <-chan struct{} {
	return s.first
}

type Status struct {
	Pending     int       `json:"pending"`
	Held        int       `json:"held"`
	LastRefresh time.Time `json:"lastRefresh"`
}

func (s *Store[M]) Status() Status {
	s.work.Lock()
	defer s.work.Unlock()
	return Status{Pending: len(s.pending), Held: s.holds, LastRefresh: s.lastRefresh}
}

func (s *Store[M]) refresh() {
	start := time.Now()
	settle, err := s.load(context.Background())
	if err != nil {
		slog.Error("refresh", "error", err)
		return
	}
	s.commits.Lock()
	defer s.commits.Unlock()
	settled := time.Now()
	swap, err := settle()
	if err != nil {
		slog.Error("refresh", "error", err)
		return
	}
	swap()
	s.afterSwap()
	s.work.Lock()
	s.lastRefresh = time.Now()
	s.work.Unlock()
	s.firstOnce.Do(func() { close(s.first) })
	slog.Info("refreshed", "took", time.Since(start).Round(time.Millisecond), "locked", time.Since(settled).Round(time.Millisecond))
}
