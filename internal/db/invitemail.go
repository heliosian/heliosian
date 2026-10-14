package db

import (
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

var queuedKinds = []string{"invitation", "reminder", "update", "post"}

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
		if _, err := iv.write(ctx, access.System(invitationsJob), edits); err != nil {
			slog.ErrorContext(ctx, "invitations: auto-invite not recorded", "group", g["id"], "error", err)
			continue
		}
		slog.InfoContext(ctx, "invitations: newcomers invited", "group", g["id"], "people", len(pending))
	}
}

func (iv *Invitations) sendQueued(ctx context.Context) {
	m := iv.s.Model()
	now := time.Now()
	edits := []Edit{}
	for _, msg := range m.Table("MESSAGE").All() {
		if msg["direction"] != "out" || !slices.Contains(queuedKinds, msg["kind"]) || (msg["kind"] == "post" && msg["parent"] != "") {
			continue
		}
		g, ok := m.Table("GROUP").Get(msg["group"])
		if !ok || !slices.Contains(guestListKinds, g["kind"]) {
			continue
		}
		for _, r := range m.Table("RECIPIENT").Referencing("message", msg["id"]) {
			if r["sent"] != "" || r["failed"] != "" {
				continue
			}
			if m.ended(g, now) && g["status"] != "cancelled" {
				edits = append(edits, Edit{Set: r["id"], Cells: map[string]any{"failed": stamp(now), "detail": "the event had passed before it went"}})
				continue
			}
			cells, err := iv.sendCopy(ctx, m, g, msg, r)
			if err != nil {
				slog.ErrorContext(ctx, "invitations: send failed", "recipient", r["id"], "group", g["id"], "kind", msg["kind"], "error", err)
				continue
			}
			edits = append(edits, Edit{Set: r["id"], Cells: cells})
		}
	}
	if _, err := iv.write(ctx, access.System(invitationsJob), edits); err != nil {
		slog.ErrorContext(ctx, "invitations: sends not recorded", "copies", len(edits), "error", err)
	}
}

func (iv *Invitations) organizer(token string) string {
	at := strings.LastIndex(iv.config.ReplyTo, "@")
	return iv.config.ReplyTo[:at] + "+" + hex.EncodeToString([]byte(token)) + iv.config.ReplyTo[at:]
}

func (iv *Invitations) linkFor(m *Model, g store.Row, person, token string) string {
	if p, ok := m.Table("PERSON").Get(person); ok && p["source"] == "guest" {
		return iv.config.Origin("when") + "/ext/" + token
	}
	key := g["slug"]
	if key == "" {
		key = g["id"]
	}
	return iv.config.Origin("when") + "/e/" + url.PathEscape(key)
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

func (m *Model) shortName(person string) string {
	if first := m.firstName(person); first != "" {
		return first
	}
	return strings.Fields(m.nameOf(person) + " ")[0]
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

func (m *Model) studentCC(person string) []string {
	if !m.students()[person] {
		return nil
	}
	return without(m.withParents(person), person)
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
	return mail.Event{
		UID: g["id"] + whenUID, Start: from, End: until, AllDay: allDay, Summary: g["name"],
		Description: strings.TrimSpace(g["description"] + "\n\n" + link), Location: g["location"], URL: link,
		Organizer: mail.Person{Name: hosts, Email: mail.AddressOf(iv.organizer(token))},
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
	cc := m.addresses(m.studentCC(person))
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
	case msg["kind"] == "invitation" || msg["kind"] == "update":
		out = iv.invitation(m, g, msg["kind"], from, person, link, len(cc) == 0)
	default:
		out = iv.note(m, g, msg, from, person, words, link)
	}
	out.To = []string{address}
	out.CC = cc
	out.ReplyTo = replyTo
	out.FromName = joinNames(m.namesOf(hosts))
	out.Headers = map[string]string{"Message-Id": mail.MessageID(r["id"], iv.config.Sender.From())}
	attach := len(cc) == 0 && msg["kind"] != "post"
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

func (iv *Invitations) invitation(m *Model, g store.Row, kind, from, person, link string, attached bool) mail.Message {
	day, hours := dayAndHours(g)
	names := []string{}
	for _, p := range m.householdOnList(g, person) {
		names = append(names, m.shortName(p))
	}
	sender := m.nameOf(from)
	sentBy := sender + " sent " + names[0] + " an invitation for"
	subject := "[" + g["name"] + "] You're invited!"
	if kind == "update" {
		sentBy = sender + " has updated the details of"
		subject = "[" + g["name"] + "] Updated: the details have changed"
	}
	hosting := "Hosted by " + joinNames(m.namesOf(m.hostsOf(g)))
	rsvpFor := "RSVP for " + joinNames(names) + " here"
	var text strings.Builder
	fmt.Fprintf(&text, "%s\n\n%s\n%s\n", sentBy, g["name"], day)
	if hours != "" {
		fmt.Fprintf(&text, "%s\n", hours)
	}
	if g["description"] != "" {
		fmt.Fprintf(&text, "\n%s\n", g["description"])
	}
	fmt.Fprintf(&text, "\n%s\nOpen the invitation: %s\n", rsvpFor, link)
	if g["location"] != "" {
		fmt.Fprintf(&text, "\n%s\n", g["location"])
	}
	fmt.Fprintf(&text, "%s\n\nInvited: %s\n", hosting, strings.Join(names, ", "))
	if attached {
		text.WriteString("\nThe invite attached puts it on your calendar.\n")
	}
	esc := html.EscapeString
	var h strings.Builder
	h.WriteString(`<div style="max-width:600px;margin:0 auto;padding:8px 0;font-family:-apple-system,Segoe UI,Roboto,sans-serif;color:#1b2a2c">`)
	h.WriteString(`<div style="background:#fff;border:1px solid #e6e6e6;border-radius:6px;padding:36px 32px 28px">`)
	fmt.Fprintf(&h, `<p style="margin:0 0 24px"><a href="%s" style="display:block"><img src="%s" width="536" alt="%s" style="display:block;width:100%%;height:auto;border:0;border-radius:4px"></a></p>`, esc(link), esc(iv.cardOf(g)), esc(g["name"]))
	fmt.Fprintf(&h, `<p style="margin:0 0 14px;font-size:16px;line-height:1.4;text-align:center">%s</p>`, esc(sentBy))
	fmt.Fprintf(&h, `<p style="margin:0 0 10px;font-size:26px;line-height:1.25;text-align:center">%s</p>`, esc(g["name"]))
	fmt.Fprintf(&h, `<p style="margin:0 0 28px;font-size:15px;text-align:center;color:#444">%s</p>`, esc(day))
	if g["description"] != "" {
		fmt.Fprintf(&h, `<p style="margin:0 0 24px;font-size:15px;line-height:1.55;color:#444;white-space:pre-wrap">%s</p>`, esc(g["description"]))
	}
	fmt.Fprintf(&h, `<p style="margin:0 0 12px;font-size:15px;font-weight:700;text-align:center"><a href="%s" style="color:#1b2a2c">%s</a></p>`, esc(link), esc(rsvpFor))
	fmt.Fprintf(&h, `<p style="margin:0 0 28px;text-align:center"><a href="%s" style="display:inline-block;padding:13px 26px;border-radius:4px;background:#1f4d53;color:#fff;font-size:14px;font-weight:600;letter-spacing:0.06em;text-decoration:none">OPEN INVITATION</a></p>`, esc(link))
	fmt.Fprintf(&h, `<p style="margin:0 0 6px;font-size:13px;font-style:italic;text-align:center;color:#777">This email is for %s. Please do not forward it.</p>`, esc(strings.Join(names, ", ")))
	h.WriteString(`<hr style="border:0;border-top:1px solid #e6e6e6;margin:22px 0"><div style="text-align:center;font-size:14px;line-height:1.7;color:#333">`)
	fmt.Fprintf(&h, `<p style="margin:0;font-weight:700">%s</p>`, esc(hosting))
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
	return mail.Message{Subject: subject, Text: text.String(), HTML: h.String()}
}

func (iv *Invitations) letter(g store.Row, link string) mail.Letter {
	day, hours := dayAndHours(g)
	return mail.Letter{Brand: whenBrand, Base: iv.config.Origin("when"), Title: g["name"], When: strings.TrimSpace(day + " " + hours), Where: g["location"], Path: link, Picture: iv.cardOf(g)}
}

func (iv *Invitations) cardOf(g store.Row) string {
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
		for _, a := range []struct{ column, word string }{{"rsvp_yes", "yes"}, {"rsvp_no", "no"}} {
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
	}
}

func (m *Model) invitedWithCalendar(g store.Row, person string) bool {
	r, ok := m.copiesOf(g["id"])[person]
	return ok && r["sent"] != "" && len(m.studentCC(person)) == 0
}

func (iv *Invitations) sendNow(ctx context.Context, m *Model, g store.Row, kind, subject string, to []string, msg mail.Message) {
	msg.To = m.addresses(to)
	if len(msg.To) == 0 {
		return
	}
	log := slog.With("group", g["id"], "kind", kind, "subject", subject)
	edits := m.copyEdits(g["id"], kind, subject, to)
	ids, err := iv.write(ctx, access.System(invitationsJob), edits)
	if err != nil {
		log.Error("invitations: mail not recorded", "error", err)
		return
	}
	sent := sendCopies(ctx, iv.config.Sender, composeShared(msg, iv.config.Sender.From(), ids[0]), m.outCopies(edits, ids))
	if _, err := iv.write(ctx, access.System(invitationsJob), sent); err != nil {
		log.Error("invitations: sends not recorded", "error", err)
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
	words := map[string]string{"yes": "is coming", "no": "can't make it"}[answer]
	l := iv.letter(g, iv.linkFor(m, g, to[0], ""))
	l.Heading = fmt.Sprintf("%s %s", m.nameOf(person), words)
	l.Intro = fmt.Sprintf("%s answered %s for %s.", m.nameOf(person), strings.ToUpper(answer[:1])+answer[1:], g["name"])
	if by != "" && by != person {
		l.Intro = fmt.Sprintf("%s answered %s for %s on %s.", m.nameOf(by), strings.ToUpper(answer[:1])+answer[1:], m.nameOf(person), g["name"])
	}
	l.Rows = [][2]string{{"Coming so far", fmt.Sprint(yes)}}
	l.Button = "See the guest list"
	subject := fmt.Sprintf("[%s] %s: %s", g["name"], m.nameOf(person), strings.ToUpper(answer[:1])+answer[1:])
	iv.sendNow(ctx, m, g, "notice", subject, to, l.Message(subject, nil, nil, nil))
}

func (iv *Invitations) calendarInvite(ctx context.Context, m *Model, g store.Row, person string) {
	link := iv.linkFor(m, g, person, "")
	e, ok := iv.calendarEvent(m, g, person, "", link)
	if !ok {
		return
	}
	e.Organizer = mail.Person{Name: whenBrand.Name, Email: mail.AddressOf(iv.config.Sender.From())}
	e.RSVP = false
	l := iv.letter(g, link)
	l.Heading = "It's on your calendar"
	l.Intro = fmt.Sprintf("Here's the calendar invite for %s - accept it and it's on your calendar.", g["name"])
	l.Button = "See the event"
	l.Calendar = e.GoogleLink()
	subject := "Calendar invite: " + g["name"]
	msg := l.Message(subject, nil, nil, nil)
	msg.Attachments = []mail.Attachment{mail.Calendar{Product: whenBrand.Name, Method: mail.MethodRequest, Stamp: time.Now(), Events: []mail.Event{e}}.Attachment()}
	iv.sendNow(ctx, m, g, "calendar", subject, []string{person}, msg)
}

func (iv *Invitations) cohost(ctx context.Context, m *Model, g store.Row, person, by string) {
	l := iv.letter(g, iv.linkFor(m, g, person, ""))
	l.Heading = "You're a co-host"
	l.Intro = fmt.Sprintf("%s made you a co-host of %s. As a co-host you build and send the guest list, read every answer, message the guests, and replies to the invitation reach you.", m.nameOf(by), g["name"])
	l.Button = "Open the event"
	subject := "[" + g["name"] + "] You're a co-host"
	msg := l.Message(subject, nil, nil, m.addresses([]string{by}))
	iv.sendNow(ctx, m, g, "notice", subject, []string{person}, msg)
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
	iv.sendNow(ctx, m, g, "notice", subject, admins, l.Message(subject, nil, nil, nil))
}
