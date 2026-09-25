package store

import (
	"testing"
	"time"
)

func TestDrainWaitsForHeldWork(t *testing.T) {
	q := NewQueue()
	q.Hold()
	done := q.Drain()
	select {
	case <-done:
		t.Fatal("the drain finished with work still held")
	case <-time.After(50 * time.Millisecond):
	}
	ran := false
	q.Add(func() { ran = true })
	q.Release()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the drain did not finish once the hold was released")
	}
	if !ran {
		t.Fatal("the held work's write was dropped")
	}
}
