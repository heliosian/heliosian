package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"heliosian/internal/mail"
	"heliosian/internal/store"
)

func (a app) deleteInvitation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor, e, ok := a.hostedEvent(w, r, body.ID)
	if !ok {
		return
	}
	inv := a.cache.Model().Invitations[e.ID]
	own := e.Source == SourceSheet
	if own && inv != nil && inv.Sent != "" {
		http.Error(w, "the invites are out: cancel the event instead", http.StatusBadRequest)
		return
	}
	op := store.Delete(InvitationsTab, store.Row{"Event ID": e.ID})
	if own {
		op = store.Delete(EventsTab, store.Row{"Event ID": e.ID})
	}
	if !a.commit(w, r, actor, op) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: invitation deleted", "actor", actor, "event", e.ID, "title", e.Title, "event too", own)
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
	actor, e, ok := a.hostedEvent(w, r, body.ID)
	if !ok {
		return
	}
	if e.Source != SourceSheet {
		http.Error(w, "an event another app runs is cancelled there", http.StatusBadRequest)
		return
	}
	if e.Cancelled {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	note := strings.TrimSpace(body.Note)
	if len(note) > maxTextLength {
		http.Error(w, "the note is too long", http.StatusBadRequest)
		return
	}
	model := a.cache.Model()
	targets := []string{}
	cc := map[string][]string{}
	if body.Notify {
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
	if !a.commit(w, r, actor, store.Update(EventsTab, store.Row{"Event ID": e.ID}, store.Row{"Status": StatusCancelled})) {
		return
	}
	hostName := actor
	if p, known := a.directory.Person(actor); known && p.Name != "" {
		hostName = p.Name
	}
	replyTo := a.hostsOf(e)
	if !slices.Contains(replyTo, actor) {
		replyTo = append([]string{actor}, replyTo...)
	}
	for _, to := range targets {
		go a.sendCancellation(context.WithoutCancel(r.Context()), to, cc[to], replyTo, hostName, note, model.invitedEvent(e))
	}
	slog.InfoContext(r.Context(), "calendar: event cancelled", "actor", actor, "event", e.ID, "title", e.Title, "told", len(targets))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"told": len(targets)})
}

func (a app) sendCancellation(ctx context.Context, to string, cc, replyTo []string, hostName, note string, e *Event) {
	if a.mail.Sender == nil {
		return
	}
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
	var text strings.Builder
	fmt.Fprintf(&text, "%s\n", lead)
	if note != "" {
		fmt.Fprintf(&text, "\n%s\n", note)
	}
	fmt.Fprintf(&text, "\n%s\n", foot)
	font := "-apple-system,Segoe UI,Roboto,sans-serif"
	var htm strings.Builder
	htm.WriteString("<div style=\"max-width:560px\">")
	fmt.Fprintf(&htm, "<p style=\"font:700 22px/1.3 %s;color:#b3261e;margin:0 0 6px\">Cancelled</p>", font)
	fmt.Fprintf(&htm, "<p style=\"font:15px/1.5 %s;margin:0 0 12px\">%s</p>", font, html.EscapeString(lead))
	if note != "" {
		fmt.Fprintf(&htm, "<blockquote style=\"margin:0 0 14px;padding:10px 16px;border-left:3px solid #0e4d54;font:15px/1.5 %s;white-space:pre-wrap\">%s</blockquote>", font, html.EscapeString(note))
	}
	fmt.Fprintf(&htm, "<p style=\"font:13px/1.5 %s;color:#647071\">%s</p>", font, html.EscapeString(foot))
	htm.WriteString("</div>")
	msg := mail.Message{
		To:      []string{to},
		CC:      cc,
		ReplyTo: replyTo,
		Subject: "[" + e.Title + "] Cancelled",
		Text:    text.String(),
		HTML:    htm.String(),
	}
	if len(cc) == 0 {
		msg.Attachments = []mail.Attachment{{
			Name:        "cancel.ics",
			ContentType: "text/calendar; method=CANCEL; charset=utf-8",
			Content:     []byte(cancellation(a.organizer(e.ID, to), to, e, now())),
		}}
	}
	if err := a.mail.Sender.Send(ctx, msg); err != nil {
		slog.ErrorContext(ctx, "[ERROR] calendar: send cancellation", "to", to, "event", e.ID, "error", err)
		return
	}
	slog.InfoContext(ctx, "calendar: cancellation sent", "to", to, "event", e.ID)
}

func cancellation(from, to string, e *Event, at time.Time) string {
	stamp := at.UTC().Format(icsStamp)
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Heliosian//Helios When//EN",
		"METHOD:CANCEL",
		"BEGIN:VEVENT",
		"UID:" + uidOf(e.ID),
		"DTSTAMP:" + stamp,
		"SEQUENCE:" + fmt.Sprint(at.Unix()),
		"STATUS:CANCELLED",
	}
	if e.AllDay {
		lines = append(lines, "DTSTART;VALUE=DATE:"+e.start.Format(icsDate))
	} else {
		lines = append(lines, "DTSTART:"+e.start.UTC().Format(icsStamp))
	}
	lines = append(lines,
		"SUMMARY:"+icsText("Cancelled: "+e.Title),
		"ORGANIZER;CN=Helios When:mailto:"+mailAddress(from),
		"ATTENDEE;CN="+icsText(to)+":mailto:"+to,
		"END:VEVENT",
		"END:VCALENDAR",
	)
	out := strings.Builder{}
	for _, line := range lines {
		for _, piece := range fold(line) {
			out.WriteString(piece)
			out.WriteString("\r\n")
		}
	}
	return out.String()
}
