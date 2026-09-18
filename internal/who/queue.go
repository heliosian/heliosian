package who

import (
	"sync"
)

type Queue struct {
	mu       sync.Mutex
	cond     *sync.Cond
	pending  []func()
	holds    int
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

// Hold is work under way that will add to the queue when it finishes, so a
// drain waits for its Release as well as for what is already queued.
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
