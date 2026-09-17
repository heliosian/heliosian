package calendarimport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestKicksDuringARunCollapseIntoOneMore(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{}, 10)
	runs := 0
	w := &Watcher{kick: make(chan struct{}, 1), refresh: func() { finished <- struct{}{} }}
	w.run = func(context.Context) error {
		runs++
		if runs == 1 {
			started <- struct{}{}
			<-release
		}
		return nil
	}
	go w.work()
	w.Kick()
	<-started
	for range 5 {
		w.Kick()
	}
	close(release)
	<-finished
	<-finished
	select {
	case <-finished:
		t.Fatal("a third run")
	case <-time.After(100 * time.Millisecond):
	}
	if runs != 2 {
		t.Fatalf("%d runs, want 2", runs)
	}
}

func post(w *Watcher, token, state string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", HookPath, nil)
	r.Header.Set("X-Goog-Channel-Token", token)
	r.Header.Set("X-Goog-Resource-State", state)
	rec := httptest.NewRecorder()
	w.ServeHTTP(rec, r)
	return rec
}

func TestHookRefusesAnotherChannelsToken(t *testing.T) {
	w := &Watcher{token: "ours", kick: make(chan struct{}, 1)}
	if rec := post(w, "theirs", "exists"); rec.Code != http.StatusForbidden {
		t.Fatalf("status %d", rec.Code)
	}
	if len(w.kick) != 0 {
		t.Fatal("kicked")
	}
}

func TestHookIgnoresTheOpeningPost(t *testing.T) {
	w := &Watcher{token: "ours", kick: make(chan struct{}, 1)}
	if rec := post(w, "ours", "sync"); rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if len(w.kick) != 0 {
		t.Fatal("kicked")
	}
}

func TestHookKicksOnAChange(t *testing.T) {
	w := &Watcher{token: "ours", kick: make(chan struct{}, 1)}
	if rec := post(w, "ours", "exists"); rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if len(w.kick) != 1 {
		t.Fatal("not kicked")
	}
}
