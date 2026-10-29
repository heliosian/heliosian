package store

import (
	"testing"
	"time"
)

func TestDrainWaitsForHeldWork(t *testing.T) {
	s := newFixture(t).store
	s.Hold()
	done := s.Drain()
	select {
	case <-done:
		t.Fatal("the drain finished with work still held")
	case <-time.After(50 * time.Millisecond):
	}
	ran := false
	s.Add(func() { ran = true })
	s.Release()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the drain did not finish once the hold was released")
	}
	if !ran {
		t.Fatal("the held work's write was dropped")
	}
}

func TestLastReadIsWhenTheSwappedRefreshStartedReading(t *testing.T) {
	s := newFixture(t).store
	asked := time.Now()
	s.Refresh()
	s.Flush()
	if !s.LastRead().After(asked) || s.LastRead().After(s.Status().LastRefresh) {
		t.Fatalf("read %v, asked %v, swapped %v", s.LastRead(), asked, s.Status().LastRefresh)
	}
}

func TestDrainedHooksRunAfterTheLastQueuedWork(t *testing.T) {
	s := newFixture(t).store
	order := []string{}
	s.OnDrained(func() { order = append(order, "drained") })
	s.Hold()
	done := s.Drain()
	s.Add(func() { order = append(order, "write") })
	s.Release()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the drain did not finish")
	}
	if len(order) != 2 || order[0] != "write" || order[1] != "drained" {
		t.Fatalf("ran %v", order)
	}
}
