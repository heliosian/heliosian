package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

type Query struct {
	Actor access.Actor
	Now   time.Time
}

type Write[S any] struct {
	Request *http.Request
	Tx      *store.Tx
	S       S
	Query   Query
	ID      string
	Body    json.RawMessage
	Taken   func(string) bool
}

func decode(body json.RawMessage, into any) error {
	if len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, into); err != nil {
		return access.Invalid("bad request body: %v", err)
	}
	return nil
}

type Type[S any] struct {
	Name      string
	Shape     any
	Has       func(s S, id string) bool
	Get       func(s S, q Query, id string) (any, bool)
	List      func(s S, q Query) []string
	Aliases   func(s S) map[string]string
	Relations map[string]Relation[S]
	Filters   map[string]Filter[S]
	Actions   map[string]Action[S]
	Create    Maker[S]
}

type Relation[S any] struct {
	Type string
	Many bool
	List func(s S, q Query, id string) []string
}

type Filter[S any] func(s S, q Query, value string) (func(id string) bool, error)

type Action[S any] struct {
	Can   func(s S, q Query, id string) bool
	do    func(w Write[S]) (string, error)
	input reflect.Type
	makes bool
}

func Do[S, In any](can func(s S, q Query, id string) bool, do func(w Write[S], in In) error) Action[S] {
	return DoFrom(can, func(Write[S]) In { var in In; return in }, do)
}

func DoFrom[S, In any](can func(s S, q Query, id string) bool, seed func(w Write[S]) In, do func(w Write[S], in In) error) Action[S] {
	return Action[S]{Can: can, input: reflect.TypeFor[In](), do: func(w Write[S]) (string, error) {
		in := seed(w)
		if err := decode(w.Body, &in); err != nil {
			return "", err
		}
		return "", do(w, in)
	}}
}

func DoMaking[S, In any](can func(s S, q Query, id string) bool, do func(w Write[S], in In) (string, error)) Action[S] {
	return Action[S]{Can: can, input: reflect.TypeFor[In](), makes: true, do: func(w Write[S]) (string, error) {
		var in In
		if err := decode(w.Body, &in); err != nil {
			return "", err
		}
		return do(w, in)
	}}
}

type Maker[S any] struct {
	do    func(w Write[S]) (string, error)
	input reflect.Type
}

func Make[S, In any](do func(w Write[S], in In) (string, error)) Maker[S] {
	return Maker[S]{input: reflect.TypeFor[In](), do: func(w Write[S]) (string, error) {
		var in In
		if err := decode(w.Body, &in); err != nil {
			return "", err
		}
		return do(w, in)
	}}
}
