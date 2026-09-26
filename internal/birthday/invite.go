package birthday

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

const schoolDomain = "@heliosschool.org"

func staffPath(email string) string {
	if strings.HasSuffix(email, schoolDomain) {
		return "/staff/" + strings.TrimSuffix(email, schoolDomain)
	}
	return "/staff/" + email
}

func baseURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil && !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") && strings.HasPrefix(r.Host, "localhost") {
		scheme = "http"
	}
	return scheme + "://" + r.Host
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
	m := assignmentMessage(baseURL(r), a.from, sv, to, movedFrom)
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
	var text, htm strings.Builder
	opening := fmt.Sprintf("%s's birthday is yours this year. The day to ask them is %s.", sv.Name, mediumDate(sv.RequestBy))
	if movedFrom != "" {
		opening = fmt.Sprintf("The day to ask %s about their birthday charity moved from %s to %s, so here is the invite again.", sv.Name, mediumDate(movedFrom), mediumDate(sv.RequestBy))
	}
	fmt.Fprintf(&text, "%s\n\nReach out by %s and record the charity they choose:\n%s\n\n", opening, ask, link)
	fmt.Fprintf(&htm, "<p style=\"font:16px/1.5 -apple-system,Segoe UI,Roboto,sans-serif\">%s</p>", strings.Replace(html.EscapeString(opening), html.EscapeString(sv.Name), "<strong>"+html.EscapeString(sv.Name)+"</strong>", 1))
	htm.WriteString("<table style=\"font:15px/1.5 -apple-system,Segoe UI,Roboto,sans-serif;border-collapse:collapse\">")
	for _, row := range rows {
		fmt.Fprintf(&text, "%s: %s\n", row[0], row[1])
		fmt.Fprintf(&htm, "<tr><td style=\"padding:4px 16px 4px 0;color:#647071\">%s</td><td style=\"padding:4px 0\">%s</td></tr>", html.EscapeString(row[0]), html.EscapeString(row[1]))
	}
	htm.WriteString("</table>")
	fmt.Fprintf(&htm, "<p style=\"margin:20px 0\"><a href=\"%s\" style=\"display:inline-block;padding:10px 18px;border-radius:8px;background:#0e4d54;color:#fff;font:700 15px -apple-system,Segoe UI,Roboto,sans-serif;text-decoration:none\">Open their birthday page</a></p>", html.EscapeString(link))
	htm.WriteString("<p style=\"font:13px/1.5 -apple-system,Segoe UI,Roboto,sans-serif;color:#647071\">The invite attached puts the day to ask by on your calendar.</p>")
	description := fmt.Sprintf("Reach out to %s about the charity they would like the HCA to give to for their birthday, and record it on their page.\n\n", sv.Name)
	for _, row := range rows {
		description += fmt.Sprintf("%s: %s\n", row[0], row[1])
	}
	description += "\n" + link
	return mail.Message{
		To:      []string{to},
		Subject: subject,
		Text:    text.String(),
		HTML:    htm.String(),
		Headers: headers,
		Attachments: []mail.Attachment{{
			Name:        "invite.ics",
			ContentType: "text/calendar; method=REQUEST; charset=utf-8",
			Content:     []byte(invite(from, to, sv, subject, description, link)),
		}},
	}
}

func invite(from, to string, sv StaffView, summary, description, link string) string {
	day, _ := ParseDate(sv.RequestBy)
	next := day.AddDate(0, 0, 1)
	stamp := time.Now().UTC()
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Helios Staff Birthdays//EN",
		"METHOD:REQUEST",
		"BEGIN:VEVENT",
		"UID:" + icsEscape(fmt.Sprintf("birthday-%s-%s@heliosian.com", sv.Email, strings.ReplaceAll(sv.Year, " ", ""))),
		"DTSTAMP:" + stamp.Format("20060102T150405Z"),
		"SEQUENCE:" + fmt.Sprint(stamp.Unix()),
		"DTSTART;VALUE=DATE:" + day.Format("20060102"),
		"DTEND;VALUE=DATE:" + next.Format("20060102"),
		"SUMMARY:" + icsEscape(summary),
		"DESCRIPTION:" + icsEscape(description),
		"URL:" + icsEscape(link),
		"ORGANIZER;CN=Helios Staff Birthdays:mailto:" + mailAddress(from),
		"ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED;RSVP=FALSE:mailto:" + to,
		"STATUS:CONFIRMED",
		"TRANSP:TRANSPARENT",
		"BEGIN:VALARM",
		"ACTION:DISPLAY",
		"DESCRIPTION:" + icsEscape(summary),
		"TRIGGER:-PT15H",
		"END:VALARM",
		"END:VEVENT",
		"END:VCALENDAR",
	}
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(icsFold(line) + "\r\n")
	}
	return b.String()
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

func mailAddress(from string) string {
	if i := strings.LastIndex(from, "<"); i >= 0 {
		return strings.TrimSuffix(strings.TrimSpace(from[i+1:]), ">")
	}
	return strings.TrimSpace(from)
}

func icsEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\r\n", `\n`, "\n", `\n`)
	return r.Replace(s)
}

func icsFold(line string) string {
	var b strings.Builder
	count := 0
	for _, r := range line {
		size := len(string(r))
		if count+size > 75 {
			b.WriteString("\r\n ")
			count = 1
		}
		b.WriteRune(r)
		count += size
	}
	return b.String()
}
