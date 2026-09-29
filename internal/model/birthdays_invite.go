package model

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

const schoolDomain = "@heliosschool.org"

var birthdaysBrand = mail.Brand{Name: "Helios Staff Birthdays", Color: "#0e4d54", Tagline: "staff birthday donations"}

func staffPath(email string) string {
	if strings.HasSuffix(email, schoolDomain) {
		return "/staff/" + strings.TrimSuffix(email, schoolDomain)
	}
	return "/staff/" + email
}

func (a birthdaysApp) world() BirthdaysWorld {
	return NewBirthdaysWorld(a.cache.Model(), a.directory())
}

func (a birthdaysApp) inviteLoop(kick <-chan struct{}) {
	for range kick {
		a.sendInvites(context.Background())
	}
}

func sendable(sv StaffView) bool {
	return sv.InDirectory && sv.AssignedTo != "" && sv.RequestBy != "" && sv.Stage != StageComplete
}

func (inv BirthdayInvite) covers(sv StaffView) bool {
	return inv.SentOn == "" || (inv.SentTo == sv.AssignedTo && inv.AskDay == sv.RequestBy)
}

func (m *Birthdays) lastSent(email, year string) (BirthdayInvite, bool) {
	invites := m.invitesFor(email, year)
	for i := len(invites) - 1; i >= 0; i-- {
		if invites[i].SentOn != "" {
			return invites[i], true
		}
	}
	return BirthdayInvite{}, false
}

func StaleInvites(w BirthdaysWorld, at time.Time) []StaffView {
	out := []StaffView{}
	for i := range w.Model.Birthdays {
		sv := w.staffOf(w.Model.Birthdays[i].Email, at)
		if !sendable(sv) {
			continue
		}
		if invites := w.Model.invitesFor(sv.Email, sv.Year); len(invites) > 0 && invites[len(invites)-1].covers(sv) {
			continue
		}
		out = append(out, sv)
	}
	return out
}

func (a birthdaysApp) sendInvites(ctx context.Context) {
	at := now()
	actor := access.System(invitesActor)
	w := a.world()
	ops := []store.Op{}
	minted := map[string]bool{}
	for _, sv := range StaleInvites(w, at) {
		key := id.New(func(k string) bool { return minted[k] || a.taken(k) })
		minted[key] = true
		queued, err := queueInvite(actor, sv, key, at)
		if err != nil {
			slog.ErrorContext(ctx, "birthday: queue invite", "error", err, "email", sv.Email)
			return
		}
		ops = append(ops, queued...)
	}
	if len(ops) > 0 {
		if err := a.cache.Commit(ctx, actor, ops...); err != nil {
			slog.ErrorContext(ctx, "birthday: queue invites", "error", err)
			return
		}
		w = a.world()
	}
	for _, inv := range w.Model.Invites {
		if inv.SentOn != "" {
			continue
		}
		sv := w.staffOf(inv.Email, at)
		if !sendable(sv) || sv.Year != inv.Year {
			continue
		}
		movedFrom := ""
		if last, ok := w.Model.lastSent(inv.Email, inv.Year); ok && last.SentTo == sv.AssignedTo && last.AskDay != sv.RequestBy {
			movedFrom = last.AskDay
		}
		if err := a.mailer.Send(ctx, assignmentMessage(w.Model, a.base, a.mailer.From(), sv, sv.AssignedTo, movedFrom)); err != nil {
			slog.ErrorContext(ctx, "birthday: mail invite", "error", err, "to", sv.AssignedTo, "email", sv.Email)
			continue
		}
		slog.InfoContext(ctx, "birthday: sent invite", "id", inv.ID, "email", sv.Email, "to", sv.AssignedTo, "ask", sv.RequestBy, "moved from", movedFrom)
		recorded, err := recordInvite(actor, inv, sv, at)
		if err == nil {
			err = a.cache.Commit(ctx, actor, recorded...)
		}
		if err != nil {
			slog.ErrorContext(ctx, "birthday: record invite", "error", err, "email", sv.Email)
		}
	}
}

func assignmentMessage(b *Birthdays, base, from string, sv StaffView, to, movedFrom string) mail.Message {
	link := base + staffPath(sv.Email)
	ask := longDate(sv.RequestBy)
	subject := threadSubject(sv)
	headers := threadHeaders(sv, true)
	if movedFrom != "" {
		subject = "Re: " + subject
		headers = threadHeaders(sv, false)
	}
	rows := [][2]string{
		{"Birthday", longDate(sv.BirthdayThisYear)},
		{"Ask by", ask},
		{"Newsletter", longDate(sv.NewsletterDate)},
	}
	if sv.LastDonation != nil {
		last := b.charityName(sv.LastDonation.Charity)
		if sv.LastDonation.Note != "" {
			last += " — " + sv.LastDonation.Note
		}
		rows = append(rows, [2]string{"Last year", last})
	}
	if sv.Level == LevelNoNewsletter {
		rows = append(rows, [2]string{"Note", "Asked to stay out of the newsletter"})
	}
	l := mail.Letter{
		Brand:    birthdaysBrand,
		Base:     base,
		Title:    sv.Name,
		Path:     link,
		Heading:  fmt.Sprintf("%s's birthday is yours", sv.Name),
		Intro:    fmt.Sprintf("%s's birthday is yours this year. The day to ask them is %s: reach out by then and record the charity they choose.", sv.Name, mediumDate(sv.RequestBy)),
		Rows:     rows,
		Button:   "Open their birthday page",
		Footnote: "The invite attached puts the day to ask by on your calendar.",
	}
	if movedFrom != "" {
		l.Heading = "The day to ask moved"
		l.Intro = fmt.Sprintf("The day to ask %s about their birthday charity moved from %s to %s, so here is the invite again.", sv.Name, mediumDate(movedFrom), mediumDate(sv.RequestBy))
	}
	description := fmt.Sprintf("Reach out to %s about the charity they would like the HCA to give to for their birthday, and record it on their page.\n\n", sv.Name)
	for _, row := range rows {
		description += fmt.Sprintf("%s: %s\n", row[0], row[1])
	}
	description += "\n" + link
	day, _ := ParseDate(sv.RequestBy)
	e := mail.Event{
		UID:         fmt.Sprintf("birthday-%s-%s@heliosian.com", sv.Email, strings.ReplaceAll(sv.Year, " ", "")),
		Start:       day,
		End:         day.AddDate(0, 0, 1),
		AllDay:      true,
		Summary:     subject,
		Description: description,
		URL:         link,
		Organizer:   mail.Person{Name: birthdaysBrand.Name, Email: from},
		Attendees:   []mail.Person{{Email: to}},
		Transparent: true,
		Alarm:       15 * time.Hour,
	}
	m := l.Message(subject, []string{to}, nil, nil)
	m.Headers = headers
	m.Attachments = []mail.Attachment{mail.Calendar{Product: birthdaysBrand.Name, Method: mail.MethodRequest, Stamp: time.Now(), Events: []mail.Event{e}}.Attachment()}
	return m
}

func longDate(cell string) string {
	if t, err := ParseDate(cell); err == nil {
		return t.Format("Monday, January 2, 2006")
	}
	return cell
}

func mediumDate(cell string) string {
	if t, err := ParseDate(cell); err == nil {
		return t.Format("January 2, 2006")
	}
	return cell
}
