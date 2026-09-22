package claude

import (
	"testing"
	"time"
)

func TestLimiterCountsAnHour(t *testing.T) {
	l := NewLimiter()
	now := time.Now()
	for i := 0; i < perHour; i++ {
		if !l.Allow("jordan@example.org", now) {
			t.Fatalf("call %d refused", i)
		}
	}
	if l.Allow("jordan@example.org", now) {
		t.Fatal("the call past the limit was allowed")
	}
	if !l.Allow("jordan@example.org", now.Add(time.Hour+time.Minute)) {
		t.Fatal("an hour later was refused")
	}
}
