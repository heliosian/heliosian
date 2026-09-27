package team

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"heliosian/internal/cells"
	"heliosian/internal/mail"
)

var NotifyKinds = []string{"events", "activities", "signups", "offers"}

var brand = mail.Brand{Name: "HCA-Team", Color: "#1f4d53", Tagline: "the HCA volunteer portal"}

func (a app) adminsWanting(kind string, except ...string) []string {
	model := a.cache.Model()
	out := []string{}
	for _, admin := range a.cache.Admins(a.superAdmins()) {
		if slices.Contains(except, admin) {
			continue
		}
		if model.notifyPrefs(admin)[kind] {
			out = append(out, admin)
		}
	}
	return out
}

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

func (a app) letterFor(base string, act *Activity) mail.Letter {
	model := a.cache.Model()
	l := mail.Letter{
		Brand: brand,
		Base:  base,
		Title: act.Title,
		When:  when(timed(model, act)),
		Where: act.Location,
		Path:  base + model.PathOf(act),
	}
	if under := lineage(model, act); under != "" {
		l.Subtitle = "Part of " + under
	}
	for n := act; n != nil; n = model.byID[n.Parent] {
		if previewable(model, n) {
			l.Picture = base + "/open/share/" + n.ID + ".png"
			break
		}
	}
	return l
}

func (a app) event(m *Model, act *Activity, email, page string, to []string) (mail.Event, bool) {
	dated := act
	for dated != nil && dated.Start == "" {
		dated = m.byID[dated.Parent]
	}
	if dated == nil {
		return mail.Event{}, false
	}
	start, until, allDay, ok := mail.Span(dated.Start, dated.End, local)
	if !ok {
		return mail.Event{}, false
	}
	title := act.Title
	if under := lineage(m, act); under != "" {
		title += " (" + under + ")"
	}
	e := mail.Event{
		UID:         fmt.Sprintf("team-%s-%s@heliosian.com", act.ID, email),
		Start:       start,
		End:         until,
		AllDay:      allDay,
		Summary:     title,
		Description: page,
		Location:    act.Location,
		URL:         page,
		Organizer:   mail.Person{Name: brand.Name, Email: a.from},
	}
	for _, t := range to {
		e.Attendees = append(e.Attendees, mail.Person{Name: a.nameOf(t), Email: t})
	}
	return e, true
}

func (a app) invite(e mail.Event, method string) mail.Attachment {
	return mail.Calendar{Product: brand.Name, Method: method, Stamp: time.Now(), Events: []mail.Event{e}}.Attachment()
}

func (a app) mailRemoved(r *http.Request, act *Activity, email, actor string) {
	m := a.cache.Model()
	l := a.letterFor(mail.Base(r), act)
	l.Heading = "You're no longer signed up"
	l.Intro = fmt.Sprintf("You removed your sign-up for %s, so it comes off your calendar.", act.Title)
	if actor != email {
		l.Intro = fmt.Sprintf("%s removed your sign-up for %s, so it comes off your calendar. If that's a surprise, the chairs can put you back - just reply.", a.nameOf(actor), act.Title)
	}
	l.Button = "See the details"
	msg := l.Message("Removed: "+act.Title, append([]string{email}, parentsOf(a.directory(), email)...), nil, without(chairsAround(m, act), email))
	e, ok := a.event(m, act, email, l.Path, msg.To)
	if !ok {
		return
	}
	msg.Attachments = []mail.Attachment{a.invite(e, mail.MethodCancel)}
	mail.Post(r.Context(), a.mailer, msg)
}

func (a app) nameOf(email string) string {
	p := a.directory().Person(email)
	if p == nil || p.FullName == "" {
		return cells.DisplayName(email)
	}
	return p.FullName
}

func (a app) mailSignUp(r *http.Request, act *Activity, email, position, note, actor string, existed bool, was string) {
	base := mail.Base(r)
	ctx := r.Context()
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
		mail.Post(ctx, a.mailer, l.Message(fmt.Sprintf("You're a co-chair of %s", act.Title), []string{email}, without(chairs, email), without(chairs, email)))
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
			label := "Your note"
			if actor != email {
				label = "Note from " + a.nameOf(actor)
			}
			l.Rows = append(l.Rows, [2]string{label, note})
		}
		l.Rows = append(l.Rows, a.chairRows(model, act)...)
		l.Button = "See the details"
		l.Footnote = "Need to change or cancel? Open the page and use Edit my sign-up."
		subject := fmt.Sprintf("Thanks for volunteering for %s", act.Title)
		to := append([]string{email}, parentsOf(a.directory(), email)...)
		replyTo := without(chairs, email)
		e, dated := a.event(model, act, email, l.Path, nil)
		if dated {
			l.Calendar = e.GoogleLink()
		}
		note := l.Message(subject, to, replyTo, replyTo)
		mail.Post(ctx, a.mailer, note)
		if dated {
			for _, t := range note.To {
				e.Attendees = append(e.Attendees, mail.Person{Name: a.nameOf(t), Email: t})
			}
			il := a.letterFor(base, act)
			il.Heading = "Add it to your calendar"
			il.Intro = fmt.Sprintf("Here's the calendar invite for %s - accept it and it's on your calendar. The details of your sign-up are in the note that came with it.", act.Title)
			il.Button = "See the details"
			il.Calendar = l.Calendar
			invite := il.Message("Calendar invite: "+act.Title, note.To, nil, replyTo)
			invite.Attachments = []mail.Attachment{a.invite(e, mail.MethodRequest)}
			mail.Post(ctx, a.mailer, invite)
		}
	}
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
			mail.Post(ctx, a.mailer, n.Message(fmt.Sprintf("New sign-up: %s for %s", name, act.Title), admins, nil, nil))
		}
	}
	if position == PositionOpen && was != PositionOpen {
		if admins := a.adminsWanting("offers", actor, email); len(admins) > 0 {
			n := a.letterFor(base, act)
			n.Heading = fmt.Sprintf("%s offered to co-chair %s", name, act.Title)
			if actor != email {
				n.Heading = fmt.Sprintf("%s added %s as co-chair of %s", a.nameOf(actor), name, act.Title)
			}
			n.Intro = "Someone is open to co-chairing. Open the page to make them a co-chair, or leave them as a volunteer."
			n.Rows = [][2]string{{"Who", fmt.Sprintf("%s (%s)", name, email)}}
			if note != "" {
				n.Rows = append(n.Rows, [2]string{"Note", note})
			}
			if actor != email {
				n.Rows = append(n.Rows, [2]string{"Added by", a.nameOf(actor)})
			}
			n.Button = "Open " + act.Title
			mail.Post(ctx, a.mailer, n.Message(fmt.Sprintf("Co-chair offer: %s for %s", name, act.Title), admins, nil, nil))
		}
	}
}

func (a app) mailNewActivity(r *http.Request, act *Activity, actor string) {
	kind, what := "events", "A new event was added"
	if act.Parent != "" {
		kind, what = "activities", "Something new was added under "+lineage(a.cache.Model(), act)
	}
	admins := a.adminsWanting(kind, actor)
	if len(admins) == 0 {
		return
	}
	l := a.letterFor(mail.Base(r), act)
	l.Heading = act.Title
	l.Intro = fmt.Sprintf("%s by %s.", what, a.nameOf(actor))
	l.Rows = [][2]string{{"Status", act.Status}}
	if act.Status == StatusPending {
		l.Rows = append(l.Rows, [2]string{"Needs", "Your approval, under Approval Needed"})
	}
	l.Button = "Open " + act.Title
	mail.Post(r.Context(), a.mailer, l.Message(fmt.Sprintf("New: %s", act.Title), admins, nil, nil))
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
