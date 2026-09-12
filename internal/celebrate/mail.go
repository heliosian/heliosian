package celebrate

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"heliosian/internal/mail"
)

// The site's email. Two kinds go out, each to whoever is billed with the
// party's hosts copied (and on Reply-To, so a reply reaches a person):
//
//   - a confirmation when tickets are taken - who is coming, what each
//     costs, and how invoicing works;
//   - a note when a host offers a waitlisted family a ticket.
//
// Sending happens off the request, and a failure is logged rather than
// shown: the tickets themselves already took.

// baseURL is the site as the request reached it, for the links and picture
// in a message.
func baseURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil && !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") && strings.HasPrefix(r.Host, "localhost") {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

// letter is what every message is built from: the party, laid out with its
// picture and details, and the words for this occasion.
type letter struct {
	Base     string
	Title    string
	Subtitle string
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

func (a app) letterFor(base string, p *Party) letter {
	model := a.cache.Model()
	l := letter{Base: base, Title: p.Title, Subtitle: p.Subtitle, When: when(p), Where: p.Location, Path: base + model.PathOf(p)}
	// The recipient is signed in, so the street address goes along too.
	if p.Address != "" {
		if l.Where != "" {
			l.Where += " · "
		}
		l.Where += p.Address
	}
	// The share card is the one picture a mail client can fetch without
	// signing in; a pending or hidden party has none.
	if previewable(p) {
		l.Picture = base + "/share/" + p.ID + ".png"
	}
	return l
}

var letterTemplate = template.Must(template.New("letter").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>{{.Heading}}</title></head>
<body style="margin:0;padding:0;background:#f3f6f6;font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;color:#1f2a2b;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f3f6f6;padding:24px 12px;">
<tr><td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%;background:#ffffff;border-radius:16px;overflow:hidden;box-shadow:0 2px 10px rgba(0,0,0,0.06);">
<tr><td style="background:#0f4e54;padding:14px 24px;">
<table role="presentation" cellpadding="0" cellspacing="0"><tr>
<td style="vertical-align:middle;"><img src="{{.Base}}/brand/logo-mark.png" width="28" height="28" alt="" style="display:block;border:0;"></td>
<td style="vertical-align:middle;padding-left:10px;color:#ffffff;font-weight:700;font-size:16px;letter-spacing:0.02em;">Helios Celebrate</td>
</tr></table>
</td></tr>
{{if .Picture}}<tr><td style="padding:0;"><a href="{{.Path}}" style="display:block;"><img src="{{.Picture}}" width="600" alt="{{.Title}}" style="display:block;width:100%;height:auto;border:0;"></a></td></tr>{{end}}
<tr><td style="padding:28px 28px 8px;">
<h1 style="margin:0 0 10px;font-size:24px;line-height:1.25;color:#0f4e54;">{{.Heading}}</h1>
<p style="margin:0 0 18px;font-size:16px;line-height:1.5;">{{.Intro}}</p>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f4f8f8;border-radius:12px;">
<tr><td style="padding:16px 18px;">
<div style="font-size:19px;font-weight:700;color:#0f4e54;">{{.Title}}</div>
{{if .Subtitle}}<div style="font-size:13.5px;color:#5c6b6c;margin-top:2px;">{{.Subtitle}}</div>{{end}}
<table role="presentation" cellpadding="0" cellspacing="0" style="margin-top:12px;font-size:15px;line-height:1.45;">
{{if .When}}<tr><td style="color:#5c6b6c;padding:3px 16px 3px 0;white-space:nowrap;vertical-align:top;">When</td><td style="padding:3px 0;">{{.When}}</td></tr>{{end}}
{{if .Where}}<tr><td style="color:#5c6b6c;padding:3px 16px 3px 0;white-space:nowrap;vertical-align:top;">Where</td><td style="padding:3px 0;">{{.Where}}</td></tr>{{end}}
{{range .Rows}}<tr><td style="color:#5c6b6c;padding:3px 16px 3px 0;white-space:nowrap;vertical-align:top;">{{index . 0}}</td><td style="padding:3px 0;">{{index . 1}}</td></tr>{{end}}
</table>
</td></tr>
</table>
</td></tr>
<tr><td style="padding:18px 28px 28px;">
<a href="{{.Path}}" style="display:inline-block;background:#0f4e54;color:#ffffff;text-decoration:none;font-weight:700;font-size:15px;padding:12px 20px;border-radius:10px;">{{.Button}}</a>
{{if .Footnote}}<p style="margin:18px 0 0;font-size:13.5px;line-height:1.5;color:#5c6b6c;">{{.Footnote}}</p>{{end}}
</td></tr>
<tr><td style="padding:14px 28px;background:#f4f8f8;font-size:12.5px;color:#7a8788;">
Sent by Helios Celebrate, the fun(d)raiser parties site · <a href="{{.Base}}" style="color:#0f4e54;">{{.Base}}</a>
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
	if l.Subtitle != "" {
		fmt.Fprintf(&text, "%s\n", l.Subtitle)
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
	seen = map[string]bool{}
	m.ReplyTo = clean(replyTo)
	m.HTML, m.Text = l.render()
	go func() {
		if err := a.mailer.Send(context.WithoutCancel(ctx), m); err != nil {
			slog.Error("mail: send", "error", err, "subject", subject, "to", m.To)
		}
	}()
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

// firstName is the greeting's name: the first word of what the directory
// calls someone.
func (a app) firstName(email string) string {
	words := strings.Fields(a.nameOf(email))
	if len(words) == 0 {
		return ""
	}
	return words[0]
}

// mailTickets follows a purchase: a confirmation to whoever is billed, the
// hosts copied - and, when someone else took the tickets for the family,
// that person too - saying who is coming, which are sold and which wait,
// what it comes to, and how invoicing works.
func (a app) mailTickets(r *http.Request, p *Party, purchaser string, taken []map[string]string, actor string) {
	if a.mailer == nil || len(taken) == 0 {
		return
	}
	base := baseURL(r)
	model := a.cache.Model()
	if now := model.Party(p.ID); now != nil {
		p = now
	}
	l := a.letterFor(base, p)
	sold := []string{}
	waiting := 0
	total := 0.0
	for _, t := range taken {
		who := t["Name"]
		if t["Email"] != "" {
			who = a.nameOf(t["Email"])
		}
		if t["Status"] == TicketSold {
			sold = append(sold, who)
			price, _ := ParsePrice(t["Price"])
			total += price
		} else {
			n, _ := strconv.Atoi(t["Quantity"])
			waiting += max(n, 1)
		}
	}
	// A single ticket for someone other than whoever is billed - a child, a
	// guest, a colleague a host added - is that person's note: it speaks to
	// them by name, and goes to them (a student: to their parents) with the
	// purchaser alongside. Anything else speaks to the purchaser.
	to := []string{purchaser}
	holder := ""
	if len(sold) == 1 && waiting == 0 {
		t := taken[0]
		if t["Email"] != purchaser {
			holder = sold[0]
			if t["Email"] != "" {
				if person, known := a.directory.Person(t["Email"]); known && person.IsStudent {
					to = append(to, person.ParentEmails...)
				} else {
					to = append(to, t["Email"])
				}
			}
		}
	}
	hi := "Hi"
	if holder != "" {
		if words := strings.Fields(holder); len(words) > 0 {
			hi = "Hi " + words[0]
		}
	} else if first := a.firstName(purchaser); first != "" {
		hi = "Hi " + first
	}
	by := ""
	free := len(sold) > 0 && total == 0
	switch {
	case free:
		by = fmt.Sprintf(" %s has added you at no charge - a gift from the hosts.", a.nameOf(actor))
	case holder != "":
		by = fmt.Sprintf(" %s took it for you.", a.nameOf(actor))
	case actor != purchaser:
		by = fmt.Sprintf(" %s took them for your family.", a.nameOf(actor))
	}
	subject := ""
	switch {
	case len(sold) > 0 && waiting > 0:
		l.Heading = "Your tickets, and a place on the waitlist"
		l.Intro = fmt.Sprintf("%s - %s. The party was full before everyone could get in, so your family is on the waitlist for %d more; the hosts will offer places as they open up. The hosts are copied here, so just reply if you have a question.", hi, ticketsWords(len(sold), p.Title), waiting) + by
		subject = "Your tickets to " + p.Title
	case holder != "":
		l.Heading = strings.Fields(holder)[0] + ", you're going!"
		l.Intro = fmt.Sprintf("%s - %s.%s The hosts are copied here, so just reply if you have a question.", hi, ticketsWords(1, p.Title), by)
		subject = holder + "'s ticket to " + p.Title
	case len(sold) > 0:
		l.Heading = "You're going!"
		l.Intro = fmt.Sprintf("%s - %s.%s The hosts are copied here, so just reply if you have a question.", hi, ticketsWords(len(sold), p.Title), by)
		subject = "Your tickets to " + p.Title
	default:
		l.Heading = "You're on the waitlist"
		l.Intro = fmt.Sprintf("%s - %s is full, so your family is on its waitlist for %d %s.%s Nothing is billed unless a place opens up; the hosts will offer places as they do, and you'll get a note when it happens.", hi, p.Title, waiting, plural(waiting, "ticket"), by)
		subject = "You're on the waitlist for " + p.Title
	}
	if len(sold) > 0 {
		l.Rows = append(l.Rows, [2]string{"Tickets", strings.Join(sold, ", ")})
	}
	if waiting > 0 {
		l.Rows = append(l.Rows, [2]string{"Waitlist", fmt.Sprintf("%d %s", waiting, plural(waiting, "ticket"))})
	}
	switch {
	case free:
		l.Rows = append(l.Rows, [2]string{"Total", "Free"})
	case len(sold) > 0:
		l.Rows = append(l.Rows, [2]string{"Total", fmt.Sprintf("$%s (%d × $%s)", PriceCell(total), len(sold), PriceCell(p.Price))})
	}
	if !free {
		l.Rows = append(l.Rows, [2]string{"Billed to", fmt.Sprintf("%s (%s)", a.nameOf(purchaser), purchaser)})
	}
	if note := taken[0]["Note"]; note != "" {
		l.Rows = append(l.Rows, [2]string{"Your note", note})
	}
	if names := a.hostNames(p); names != "" {
		l.Rows = append(l.Rows, [2]string{"Hosts", names})
	}
	l.Button = "See the party"
	if len(sold) > 0 {
		l.Footnote = model.Settings.TicketNote
	}
	cc := []string{}
	if actor != purchaser {
		cc = append(cc, actor)
	}
	// A purchase copies the hosts in; a waitlist request sends them a note
	// of their own instead (mailWaitlistHosts), whose reply goes to the
	// family rather than back to themselves.
	if len(sold) > 0 {
		cc = append(cc, without(p.HostEmails, purchaser)...)
	}
	a.send(r.Context(), subject, to, cc, l, without(p.HostEmails, purchaser)...)
	if len(sold) == 0 && waiting > 0 {
		a.mailWaitlistHosts(r, p, purchaser, waiting, taken[0]["Note"], actor)
	}
}

// mailWaitlistHosts tells the hosts a family is waiting: who, how many, and
// their note, with Reply-To set to the family so a host can write straight
// back, and the party page a click away to offer the places.
func (a app) mailWaitlistHosts(r *http.Request, p *Party, purchaser string, waiting int, note, actor string) {
	hosts := without(p.HostEmails, purchaser)
	if a.mailer == nil || len(hosts) == 0 {
		return
	}
	l := a.letterFor(baseURL(r), p)
	who := a.nameOf(purchaser)
	l.Heading = fmt.Sprintf("%s joined the waitlist", who)
	l.Intro = fmt.Sprintf("%s would like %d %s to %s once places open up. Nothing is billed until you offer them - open the party and use Offer beside the request when you can. Reply to this note to reach %s directly.", who, waiting, plural(waiting, "ticket"), p.Title, who)
	if actor != purchaser {
		l.Intro += fmt.Sprintf(" (%s made the request for the family.)", a.nameOf(actor))
	}
	l.Rows = [][2]string{{"Waiting", fmt.Sprintf("%s (%s)", who, purchaser)}, {"Tickets", fmt.Sprintf("%d", waiting)}}
	if note != "" {
		l.Rows = append(l.Rows, [2]string{"Their note", note})
	}
	l.Rows = append(l.Rows, [2]string{"Waitlist", fmt.Sprintf("%d %s in all", p.Waiting(), plural(p.Waiting(), "ticket"))})
	l.Button = "Open the party"
	a.send(r.Context(), fmt.Sprintf("Waitlist for %s: %s wants %d %s", p.Title, who, waiting, plural(waiting, "ticket")), hosts, nil, l, purchaser)
}

// mailOffered follows a host answering a waitlist request: the family hears
// the places are theirs, and that any guests are named with Reassign.
func (a app) mailOffered(r *http.Request, p *Party, purchaser string, tickets []map[string]string, actor string) {
	if a.mailer == nil || len(tickets) == 0 {
		return
	}
	base := baseURL(r)
	if now := a.cache.Model().Party(p.ID); now != nil {
		p = now
	}
	l := a.letterFor(base, p)
	hi := "Hi"
	if first := a.firstName(purchaser); first != "" {
		hi = "Hi " + first
	}
	n := len(tickets)
	names := []string{}
	total := 0.0
	for _, t := range tickets {
		if t["Email"] != "" {
			names = append(names, a.nameOf(t["Email"]))
		} else {
			names = append(names, t["Name"])
		}
		price, _ := ParsePrice(t["Price"])
		total += price
	}
	l.Heading = "A place opened up!"
	l.Intro = fmt.Sprintf("%s - %s has offered your family %d %s to %s, off the waitlist. They're yours now. The hosts are copied here, so just reply if you have a question.", hi, a.nameOf(actor), n, plural(n, "ticket"), p.Title)
	l.Rows = [][2]string{{"Tickets", strings.Join(names, ", ")}, {"Total", fmt.Sprintf("$%s (%d × $%s)", PriceCell(total), n, PriceCell(total/float64(n)))}, {"Billed to", fmt.Sprintf("%s (%s)", a.nameOf(purchaser), purchaser)}}
	if hosts := a.hostNames(p); hosts != "" {
		l.Rows = append(l.Rows, [2]string{"Hosts", hosts})
	}
	l.Button = "See the party"
	l.Footnote = "A ticket marked \"to be named\" is a guest's: open the party and use Reassign beside it to say who is coming. " + a.cache.Model().Settings.TicketNote
	a.send(r.Context(), fmt.Sprintf("You're in: %d %s to %s", n, plural(n, "ticket"), p.Title), []string{purchaser}, without(p.HostEmails, purchaser), l, without(p.HostEmails, purchaser)...)
}

func (a app) hostNames(p *Party) string {
	if p.Hosts != "" {
		return p.Hosts
	}
	names := []string{}
	for _, h := range p.HostEmails {
		names = append(names, a.nameOf(h))
	}
	return strings.Join(names, ", ")
}

func ticketsWords(n int, title string) string {
	if n == 1 {
		return "you have a ticket to " + title
	}
	return fmt.Sprintf("you have %d tickets to %s", n, title)
}
