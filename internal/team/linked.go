package team

import (
	"maps"
	"slices"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/model"
	"heliosian/internal/when"
)

func (m *Model) imageOf(a *Activity) string {
	if a.ImageURL != "" {
		return a.ImageURL
	}
	if c := m.Category(a.Category); c != nil {
		return c.ImageURL
	}
	return ""
}

func (m *Model) Linked(family when.Family) []when.Linked {
	out := []when.Linked{}
	for _, raw := range m.Activities {
		if !m.VisibleTo(raw, access.Actor{}) || raw.Start == "" {
			continue
		}
		a := m.ActivityFor(raw, family.Actor)
		availability := "open"
		switch {
		case a.Status == StatusDone:
			availability = "done"
		case a.VolunteersComplete || (a.Spots > 0 && a.Taken >= a.Spots):
			availability = "full"
		}
		var signed when.Circle
		people := []when.Standing{}
		for _, item := range append([]*Activity{a}, a.Descendants()...) {
			for _, v := range item.Volunteers {
				if !family.Has(v.Email) {
					continue
				}
				signed.Add(family, v.Email, "")
				note := v.Position
				if item != a {
					note = item.Title
				}
				people = append(people, when.Standing{Name: family.Name(v.Email, ""), Note: note, Mine: family.Me(v.Email)})
			}
		}
		mine, names := "", []string(nil)
		if signed.Any() {
			mine, names = when.MineGoing, signed.Who()
		}
		out = append(out, when.Linked{
			Source: when.SourceTeam, ID: a.ID, EventID: raw.CalendarEventID, Title: a.Title, Description: a.Description, Location: a.Location,
			Start: a.Start, End: a.End, Path: m.PathOf(a), Availability: availability, Mine: mine, Who: names, People: people, Image: m.imageOf(a),
			Hosts: a.CoChairs(),
		})
	}
	return out
}

func (m *Model) Lists(directory *model.Directory, email string, now time.Time) []model.MagicTag {
	out := []model.MagicTag{}
	chairs := func(a *Activity) bool {
		return slices.ContainsFunc(a.CoChairs(), func(c string) bool { return directory.Resolve(c) == email })
	}
	year := SchoolYear(now)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	over := func(a *Activity) bool {
		if a.Status == StatusDone {
			return true
		}
		cell := a.End
		if cell == "" {
			cell = a.Start
		}
		if cell == "" {
			return false
		}
		last, _ := cells.When(cell)
		return last.Before(today)
	}
	var walk func(a, root *Activity, parent string)
	walk = func(a, root *Activity, parent string) {
		if over(a) {
			return
		}
		key := ""
		if parent != "" || chairs(a) {
			key = model.MagicTagActivity + ":" + a.ID
			list := model.MagicTag{Key: key, Name: a.Title, Kind: model.MagicTagActivity, Parent: parent, Guests: []model.Guest{}, Hosts: directory.ResolveAll(a.CoChairs())}
			if a != root {
				list.Name = root.Title + ": " + a.Title
			}
			people := map[string]bool{}
			for _, node := range append([]*Activity{a}, a.Descendants()...) {
				for _, v := range node.Volunteers {
					if person := directory.Resolve(v.Email); directory.Person(person) != nil {
						people[person] = true
					}
				}
			}
			for _, host := range list.Hosts {
				if directory.Person(host) != nil {
					people[host] = true
				}
			}
			list.People = slices.Sorted(maps.Keys(people))
			out = append(out, list)
		}
		for _, c := range a.Children {
			walk(c, root, key)
		}
	}
	for _, a := range m.Activities {
		if a.Year == year {
			walk(a, a, "")
		}
	}
	return out
}

func (c *Cache) Pending(email string) []model.Approval {
	out := []model.Approval{}
	if !c.IsAdmin(email) {
		return out
	}
	m := c.Model()
	var walk func([]*Activity)
	walk = func(list []*Activity) {
		for _, a := range list {
			if a.Status == StatusPending {
				out = append(out, model.Approval{App: "team", Title: a.Title, Start: a.Start, Path: m.PathOf(a)})
			}
			walk(a.Children)
		}
	}
	walk(m.Activities)
	return out
}
