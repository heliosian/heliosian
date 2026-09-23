package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAskReadsTheStream(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "session=abc" {
			t.Errorf("cookie = %q", r.Header.Get("Cookie"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: tool\ndata: \"Looking at the calendar\"\n\n")
		fmt.Fprint(w, "event: text\ndata: \"Hello\"\n\n")
		fmt.Fprint(w, "event: done\ndata: {\"text\":\"Hello there\",\"tools\":[\"Looking at the calendar\"],\"usage\":{\"input\":100,\"cached\":80,\"output\":7,\"rounds\":2}}\n\n")
	}))
	defer server.Close()
	r := ask(server.Client(), server.URL, "session=abc", "askrun-1", "hi")
	if r.Err != "" {
		t.Fatalf("err = %q", r.Err)
	}
	if r.Text != "Hello there" || len(r.Tools) != 1 || r.Usage.Rounds != 2 || r.Usage.Cached != 80 {
		t.Errorf("result = %+v", r)
	}
}

func TestAskKeepsAnErrorAndAStatus(t *testing.T) {
	failing := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: error\ndata: {\"message\":\"Something went wrong\"}\n\n")
	}))
	defer failing.Close()
	if r := ask(failing.Client(), failing.URL, "session=abc", "askrun-1", "hi"); r.Err != "Something went wrong" {
		t.Errorf("err = %q", r.Err)
	}
	refusing := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "that's a lot of questions for one hour; try again a little later", http.StatusTooManyRequests)
	}))
	defer refusing.Close()
	if r := ask(refusing.Client(), refusing.URL, "session=abc", "askrun-1", "hi"); r.Err != "429 Too Many Requests: that's a lot of questions for one hour; try again a little later" {
		t.Errorf("err = %q", r.Err)
	}
}
