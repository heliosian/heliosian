package when

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/config"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
)

type Answerer func(ctx context.Context, email, id, answer string) error

var errNotRecorded = errors.New("the answer was not recorded")

func (a app) recordBy(ctx context.Context, actor access.Actor, email, id, answer, via string, invite, wait bool) error {
	email = config.NormalizeEmail(email)
	answer = strings.ToLower(strings.TrimSpace(answer))
	ops, e, err := a.answerOps(actor, email, id, answer, via)
	if err != nil {
		return err
	}
	if inv := a.cache.Model().InviteOf(e.ID, email); inv != nil && inv.Sent != "" {
		invite = false
	}
	commit := a.cache.Commit
	if wait {
		commit = a.cache.CommitAndWait
	}
	if err := commit(ctx, actor, ops...); err != nil {
		return fmt.Errorf("%w: %w", errNotRecorded, err)
	}
	model := a.cache.Model()
	if invite && answer == AnswerYes && !isGuestKey(email) {
		go a.sendInvite(context.WithoutCancel(ctx), email, e)
	}
	if inv := model.Invitations[e.ID]; inv != nil && answer != "" && answer != AnswerHidden {
		for _, h := range inv.Notify {
			if h != actor.Email {
				go a.sendAnswerNote(context.WithoutCancel(ctx), h, actor.Email, email, answer, model.invitedEvent(e))
			}
		}
	}
	return nil
}

func (a app) sendAnswerNote(ctx context.Context, to, actor, email, answer string, e *Event) {
	model := a.cache.Model()
	name := email
	if p := a.directory().Person(email); p != nil && p.FullName != "" {
		name = p.FullName
	} else if inv := model.InviteOf(e.ID, email); inv != nil && inv.Name != "" {
		name = inv.Name
	}
	by := ""
	if actor != email {
		who := actor
		if p := a.directory().Person(actor); p != nil && p.FullName != "" {
			who = p.FullName
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
	l := a.letterFor(e, EventPath(e))
	l.Heading = name + " said " + answerWord(answer)
	l.Intro = fmt.Sprintf("%s said %s to %s%s.", name, answerWord(answer), e.Title, by)
	l.Rows = [][2]string{{"So far", fmt.Sprintf("%d yes, %d maybe, %d no, %d still to answer", yes, maybe, no, waiting)}}
	l.Button = "See the guest list"
	l.Footnote = "You asked to hear as answers come in; turn it off under Who's coming on the event's page."
	if err := a.mail.Sender.Send(ctx, l.Message("["+e.Title+"] "+name+" said "+answerWord(answer), []string{to}, nil, nil)); err != nil {
		slog.ErrorContext(ctx, "calendar: send answer note", "to", to, "event", e.ID, "error", err)
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
	if admin || config.NormalizeEmail(e.AddedBy) == email || a.isHost(access.Actor{Email: email, Admin: admin}, e) {
		return true
	}
	if e.Sharing == SharingInvited {
		return a.cache.Model().Listed(a.directory(), email, e.ID)
	}
	return true
}

type rsvpBody struct {
	ID     string `json:"id"`
	Answer string `json:"answer"`
}

func (a app) rsvp(r *http.Request, body rsvpBody) (serve.None, error) {
	actor := a.actor(r)
	if err := a.recordBy(r.Context(), actor, actor.Email, body.ID, body.Answer, ViaPage, true, false); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "calendar: answered", "actor", actor.Email, "event", body.ID, "answer", body.Answer)
	return serve.None{}, nil
}

func (a app) sendInvite(ctx context.Context, email string, e *Event) {
	day, _ := whenLines(e)
	l := a.letterFor(e, EventPath(e))
	l.Heading = "You said yes"
	l.Intro = "You said yes to " + e.Title + "."
	l.Button = "Open the event"
	l.Footnote = "The invite attached puts it on your calendar."
	msg := l.Message("Invitation: "+e.Title+" · "+day, []string{email}, nil, a.replyTo(e, email))
	msg.Attachments = []mail.Attachment{a.invite(e, email, l.Path, mail.MethodRequest)}
	if err := a.mail.Sender.Send(ctx, msg); err != nil {
		slog.ErrorContext(ctx, "calendar: send invite", "to", email, "event", e.ID, "error", err)
		return
	}
	slog.InfoContext(ctx, "calendar: invite sent", "to", email, "event", e.ID)
}

func (a app) organizer(id, email string) string {
	address := mail.AddressOf(a.mail.ReplyTo)
	local, domain, _ := strings.Cut(address, "@")
	return strings.Replace(a.mail.ReplyTo, address, local+"+"+a.replyToken(id, email)+"@"+domain, 1)
}

func (a app) letterFor(e *Event, path string) mail.Letter {
	day, hours := whenLines(e)
	when := day
	if hours != "" {
		when += " · " + hours
	}
	return mail.Letter{Brand: brand, Base: a.mail.Base, Title: e.Title, When: when, Where: e.Location, Path: a.mail.Base + path}
}

func (e *Event) mailEvent(link string) mail.Event {
	m := mail.Event{UID: uidOf(e.ID), Start: e.start, End: e.end, AllDay: e.AllDay, Summary: e.Title, Description: strings.TrimSpace(e.Description + "\n\n" + link), Location: e.Location, URL: link}
	if e.AllDay {
		m.End = e.end.AddDate(0, 0, 1)
	}
	return m
}

func (a app) invite(e *Event, to, link, method string) mail.Attachment {
	m := e.mailEvent(link)
	if method == mail.MethodCancel {
		m.Summary = "Cancelled: " + e.Title
	}
	m.Organizer = mail.Person{Name: brand.Name, Email: a.organizer(e.ID, to)}
	m.Attendees = []mail.Person{{Name: to, Email: to}}
	m.RSVP = method == mail.MethodRequest
	return mail.Calendar{Product: brand.Name, Method: method, Stamp: now(), Events: []mail.Event{m}}.Attachment()
}
