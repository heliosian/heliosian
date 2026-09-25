package ratelimit

import (
	"testing"
	"time"
)

func TestLimiterCountsAWindow(t *testing.T) {
	l := New(3, time.Minute)
	now := time.Now()
	for i := range 3 {
		if !l.Allow("a@example.org", now.Add(time.Duration(i)*time.Second)) {
			t.Fatalf("call %d refused", i)
		}
	}
	if l.Allow("a@example.org", now.Add(10*time.Second)) {
		t.Error("the call past the limit was allowed")
	}
	if !l.Allow("b@example.org", now) {
		t.Error("another key refused")
	}
	if !l.Allow("a@example.org", now.Add(time.Minute+time.Second)) {
		t.Error("a call after the window refused")
	}
}
