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
	})
	reg.Add(Type[*fake]{
		Name: "people",
		Has:  func(s *fake, key string) bool { _, ok := s.people[key]; return ok },
		Get: func(s *fake, _ Query, key string) (any, bool) {
			p, ok := s.people[key]
			return p, ok && !p.Hidden
		},
		List: func(s *fake, _ Query) []string { return sortedKeys(s.people) },
	})
	reg.Add(Type[*fake]{
		Name:    "groups",
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
			"rename": {
				Can: func(s *fake, q Query, key string) bool {
					return s.groups[key].Owner == q.Actor.Email || q.Actor.May(edit)
				},
				Do: func(w Write[*fake]) error {
					if w.S.groups[w.ID].Owner != w.Query.Actor.Email && !w.Query.Actor.May(edit) {
						return access.Forbidden("not yours")
					}
					w.Tx.After(func() { w.S.renamed = append(w.S.renamed, w.ID) })
					return nil
				},
			},
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
	Now      string                               `json:"now"`
	Data     json.RawMessage                      `json:"data"`
	Included map[string]map[string]map[string]any `json:"included"`
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
		t.Fatalf("data %s: %v", raw, err)
	}
	return out
}

func TestIncludesNameEachResourceOnceAndNeverReachHiddenOnes(t *testing.T) {
	code, out := call(t, registry(sample()), "GET", "/api/groups?include=members,lead", "", nil)
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if got := ids(t, out.Data); !slices.Equal(got, []string{chess, choir}) {
		t.Errorf("data %v", got)
	}
	if out.Now != "2026-09-27 14:05" {
		t.Errorf("now %q", out.Now)
	}
	if got := sortedKeys(out.Included["people"]); !slices.Equal(got, []string{ann, bob}) {
		t.Errorf("people %v, want Ann and Bob once each and not the hidden Cat", got)
	}
	members := out.Included["groups"][chess]["members"].([]any)
	if len(members) != 2 {
		t.Errorf("chess members %v", members)
	}
	if lead := out.Included["groups"][chess]["lead"]; lead != ann {
		t.Errorf("lead %v", lead)
	}
	if out.Included["people"][ann]["id"] != ann {
		t.Errorf("person has no id: %v", out.Included["people"][ann])
	}
}

func TestRelationsAreAbsentUnlessIncluded(t *testing.T) {
	_, out := call(t, registry(sample()), "GET", "/api/groups/"+chess, "", nil)
	if _, ok := out.Included["groups"][chess]["members"]; ok {
		t.Error("members present without include")
	}
	if len(out.Included["people"]) != 0 {
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
		if err := json.Unmarshal(out.Data, &got); err != nil || got != chess {
			t.Errorf("%s: data %s", path, out.Data)
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
	if got := ids(t, out.Data); !slices.Equal(got, []string{chess}) {
		t.Errorf("member filter %v", got)
	}
	_, out = call(t, reg, "GET", "/api/groups?can=rename", "bob@example.org", nil)
	if got := ids(t, out.Data); !slices.Equal(got, []string{choir}) {
		t.Errorf("can filter %v", got)
	}
	_, out = call(t, reg, "GET", "/api/groups?mine", "ann@example.org", nil)
	if got := ids(t, out.Data); !slices.Equal(got, []string{chess}) {
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
	if can := out.Included["groups"][chess]["can"].(map[string]any); can["rename"] != false {
		t.Errorf("bob can %v", can)
	}
	_, out = call(t, reg, "GET", "/api/groups/"+chess, admin, nil)
	if can := out.Included["groups"][chess]["can"].(map[string]any); can["rename"] != true {
		t.Errorf("admin can %v", can)
	}
	if _, ok := out.Included["people"]; ok {
		t.Error("people included")
	}
}

func TestBatchAnswersFromOneSnapshot(t *testing.T) {
	code, out := call(t, registry(sample()), "POST", "/api/query", "", map[string]entry{
		"groups": {Path: "/api/groups?include=members"},
		"bob":    {Path: "/api/people/" + bob},
	})
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(out.Data, &data); err != nil {
		t.Fatal(err)
	}
	if got := ids(t, data["groups"]); !slices.Equal(got, []string{chess, choir}) {
		t.Errorf("groups %v", got)
	}
	if string(data["bob"]) != `"`+bob+`"` {
		t.Errorf("bob %s", data["bob"])
	}
	if len(out.Included["people"]) != 2 {
		t.Errorf("people %v", out.Included["people"])
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
	reg.Add(Type[*fake]{Name: "groups", Actions: map[string]Action[*fake]{"rename": {Do: func(Write[*fake]) error { return nil }}}})
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
