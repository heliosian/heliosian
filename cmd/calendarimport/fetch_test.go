package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func throttled(t *testing.T, refusals int) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls <= refusals {
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		w.Write([]byte("BEGIN:VCALENDAR"))
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func TestFetchWaitsOutThrottling(t *testing.T) {
	retryWaits = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	server, calls := throttled(t, 2)
	body, err := fetch(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "BEGIN:VCALENDAR" {
		t.Fatalf("body %q", body)
	}
	if *calls != 3 {
		t.Fatalf("%d calls, want 3", *calls)
	}
}

func TestFetchGivesUpAfterTheLastWait(t *testing.T) {
	retryWaits = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	server, calls := throttled(t, 10)
	_, err := fetch(server.URL)
	if err == nil {
		t.Fatal("no error")
	}
	if *calls != 4 {
		t.Fatalf("%d calls, want 4", *calls)
	}
}

func TestFetchDoesNotRetryOtherFailures(t *testing.T) {
	retryWaits = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "gone", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	_, err := fetch(server.URL)
	if err == nil {
		t.Fatal("no error")
	}
	if calls != 1 {
		t.Fatalf("%d calls, want 1", calls)
	}
}
