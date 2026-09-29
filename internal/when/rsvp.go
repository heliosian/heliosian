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
	ops, _, err := a.answerOps(actor, email, id, answer, via, invite)
	if err != nil {
		return err
	}
	commit := a.cache.Commit
	if wait {
		commit = a.cache.CommitAndWait
	}
	if err := commit(ctx, actor, ops...); err != nil {
		return fmt.Errorf("%w: %w", errNotRecorded, err)
	}
	return nil
}

func (a app) sendAnswerNote(ctx context.Context, to, actor, email, answer string, e *Event) error {
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
	return a.mail.Sender.Send(ctx, l.Message("["+e.Title+"] "+name+" said "+answerWord(answer), []string{to}, nil, nil))
}

func (a app) eventFor(actor access.Actor, id string) *Event {
	return a.lookup(actor.Email, id, func(e *Event) bool { return a.sees(actor, e) })
}

func (a app) anyEvent(email, id string) *Event {
	return a.lookup(email, id, func(*Event) bool { return true })
}

func (a app) lookup(email, key string, sees func(*Event) bool) *Event {
	model := a.cache.Model()
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

func (a app) canonical(key string) string {
	source, rest, ok := strings.Cut(key, "/")
	if !ok || (source != SourceCelebrate && source != SourceTeam) {
		return a.cache.Model().aliases.Resolve(key)
	}
	found := a.sourceID(source, rest)
	for _, e := range withLinked(a.cache.Model().Events, a.linked("")) {
		if found != "" && e.linkedID() == found {
			return e.ID
		}
	}
	return key
}

func (a app) sees(actor access.Actor, e *Event) bool {
	if actor.May(SeeAll) || config.NormalizeEmail(e.AddedBy) == actor.Email || a.isHost(actor, e) {
		return true
	}
	if e.Sharing == SharingInvited {
		return a.cache.Model().Listed(a.directory(), actor.Email, e.ID)
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

func (a app) sendInvite(ctx context.Context, email string, e *Event) error {
	day, _ := whenLines(e)
	l := a.letterFor(e, EventPath(e))
	l.Heading = "You said yes"
	l.Intro = "You said yes to " + e.Title + "."
	l.Button = "Open the event"
	l.Footnote = "The invite attached puts it on your calendar."
	msg := l.Message("Invitation: "+e.Title+" · "+day, []string{email}, nil, a.replyTo(e, email))
	msg.Attachments = []mail.Attachment{a.invite(e, email, l.Path, mail.MethodRequest)}
	return a.mail.Sender.Send(ctx, msg)
}

func (a app) organizer(id, email string) string {
	address := mail.AddressOf(a.mail.ReplyTo)
	local, domain, _ := strings.Cut(address, "@")
	return strings.Replace(a.mail.ReplyTo, address, local+"+"+a.replyToken(id, email)+"@"+domain, 1)
}

func (a app) letterFor(e *Event, path string) mail.Letter {
	return mail.Letter{Brand: brand, Base: a.mail.Base, Title: e.Title, When: when(e), Where: e.Location, Path: a.mail.Base + path}
}

func (e *Event) mailEvent(link string) mail.Event {
	m := mail.Event{UID: uidOf(e.uidKey()), Start: e.start, End: e.end, AllDay: e.AllDay, Summary: e.Title, Description: strings.TrimSpace(e.Description + "\n\n" + link), Location: e.Location, URL: link}
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
	m.Organizer = mail.Person{Name: brand.Name, Email: a.organizer(e.uidKey(), to)}
	m.Attendees = []mail.Person{{Name: to, Email: to}}
	m.RSVP = method == mail.MethodRequest
	return mail.Calendar{Product: brand.Name, Method: method, Stamp: now(), Events: []mail.Event{m}}.Attachment()
}
