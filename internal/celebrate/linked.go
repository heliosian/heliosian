package celebrate

import (
	"maps"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/model"
)

func (m *Model) Linked(family model.Household, now time.Time) []model.Linked {
	out := []model.Linked{}
	for _, p := range m.SortedParties("") {
		if !p.VisibleTo(access.Actor{}) || p.Start == "" {
			continue
		}
		var going, waiting model.Circle
		people := []model.Standing{}
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
					people = append(people, model.Standing{Name: family.Name(t.Email, t.Name), Mine: family.Me(t.Email)})
				} else {
					people = append(people, model.Standing{Name: family.Name("", t.Name), Note: "guest"})
				}
			case TicketWaitlist:
				waiting.Add(family, holder, t.Name)
				people = append(people, model.Standing{Name: family.Name(holder, t.Name), Note: "waitlisted"})
			}
		}
		mine, names := "", []string(nil)
		if going.Any() {
			mine, names = model.MineGoing, going.Who()
		} else if waiting.Any() {
			mine, names = model.MineWaitlisted, waiting.Who()
		}
		out = append(out, model.Linked{
			Source: model.SourceCelebrate, ID: p.ID, EventID: p.ID, Title: p.Title, Summary: p.Summary, Description: p.Description, Location: p.Location,
			Start: p.Start, End: p.End, Path: m.PathOf(p), Availability: p.Availability(now), Mine: mine, Who: names, People: people, Image: p.ImageURL,
			Hosts: append([]string{}, p.HostEmails...),
		})
	}
	return out
}

func (m *Model) PartyPeople(id string) *model.PartyPeople {
	party := m.Party(id)
	if party == nil {
		return nil
	}
	out := &model.PartyPeople{Hosts: append([]string{}, party.HostEmails...), Attendees: []model.Attendee{}}
	for _, t := range party.Tickets {
		status := "ticket"
		if t.Status == TicketWaitlist {
			status = "waitlist"
		} else if t.Price <= 0 {
			status = "free"
		}
		out.Attendees = append(out.Attendees, model.Attendee{Email: strings.ToLower(strings.TrimSpace(t.Email)), Name: t.Name, Status: status})
	}
	return out
}

func (m *Model) Lists(directory *model.Directory, email string, now time.Time) []model.MagicTag {
	out := []model.MagicTag{}
	for _, p := range m.Parties {
		if p.Past(now) || !slices.ContainsFunc(p.HostEmails, func(h string) bool { return directory.Resolve(h) == email }) {
			continue
		}
		list := model.MagicTag{Key: model.MagicTagParty + ":" + p.ID, Name: p.Title, Kind: model.MagicTagParty, Guests: []model.Guest{}, Hosts: directory.ResolveAll(p.HostEmails)}
		people := map[string]bool{}
		for _, t := range p.Tickets {
			if t.Status != TicketSold {
				continue
			}
			holder := directory.Resolve(t.Email)
			buyer := directory.Resolve(t.Purchaser)
			known := directory.Person(buyer) != nil
			if t.Email == "" || directory.Person(holder) == nil {
				guest := model.Guest{ID: t.ID, Name: t.Name, Email: t.Email}
				if guest.Name == "" {
					guest.Name = DisplayName(t.Email)
				}
				if buyer != holder {
					guest.Purchaser = buyer
					if p := directory.Person(buyer); p != nil {
						guest.PurchaserName = p.FullName
					} else {
						guest.PurchaserName = DisplayName(buyer)
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
		slices.SortFunc(list.Guests, func(a, b model.Guest) int { return strings.Compare(a.Name, b.Name) })
		out = append(out, list)
	}
	return out
}

func (c *Cache) Pending(email string) []model.Approval {
	out := []model.Approval{}
	if !c.IsAdmin(email) {
		return out
	}
	m := c.Model()
	for _, p := range m.Parties {
		if p.Status == StatusPending {
			out = append(out, model.Approval{App: "celebrate", Title: p.Title, Start: p.Start, Path: m.PathOf(p)})
		}
	}
	return out
}
