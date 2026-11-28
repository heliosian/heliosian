package db

import (
	"testing"
	"time"
)

func TestACelebrationStopsTakingPartiesOnceItHasPassed(t *testing.T) {
	s := sample(t)
	apply(t, s, []Edit{{Set: celebration, Cells: map[string]any{"adding": "approval", "invitation": "", "start": "2027-03-06 17:30", "end": "2027-03-06 22:00"}}})
	adding := func() string {
		row, _ := s.Model().Table("GROUP").Get(celebration)
		return row["adding"]
	}
	closePassedCelebrations(t.Context(), s, time.Date(2027, 3, 6, 23, 0, 0, 0, School))
	if got := adding(); got != "approval" {
		t.Fatalf("on its own day the celebration takes parties %q", got)
	}
	closePassedCelebrations(t.Context(), s, time.Date(2027, 3, 7, 0, 30, 0, 0, School))
	if got := adding(); got != "" {
		t.Fatalf("the day after, the celebration still takes parties %q", got)
	}
}
