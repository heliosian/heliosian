package calendar

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/mail"
)

// An answer is a person's word on an event: yes, no, maybe, or hidden. Yes,
// no and maybe are marks on the event wherever it is listed for them;
// hidden takes the event out of their lists - Upcoming here and on
// Heliosian, their personal feeds - while it stays on the month in gray and
// turns up in the search. A yes also sends them a calendar invite, so the
// event lands on their own calendar with one tap of Accept - unless an
// invitation already brought them one (invites.go).

// Answerer records an answer for a person, for the calendar's own page and
// for Heliosian's cards alike: Register hands one back.
type Answerer func(ctx context.Context, email, id, answer string) error

// answer validates and records one, and sends the invite for a yes.
func (a app) answer(ctx context.Context, email, id, answer string) error {
	return a.record(ctx, email, id, answer, true)
}

// record keeps one answer a person gives for themselves on a page.
func (a app) record(ctx context.Context, email, id, answer string, invite bool) error {
	return a.recordBy(ctx, email, email, id, answer, ViaPage, invite)
}

// recordBy keeps one answer, in the model at once and in the sheet behind
// it, noting who gave it when it was not the person themselves - a parent
// for a child, a host - and how, a page or a calendar app's reply, and
// sends the invite for a yes when asked - not for a yes that came back
// as a reply to the invite itself, nor to someone whose invitation
// already carried one.
func (a app) recordBy(ctx context.Context, actor, email, id, answer, via string, invite bool) error {
	email = normalizeEmail(email)
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "" && !isAnswer(answer) {
		return fmt.Errorf("an answer is yes, no, maybe, or hidden")
	}
	e := a.eventFor(email, id)
	if e == nil {
		return fmt.Errorf("that event is not on the calendar")
	}
	cells := map[string]string{"Email": email, "Event ID": e.ID, "Answer": answer, "Answered": now().Format(DateTimeFormat), "Answered By": actor, "Via": via}
	if inv := a.cache.Model().InviteOf(e.ID, email); inv != nil && inv.Sent != "" {
		invite = false
	}
	tables := a.cache.Tables().WithAnswer(email, e.ID, answer, cells)
	model, err := BuildModel(tables, a.cache.roster())
	if err != nil {
		return err
	}
	a.cache.set(tables, model)
	a.queue.Add(func() {
		var err error
		if answer == "" {
			err = a.writer.Delete(appName, RSVPsTab, map[string]string{"Email": email, "Event ID": e.ID})
		} else {
			err = a.writer.Set(appName, RSVPsTab, map[string]string{"Email": email, "Event ID": e.ID}, cells)
		}
		if err != nil {
			slog.ErrorContext(ctx, "calendar write", "error", err)
		}
	})
	if invite && answer == AnswerYes && a.mail.Sender != nil && !isGuestKey(email) {
		go a.sendInvite(context.WithoutCancel(ctx), email, e)
	}
	return nil
}

// eventFor is an event as this person sees it, the other apps' folded in.
func (a app) eventFor(email, id string) *Event {
	model := a.cache.Model()
	for _, e := range withLinked(model.Events, a.linked(email)) {
		if e.ID == id {
			return model.withInvitation(e)
		}
	}
	// A direct-link event takes an answer from anyone with its link, and so
	// does one waiting for approval - its link works in the meantime; a
	// declined one, only from the person who shared it.
	if e := model.Event(id); e != nil && (e.InviteOnly || e.Pending || normalizeEmail(e.AddedBy) == email) {
		return model.withInvitation(e)
	}
	return nil
}

// rsvp is the calendar's own route: the viewer answering for themselves.
func (a app) rsvp(w http.ResponseWriter, r *http.Request) {
	email, _ := a.who(r)
	var body struct {
		ID     string `json:"id"`
		Answer string `json:"answer"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := a.answer(r.Context(), email, body.ID, body.Answer); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	slog.InfoContext(r.Context(), "calendar: answered", "actor", email, "event", body.ID, "answer", body.Answer)
	w.WriteHeader(http.StatusNoContent)
}

// sendInvite mails the person a calendar invite for the event: a message
// with the event's words and its page, and an .ics they accept into their
// own calendar. The invite's UID is the feed's for the event, so an event
// they also take by feed is one entry, not two; its organizer is the reply
// address, so an Accept or Decline in their calendar app comes back here
// (replies.go).
func (a app) sendInvite(ctx context.Context, email string, e *Event) {
	origin := "https://when.heliosian.com"
	link := origin + EventPath(e)
	day, hours := whenLines(e)
	when := day
	if hours != "" {
		when += " · " + hours
	}
	var text strings.Builder
	fmt.Fprintf(&text, "You said yes to %s.\n\n%s\n", e.Title, when)
	if e.Location != "" {
		fmt.Fprintf(&text, "%s\n", e.Location)
	}
	fmt.Fprintf(&text, "\nThe invite attached puts it on your calendar. The event's page: %s\n", link)
	var htm strings.Builder
	fmt.Fprintf(&htm, "<p style=\"font:16px/1.5 -apple-system,Segoe UI,Roboto,sans-serif\">You said yes to <strong>%s</strong>.</p>", html.EscapeString(e.Title))
	fmt.Fprintf(&htm, "<p style=\"font:15px/1.5 -apple-system,Segoe UI,Roboto,sans-serif;color:#0e4d54\">%s", html.EscapeString(when))
	if e.Location != "" {
		fmt.Fprintf(&htm, "<br>%s", html.EscapeString(e.Location))
	}
	htm.WriteString("</p>")
	fmt.Fprintf(&htm, "<p style=\"margin:20px 0\"><a href=\"%s\" style=\"display:inline-block;padding:10px 18px;border-radius:8px;background:#0e4d54;color:#fff;font:700 15px -apple-system,Segoe UI,Roboto,sans-serif;text-decoration:none\">Open the event</a></p>", html.EscapeString(link))
	htm.WriteString("<p style=\"font:13px/1.5 -apple-system,Segoe UI,Roboto,sans-serif;color:#647071\">The invite attached puts it on your calendar.</p>")
	err := a.mail.Sender.Send(ctx, mail.Message{
		To:      []string{email},
		ReplyTo: a.replyTo(e),
		Subject: "Invitation: " + e.Title + " · " + day,
		Text:    text.String(),
		HTML:    htm.String(),
		Attachments: []mail.Attachment{{
			Name:        "invite.ics",
			ContentType: "text/calendar; method=REQUEST; charset=utf-8",
			Content:     []byte(invite(a.organizer(), email, e, link, now())),
		}},
	})
	if err != nil {
		slog.ErrorContext(ctx, "calendar: send invite", "to", email, "event", e.ID, "error", err)
		return
	}
	slog.InfoContext(ctx, "calendar: invite sent", "to", email, "event", e.ID)
}

// organizer is the address the invites name as their organizer, where a
// calendar app sends its reply: the reply address, else the from address.
func (a app) organizer() string {
	if a.mail.ReplyTo != "" {
		return a.mail.ReplyTo
	}
	return a.mail.From
}

// invite is the calendar file: the one event, from the calendar to the
// person, as a request they accept - the organizer being where their
// calendar app sends the answer.
func invite(from, to string, e *Event, link string, at time.Time) string {
	stamp := at.UTC().Format(icsStamp)
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Heliosian//Helios When//EN",
		"METHOD:REQUEST",
		"BEGIN:VEVENT",
		"UID:" + uidOf(e.ID),
		"DTSTAMP:" + stamp,
		"SEQUENCE:" + fmt.Sprint(at.Unix()),
	}
	if e.AllDay {
		lines = append(lines, "DTSTART;VALUE=DATE:"+e.start.Format(icsDate), "DTEND;VALUE=DATE:"+e.end.AddDate(0, 0, 1).Format(icsDate))
	} else {
		lines = append(lines, "DTSTART:"+e.start.UTC().Format(icsStamp))
		if e.end.After(e.start) {
			lines = append(lines, "DTEND:"+e.end.UTC().Format(icsStamp))
		}
	}
	lines = append(lines, "SUMMARY:"+icsText(e.Title))
	description := strings.TrimSpace(e.Description + "\n\n" + link)
	lines = append(lines, "DESCRIPTION:"+icsText(description))
	if e.Location != "" {
		lines = append(lines, "LOCATION:"+icsText(e.Location))
	}
	lines = append(lines,
		"URL:"+link,
		"ORGANIZER;CN=Helios When:mailto:"+mailAddress(from),
		"ATTENDEE;CN="+icsText(to)+";ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:"+to,
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

// mailAddress is the address inside "Helios When <when@reply.heliosian.com>",
// or the string itself when it is bare.
func mailAddress(from string) string {
	if i := strings.LastIndex(from, "<"); i >= 0 {
		return strings.TrimSuffix(from[i+1:], ">")
	}
	return from
}
