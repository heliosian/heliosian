package loop

import (
	"slices"

	"heliosian/internal/model"
)

func (m *Model) Lists(sources Sources, email string) []model.MagicTag {
	out := []model.MagicTag{}
	for _, g := range m.Groups {
		if !g.Manages(email) {
			continue
		}
		people, guests := []string{}, []model.Guest{}
		for _, member := range Members(g, sources) {
			switch {
			case member == email:
			case sources.Directory.Person(member) != nil:
				people = append(people, member)
			default:
				name := member
				if added := g.Addition(member); added != nil && added.Name != "" {
					name = added.Name
				}
				guests = append(guests, model.Guest{ID: g.ID + ":" + member, Name: name, Email: member})
			}
		}
		out = append(out, model.MagicTag{Key: model.MagicTagGroup + ":" + g.ID, Slug: g.Name, Name: g.Title, Kind: model.MagicTagGroup, People: people, Guests: guests, Archived: m.Archived(g.ID, email)})
	}
	return out
}

func (m *Model) Tagged(tag string) string {
	for _, g := range m.Groups {
		for _, r := range g.Rules {
			if slices.Contains(r.Tags, tag) {
				return g.Name
			}
		}
	}
	return ""
}
