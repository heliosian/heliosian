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

func (m *EmailLists) naming(tag string) []EmailList {
	out := []EmailList{}
	for _, g := range m.Groups {
		if slices.ContainsFunc(g.Rules, func(r Rule) bool { return slices.Contains(r.Tags, tag) }) {
			out = append(out, g)
		}
	}
	return out
}

func (m *EmailLists) namesItself(g EmailList) bool {
	seen := map[string]bool{}
	var reaches func(rules []Rule) bool
	reaches = func(rules []Rule) bool {
		for _, r := range rules {
			for _, tag := range r.Tags {
				key, ok := listRef(tag)
				if !ok {
					continue
				}
				other := m.Group(key)
				if other == nil {
					continue
				}
				if other.ID == g.ID {
					return true
				}
				if seen[other.ID] {
					continue
				}
				seen[other.ID] = true
				if reaches(other.Rules) {
					return true
				}
			}
		}
		return false
	}
	return reaches(g.Rules)
}
