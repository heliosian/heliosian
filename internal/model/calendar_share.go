package model

import "strings"

const defaultHeader = "brand/default-header.jpg"

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
