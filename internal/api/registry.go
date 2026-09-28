package api

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/id"
	"heliosian/internal/store"
)

const nowLayout = "2006-01-02 15:04"

type Config[S any] struct {
	Actor  func(r *http.Request, s S) access.Actor
	Held   func(email string) []access.Allowance
	Now    func() time.Time
	Queue  *store.Queue
	Staged func(tx *store.Tx) S
}

type Registry[S any] struct {
	config  Config[S]
	types   map[string]*Type[S]
	current atomic.Pointer[world[S]]
}

type world[S any] struct {
	s       S
	aliases map[string]map[string]string
}

func New[S any](config Config[S]) *Registry[S] {
	return &Registry[S]{config: config, types: map[string]*Type[S]{}}
}

func (reg *Registry[S]) Add(t Type[S]) {
	if _, ok := reg.types[t.Name]; ok {
		panic(fmt.Sprintf("api: type %q registered twice", t.Name))
	}
	for name, action := range t.Actions {
		if action.Can == nil || action.Do == nil {
			panic(fmt.Sprintf("api: %s action %q needs both a can rule and a route", t.Name, name))
		}
	}
	reg.types[t.Name] = &t
}

func (reg *Registry[S]) Relate(typeName, name string, rel Relation[S]) {
	t, ok := reg.types[typeName]
	if !ok {
		panic(fmt.Sprintf("api: relation %q on unregistered type %q", name, typeName))
	}
	if t.Relations == nil {
		t.Relations = map[string]Relation[S]{}
	}
	if _, ok := t.Relations[name]; ok {
		panic(fmt.Sprintf("api: %s relation %q declared twice", typeName, name))
	}
	t.Relations[name] = rel
}

func (reg *Registry[S]) Publish(s S) {
	w := &world[S]{s: s, aliases: map[string]map[string]string{}}
	for name, t := range reg.types {
		if t.Aliases == nil {
			continue
		}
		index := map[string]string{}
		for alias, target := range t.Aliases(s) {
			index[strings.ToLower(strings.TrimSpace(alias))] = target
		}
		w.aliases[name] = index
	}
	reg.current.Store(w)
}

func (reg *Registry[S]) Taken(candidate string) bool {
	w := reg.current.Load()
	return reg.takenIn(w, w.s)(candidate)
}

func (reg *Registry[S]) takenIn(w *world[S], s S) func(string) bool {
	return func(candidate string) bool {
		for name, t := range reg.types {
			if t.Has(s, candidate) {
				return true
			}
			if _, ok := w.aliases[name][candidate]; ok {
				return true
			}
		}
		return false
	}
}

func (w *world[S]) resolve(t *Type[S], segment string) (string, bool) {
	return w.resolveIn(w.s, t, segment)
}

func (w *world[S]) resolveIn(s S, t *Type[S], segment string) (string, bool) {
	if parsed, ok := id.Parse(segment); ok && t.Has(s, parsed) {
		return parsed, true
	}
	target, ok := w.aliases[t.Name][strings.ToLower(strings.TrimSpace(segment))]
	return target, ok
}

func (reg *Registry[S]) owner(w *world[S], candidate string) (*Type[S], bool) {
	for _, t := range reg.types {
		if t.Has(w.s, candidate) {
			return t, true
		}
	}
	return nil, false
}

func (reg *Registry[S]) query(r *http.Request, w *world[S]) Query {
	return Query{Actor: reg.config.Actor(r, w.s), Now: reg.config.Now()}
}
