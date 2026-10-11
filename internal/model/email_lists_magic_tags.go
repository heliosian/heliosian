package model

import (
	"slices"
)

func (m *EmailLists) MagicTags(sources AudienceSources, email string) []MagicTag {
	out := []MagicTag{}
	for _, g := range m.Groups {
		if !g.Manages(email) {
			continue
		}
		people, guests := []string{}, []Guest{}
		for _, member := range g.Members(sources) {
			switch {
			case member == email:
			case sources.Directory.Person(member) != nil:
				people = append(people, member)
			default:
				name := member
				if added := g.Addition(member); added != nil && added.Name != "" {
					name = added.Name
				}
				guests = append(guests, Guest{ID: g.ID + ":" + member, Name: name, Email: member})
			}
		}
		out = append(out, MagicTag{Key: MagicTagGroup + ":" + g.ID, Slug: g.Name, Name: g.Title, Kind: MagicTagGroup, People: people, Guests: guests, Hosts: slices.Clone(g.Managers), Archived: m.Archived(g.ID, email)})
	}
	return out
}
