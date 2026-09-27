package birthday

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/mail"
)

const schoolDomain = "@heliosschool.org"

var brand = mail.Brand{Name: "Helios Staff Birthdays", Color: "#0e4d54", Tagline: "staff birthday donations"}

func staffPath(email string) string {
	if strings.HasSuffix(email, schoolDomain) {
		return "/staff/" + strings.TrimSuffix(email, schoolDomain)
	}
	return "/staff/" + email
}

func (a app) staffView(model *Model, staffEmail string) (StaffView, bool) {
	b := model.Birthday(staffEmail)
	if b == nil {
		return StaffView{}, false
	}
	v := viewer{directory: a.directory}
	month, day, _ := ParseMonthDay(model.Settings.YearStart)
	return v.staff(model, b, YearContaining(now(), month, day), now()), true
}

func (a app) mailAssignment(r *http.Request, staffEmail, to string) {
	if a.mailer == nil {
		return
	}
	sv, ok := a.staffView(a.cache.Model(), staffEmail)
	if !ok {
		return
	}
	if sv.RequestBy == "" {
		slog.InfoContext(r.Context(), "birthday: no invite, no newsletter date yet", "email", staffEmail, "to", to)
		return
	}
	a.sendInvite(r, sv, to, "")
}

type askDay struct {
	requestBy, assignedTo string
}

func (a app) askDays(model *Model) map[string]askDay {
	out := map[string]askDay{}
	if model == nil {
		return out
	}
	for i := range model.Birthdays {
		sv, ok := a.staffView(model, model.Birthdays[i].Email)
		if ok && sv.AssignedTo != "" && sv.RequestBy != "" {
			out[sv.Email] = askDay{requestBy: sv.RequestBy, assignedTo: sv.AssignedTo}
		}
	}
	return out
}

func (a app) mailMovedAskDays(r *http.Request, before, after map[string]askDay) {
	if a.mailer == nil {
		return
	}
	model := a.cache.Model()
	for email, now := range after {
		was, had := before[email]
		if !had || was.assignedTo != now.assignedTo || was.requestBy == now.requestBy {
			continue
		}
		sv, ok := a.staffView(model, email)
		if !ok {
			continue
		}
		slog.InfoContext(r.Context(), "birthday: ask day moved", "email", email, "from", was.requestBy, "to", now.requestBy, "assignee", now.assignedTo)
		a.sendInvite(r, sv, now.assignedTo, was.requestBy)
	}
}

func (a app) sendInvite(r *http.Request, sv StaffView, to, movedFrom string) {
	m := assignmentMessage(mail.Base(r), a.from, sv, to, movedFrom)
	go func() {
		if err := a.mailer.Send(context.WithoutCancel(r.Context()), m); err != nil {
			slog.Error("birthday: mail invite", "error", err, "to", to, "email", sv.Email)
		}
	}()
}

func assignmentMessage(base, from string, sv StaffView, to, movedFrom string) mail.Message {
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
		last := sv.LastDonation.Charity
		if sv.LastDonation.Note != "" {
			last += " — " + sv.LastDonation.Note
		}
		rows = append(rows, [2]string{"Last year", last})
	}
	if sv.Level == LevelNoNewsletter {
		rows = append(rows, [2]string{"Note", "Asked to stay out of the newsletter"})
	}
	l := mail.Letter{
		Brand:    brand,
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
		Organizer:   mail.Person{Name: brand.Name, Email: from},
		Attendees:   []mail.Person{{Email: to}},
		Transparent: true,
		Alarm:       15 * time.Hour,
	}
	m := l.Message(subject, []string{to}, nil, nil)
	m.Headers = headers
	m.Attachments = []mail.Attachment{mail.Calendar{Product: brand.Name, Method: mail.MethodRequest, Stamp: time.Now(), Events: []mail.Event{e}}.Attachment()}
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
