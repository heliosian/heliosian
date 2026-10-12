package db

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

type MembershipMailConfig struct {
	Sender *mail.Mailgun
	Origin func(app string) string
}

type MembershipMailer struct {
	s      *Store
	queue  *store.Queue
	config MembershipMailConfig
	work   chan placeChange
}

type placeChange struct {
	what, group, person, actor, note string
	lead                             bool
}

const (
	placeJoined = "joined"
	placeLeft   = "left"
	placeLeads  = "leads"
	placeAdded  = "added"
)

var teamBrand = mail.Brand{Name: "HCA-Team", Color: "#1f4d53", Tagline: "the HCA volunteer portal"}

var heliosianBrand = mail.Brand{Name: "Heliosian", Color: "#1f4d53", Tagline: "the Helios community apps"}

func NewMembershipMailer(s *Store, queue *store.Queue, config MembershipMailConfig) *MembershipMailer {
	mm := &MembershipMailer{s: s, queue: queue, config: config, work: make(chan placeChange, 256)}
	watch(s, mm.watch)
	go mm.run()
	return mm
}

func yes(cell string) bool {
	b, _ := cells.YesNo(cell, false)
	return b
}

func (mm *MembershipMailer) watch(tx *store.Tx, c Change, env Env) {
	if env.System != "" || env.Mode == "import" {
		return
	}
	changes := []placeChange{}
	switch c.Table {
	case "MEMBER":
		row := c.New
		if row == nil {
			row = c.Old
		}
		was := c.Old != nil && memberAs(c.Old) == "yes"
		is := c.New != nil && memberAs(c.New) == "yes"
		leads := c.New != nil && yes(c.New["lead"]) && (c.Old == nil || !yes(c.Old["lead"]))
		place := placeChange{group: row["group"], person: row["person"], actor: env.Viewer, note: row["note"], lead: leads}
		switch {
		case is && !was:
			place.what = placeJoined
			changes = append(changes, place)
		case was && !is:
			place.what = placeLeft
			changes = append(changes, place)
		case leads:
			place.what = placeLeads
			changes = append(changes, place)
		}
	case "GROUP":
		if c.Old == nil && c.New["parent"] != "" {
			changes = append(changes, placeChange{what: placeAdded, group: c.New["id"], actor: env.Viewer})
		}
	}
	if len(changes) == 0 {
		return
	}
	tx.After(func() {
		for _, change := range changes {
			mm.work <- change
		}
	})
}

func (mm *MembershipMailer) run() {
	for change := range mm.work {
		mm.mail(context.Background(), change)
	}
}

func (mm *MembershipMailer) mail(ctx context.Context, change placeChange) {
	m := mm.s.Model()
	g, ok := m.Table("GROUP").Get(change.group)
	if !ok {
		return
	}
	activity := g["kind"] == "activity"
	switch change.what {
	case placeJoined:
		if served, ok := m.offersFor(g); ok {
			mm.notice(ctx, m, served, change.actor, change.person, fmt.Sprintf("Offer to co-chair: %s for %s", m.nameOf(change.person), served["name"]),
				fmt.Sprintf("%s offered to co-chair %s", m.nameOf(change.person), served["name"]), "Its co-chairs can make them one.", m.signUpRows(change))
			return
		}
		if activity && change.lead {
			mm.leads(ctx, m, g, change)
		} else if activity {
			mm.thank(ctx, m, g, change)
		}
		mm.notice(ctx, m, g, change.actor, change.person, fmt.Sprintf("New sign-up: %s for %s", m.nameOf(change.person), g["name"]),
			fmt.Sprintf("%s signed up for %s", m.nameOf(change.person), g["name"]), "A new sign-up.", m.signUpRows(change))
	case placeLeft:
		if activity {
			mm.removed(ctx, m, g, change)
		}
	case placeLeads:
		if activity {
			mm.leads(ctx, m, g, change)
		}
	case placeAdded:
		parent, ok := m.Table("GROUP").Get(g["parent"])
		if !ok || m.serves(g) {
			return
		}
		rows := [][2]string{{"Status", g["status"]}}
		if g["status"] == "pending" {
			rows = append(rows, [2]string{"Needs", "Approval from those who run " + parent["name"]})
		}
		mm.notice(ctx, m, parent, change.actor, "", "New: "+g["name"], g["name"],
			fmt.Sprintf("Added under %s by %s.", parent["name"], m.nameOf(change.actor)), rows)
	}
}

func (m *Model) serves(g store.Row) bool {
	for _, column := range servingColumns {
		for _, other := range m.Table("GROUP").Referencing(column, g["id"]) {
			if other["id"] != g["id"] {
				return true
			}
		}
	}
	return false
}

func (m *Model) offersFor(g store.Row) (store.Row, bool) {
	served := m.Table("GROUP").Referencing("chair_offers", g["id"])
	if len(served) == 0 {
		return nil, false
	}
	return served[0], true
}

func (m *Model) nameOf(person string) string {
	if p, ok := m.Table("PERSON").Get(person); ok && p["name_show"] != "" {
		return p["name_show"]
	}
	return "Someone"
}

func (m *Model) firstName(person string) string {
	if p, ok := m.Table("PERSON").Get(person); ok && p["name_short"] != "" {
		return p["name_short"]
	}
	return ""
}

func (m *Model) signUpRows(change placeChange) [][2]string {
	rows := [][2]string{{"Who", m.nameOf(change.person)}}
	if change.note != "" {
		rows = append(rows, [2]string{"Note", change.note})
	}
	if change.actor != "" && change.actor != change.person {
		rows = append(rows, [2]string{"Signed up by", m.nameOf(change.actor)})
	}
	return rows
}

func (m *Model) chain(g store.Row) []store.Row {
	out := []store.Row{}
	for row, ok := g, true; ok; row, ok = m.Table("GROUP").Get(row["parent"]) {
		out = append(out, row)
		if row["parent"] == "" {
			break
		}
	}
	return out
}

func (m *Model) activities(g store.Row) []store.Row {
	out := []store.Row{}
	for _, row := range m.chain(g) {
		if row["kind"] == "activity" {
			out = append(out, row)
		}
	}
	if len(out) > 1 {
		out = out[:len(out)-1]
	}
	return out
}

func (m *Model) membersOf(group string) []string {
	out := []string{}
	if group == "" {
		return out
	}
	for _, row := range m.effectiveRows(group) {
		out = append(out, row["person"])
	}
	return out
}

func (m *Model) managersAbove(g store.Row) []string {
	out := []string{}
	for _, row := range m.activities(g) {
		for _, p := range m.membersOf(row["managed_by"]) {
			if !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	return out
}

func (m *Model) notified(g store.Row) []string {
	out := []string{}
	for _, row := range m.chain(g) {
		for _, p := range m.membersOf(row["notify"]) {
			if !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	return out
}

func without(people []string, drop ...string) []string {
	return slices.DeleteFunc(slices.Clone(people), func(p string) bool { return slices.Contains(drop, p) })
}

func (m *Model) withParents(person string) []string {
	return append([]string{person}, without(slices.Sorted(maps.Keys(m.replaceWith(map[string]bool{person: true}, "parents"))), person)...)
}

func (m *Model) chairRows(g store.Row) [][2]string {
	levels := m.activities(g)
	rows := [][2]string{}
	for i, level := range levels {
		names := []string{}
		for _, p := range m.membersOf(level["managed_by"]) {
			names = append(names, m.nameOf(p))
		}
		if len(names) == 0 {
			continue
		}
		label := level["name"] + " Leads"
		switch {
		case len(levels) == 1:
			label = "Chairs"
		case i == len(levels)-1:
			label = "Event Chairs"
		}
		rows = append(rows, [2]string{label, strings.Join(names, ", ")})
	}
	return rows
}

func (m *Model) dated(g store.Row) (store.Row, bool) {
	for _, row := range m.activities(g) {
		if row["start"] != "" {
			return row, true
		}
	}
	return nil, false
}

func whenWords(row store.Row) string {
	from, until, allDay, ok := mail.Span(row["start"], row["end"], School)
	if !ok {
		return row["timing"]
	}
	if allDay {
		return from.Format("Monday, January 2")
	}
	return from.Format("Monday, January 2 · 3:04") + "–" + until.Format("3:04 PM")
}

func (mm *MembershipMailer) letter(m *Model, g store.Row) mail.Letter {
	l := mail.Letter{Brand: teamBrand, Base: mm.config.Origin("team"), Title: g["name"], Where: g["location"], Path: m.Link("GROUP", g, mm.config.Origin), When: g["timing"]}
	if d, ok := m.dated(g); ok {
		l.When = whenWords(d)
		if l.Where == "" {
			l.Where = d["location"]
		}
	}
	levels := m.activities(g)
	if len(levels) > 1 {
		names := []string{}
		for _, level := range levels[1:] {
			names = append(names, level["name"])
		}
		l.Subtitle = "Part of " + strings.Join(names, " › ")
	}
	return l
}

func (mm *MembershipMailer) invite(m *Model, g store.Row, person string, to []string) (mail.Event, bool) {
	d, ok := m.dated(g)
	if !ok {
		return mail.Event{}, false
	}
	from, until, allDay, ok := mail.Span(d["start"], d["end"], School)
	if !ok {
		return mail.Event{}, false
	}
	summary := g["name"]
	if levels := m.activities(g); len(levels) > 1 {
		summary += " (" + levels[len(levels)-1]["name"] + ")"
	}
	path := m.Link("GROUP", g, mm.config.Origin)
	e := mail.Event{UID: fmt.Sprintf("group-%s-%s@heliosian.com", g["id"], person), Start: from, End: until, AllDay: allDay, Summary: summary,
		Description: path, Location: d["location"], URL: path, Organizer: mail.Person{Name: teamBrand.Name, Email: mail.AddressOf(mm.config.Sender.From())}}
	for _, p := range to {
		e.Attendees = append(e.Attendees, mail.Person{Name: m.nameOf(p), Email: m.addressOf(p)})
	}
	return e, true
}

func (mm *MembershipMailer) calendar(e mail.Event, method string) mail.Attachment {
	return mail.Calendar{Product: teamBrand.Name, Method: method, Stamp: time.Now(), Events: []mail.Event{e}}.Attachment()
}

func (mm *MembershipMailer) thank(ctx context.Context, m *Model, g store.Row, change placeChange) {
	chairs := without(m.managersAbove(g), change.person)
	l := mm.letter(m, g)
	hi := "Hi"
	if first := m.firstName(change.person); first != "" {
		hi += " " + first
	}
	by := ""
	if change.actor != "" && change.actor != change.person {
		by = fmt.Sprintf(" %s signed you up.", m.nameOf(change.actor))
	}
	l.Heading = "Thank you for volunteering!"
	l.Intro = fmt.Sprintf("%s - you're signed up for %s.%s The chairs are copied here, so just reply if you have a question.", hi, g["name"], by)
	l.Rows = [][2]string{{"Your role", "Volunteer"}}
	if change.note != "" {
		label := "Your note"
		if change.actor != "" && change.actor != change.person {
			label = "Note from " + m.nameOf(change.actor)
		}
		l.Rows = append(l.Rows, [2]string{label, change.note})
	}
	l.Rows = append(l.Rows, m.chairRows(g)...)
	l.Button = "See the details"
	l.Footnote = "Need to change or cancel? Open the page and use Edit my sign-up."
	to := m.withParents(change.person)
	e, dated := mm.invite(m, g, change.person, to)
	if dated {
		l.Calendar = e.GoogleLink()
	}
	mm.send(ctx, m, g, "notice", "Thanks for volunteering for "+g["name"], l, to, chairs, chairs, nil)
	if !dated {
		return
	}
	il := mm.letter(m, g)
	il.Heading = "Add it to your calendar"
	il.Intro = fmt.Sprintf("Here's the calendar invite for %s - accept it and it's on your calendar. The details of your sign-up are in the note that came with it.", g["name"])
	il.Button = "See the details"
	il.Calendar = l.Calendar
	mm.send(ctx, m, g, "calendar", "Calendar invite: "+g["name"], il, to, nil, chairs, []mail.Attachment{mm.calendar(e, mail.MethodRequest)})
}

func (mm *MembershipMailer) removed(ctx context.Context, m *Model, g store.Row, change placeChange) {
	to := m.withParents(change.person)
	e, dated := mm.invite(m, g, change.person, to)
	if !dated {
		return
	}
	l := mm.letter(m, g)
	l.Heading = "You're no longer signed up"
	l.Intro = fmt.Sprintf("You removed your sign-up for %s, so it comes off your calendar.", g["name"])
	if change.actor != "" && change.actor != change.person {
		l.Intro = fmt.Sprintf("%s removed your sign-up for %s, so it comes off your calendar. If that's a surprise, the chairs can put you back - just reply.", m.nameOf(change.actor), g["name"])
	}
	l.Button = "See the details"
	chairs := without(m.managersAbove(g), change.person)
	mm.send(ctx, m, g, "notice", "Removed: "+g["name"], l, to, chairs, chairs, []mail.Attachment{mm.calendar(e, mail.MethodCancel)})
}

func (mm *MembershipMailer) leads(ctx context.Context, m *Model, g store.Row, change placeChange) {
	others := without(m.managersAbove(g), change.person)
	l := mm.letter(m, g)
	hi := "Hi"
	if first := m.firstName(change.person); first != "" {
		hi += " " + first
	}
	l.Heading = "You're a co-chair of " + g["name"]
	l.Intro = fmt.Sprintf("%s - %s made you a co-chair. You can now edit the page, add things under it, and see and manage everyone who signs up.", hi, m.nameOf(change.actor))
	if len(others) > 0 {
		names := []string{}
		for _, p := range others {
			names = append(names, m.nameOf(p))
		}
		l.Rows = [][2]string{{"Co-chairs", strings.Join(names, ", ")}}
	}
	l.Button = "Open " + g["name"]
	l.Footnote = "The other co-chairs are copied on this note."
	mm.send(ctx, m, g, "notice", "You're a co-chair of "+g["name"], l, []string{change.person}, others, others, nil)
}

func (mm *MembershipMailer) notice(ctx context.Context, m *Model, g store.Row, actor, person, subject, heading, intro string, rows [][2]string) {
	to := without(m.notified(g), actor, person)
	if len(to) == 0 {
		return
	}
	l := mm.letter(m, g)
	if len(m.activities(g)) == 0 {
		l.Brand = heliosianBrand
	}
	l.Heading, l.Intro, l.Rows, l.Button = heading, intro, rows, "Open "+g["name"]
	mm.send(ctx, m, g, "notice", subject, l, to, nil, nil, nil)
}

func (mm *MembershipMailer) addresses(m *Model, people []string) []string {
	out := []string{}
	for _, p := range people {
		if a := m.addressOf(p); a != "" {
			out = append(out, a)
		}
	}
	return out
}

func (mm *MembershipMailer) send(ctx context.Context, m *Model, g store.Row, kind, subject string, l mail.Letter, to, cc, replyTo []string, attachments []mail.Attachment) {
	msg := l.Message(subject, mm.addresses(m, to), mm.addresses(m, cc), mm.addresses(m, replyTo))
	if len(msg.To) == 0 {
		return
	}
	msg.Attachments = attachments
	log := slog.With("group", g["id"], "kind", kind, "subject", subject)
	if err := mm.config.Sender.Send(ctx, msg); err != nil {
		log.Error("membership mail: send failed", "error", err)
		return
	}
	created := stamp(time.Now())
	edits := []Edit{{Insert: "MESSAGE", As: "m", Row: map[string]any{"direction": "out", "kind": kind, "group": g["id"], "subject": subject, "created": created}}}
	for _, p := range slices.Compact(append(slices.Clone(to), cc...)) {
		edits = append(edits, Edit{Insert: "RECIPIENT", Row: map[string]any{"message": "@m", "person": p, "created": created, "sent": created}})
	}
	if _, err := Write(ctx, mm.s, mm.queue, access.System(mailerSystem), Env{System: mailerSystem, Now: time.Now()}, Batch{Batch: edits}); err != nil {
		log.Error("membership mail: not recorded", "error", err)
		return
	}
	log.Info("membership mail: sent", "to", len(msg.To), "cc", len(msg.CC))
}
