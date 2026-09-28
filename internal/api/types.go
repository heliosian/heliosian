package api

import (
	"net/http"
	"time"

	"heliosian/internal/access"
)

type Query struct {
	Actor access.Actor
	Now   time.Time
}

type Type[S any] struct {
	Name      string
	Has       func(s S, id string) bool
	Get       func(s S, q Query, id string) (any, bool)
	List      func(s S, q Query) []string
	Aliases   func(s S) map[string]string
	Relations map[string]Relation[S]
	Filters   map[string]Filter[S]
	Actions   map[string]Action[S]
	Create    func(r *http.Request, s S, q Query) (string, error)
}

type Relation[S any] struct {
	Type string
	Many bool
	List func(s S, q Query, id string) []string
}

type Filter[S any] func(s S, q Query, value string) (func(id string) bool, error)

type Action[S any] struct {
	Can func(s S, q Query, id string) bool
	Do  func(r *http.Request, s S, q Query, id string) error
}

func Lift[S, M any](t Type[M], of func(S) M) Type[S] {
	out := Type[S]{
		Name:      t.Name,
		Has:       func(s S, id string) bool { return t.Has(of(s), id) },
		Get:       func(s S, q Query, id string) (any, bool) { return t.Get(of(s), q, id) },
		List:      func(s S, q Query) []string { return t.List(of(s), q) },
		Relations: map[string]Relation[S]{},
		Filters:   map[string]Filter[S]{},
		Actions:   map[string]Action[S]{},
	}
	if t.Aliases != nil {
		out.Aliases = func(s S) map[string]string { return t.Aliases(of(s)) }
	}
	for name, rel := range t.Relations {
		out.Relations[name] = LiftRelation(rel, of)
	}
	for name, filter := range t.Filters {
		out.Filters[name] = func(s S, q Query, value string) (func(string) bool, error) { return filter(of(s), q, value) }
	}
	for name, action := range t.Actions {
		out.Actions[name] = Action[S]{
			Can: func(s S, q Query, id string) bool { return action.Can(of(s), q, id) },
			Do:  func(r *http.Request, s S, q Query, id string) error { return action.Do(r, of(s), q, id) },
		}
	}
	if t.Create != nil {
		out.Create = func(r *http.Request, s S, q Query) (string, error) { return t.Create(r, of(s), q) }
	}
	return out
}

func LiftRelation[S, M any](rel Relation[M], of func(S) M) Relation[S] {
	return Relation[S]{Type: rel.Type, Many: rel.Many, List: func(s S, q Query, id string) []string { return rel.List(of(s), q, id) }}
}
