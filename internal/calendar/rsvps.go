package calendar

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/config"
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
	for _, e := range model.eventsFor(a.directory, email, a.linked(email)) {
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

func (a app) rsvps(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(struct {
		Waiting []RSVP `json:"waiting"`
	}{a.waiting(auth.Email(r))}); err != nil {
		slog.ErrorContext(r.Context(), "encode rsvps", "error", err)
	}
}
