package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

type person struct {
	Name   string `json:"name"`
	Hidden bool   `json:"-"`
}

type group struct {
	Name    string   `json:"name"`
	Members []string `json:"-"`
	Owner   string   `json:"-"`
	Me      struct {
		Mine bool `json:"mine"`
	} `json:"me"`
}

type fake struct {
	people  map[string]person
	groups  map[string]group
	aliases map[string]string
	renamed []string
	names   []string
	scoped  []time.Time
}

type rename struct {
	Name string `json:"name"`
}

const (
	ann   = "p000000000001"
	bob   = "p000000000002"
	cat   = "p000000000003"
	chess = "g000000000001"
	choir = "g000000000002"
	old   = "g0000000000zz"
	admin = "admin@example.org"
)

var edit = access.Named("groups.edit")

func sample() *fake {
	return &fake{
		people: map[string]person{ann: {Name: "Ann"}, bob: {Name: "Bob"}, cat: {Name: "Cat", Hidden: true}},
		groups: map[string]group{
			chess: {Name: "Chess", Members: []string{ann, bob, cat}, Owner: "ann@example.org"},
			choir: {Name: "Choir", Members: []string{bob}, Owner: "bob@example.org"},
		},
		aliases: map[string]string{"chess-club": chess, old: chess},
	}
}

func registry(w *fake) *Registry[*fake] {
	reg := New(Config[*fake]{
		Actor: func(r *http.Request, _ *fake) access.Actor {
			email := r.Header.Get("X-As")
			held := []access.Allowance{}
			if email == admin {
				held = append(held, edit)
			}
			return access.Actor{Email: email, Allowances: access.Grant(held)}
		},
		Held: func(email string) []access.Allowance {
			if email == admin {
				return []access.Allowance{edit}
			}
			return nil
		},
		Now:    func() time.Time { return time.Date(2026, 9, 27, 14, 5, 0, 0, time.UTC) },
		Queue:  store.NewQueue(),
		Staged: func(*store.Tx) *fake { return w },
		Scope: func(s *fake, q Query) *fake {
			s.scoped = append(s.scoped, q.Now)
			return s
		},
	})
	reg.Add(Type[*fake]{
		Name:  "people",
		Shape: person{},
		Has:   func(s *fake, key string) bool { _, ok := s.people[key]; return ok },
		Get: func(s *fake, _ Query, key string) (any, bool) {
			p, ok := s.people[key]
			return p, ok && !p.Hidden
		},
		List: func(s *fake, _ Query) []string { return sortedKeys(s.people) },
	})
	reg.Add(Type[*fake]{
		Name:    "groups",
		Shape:   group{},
		Has:     func(s *fake, key string) bool { _, ok := s.groups[key]; return ok },
		Aliases: func(s *fake) map[string]string { return s.aliases },
		Get: func(s *fake, q Query, key string) (any, bool) {
			g, ok := s.groups[key]
			g.Me.Mine = g.Owner == q.Actor.Email
			return g, ok
		},
		List: func(s *fake, _ Query) []string { return sortedKeys(s.groups) },
		Relations: map[string]Relation[*fake]{
			"members": {Type: "people", Many: true, List: func(s *fake, _ Query, key string) []string { return s.groups[key].Members }},
			"lead":    {Type: "people", List: func(s *fake, _ Query, key string) []string { return s.groups[key].Members[:1] }},
		},
		Filters: map[string]Filter[*fake]{
			"member": func(s *fake, _ Query, value string) (func(string) bool, error) {
				if _, ok := s.people[value]; !ok {
					return nil, access.Invalid("no person %s", value)
				}
				return func(key string) bool { return slices.Contains(s.groups[key].Members, value) }, nil
			},
		},
		Actions: map[string]Action[*fake]{
			"rename": Do(func(s *fake, q Query, key string) bool {
				return s.groups[key].Owner == q.Actor.Email || q.Actor.May(edit)
			}, func(w Write[*fake], in rename) error {
				if w.S.groups[w.ID].Owner != w.Query.Actor.Email && !w.Query.Actor.May(edit) {
					return access.Forbidden("not yours")
				}
				w.Tx.After(func() {
					w.S.renamed = append(w.S.renamed, w.ID)
					w.S.names = append(w.S.names, in.Name)
				})
				return nil
			}),
			"split": DoMaking(func(s *fake, q Query, key string) bool {
				return q.Actor.May(edit)
			}, func(w Write[*fake], in rename) (string, error) {
				return "g000000000003", nil
			}),
		},
	})
	reg.Publish(w)
	return reg
}

func sortedKeys[V any](m map[string]V) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

type reply struct {
	Now       string                               `json:"now"`
	Result    json.RawMessage                      `json:"result"`
	Resources map[string]map[string]map[string]any `json:"resources"`
}

func call(t *testing.T, reg *Registry[*fake], method, path, as string, body any) (int, reply) {
	t.Helper()
	mux := http.NewServeMux()
	reg.Register(mux)
	var in bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&in).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &in)
	req.Header.Set("X-As", as)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var out reply
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s %s: %v: %s", method, path, err, rec.Body.String())
		}
	}
	return rec.Code, out
}

func ids(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("result %s: %v", raw, err)
	}
	return out
}

func TestIncludesNameEachResourceOnceAndNeverReachHiddenOnes(t *testing.T) {
	code, out := call(t, registry(sample()), "GET", "/api/groups?include=members,lead", "", nil)
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if got := ids(t, out.Result); !slices.Equal(got, []string{chess, choir}) {
		t.Errorf("result %v", got)
	}
	if out.Now != "2026-09-27 14:05" {
		t.Errorf("now %q", out.Now)
	}
	if got := sortedKeys(out.Resources["people"]); !slices.Equal(got, []string{ann, bob}) {
		t.Errorf("people %v, want Ann and Bob once each and not the hidden Cat", got)
	}
	members := out.Resources["groups"][chess]["members"].([]any)
	if len(members) != 2 {
		t.Errorf("chess members %v", members)
	}
	if lead := out.Resources["groups"][chess]["lead"]; lead != ann {
		t.Errorf("lead %v", lead)
	}
	if out.Resources["people"][ann]["id"] != ann {
		t.Errorf("person has no id: %v", out.Resources["people"][ann])
	}
}

func TestRelationsAreAbsentUnlessIncluded(t *testing.T) {
	_, out := call(t, registry(sample()), "GET", "/api/groups/"+chess, "", nil)
	if _, ok := out.Resources["groups"][chess]["members"]; ok {
		t.Error("members present without include")
	}
	if len(out.Resources["people"]) != 0 {
		t.Error("people included without include")
	}
}

func TestPathsResolveByIDThenAliasOfThatType(t *testing.T) {
	reg := registry(sample())
	for _, path := range []string{"/api/groups/" + chess, "/api/groups/G000000000001", "/api/groups/chess-club", "/api/groups/" + old, "/api/r/" + chess} {
		code, out := call(t, reg, "GET", path, "", nil)
		if code != http.StatusOK {
			t.Errorf("%s: status %d", path, code)
			continue
		}
		var got string
		if err := json.Unmarshal(out.Result, &got); err != nil || got != chess {
			t.Errorf("%s: result %s", path, out.Result)
		}
	}
	for _, path := range []string{"/api/people/chess-club", "/api/r/chess-club", "/api/r/" + old, "/api/people/" + cat, "/api/nothing/" + ann} {
		if code, _ := call(t, reg, "GET", path, "", nil); code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", path, code)
		}
	}
}

func TestFiltersAreDeclaredOrRefused(t *testing.T) {
	reg := registry(sample())
	_, out := call(t, reg, "GET", "/api/groups?member="+ann, "", nil)
	if got := ids(t, out.Result); !slices.Equal(got, []string{chess}) {
		t.Errorf("member filter %v", got)
	}
	_, out = call(t, reg, "GET", "/api/groups?can=rename", "bob@example.org", nil)
	if got := ids(t, out.Result); !slices.Equal(got, []string{choir}) {
		t.Errorf("can filter %v", got)
	}
	_, out = call(t, reg, "GET", "/api/groups?mine", "ann@example.org", nil)
	if got := ids(t, out.Result); !slices.Equal(got, []string{chess}) {
		t.Errorf("mine filter %v", got)
	}
	for _, path := range []string{"/api/groups?colour=red", "/api/groups?can=explode", "/api/groups?member=nobody", "/api/groups?include=owners", "/api/groups?include=members..x"} {
		if code, _ := call(t, reg, "GET", path, "", nil); code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", path, code)
		}
	}
}

func TestCanIsTheActionsOwnRule(t *testing.T) {
	reg := registry(sample())
	_, out := call(t, reg, "GET", "/api/groups/"+chess, "bob@example.org", nil)
	if can := out.Resources["groups"][chess]["can"].(map[string]any); can["rename"] != false {
		t.Errorf("bob can %v", can)
	}
	_, out = call(t, reg, "GET", "/api/groups/"+chess, admin, nil)
	if can := out.Resources["groups"][chess]["can"].(map[string]any); can["rename"] != true {
		t.Errorf("admin can %v", can)
	}
	if _, ok := out.Resources["people"]; ok {
		t.Error("people included")
	}
}

func TestBatchAnswersFromOneSnapshot(t *testing.T) {
	w := sample()
	code, out := call(t, registry(w), "POST", "/api/query", "", map[string]entry{
		"groups": {Path: "/api/groups?include=members"},
		"bob":    {Path: "/api/people/" + bob},
	})
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(out.Result, &result); err != nil {
		t.Fatal(err)
	}
	if got := ids(t, result["groups"]); !slices.Equal(got, []string{chess, choir}) {
		t.Errorf("groups %v", got)
	}
	if string(result["bob"]) != `"`+bob+`"` {
		t.Errorf("bob %s", result["bob"])
	}
	if len(out.Resources["people"]) != 2 {
		t.Errorf("people %v", out.Resources["people"])
	}
	if len(w.scoped) != 1 || !w.scoped[0].Equal(time.Date(2026, 9, 27, 14, 5, 0, 0, time.UTC)) {
		t.Errorf("the batch was scoped %v, want once at its own time", w.scoped)
	}
	code, _ = call(t, registry(sample()), "POST", "/api/query", "", map[string]entry{"x": {Path: "/api/groups?colour=red"}})
	if code != http.StatusBadRequest {
		t.Errorf("bad entry: status %d", code)
	}
}

func TestActionsRunByResourceAndAlias(t *testing.T) {
	w := sample()
	reg := registry(w)
	if code, _ := call(t, reg, "POST", "/api/groups/chess-club/rename", "ann@example.org", nil); code != http.StatusNoContent {
		t.Errorf("owner rename: status %d", code)
	}
	if code, _ := call(t, reg, "POST", "/api/groups/"+chess+"/rename", "bob@example.org", nil); code != http.StatusForbidden {
		t.Errorf("stranger rename: status %d", code)
	}
	if code, _ := call(t, reg, "POST", "/api/groups/"+chess+"/explode", admin, nil); code != http.StatusNotFound {
		t.Errorf("unknown action: status %d", code)
	}
	if code, _ := call(t, reg, "DELETE", "/api/groups/"+chess, admin, nil); code != http.StatusNotFound {
		t.Errorf("delete without a delete action: status %d", code)
	}
	if code, _ := call(t, reg, "POST", "/api/groups", admin, nil); code != http.StatusMethodNotAllowed {
		t.Errorf("create without create: status %d", code)
	}
	if !slices.Equal(w.renamed, []string{chess}) {
		t.Errorf("renamed %v", w.renamed)
	}
}

func TestAnActionNeedsBothItsRuleAndItsRoute(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("an action without a can rule registered")
		}
	}()
	reg := New(Config[*fake]{})
	reg.Add(Type[*fake]{Name: "groups", Shape: group{}, Actions: map[string]Action[*fake]{"rename": Do(nil, func(Write[*fake], serve.None) error { return nil })}})
}

func TestAnActionGetsItsBodyTyped(t *testing.T) {
	w := sample()
	reg := registry(w)
	if code, _ := call(t, reg, "POST", "/api/groups/"+chess+"/rename", admin, rename{Name: "Chess Club"}); code != http.StatusNoContent {
		t.Errorf("rename: status %d", code)
	}
	if code, _ := call(t, reg, "POST", "/api/groups/"+chess+"/rename", admin, map[string]int{"name": 7}); code != http.StatusBadRequest {
		t.Errorf("a body of the wrong shape: status %d, want 400", code)
	}
	if !slices.Equal(w.names, []string{"Chess Club"}) {
		t.Errorf("names %v", w.names)
	}
}

func TestAMakingActionAnswersTheNewID(t *testing.T) {
	reg := registry(sample())
	mux := http.NewServeMux()
	reg.Register(mux)
	req := httptest.NewRequest("POST", "/api/groups/"+chess+"/split", bytes.NewBufferString(`{"name":"Chess B"}`))
	req.Header.Set("X-As", admin)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var out struct {
		ID string `json:"id"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil || out.ID != "g000000000003" {
		t.Errorf("split: status %d, body %s", rec.Code, rec.Body)
	}
}

func TestATypeNeedsItsShape(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a type without a shape registered")
		}
	}()
	New(Config[*fake]{}).Add(Type[*fake]{Name: "groups"})
}

func TestAResourceMustMatchItsShape(t *testing.T) {
	w := sample()
	reg := registry(w)
	reg.types["people"].Shape = group{}
	if code, _ := call(t, reg, "GET", "/api/people/"+ann, "", nil); code != http.StatusInternalServerError {
		t.Errorf("status %d, want 500", code)
	}
}

func TestTheSpecDescribesEveryType(t *testing.T) {
	mux := http.NewServeMux()
	registry(sample()).Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/openapi.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var spec struct {
		Paths      map[string]map[string]any `json:"paths"`
		Components struct {
			Schemas map[string]struct {
				Properties map[string]map[string]any `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatal(err)
	}
	for path, method := range map[string]string{"/api/people": "get", "/api/groups/{id}": "get", "/api/groups/{id}/rename": "post", "/api/query": "post", "/api/act": "post", "/api/me": "get"} {
		if _, ok := spec.Paths[path][method]; !ok {
			t.Errorf("no %s %s", method, path)
		}
	}
	if _, ok := spec.Paths["/api/groups"]["post"]; ok {
		t.Error("groups can't be created but the spec says so")
	}
	raw, err := json.Marshal(spec.Paths["/api/groups/{id}/rename"]["post"].(map[string]any)["requestBody"])
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Content struct {
			JSON struct {
				Schema struct {
					Properties map[string]map[string]any `json:"properties"`
				} `json:"schema"`
			} `json:"application/json"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if got := body.Content.JSON.Schema.Properties["name"]["type"]; got != "string" {
		t.Errorf("rename body %s, want a string name", raw)
	}
	groups := spec.Components.Schemas["groups"].Properties
	for _, field := range []string{"id", "name", "me", "can", "members", "lead"} {
		if _, ok := groups[field]; !ok {
			t.Errorf("groups has no %s: %v", field, groups)
		}
	}
	if _, ok := groups["Members"]; ok {
		t.Error("a field tagged - is in the spec")
	}
	if got := groups["members"]["type"]; got != "array" {
		t.Errorf("members type %v", got)
	}
	reachedFrom := func(path string) []string {
		var reply struct {
			Properties struct {
				Resources struct {
					Properties map[string]any `json:"properties"`
				} `json:"resources"`
			} `json:"properties"`
		}
		raw, err := json.Marshal(spec.Paths[path]["get"].(map[string]any)["responses"].(map[string]any)["200"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"])
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &reply); err != nil {
			t.Fatal(err)
		}
		return sortedKeys(reply.Properties.Resources.Properties)
	}
	for _, p := range spec.Paths["/api/groups"]["get"].(map[string]any)["parameters"].([]any) {
		p := p.(map[string]any)
		if p["name"] != "mine" {
			continue
		}
		s := p["schema"].(map[string]any)
		if s["type"] != "boolean" || !slices.Equal(s["enum"].([]any), []any{true}) {
			t.Errorf("mine schema %v, want a boolean that is only true", s)
		}
	}
	if got := reachedFrom("/api/people"); !slices.Equal(got, []string{"people"}) {
		t.Errorf("people reach %v", got)
	}
	if got := reachedFrom("/api/groups/{id}"); !slices.Equal(got, []string{"groups", "people"}) {
		t.Errorf("groups reach %v", got)
	}
}

func TestWritesBatchAllOrNothing(t *testing.T) {
	w := sample()
	reg := registry(w)
	rename := func(key string) step { return step{Method: "POST", Path: "/api/groups/" + key + "/rename"} }
	if code, _ := call(t, reg, "POST", "/api/act", "ann@example.org", []step{rename(chess), rename(choir)}); code != http.StatusForbidden {
		t.Errorf("a batch with a refused write: status %d", code)
	}
	if len(w.renamed) != 0 {
		t.Fatalf("a refused batch ran %v", w.renamed)
	}
	if code, _ := call(t, reg, "POST", "/api/act", admin, []step{rename("chess-club"), rename(choir), {Method: "DELETE", Path: "/api/groups/" + choir}}); code != http.StatusNotFound {
		t.Errorf("a batch with a missing action: status %d", code)
	}
	if code, _ := call(t, reg, "POST", "/api/act", admin, []step{rename("chess-club"), rename(choir)}); code != http.StatusOK {
		t.Errorf("admin batch: status %d", code)
	}
	if !slices.Equal(w.renamed, []string{chess, choir}) {
		t.Errorf("renamed %v", w.renamed)
	}
	if code, _ := call(t, reg, "POST", "/api/act", admin, []step{}); code != http.StatusBadRequest {
		t.Errorf("an empty batch: status %d", code)
	}
}

func TestTakenCoversEveryTypesIDsAndAliases(t *testing.T) {
	reg := registry(sample())
	for _, key := range []string{ann, cat, chess, old} {
		if !reg.Taken(key) {
			t.Errorf("%s not taken", key)
		}
	}
	if reg.Taken("z000000000001") {
		t.Error("free ID taken")
	}
}

func TestMeListsHeldAllowances(t *testing.T) {
	reg := registry(sample())
	mux := http.NewServeMux()
	reg.Register(mux)
	for as, want := range map[string][]string{admin: {edit.Name}, "ann@example.org": {}} {
		req := httptest.NewRequest("GET", "/api/me", nil)
		req.Header.Set("X-As", as)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var got me
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got.Allowances, want) {
			t.Errorf("%s: allowances %v", as, got.Allowances)
		}
	}
}
