package calendar

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/mail"
	"heliosian/internal/store"
)

type Answerer func(ctx context.Context, email, id, answer string) error

var errNotRecorded = errors.New("the answer was not recorded")

func (a app) answer(ctx context.Context, email, id, answer string) error {
	return a.record(ctx, email, id, answer, true)
}

func (a app) record(ctx context.Context, email, id, answer string, invite bool) error {
	return a.recordBy(ctx, email, email, id, answer, ViaPage, invite, false)
}

func (a app) recordBy(ctx context.Context, actor, email, id, answer, via string, invite, wait bool) error {
	email = normalizeEmail(email)
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "" && !isAnswer(answer) {
		return fmt.Errorf("an answer is yes, no, maybe, or hidden")
	}
	e := a.eventFor(email, false, id)
	if e == nil {
		return fmt.Errorf("that event is not on the calendar")
	}
	if inv := a.cache.Model().InviteOf(e.ID, email); inv != nil && inv.Sent != "" {
		invite = false
	}
	key := store.Row{"Event ID": e.ID, "Email": email}
	op := store.Delete(RSVPsTab, key)
	if answer != "" {
		op = store.Set(RSVPsTab, key, store.Row{"Answer": answer, "Answered": now().Format(DateTimeFormat), "Answered By": actor, "Via": via})
	}
	commit := a.cache.Commit
	if wait {
		commit = a.cache.CommitAndWait
	}
	if err := commit(ctx, actor, op); err != nil {
		return fmt.Errorf("%w: %w", errNotRecorded, err)
	}
	model := a.cache.Model()
	if invite && answer == AnswerYes && a.mail.Sender != nil && !isGuestKey(email) {
		go a.sendInvite(context.WithoutCancel(ctx), email, e)
	}
	if inv := model.Invitations[e.ID]; inv != nil && a.mail.Sender != nil && answer != "" && answer != AnswerHidden {
		for _, h := range inv.Notify {
			if h != actor {
				go a.sendAnswerNote(context.WithoutCancel(ctx), h, actor, email, answer, model.invitedEvent(e))
			}
		}
	}
	return nil
}

func (a app) sendAnswerNote(ctx context.Context, to, actor, email, answer string, e *Event) {
	model := a.cache.Model()
	name := email
	if p, known := a.directory.Person(email); known && p.Name != "" {
		name = p.Name
	} else if inv := model.InviteOf(e.ID, email); inv != nil && inv.Name != "" {
		name = inv.Name
	}
	by := ""
	if actor != email {
		who := actor
		if p, known := a.directory.Person(actor); known && p.Name != "" {
			who = p.Name
		}
		by = " (answered by " + who + ")"
	}
	yes, maybe, no, waiting := 0, 0, 0, 0
	for _, row := range model.Invites[e.ID] {
		switch model.AnswerOf(row.Email, e.ID) {
		case AnswerYes:
			yes++
		case AnswerMaybe:
			maybe++
		case AnswerNo:
			no++
		default:
			waiting++
		}
	}
	link := "https://when.heliosian.com" + EventPath(e)
	line := fmt.Sprintf("%s said %s to %s%s.", name, answerWord(answer), e.Title, by)
	standing := fmt.Sprintf("So far: %d yes, %d maybe, %d no, %d still to answer.", yes, maybe, no, waiting)
	font := "-apple-system,Segoe UI,Roboto,sans-serif"
	htm := fmt.Sprintf("<p style=\"font:16px/1.5 %s\">%s</p><p style=\"font:14px/1.5 %s;color:#647071\">%s</p><p style=\"margin:16px 0\"><a href=\"%s\" style=\"display:inline-block;padding:10px 18px;border-radius:8px;background:#0e4d54;color:#fff;font:700 15px %s;text-decoration:none\">See the guest list</a></p><p style=\"font:12px/1.5 %s;color:#647071\">You asked to hear as answers come in; turn it off under Who's coming on the event's page.</p>", font, html.EscapeString(line), font, html.EscapeString(standing), html.EscapeString(link), font, font)
	err := a.mail.Sender.Send(ctx, mail.Message{
		To:      []string{to},
		Subject: "[" + e.Title + "] " + name + " said " + answerWord(answer),
		Text:    line + "\n\n" + standing + "\n\n" + link + "\n",
		HTML:    htm,
	})
	if err != nil {
		slog.ErrorContext(ctx, "[ERROR] calendar: send answer note", "to", to, "event", e.ID, "error", err)
	}
}

func (a app) eventFor(email string, admin bool, id string) *Event {
	model := a.cache.Model()
	for _, e := range withLinked(model.Events, a.linked(email)) {
		if e.ID == id || (e.Address != "" && e.Address == id) {
			return model.withInvitation(e)
		}
	}
	e := model.Event(id)
	if e == nil || !a.sees(email, admin, e) {
		return nil
	}
	return model.withInvitation(e)
}

func (a app) sees(email string, admin bool, e *Event) bool {
	if admin || normalizeEmail(e.AddedBy) == email || a.isHost(email, admin, e) {
		return true
	}
	if e.Sharing == SharingInvited {
		return a.cache.Model().Listed(a.directory, email, e.ID)
	}
	return true
}

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
		ReplyTo: a.replyTo(e, email),
		Subject: "Invitation: " + e.Title + " · " + day,
		Text:    text.String(),
		HTML:    htm.String(),
		Attachments: []mail.Attachment{{
			Name:        "invite.ics",
			ContentType: "text/calendar; method=REQUEST; charset=utf-8",
			Content:     []byte(invite(a.organizer(e.ID, email), email, e, link, now())),
		}},
	})
	if err != nil {
		slog.ErrorContext(ctx, "[ERROR] calendar: send invite", "to", email, "event", e.ID, "error", err)
		return
	}
	slog.InfoContext(ctx, "calendar: invite sent", "to", email, "event", e.ID)
}

func (a app) organizer(id, email string) string {
	address := mailAddress(a.mail.ReplyTo)
	local, domain, _ := strings.Cut(address, "@")
	return strings.Replace(a.mail.ReplyTo, address, local+"+"+a.replyToken(id, email)+"@"+domain, 1)
}

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

func mailAddress(from string) string {
	if i := strings.LastIndex(from, "<"); i >= 0 {
		return strings.TrimSuffix(from[i+1:], ">")
	}
	return from
}
