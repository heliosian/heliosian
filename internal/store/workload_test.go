package store

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"heliosian/internal/access"
)

func TestRefreshDuringAnActivePassRereadsBeforeFlushReturns(t *testing.T) {
	f := newFixture(t)
	s := f.store
	done := make(chan struct{})
	func() {
		end := s.Work.Foreground()
		defer end()
		s.Refresh()
		read := make(chan struct{})
		s.Add(func() { close(read) })
		<-read
		if err := f.dir.Insert("app", "Things", []Row{{"Name": "sock"}}); err != nil {
			t.Fatal(err)
		}
		s.Refresh()
		s.Refresh()
		go func() { s.Flush(); close(done) }()
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("refresh rerun did not finish: %+v", s.Status())
	}
	if s.Model().things != 3 || count(s, "app", "Things", Row{"Name": "sock"}) != 1 {
		t.Fatal("flush returned without the sheet edit requested during the first refresh")
	}
	if status := s.Status(); status.Held != 0 {
		t.Fatalf("completed refresh retained its drain hold: %+v", status)
	}
}

func TestRefreshWaitingForCPUDoesNotBlockPersistence(t *testing.T) {
	f := newFixture(t)
	paused := pausedSource{Dir: f.dir, pause: &atomic.Bool{}, reading: make(chan struct{}), release: make(chan struct{})}
	s, err := New([]Part[counts]{part()}, consent, noCheck, paused, f.dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.dir.Insert("app", "Things", []Row{{"Name": "sock"}}); err != nil {
		t.Fatal(err)
	}
	end := s.Work.Foreground()
	paused.pause.Store(true)
	s.Refresh()
	<-paused.reading
	close(paused.release)
	read := make(chan struct{})
	s.Add(func() { close(read) })
	<-read
	done := make(chan error, 1)
	go func() {
		done <- commitAndWait(s, context.Background(), access.Actor{Email: "ann"}, "app", Insert("Things", Row{"Name": "cap"}))
	}()
	select {
	case err := <-done:
		if err != nil {
			end()
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		end()
		t.Fatalf("persistence waited for refresh CPU admission: %+v", s.Status())
	}
	if len(f.rows(t, "Things")) != 4 {
		end()
		t.Fatal("commit did not reach the sheet while refresh waited")
	}
	end()
	s.Flush()
	if m := s.Model(); m.things != 4 || count(s, "app", "Things", Row{"Name": "cap"}) != 1 {
		t.Fatalf("refresh lost or duplicated the persisted commit: %+v", *m)
	}
}

func TestRequestSystemTransactionDoesNotWaitForBackgroundCPU(t *testing.T) {
	s := newFixture(t).store
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(finished)
		if err := s.Work.Do(t.Context(), func() error { close(started); <-release; return nil }); err != nil {
			t.Error(err)
		}
	}()
	<-started
	defer func() { close(release); <-finished }()
	done := make(chan struct{})
	go func() {
		defer close(done)
		h := s.Work.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := commit(s, r.Context(), access.System("feedback"), "app", Update("Things", Row{"Name": "hat"}, Row{"Color": "green"})); err != nil {
				t.Error(err)
			}
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/feedback", nil))
	}()
	select {
	case <-done:
		if count(s, "app", "Things", Row{"Name": "hat", "Color": "green"}) != 1 {
			t.Fatal("system request did not commit")
		}
	case <-time.After(time.Second):
		t.Fatal("system request waited for background CPU")
	}
}

func TestRefreshSheetReadDoesNotHoldBackgroundCPU(t *testing.T) {
	f := newFixture(t)
	paused := pausedSource{Dir: f.dir, pause: &atomic.Bool{}, reading: make(chan struct{}), release: make(chan struct{})}
	s, err := New([]Part[counts]{part()}, consent, noCheck, paused, f.dir)
	if err != nil {
		t.Fatal(err)
	}
	paused.pause.Store(true)
	s.Refresh()
	<-paused.reading
	defer func() { close(paused.release); s.Flush() }()
	done := make(chan error, 1)
	go func() { done <- s.Work.Do(t.Context(), func() error { return nil }) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("sheet read held background CPU")
	}
}

func TestInteractiveTransactionDoesNotWaitForBackgroundCPU(t *testing.T) {
	s := newFixture(t).store
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(finished)
		if err := s.Work.Do(t.Context(), func() error { close(started); <-release; return nil }); err != nil {
			t.Error(err)
		}
	}()
	<-started
	defer func() { close(release); <-finished }()
	done := make(chan error, 1)
	go func() {
		_, err := s.Transact(t.Context(), access.Actor{Email: "ann@example.org"}, func(tx *Tx[counts]) error { return nil })
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("interactive transaction waited for background CPU")
	}
}

func TestBackgroundAfterCanStartAnotherBackgroundTransaction(t *testing.T) {
	s := newFixture(t).store
	done := make(chan error, 1)
	go func() {
		_, err := s.Transact(t.Context(), access.System("first"), func(tx *Tx[counts]) error {
			tx.After(func() {
				_, nested := s.Transact(t.Context(), access.System("second"), func(tx *Tx[counts]) error { return nil })
				if nested != nil {
					t.Error(nested)
				}
			})
			return nil
		})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("after callback retained the background permit")
	}
}
