package model

import (
	"maps"
	"slices"
	"sort"
	"strings"
	"time"
)

type MagicTag struct {
	Key      string   `json:"key"`
	Slug     string   `json:"slug,omitempty"`
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	People   []string `json:"people"`
	Guests   []Guest  `json:"guests"`
	Hosts    []string `json:"-"`
	Parent   string   `json:"parent,omitempty"`
	Archived bool     `json:"archived,omitempty"`
}

type Guest struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Email         string `json:"email,omitempty"`
	Purchaser     string `json:"purchaser,omitempty"`
	PurchaserName string `json:"purchaserName,omitempty"`
}

const (
	MagicTagParty    = "party"
	MagicTagActivity = "activity"
	MagicTagRoom     = "room"
	MagicTagGroup    = "group"
)

func (m *Model) heldMagicTags(holder string, now time.Time) []MagicTag {
	tags := append(m.Directory.RoomParentTags(holder), m.Parties.MagicTags(m.Directory, holder, now)...)
	if m.AdminList("team").IsAdmin(holder) {
		return append(tags, m.Activities.AllMagicTags(m.Directory, now)...)
	}
	return append(tags, m.Activities.MagicTags(m.Directory, holder, now)...)
}

func (m *Model) Audience(now time.Time) AudienceSources {
	return AudienceSources{Directory: m.Directory, MagicTags: func(holder string) []MagicTag {
		return m.heldMagicTags(holder, now)
	}, EmailLists: m.EmailLists}
}

func (m *Model) MagicTagsOf(holder string, now time.Time) []MagicTag {
	return append(m.heldMagicTags(holder, now), m.EmailLists.MagicTags(m.Audience(now), holder)...)
}

func (m *Model) magicTagExists(key string) bool {
	kind, rest, _ := strings.Cut(key, ":")
	switch kind {
	case MagicTagParty:
		return m.Parties.byParty[rest] != nil
	case MagicTagActivity:
		return m.Activities.byID[rest] != nil
	case MagicTagRoom:
		_, ok := m.Directory.RoomParents[rest]
		return ok
	case MagicTagGroup:
		return m.EmailLists.Group(rest) != nil
	}
	return false
}

func (m *Model) magicTagKeys() []string {
	out := []string{}
	for _, p := range m.Parties.Parties {
		out = append(out, MagicTagParty+":"+p.ID)
	}
	for key := range m.Activities.byID {
		out = append(out, MagicTagActivity+":"+key)
	}
	for band := range m.Directory.RoomParents {
		out = append(out, MagicTagRoom+":"+band)
	}
	for _, g := range m.EmailLists.Groups {
		out = append(out, MagicTagGroup+":"+g.ID)
	}
	return out
}

func (m *Directory) RoomParentsOf(band string) []string {
	return m.RoomParents[bandLabel(band)]
}

func (m *Directory) RoomParentTags(email string) []MagicTag {
	out := []MagicTag{}
	for label, parents := range m.RoomParents {
		if !slices.Contains(parents, email) {
			continue
		}
		people := map[string]bool{}
		for _, p := range m.People {
			if !p.IsStudent || bandLabel(gradeBands[p.Grade]) != label {
				continue
			}
			for _, parent := range m.Parents(p.Email) {
				if parent.Email != email {
					people[parent.Email] = true
				}
			}
		}
		out = append(out, MagicTag{Key: MagicTagRoom + ":" + label, Name: label + " Parents", Kind: MagicTagRoom, People: slices.Sorted(maps.Keys(people)), Guests: []Guest{}, Hosts: slices.Clone(parents)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
