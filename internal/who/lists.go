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
	Guests []Guest  `json:"guests"`
	Hosts  []string `json:"-"`
	// Archived is a group's list its manager has put away in Loop: kept
	// off Who?'s rail and filters for them, though its page still opens.
	Archived bool `json:"archived,omitempty"`
}

// Guest is someone on a list the directory does not hold: a party's ticket
// holder, keyed by the ticket, or a group's addition, keyed by the group and
// the address. Purchaser is a party guest's alone.
type Guest struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Email         string `json:"email,omitempty"`
	Purchaser     string `json:"purchaser,omitempty"`
	PurchaserName string `json:"purchaserName,omitempty"`
}

const (
	ListParty    = "party"
	ListActivity = "activity"
	ListRoom     = "room"
	ListGroup    = "group"
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
			for _, parent := range m.Parents(p.Email) {
				if parent.Email != email {
					people[parent.Email] = true
				}
			}
		}
		out = append(out, List{Key: ListRoom + ":" + label, Name: label + " Parents", Kind: ListRoom, People: slices.Sorted(maps.Keys(people)), Guests: []Guest{}})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
