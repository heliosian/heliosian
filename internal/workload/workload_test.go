package workload

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type waitingContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *waitingContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestBackgroundCommitDoesNotHoldCPUWhileWaitingForCommit(t *testing.T) {
	s := New()
	release, err := s.Commit(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	waiting := &waitingContext{Context: ctx, waiting: make(chan struct{})}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		end, err := s.Commit(waiting, true)
		if err != nil {
			t.Error(err)
			return
		}
		end()
	}()
	await(t, waiting.waiting)
	ctx, stop := context.WithTimeout(t.Context(), time.Second)
	if err := s.Do(ctx, func() error { return nil }); err != nil {
		t.Error("commit waiter held CPU:", err)
	}
	stop()
	release()
	await(t, finished)
}

func TestBackgroundCommitDoesNotHoldCommitGateDuringRest(t *testing.T) {
	s := New()
	s.next = time.Now().Add(time.Hour)
	ctx, cancel := context.WithCancel(t.Context())
	waiting := &waitingContext{Context: ctx, waiting: make(chan struct{})}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, err := s.Commit(waiting, true)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("background commit: %v", err)
		}
	}()
	await(t, waiting.waiting)
	ctx, stop := context.WithTimeout(t.Context(), time.Second)
	release, err := s.Commit(ctx, false)
	if err != nil {
		t.Error("background rest held commit gate:", err)
	} else {
		release()
	}
	stop()
	cancel()
	await(t, finished)
}

func await(t *testing.T, ready <-chan struct{}) {
	t.Helper()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("work did not finish")
	}
}

func TestForegroundRequestRunsWhileBackgroundIsBusy(t *testing.T) {
	s := New()
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(finished)
		if err := s.Do(t.Context(), func() error { close(started); <-release; return nil }); err != nil {
			t.Error(err)
		}
	}()
	await(t, started)
	request := httptest.NewRequest("QUERY", "/api/q", nil)
	rec := httptest.NewRecorder()
	s.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })).ServeHTTP(rec, request)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code)
	}
	close(release)
	await(t, finished)
}

func TestBackgroundYieldsToActiveReadsAndHonorsCancellation(t *testing.T) {
	s := New()
	end := s.Foreground()
	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan struct{})
	called := atomic.Bool{}
	go func() {
		defer close(finished)
		err := s.Do(ctx, func() error { called.Store(true); return nil })
		if !errors.Is(err, context.Canceled) {
			t.Errorf("waiting job: %v", err)
		}
	}()
	cancel()
	await(t, finished)
	if called.Load() {
		t.Fatal("background job started during active foreground work")
	}
	end()
	if err := s.Do(t.Context(), func() error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestInteractiveCommitOvertakesWaitingBackgroundCommit(t *testing.T) {
	s := New()
	release, err := s.Commit(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	order := make(chan string, 2)
	go func() {
		end, err := s.Commit(t.Context(), true)
		if err != nil {
			t.Error(err)
			return
		}
		order <- "background"
		end()
	}()
	go func() {
		end, err := s.Commit(t.Context(), false)
		if err != nil {
			t.Error(err)
			return
		}
		order <- "interactive"
		end()
	}()
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		pending := s.interactive
		s.mu.Unlock()
		if pending > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("interactive commit was not queued")
		}
		time.Sleep(time.Millisecond)
	}
	release()
	if got := <-order; got != "interactive" {
		t.Fatal("first commit:", got)
	}
	if got := <-order; got != "background" {
		t.Fatal("second commit:", got)
	}
}

func TestBulkRequestDoesNotHoldPermitDuringIO(t *testing.T) {
	s := New()
	first, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	handler := s.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if Interactive(r.Context()) {
			t.Error("import request was interactive")
		}
		close(first)
		<-release
	}))
	go func() {
		defer close(finished)
		r := httptest.NewRequest(http.MethodPost, "/api/do/pdf-page", nil)
		r.Header.Set("Helios-Mode", "import")
		handler.ServeHTTP(httptest.NewRecorder(), r)
	}()
	await(t, first)
	if err := s.Do(t.Context(), func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	close(release)
	await(t, finished)
}

func TestPanicReleasesBackgroundPermit(t *testing.T) {
	s := New()
	func() {
		defer func() {
			if recover() == nil {
				t.Error("missing panic")
			}
		}()
		s.Do(t.Context(), func() error { panic("broken work") })
	}()
	if err := s.Do(t.Context(), func() error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestLongStreamsDoNotStopBackgroundWork(t *testing.T) {
	s := New()
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	handler := s.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-release }))
	go func() {
		defer close(finished)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))
	}()
	await(t, started)
	if err := s.Do(t.Context(), func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	close(release)
	await(t, finished)
}

func TestCanceledWorkDoesNotRun(t *testing.T) {
	s := New()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := s.Do(ctx, func() error { t.Fatal("canceled work ran"); return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestBackgroundWorkDoesNotOverlap(t *testing.T) {
	s := New()
	active := atomic.Int32{}
	finished := make(chan struct{}, 8)
	for range 8 {
		go func() {
			defer func() { finished <- struct{}{} }()
			if err := s.Do(t.Context(), func() error {
				if active.Add(1) != 1 {
					t.Error("background work overlapped")
				}
				time.Sleep(time.Millisecond)
				active.Add(-1)
				return nil
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	for range 8 {
		await(t, finished)
	}
}
