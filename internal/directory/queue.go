package directory

import (
	"sync"
)

type Queue struct {
	mu       sync.Mutex
	cond     *sync.Cond
	pending  []func()
	draining bool
	done     chan struct{}
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
		for len(q.pending) == 0 && !q.draining {
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
