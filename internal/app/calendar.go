package app

import (
	"slices"
	"sort"
	"strings"
	"time"

	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

// calendarLinked hands the calendar what the other apps run, as one viewer
// stands with it: every open, dated party on Helios Celebrate and every dated
// event HCA-Team lists, open or done, each with the page it is served at
// there, what a reader can do now, and whether the viewer's familyNames - the
// viewer and everyone in their families, as the directory lists them - is
// already going or waiting. An app whose sheet has not loaded contributes
// nothing.
type calendarLinked struct {
	celebrate *celebrate.Cache
	team      *team.Cache
	directory familyLookup
}

// familyLookup is the one thing the linked list asks of the directory: who
// else is in a person's families.
type familyLookup interface {
	Household(email string) (adults, kids []celebrate.Person)
	Person(email string) (celebrate.Person, bool)
}

// A familyNames is the viewer and everyone in their families, each by
// address with the name the calendar says them by - the viewer's own
// blank, since a standing of the viewer's own is said as "you".
type familyNames map[string]string

func (h familyNames) has(email string) bool {
	_, ok := h[email]
	return ok
}

// firstName is how the calendar names a member of the familyNames: the
// first word of their name, or their address before the @ when the
// directory gives none.
func firstName(name, email string) string {
	if words := strings.Fields(name); len(words) > 0 {
		return words[0]
	}
	local, _, _ := strings.Cut(email, "@")
	return local
}

func (c calendarLinked) list(email string) []calendar.Linked {
	now := time.Now().In(calendar.Location)
	family := familyNames{email: ""}
	full := map[string]string{}
	adults, kids := c.directory.Household(email)
	for _, p := range append(append([]celebrate.Person{}, adults...), kids...) {
		full[p.Email] = p.Name
		if p.Email != email {
			family[p.Email] = firstName(p.Name, p.Email)
		}
	}
	if me, ok := c.directory.Person(email); ok && me.Name != "" {
		full[email] = me.Name
	}
	return append(c.parties(now, family, full), c.activities(family, full)...)
}

// nameOf is a household member in full, for a list: the directory's name,
// else the name a ticket carries, else the address before the @.
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

// standing is who in the familyNames the tickets or sign-ups belong to:
// nothing but "" when the viewer is among them, since that is said as
// "you"; else the members' names, each once, in the order found.
type standing struct {
	mine  bool
	names []string
}

func (s *standing) add(family familyNames, email, name string) {
	if email == "" {
		return
	}
	// The viewer's own entry is the blank one.
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

// A party is the familyNames's when someone in it bought a ticket or holds
// one; a waitlist request counts only while no ticket does.
func (c calendarLinked) parties(now time.Time, family familyNames, full map[string]string) []calendar.Linked {
	out := []calendar.Linked{}
	model := c.celebrate.Model()
	if model == nil {
		return out
	}
	for _, p := range model.SortedParties("") {
		if !p.VisibleTo("", false) || p.Start == "" {
			continue
		}
		// Whose the tickets are: the person named on each, else the buyer
		// - a guest still to be named is the buyer's.
		var going, waiting standing
		people := []calendar.Standing{}
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
				// A ticket names its holder; a guest still to be named is
				// listed by the words on the ticket.
				if family.has(t.Email) {
					people = append(people, calendar.Standing{Name: nameOf(full, t.Email, t.Name), Mine: family[t.Email] == ""})
				} else {
					people = append(people, calendar.Standing{Name: nameOf(full, "", t.Name), Note: "guest"})
				}
			case celebrate.TicketWaitlist:
				waiting.add(family, holder, t.Name)
				people = append(people, calendar.Standing{Name: nameOf(full, holder, t.Name), Note: "waitlisted"})
			}
		}
		mine, who := "", []string(nil)
		if going.mine || len(going.names) > 0 {
			mine, who = calendar.MineGoing, going.who()
		} else if waiting.mine || len(waiting.names) > 0 {
			mine, who = calendar.MineWaitlisted, waiting.who()
		}
		out = append(out, calendar.Linked{
			Source: calendar.SourceCelebrate, ID: p.ID, Title: p.Title, Summary: p.Summary, Description: p.Description, Location: p.Location,
			Start: p.Start, End: p.End, Path: model.PathOf(p), Availability: p.Availability(now), Mine: mine, Who: who, People: people, Image: p.ImageURL,
		})
	}
	return out
}

// activityImage is the picture HCA-Team's own page gives an event: its
// own, else its category's, as a path on that site.
func activityImage(model *team.Model, a *team.Activity) string {
	if a.ImageURL != "" {
		return a.ImageURL
	}
	if c := model.Category(a.Category); c != nil {
		return c.ImageURL
	}
	return ""
}

// An HCA-Team event is open to join until it is done or every spot is taken;
// the things under it stay off the calendar, since the event stands for them,
// and a sign-up on any of them makes the event the familyNames's.
func (c calendarLinked) activities(family familyNames, full map[string]string) []calendar.Linked {
	out := []calendar.Linked{}
	model := c.team.Model()
	if model == nil {
		return out
	}
	for _, a := range model.Activities {
		if !model.VisibleTo(a, "", false) || a.Start == "" {
			continue
		}
		availability := "open"
		switch {
		case a.Status == team.StatusDone:
			availability = "done"
		case a.VolunteersComplete || (a.Spots > 0 && len(a.Volunteers) >= a.Spots):
			availability = "full"
		}
		var signed standing
		people := []calendar.Standing{}
		for _, item := range append([]*team.Activity{a}, a.Descendants()...) {
			for _, v := range item.Volunteers {
				if !family.has(v.Email) {
					continue
				}
				signed.add(family, v.Email, "")
				// The part is the thing under the event they signed up for,
				// or their position on the event itself.
				note := v.Position
				if item != a {
					note = item.Title
				}
				people = append(people, calendar.Standing{Name: nameOf(full, v.Email, ""), Note: note, Mine: family[v.Email] == ""})
			}
		}
		mine, who := "", []string(nil)
		if signed.mine || len(signed.names) > 0 {
			mine, who = calendar.MineGoing, signed.who()
		}
		out = append(out, calendar.Linked{
			Source: calendar.SourceTeam, ID: a.ID, Title: a.Title, Description: a.Description, Location: a.Location,
			Start: a.Start, End: a.End, Path: model.PathOf(a), Availability: availability, Mine: mine, Who: who, People: people, Image: activityImage(model, a),
		})
	}
	return out
}

// partyPeople hands the calendar a party as its guest list needs it: the
// hosts, and every ticket holder and waitlist place, by address where the
// ticket names one.
type partyPeople struct {
	celebrate *celebrate.Cache
}

func (p partyPeople) people(id string) *calendar.PartyPeople {
	model := p.celebrate.Model()
	if model == nil {
		return nil
	}
	party := model.Party(id)
	if party == nil {
		return nil
	}
	out := &calendar.PartyPeople{Hosts: append([]string{}, party.HostEmails...), Attendees: []calendar.Attendee{}}
	for _, t := range party.Tickets {
		status := "ticket"
		if t.Status == celebrate.TicketWaitlist {
			status = "waitlist"
		} else if t.Price <= 0 {
			status = "free"
		}
		out.Attendees = append(out.Attendees, calendar.Attendee{Email: strings.ToLower(strings.TrimSpace(t.Email)), Name: t.Name, Status: status})
	}
	return out
}

// CalendarRoster is the directory's classrooms as the calendar resolves
// audiences against them: each with its band, the grades its students are
// in, and its crews - and the households, each address to the others in
// its families, adults first.
func CalendarRoster(m *who.Model) calendar.Roster {
	bandOf := map[string]string{}
	order := map[string]int{}
	for i, g := range m.Grades {
		bandOf[g.Name] = g.Band
		order[g.Name] = i
	}
	grades := map[string]map[string]bool{}
	for _, p := range m.People {
		if !p.IsStudent || p.Classroom == "" || p.Grade == "" {
			continue
		}
		if grades[p.Classroom] == nil {
			grades[p.Classroom] = map[string]bool{}
		}
		grades[p.Classroom][p.Grade] = true
	}
	crews := map[string][]string{}
	for _, c := range m.Crews {
		if c.Name != "" {
			crews[c.Classroom] = append(crews[c.Classroom], c.Name)
		}
	}
	roster := calendar.Roster{Classrooms: []calendar.Classroom{}, Households: map[string][]string{}, Parents: map[string][]string{}}
	for i := range m.People {
		email := m.People[i].Email
		seen := map[string]bool{email: true}
		for _, key := range m.FamilyKeysOf(email) {
			family := m.Families[key]
			for _, member := range append(append([]string{}, family.AdultEmails...), family.KidEmails...) {
				if member = m.Resolve(member); !seen[member] && m.Person(member) != nil {
					seen[member] = true
					roster.Households[email] = append(roster.Households[email], member)
					if m.People[i].IsStudent && slices.Contains(family.AdultEmails, member) {
						roster.Parents[email] = append(roster.Parents[email], member)
					}
				}
			}
		}
	}
	for _, c := range m.Classrooms {
		names := []string{}
		for g := range grades[c.Name] {
			names = append(names, g)
		}
		sort.Slice(names, func(i, j int) bool { return order[names[i]] < order[names[j]] })
		band := ""
		if len(names) > 0 {
			band = bandOf[names[0]]
		}
		roster.Classrooms = append(roster.Classrooms, calendar.Classroom{Name: c.Name, Band: band, Grades: names, Crews: crews[c.Name]})
	}
	return roster
}
