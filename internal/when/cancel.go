package when

import (
	"context"
	"fmt"

	"heliosian/internal/mail"
)

type cancelBody struct {
	ID     string `json:"id"`
	Notify bool   `json:"notify"`
	Note   string `json:"note"`
}

func (a app) sendCancellation(ctx context.Context, to string, cc, replyTo []string, hostName, note string, e *Event) error {
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
	return a.mail.Sender.Send(ctx, msg)
}
