package when

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"heliosian/internal/mail"
	"heliosian/internal/serve"
)

func (a app) deleteInvitation(r *http.Request, body idBody) (map[string]bool, error) {
	actor := a.actor(r)
	ops, e, own, err := a.deleteInvitationOps(actor, body.ID)
	if err != nil {
		return nil, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return nil, err
	}
	slog.InfoContext(r.Context(), "calendar: invitation deleted", "actor", actor.Email, "event", e.ID, "title", e.Title, "event too", own)
	return map[string]bool{"event": own}, nil
}

type cancelBody struct {
	ID     string `json:"id"`
	Notify bool   `json:"notify"`
	Note   string `json:"note"`
}

func (a app) cancelEvent(r *http.Request, body cancelBody) (any, error) {
	actor := a.actor(r)
	ops, e, err := a.cancelOps(actor, body.ID, body.Note)
	if err != nil {
		return nil, err
	}
	if len(ops) == 0 {
		return serve.None{}, nil
	}
	note := strings.TrimSpace(body.Note)
	model := a.cache.Model()
	sent := []string{}
	if body.Notify {
		for _, inv := range model.Invites[e.ID] {
			if inv.Sent != "" {
				sent = append(sent, inv.Email)
			}
		}
	}
	targets, cc := a.recipients(e, sent)
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return nil, err
	}
	hostName, replyTo := a.senderAndReplyTo(actor, e)
	for _, to := range targets {
		go a.sendCancellation(context.WithoutCancel(r.Context()), to, cc[to], replyTo, hostName, note, model.invitedEvent(e))
	}
	slog.InfoContext(r.Context(), "calendar: event cancelled", "actor", actor.Email, "event", e.ID, "title", e.Title, "told", len(targets))
	return map[string]int{"told": len(targets)}, nil
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
