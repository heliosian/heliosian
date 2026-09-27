package ask

import (
	"strings"

	"heliosian/internal/who"
)

type linkCard struct {
	URL   string `json:"url"`
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Image string `json:"image,omitempty"`
	Badge string `json:"badge,omitempty"`
	Color string `json:"color,omitempty"`
}

func (v *viewer) linkCard(address string) (linkCard, bool) {
	switch {
	case strings.HasPrefix(address, whoBase+"/"):
		for i := range v.directory.People {
			p := &v.directory.People[i]
			if whoLink(p.Email) == address {
				return linkCard{URL: address, Kind: "person", Name: p.FullName, Image: v.directory.HeroPhoto(p.Email)}, true
			}
		}
		for key, family := range v.directory.Families {
			if whoBase+who.FamilyPath(key) == address {
				return linkCard{URL: address, Kind: "family", Name: family.Name, Image: family.PhotoURL}, true
			}
		}
		for _, c := range v.directory.Classrooms {
			if whoBase+who.ClassroomPath(c.Name) == address {
				card := linkCard{URL: address, Kind: "classroom", Name: c.Name, Color: v.sources.Settings().ClassroomColors[c.Name]}
				if c.ImageURL != "" {
					card.Image = whoBase + c.ImageURL
				}
				return card, true
			}
		}
	case strings.HasPrefix(address, whenBase+"/"):
		for _, e := range v.calendar.EventsFor(v.whenAs, v.directory, v.sources.Linked(v.email)) {
			if eventLink(e) != address {
				continue
			}
			return linkCard{URL: address, Kind: "event", Name: e.Title, Badge: e.StartTime().Format("Mon Jan 2")}, true
		}
	case strings.HasPrefix(address, loopBase+"/"):
		sources := v.sources.LoopSources()
		for _, g := range v.loop.Groups {
			if loopBase+g.Path() == address && g.VisibleTo(v.loopAs, sources) {
				return linkCard{URL: address, Kind: "group", Name: g.Title}, true
			}
		}
	case strings.HasPrefix(address, teamBase+"/"):
		a := v.team.Resolve(strings.TrimPrefix(address, teamBase))
		if a == nil || !v.team.VisibleTo(a, v.teamAs) {
			return linkCard{}, false
		}
		c := linkCard{URL: address, Kind: "activity", Name: a.Title}
		if a.ImageURL != "" {
			c.Image = teamBase + a.ImageURL
		}
		return c, true
	case strings.HasPrefix(address, celebrateBase+"/"):
		p := v.celebrate.Resolve(strings.TrimPrefix(address, celebrateBase))
		if p == nil || !p.VisibleTo(v.partyAs) {
			return linkCard{}, false
		}
		c := linkCard{URL: address, Kind: "party", Name: p.Title}
		if p.ImageURL != "" {
			c.Image = celebrateBase + p.ImageURL
		}
		return c, true
	}
	return linkCard{}, false
}
