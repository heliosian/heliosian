package mail

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	MethodRequest = "REQUEST"
	MethodCancel  = "CANCEL"
	MethodPublish = "PUBLISH"

	icsStamp = "20060102T150405Z"
	icsDate  = "20060102"
)

type Calendar struct {
	Product  string
	Method   string
	Name     string
	TimeZone string
	Stamp    time.Time
	Events   []Event
}

type Event struct {
	UID         string
	Start, End  time.Time
	AllDay      bool
	Modified    time.Time
	Summary     string
	Description string
	Location    string
	Categories  string
	URL         string
	Organizer   Person
	Attendees   []Person
	RSVP        bool
	Transparent bool
	Alarm       time.Duration
}

type Person struct {
	Name, Email string
}

func (c Calendar) ICS() string {
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Heliosian//" + c.Product + "//EN",
		"CALSCALE:GREGORIAN",
		"METHOD:" + c.Method,
	}
	if c.Name != "" {
		lines = append(lines, "X-WR-CALNAME:"+escape(c.Name))
	}
	if c.TimeZone != "" {
		lines = append(lines, "X-WR-TIMEZONE:"+c.TimeZone)
	}
	if c.Method == MethodPublish {
		lines = append(lines, "REFRESH-INTERVAL;VALUE=DURATION:PT1H", "X-PUBLISHED-TTL:PT1H")
	}
	for _, e := range c.Events {
		lines = append(lines, c.event(e)...)
	}
	lines = append(lines, "END:VCALENDAR")
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(fold(line) + "\r\n")
	}
	return b.String()
}

func (c Calendar) event(e Event) []string {
	lines := []string{"BEGIN:VEVENT", "UID:" + escape(e.UID), "DTSTAMP:" + c.Stamp.UTC().Format(icsStamp)}
	if c.Method != MethodPublish {
		lines = append(lines, "SEQUENCE:"+fmt.Sprint(c.Stamp.Unix()))
	}
	switch c.Method {
	case MethodRequest:
		lines = append(lines, "STATUS:CONFIRMED")
	case MethodCancel:
		lines = append(lines, "STATUS:CANCELLED")
	}
	if e.AllDay {
		lines = append(lines, "DTSTART;VALUE=DATE:"+e.Start.Format(icsDate))
		if e.End.After(e.Start) {
			lines = append(lines, "DTEND;VALUE=DATE:"+e.End.Format(icsDate))
		}
	} else {
		lines = append(lines, "DTSTART:"+e.Start.UTC().Format(icsStamp))
		if e.End.After(e.Start) {
			lines = append(lines, "DTEND:"+e.End.UTC().Format(icsStamp))
		}
	}
	if !e.Modified.IsZero() {
		lines = append(lines, "LAST-MODIFIED:"+e.Modified.UTC().Format(icsStamp))
	}
	lines = append(lines, "SUMMARY:"+escape(e.Summary))
	if e.Description != "" {
		lines = append(lines, "DESCRIPTION:"+escape(e.Description))
	}
	if e.Location != "" {
		lines = append(lines, "LOCATION:"+escape(e.Location))
	}
	if e.Categories != "" {
		lines = append(lines, "CATEGORIES:"+escape(e.Categories))
	}
	if e.URL != "" {
		lines = append(lines, "URL:"+e.URL)
	}
	if e.Organizer.Email != "" {
		lines = append(lines, "ORGANIZER"+cn(e.Organizer)+":mailto:"+AddressOf(e.Organizer.Email))
	}
	answer := "PARTSTAT=ACCEPTED;RSVP=FALSE"
	if e.RSVP {
		answer = "PARTSTAT=NEEDS-ACTION;RSVP=TRUE"
	}
	for _, p := range e.Attendees {
		lines = append(lines, "ATTENDEE"+cn(p)+";ROLE=REQ-PARTICIPANT;"+answer+":mailto:"+p.Email)
	}
	if e.Transparent {
		lines = append(lines, "TRANSP:TRANSPARENT")
	}
	if e.Alarm > 0 {
		lines = append(lines, "BEGIN:VALARM", "ACTION:DISPLAY", "DESCRIPTION:"+escape(e.Summary), fmt.Sprintf("TRIGGER:-PT%dM", int(e.Alarm.Minutes())), "END:VALARM")
	}
	return append(lines, "END:VEVENT")
}

func cn(p Person) string {
	if p.Name == "" {
		return ""
	}
	return ";CN=" + escape(p.Name)
}

func (c Calendar) Attachment() Attachment {
	name := "invite.ics"
	if c.Method == MethodCancel {
		name = "cancel.ics"
	}
	return Attachment{Name: name, ContentType: "text/calendar; method=" + c.Method + "; charset=utf-8", Content: []byte(c.ICS())}
}

func (e Event) GoogleLink() string {
	stamp := func(t time.Time) string {
		if e.AllDay {
			return t.Format(icsDate)
		}
		return t.UTC().Format(icsStamp)
	}
	until := e.End
	switch {
	case until.After(e.Start):
	case e.AllDay:
		until = e.Start.AddDate(0, 0, 1)
	default:
		until = e.Start.Add(time.Hour)
	}
	q := url.Values{
		"action": {"TEMPLATE"}, "text": {e.Summary}, "dates": {stamp(e.Start) + "/" + stamp(until)}, "details": {e.Description}, "location": {e.Location},
	}
	return "https://calendar.google.com/calendar/render?" + q.Encode()
}

func Span(start, end string, loc *time.Location) (from, until time.Time, allDay, ok bool) {
	from, err := time.ParseInLocation(dateTimeFormat, start, loc)
	allDay = err != nil
	if allDay {
		if from, err = time.ParseInLocation(dateFormat, start, loc); err != nil {
			return from, until, false, false
		}
	}
	if t, err := time.ParseInLocation(dateTimeFormat, end, loc); err == nil && !allDay && t.After(from) {
		return from, t, false, true
	}
	if t, err := time.ParseInLocation(dateFormat, end, loc); err == nil && allDay && !t.Before(from) {
		return from, t.AddDate(0, 0, 1), true, true
	}
	if allDay {
		return from, from.AddDate(0, 0, 1), true, true
	}
	return from, from.Add(2 * time.Hour), false, true
}

const (
	dateFormat     = "2006-01-02"
	dateTimeFormat = "2006-01-02 15:04"
)

var escaper = strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`)

func escape(s string) string {
	return escaper.Replace(s)
}

func fold(line string) string {
	var b strings.Builder
	count := 0
	for _, r := range line {
		size := len(string(r))
		if count+size > 75 {
			b.WriteString("\r\n ")
			count = 1
		}
		b.WriteRune(r)
		count += size
	}
	return b.String()
}
