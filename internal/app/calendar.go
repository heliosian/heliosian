package app

import (
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/celebrate"
	"heliosian/internal/team"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

type calendarLinked struct {
	celebrate *celebrate.Cache
	team      *team.Cache
	directory func() *who.Model
}

type familyNames map[string]string

func (h familyNames) has(email string) bool {
	_, ok := h[email]
	return ok
}

func firstName(name, email string) string {
	if words := strings.Fields(name); len(words) > 0 {
		return words[0]
	}
	local, _, _ := strings.Cut(email, "@")
	return local
}

func (c calendarLinked) list(email string) []when.Linked {
	now := time.Now().In(when.Location)
	model := c.directory()
	as := access.Actor{Email: email, Household: model.Family(email)}
	family := familyNames{email: ""}
	full := map[string]string{}
	adults, kids := model.Household(email)
	for _, p := range append(adults, kids...) {
		if as.Household[p.Email] {
			full[p.Email] = p.FullName
			family[p.Email] = firstName(p.FullName, p.Email)
		}
	}
	if me := model.Person(email); me != nil && me.FullName != "" {
		full[email] = me.FullName
	}
	return append(c.parties(now, family, full), c.activities(as, family, full)...)
}

func nameOf(full map[string]string, email, fallback string) string {
	if n := full[email]; n != "" {
		return n
	}
	if fallback != "" {
		return fallback
	}
	local, _, _ := strings.Cut(email, "@")
	return local
}

type standing struct {
	mine  bool
	names []string
}

func (s *standing) add(family familyNames, email, name string) {
	if email == "" {
		return
	}
	if family.has(email) && family[email] == "" {
		s.mine = true
		return
	}
	who := family[email]
	if who == "" {
		who = firstName(name, email)
	}
	if !slices.Contains(s.names, who) {
		s.names = append(s.names, who)
	}
}

func (s standing) who() []string {
	if s.mine {
		return nil
	}
	return s.names
}

func (c calendarLinked) parties(now time.Time, family familyNames, full map[string]string) []when.Linked {
	out := []when.Linked{}
	model := c.celebrate.Model()
	for _, p := range model.SortedParties("") {
		if !p.VisibleTo(access.Actor{}) || p.Start == "" {
			continue
		}
		var going, waiting standing
		people := []when.Standing{}
		for _, t := range p.Tickets {
			if !family.has(t.Purchaser) && !family.has(t.Email) {
				continue
			}
			holder := t.Email
			if !family.has(holder) {
				holder = t.Purchaser
			}
			switch t.Status {
			case celebrate.TicketSold:
				going.add(family, holder, t.Name)
				if family.has(t.Email) {
					people = append(people, when.Standing{Name: nameOf(full, t.Email, t.Name), Mine: family[t.Email] == ""})
				} else {
					people = append(people, when.Standing{Name: nameOf(full, "", t.Name), Note: "guest"})
				}
			case celebrate.TicketWaitlist:
				waiting.add(family, holder, t.Name)
				people = append(people, when.Standing{Name: nameOf(full, holder, t.Name), Note: "waitlisted"})
			}
		}
		mine, who := "", []string(nil)
		if going.mine || len(going.names) > 0 {
			mine, who = when.MineGoing, going.who()
		} else if waiting.mine || len(waiting.names) > 0 {
			mine, who = when.MineWaitlisted, waiting.who()
		}
		out = append(out, when.Linked{
			Source: when.SourceCelebrate, ID: p.ID, Title: p.Title, Summary: p.Summary, Description: p.Description, Location: p.Location,
			Start: p.Start, End: p.End, Path: model.PathOf(p), Availability: p.Availability(now), Mine: mine, Who: who, People: people, Image: p.ImageURL,
		})
	}
	return out
}

func activityImage(model *team.Model, a *team.Activity) string {
	if a.ImageURL != "" {
		return a.ImageURL
	}
	if c := model.Category(a.Category); c != nil {
		return c.ImageURL
	}
	return ""
}

func (c calendarLinked) activities(as access.Actor, family familyNames, full map[string]string) []when.Linked {
	out := []when.Linked{}
	model := c.team.Model()
	for _, raw := range model.Activities {
		if !model.VisibleTo(raw, access.Actor{}) || raw.Start == "" {
			continue
		}
		a := model.ActivityFor(raw, as)
		availability := "open"
		switch {
		case a.Status == team.StatusDone:
			availability = "done"
		case a.VolunteersComplete || (a.Spots > 0 && a.Taken >= a.Spots):
			availability = "full"
		}
		var signed standing
		people := []when.Standing{}
		for _, item := range append([]*team.Activity{a}, a.Descendants()...) {
			for _, v := range item.Volunteers {
				if !family.has(v.Email) {
					continue
				}
				signed.add(family, v.Email, "")
				note := v.Position
				if item != a {
					note = item.Title
				}
				people = append(people, when.Standing{Name: nameOf(full, v.Email, ""), Note: note, Mine: family[v.Email] == ""})
			}
		}
		mine, who := "", []string(nil)
		if signed.mine || len(signed.names) > 0 {
			mine, who = when.MineGoing, signed.who()
		}
		out = append(out, when.Linked{
			Source: when.SourceTeam, ID: a.ID, Title: a.Title, Description: a.Description, Location: a.Location,
			Start: a.Start, End: a.End, Path: model.PathOf(a), Availability: availability, Mine: mine, Who: who, People: people, Image: activityImage(model, a),
			Hosts: a.CoChairs(),
		})
	}
	return out
}

type partyPeople struct {
	celebrate *celebrate.Cache
}

func (p partyPeople) people(id string) *when.PartyPeople {
	party := p.celebrate.Model().Party(id)
	if party == nil {
		return nil
	}
	out := &when.PartyPeople{Hosts: append([]string{}, party.HostEmails...), Attendees: []when.Attendee{}}
	for _, t := range party.Tickets {
		status := "ticket"
		if t.Status == celebrate.TicketWaitlist {
			status = "waitlist"
		} else if t.Price <= 0 {
			status = "free"
		}
		out.Attendees = append(out.Attendees, when.Attendee{Email: strings.ToLower(strings.TrimSpace(t.Email)), Name: t.Name, Status: status})
	}
	return out
}
