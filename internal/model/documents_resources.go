package model

import (
	"heliosian/internal/api"
)

func (m *Documents) index(key []byte) {
	m.byKey, m.toDoIDs = map[string]*Document{}, map[string]*ToDo{}
	for _, t := range m.ToDos {
		t.ID = toDoID(key, t.Document, t.Title)
		m.toDoIDs[t.ID] = t
	}
	for _, d := range m.Documents {
		m.byKey[d.Key] = d
	}
}

func (m *Model) readsDocument(q api.Query, d *Document) bool {
	if d.Kind != DocumentKindGroup {
		return true
	}
	s := m.scope
	s.mailOnce.Do(func() {
		s.mailReadable = map[string]bool{}
		sources := m.listSources()
		for _, g := range m.EmailLists.Groups {
			if g.MailReadableBy(q.Actor.Email, sources) {
				s.mailReadable[g.Name] = true
			}
		}
	})
	return s.mailReadable[d.Channel]
}
