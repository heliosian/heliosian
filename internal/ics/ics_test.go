package ics

import (
	"strings"
	"testing"
	"time"
)

const feed = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
DTSTART;VALUE=DATE:20251222
DTEND;VALUE=DATE:20251227
UID:break@example
SUMMARY:Winter Break - No School
STATUS:CONFIRMED
END:VEVENT
BEGIN:VEVENT
DTSTART:20251016T230000Z
DTEND:20251017T010000Z
UID:night@example
LAST-MODIFIED:20250901T120000Z
SEQUENCE:2
LOCATION:Gym\, Main Campus
DESCRIPTION:Line one\nLine two
SUMMARY:International Night
BEGIN:VALARM
TRIGGER:-PT15H
UID:alarm@example
END:VALARM
END:VEVENT
BEGIN:VEVENT
DTSTART;TZID=America/Los_Angeles:20260414T110000
DTEND;TZID=America/Los_Angeles:20260414T151500
RRULE:FREQ=WEEKLY;WKST=SU;UNTIL=20260421T065959Z;BYDAY=TU,WE,TH,FR
UID:projects@example
SUMMARY:Passion Projects
END:VEVENT
BEGIN:VEVENT
DTSTART;TZID=America/Los_Angeles:20260416T110000
DTEND;TZID=America/Los_Angeles:20260416T120000
UID:projects@example
RECURRENCE-ID;TZID=America/Los_Angeles:20260416T110000
SUMMARY:Passion Projects (short)
END:VEVENT
BEGIN:VEVENT
DTSTART;VALUE=DATE:20250101
DTEND;VALUE=DATE:20250102
UID:old@example
SUMMARY:Too old
END:VEVENT
BEGIN:VEVENT
DTSTART:20251111T030000Z
DTEND:20251111T043000Z
UID:monthly@example
RRULE:FREQ=MONTHLY;COUNT=3;BYDAY=2MO
SUMMARY:Board Meeting
END:VEVENT
END:VCALENDAR
`

func TestParse(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2025, 8, 1, 0, 0, 0, 0, loc)
	to := time.Date(2027, 8, 1, 0, 0, 0, 0, loc)
	events, err := Parse(strings.NewReader(strings.ReplaceAll(feed, "\n", "\r\n")), loc, from, to)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]Event{}
	for _, e := range events {
		byKey[e.Key] = e
	}

	night := byKey["night@example"]
	if night.Start.Format("2006-01-02 15:04") != "2025-10-16 16:00" || night.End.Format("15:04") != "18:00" {
		t.Errorf("utc times not converted: %v %v", night.Start, night.End)
	}
	if night.Location != "Gym, Main Campus" || night.Description != "Line one\nLine two" {
		t.Errorf("text not unescaped: %q %q", night.Location, night.Description)
	}
	if night.Sequence != 2 || night.Modified.IsZero() {
		t.Errorf("sequence or modified missing: %+v", night)
	}

	brk := byKey["break@example"]
	if !brk.AllDay || brk.Start.Day() != 22 || brk.End.Day() != 27 {
		t.Errorf("all-day range wrong: %+v", brk)
	}

	if _, ok := byKey["old@example"]; ok {
		t.Error("event before the window was returned")
	}

	wantDays := []string{"20260414", "20260415", "20260416", "20260417"}
	for _, day := range wantDays {
		e, ok := byKey["projects@example/"+day+"T110000"]
		if !ok {
			t.Errorf("missing instance %s", day)
			continue
		}
		if !e.Recurring {
			t.Errorf("instance %s not marked recurring", day)
		}
		if day == "20260416" {
			if e.Summary != "Passion Projects (short)" || e.End.Hour() != 12 {
				t.Errorf("exception not applied: %+v", e)
			}
			continue
		}
		if e.End.Hour() != 15 || e.End.Minute() != 15 {
			t.Errorf("instance %s end wrong: %v", day, e.End)
		}
	}
	if _, ok := byKey["projects@example/20260421T110000"]; ok {
		t.Error("instance past UNTIL was returned")
	}

	meetings := []string{}
	for _, e := range events {
		if e.UID == "monthly@example" {
			meetings = append(meetings, e.Start.Format("2006-01-02 15:04"))
		}
	}
	want := "2025-11-10 19:00,2025-12-08 19:00,2026-01-12 19:00"
	if strings.Join(meetings, ",") != want {
		t.Errorf("monthly instances = %v, want %s", meetings, want)
	}
}

func TestParseProperty(t *testing.T) {
	p, err := parseProperty(`DTSTART;TZID="America/Los_Angeles";VALUE=DATE-TIME:20201119T153000`)
	if err != nil {
		t.Fatal(err)
	}
	if p.name != "DTSTART" || p.params["TZID"] != "America/Los_Angeles" || p.params["VALUE"] != "DATE-TIME" || p.value != "20201119T153000" {
		t.Errorf("parsed %+v", p)
	}
	p, err = parseProperty("ATTACH;VALUE=URI:https://example.com/a:b")
	if err != nil {
		t.Fatal(err)
	}
	if p.value != "https://example.com/a:b" {
		t.Errorf("value with colons = %q", p.value)
	}
}
