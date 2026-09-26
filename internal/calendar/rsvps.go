package calendar

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"

	"heliosian/internal/auth"
)

// An RSVP is an invitation waiting for someone's reply, as the shared
// toolbar lists it in every app: the event's title, its start, and its page
// on the calendar, a path there.
type RSVP struct {
	Title  string `json:"title"`
	Start  string `json:"start"`
	AllDay bool   `json:"allDay"`
	Path   string `json:"path"`
}

// waiting is what a person owes a reply, as My Events' RSVP has it: the
// events their household is invited to, still to come or under way, not
// called off, that they do not host and have not answered - soonest first.
func (a app) waiting(email string) []RSVP {
	email = normalizeEmail(email)
	model := a.cache.Model()
	answers := model.Answers[email]
	today := now().Format(DateFormat)
	events := []*Event{}
	for _, e := range model.eventsFor(a.directory, email, a.linked(email)) {
		if !e.Invited || e.Cancelled || answers[e.ID] != "" || e.end.Format(DateFormat) < today || a.isHost(email, false, e) {
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

// rsvps is GET /api/apps/rsvp, served on every app's host (Hooks.RSVPs)
// for the shared toolbar's badge: the invitations waiting for the viewer's
// reply.
func (a app) rsvps(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(struct {
		Waiting []RSVP `json:"waiting"`
	}{a.waiting(auth.Email(r))}); err != nil {
		slog.ErrorContext(r.Context(), "encode rsvps", "error", err)
	}
}
