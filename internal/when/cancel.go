package when

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"heliosian/internal/mail"
)

func (a app) deleteInvitation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, e, own, err := a.deleteInvitationOps(actor, body.ID)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: invitation deleted", "actor", actor.Email, "event", e.ID, "title", e.Title, "event too", own)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"event": own})
}

func (a app) cancelEvent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID     string `json:"id"`
		Notify bool   `json:"notify"`
		Note   string `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, e, err := a.cancelOps(actor, body.ID, body.Note)
	if err != nil {
		refuse(w, err)
		return
	}
	if len(ops) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	note := strings.TrimSpace(body.Note)
	model := a.cache.Model()
	targets := []string{}
	cc := map[string][]string{}
	if body.Notify && a.mail.Sender != nil {
		for _, inv := range model.Invites[e.ID] {
			if inv.Sent == "" || isGuestKey(inv.Email) || slices.Contains(targets, inv.Email) {
				continue
			}
			with, reachable := a.ccFor(inv.Email)
			if !reachable {
				continue
			}
			cc[inv.Email] = with
			targets = append(targets, inv.Email)
		}
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	hostName := actor.Email
	if p := a.directory().Person(actor.Email); p != nil && p.FullName != "" {
		hostName = p.FullName
	}
	replyTo := a.hostsOf(e)
	if !slices.Contains(replyTo, actor.Email) {
		replyTo = append([]string{actor.Email}, replyTo...)
	}
	for _, to := range targets {
		go a.sendCancellation(context.WithoutCancel(r.Context()), to, cc[to], replyTo, hostName, note, model.invitedEvent(e))
	}
	slog.InfoContext(r.Context(), "calendar: event cancelled", "actor", actor.Email, "event", e.ID, "title", e.Title, "told", len(targets))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"told": len(targets)})
}

func (a app) sendCancellation(ctx context.Context, to string, cc, replyTo []string, hostName, note string, e *Event) {
	day, hours := whenLines(e)
	when := day
	if hours != "" {
		when += ", " + hours
	}
	lead := fmt.Sprintf("%s has cancelled %s, which was on %s.", hostName, e.Title, when)
	foot := "A reply reaches the hosts."
	if len(cc) == 0 {
		foot = "The cancellation attached takes it off your calendar. " + foot
	}
	l := a.letterFor(e, EventPath(e))
	l.Heading = "Cancelled"
	l.Intro = lead
	l.Note = note
	l.Footnote = foot
	msg := l.Message("["+e.Title+"] Cancelled", []string{to}, cc, replyTo)
	if len(cc) == 0 {
		msg.Attachments = []mail.Attachment{a.invite(e, to, l.Path, mail.MethodCancel)}
	}
	if err := a.mail.Sender.Send(ctx, msg); err != nil {
		slog.ErrorContext(ctx, "calendar: send cancellation", "to", to, "event", e.ID, "error", err)
		return
	}
	slog.InfoContext(ctx, "calendar: cancellation sent", "to", to, "event", e.ID)
}
