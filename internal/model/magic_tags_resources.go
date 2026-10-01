package model

import (
	"strings"

	"heliosian/internal/api"
	"heliosian/internal/id"
)

const kindMagicTag = "magic-tag"

type magicTagResource struct {
	Key      string  `json:"key"`
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	Slug     string  `json:"slug,omitempty"`
	Archived bool    `json:"archived"`
	Guests   []Guest `json:"guests"`
	Path     string  `json:"path"`
	App      string  `json:"app"`
}

func (m *Model) magicTagID(key string) string {
	return id.Of(m.EmailLists.idKey, kindMagicTag, key)
}

func (m *Model) magicTagKnown(key string) bool {
	s := m.scope
	s.magicKeysOnce.Do(func() {
		s.magicKeys = map[string]bool{}
		for _, k := range m.magicTagKeys() {
			s.magicKeys[m.magicTagID(k)] = true
		}
	})
	return s.magicKeys[key]
}

func (m *Model) heldTags(q api.Query) ([]string, map[string]MagicTag) {
	s := m.scope
	s.heldOnce.Do(func() {
		s.heldOrder, s.held = []string{}, map[string]MagicTag{}
		for _, t := range m.MagicTagsOf(q.Actor.Email, q.Now) {
			key := m.magicTagID(t.Key)
			s.heldOrder = append(s.heldOrder, key)
			s.held[key] = t
		}
	})
	return s.heldOrder, s.held
}

func (m *Model) heldTag(q api.Query, key string) (MagicTag, bool) {
	_, held := m.heldTags(q)
	t, ok := held[key]
	return t, ok
}

func (m *Model) peopleIDs(emails []string) []string {
	out := []string{}
	for _, email := range emails {
		out = append(out, m.personID(email)...)
	}
	return out
}

func sourceOf(kind string) func(m *Model, q api.Query, key string) []string {
	return func(m *Model, q api.Query, key string) []string {
		t, ok := m.heldTag(q, key)
		if !ok || t.Kind != kind {
			return nil
		}
		_, rest, _ := strings.Cut(t.Key, ":")
		if kind == MagicTagParty {
			return []string{id.Of(m.EmailLists.idKey, kindParty, rest)}
		}
		return []string{rest}
	}
}

func MagicTagResources() []api.Type[*Model] {
	return []api.Type[*Model]{{
		Name:  "magic-tags",
		Shape: magicTagResource{},
		Has:   func(m *Model, key string) bool { return m.magicTagKnown(key) },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			t, ok := m.heldTag(q, key)
			if !ok {
				return nil, false
			}
			return magicTagResource{Key: t.Key, Name: t.Name, Kind: t.Kind, Slug: t.Slug, Archived: t.Archived, Guests: t.Guests, Path: ListPath(t.Key), App: whoHost}, true
		},
		List: func(m *Model, q api.Query) []string {
			order, _ := m.heldTags(q)
			return order
		},
		Aliases: func(m *Model) map[string]string {
			out := map[string]string{}
			for _, k := range m.magicTagKeys() {
				out[k] = m.magicTagID(k)
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"people": {Type: "people", Many: true, List: func(m *Model, q api.Query, key string) []string {
				t, ok := m.heldTag(q, key)
				if !ok {
					return nil
				}
				return m.peopleIDs(t.People)
			}},
			"holders": {Type: "people", Many: true, List: func(m *Model, q api.Query, key string) []string {
				t, ok := m.heldTag(q, key)
				if !ok {
					return nil
				}
				return m.peopleIDs(t.Hosts)
			}},
			"parent": {Type: "magic-tags", List: func(m *Model, q api.Query, key string) []string {
				t, ok := m.heldTag(q, key)
				if !ok || t.Parent == "" {
					return nil
				}
				return []string{m.magicTagID(t.Parent)}
			}},
			"email-lists": {Type: "email-lists", Many: true, List: func(m *Model, q api.Query, key string) []string {
				t, ok := m.heldTag(q, key)
				if !ok {
					return nil
				}
				out := []string{}
				for _, g := range m.EmailLists.naming(t.Key) {
					if m.listVisible(g, q.Actor) {
						out = append(out, g.ID)
					}
				}
				return out
			}},
			"party":      {Type: "parties", List: sourceOf(MagicTagParty)},
			"activity":   {Type: "activities", List: sourceOf(MagicTagActivity)},
			"email-list": {Type: "email-lists", List: sourceOf(MagicTagGroup)},
		},
	}}
}
