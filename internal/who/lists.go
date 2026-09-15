package who

import (
	"maps"
	"slices"
	"sort"
)

type List struct {
	Key    string   `json:"key"`
	Name   string   `json:"name"`
	Kind   string   `json:"kind"`
	People []string `json:"people"`
	Guests int      `json:"guests,omitempty"`
}

const (
	ListParty    = "party"
	ListActivity = "activity"
	ListRoom     = "room"
)

type Lister interface {
	Lists(email string) []List
}

func (m *Model) RoomParentLists(email string) []List {
	out := []List{}
	for label, parents := range m.RoomParents {
		if !slices.Contains(parents, email) {
			continue
		}
		people := map[string]bool{}
		for _, p := range m.People {
			if !p.IsStudent || bandLabel(gradeBands[p.Grade]) != label {
				continue
			}
			for _, key := range m.FamilyKeysOf(p.Email) {
				for _, adult := range m.Families[key].AdultEmails {
					if adult != email && m.Person(adult) != nil {
						people[adult] = true
					}
				}
			}
		}
		out = append(out, List{Key: ListRoom + ":" + label, Name: label + " Parents", Kind: ListRoom, People: slices.Sorted(maps.Keys(people))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
