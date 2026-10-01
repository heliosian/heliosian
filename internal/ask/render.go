package ask

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"heliosian/internal/api"
	"heliosian/internal/model"
)

type ref struct {
	typ, id string
}

type found struct {
	mu   sync.Mutex
	refs map[string]ref
}

func newFound() *found {
	return &found{refs: map[string]ref{}}
}

func (f *found) note(url string, r ref) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refs[url] = r
}

func (f *found) of(url string) (ref, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.refs[url]
	return r, ok
}

type turn struct {
	reg   *api.Registry[*model.Model]
	r     *http.Request
	clock func() time.Time
	found *found
}

type node struct {
	clip int
	kids map[string]*node
}

func fields(spec string) *node {
	root := &node{kids: map[string]*node{}}
	for path := range strings.SplitSeq(spec, ",") {
		at := root
		for name := range strings.SplitSeq(strings.TrimSpace(path), ".") {
			clip := 0
			if bare, n, ok := strings.Cut(name, "~"); ok {
				name = bare
				clip, _ = strconv.Atoi(n)
			}
			if at.kids[name] == nil {
				at.kids[name] = &node{kids: map[string]*node{}}
			}
			at = at.kids[name]
			at.clip = max(at.clip, clip)
		}
	}
	return root
}

type query struct {
	name    string
	path    string
	fields  *node
	scoreAs string
	limit   int
}

func (t *turn) read(paths map[string]string) (api.Envelope, error) {
	return t.reg.Read(t.r, paths)
}

func (t *turn) ask(queries ...query) (map[string]any, error) {
	paths := map[string]string{}
	for _, q := range queries {
		paths[q.name] = q.path
	}
	env, err := t.read(paths)
	if err != nil {
		return nil, err
	}
	results := env.Result.(map[string]any)
	out := map[string]any{}
	for _, q := range queries {
		rendered := t.render(env, results[q.name], q.fields, q.scoreAs)
		list, isList := rendered.([]any)
		if isList && q.limit > 0 && len(list) > q.limit {
			rendered, out[q.name+"More"] = list[:q.limit], true
		}
		out[q.name] = rendered
	}
	return out, nil
}

type rendering struct {
	t     *turn
	env   api.Envelope
	types map[string]string
}

func (t *turn) render(env api.Envelope, result any, spec *node, scoreAs string) any {
	rd := rendering{t: t, env: env, types: map[string]string{}}
	for typ, objects := range env.Resources {
		for key := range objects {
			rd.types[key] = typ
		}
	}
	switch result := result.(type) {
	case string:
		return rd.one(result, spec)
	case []string:
		out := []any{}
		for _, key := range result {
			out = append(out, rd.one(key, spec))
		}
		return out
	case []api.Hit:
		out := []any{}
		for _, hit := range result {
			one := rd.one(hit.ID, spec)
			one[scoreAs] = hit.Score
			out = append(out, one)
		}
		return out
	}
	return nil
}

func appURL(app, path string) string {
	if app == "" || path == "" {
		return ""
	}
	host := app + ".heliosian.com"
	if app == "home" {
		host = "heliosian.com"
	}
	return "https://" + host + path
}

func (rd rendering) one(key string, spec *node) map[string]any {
	typ := rd.types[key]
	obj := rd.env.Resources[typ][key]
	out := map[string]any{}
	for name, child := range spec.kids {
		if name == "link" {
			if url := appURL(text(obj, "app"), text(obj, "path")); url != "" {
				out["link"] = url
				rd.t.found.note(url, ref{typ: typ, id: key})
			}
			continue
		}
		raw, ok := obj[name]
		if !ok {
			continue
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			panic(err)
		}
		if v = rd.value(v, child); !empty(v) {
			out[name] = v
		}
	}
	return out
}

func (rd rendering) value(v any, spec *node) any {
	if len(spec.kids) == 0 {
		if s, ok := v.(string); ok && spec.clip > 0 {
			return clip(s, spec.clip)
		}
		return v
	}
	switch v := v.(type) {
	case string:
		if _, known := rd.types[v]; !known {
			return v
		}
		one := rd.one(v, spec)
		if len(spec.kids) == 1 {
			for name := range spec.kids {
				return one[name]
			}
		}
		return one
	case []any:
		out := []any{}
		for _, item := range v {
			if item = rd.value(item, spec); !empty(item) {
				out = append(out, item)
			}
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for name, child := range spec.kids {
			if item, ok := v[name]; ok {
				if item = rd.value(item, child); !empty(item) {
					out[name] = item
				}
			}
		}
		return out
	}
	return v
}

func text(obj api.Object, name string) string {
	raw, ok := obj[name]
	if !ok {
		return ""
	}
	var out string
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(err)
	}
	return out
}

func empty(v any) bool {
	switch v := v.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case bool:
		return !v
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	}
	return false
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + "…"
}
