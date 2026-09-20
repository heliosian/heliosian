package team

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"heliosian/internal/mail"
)

// The portal's email. Three kinds go out:
//
//   - a thank-you to whoever signed up (or was signed up), copied to the
//     chairs of the event - and to a student's parents - followed by a
//     second, short note to the volunteer alone carrying a calendar invite
//     for the thing, so a sign-up sheet never lands on a chair's calendar
//     forty times; and a cancellation for that invite when the sign-up is
//     removed;
//   - a note to someone made a co-chair, copied to the other chairs;
//   - notices to the admins who asked for them: a new event, a new thing
//     under one, a new sign-up, an offer to co-chair.
//
// Each admin's preferences live in the Settings tab as one row,
// notify:<email> = events,activities,signups,offers - whichever of those
// they turned on. Sending happens off the request, and a failure is logged
// rather than shown: the sign-up itself already took.

const notifyPrefix = "notify:"

// NotifyKinds are the admin notices, in the order the Admin Tools card
// lists them.
var NotifyKinds = []string{"events", "activities", "signups", "offers"}

// notifyPrefs is one admin's choices, by kind.
func (t *Tables) notifyPrefs(email string) map[string]bool {
	prefs := map[string]bool{}
	key := notifyPrefix + strings.ToLower(strings.TrimSpace(email))
	for _, row := range t.Settings {
		if strings.ToLower(strings.TrimSpace(row["Key"])) != key {
			continue
		}
		for _, k := range strings.Split(row["Value"], ",") {
			if k = strings.TrimSpace(k); k != "" {
				prefs[k] = true
			}
		}
	}
	return prefs
}

// adminsWanting is every admin who turned a kind of notice on, minus the
// person the notice is about or from - nobody needs telling what they did.
func (a app) adminsWanting(kind string, except ...string) []string {
	tables := a.cache.Tables()
	out := []string{}
	for _, admin := range a.cache.Admins(a.superAdmins()) {
		if slices.Contains(except, admin) {
			continue
		}
		if tables.notifyPrefs(admin)[kind] {
			out = append(out, admin)
		}
	}
	return out
}

// chairsAround is who runs a thing: its co-chairs, and those of everything
// above it, nearest first, each once - the people to copy on its mail.
func chairsAround(m *Model, act *Activity) []string {
	out := []string{}
	for n := act; n != nil; n = m.byID[n.Parent] {
		for _, email := range n.CoChairs() {
			if !slices.Contains(out, email) {
				out = append(out, email)
			}
		}
	}
	return out
}

// chairRows names who runs a thing, a row for each level from the thing
// itself up to the event, so a volunteer on something within an event sees
// its leads apart from the event's chairs: "Sealand Leads", then "Event
// Chairs". A level with nobody is left out; a thing on its own has "Chairs".
func (a app) chairRows(m *Model, act *Activity) [][2]string {
	rows := [][2]string{}
	for n := act; n != nil; n = m.byID[n.Parent] {
		names := []string{}
		for _, email := range n.CoChairs() {
			names = append(names, a.nameOf(email))
		}
		if len(names) == 0 {
			continue
		}
		label := "Chairs"
		if n.Parent != "" && m.byID[n.Parent] != nil {
			label = n.Title + " Leads"
		} else if n != act {
			label = "Event Chairs"
		}
		rows = append(rows, [2]string{label, strings.Join(names, ", ")})
	}
	return rows
}

// baseURL is the site as the request reached it, for the links and pictures
// in a message - team.heliosian.com in production.
func baseURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil && !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") && strings.HasPrefix(r.Host, "localhost") {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

// letter is what every message is built from: the thing it is about, laid
// out with its picture and details, and the words for this occasion.
type letter struct {
	Base    string
	Title   string
	Under   string
	When    string
	Where   string
	Path    string
	Picture string
	Heading string
	Intro   string
	Rows    [][2]string
	Button  string
	// Calendar is an "add to calendar" address for the thing, shown as a
	// second button when set - on a note that says someone is signed up.
	Calendar string
	Footnote string
}

func (a app) letterFor(base string, act *Activity) letter {
	model := a.cache.Model()
	l := letter{
		Base:  base,
		Title: act.Title,
		Under: lineage(model, act),
		When:  when(timed(model, act)),
		Where: act.Location,
		Path:  base + model.PathOf(act),
	}
	// The share card is the one picture of a thing a mail client can fetch
	// without signing in; it carries the banner or flyer with the title and
	// date. A pending or hidden thing has none.
	for n := act; n != nil; n = model.byID[n.Parent] {
		if previewable(n) {
			l.Picture = base + "/open/share/" + n.ID + ".png"
			break
		}
	}
	return l
}

var letterTemplate = template.Must(template.New("letter").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>{{.Heading}}</title></head>
<body style="margin:0;padding:0;background:#f3f6f6;font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;color:#1f2a2b;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f3f6f6;padding:24px 12px;">
<tr><td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%;background:#ffffff;border-radius:16px;overflow:hidden;box-shadow:0 2px 10px rgba(0,0,0,0.06);">
<tr><td style="background:#1f4d53;padding:14px 24px;">
<table role="presentation" cellpadding="0" cellspacing="0"><tr>
<td style="vertical-align:middle;"><img src="{{.Base}}/brand/logo-mark.png" width="28" height="28" alt="" style="display:block;border:0;"></td>
<td style="vertical-align:middle;padding-left:10px;color:#ffffff;font-weight:700;font-size:16px;letter-spacing:0.02em;">HCA-Team</td>
</tr></table>
</td></tr>
{{if .Picture}}<tr><td style="padding:0;"><a href="{{.Path}}" style="display:block;"><img src="{{.Picture}}" width="600" alt="{{.Title}}" style="display:block;width:100%;height:auto;border:0;"></a></td></tr>{{end}}
<tr><td style="padding:28px 28px 8px;">
<h1 style="margin:0 0 10px;font-size:24px;line-height:1.25;color:#1f4d53;">{{.Heading}}</h1>
<p style="margin:0 0 18px;font-size:16px;line-height:1.5;">{{.Intro}}</p>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f4f8f8;border-radius:12px;">
<tr><td style="padding:16px 18px;">
<div style="font-size:19px;font-weight:700;color:#1f4d53;">{{.Title}}</div>
{{if .Under}}<div style="font-size:13.5px;color:#5c6b6c;margin-top:2px;">Part of {{.Under}}</div>{{end}}
<table role="presentation" cellpadding="0" cellspacing="0" style="margin-top:12px;font-size:15px;line-height:1.45;">
{{if .When}}<tr><td style="color:#5c6b6c;padding:3px 16px 3px 0;white-space:nowrap;vertical-align:top;">When</td><td style="padding:3px 0;">{{.When}}</td></tr>{{end}}
{{if .Where}}<tr><td style="color:#5c6b6c;padding:3px 16px 3px 0;white-space:nowrap;vertical-align:top;">Where</td><td style="padding:3px 0;">{{.Where}}</td></tr>{{end}}
{{range .Rows}}<tr><td style="color:#5c6b6c;padding:3px 16px 3px 0;white-space:nowrap;vertical-align:top;">{{index . 0}}</td><td style="padding:3px 0;">{{index . 1}}</td></tr>{{end}}
</table>
</td></tr>
</table>
</td></tr>
<tr><td style="padding:18px 28px 28px;">
<a href="{{.Path}}" style="display:inline-block;background:#1f4d53;color:#ffffff;text-decoration:none;font-weight:700;font-size:15px;padding:12px 20px;border-radius:10px;">{{.Button}}</a>
{{if .Calendar}}<a href="{{.Calendar}}" style="display:inline-block;margin-left:10px;background:#ffffff;color:#1f4d53;border:2px solid #1f4d53;text-decoration:none;font-weight:700;font-size:15px;padding:10px 18px;border-radius:10px;">Add to Calendar</a>{{end}}
{{if .Footnote}}<p style="margin:18px 0 0;font-size:13.5px;line-height:1.5;color:#5c6b6c;">{{.Footnote}}</p>{{end}}
</td></tr>
<tr><td style="padding:14px 28px;background:#f4f8f8;font-size:12.5px;color:#7a8788;">
Sent by HCA-Team, the HCA volunteer portal · <a href="{{.Base}}" style="color:#1f4d53;">{{.Base}}</a>
</td></tr>
</table>
</td></tr>
</table>
</body></html>`))

func (l letter) render() (string, string) {
	var html bytes.Buffer
	if err := letterTemplate.Execute(&html, l); err != nil {
		slog.Error("mail: render", "error", err)
	}
	var text strings.Builder
	fmt.Fprintf(&text, "%s\n\n%s\n\n%s\n", l.Heading, l.Intro, l.Title)
	if l.Under != "" {
		fmt.Fprintf(&text, "Part of %s\n", l.Under)
	}
	if l.When != "" {
		fmt.Fprintf(&text, "When: %s\n", l.When)
	}
	if l.Where != "" {
		fmt.Fprintf(&text, "Where: %s\n", l.Where)
	}
	for _, row := range l.Rows {
		fmt.Fprintf(&text, "%s: %s\n", row[0], row[1])
	}
	fmt.Fprintf(&text, "\n%s\n", l.Path)
	if l.Calendar != "" {
		fmt.Fprintf(&text, "Add to calendar: %s\n", l.Calendar)
	}
	if l.Footnote != "" {
		fmt.Fprintf(&text, "\n%s\n", l.Footnote)
	}
	return html.String(), text.String()
}

// send posts a message off the request; the outcome is logged.
func (a app) send(ctx context.Context, subject string, to []string, cc []string, l letter, replyTo ...string) {
	a.post(ctx, a.compose(subject, to, cc, l, replyTo...))
}

// compose is the message: recipients deduplicated, nobody copied on their
// own message, the letter rendered.
func (a app) compose(subject string, to []string, cc []string, l letter, replyTo ...string) mail.Message {
	seen := map[string]bool{}
	clean := func(list []string) []string {
		out := []string{}
		for _, e := range list {
			e = strings.ToLower(strings.TrimSpace(e))
			if e == "" || !strings.Contains(e, "@") || seen[e] {
				continue
			}
			seen[e] = true
			out = append(out, e)
		}
		return out
	}
	m := mail.Message{To: clean(to), CC: clean(cc), Subject: subject}
	// Reply-To goes to whoever should hear back - the chairs - so a reply to
	// the portal's address reaches a person.
	seen = map[string]bool{}
	m.ReplyTo = clean(replyTo)
	m.HTML, m.Text = l.render()
	return m
}

func (a app) post(ctx context.Context, m mail.Message) {
	if a.mailer == nil || len(m.To) == 0 {
		return
	}
	go func() {
		if err := a.mailer.Send(context.WithoutCancel(ctx), m); err != nil {
			slog.Error("mail: send", "error", err, "subject", m.Subject, "to", m.To)
		}
	}()
}

// window is a thing's hours on a calendar: those of the thing itself or the
// nearest thing above it with a date, the start and end as written, two
// hours from the start when there is no end, the whole day when there is
// no time; ok is false when nothing above it is dated either. A thing with
// only a Timing in words ("two hours before doors") counts as undated.
func window(m *Model, act *Activity) (start, until time.Time, timed, ok bool) {
	dated := act
	for dated != nil && dated.Start == "" {
		dated = m.byID[dated.Parent]
	}
	if dated == nil {
		return start, until, false, false
	}
	start, err := time.ParseInLocation(DateTimeFormat, dated.Start, local)
	timed = err == nil
	if !timed {
		if start, err = time.ParseInLocation(DateFormat, dated.Start, local); err != nil {
			return start, until, false, false
		}
	}
	if end, err := time.ParseInLocation(DateTimeFormat, dated.End, local); err == nil && timed && end.After(start) {
		until = end
	} else if end, err := time.ParseInLocation(DateFormat, dated.End, local); err == nil && !timed && end.After(start) {
		until = end.Add(24 * time.Hour)
	} else if timed {
		until = start.Add(2 * time.Hour)
	} else {
		until = start.Add(24 * time.Hour)
	}
	return start, until, timed, true
}

// calendarLink is the Google Calendar "add this" address for a thing: the
// title, its hours, the location, and its page in the notes. Empty for a
// thing with no date.
func calendarLink(m *Model, act *Activity, page string) string {
	start, until, timed, ok := window(m, act)
	if !ok {
		return ""
	}
	stamp := func(t time.Time) string {
		if timed {
			return t.Format("20060102T150405")
		}
		return t.Format("20060102")
	}
	q := url.Values{
		"action": {"TEMPLATE"}, "text": {act.Title}, "dates": {stamp(start) + "/" + stamp(until)}, "details": {page}, "location": {act.Location},
	}
	return "https://calendar.google.com/calendar/render?" + q.Encode()
}

// invite is the calendar file on a note that says someone is signed up -
// or, cancelled, that they no longer are: the thing as one event, from the
// portal to each person the note is for, all marked as coming. Its UID is
// the thing's and the volunteer's, so a later note for the same sign-up
// replaces the entry rather than adding a twin, and a cancellation finds
// it. Nothing for a thing with no date.
func (a app) invite(m *Model, act *Activity, email, page string, to []string, cancel bool) (mail.Attachment, bool) {
	start, until, timed, ok := window(m, act)
	if !ok {
		return mail.Attachment{}, false
	}
	method, status := "REQUEST", "CONFIRMED"
	if cancel {
		method, status = "CANCEL", "CANCELLED"
	}
	stamp := time.Now().UTC()
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//HCA-Team//EN",
		"METHOD:" + method,
		"BEGIN:VEVENT",
		"UID:" + mail.ICSEscape(fmt.Sprintf("team-%s-%s@heliosian.com", act.ID, email)),
		"DTSTAMP:" + stamp.Format("20060102T150405Z"),
		"SEQUENCE:" + fmt.Sprint(stamp.Unix()),
	}
	if timed {
		lines = append(lines, "DTSTART:"+start.UTC().Format("20060102T150405Z"), "DTEND:"+until.UTC().Format("20060102T150405Z"))
	} else {
		lines = append(lines, "DTSTART;VALUE=DATE:"+start.Format("20060102"), "DTEND;VALUE=DATE:"+until.Format("20060102"))
	}
	title := act.Title
	if under := lineage(m, act); under != "" {
		title += " (" + under + ")"
	}
	lines = append(lines,
		"SUMMARY:"+mail.ICSEscape(title),
		"DESCRIPTION:"+mail.ICSEscape(page),
		"URL:"+mail.ICSEscape(page),
		"ORGANIZER;CN=HCA-Team:mailto:"+mail.Address(a.from),
	)
	for _, e := range to {
		lines = append(lines, fmt.Sprintf("ATTENDEE;CN=%s;ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED;RSVP=FALSE:mailto:%s", mail.ICSEscape(a.nameOf(e)), e))
	}
	if act.Location != "" {
		lines = append(lines, "LOCATION:"+mail.ICSEscape(act.Location))
	}
	lines = append(lines, "STATUS:"+status, "END:VEVENT", "END:VCALENDAR")
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(mail.ICSFold(line) + "\r\n")
	}
	return mail.Attachment{Name: "invite.ics", ContentType: "text/calendar; method=" + method + "; charset=utf-8", Content: []byte(b.String())}, true
}

// mailRemoved follows a sign-up's removal: whoever held it (a student's
// parents too) gets a cancellation for the invite, so the thing comes off
// their calendar. Nothing goes to anyone else, and nothing for a thing
// with no date - there was no invite.
func (a app) mailRemoved(r *http.Request, act *Activity, email string) {
	m := a.cache.Model()
	l := a.letterFor(baseURL(r), act)
	l.Heading = "You're no longer signed up"
	l.Intro = fmt.Sprintf("Your sign-up for %s was removed, so it comes off your calendar. If that's a surprise, the chairs can put you back - just reply.", act.Title)
	l.Button = "See the details"
	msg := a.compose("Removed: "+act.Title, append([]string{email}, a.directory.Parents(email)...), nil, l, without(chairsAround(m, act), email)...)
	inv, ok := a.invite(m, act, email, l.Path, msg.To, true)
	if !ok {
		return
	}
	msg.Attachments = []mail.Attachment{inv}
	a.post(r.Context(), msg)
}

// nameOf is someone's name as the directory has it, or their address.
func (a app) nameOf(email string) string {
	name, _, _ := a.directory.Person(email)
	if name == "" {
		return displayName(email)
	}
	return name
}

// mailSignUp follows a saved sign-up: a new volunteer is thanked (copying the
// chairs, and a student's parents), a new co-chair is told, and the admins
// who asked hear of a new sign-up or an offer to co-chair.
func (a app) mailSignUp(r *http.Request, act *Activity, email, position, note, actor string, existed bool, was string) {
	base := baseURL(r)
	ctx := r.Context()
	// The model as it stands after the save, so a fresh co-chair counts.
	model := a.cache.Model()
	if now := model.Activity(act.ID); now != nil {
		act = now
	}
	chairs := chairsAround(model, act)
	l := a.letterFor(base, act)
	name := a.nameOf(email)
	first := strings.Fields(name)
	hi := "Hi"
	if len(first) > 0 {
		hi = "Hi " + first[0]
	}
	root := act
	for root.Parent != "" && model.byID[root.Parent] != nil {
		root = model.byID[root.Parent]
	}
	switch {
	case position == PositionCoChair && was != PositionCoChair:
		l.Heading = fmt.Sprintf("You're a co-chair of %s", act.Title)
		l.Intro = fmt.Sprintf("%s - %s made you a co-chair. You can now edit the page, add things under it, and see and manage everyone who signs up.", hi, a.nameOf(actor))
		if len(chairs) > 1 {
			others := []string{}
			for _, c := range chairs {
				if c != email {
					others = append(others, a.nameOf(c))
				}
			}
			l.Rows = append(l.Rows, [2]string{"Co-chairs", strings.Join(others, ", ")})
		}
		l.Button = "Open " + act.Title
		l.Footnote = "The other co-chairs are copied on this note."
		a.send(ctx, fmt.Sprintf("You're a co-chair of %s", act.Title), []string{email}, without(chairs, email), l, without(chairs, email)...)
	case !existed:
		by := ""
		if actor != email {
			by = fmt.Sprintf(" %s signed you up.", a.nameOf(actor))
		}
		l.Heading = "Thank you for volunteering!"
		l.Intro = fmt.Sprintf("%s - you're signed up for %s.%s The chairs are copied here, so just reply if you have a question.", hi, act.Title, by)
		if position == PositionOpen {
			l.Rows = append(l.Rows, [2]string{"Your role", "Volunteer, and open to co-chairing"})
		} else {
			l.Rows = append(l.Rows, [2]string{"Your role", "Volunteer"})
		}
		if note != "" {
			l.Rows = append(l.Rows, [2]string{"Your note", note})
		}
		l.Rows = append(l.Rows, a.chairRows(model, act)...)
		l.Button = "See the details"
		l.Calendar = calendarLink(model, act, l.Path)
		l.Footnote = "Need to change or cancel? Open the page and use Edit my sign-up."
		// The thank-you goes to the volunteer - a student's parents too -
		// with the chairs copied, so everyone concerned reads the same
		// thing; then a second, short note to the volunteer alone carries
		// the calendar invite, so nothing lands on a chair's calendar.
		subject := fmt.Sprintf("Thanks for volunteering for %s", act.Title)
		to := append([]string{email}, a.directory.Parents(email)...)
		replyTo := without(chairs, email)
		note := a.compose(subject, to, replyTo, l, replyTo...)
		a.post(ctx, note)
		if inv, ok := a.invite(model, act, email, l.Path, note.To, false); ok {
			il := a.letterFor(base, act)
			il.Heading = "Add it to your calendar"
			il.Intro = fmt.Sprintf("Here's the calendar invite for %s - accept it and it's on your calendar. The details of your sign-up are in the note that came with it.", act.Title)
			il.Button = "See the details"
			il.Calendar = l.Calendar
			invite := a.compose("Calendar invite: "+act.Title, note.To, nil, il, replyTo...)
			invite.Attachments = []mail.Attachment{inv}
			a.post(ctx, invite)
		}
	}
	// Admin notices: a fresh sign-up, and any offer to co-chair - whether it
	// came with the sign-up or was added to one later.
	if !existed {
		if admins := a.adminsWanting("signups", actor, email); len(admins) > 0 {
			n := a.letterFor(base, act)
			n.Heading = fmt.Sprintf("%s signed up for %s", name, act.Title)
			n.Intro = fmt.Sprintf("A new sign-up on %s.", root.Title)
			n.Rows = [][2]string{{"Who", fmt.Sprintf("%s (%s)", name, email)}, {"Role", position}}
			if note != "" {
				n.Rows = append(n.Rows, [2]string{"Note", note})
			}
			if actor != email {
				n.Rows = append(n.Rows, [2]string{"Signed up by", a.nameOf(actor)})
			}
			n.Button = "Open " + act.Title
			a.send(ctx, fmt.Sprintf("New sign-up: %s for %s", name, act.Title), admins, nil, n)
		}
	}
	if position == PositionOpen && was != PositionOpen {
		if admins := a.adminsWanting("offers", actor, email); len(admins) > 0 {
			n := a.letterFor(base, act)
			n.Heading = fmt.Sprintf("%s offered to co-chair %s", name, act.Title)
			n.Intro = "Someone is open to co-chairing. Open the page to make them a co-chair, or leave them as a volunteer."
			n.Rows = [][2]string{{"Who", fmt.Sprintf("%s (%s)", name, email)}}
			if note != "" {
				n.Rows = append(n.Rows, [2]string{"Note", note})
			}
			n.Button = "Open " + act.Title
			a.send(ctx, fmt.Sprintf("Co-chair offer: %s for %s", name, act.Title), admins, nil, n)
		}
	}
}

// mailNewActivity tells the admins who asked that something was added: an
// event on the page, or a thing under one.
func (a app) mailNewActivity(r *http.Request, act *Activity, actor string) {
	kind, what := "events", "A new event was added"
	if act.Parent != "" {
		kind, what = "activities", "Something new was added under "+lineage(a.cache.Model(), act)
	}
	admins := a.adminsWanting(kind, actor)
	if len(admins) == 0 {
		return
	}
	l := a.letterFor(baseURL(r), act)
	l.Heading = act.Title
	l.Intro = fmt.Sprintf("%s by %s.", what, a.nameOf(actor))
	l.Rows = [][2]string{{"Status", act.Status}}
	if act.Status == StatusPending {
		l.Rows = append(l.Rows, [2]string{"Needs", "Your approval, under Approval Needed"})
	}
	l.Button = "Open " + act.Title
	a.send(r.Context(), fmt.Sprintf("New: %s", act.Title), admins, nil, l)
}

func without(list []string, drop string) []string {
	out := []string{}
	for _, e := range list {
		if e != drop {
			out = append(out, e)
		}
	}
	return out
}
