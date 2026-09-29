package model

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/mail"
)

const (
	remindAsk      = "ask"
	remindLate     = "late"
	remindDonation = "donation"

	lateAfterDays = 2
)

var remindHours = [2]int{7, 21}

type reminder struct {
	sv   StaffView
	kind string
	to   string
}

func (a birthdaysApp) dueReminders(m *Model, today time.Time) []reminder {
	out := []reminder{}
	day := today.Format(DateFormat)
	b := m.Birthdays
	for i := range b.Birthdays {
		sv := m.staffOf(b.Birthdays[i].Email, today)
		if sv.AssignedTo == "" || sv.RequestBy == "" || sv.Stage == StageComplete {
			continue
		}
		sent := func(kind string) bool { return b.Reminders[reminderKey(sv.Email, sv.Year, kind)] }
		contacted := sv.ContactedOn != ""
		ask, _ := ParseDate(sv.RequestBy)
		late := ask.AddDate(0, 0, lateAfterDays).Format(DateFormat)
		switch {
		case !contacted && day >= late && !sent(remindLate):
			out = append(out, reminder{sv, remindLate, sv.AssignedTo})
		case !contacted && day >= sv.RequestBy && !sent(remindAsk) && !sent(remindLate):
			out = append(out, reminder{sv, remindAsk, sv.AssignedTo})
		}
		if sv.Donation == nil && sv.DueBy != "" && !sent(remindDonation) {
			if day >= sv.DueBy && day <= sv.NewsletterDate {
				out = append(out, reminder{sv, remindDonation, sv.AssignedTo})
			}
		}
	}
	return out
}

func (a birthdaysApp) remindLoop() {
	time.Sleep(time.Minute)
	for {
		if h := now().Hour(); h >= remindHours[0] && h < remindHours[1] {
			a.sendDueReminders(context.Background(), now())
		}
		time.Sleep(time.Hour)
	}
}

func (a birthdaysApp) sendDueReminders(ctx context.Context, today time.Time) int {
	m := a.store.Model()
	b := m.Birthdays
	due := a.dueReminders(m, today)
	n := 0
	for _, rem := range due {
		msg := a.reminderMessage(m, rem)
		if err := a.mailer.Send(ctx, msg); err != nil {
			slog.ErrorContext(ctx, "birthday: send reminder", "error", err, "kind", rem.kind, "email", rem.sv.Email, "to", rem.to)
			continue
		}
		n++
		slog.InfoContext(ctx, "birthday: sent reminder", "kind", rem.kind, "email", rem.sv.Email, "to", rem.to)
		kinds := []string{rem.kind}
		if rem.kind == remindLate && !b.Reminders[reminderKey(rem.sv.Email, rem.sv.Year, remindAsk)] {
			kinds = append(kinds, remindAsk)
		}
		for _, kind := range kinds {
			a.recordReminder(ctx, rem.sv, kind, rem.to, today)
		}
	}
	return n
}

func (a birthdaysApp) recordReminder(ctx context.Context, sv StaffView, kind, to string, today time.Time) {
	actor := access.System(remindersActor)
	ops, err := recordReminder(actor, sv, kind, to, today)
	if err != nil {
		slog.ErrorContext(ctx, "birthday: record reminder", "error", err)
		return
	}
	if err := a.store.Commit(ctx, actor, birthdaysAppName, ops...); err != nil {
		slog.ErrorContext(ctx, "birthday: record reminder", "error", err)
	}
}

func (a birthdaysApp) reminderMessage(m *Model, rem reminder) mail.Message {
	b := m.Birthdays
	sv := rem.sv
	link := a.base + staffPath(sv.Email)
	assignee, _ := birthdayViewer{directory: m.Directory}.person(rem.to)
	var text, htm strings.Builder
	p := func(t string) {
		text.WriteString(t + "\n\n")
		fmt.Fprintf(&htm, "<p style=\"font:16px/1.5 -apple-system,Segoe UI,Roboto,sans-serif\">%s</p>", html.EscapeString(t))
	}
	button := func(label, href string) {
		fmt.Fprintf(&text, "%s: %s\n\n", label, href)
		fmt.Fprintf(&htm, "<p style=\"margin:16px 0\"><a href=\"%s\" style=\"display:inline-block;padding:10px 18px;border-radius:8px;background:#0e4d54;color:#fff;font:700 15px -apple-system,Segoe UI,Roboto,sans-serif;text-decoration:none\">%s</a></p>", html.EscapeString(href), html.EscapeString(label))
	}
	switch rem.kind {
	case remindAsk, remindLate:
		if rem.kind == remindAsk {
			p(fmt.Sprintf("Today is the day to ask %s which charity they would like the HCA to give to for their birthday (%s).", sv.Name, monthDay(sv.BirthdayThisYear)))
		} else {
			p(fmt.Sprintf("%s was due to be asked about their birthday charity on %s, and their outreach is not marked done. If you have asked, mark it done on their page; if not, here is the email, ready to send.", sv.Name, mediumDate(sv.RequestBy)))
		}
		l := letter(b, sv, assignee.Name)
		button("Send the email", mailto(l))
		fmt.Fprintf(&text, "To: %s\nCC: %s\nSubject: %s\n\n%s\n\n", l.To, l.CC, l.Subject, l.Body)
		fmt.Fprintf(&htm, "<table style=\"font:14px/1.5 -apple-system,Segoe UI,Roboto,sans-serif;border-collapse:collapse;color:#33474c\"><tr><td style=\"padding:2px 12px 2px 0;color:#647071\">To</td><td>%s</td></tr><tr><td style=\"padding:2px 12px 2px 0;color:#647071\">CC</td><td>%s</td></tr><tr><td style=\"padding:2px 12px 2px 0;color:#647071\">Subject</td><td>%s</td></tr></table>", html.EscapeString(l.To), html.EscapeString(l.CC), html.EscapeString(l.Subject))
		fmt.Fprintf(&htm, "<blockquote style=\"margin:12px 0;padding:12px 16px;border-left:3px solid #cfdcde;background:#f4f8f8;font:15px/1.5 -apple-system,Segoe UI,Roboto,sans-serif;white-space:pre-wrap\">%s</blockquote>", html.EscapeString(l.Body))
		p("Once it is sent, mark the outreach done on their page so the team can see it.")
		button("Mark outreach done", link)
	case remindDonation:
		p(fmt.Sprintf("The %s newsletter is two days out and %s's charity is not recorded yet.", mediumDate(sv.NewsletterDate), sv.Name))
		p("If they answered, record what they chose on their page. There is no need to ask again - whether to reply is their choice, and if they do not, the donation goes to " + b.charityName(b.Settings.DefaultCharity) + ".")
		button("Record their charity", link)
	}
	subject := "Re: " + threadSubject(sv)
	return mail.Message{
		To:      []string{rem.to},
		Subject: subject,
		Text:    strings.TrimSpace(text.String()),
		HTML:    htm.String(),
		Headers: threadHeaders(sv, false),
	}
}

func mailto(l Letter) string {
	q := url.Values{}
	if l.CC != "" {
		q.Set("cc", l.CC)
	}
	q.Set("subject", l.Subject)
	q.Set("body", l.Body)
	// url.Values writes spaces as +, which mail apps read as pluses.
	return "mailto:" + l.To + "?" + strings.ReplaceAll(q.Encode(), "+", "%20")
}

func threadSubject(sv StaffView) string {
	return fmt.Sprintf("Ask %s about their birthday charity", sv.Name)
}

func threadHeaders(sv StaffView, first bool) map[string]string {
	id := fmt.Sprintf("<birthday-%s-%s@heliosian.com>", sv.Email, strings.ReplaceAll(sv.Year, " ", ""))
	if first {
		return map[string]string{"Message-ID": id}
	}
	return map[string]string{"In-Reply-To": id, "References": id}
}
