package model

import (
	"slices"
	"strings"

	"heliosian/internal/api"
	"heliosian/internal/serve"
)

const toDosType = "to-dos"

type toDoSource struct {
	Title string `json:"title"`
	Date  string `json:"date"`
	Time  string `json:"time,omitempty"`
	To    string `json:"to"`
}

type toDoMe struct {
	State string `json:"state,omitempty"`
}

type toDoResource struct {
	Title   string     `json:"title"`
	Summary string     `json:"summary"`
	Details string     `json:"details"`
	Link    string     `json:"link,omitempty"`
	Due     string     `json:"due,omitempty"`
	Email   string     `json:"email,omitempty"`
	Point   int        `json:"point,omitempty"`
	Source  toDoSource `json:"source"`
	Me      toDoMe     `json:"me"`
}

func (m *Model) toDosFor(q api.Query) ([]string, map[string]*ToDo) {
	s := m.scope
	s.toDosOnce.Do(func() {
		s.toDoOrder, s.toDos = []string{}, map[string]*ToDo{}
		docs := m.Documents
		seats, grades := classSeatsOf(m.Directory, q.Actor.Email), m.Directory.GradeNames()
		states := m.Home.ToDoStates[q.Actor.Email]
		today := q.Now.In(Location).Format(DateFormat)
		shown := []*ToDo{}
		for _, t := range docs.ToDos {
			d := docs.byKey[t.Document]
			if d == nil {
				continue
			}
			reaches := forClassrooms(d, seats, docs.Audience[d.Key], grades) && d.School()
			if d.Kind == DocumentKindGroup {
				reaches = m.readsDocument(q, d)
			}
			if !reaches || (states[t.ID] != ToDoSaved && !docs.Current(t, today)) {
				continue
			}
			shown = append(shown, t)
		}
		slices.SortStableFunc(shown, func(a, b *ToDo) int {
			if (a.Due == "") != (b.Due == "") {
				if a.Due == "" {
					return 1
				}
				return -1
			}
			if c := strings.Compare(a.Due, b.Due); c != 0 {
				return c
			}
			if c := strings.Compare(docs.byKey[b.Document].Date, docs.byKey[a.Document].Date); c != 0 {
				return c
			}
			return strings.Compare(a.Title, b.Title)
		})
		for _, t := range shown {
			s.toDoOrder = append(s.toDoOrder, t.ID)
			s.toDos[t.ID] = t
		}
	})
	return s.toDoOrder, s.toDos
}

func (m *Model) toDoState(q api.Query, key string) (string, bool) {
	_, shown := m.toDosFor(q)
	if shown[key] == nil {
		return "", false
	}
	return m.Home.ToDoStates[q.Actor.Email][key], true
}

func (m *Model) DocumentSentTo(d *Document) string {
	if d.Kind != DocumentKindGroup {
		return sentTo(SchoolEmail{Kind: d.Kind, Channel: d.Channel, Audience: m.Documents.Audience[d.Key]})
	}
	if g := m.EmailLists.Named(d.Channel); g != nil {
		return g.Title
	}
	return d.Channel
}

func toDoStateIs(states ...string) func(m *Model, q api.Query, key string) bool {
	return func(m *Model, q api.Query, key string) bool {
		state, ok := m.toDoState(q, key)
		return ok && slices.Contains(states, state)
	}
}

func (a homeApp) toDosType() api.Type[*Model] {
	stage := a.store.stage
	set := func(state string) func(wr api.Write[*Model], _ serve.None) error {
		return func(wr api.Write[*Model], _ serve.None) error {
			ops := wr.S.Home.setToDoState(wr.Query.Actor, wr.ID, state, wr.Query.Now)
			logAfter(wr, "home: set a to-do", "to-do", wr.ID, "state", state)
			return stage(wr, homeAppName, ops, nil)
		}
	}
	return api.Type[*Model]{
		Name:  toDosType,
		Shape: toDoResource{},
		Has:   func(m *Model, key string) bool { return m.Documents.toDoIDs[key] != nil },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			_, shown := m.toDosFor(q)
			t := shown[key]
			if t == nil {
				return nil, false
			}
			d := m.Documents.byKey[t.Document]
			email := ""
			if d.School() {
				email = derived(m, kindSchoolEmail, d.Key)
			}
			return toDoResource{
				Title: t.Title, Summary: t.Summary, Details: t.Details, Link: t.Link, Due: t.Due, Email: email, Point: t.Point,
				Source: toDoSource{Title: d.Title, Date: d.Date, Time: d.Time, To: m.DocumentSentTo(d)},
				Me:     toDoMe{State: m.Home.ToDoStates[q.Actor.Email][key]},
			}, true
		},
		List: func(m *Model, q api.Query) []string {
			order, _ := m.toDosFor(q)
			return order
		},
		Actions: map[string]api.Action[*Model]{
			"complete": api.Do(toDoStateIs("", ToDoSaved), set(ToDoDone)),
			"save":     api.Do(toDoStateIs(""), set(ToDoSaved)),
			"clear":    api.Do(toDoStateIs(ToDoDone, ToDoSaved), set("")),
		},
	}
}
