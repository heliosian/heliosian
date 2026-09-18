package calendar

import (
	"strings"
	"time"
)

const (
	icsStamp   = "20060102T150405Z"
	icsDate    = "20060102"
	icsLineMax = 75
)

var icsEscaper = strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`)

func icsText(s string) string {
	return icsEscaper.Replace(s)
}

// fold breaks a content line at 75 octets, continuing on a line that starts
// with a space, without splitting a multi-byte character.
func fold(line string) []string {
	out := []string{}
	current := strings.Builder{}
	limit := icsLineMax
	for _, r := range line {
		size := len(string(r))
		if current.Len()+size > limit {
			out = append(out, current.String())
			current.Reset()
			current.WriteByte(' ')
			limit = icsLineMax
		}
		current.WriteRune(r)
	}
	out = append(out, current.String())
	return out
}

func uidOf(id string) string {
	if strings.Contains(id, "@") {
		return id
	}
	return id + "@calendar.heliosian.com"
}

// ICS is a feed as a calendar app reads it: the sheet's events and the
// other apps' folded in, read for the feed's owner - so a feed can carry
// the parties and HCA events, and the owner's household's standing with
// them as Going or Waitlisted - each carried when the feed's classrooms
// and tags admit it.
func ICS(model *Model, f *Feed, linked []Linked, origin string, now time.Time) []byte {
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Heliosian//Helios When//EN",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"X-WR-CALNAME:" + icsText(f.Name),
		"X-WR-TIMEZONE:" + Location.String(),
		"REFRESH-INTERVAL;VALUE=DURATION:PT1H",
		"X-PUBLISHED-TTL:PT1H",
	}
	stamp := now.UTC().Format(icsStamp)
	for _, e := range model.eventsFor(f.Email, linked) {
		// The owner's own no, like their hiding it, keeps an event out.
		if answer := model.AnswerOf(f.Email, e.ID); !f.Carries(e) || answer == AnswerHidden || answer == AnswerNo {
			continue
		}
		lines = append(lines, "BEGIN:VEVENT", "UID:"+uidOf(e.ID), "DTSTAMP:"+stamp)
		if e.AllDay {
			lines = append(lines, "DTSTART;VALUE=DATE:"+e.start.Format(icsDate), "DTEND;VALUE=DATE:"+e.end.AddDate(0, 0, 1).Format(icsDate))
		} else {
			lines = append(lines, "DTSTART:"+e.start.UTC().Format(icsStamp))
			if e.end.After(e.start) {
				lines = append(lines, "DTEND:"+e.end.UTC().Format(icsStamp))
			}
		}
		if modified, err := time.ParseInLocation(DateTimeFormat, e.Updated, Location); err == nil {
			lines = append(lines, "LAST-MODIFIED:"+modified.UTC().Format(icsStamp))
		}
		lines = append(lines, "SUMMARY:"+icsText(e.Title))
		description := e.Description
		if e.DayType != "" {
			description = strings.TrimSpace(e.DayType + "\n\n" + description)
		}
		if description != "" {
			lines = append(lines, "DESCRIPTION:"+icsText(description))
		}
		if e.Location != "" {
			lines = append(lines, "LOCATION:"+icsText(e.Location))
		}
		if len(e.Tags) > 0 {
			lines = append(lines, "CATEGORIES:"+icsText(JoinList(e.Tags)))
		}
		lines = append(lines, "URL:"+origin+EventPath(e), "END:VEVENT")
	}
	lines = append(lines, "END:VCALENDAR")
	out := strings.Builder{}
	for _, line := range lines {
		for _, piece := range fold(line) {
			out.WriteString(piece)
			out.WriteString("\r\n")
		}
	}
	return []byte(out.String())
}
