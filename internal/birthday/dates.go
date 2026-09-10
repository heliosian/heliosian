package birthday

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// requestLeadDays is how far ahead of the newsletter a staff member is asked
// for their charity, so there is time for a reply before the deadline.
const requestLeadDays = 10

var yearForm = regexp.MustCompile(`^(\d{4}) - (\d{4})$`)

// ParseMonthDay reads the Year Start setting, 08-14.
func ParseMonthDay(cell string) (time.Month, int, error) {
	t, err := time.Parse(MonthDayFormat, cell)
	if err != nil {
		return 0, 0, fmt.Errorf("%q is not a month and day like 08-14", cell)
	}
	return t.Month(), t.Day(), nil
}

// CheckYear accepts a birthday year written as its two calendar years.
func CheckYear(year string) error {
	m := yearForm.FindStringSubmatch(year)
	if m == nil {
		return fmt.Errorf("year %q is not like 2026 - 2027", year)
	}
	from, _ := strconv.Atoi(m[1])
	to, _ := strconv.Atoi(m[2])
	if to != from+1 {
		return fmt.Errorf("year %q does not span consecutive years", year)
	}
	return nil
}

// ShiftYear moves a well-formed year by n years.
func ShiftYear(year string, n int) string {
	m := yearForm.FindStringSubmatch(year)
	from, _ := strconv.Atoi(m[1])
	return fmt.Sprintf("%d - %d", from+n, from+n+1)
}

// dateIn is the day in a given year, with February 29 landing on the 28th in a
// year that has no 29th rather than rolling into March.
func dateIn(year int, month time.Month, day int) time.Time {
	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if t.Month() != month {
		t = t.AddDate(0, 0, -1)
	}
	return t
}

// Year is one birthday year: it starts on the Year Start day and runs to the
// day before the next one, so summer birthdays belong to the year that just
// ended and its last newsletter, not to the year about to begin.
type Year struct {
	Label string
	Start time.Time
	End   time.Time
}

// YearContaining is the birthday year a date falls in.
func YearContaining(t time.Time, month time.Month, day int) Year {
	start := dateIn(t.Year(), month, day)
	day0 := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	if day0.Before(start) {
		start = dateIn(t.Year()-1, month, day)
	}
	return Year{Label: fmt.Sprintf("%d - %d", start.Year(), start.Year()+1), Start: start, End: start.AddDate(1, 0, 0)}
}

// YearLabelled is the birthday year with a given label.
func YearLabelled(label string, month time.Month, day int) Year {
	m := yearForm.FindStringSubmatch(label)
	from, _ := strconv.Atoi(m[1])
	start := dateIn(from, month, day)
	return Year{Label: label, Start: start, End: start.AddDate(1, 0, 0)}
}

// Occurrence places a birthday's month and day inside a year.
func (y Year) Occurrence(birthday time.Time) time.Time {
	t := dateIn(y.Start.Year(), birthday.Month(), birthday.Day())
	if t.Before(y.Start) {
		t = dateIn(y.Start.Year()+1, birthday.Month(), birthday.Day())
	}
	return t
}

// Newsletter picks the newsletter that carries a birthday: the first one on or
// after it within the year, else the last one before it, so a birthday after
// the final newsletter of the year still lands in that final issue.
func (y Year) Newsletter(birthday time.Time, dates []string) (time.Time, bool) {
	var last time.Time
	found := false
	for _, cell := range dates {
		d, err := ParseDate(cell)
		if err != nil || d.Before(y.Start) || !d.Before(y.End) {
			continue
		}
		if !d.Before(birthday) {
			return d, true
		}
		last = d
		found = true
	}
	return last, found
}

// RequestBy is the day outreach is due for a newsletter.
func RequestBy(newsletter time.Time) time.Time {
	return newsletter.AddDate(0, 0, -requestLeadDays)
}

// Stage reads how far a birthday has come this year from what has been
// recorded about it. Assignment is not a stage: an unassigned birthday sits in
// whatever stage its progress says, and the client groups it separately.
func Stage(level string, contacted, donated, used bool, requestBy time.Time, hasRequestBy bool, today time.Time) string {
	switch {
	case used, donated && level == LevelNoNewsletter:
		return StageComplete
	case donated:
		return StageNewsletter
	case contacted:
		return StageResponse
	case hasRequestBy && !today.Before(requestBy):
		return StageOutreach
	}
	return StageWait
}
