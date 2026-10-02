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
		w.Write([]byte("%PDF-1.4"))
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func TestFetch(t *testing.T) {
	retryWaits = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	server, calls := throttled(t, 2)
	if body, err := fetch(server.URL); err != nil || string(body) != "%PDF-1.4" || *calls != 3 {
		t.Fatalf("throttled twice: %q, %v, %d calls", body, err, *calls)
	}
	server, calls = throttled(t, 10)
	if _, err := fetch(server.URL); err == nil || *calls != 4 {
		t.Fatalf("throttled throughout: %v, %d calls", err, *calls)
	}
	calls404 := 0
	gone := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls404++
		http.Error(w, "gone", http.StatusNotFound)
	}))
	t.Cleanup(gone.Close)
	if _, err := fetch(gone.URL); err == nil || calls404 != 1 {
		t.Fatalf("not found: %v, %d calls", err, calls404)
	}
}

func TestFindPDF(t *testing.T) {
	got, err := findPDF([]byte(`<a href="/uploads/Year&amp;Days.pdf">x</a><a href="other.pdf">`))
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://www.heliosschool.org/uploads/Year&Days.pdf"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := findPDF([]byte("no link")); err == nil {
		t.Fatal("a page with no pdf link")
	}
}
