package store

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"
)

type Load func(context.Context) (func(), error)

type Queue struct {
	mu       sync.Mutex
	cond     *sync.Cond
	pending  []func()
	holds    int
	draining bool
	done     chan struct{}

	commits sync.Mutex
	cancel  context.CancelFunc
	loads   []Load
}

func NewQueue() *Queue {
	q := &Queue{done: make(chan struct{})}
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
		// Emptiness is only rechecked between tasks, so a running task never reads as empty.
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

func (q *Queue) Tick() {
	for range time.Tick(refreshInterval) {
		q.Refresh()
	}
}

func (q *Queue) Refresh() {
	q.Add(q.refresh)
}

func (q *Queue) interrupt() {
	if q.cancel != nil {
		q.cancel()
		q.cancel = nil
	}
}

func (q *Queue) refresh() {
	start := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.commits.Lock()
	q.cancel = cancel
	loads := slices.Clone(q.loads)
	q.commits.Unlock()
	swaps := []func(){}
	for _, load := range loads {
		swap, err := load(ctx)
		if ctx.Err() != nil {
			slog.Info("refresh abandoned: a commit came in")
			return
		}
		if err != nil {
			slog.Error("[ERROR] refresh", "error", err)
			return
		}
		swaps = append(swaps, swap)
	}
	q.commits.Lock()
	defer q.commits.Unlock()
	if ctx.Err() != nil {
		slog.Info("refresh abandoned: a commit came in")
		return
	}
	q.cancel = nil
	for _, swap := range swaps {
		swap()
	}
	slog.Info("refreshed", "took", time.Since(start).Round(time.Millisecond))
}
