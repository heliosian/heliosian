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

// When a birthday is assigned, the person it goes to gets an email with a
// calendar invite for the day the request is due: what to do, whose birthday,
// the dates, last year's charity, and a link to the staff member's page.
// Sending happens off the request, and a failure is logged rather than shown:
// the assignment itself already took.

const schoolDomain = "@heliosschool.org"

// staffPath is a staff member's page, as the client addresses it: the
// address's prefix for a school address, the whole address for anyone else.
func staffPath(email string) string {
	if strings.HasSuffix(email, schoolDomain) {
		return "/staff/" + strings.TrimSuffix(email, schoolDomain)
	}
	return "/staff/" + email
}

// baseURL is the app as the request reached it, for the link in a message.
func baseURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil && !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") && strings.HasPrefix(r.Host, "localhost") {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

// mailAssignment sends the assignee their invite, when the app has mail and
// the birthday has a day to ask by.
func (a app) mailAssignment(r *http.Request, staffEmail, to string) {
	if a.mailer == nil {
		return
	}
	model := a.cache.Model()
	b := model.Birthday(staffEmail)
	if b == nil {
		return
	}
	v := viewer{directory: a.directory}
	month, day, _ := ParseMonthDay(model.Settings.YearStart)
	sv := v.staff(model, b, YearContaining(now(), month, day), now())
	if sv.RequestBy == "" {
		slog.InfoContext(r.Context(), "birthday: no invite, no newsletter date yet", "email", staffEmail, "to", to)
		return
	}
	m := assignmentMessage(baseURL(r), a.from, sv, to)
	go func() {
		if err := a.mailer.Send(context.WithoutCancel(r.Context()), m); err != nil {
			slog.Error("birthday: mail invite", "error", err, "to", to, "email", staffEmail)
		}
	}()
}

// assignmentMessage is the email and its invite.
func assignmentMessage(base, from string, sv StaffView, to string) mail.Message {
	link := base + staffPath(sv.Email)
	ask := longDate(sv.RequestBy)
	subject := fmt.Sprintf("Ask %s about their birthday charity by %s", sv.Name, mediumDate(sv.RequestBy))
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
	fmt.Fprintf(&text, "%s's birthday is yours this year.\n\nReach out by %s and record the charity they choose:\n%s\n\n", sv.Name, ask, link)
	fmt.Fprintf(&htm, "<p style=\"font:16px/1.5 -apple-system,Segoe UI,Roboto,sans-serif\"><strong>%s</strong>'s birthday is yours this year.</p>", html.EscapeString(sv.Name))
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
		Attachments: []mail.Attachment{{
			Name:        "invite.ics",
			ContentType: "text/calendar; method=REQUEST; charset=utf-8",
			Content:     []byte(invite(from, to, sv, subject, description, link)),
		}},
	}
}

// invite is the calendar file: one all-day event on the day to ask by, from
// the app to the assignee, with a UID that is the birthday's for the year, so
// a later assignment of the same birthday replaces it on the calendar.
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

// longDate and mediumDate say a date cell the way the pages do: Friday,
// September 18, 2026, and September 18, 2026.
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

// mailAddress is the bare address inside "Name <address>".
func mailAddress(from string) string {
	if i := strings.LastIndex(from, "<"); i >= 0 {
		return strings.TrimSuffix(strings.TrimSpace(from[i+1:]), ">")
	}
	return strings.TrimSpace(from)
}

// icsEscape and icsFold write text the way RFC 5545 wants it: commas,
// semicolons, backslashes and newlines escaped, lines folded at 75 octets.
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
