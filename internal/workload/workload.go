package workload

import (
	"context"
	"net/http"
	"sync"
	"time"

	"heliosian/internal/trace"
)

type Scheduler struct {
	mu          sync.Mutex
	changed     chan struct{}
	requests    int
	working     bool
	next        time.Time
	committing  bool
	interactive int
}

type interactiveKey struct{}

func Interactive(ctx context.Context) bool {
	interactive, _ := ctx.Value(interactiveKey{}).(bool)
	return interactive
}

func New() *Scheduler { return &Scheduler{changed: make(chan struct{})} }

func (s *Scheduler) signal() { close(s.changed); s.changed = make(chan struct{}) }

func (s *Scheduler) Foreground() func() {
	s.mu.Lock()
	s.requests++
	s.signal()
	s.mu.Unlock()
	start := time.Now()
	return func() {
		elapsed := time.Since(start)
		s.mu.Lock()
		s.requests--
		if elapsed >= 750*time.Millisecond {
			s.next = maxTime(s.next, time.Now().Add(elapsed))
		}
		s.signal()
		s.mu.Unlock()
	}
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func (s *Scheduler) Do(ctx context.Context, run func() error) error {
	release, err := s.acquire(ctx, true, false)
	if err != nil {
		return err
	}
	defer release()
	return run()
}

func (s *Scheduler) Commit(ctx context.Context, background bool) (func(), error) {
	return s.acquire(ctx, background, true)
}

func (s *Scheduler) acquire(ctx context.Context, background, commit bool) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	interactive := commit && !background
	if interactive {
		s.interactive++
	}
	for {
		if err := ctx.Err(); err != nil {
			if interactive {
				s.interactive--
			}
			s.signal()
			s.mu.Unlock()
			return nil, err
		}
		wait := time.Until(s.next)
		cpuReady := !background || !s.working && s.requests == 0 && wait <= 0
		commitReady := !commit || !s.committing && (!background || s.interactive == 0)
		if cpuReady && commitReady {
			break
		}
		changed := s.changed
		s.mu.Unlock()
		if !background || wait <= 0 {
			wait = time.Hour
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-changed:
			timer.Stop()
		case <-timer.C:
		}
		s.mu.Lock()
	}
	if interactive {
		s.interactive--
	}
	if background {
		s.working = true
	}
	if commit {
		s.committing = true
	}
	s.mu.Unlock()
	var start time.Duration
	if background {
		start = trace.CPUTime()
	}
	return func() {
		var cpu time.Duration
		if background {
			cpu = trace.CPUTime() - start
		}
		s.mu.Lock()
		if background {
			s.working = false
			s.next = maxTime(s.next, time.Now().Add(cpu))
		}
		if commit {
			s.committing = false
		}
		s.signal()
		s.mu.Unlock()
	}, nil
}

func (s *Scheduler) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		interactive := r.Header.Get("Helios-Mode") != "import"
		ctx := context.WithValue(r.Context(), interactiveKey{}, interactive)
		if interactive && r.URL.Path == "/api/q" && r.Method == "QUERY" {
			release := s.Foreground()
			defer release()
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func Run[T any](ctx context.Context, s *Scheduler, run func() (T, error)) (T, error) {
	var value T
	err := s.Do(ctx, func() error { var err error; value, err = run(); return err })
	return value, err
}
