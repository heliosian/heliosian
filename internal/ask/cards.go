package ask

import (
	"strings"

	"heliosian/internal/celebrate"
)

type linkCard struct {
	URL   string `json:"url"`
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Image string `json:"image,omitempty"`
}

func (v *viewer) linkCard(address string) (linkCard, bool) {
	switch {
	case strings.HasPrefix(address, whoBase+"/people/"):
		for i := range v.directory.People {
			p := &v.directory.People[i]
			if whoLink(p.Email) == address {
				return linkCard{URL: address, Kind: "person", Name: p.FullName, Image: v.directory.HeroPhoto(p.Email)}, true
			}
		}
	case strings.HasPrefix(address, whoBase+"/families/"):
		key := strings.TrimPrefix(address, whoBase+"/families/")
		if family, ok := v.directory.Families[key]; ok {
			return linkCard{URL: address, Kind: "family", Name: family.Name, Image: family.PhotoURL}, true
		}
	case strings.HasPrefix(address, teamBase+"/") && v.team != nil:
		a := v.team.Resolve(strings.TrimPrefix(address, teamBase))
		if a == nil || !v.visibleActivity(a) {
			return linkCard{}, false
		}
		c := linkCard{URL: address, Kind: "activity", Name: a.Title}
		if a.ImageURL != "" {
			c.Image = teamBase + a.ImageURL
		}
		return c, true
	case strings.HasPrefix(address, celebrateBase+"/") && v.celebrate != nil:
		p := v.celebrate.Resolve(strings.TrimPrefix(address, celebrateBase))
		if p == nil || (p.Status != celebrate.StatusOpen && !p.Hosted(v.email)) {
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
