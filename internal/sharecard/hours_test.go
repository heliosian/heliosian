package sharecard

import (
	"testing"
	"time"
)

func TestHours(t *testing.T) {
	at := func(day, clock int) time.Time {
		return time.Date(2026, 10, day, clock, 0, 0, 0, time.UTC)
	}
	for _, tc := range []struct {
		start, end time.Time
		want       string
	}{
		{at(3, 17), at(3, 21), "5:00 – 9:00 PM"},
		{at(3, 9), at(3, 11), "9:00 – 11:00 AM"},
		{at(3, 11), at(3, 13), "11:00 AM – 1:00 PM"},
		{at(3, 21), at(4, 1), "9:00 PM – 1:00 AM"},
		{at(3, 11), at(3, 11), "11:00 AM"},
	} {
		if got := Hours(tc.start, tc.end); got != tc.want {
			t.Errorf("Hours(%v, %v) = %q, want %q", tc.start, tc.end, got, tc.want)
		}
	}
}
