package celebrate

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/mail"
)

var brand = mail.Brand{Name: "Helios Celebrate", Color: "#0f4e54", Tagline: "the fun(d)raiser parties site"}

func partyEvent(p *Party, page string) (mail.Event, bool) {
	start, until, allDay, ok := mail.Span(p.Start, p.End, local)
	if !ok {
		return mail.Event{}, false
	}
	where := p.Location
	if p.Address != "" {
		where = p.Address
	}
	details := strings.TrimSpace(p.Summary)
	if details != "" {
		details += "\n\n"
	}
	return mail.Event{Start: start, End: until, AllDay: allDay, Summary: p.Title, Description: details + page, Location: where, URL: page}, true
}

func (a app) event(p *Party, purchaser, page string, to []string) (mail.Event, bool) {
	e, ok := partyEvent(p, page)
	if !ok {
		return e, false
	}
	e.UID = fmt.Sprintf("celebrate-%s-%s@heliosian.com", p.ID, purchaser)
	e.Organizer = mail.Person{Name: brand.Name, Email: a.from}
	for _, email := range to {
		e.Attendees = append(e.Attendees, mail.Person{Name: a.attendeeName(p, email), Email: email})
	}
	return e, true
}

func calendarLink(p *Party, page string) string {
	e, ok := partyEvent(p, page)
	if !ok {
		return ""
	}
	return e.GoogleLink()
}

func (a app) attendeeName(p *Party, email string) string {
	for _, t := range p.Tickets {
		if t.Email == email {
			return ticketName(a.directory, map[string]string{"Email": t.Email, "Name": t.Name})
		}
	}
	return nameOf(a.directory, email)
}

func (a app) letterFor(base string, p *Party) mail.Letter {
	model := a.cache.Model()
	l := mail.Letter{Brand: brand, Base: base, Title: p.Title, Subtitle: p.Subtitle, When: when(p), Where: p.Location, Path: base + model.PathOf(p)}
	if p.Address != "" {
		if l.Where != "" {
			l.Where += " · "
		}
		l.Where += p.Address
	}
	if previewable(p) {
		l.Picture = base + "/open/share/" + p.ID + ".png"
	}
	return l
}

func (a app) sendGoing(ctx context.Context, subject string, to []string, cc []string, l mail.Letter, p *Party, purchaser string) {
	replyTo := without(p.HostEmails, purchaser)
	note := l.Message(subject, to, cc, replyTo)
	mail.Post(ctx, a.mailer, note)
	e, ok := a.event(p, purchaser, l.Path, note.To)
	if !ok {
		return
	}
	il := a.letterFor(l.Base, p)
	il.Heading = "Add it to your calendar"
	il.Intro = fmt.Sprintf("Here's the calendar invite for %s - accept it and the party is on your calendar. The details of your tickets are in the note that came with it.", p.Title)
	il.Button = "See the party"
	il.Calendar = l.Calendar
	invite := il.Message("Calendar invite: "+p.Title, note.To, nil, replyTo)
	invite.Attachments = []mail.Attachment{mail.Calendar{Product: brand.Name, Method: mail.MethodRequest, Stamp: time.Now(), Events: []mail.Event{e}}.Attachment()}
	mail.Post(ctx, a.mailer, invite)
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

func (a app) firstName(email string) string {
	words := strings.Fields(nameOf(a.directory, email))
	if len(words) == 0 {
		return ""
	}
	return words[0]
}

func (a app) mailTickets(r *http.Request, p *Party, purchaser string, taken []map[string]string, actor string) {
	if a.mailer == nil || len(taken) == 0 {
		return
	}
	base := mail.Base(r)
	model := a.cache.Model()
	if now := model.Party(p.ID); now != nil {
		p = now
	}
	l := a.letterFor(base, p)
	sold := []string{}
	waiting := 0
	total := 0.0
	for _, t := range taken {
		who := ticketName(a.directory, t)
		if t["Status"] == TicketSold {
			sold = append(sold, who)
			price, _ := ParsePrice(t["Price"])
			total += price
		} else {
			n, _ := strconv.Atoi(t["Quantity"])
			waiting += max(n, 1)
		}
	}
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
	free := len(sold) > 0 && total == 0
	guestOf := free && holder != "" && taken[0]["Email"] == "" && purchaser != actor
	hi := "Hi"
	if holder != "" && !guestOf {
		if words := strings.Fields(holder); len(words) > 0 {
			hi = "Hi " + words[0]
		}
	} else if first := a.firstName(purchaser); first != "" {
		hi = "Hi " + first
	}
	by := ""
	switch {
	case free && purchaser != actor:
		by = fmt.Sprintf(" %s has added you at no charge as %s's guest - a gift from the hosts.", nameOf(a.directory, actor), nameOf(a.directory, purchaser))
	case free:
		by = fmt.Sprintf(" %s has added you at no charge - a gift from the hosts.", nameOf(a.directory, actor))
	case holder != "":
		by = fmt.Sprintf(" %s took it for you.", nameOf(a.directory, actor))
	case actor != purchaser:
		by = fmt.Sprintf(" %s took them for your family.", nameOf(a.directory, actor))
	}
	subject := ""
	switch {
	case len(sold) > 0 && waiting > 0:
		l.Heading = "Your tickets, and a place on the waitlist"
		l.Intro = fmt.Sprintf("%s - %s. The party was full before everyone could get in, so your family is on the waitlist for %d more; the hosts will offer places as they open up. The hosts are copied here, so just reply if you have a question.", hi, ticketsWords(len(sold), p.Title), waiting) + by
		subject = "Your tickets to " + p.Title
	case guestOf:
		l.Heading = strings.Fields(holder)[0] + " is going!"
		l.Intro = fmt.Sprintf("%s - %s has a ticket to %s as your guest; %s added them at no charge - a gift from the hosts. The ticket sits with your family, yours to pass on if plans change. The hosts are copied here, so just reply if you have a question.", hi, holder, p.Title, nameOf(a.directory, actor))
		subject = holder + "'s ticket to " + p.Title
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
		l.Rows = append(l.Rows, [2]string{"Billed to", fmt.Sprintf("%s (%s)", nameOf(a.directory, purchaser), purchaser)})
	}
	if note := taken[0]["Note"]; note != "" {
		l.Rows = append(l.Rows, [2]string{"Your note", note})
	}
	if names := a.hostNames(p); names != "" {
		l.Rows = append(l.Rows, [2]string{"Hosts", names})
	}
	l.Button = "See the party"
	if len(sold) > 0 {
		l.Calendar = calendarLink(p, l.Path)
		if !free {
			l.Footnote = model.Settings.TicketNote
		}
	}
	cc := []string{}
	if actor != purchaser {
		cc = append(cc, actor)
	}
	if len(sold) > 0 {
		a.sendGoing(r.Context(), subject, to, append(cc, without(p.HostEmails, purchaser)...), l, p, purchaser)
		return
	}
	mail.Post(r.Context(), a.mailer, l.Message(subject, to, cc, without(p.HostEmails, purchaser)))
	a.mailWaitlistHosts(r, p, purchaser, waiting, taken[0]["Note"], actor)
}

func (a app) mailWaitlistHosts(r *http.Request, p *Party, purchaser string, waiting int, note, actor string) {
	hosts := without(p.HostEmails, purchaser)
	if a.mailer == nil || len(hosts) == 0 {
		return
	}
	l := a.letterFor(mail.Base(r), p)
	who := nameOf(a.directory, purchaser)
	l.Heading = fmt.Sprintf("%s joined the waitlist", who)
	l.Intro = fmt.Sprintf("%s would like %d %s to %s once places open up. Nothing is billed until you offer them - open the party and use Offer beside the request when you can. Reply to this note to reach %s directly.", who, waiting, plural(waiting, "ticket"), p.Title, who)
	if actor != purchaser {
		l.Intro += fmt.Sprintf(" (%s made the request for the family.)", nameOf(a.directory, actor))
	}
	l.Rows = [][2]string{{"Waiting", fmt.Sprintf("%s (%s)", who, purchaser)}, {"Tickets", fmt.Sprintf("%d", waiting)}}
	if note != "" {
		l.Rows = append(l.Rows, [2]string{"Their note", note})
	}
	l.Rows = append(l.Rows, [2]string{"Waitlist", fmt.Sprintf("%d %s in all", p.Waiting(), plural(p.Waiting(), "ticket"))})
	l.Button = "Open the party"
	mail.Post(r.Context(), a.mailer, l.Message(fmt.Sprintf("Waitlist for %s: %s wants %d %s", p.Title, who, waiting, plural(waiting, "ticket")), hosts, nil, []string{purchaser}))
}

func (a app) mailOffered(r *http.Request, p *Party, purchaser string, tickets []map[string]string, actor string) {
	if a.mailer == nil || len(tickets) == 0 {
		return
	}
	base := mail.Base(r)
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
		names = append(names, ticketName(a.directory, t))
		price, _ := ParsePrice(t["Price"])
		total += price
	}
	l.Heading = "A place opened up!"
	l.Intro = fmt.Sprintf("%s - %s has offered your family %d %s to %s, off the waitlist. They're yours now. The hosts are copied here, so just reply if you have a question.", hi, nameOf(a.directory, actor), n, plural(n, "ticket"), p.Title)
	l.Rows = [][2]string{{"Tickets", strings.Join(names, ", ")}, {"Total", fmt.Sprintf("$%s (%d × $%s)", PriceCell(total), n, PriceCell(total/float64(n)))}, {"Billed to", fmt.Sprintf("%s (%s)", nameOf(a.directory, purchaser), purchaser)}}
	if hosts := a.hostNames(p); hosts != "" {
		l.Rows = append(l.Rows, [2]string{"Hosts", hosts})
	}
	l.Button = "See the party"
	l.Calendar = calendarLink(p, l.Path)
	l.Footnote = "A ticket marked \"to be named\" is a guest's: open the party and use Reassign beside it to say who is coming. " + a.cache.Model().Settings.TicketNote
	a.sendGoing(r.Context(), fmt.Sprintf("You're in: %d %s to %s", n, plural(n, "ticket"), p.Title), []string{purchaser}, without(p.HostEmails, purchaser), l, p, purchaser)
}

func (a app) hostNames(p *Party) string {
	if p.Hosts != "" {
		return p.Hosts
	}
	names := []string{}
	for _, h := range p.HostEmails {
		names = append(names, nameOf(a.directory, h))
	}
	return strings.Join(names, ", ")
}

func ticketsWords(n int, title string) string {
	if n == 1 {
		return "you have a ticket to " + title
	}
	return fmt.Sprintf("you have %d tickets to %s", n, title)
}
