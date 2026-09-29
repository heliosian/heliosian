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

func MagicTagsOf(directory *Directory, parties *Parties, activities *Activities, owner string, now time.Time) []MagicTag {
	tags := append(directory.RoomParentTags(owner), parties.MagicTags(directory, owner, now)...)
	return append(tags, activities.MagicTags(directory, owner, now)...)
}

func EveryActivityMagicTagsOf(directory *Directory, parties *Parties, activities *Activities, owner string, now time.Time) []MagicTag {
	tags := append(directory.RoomParentTags(owner), parties.MagicTags(directory, owner, now)...)
	return append(tags, activities.AllMagicTags(directory, now)...)
}

func MagicTagKeys(parties *Parties, activities *Activities) []string {
	out := []string{}
	for _, p := range parties.Parties {
		out = append(out, MagicTagParty+":"+p.ID)
	}
	for _, a := range activities.Activities {
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
