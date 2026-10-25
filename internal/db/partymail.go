package db

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/mail"
	"heliosian/internal/store"
)

var celebrateBrand = mail.Brand{Name: "Helios Celebrate", Color: "#0f4e54", Tagline: "the fun(d)raiser parties site"}

type partyWrite struct {
	write       uint64
	party, what string
	waiting     bool
}

func (m *Model) partyOf(change placeChange) (string, bool, bool) {
	g, ok := m.Table("GROUP").Get(change.group)
	if !ok {
		return "", false, false
	}
	if g["kind"] == "party" {
		return g["id"], false, true
	}
	if served, ok := m.servedBy("waitlist", g); ok && served["kind"] == "party" {
		return served["id"], true, true
	}
	return "", false, false
}

func (mm *MembershipMailer) partyMail(ctx context.Context, key partyWrite, changes []placeChange) {
	m := mm.s.Model()
	g, ok := m.Table("GROUP").Get(key.party)
	if !ok {
		return
	}
	switch {
	case key.what == placeMoved:
		for _, change := range changes {
			mm.passed(ctx, m, g, change)
		}
	case key.what == placeJoined && key.waiting:
		mm.waiting(ctx, m, g, changes)
	case key.what == placeJoined:
		mm.tickets(ctx, m, g, changes)
	}
}

func (m *Model) unnamed(person string) bool {
	p, ok := m.Table("PERSON").Get(person)
	return ok && p["source"] == "guest" && p["name_long_override"] == ""
}

func (m *Model) placeName(person string) string {
	if m.unnamed(person) {
		return "a guest"
	}
	return m.nameOf(person)
}

func (m *Model) paymentFor(g store.Row, person string) (store.Row, bool) {
	var found store.Row
	for _, p := range m.Table("PAYMENT").Referencing("group", g["id"]) {
		if p["for"] == person && p["status"] != "cancelled" && (found == nil || p["added"] > found["added"]) {
			found = p
		}
	}
	return found, found != nil
}

func (m *Model) Setting(app, key string) string {
	for _, s := range m.Table("SETTING").All() {
		if s["app"] == app && s["key"] == key {
			return s["value"]
		}
	}
	return ""
}

func dollarsOf(cents int) string {
	return strconv.FormatFloat(float64(cents)/100, 'f', -1, 64)
}

func centsOf(cell string) int {
	f, err := strconv.ParseFloat(strings.TrimSpace(cell), 64)
	if err != nil {
		return 0
	}
	return int(f*100 + 0.5)
}

func (mm *MembershipMailer) partyLetter(m *Model, g store.Row) mail.Letter {
	base := mm.config.Origin("celebrate")
	l := mail.Letter{Brand: celebrateBrand, Base: base, Title: g["name"], Subtitle: g["subtitle"], When: whenWords(g), Where: g["location"], Path: g["href"],
		Picture: base + "/open/share/" + g["id"] + ".png"}
	if g["address"] != "" {
		if l.Where != "" {
			l.Where += " · "
		}
		l.Where += g["address"]
	}
	return l
}

func (m *Model) hostsOfParty(g store.Row) []string {
	return m.membersOf(g["managed_by"])
}

func (m *Model) names(people []string) string {
	out := []string{}
	for _, p := range people {
		out = append(out, m.placeName(p))
	}
	return joinNames(out)
}

func (mm *MembershipMailer) partyEvent(m *Model, g store.Row, payer string, to []string) (mail.Event, bool) {
	from, until, allDay, ok := mail.Span(g["start"], g["end"], School)
	if !ok {
		return mail.Event{}, false
	}
	path := g["href"]
	where := g["location"]
	if g["address"] != "" {
		where = g["address"]
	}
	e := mail.Event{UID: fmt.Sprintf("celebrate-%s-%s@heliosian.com", g["id"], payer), Start: from, End: until, AllDay: allDay, Summary: g["name"],
		Description: path, Location: where, URL: path, Organizer: mail.Person{Name: celebrateBrand.Name, Email: mail.AddressOf(mm.config.Celebrate.From())}}
	for _, p := range to {
		e.Attendees = append(e.Attendees, mail.Person{Name: m.nameOf(p), Email: m.addressOf(p)})
	}
	return e, true
}

func (mm *MembershipMailer) tickets(ctx context.Context, m *Model, g store.Row, changes []placeChange) {
	actor := changes[0].actor
	holders := []string{}
	payer, total := "", 0
	for _, change := range changes {
		holders = append(holders, change.person)
		if p, ok := m.paymentFor(g, change.person); ok {
			payer = p["person"]
			total += centsOf(p["amount"])
		}
	}
	billed := payer != ""
	if !billed {
		payer = actor
	}
	hosts := m.hostsOfParty(g)
	to := []string{payer}
	one := len(holders) == 1 && holders[0] != payer && !m.unnamed(holders[0])
	if one {
		to = append(without(m.withParents(holders[0]), payer), payer)
	}
	l := mm.partyLetter(m, g)
	hi := m.greeting(to[0])
	by := ""
	switch {
	case !billed && actor != "":
		by = fmt.Sprintf(" %s added you at no charge - a gift from the hosts.", m.nameOf(actor))
	case actor != "" && actor != payer:
		by = fmt.Sprintf(" %s took them for your family.", m.nameOf(actor))
	}
	words := fmt.Sprintf("you have %d tickets to %s", len(holders), g["name"])
	if len(holders) == 1 {
		words = "you have a ticket to " + g["name"]
	}
	subject := "Your tickets to " + g["name"]
	l.Heading = "You're going!"
	if one {
		subject = m.nameOf(holders[0]) + "'s ticket to " + g["name"]
		if first := m.firstName(holders[0]); first != "" {
			l.Heading = first + ", you're going!"
		}
	}
	l.Intro = fmt.Sprintf("%s - %s.%s The hosts are copied here, so just reply if you have a question.", hi, words, by)
	l.Rows = [][2]string{{"Tickets", m.names(holders)}}
	if billed {
		l.Rows = append(l.Rows, [2]string{"Total", fmt.Sprintf("$%s (%d × $%s)", dollarsOf(total), len(holders), g["price"])}, [2]string{"Billed to", m.nameOf(payer)})
		l.Footnote = m.Setting("celebrate", "Ticket Note")
	} else {
		l.Rows = append(l.Rows, [2]string{"Total", "Free"})
	}
	if changes[0].note != "" {
		l.Rows = append(l.Rows, [2]string{"Your note", changes[0].note})
	}
	if names := m.names(hosts); names != "" {
		l.Rows = append(l.Rows, [2]string{"Hosts", names})
	}
	l.Button = "See the party"
	cc := without(hosts, to...)
	if actor != "" && !slices.Contains(to, actor) && !slices.Contains(cc, actor) {
		cc = append(cc, actor)
	}
	replyTo := without(hosts, payer)
	e, dated := mm.partyEvent(m, g, payer, to)
	if dated {
		l.Calendar = e.GoogleLink()
	}
	mm.send(ctx, mm.config.Celebrate, m, g, "notice", subject, l, to, cc, replyTo, nil)
	if !dated {
		return
	}
	il := mm.partyLetter(m, g)
	il.Heading = "Add it to your calendar"
	il.Intro = fmt.Sprintf("Here's the calendar invite for %s - accept it and the party is on your calendar. The details of your tickets are in the note that came with it.", g["name"])
	il.Button = "See the party"
	il.Calendar = l.Calendar
	invite := mail.Calendar{Product: celebrateBrand.Name, Method: mail.MethodRequest, Stamp: time.Now(), Events: []mail.Event{e}}.Attachment()
	mm.send(ctx, mm.config.Celebrate, m, g, "calendar", "Calendar invite: "+g["name"], il, to, nil, replyTo, []mail.Attachment{invite})
}

func (mm *MembershipMailer) waiting(ctx context.Context, m *Model, g store.Row, changes []placeChange) {
	actor := changes[0].actor
	asker := actor
	for _, change := range changes {
		if !m.unnamed(change.person) {
			asker = change.person
			break
		}
	}
	n := len(changes)
	tickets := "tickets"
	if n == 1 {
		tickets = "ticket"
	}
	hosts := m.hostsOfParty(g)
	note := changes[0].note
	l := mm.partyLetter(m, g)
	hi := m.greeting(asker)
	l.Heading = "You're on the waitlist"
	l.Intro = fmt.Sprintf("%s - %s is full, so your family is on its waitlist for %d %s. Nothing is billed unless a place opens up; the hosts give places as they do, and you'll get a note when it happens.", hi, g["name"], n, tickets)
	l.Rows = [][2]string{{"Waitlist", fmt.Sprintf("%d %s", n, tickets)}}
	if note != "" {
		l.Rows = append(l.Rows, [2]string{"Your note", note})
	}
	l.Button = "See the party"
	cc := []string{}
	if actor != "" && actor != asker {
		cc = append(cc, actor)
	}
	mm.send(ctx, mm.config.Celebrate, m, g, "notice", "You're on the waitlist for "+g["name"], l, []string{asker}, cc, without(hosts, asker), nil)
	hosts = without(hosts, asker)
	if len(hosts) == 0 {
		return
	}
	waiting := len(m.Table("MEMBER").Referencing("group", g["waitlist"]))
	hl := mm.partyLetter(m, g)
	who := m.nameOf(asker)
	hl.Heading = who + " joined the waitlist"
	hl.Intro = fmt.Sprintf("%s would like %d %s to %s once places open up. When one does, open the party and give them the place from the waitlist. Reply to this note to reach %s directly.", who, n, tickets, g["name"], who)
	hl.Rows = [][2]string{{"Waiting", who}, {"Tickets", strconv.Itoa(n)}}
	if note != "" {
		hl.Rows = append(hl.Rows, [2]string{"Their note", note})
	}
	hl.Rows = append(hl.Rows, [2]string{"Waitlist", fmt.Sprintf("%d in all", waiting)})
	hl.Button = "Open the party"
	mm.send(ctx, mm.config.Celebrate, m, g, "notice", fmt.Sprintf("Waitlist for %s: %s wants %d %s", g["name"], who, n, tickets), hl, hosts, nil, []string{asker}, nil)
}

func (mm *MembershipMailer) passed(ctx context.Context, m *Model, g store.Row, change placeChange) {
	if m.unnamed(change.person) {
		return
	}
	hosts := m.hostsOfParty(g)
	to := m.withParents(change.person)
	cc := without(hosts, to...)
	if change.actor != "" && !slices.Contains(to, change.actor) && !slices.Contains(cc, change.actor) {
		cc = append(cc, change.actor)
	}
	l := mm.partyLetter(m, g)
	first := m.firstName(change.person)
	l.Heading = first + ", you're going!"
	l.Intro = fmt.Sprintf("Hi %s - %s has passed you a ticket to %s. The hosts are copied here, so just reply if you have a question.", first, m.nameOf(change.actor), g["name"])
	l.Rows = [][2]string{{"Ticket", m.nameOf(change.person)}}
	if names := m.names(hosts); names != "" {
		l.Rows = append(l.Rows, [2]string{"Hosts", names})
	}
	l.Button = "See the party"
	if e, ok := mm.partyEvent(m, g, change.person, to); ok {
		l.Calendar = e.GoogleLink()
	}
	mm.send(ctx, mm.config.Celebrate, m, g, "notice", m.nameOf(change.person)+"'s ticket to "+g["name"], l, to, cc, without(hosts, change.person), nil)
}
