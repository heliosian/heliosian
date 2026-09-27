package when

import (
	"net/http"
	"sort"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/config"
	"heliosian/internal/serve"
)

type RSVP struct {
	Title  string `json:"title"`
	Start  string `json:"start"`
	AllDay bool   `json:"allDay"`
	Path   string `json:"path"`
}

func (a app) waiting(email string) []RSVP {
	email = config.NormalizeEmail(email)
	model := a.cache.Model()
	answers := model.Answers[email]
	today := now().Format(DateFormat)
	events := []*Event{}
	for _, e := range model.eventsFor(a.directory(), email, a.linked(email)) {
		if !e.Invited || e.Cancelled || answers[e.ID] != "" || e.end.Format(DateFormat) < today || a.isHost(access.Actor{Email: email}, e) {
			continue
		}
		events = append(events, e)
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].start.Before(events[j].start) })
	out := []RSVP{}
	for _, e := range events {
		out = append(out, RSVP{Title: e.Title, Start: e.Start, AllDay: e.AllDay, Path: EventPath(e)})
	}
	return out
}

type rsvpsView struct {
	Waiting []RSVP `json:"waiting"`
}

func (a app) rsvps(r *http.Request, _ serve.None) (rsvpsView, error) {
	return rsvpsView{a.waiting(auth.Email(r))}, nil
}
