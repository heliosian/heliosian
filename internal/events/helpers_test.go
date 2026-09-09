package events

import "time"

func now() time.Time {
	return mustTime("2026-09-09")
}

func mustTime(s string) time.Time {
	t, err := time.Parse(DateFormat, s)
	if err != nil {
		panic(err)
	}
	return t
}
