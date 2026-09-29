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

func DirectoryAudience(directory *Directory, parties *Parties, activities *Activities, now time.Time) AudienceSources {
	return AudienceSources{Directory: directory, MagicTags: func(owner string) []MagicTag {
		return MagicTagsOf(directory, parties, activities, owner, now)
	}}
}

func EmailListAudience(directory *Directory, parties *Parties, activities *Activities, activityAdmins *ActivitiesCache, now time.Time) AudienceSources {
	return AudienceSources{Directory: directory, MagicTags: func(owner string) []MagicTag {
		if activityAdmins.IsAdmin(owner) {
			return EveryActivityMagicTagsOf(directory, parties, activities, owner, now)
		}
		return MagicTagsOf(directory, parties, activities, owner, now)
	}}
}

func ManagedMagicTags(directory *Directory, parties *Parties, activities *Activities, lists *EmailLists, activityAdmins *ActivitiesCache, email string, now time.Time) []MagicTag {
	tags := append(parties.MagicTags(directory, email, now), activities.MagicTags(directory, email, now)...)
	return append(tags, lists.MagicTags(EmailListAudience(directory, parties, activities, activityAdmins, now), email)...)
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
