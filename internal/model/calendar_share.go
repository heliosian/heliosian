package model

import (
	"strings"

	"heliosian/internal/sharecard"
)

const defaultHeader = "brand/default-header.jpg"

func whenLines(e *Event) (string, string) {
	if e.AllDay || spansDays(e) {
		if e.start.Equal(e.end) || e.start.Format(DateFormat) == e.end.Format(DateFormat) {
			return e.start.Format("Monday, January 2"), ""
		}
		return e.start.Format("Monday, January 2") + " – " + e.end.Format("Monday, January 2"), ""
	}
	return e.start.Format("Monday, January 2"), sharecard.Hours(e.start, e.end)
}

func spansDays(e *Event) bool {
	return e.start.Format(DateFormat) != e.end.Format(DateFormat) && !e.AllDay
}

func when(e *Event) string {
	day, hours := whenLines(e)
	if hours != "" {
		return day + " · " + hours
	}
	return day
}

func blurb(e *Event) string {
	text := strings.Join(strings.Fields(e.Description), " ")
	if len(text) > 200 {
		cut := strings.LastIndex(text[:200], " ")
		if cut < 120 {
			cut = 200
		}
		text = text[:cut] + "…"
	}
	return text
}

func (a calendarApp) events() []*Event {
	return withLinked(a.model().Events, a.linked(""))
}

func (a calendarApp) event(key string) *Event {
	key = a.canonical(key)
	for _, e := range a.events() {
		if e.ID == key {
			return e
		}
	}
	return a.model().Event(key)
}

func (m *Calendar) pictureOf(e *Event) string {
	if e.Image != "" {
		return strings.TrimPrefix(e.Image, "/")
	}
	for _, key := range e.Tags {
		if t := m.Tag(key); t != nil && t.ImageURL != "" {
			return strings.TrimPrefix(t.ImageURL, "/")
		}
	}
	return defaultHeader
}
