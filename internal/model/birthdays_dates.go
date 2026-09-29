package model

import (
	"fmt"
	"time"
)

const DefaultRequestLeadDays = 8

const DefaultDueByLeadDays = 2

func ParseMonthDay(cell string) (time.Month, int, error) {
	t, err := time.Parse(MonthDayFormat, cell)
	if err != nil {
		return 0, 0, fmt.Errorf("%q is not a month and day like 08-14", cell)
	}
	return t.Month(), t.Day(), nil
}

func dateIn(year int, month time.Month, day int) time.Time {
	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if t.Month() != month {
		t = t.AddDate(0, 0, -1)
	}
	return t
}

type BirthdayYear struct {
	Label string
	Start time.Time
	End   time.Time
}

func BirthdayYearContaining(t time.Time, month time.Month, day int) BirthdayYear {
	start := dateIn(t.Year(), month, day)
	day0 := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	if day0.Before(start) {
		start = dateIn(t.Year()-1, month, day)
	}
	return BirthdayYear{Label: fmt.Sprintf("%d - %d", start.Year(), start.Year()+1), Start: start, End: start.AddDate(1, 0, 0)}
}

func (y BirthdayYear) Occurrence(month time.Month, day int) time.Time {
	t := dateIn(y.Start.Year(), month, day)
	if t.Before(y.Start) {
		t = dateIn(y.Start.Year()+1, month, day)
	}
	return t
}

func (y BirthdayYear) Newsletter(birthday time.Time, dates []string) (time.Time, bool) {
	var first, last time.Time
	found, before := false, false
	for _, cell := range dates {
		d, err := ParseDate(cell)
		if err != nil || d.Before(y.Start) || !d.Before(y.End) {
			continue
		}
		if !found {
			first = d
			found = true
		}
		if d.Before(birthday) {
			last = d
			before = true
		}
	}
	if before {
		return last, true
	}
	return first, found
}

func RequestBy(newsletter time.Time, lead int) time.Time {
	return newsletter.AddDate(0, 0, -lead)
}

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
