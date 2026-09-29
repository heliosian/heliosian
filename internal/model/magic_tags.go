package model

import (
	"maps"
	"slices"
	"sort"
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

func (m *Model) MagicTagsOf(owner string, now time.Time) []MagicTag {
	tags := append(m.Directory.RoomParentTags(owner), m.Parties.MagicTags(m.Directory, owner, now)...)
	return append(tags, m.Activities.MagicTags(m.Directory, owner, now)...)
}

func (m *Model) EveryActivityMagicTagsOf(owner string, now time.Time) []MagicTag {
	tags := append(m.Directory.RoomParentTags(owner), m.Parties.MagicTags(m.Directory, owner, now)...)
	return append(tags, m.Activities.AllMagicTags(m.Directory, now)...)
}

func (m *Model) DirectoryAudience(now time.Time) AudienceSources {
	return AudienceSources{Directory: m.Directory, MagicTags: func(owner string) []MagicTag {
		return m.MagicTagsOf(owner, now)
	}}
}

func (m *Model) EmailListAudience(now time.Time) AudienceSources {
	activityAdmins := m.AdminList("team")
	return AudienceSources{Directory: m.Directory, MagicTags: func(owner string) []MagicTag {
		if activityAdmins.IsAdmin(owner) {
			return m.EveryActivityMagicTagsOf(owner, now)
		}
		return m.MagicTagsOf(owner, now)
	}}
}

func (m *Model) ManagedMagicTags(email string, now time.Time) []MagicTag {
	tags := append(m.Parties.MagicTags(m.Directory, email, now), m.Activities.MagicTags(m.Directory, email, now)...)
	return append(tags, m.EmailLists.MagicTags(m.EmailListAudience(now), email)...)
}

func PickerLists(directory *Directory, managed []MagicTag, email string) []PickerList {
	out := []PickerList{}
	for _, t := range directory.Tags(email) {
		out = append(out, PickerList{Key: TagKey(t.ID), Name: t.Name, Kind: "tag", People: t.People})
	}
	for _, t := range directory.SharedTags(email) {
		out = append(out, PickerList{Key: TagKey(t.ID), Name: t.Name + " (" + t.OwnerName + "'s)", Kind: "tag", People: t.People})
	}
	for _, list := range managed {
		if list.Archived {
			continue
		}
		out = append(out, PickerList{Key: list.Key, Name: list.Name, Kind: list.Kind, People: list.People})
	}
	return out
}

func (m *Model) MagicTagKeys() []string {
	out := []string{}
	for _, p := range m.Parties.Parties {
		out = append(out, MagicTagParty+":"+p.ID)
	}
	for _, a := range m.Activities.Activities {
		out = append(out, MagicTagActivity+":"+a.ID)
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
		out = append(out, MagicTag{Key: MagicTagRoom + ":" + label, Name: label + " Parents", Kind: MagicTagRoom, People: slices.Sorted(maps.Keys(people)), Guests: []Guest{}})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
