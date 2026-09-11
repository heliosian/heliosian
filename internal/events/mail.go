package events

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"heliosian/internal/mail"
)

// The portal's email. Three kinds go out:
//
//   - a thank-you to whoever signed up (or was signed up), copied to the
//     chairs of the event - and to a student's parents;
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

// baseURL is the site as the request reached it, for the links and pictures
// in a message - hca.lab.heliosian.com today, hca.heliosian.com later.
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
	Base     string
	Title    string
	Under    string
	When     string
	Where    string
	Path     string
	Picture  string
	Heading  string
	Intro    string
	Rows     [][2]string
	Button   string
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
			l.Picture = base + "/share/" + n.ID + ".png"
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
	if l.Footnote != "" {
		fmt.Fprintf(&text, "\n%s\n", l.Footnote)
	}
	return html.String(), text.String()
}

// send posts a message off the request; the outcome is logged. Recipients
// are deduplicated and nobody is copied on their own message.
func (a app) send(ctx context.Context, subject string, to []string, cc []string, l letter, replyTo ...string) {
	if a.mailer == nil || len(to) == 0 {
		return
	}
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
	go func() {
		if err := a.mailer.Send(context.WithoutCancel(ctx), m); err != nil {
			slog.Error("mail: send", "error", err, "subject", subject, "to", m.To)
		}
	}()
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
		if len(chairs) > 0 {
			names := []string{}
			for _, c := range chairs {
				names = append(names, a.nameOf(c))
			}
			l.Rows = append(l.Rows, [2]string{"Chairs", strings.Join(names, ", ")})
		}
		l.Button = "See the details"
		l.Footnote = "Need to change or cancel? Open the page and use Edit my sign-up."
		cc := append(without(chairs, email), a.directory.Parents(email)...)
		a.send(ctx, fmt.Sprintf("Thanks for volunteering for %s", act.Title), []string{email}, cc, l, without(chairs, email)...)
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
