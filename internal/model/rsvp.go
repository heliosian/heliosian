package model

import (
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/mail"
)

func (a calendarApp) eventFor(actor access.Actor, id string) *Event {
	return a.lookup(actor.Email, id, func(e *Event) bool { return a.sees(actor, e) })
}

func (a calendarApp) anyEvent(email, id string) *Event {
	return a.lookup(email, id, func(*Event) bool { return true })
}

func (a calendarApp) lookup(email, key string, sees func(*Event) bool) *Event {
	model := a.model()
	key = a.canonical(key)
	for _, e := range withLinked(model.Events, a.linked(email)) {
		if e.ID == key || (e.Address != "" && e.Address == key) {
			return model.withInvitation(e)
		}
	}
	e := model.Event(key)
	if e == nil || !sees(e) {
		return nil
	}
	return model.withInvitation(e)
}

func (a calendarApp) canonical(key string) string {
	return a.model().aliases.Resolve(key)
}

func (a calendarApp) sees(actor access.Actor, e *Event) bool {
	if actor.May(SeeAllEvents) || mail.Normalize(e.AddedBy) == actor.Email || a.isHost(actor, e) {
		return true
	}
	if e.Sharing == SharingInvited {
		return a.model().Listed(a.directory(), actor.Email, e.ID)
	}
	return true
}

type rsvpBody struct {
	ID     string `json:"id"`
	Answer string `json:"answer"`
}

func (a calendarApp) organizer(id, email string) string {
	address := mail.AddressOf(a.mail.ReplyTo)
	local, domain, _ := strings.Cut(address, "@")
	return strings.Replace(a.mail.ReplyTo, address, local+"+"+a.replyToken(id, email)+"@"+domain, 1)
}

func (e *Event) mailEvent(link string) mail.Event {
	m := mail.Event{UID: uidOf(e.uidKey()), Start: e.start, End: e.end, AllDay: e.AllDay, Summary: e.Title, Description: strings.TrimSpace(e.Description + "\n\n" + link), Location: e.Location, URL: link}
	if e.AllDay {
		m.End = e.end.AddDate(0, 0, 1)
	}
	return m
}

func (a calendarApp) invite(e *Event, to, link, method string) mail.Attachment {
	m := e.mailEvent(link)
	if method == mail.MethodCancel {
		m.Summary = "Cancelled: " + e.Title
	}
	m.Organizer = mail.Person{Name: brand.Name, Email: a.organizer(e.uidKey(), to)}
	m.Attendees = []mail.Person{{Name: to, Email: to}}
	m.RSVP = method == mail.MethodRequest
	return mail.Calendar{Product: brand.Name, Method: method, Stamp: now(), Events: []mail.Event{m}}.Attachment()
}
