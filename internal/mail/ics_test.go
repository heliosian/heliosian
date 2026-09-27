package mail

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestFoldAndEscape(t *testing.T) {
	long := "DESCRIPTION:" + strings.Repeat("ünïcödé ", 30)
	folded := fold(long)
	for _, piece := range strings.Split(folded, "\r\n") {
		if len(piece) > 75 || !utf8.ValidString(piece) {
			t.Errorf("fold broke a line: %q", piece)
		}
	}
	if strings.ReplaceAll(folded, "\r\n ", "") != long {
		t.Errorf("fold lost text")
	}
	if got := escape("a;b,c\\d\nline"); got != `a\;b\,c\\d\nline` {
		t.Errorf("escape = %q", got)
	}
}

func TestCalendarWritesOneEvent(t *testing.T) {
	loc, _ := time.LoadLocation("America/Los_Angeles")
	start, end, allDay, ok := Span("2026-10-03 18:00", "", loc)
	if !ok || allDay || end.Sub(start) != 2*time.Hour {
		t.Fatalf("span %v %v %v %v", start, end, allDay, ok)
	}
	e := Event{UID: "x@heliosian.com", Start: start, End: end, Summary: "Picnic, in the park", URL: "https://example.org/e/x", Organizer: Person{Name: "Helios When", Email: "Helios When <when@example.org>"}, Attendees: []Person{{Name: "Ana", Email: "ana@example.org"}}, RSVP: true}
	c := Calendar{Product: "Helios When", Method: MethodRequest, Stamp: time.Unix(0, 0), Events: []Event{e}}
	ics := strings.ReplaceAll(c.ICS(), "\r\n ", "")
	for _, want := range []string{"PRODID:-//Heliosian//Helios When//EN", "METHOD:REQUEST", "STATUS:CONFIRMED", "DTSTART:20261004T010000Z", "DTEND:20261004T030000Z", `SUMMARY:Picnic\, in the park`, "ORGANIZER;CN=Helios When:mailto:when@example.org", "ATTENDEE;CN=Ana;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:ana@example.org"} {
		if !strings.Contains(ics, want) {
			t.Errorf("invite lacks %q:\n%s", want, ics)
		}
	}
	if a := c.Attachment(); a.Name != "invite.ics" || a.ContentType != "text/calendar; method=REQUEST; charset=utf-8" {
		t.Errorf("attachment %s %s", a.Name, a.ContentType)
	}
	day, until, allDay, _ := Span("2026-10-03", "2026-10-04", loc)
	if !allDay || until.Sub(day) != 48*time.Hour {
		t.Errorf("date span %v %v %v", day, until, allDay)
	}
}
