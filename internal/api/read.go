package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/id"
)

type object map[string]json.RawMessage

type envelope struct {
	Now       string                       `json:"now"`
	Result    any                          `json:"result"`
	Resources map[string]map[string]object `json:"resources"`
}

type tree map[string]tree

type reader[S any] struct {
	reg       *Registry[S]
	w         *world[S]
	s         S
	q         Query
	resources map[string]map[string]object
}

func (reg *Registry[S]) reader(w *world[S], q Query) *reader[S] {
	return &reader[S]{reg: reg, w: w, s: reg.config.Scope(w.s, q), q: q, resources: map[string]map[string]object{}}
}

func (rd *reader[S]) envelope(result any) envelope {
	return envelope{Now: rd.q.Now.Format(nowLayout), Result: result, Resources: rd.resources}
}

func (rd *reader[S]) read(target *url.URL) (any, error) {
	rest, ok := strings.CutPrefix(target.Path, "/api/")
	if !ok {
		return nil, access.Invalid("%s is not a resource path", target.Path)
	}
	params := target.Query()
	includes, err := parseIncludes(params["include"])
	if err != nil {
		return nil, err
	}
	parts := strings.Split(rest, "/")
	switch {
	case len(parts) == 2 && parts[0] == "r":
		parsed, ok := id.Parse(parts[1])
		if !ok {
			return nil, access.Missing("nothing has the ID %s", parts[1])
		}
		t, ok := rd.reg.owner(rd.s, parsed)
		if !ok {
			return nil, access.Missing("nothing has the ID %s", parts[1])
		}
		return rd.one(t, parsed, includes)
	case len(parts) == 1:
		t, err := rd.reg.typeNamed(parts[0])
		if err != nil {
			return nil, err
		}
		return rd.collection(t, params, includes)
	case len(parts) == 2:
		t, err := rd.reg.typeNamed(parts[0])
		if err != nil {
			return nil, err
		}
		resolved, ok := rd.w.resolveIn(rd.s, t, parts[1])
		if !ok {
			return nil, access.Missing("no %s %s", t.Name, parts[1])
		}
		return rd.one(t, resolved, includes)
	}
	return nil, access.Missing("%s is not a resource path", target.Path)
}

func (reg *Registry[S]) typeNamed(name string) (*Type[S], error) {
	t, ok := reg.types[name]
	if !ok {
		return nil, access.Missing("no resource type %s", name)
	}
	return t, nil
}

func parseIncludes(values []string) (tree, error) {
	out := tree{}
	for _, value := range values {
		for path := range strings.SplitSeq(value, ",") {
			if path == "" {
				continue
			}
			node := out
			for name := range strings.SplitSeq(path, ".") {
				if name == "" {
					return nil, access.Invalid("include %q has an empty step", path)
				}
				if node[name] == nil {
					node[name] = tree{}
				}
				node = node[name]
			}
		}
	}
	return out, nil
}

func (rd *reader[S]) one(t *Type[S], key string, includes tree) (any, error) {
	obj, ok, err := rd.object(t, key)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, access.Missing("no %s %s", t.Name, key)
	}
	if err := rd.expand(t, key, obj, includes); err != nil {
		return nil, err
	}
	return key, nil
}

func (rd *reader[S]) collection(t *Type[S], params url.Values, includes tree) (any, error) {
	keep := []func(string) (bool, error){}
	for name, values := range params {
		if name == "include" {
			continue
		}
		for _, value := range values {
			pred, err := rd.filter(t, name, value)
			if err != nil {
				return nil, err
			}
			keep = append(keep, pred)
		}
	}
	out := []string{}
	for _, key := range t.List(rd.s, rd.q) {
		kept, err := rd.passes(keep, key)
		if err != nil {
			return nil, err
		}
		if !kept {
			continue
		}
		obj, ok, err := rd.object(t, key)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if err := rd.expand(t, key, obj, includes); err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, nil
}

func (rd *reader[S]) passes(keep []func(string) (bool, error), key string) (bool, error) {
	for _, pred := range keep {
		ok, err := pred(key)
		if err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}

func (rd *reader[S]) filter(t *Type[S], name, value string) (func(string) (bool, error), error) {
	switch name {
	case "can":
		action, ok := t.Actions[value]
		if !ok {
			return nil, access.Invalid("%s has no action %s", t.Name, value)
		}
		return func(key string) (bool, error) { return action.Can(rd.s, rd.q, key), nil }, nil
	case "mine":
		if value != "" && value != "true" {
			return nil, access.Invalid("mine takes no value")
		}
		return func(key string) (bool, error) { return rd.mine(t, key) }, nil
	}
	filter, ok := t.Filters[name]
	if !ok {
		return nil, access.Invalid("%s has no filter %s", t.Name, name)
	}
	pred, err := filter(rd.s, rd.q, value)
	if err != nil {
		return nil, err
	}
	return func(key string) (bool, error) { return pred(key), nil }, nil
}

func (rd *reader[S]) mine(t *Type[S], key string) (bool, error) {
	obj, ok, err := rd.object(t, key)
	if err != nil || !ok || obj["me"] == nil {
		return false, err
	}
	var me struct {
		Mine bool `json:"mine"`
	}
	if err := json.Unmarshal(obj["me"], &me); err != nil {
		return false, err
	}
	return me.Mine, nil
}

func (rd *reader[S]) object(t *Type[S], key string) (object, bool, error) {
	if obj, ok := rd.resources[t.Name][key]; ok {
		return obj, true, nil
	}
	v, ok := t.Get(rd.s, rd.q, key)
	if !ok {
		return nil, false, nil
	}
	if reflect.TypeOf(v) != reflect.TypeOf(t.Shape) {
		return nil, false, fmt.Errorf("api: %s answered a %T, not its shape %T", t.Name, v, t.Shape)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, false, err
	}
	obj := object{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, false, err
	}
	obj["id"] = must(key)
	if len(t.Actions) > 0 {
		can := map[string]bool{}
		for name, action := range t.Actions {
			can[name] = action.Can(rd.s, rd.q, key)
		}
		obj["can"] = must(can)
	}
	if rd.resources[t.Name] == nil {
		rd.resources[t.Name] = map[string]object{}
	}
	rd.resources[t.Name][key] = obj
	return obj, true, nil
}

func (rd *reader[S]) expand(t *Type[S], key string, obj object, includes tree) error {
	for name, sub := range includes {
		rel, ok := t.Relations[name]
		if !ok {
			return access.Invalid("%s has no relation %s", t.Name, name)
		}
		target, ok := rd.reg.types[rel.Type]
		if !ok {
			return errors.New("api: " + t.Name + "." + name + " names unregistered type " + rel.Type)
		}
		keys := []string{}
		for _, related := range rel.List(rd.s, rd.q, key) {
			relatedObj, ok, err := rd.object(target, related)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			if err := rd.expand(target, related, relatedObj, sub); err != nil {
				return err
			}
			keys = append(keys, related)
		}
		if rel.Many {
			obj[name] = must(keys)
			continue
		}
		if len(keys) == 0 {
			obj[name] = json.RawMessage("null")
			continue
		}
		obj[name] = must(keys[0])
	}
	return nil
}

func must(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}
