package model

import (
	"maps"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
)

func (m *Parties) Linked(family Household, now time.Time) []Linked {
	out := []Linked{}
	for _, p := range m.SortedParties("") {
		if !p.VisibleTo(access.Actor{}) || p.Start == "" {
			continue
		}
		var going, waiting Circle
		people := []Standing{}
		for _, t := range p.Tickets {
			if !family.Has(t.Purchaser) && !family.Has(t.Email) {
				continue
			}
			holder := t.Email
			if !family.Has(holder) {
				holder = t.Purchaser
			}
			switch t.Status {
			case TicketSold:
				going.Add(family, holder, t.Name)
				if family.Has(t.Email) {
					people = append(people, Standing{Name: family.Name(t.Email, t.Name), Mine: family.Me(t.Email)})
				} else {
					people = append(people, Standing{Name: family.Name("", t.Name), Note: "guest"})
				}
			case TicketWaitlist:
				waiting.Add(family, holder, t.Name)
				people = append(people, Standing{Name: family.Name(holder, t.Name), Note: "waitlisted"})
			}
		}
		mine, names := "", []string(nil)
		if going.Any() {
			mine, names = MineGoing, going.Who()
		} else if waiting.Any() {
			mine, names = MineWaitlisted, waiting.Who()
		}
		out = append(out, Linked{
			Source: SourceCelebrate, ID: p.ID, EventID: p.ID, Title: p.Title, Summary: p.Summary, Description: p.Description, Location: p.Location,
			Start: p.Start, End: p.End, Path: m.PathOf(p), Availability: p.Availability(now), Mine: mine, Who: names, People: people, Image: p.ImageURL,
			Hosts: append([]string{}, p.HostEmails...),
		})
	}
	return out
}

func (m *Parties) PartyPeople(id string) *PartyPeople {
	party := m.Party(id)
	if party == nil {
		return nil
	}
	out := &PartyPeople{Hosts: append([]string{}, party.HostEmails...), Attendees: []Attendee{}}
	for _, t := range party.Tickets {
		status := "ticket"
		if t.Status == TicketWaitlist {
			status = "waitlist"
		} else if t.Price <= 0 {
			status = "free"
		}
		out.Attendees = append(out.Attendees, Attendee{Email: strings.ToLower(strings.TrimSpace(t.Email)), Name: t.Name, Status: status})
	}
	return out
}

func (m *Parties) MagicTags(directory *Directory, email string, now time.Time) []MagicTag {
	out := []MagicTag{}
	for _, p := range m.Parties {
		if !slices.ContainsFunc(p.HostEmails, func(h string) bool { return directory.Resolve(h) == email }) {
			continue
		}
		list := MagicTag{Key: MagicTagParty + ":" + p.ID, Name: p.Title, Kind: MagicTagParty, Guests: []Guest{}, Hosts: directory.ResolveAll(p.HostEmails), Archived: p.Past(now)}
		people := map[string]bool{}
		for _, t := range p.Tickets {
			if t.Status != TicketSold {
				continue
			}
			holder := directory.Resolve(t.Email)
			buyer := directory.Resolve(t.Purchaser)
			known := directory.Person(buyer) != nil
			if t.Email == "" || directory.Person(holder) == nil {
				guest := Guest{ID: t.ID, Name: t.Name, Email: t.Email}
				if guest.Name == "" {
					guest.Name = cells.DisplayName(t.Email)
				}
				if buyer != holder {
					guest.Purchaser = buyer
					if p := directory.Person(buyer); p != nil {
						guest.PurchaserName = p.FullName
					} else {
						guest.PurchaserName = cells.DisplayName(buyer)
					}
				}
				list.Guests = append(list.Guests, guest)
			} else {
				people[holder] = true
			}
			if known {
				people[buyer] = true
			}
		}
		for _, host := range list.Hosts {
			if directory.Person(host) != nil {
				people[host] = true
			}
		}
		list.People = slices.Sorted(maps.Keys(people))
		slices.SortFunc(list.Guests, func(a, b Guest) int { return strings.Compare(a.Name, b.Name) })
		out = append(out, list)
	}
	return out
}

func (c *PartiesCache) Pending(email string) []Approval {
	out := []Approval{}
	if !c.IsAdmin(email) {
		return out
	}
	m := c.Model()
	for _, p := range m.Parties {
		if p.Status == StatusPending {
			out = append(out, Approval{App: "celebrate", Title: p.Title, Start: p.Start, Path: m.PathOf(p)})
		}
	}
	return out
}
