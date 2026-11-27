package db

import (
	"cmp"
	"context"
	"encoding/hex"
	"fmt"
	"html"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

const whenUID = "@when.heliosian.com"

var whenBrand = mail.Brand{Name: "Helios When", Color: "#1f4d53", Tagline: "the school year, day by day"}

var queuedKinds = []string{"invitation", "resend", "reminder", "update", "post", "thanks"}

const inviteProgressBatch = 50

func (iv *Invitations) run() {
	ctx := context.Background()
	for {
		select {
		case <-iv.in.wake:
			changes := iv.in.take()
			if !iv.started.Load() {
				continue
			}
			for _, d := range changes {
				iv.react(ctx, d)
			}
			iv.pass(ctx)
		case <-iv.kick:
			if iv.started.Load() {
				iv.pass(ctx)
			}
		}
	}
}

func (iv *Invitations) pass(ctx context.Context) {
	iv.autoInvite(ctx)
	iv.autoRemind(ctx)
	iv.sendQueued(ctx)
}

func (iv *Invitations) autoInvite(ctx context.Context) {
	m := iv.s.Model()
	now := time.Now()
	for _, g := range m.Table("GROUP").All() {
		auto, _ := cells.YesNo(g["auto_invite"], false)
		if !auto || !slices.Contains(guestListKinds, g["kind"]) || g["status"] != "open" || m.ended(g, now) {
			continue
		}
		pending := m.pendingOf(g["id"])
		if len(pending) == 0 {
			continue
		}
		edits := append(answerGroupEdits(g), invitationEdits(g["id"], "", "invitation", pending, now)...)
		if _, err := iv.writeGuests(ctx, access.System(invitationsJob), edits, g["id"], "", pending, 0); err != nil {
			slog.ErrorContext(ctx, "invitations: auto-invite not recorded", "group", g["id"], "error", err)
			continue
		}
		slog.InfoContext(ctx, "invitations: newcomers invited", "group", g["id"], "people", len(pending))
	}
}

func (iv *Invitations) sendQueued(ctx context.Context) {
	iv.s.Flush()
	m := iv.s.Model()
	edits := []Edit{}
	persist := func() bool {
		if len(edits) == 0 {
			return true
		}
		if _, err := iv.write(ctx, access.System(invitationsJob), edits); err != nil {
			slog.ErrorContext(ctx, "invitations: sends not recorded", "copies", len(edits), "error", err)
			return false
		}
		iv.s.Flush()
		edits = nil
		return true
	}
	for _, msg := range m.Table("MESSAGE").All() {
		if msg["direction"] != "out" || !slices.Contains(queuedKinds, msg["kind"]) || (msg["kind"] == "post" && msg["parent"] != "") {
			continue
		}
		for _, queued := range m.Table("RECIPIENT").Referencing("message", msg["id"]) {
			current := iv.s.Model()
			r, ok := current.Table("RECIPIENT").Get(queued["id"])
			if !ok {
				continue
			}
			if r["sent"] != "" || r["failed"] != "" {
				continue
			}
			g, ok := current.Table("GROUP").Get(msg["group"])
			if !ok || !slices.Contains(guestListKinds, g["kind"]) {
				continue
			}
			now := time.Now()
			var cells map[string]any
			switch {
			case current.keptOff(g, r["person"]) && !(msg["kind"] == "update" && g["status"] == "cancelled" && current.copiesOf(g["id"])[r["person"]]["sent"] != ""):
				cells = map[string]any{"failed": stamp(now), "detail": "the person was taken off the guest list before it went"}
			case g["status"] == "closed" || (g["status"] == "cancelled" && (msg["kind"] == "invitation" || msg["kind"] == "reminder")):
				cells = map[string]any{"failed": stamp(now), "detail": "the event was closed or cancelled before it went"}
			case current.ended(g, now) && g["status"] != "cancelled" && msg["kind"] != "thanks":
				cells = map[string]any{"failed": stamp(now), "detail": "the event had passed before it went"}
			default:
				var err error
				cells, err = iv.sendCopy(ctx, current, g, msg, r)
				if err != nil {
					slog.ErrorContext(ctx, "invitations: send failed", "recipient", r["id"], "group", g["id"], "kind", msg["kind"], "error", err)
					continue
				}
			}
			edits = append(edits, Edit{Set: r["id"], Cells: cells})
			if len(edits) == inviteProgressBatch && !persist() {
				return
			}
		}
	}
	persist()
}

func (iv *Invitations) organizer(token string) string {
	at := strings.LastIndex(iv.config.ReplyTo, "@")
	return iv.config.ReplyTo[:at] + "+" + hex.EncodeToString([]byte(token)) + iv.config.ReplyTo[at:]
}

func (iv *Invitations) linkFor(m *Model, g store.Row, person, token string) string {
	app := "when"
	if g["theme"] != "" {
		app = "gather"
	}
	if p, ok := m.Table("PERSON").Get(person); ok && p["source"] == "guest" {
		return iv.config.Origin(app) + "/ext/" + token
	}
	return iv.config.Origin(app) + g["path"]
}

func (m *Model) namesOf(people []string) []string {
	out := []string{}
	for _, p := range people {
		out = append(out, m.nameOf(p))
	}
	return out
}

func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

func (m *Model) householdOnList(g store.Row, person string) []string {
	out := []string{person}
	for _, p := range m.householdOf(person)[1:] {
		if m.onList(g, p) {
			out = append(out, p)
		}
	}
	return out
}

func (m *Model) studentCC(person string, msg store.Row) []string {
	if !m.students()[person] {
		return nil
	}
	parents := without(m.withParents(person), person)
	for _, copy := range m.Table("RECIPIENT").Referencing("message", msg["id"]) {
		parents = without(parents, copy["person"])
	}
	return parents
}

func dayAndHours(g store.Row) (string, string) {
	from, until, allDay, ok := mail.Span(g["start"], g["end"], School)
	if !ok {
		return g["timing"], ""
	}
	if allDay {
		last := until.AddDate(0, 0, -1)
		if last.After(from) {
			return from.Format("Monday, January 2") + " – " + last.Format("Monday, January 2, 2006"), ""
		}
		return from.Format("Monday, January 2, 2006"), ""
	}
	return from.Format("Monday, January 2, 2006"), from.Format("3:04 PM") + " – " + until.Format("3:04 PM")
}

func (iv *Invitations) calendarEvent(m *Model, g store.Row, person, token, link string) (mail.Event, bool) {
	from, until, allDay, ok := mail.Span(g["start"], g["end"], School)
	if !ok {
		return mail.Event{}, false
	}
	hosts := joinNames(m.namesOf(m.hostsOf(g)))
	if hosts == "" {
		hosts = whenBrand.Name
	}
	if original, ok := m.copiesOf(g["id"])[person]; ok {
		token = original["token"]
	}
	organizer := mail.AddressOf(iv.config.Sender.From())
	if token != "" {
		organizer = mail.AddressOf(iv.organizer(token))
	}
	return mail.Event{
		UID: g["id"] + whenUID, Start: from, End: until, AllDay: allDay, Summary: g["name"],
		Description: strings.TrimSpace(withoutMarkers(g["description"]) + "\n\n" + link), Location: g["location"], URL: link,
		Organizer: mail.Person{Name: hosts, Email: organizer},
		Attendees: []mail.Person{{Name: m.nameOf(person), Email: m.addressOf(person)}}, RSVP: true,
	}, true
}

func (iv *Invitations) words(ctx context.Context, m *Model, msg store.Row) (string, error) {
	if msg["content"] == "" {
		return "", nil
	}
	content, ok := m.Table("CONTENT").Get(msg["content"])
	if !ok {
		return "", fmt.Errorf("no content %s", msg["content"])
	}
	raw, _, err := iv.pics.bucket.Get(ctx, content["blob"])
	return string(raw), err
}

func (iv *Invitations) sendCopy(ctx context.Context, m *Model, g, msg, r store.Row) (map[string]any, error) {
	person := r["person"]
	address := m.addressOf(person)
	if address == "" {
		return map[string]any{"sent": stamp(time.Now()), "detail": "no address to send to"}, nil
	}
	words, err := iv.words(ctx, m, msg)
	if err != nil {
		return nil, err
	}
	link := iv.linkFor(m, g, person, r["token"])
	cc := m.addresses(m.studentCC(person, msg))
	hosts := m.hostsOf(g)
	replyTo := append(m.addresses(hosts), iv.organizer(r["token"]))
	from := msg["from_person"]
	if from == "" && len(hosts) > 0 {
		from = hosts[0]
	}
	var out mail.Message
	method := mail.MethodRequest
	switch {
	case msg["kind"] == "update" && g["status"] == "cancelled":
		out = iv.cancellation(m, g, from, person, words, link)
		method = mail.MethodCancel
	case msg["kind"] == "invitation" || msg["kind"] == "resend" || msg["kind"] == "update":
		if out, err = iv.invitation(m, g, msg["kind"], person, link, len(cc) == 0); err != nil {
			return nil, err
		}
	default:
		out = iv.note(m, g, msg, from, person, words, link)
	}
	out.To = []string{address}
	out.CC = cc
	out.ReplyTo = replyTo
	out.FromName = joinNames(m.namesOf(hosts))
	out.Headers = map[string]string{"Message-Id": mail.MessageID(r["id"], iv.config.Sender.From())}
	attach := len(cc) == 0 && msg["kind"] != "post" && msg["kind"] != "thanks"
	if e, ok := iv.calendarEvent(m, g, person, r["token"], link); ok && attach {
		out.Attachments = []mail.Attachment{mail.Calendar{Product: whenBrand.Name, Method: method, Stamp: time.Now(), Events: []mail.Event{e}}.Attachment()}
	}
	sender := iv.config.Sender.From()
	if err := iv.config.Sender.SendCopy(ctx, sender, append([]string{address}, cc...), []byte(mail.Compose(sender, out)), r["id"]); err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "invitations: mailed", "recipient", r["id"], "group", g["id"], "kind", msg["kind"])
	return map[string]any{"sent": stamp(time.Now())}, nil
}

func (m *Model) addresses(people []string) []string {
	out := []string{}
	for _, p := range people {
		if a := m.addressOf(p); a != "" && !slices.Contains(out, a) {
			out = append(out, a)
		}
	}
	return out
}

func (iv *Invitations) invitation(m *Model, g store.Row, kind, person, link string, attached bool) (mail.Message, error) {
	day, hours := dayAndHours(g)
	names := []string{}
	for _, p := range m.householdOnList(g, person) {
		names = append(names, m.shortName(p))
	}
	intro := "You're invited to"
	subject := "[" + g["name"] + "] You're invited!"
	if kind == "update" {
		intro = "The details have been updated for"
		subject = "[" + g["name"] + "] Updated: the details have changed"
	}
	if kind == "resend" {
		intro = "A reminder: you're invited to"
		subject = "[" + g["name"] + "] Reminder: you're invited!"
	}
	hosting := ""
	if by := cmp.Or(g["hosted_by"], joinNames(m.namesOf(m.hostsOf(g)))); by != "" {
		hosting = "Hosted by " + by
	}
	rsvpFor := "RSVP for " + joinNames(names) + " here"
	var text strings.Builder
	fmt.Fprintf(&text, "%s\n\n%s\n%s\n", intro, g["name"], day)
	if hours != "" {
		fmt.Fprintf(&text, "%s\n", hours)
	}
	description := withoutMarkers(g["description"])
	if description != "" {
		fmt.Fprintf(&text, "\n%s\n", description)
	}
	fmt.Fprintf(&text, "\n%s\nOpen the invitation: %s\n", rsvpFor, link)
	if g["location"] != "" {
		fmt.Fprintf(&text, "\n%s\n", g["location"])
	}
	if hosting != "" {
		fmt.Fprintf(&text, "%s\n", hosting)
	}
	fmt.Fprintf(&text, "\nInvited: %s\n", strings.Join(names, ", "))
	if attached {
		text.WriteString("\nThe invite attached puts it on your calendar.\n")
	}
	if g["theme"] != "" {
		h, err := iv.gatherLetter(m, g, kind, intro, day, hours, description, hosting, person, link, names, attached)
		return mail.Message{Subject: subject, Text: text.String(), HTML: h}, err
	}
	esc := html.EscapeString
	var h strings.Builder
	h.WriteString(`<div style="max-width:600px;margin:0 auto;padding:8px 0;font-family:-apple-system,Segoe UI,Roboto,sans-serif;color:#1b2a2c">`)
	h.WriteString(`<div style="background:#fff;border:1px solid #e6e6e6;border-radius:6px;padding:36px 32px 28px">`)
	fmt.Fprintf(&h, `<p style="margin:0 0 24px"><a href="%s" style="display:block"><img src="%s" width="536" alt="%s" style="display:block;width:100%%;height:auto;border:0;border-radius:4px"></a></p>`, esc(link), esc(iv.cardOf(g)), esc(g["name"]))
	fmt.Fprintf(&h, `<p style="margin:0 0 14px;font-size:16px;line-height:1.4;text-align:center">%s</p>`, esc(intro))
	fmt.Fprintf(&h, `<p style="margin:0 0 10px;font-size:26px;line-height:1.25;text-align:center">%s</p>`, esc(g["name"]))
	fmt.Fprintf(&h, `<p style="margin:0 0 28px;font-size:15px;text-align:center;color:#444">%s</p>`, esc(day))
	if description != "" {
		fmt.Fprintf(&h, `<p style="margin:0 0 24px;font-size:15px;line-height:1.55;color:#444;white-space:pre-wrap">%s</p>`, esc(description))
	}
	fmt.Fprintf(&h, `<p style="margin:0 0 12px;font-size:15px;font-weight:700;text-align:center"><a href="%s" style="color:#1b2a2c">%s</a></p>`, esc(link), esc(rsvpFor))
	fmt.Fprintf(&h, `<p style="margin:0 0 28px;text-align:center"><a href="%s" style="display:inline-block;padding:13px 26px;border-radius:4px;background:#1f4d53;color:#fff;font-size:14px;font-weight:600;letter-spacing:0.06em;text-decoration:none">OPEN INVITATION</a></p>`, esc(link))
	fmt.Fprintf(&h, `<p style="margin:0 0 6px;font-size:13px;font-style:italic;text-align:center;color:#777">This email is for %s. Please do not forward it.</p>`, esc(strings.Join(names, ", ")))
	h.WriteString(`<hr style="border:0;border-top:1px solid #e6e6e6;margin:22px 0"><div style="text-align:center;font-size:14px;line-height:1.7;color:#333">`)
	if hosting != "" {
		fmt.Fprintf(&h, `<p style="margin:0;font-weight:700">%s</p>`, esc(hosting))
	}
	if g["location"] != "" {
		maps := "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(g["location"])
		fmt.Fprintf(&h, `<p style="margin:0"><a href="%s" style="color:#1a73e8">%s</a> <a href="%s" style="color:#2f9e6a;text-decoration:none">(View Map)</a></p>`, esc(maps), esc(g["location"]), esc(maps))
	}
	fmt.Fprintf(&h, `<p style="margin:0">%s</p>`, esc(strings.TrimSpace(day+" "+hours)))
	if e, ok := iv.calendarEvent(m, g, person, "", link); ok {
		fmt.Fprintf(&h, `<p style="margin:10px 0 0"><a href="%s" style="color:#2f9e6a;text-decoration:none;margin:0 8px">Add to Google</a> <a href="%s" style="color:#2f9e6a;text-decoration:none;margin:0 8px">RSVP</a></p>`, esc(e.GoogleLink()), esc(link))
	}
	if attached {
		h.WriteString(`<p style="margin:12px 0 0;font-size:12px;color:#777">The invite attached puts it on your calendar.</p>`)
	}
	h.WriteString(`</div></div><p style="margin:14px 0 0;font-size:11px;letter-spacing:0.08em;text-align:center;color:#999">SENT WITH HELIOS WHEN</p></div>`)
	return mail.Message{Subject: subject, Text: text.String(), HTML: h.String()}, nil
}

func roleColor(card map[string]any, role string) (string, error) {
	switch c := designValue(card, "text."+role+".color"); c {
	case "ink", "accent":
		return designHex(card, "card."+c)
	default:
		return designHex(card, "text."+role+".color")
	}
}

func (iv *Invitations) gatherLetter(m *Model, g store.Row, kind, intro, day, hours, description, hosting, person, link string, names []string, attached bool) (string, error) {
	card, err := cardDesign(m, lookOf(g))
	if err != nil {
		return "", err
	}
	colors := map[string]string{}
	for name, path := range map[string]string{"backdrop": "page.backdrop", "pageInk": "page.ink", "muted": "page.muted", "paper": "card.background", "ink": "card.ink", "accent": "card.accent", "accentInk": "card.accentInk"} {
		if colors[name], err = designHex(card, path); err != nil {
			return "", err
		}
	}
	if colors["kicker"], err = roleColor(card, "kicker"); err != nil {
		return "", err
	}
	display := designValue(card, "fonts.display")
	body := designValue(card, "fonts.body")
	families := []string{}
	loads, _ := designAt(card, "fonts.load").([]any)
	for _, f := range loads {
		if s, ok := f.(string); ok {
			families = append(families, "family="+strings.ReplaceAll(s, " ", "+"))
		}
	}
	radius := designValue(card, "card.radius")
	esc := html.EscapeString
	var h strings.Builder
	if len(families) > 0 {
		fmt.Fprintf(&h, `<link rel="stylesheet" href="%s">`, esc("https://fonts.googleapis.com/css2?"+strings.Join(families, "&")+"&display=swap"))
	}
	fmt.Fprintf(&h, `<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" bgcolor="%s" style="background:%s;font-family:%s"><tr><td align="center" style="padding:36px 14px 28px">`, colors["backdrop"], colors["backdrop"], esc(body))
	h.WriteString(`<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:560px"><tr><td>`)
	if kind != "invitation" {
		fmt.Fprintf(&h, `<p style="margin:0 0 18px;font-size:15px;line-height:1.4;text-align:center;color:%s">%s</p>`, colors["pageInk"], esc(strings.TrimSuffix(intro, " to")))
	}
	fmt.Fprintf(&h, `<a href="%s" style="display:block;text-decoration:none"><img src="%s" width="560" alt="%s" style="display:block;width:100%%;height:auto;border:0;border-radius:%s"></a>`, esc(link), esc(iv.cardOf(g)), esc(g["name"]), esc(radius))
	fmt.Fprintf(&h, `<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" bgcolor="%s" style="margin-top:20px;background:%s;border-radius:%s;color:%s"><tr><td align="center" style="padding:40px 32px 34px;text-align:center">`, colors["paper"], colors["paper"], esc(radius), colors["ink"])
	fmt.Fprintf(&h, `<p style="margin:0 0 14px;font-size:13px;line-height:1.4;letter-spacing:0.2em;text-transform:uppercase;color:%s">%s</p>`, colors["kicker"], esc(designValue(card, "text.kicker.words")))
	fmt.Fprintf(&h, `<p style="margin:0 0 20px;font-family:%s;font-size:40px;line-height:1.1;color:%s">%s</p>`, esc(display), colors["ink"], esc(g["name"]))
	fmt.Fprintf(&h, `<div style="width:56px;margin:0 auto 20px;border-top:2px solid %s"></div>`, colors["accent"])
	fmt.Fprintf(&h, `<p style="margin:0;font-size:19px;line-height:1.4;font-weight:600">%s</p>`, esc(day))
	if hours != "" {
		fmt.Fprintf(&h, `<p style="margin:2px 0 0;font-size:17px;line-height:1.4">%s</p>`, esc(hours))
	}
	if g["location"] != "" {
		maps := "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(g["location"])
		fmt.Fprintf(&h, `<p style="margin:10px 0 0;font-size:16px;line-height:1.4;font-style:italic"><a href="%s" style="color:%s;text-decoration:underline;text-decoration-color:%s">%s</a></p>`, esc(maps), colors["ink"], colors["accent"], esc(g["location"]))
	}
	if description != "" {
		fmt.Fprintf(&h, `<p style="margin:26px 0 0;font-size:16px;line-height:1.6;white-space:pre-wrap">%s</p>`, esc(description))
	}
	if hosting != "" {
		fmt.Fprintf(&h, `<p style="margin:26px 0 0;font-size:12px;line-height:1.5;letter-spacing:0.16em;text-transform:uppercase;color:%s">%s</p>`, colors["kicker"], esc(hosting))
	}
	fmt.Fprintf(&h, `<p style="margin:30px 0 0"><a href="%s" style="display:inline-block;padding:15px 34px;border-radius:999px;background:%s;color:%s;font-size:16px;font-weight:700;letter-spacing:0.02em;text-decoration:none">Open your invitation</a></p>`, esc(link), colors["accent"], colors["accentInk"])
	fmt.Fprintf(&h, `<p style="margin:12px 0 0;font-size:14px;line-height:1.4">RSVP for %s</p>`, esc(joinNames(names)))
	if e, ok := iv.calendarEvent(m, g, person, "", link); ok {
		fmt.Fprintf(&h, `<p style="margin:18px 0 0"><a href="%s" style="display:inline-block;padding:10px 24px;border:2px solid %s;border-radius:999px;color:%s;font-size:14px;font-weight:700;text-decoration:none">Add to Google Calendar</a></p>`, esc(e.GoogleLink()), colors["accent"], colors["accent"])
	}
	h.WriteString(`</td></tr></table>`)
	fmt.Fprintf(&h, `<p style="margin:22px 0 0;font-size:13px;line-height:1.5;text-align:center;color:%s">This invitation is for %s. Please don’t forward it.`, colors["pageInk"], esc(joinNames(names)))
	if attached {
		h.WriteString(`<br>The invite attached puts it on your calendar.`)
	}
	h.WriteString(`</p>`)
	gather := iv.config.Origin("gather")
	fmt.Fprintf(&h, `<p style="margin:22px 0 0;text-align:center"><a href="%s" style="text-decoration:none;color:%s;font-size:11px;letter-spacing:0.14em"><img src="%s" width="22" height="22" alt="" style="display:inline-block;vertical-align:middle;border:0;border-radius:5px;margin-right:8px">SENT WITH HELIOS GATHER</a></p>`, esc(gather), colors["muted"], esc(gather+"/brand/icon-192.png"))
	h.WriteString(`</td></tr></table></td></tr></table>`)
	return h.String(), nil
}

func (iv *Invitations) letter(g store.Row, link string) mail.Letter {
	day, hours := dayAndHours(g)
	return mail.Letter{Brand: whenBrand, Base: iv.config.Origin("when"), Title: g["name"], When: strings.TrimSpace(day + " " + hours), Where: g["location"], Path: link, Picture: iv.cardOf(g)}
}

func (iv *Invitations) cardOf(g store.Row) string {
	if g["theme"] != "" {
		return iv.config.Origin("gather") + "/open/envelope/" + g["id"] + ".png"
	}
	return iv.config.Origin("when") + "/open/share/" + g["id"] + ".png"
}

func (iv *Invitations) note(m *Model, g, msg store.Row, from, person, words, link string) mail.Message {
	l := iv.letter(g, link)
	l.Heading = msg["subject"]
	l.Intro = fmt.Sprintf("A message from %s about %s:", m.nameOf(from), g["name"])
	l.Note = words
	waiting := false
	for _, p := range m.householdOnList(g, person) {
		answer := m.answerOf(g, p)
		word := map[string]string{"yes": "Yes", "no": "No"}[answer]
		if word == "" {
			word = "No response yet"
			waiting = true
		}
		name := m.nameOf(p)
		if p == person {
			name = "You"
		}
		l.Rows = append(l.Rows, [2]string{name, word})
	}
	l.Button = "Open the event"
	if waiting {
		l.Button = "RSVP now"
		l.Footnote = "Someone in your household has not answered yet - the hosts would love to know."
	}
	return l.Message("["+g["name"]+"] "+msg["subject"], nil, nil, nil)
}

func (iv *Invitations) cancellation(m *Model, g store.Row, from, person, words, link string) mail.Message {
	l := iv.letter(g, link)
	l.Heading = g["name"] + " is cancelled"
	l.Intro = fmt.Sprintf("%s called off %s.", m.nameOf(from), g["name"])
	l.Note = words
	l.Button = "See the event"
	return l.Message("["+g["name"]+"] Cancelled", nil, nil, nil)
}

func (iv *Invitations) react(ctx context.Context, d delivery) {
	c, env := d.change, d.env
	if env.Mode == "import" || c.New == nil {
		return
	}
	m := iv.s.Model()
	switch c.Table {
	case "MEMBER":
		if memberAs(c.New) != "yes" || (c.Old != nil && memberAs(c.Old) == "yes") {
			return
		}
		person := c.New["person"]
		for _, a := range answerColumns {
			for _, g := range m.Table("GROUP").Referencing(a.column, c.New["group"]) {
				if !slices.Contains(guestListKinds, g["kind"]) {
					continue
				}
				iv.tellHosts(ctx, m, g, person, a.word, env.Viewer)
				if a.word == "yes" && env.System == "" && !m.invitedWithCalendar(g, person) {
					iv.calendarInvite(ctx, m, g, person)
				}
			}
		}
		if env.System != "" || env.Viewer == person {
			return
		}
		for _, g := range m.Table("GROUP").Referencing("managed_by", c.New["group"]) {
			if g["id"] != c.New["group"] && slices.Contains(guestListKinds, g["kind"]) {
				iv.cohost(ctx, m, g, person, env.Viewer)
			}
		}
	case "GROUP":
		if c.Old == nil && c.New["kind"] == "event" && env.System == "" {
			iv.tellAdmins(ctx, m, c.New, env.Viewer)
		}
	case "COMMENT":
		if c.Old == nil {
			iv.passNote(ctx, m, c.New)
		}
	}
}

func (iv *Invitations) passNote(ctx context.Context, m *Model, note store.Row) {
	g, ok := m.Table("GROUP").Get(note["group"])
	if !ok {
		return
	}
	writer := m.nameOf(note["person"])
	if note["reply_to"] == "" {
		to := without(m.hostsOf(g), note["person"])
		if len(to) == 0 {
			return
		}
		l := iv.letter(g, iv.linkFor(m, g, to[0], ""))
		l.Heading = writer + " wrote to the hosts"
		l.Intro = fmt.Sprintf("%s wrote about %s:", writer, g["name"])
		l.Note = note["text"]
		l.Button = "Reply on the event's page"
		iv.sendNow(ctx, m, g, "notice", to, l.Message(fmt.Sprintf("[%s] A note from %s", g["name"], writer), nil, nil, nil))
		return
	}
	asked, ok := m.Table("COMMENT").Get(note["reply_to"])
	if !ok || asked["person"] == note["person"] {
		return
	}
	guest := asked["person"]
	l := iv.letter(g, iv.linkFor(m, g, guest, m.copiesOf(g["id"])[guest]["token"]))
	l.Heading = writer + " replied"
	l.Intro = fmt.Sprintf("%s replied to your note about %s, “%s”:", writer, g["name"], asked["text"])
	l.Note = note["text"]
	l.Button = "Open the invitation"
	iv.sendNow(ctx, m, g, "notice", []string{guest}, l.Message(fmt.Sprintf("[%s] %s replied", g["name"], writer), nil, nil, nil))
}

func (m *Model) invitedWithCalendar(g store.Row, person string) bool {
	r, ok := m.copiesOf(g["id"])[person]
	msg, _ := m.Table("MESSAGE").Get(r["message"])
	return ok && r["sent"] != "" && len(m.addresses(m.studentCC(person, msg))) == 0
}

func (iv *Invitations) sendNow(ctx context.Context, m *Model, g store.Row, kind string, to []string, msg mail.Message) {
	msg.To = m.addresses(to)
	if len(msg.To) == 0 {
		return
	}
	log := slog.With("group", g["id"], "kind", kind, "subject", msg.Subject)
	if err := m.sendShared(ctx, iv.s, iv.config.Sender, invitationsJob, g["id"], kind, to, msg); err != nil {
		log.Error("invitations: mail not sent", "error", err)
		return
	}
	log.Info("invitations: mailed now", "to", len(msg.To))
}

func (iv *Invitations) tellHosts(ctx context.Context, m *Model, g store.Row, person, answer, by string) {
	to := []string{}
	for _, h := range m.hostsOf(g) {
		if h != by && notifies(m, h, g["id"]) {
			to = append(to, h)
		}
	}
	if len(to) == 0 {
		return
	}
	yes := 0
	if g["rsvp_yes"] != "" {
		for _, row := range m.Table("MEMBER").Referencing("group", g["rsvp_yes"]) {
			if memberAs(row) == "yes" {
				yes++
			}
		}
	}
	words := map[string]string{"yes": "is coming", "maybe": "might come", "no": "can't make it"}[answer]
	l := iv.letter(g, iv.linkFor(m, g, to[0], ""))
	l.Heading = fmt.Sprintf("%s %s", m.nameOf(person), words)
	l.Intro = fmt.Sprintf("%s answered %s for %s.", m.nameOf(person), strings.ToUpper(answer[:1])+answer[1:], g["name"])
	if by != "" && by != person {
		l.Intro = fmt.Sprintf("%s answered %s for %s on %s.", m.nameOf(by), strings.ToUpper(answer[:1])+answer[1:], m.nameOf(person), g["name"])
	}
	l.Rows = [][2]string{{"Coming so far", fmt.Sprint(yes)}}
	l.Button = "See the guest list"
	subject := fmt.Sprintf("[%s] %s: %s", g["name"], m.nameOf(person), strings.ToUpper(answer[:1])+answer[1:])
	iv.sendNow(ctx, m, g, "notice", to, l.Message(subject, nil, nil, nil))
}

func (iv *Invitations) calendarInvite(ctx context.Context, m *Model, g store.Row, person string) {
	token := ""
	if original, ok := m.copiesOf(g["id"])[person]; ok {
		token = original["token"]
	}
	link := iv.linkFor(m, g, person, token)
	e, ok := iv.calendarEvent(m, g, person, "", link)
	if !ok {
		return
	}
	e.RSVP = false
	l := iv.letter(g, link)
	l.Heading = "It's on your calendar"
	l.Intro = fmt.Sprintf("Here's the calendar invite for %s - accept it and it's on your calendar.", g["name"])
	l.Button = "See the event"
	l.Calendar = e.GoogleLink()
	subject := "Calendar invite: " + g["name"]
	msg := l.Message(subject, nil, nil, nil)
	msg.Attachments = []mail.Attachment{mail.Calendar{Product: whenBrand.Name, Method: mail.MethodRequest, Stamp: time.Now(), Events: []mail.Event{e}}.Attachment()}
	iv.sendNow(ctx, m, g, "calendar", []string{person}, msg)
}

func (iv *Invitations) cohost(ctx context.Context, m *Model, g store.Row, person, by string) {
	l := iv.letter(g, iv.linkFor(m, g, person, ""))
	l.Heading = "You're a co-host"
	l.Intro = fmt.Sprintf("%s made you a co-host of %s. As a co-host you build and send the guest list, read every answer, message the guests, and replies to the invitation reach you.", m.nameOf(by), g["name"])
	l.Button = "Open the event"
	subject := "[" + g["name"] + "] You're a co-host"
	msg := l.Message(subject, nil, nil, m.addresses([]string{by}))
	iv.sendNow(ctx, m, g, "notice", []string{person}, msg)
}

func (iv *Invitations) tellAdmins(ctx context.Context, m *Model, g store.Row, by string) {
	admins := []string{}
	for _, app := range m.Table("APP").All() {
		if app["key"] == "when" {
			admins = without(m.membersOf(app["admins"]), by)
		}
	}
	if len(admins) == 0 {
		return
	}
	link := iv.linkFor(m, g, admins[0], "")
	l := iv.letter(g, link)
	subject := "Event added: " + g["name"]
	l.Heading = g["name"]
	l.Intro = fmt.Sprintf("%s added %s to the calendar.", m.nameOf(by), g["name"])
	l.Button = "See the event"
	if g["status"] == "pending" {
		subject = "Event to approve: " + g["name"]
		l.Intro = fmt.Sprintf("%s shared %s, and it waits for an admin's approval before it goes on the calendar.", m.nameOf(by), g["name"])
		l.Button = "Review the event"
	}
	iv.sendNow(ctx, m, g, "notice", admins, l.Message(subject, nil, nil, nil))
}
