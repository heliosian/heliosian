package when

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/mail"
)

const (
	inviteReminder = "reminder"
	inviteUpdate   = "update"
)

func (a app) fullName(email string) string {
	if p := a.directory().Person(email); p != nil && p.FullName != "" {
		return p.FullName
	}
	return email
}

func (a app) senderAndReplyTo(actor access.Actor, e *Event) (string, []string) {
	replyTo := a.hostsOf(e)
	if !slices.Contains(replyTo, actor.Email) {
		replyTo = append([]string{actor.Email}, replyTo...)
	}
	return a.fullName(actor.Email), replyTo
}

func (a app) recipients(e *Event, emails []string) ([]string, map[string][]string) {
	model := a.model()
	order := []string{}
	cc := map[string][]string{}
	for _, email := range emails {
		if model.InviteOf(e.ID, email) == nil || isGuestKey(email) || slices.Contains(order, email) {
			continue
		}
		with, reachable := a.ccFor(email)
		if !reachable {
			continue
		}
		cc[email] = with
		order = append(order, email)
	}
	return order, cc
}

func (a app) ccFor(email string) ([]string, bool) {
	directory := a.directory()
	p := directory.Person(email)
	if p == nil {
		return nil, true
	}
	if p.EmailMasked {
		return nil, false
	}
	cc := []string{}
	if !isAdult(p) {
		adults, kids := directory.Household(email)
		for _, member := range append(adults, kids...) {
			if isAdult(member) && !slices.Contains(cc, member.Email) {
				cc = append(cc, member.Email)
			}
		}
	}
	return cc, true
}

func (a app) sendCohostNote(ctx context.Context, to, actor string, e *Event) error {
	e = a.model().invitedEvent(e)
	l := a.letterFor(e, EventPath(e))
	l.Heading = "You're a co-host"
	l.Intro = fmt.Sprintf("%s made you a co-host of %s. As a co-host you can build and send the guest list, read every answer, message the guests, and replies to the invitation reach you.", a.fullName(actor), e.Title)
	l.Button = "Open the event"
	return a.mail.Sender.Send(ctx, l.Message("["+e.Title+"] You're a co-host", []string{to}, nil, []string{actor}))
}

func (a app) reachable(e *Event, emails []string) int {
	order, _ := a.recipients(e, emails)
	return len(order)
}

func (a app) replyTo(e *Event, to string) []string {
	return append(append([]string{}, a.hostsOf(e)...), a.organizer(e.uidKey(), to))
}

func FirstWord(name string) string {
	if words := strings.Fields(name); len(words) > 0 {
		return words[0]
	}
	return name
}

func (a app) sendInvitation(ctx context.Context, to string, cc, names []string, host, message string, e *Event, link string, replyTo []string, kind string) error {
	origin := a.mail.Base
	outside := strings.Contains(link, "/ext/")
	day, hours := whenLines(e)
	at := when(e)
	hosts := []string{}
	for _, h := range a.hostsOf(e) {
		if p := a.directory().Person(h); p != nil && p.FullName != "" {
			hosts = append(hosts, p.FullName)
		}
	}
	if len(hosts) == 0 {
		hosts = []string{host}
	}
	hosting := "Hosted by " + strings.Join(hosts, " and ")
	invited := strings.Join(names, ", ")
	rsvpFor := "RSVP for " + joinNames(names) + " here"
	picture := origin + "/open/share/" + e.ID + ".png"
	if inv := a.model().Invitations[e.ID]; inv != nil && inv.Flyer != "" {
		picture = origin + flyerPath(e.ID)
	}
	whom := FirstWord(cells.DisplayName(to))
	if len(names) > 0 {
		whom = names[0]
	}
	sentBy := host + " sent " + whom + " an invitation for"
	switch kind {
	case inviteReminder:
		sentBy = host + " is still hoping to hear from " + whom + " about"
	case inviteUpdate:
		sentBy = host + " has updated the details of"
	}
	year := e.start.Format(", 2006")
	var text strings.Builder
	fmt.Fprintf(&text, "%s\n\n%s\n%s%s\n", sentBy, e.Title, day, year)
	if hours != "" {
		fmt.Fprintf(&text, "%s\n", hours)
	}
	if message != "" {
		fmt.Fprintf(&text, "\n%s\n", message)
	}
	if e.Description != "" {
		fmt.Fprintf(&text, "\n%s\n", e.Description)
	}
	fmt.Fprintf(&text, "\n%s\nOpen the invitation: %s\n", rsvpFor, link)
	if e.Location != "" {
		fmt.Fprintf(&text, "\n%s\n", e.Location)
	}
	fmt.Fprintf(&text, "%s\n%s\n\nInvited: %s\n", hosting, at, invited)
	attached := ""
	if len(cc) == 0 {
		attached = " The invite attached puts it on your calendar."
		text.WriteString("\nThe invite attached puts it on your calendar.\n")
	}
	font := "-apple-system,Segoe UI,Roboto,sans-serif"
	esc := html.EscapeString
	var htm strings.Builder
	fmt.Fprintf(&htm, "<div style=\"max-width:600px;margin:0 auto;padding:8px 0;font-family:%s;color:#1b2a2c\">", font)
	htm.WriteString("<div style=\"background:#fff;border:1px solid #e6e6e6;border-radius:6px;padding:36px 32px 28px\">")
	fmt.Fprintf(&htm, "<p style=\"margin:0 0 14px;font-size:16px;line-height:1.4;text-align:center;color:#1b2a2c\">%s</p>", esc(sentBy))
	fmt.Fprintf(&htm, "<p style=\"margin:0 0 10px;font-size:26px;line-height:1.25;font-weight:400;text-align:center;color:#1b2a2c\">%s</p>", esc(e.Title))
	fmt.Fprintf(&htm, "<p style=\"margin:0 0 28px;font-size:15px;text-align:center;color:#444\">%s%s</p>", esc(day), esc(year))
	if message != "" {
		fmt.Fprintf(&htm, "<p style=\"margin:0 0 20px;font-size:15px;line-height:1.55;color:#444;white-space:pre-wrap\">%s</p>", esc(message))
	}
	if e.Description != "" {
		fmt.Fprintf(&htm, "<p style=\"margin:0 0 24px;font-size:15px;line-height:1.55;color:#444;white-space:pre-wrap\">%s</p>", esc(e.Description))
	}
	fmt.Fprintf(&htm, "<p style=\"margin:0 0 12px;font-size:15px;font-weight:700;text-align:center\"><a href=\"%s\" style=\"color:#1b2a2c;font-weight:700\">%s</a></p>", esc(link), esc(rsvpFor))
	fmt.Fprintf(&htm, "<p style=\"margin:0 0 28px;text-align:center\"><a href=\"%s\" style=\"display:inline-block;padding:13px 26px;border-radius:4px;background:#9a9a9a;color:#fff;font-size:14px;font-weight:600;letter-spacing:0.06em;text-decoration:none\">OPEN INVITATION</a></p>", esc(link))
	fmt.Fprintf(&htm, "<p style=\"margin:0 0 24px;text-align:center\"><a href=\"%s\"><img src=\"%s\" alt=\"%s\" width=\"480\" style=\"display:inline-block;width:100%%;max-width:480px;height:auto;border-radius:4px\"></a></p>", esc(link), esc(picture), esc(e.Title))
	fmt.Fprintf(&htm, "<p style=\"margin:0 0 6px;font-size:13px;font-style:italic;text-align:center;color:#777\">This email is for %s. Please do not forward it.</p>", esc(invited))
	htm.WriteString("<hr style=\"border:0;border-top:1px solid #e6e6e6;margin:22px 0\">")
	htm.WriteString("<div style=\"text-align:center;font-size:14px;line-height:1.7;color:#333\">")
	fmt.Fprintf(&htm, "<p style=\"margin:0;font-weight:700;color:#1b2a2c\">%s</p>", esc(hosting))
	if e.Location != "" {
		maps := "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(e.Location)
		fmt.Fprintf(&htm, "<p style=\"margin:0\"><a href=\"%s\" style=\"color:#1a73e8\">%s</a> <a href=\"%s\" style=\"color:#2f9e6a;text-decoration:none\">(View Map)</a></p>", esc(maps), esc(e.Location), esc(maps))
	}
	fmt.Fprintf(&htm, "<p style=\"margin:0\">%s</p>", esc(at))
	fmt.Fprintf(&htm, "<p style=\"margin:10px 0 0\"><a href=\"%s\" style=\"color:#2f9e6a;text-decoration:none;margin:0 8px\">Add to Google</a> <a href=\"%s\" style=\"color:#2f9e6a;text-decoration:none;margin:0 8px\">RSVP</a></p>", esc(e.mailEvent(link).GoogleLink()), esc(link))
	if outside {
		fmt.Fprintf(&htm, "<p style=\"margin:12px 0 0;font-size:12px;color:#777\">The page is yours alone - no account needed.%s</p>", attached)
	} else {
		fmt.Fprintf(&htm, "<p style=\"margin:12px 0 0;font-size:12px;color:#777\">Yes, no or maybe on the page answers for everyone in your household who is invited.%s</p>", attached)
	}
	htm.WriteString("</div></div>")
	fmt.Fprintf(&htm, "<p style=\"margin:14px 0 0;font-size:11px;letter-spacing:0.08em;text-align:center;color:#999\">SENT WITH HELIOS WHEN</p>")
	htm.WriteString("</div>")
	subject := "[" + e.Title + "] You're invited!"
	switch kind {
	case inviteReminder:
		subject = "[" + e.Title + "] Reminder: you're invited!"
	case inviteUpdate:
		subject = "[" + e.Title + "] Updated: the details have changed"
	}
	msg := mail.Message{
		To:       []string{to},
		CC:       cc,
		ReplyTo:  replyTo,
		FromName: strings.Join(hosts, " and "),
		Subject:  subject,
		Text:     text.String(),
		HTML:     htm.String(),
	}
	if len(cc) == 0 {
		msg.Attachments = []mail.Attachment{a.invite(e, to, link, mail.MethodRequest)}
	}
	return a.mail.Sender.Send(ctx, msg)
}

func answerWord(answer string) string {
	switch answer {
	case AnswerYes:
		return "Yes"
	case AnswerMaybe:
		return "Maybe"
	case AnswerNo:
		return "No"
	}
	return "No response yet"
}

func (a app) sendMessage(ctx context.Context, to string, cc, replyTo []string, hostName, subject, message string, e *Event, attach bool) error {
	model := a.model()
	e = model.invitedEvent(e)
	path := EventPath(e)
	if row := model.InviteOf(e.ID, to); row != nil && row.Token != "" {
		path = extPath(row.Token)
	}
	household := a.householdOn(e, to)
	lines := [][2]string{}
	waiting := false
	for _, inv := range model.Invites[e.ID] {
		mine := slices.Contains(household, inv.Email) || (inv.GuestOf != "" && slices.Contains(household, inv.GuestOf))
		if !mine {
			continue
		}
		name := inv.Name
		if p := a.directory().Person(inv.Email); p != nil && p.FullName != "" {
			name = p.FullName
		}
		if inv.Email == to {
			name = "You"
		}
		answer := model.AnswerOf(inv.Email, e.ID)
		if answer == AnswerHidden {
			answer = ""
		}
		if answer == "" {
			waiting = true
		}
		l := [2]string{name, answerWord(answer)}
		if inv.Email == to {
			lines = append([][2]string{l}, lines...)
		} else {
			lines = append(lines, l)
		}
	}
	l := a.letterFor(e, path)
	l.Heading = subject
	l.Intro = fmt.Sprintf("A message from %s about %s:", hostName, e.Title)
	l.Note = message
	l.Rows = lines
	l.Button = "Open the event"
	if waiting {
		l.Button = "RSVP now"
		l.Footnote = "Someone in your household has not answered yet - the hosts would love to know."
	}
	msg := l.Message("["+e.Title+"] "+subject, []string{to}, cc, replyTo)
	if attach && len(cc) == 0 {
		msg.Attachments = []mail.Attachment{a.invite(e, to, l.Path, mail.MethodRequest)}
	}
	return a.mail.Sender.Send(ctx, msg)
}
