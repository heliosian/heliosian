package store

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"
)

type Load func(context.Context) (Settle, error)

type Settle func() (swap func(), err error)

type Queue struct {
	mu          sync.Mutex
	cond        *sync.Cond
	pending     []func()
	holds       int
	draining    bool
	done        chan struct{}
	lastRefresh time.Time

	commits   sync.Mutex
	loads     []Load
	swapped   []func()
	refreshed chan struct{}
	firstOnce sync.Once
}

func NewQueue() *Queue {
	q := &Queue{done: make(chan struct{}), refreshed: make(chan struct{})}
	q.cond = sync.NewCond(&q.mu)
	go q.run()
	return q
}

func (q *Queue) Add(task func()) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pending = append(q.pending, task)
	q.cond.Signal()
}

func (q *Queue) Hold() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.holds++
}

func (q *Queue) Release() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.holds--
	q.cond.Signal()
}

func (q *Queue) Flush() {
	done := make(chan struct{})
	q.Add(func() { close(done) })
	<-done
}

func (q *Queue) Drain() <-chan struct{} {
	q.mu.Lock()
	q.draining = true
	q.cond.Signal()
	q.mu.Unlock()
	return q.done
}

func (q *Queue) run() {
	for {
		q.mu.Lock()
		for len(q.pending) == 0 && (!q.draining || q.holds > 0) {
			q.cond.Wait()
		}
		if len(q.pending) == 0 {
			q.mu.Unlock()
			close(q.done)
			return
		}
		task := q.pending[0]
		q.pending = q.pending[1:]
		q.mu.Unlock()
		task()
	}
}

func (q *Queue) Register(load Load) {
	q.commits.Lock()
	defer q.commits.Unlock()
	q.loads = append(q.loads, load)
}

func (q *Queue) OnSwap(swapped func()) {
	q.commits.Lock()
	defer q.commits.Unlock()
	q.swapped = append(q.swapped, swapped)
	swapped()
}

func (q *Queue) afterSwap() {
	for _, swapped := range q.swapped {
		swapped()
	}
}

func (q *Queue) Tick() {
	for range time.Tick(refreshInterval) {
		q.Refresh()
	}
}

func (q *Queue) Refresh() {
	q.Add(q.refresh)
}

func (q *Queue) Refreshed() <-chan struct{} {
	return q.refreshed
}

type Status struct {
	Pending     int       `json:"pending"`
	Held        int       `json:"held"`
	LastRefresh time.Time `json:"lastRefresh"`
}

func (q *Queue) Status() Status {
	q.mu.Lock()
	defer q.mu.Unlock()
	return Status{Pending: len(q.pending), Held: q.holds, LastRefresh: q.lastRefresh}
}

func (q *Queue) refresh() {
	start := time.Now()
	q.commits.Lock()
	loads := slices.Clone(q.loads)
	q.commits.Unlock()
	settles := []Settle{}
	for _, load := range loads {
		settle, err := load(context.Background())
		if err != nil {
			slog.Error("refresh", "error", err)
			return
		}
		settles = append(settles, settle)
	}
	q.commits.Lock()
	defer q.commits.Unlock()
	settled := time.Now()
	swaps := []func(){}
	for _, settle := range settles {
		swap, err := settle()
		if err != nil {
			slog.Error("refresh", "error", err)
			return
		}
		swaps = append(swaps, swap)
	}
	for _, swap := range swaps {
		swap()
	}
	q.afterSwap()
	q.mu.Lock()
	q.lastRefresh = time.Now()
	q.mu.Unlock()
	q.firstOnce.Do(func() { close(q.refreshed) })
	slog.Info("refreshed", "took", time.Since(start).Round(time.Millisecond), "locked", time.Since(settled).Round(time.Millisecond))
}
