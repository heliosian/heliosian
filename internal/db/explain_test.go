package db

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"heliosian/internal/auth"
)

func getJSON(t *testing.T, s *Store, as, path string, into any) int {
	t.Helper()
	mux := http.NewServeMux()
	Register(mux, s, nil, nil, []byte(testImportKey), func() time.Time { return testNow })
	rec := httptest.NewRecorder()
	auth.Fixed(as, mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code
}

func TestThePoliciesAreListedWithTheirComments(t *testing.T) {
	s := sample(t)
	var out policyList
	if code := getJSON(t, s, "rowan.ashdown@example.org", "/api/policies", &out); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	found := map[string]Clause{}
	for _, c := range out.Clauses {
		if _, seen := found[c.Kind+" "+c.Table+" "+c.Name]; !seen {
			found[c.Kind+" "+c.Table+" "+c.Name] = c
		}
	}
	if c := found["read PERSON "]; c.Comment != "people the viewer may see" || c.Section != "Everyone" || c.Condition != "(person_visible @row)" {
		t.Errorf("PERSON's read clause: %+v", c)
	}
	if c := found["define  manages"]; c.Section != "Definitions" || len(c.Params) != 1 || c.Params[0] != "@g" || c.Comment == "" {
		t.Errorf("manages: %+v", c)
	}
}

func TestExplainingARow(t *testing.T) {
	s := sample(t)
	var out explanation
	if code := getJSON(t, s, "rowan.ashdown@example.org", "/api/explain/"+staff, &out); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if !out.Readable || len(out.Clauses) == 0 {
		t.Fatalf("explanation %+v", out)
	}
	held := false
	for _, v := range out.Clauses {
		held = held || v.Holds
	}
	if !held {
		t.Errorf("a readable row with no clause holding: %+v", out.Clauses)
	}
	columns := map[string]columnVerdict{}
	for _, c := range out.Columns {
		columns[c.Column] = c
	}
	if c := columns["consent"]; !c.Private {
		t.Errorf("consent: %+v", c)
	}
	if c := columns["name_show"]; !c.Readable {
		t.Errorf("name_show: %+v", c)
	}
	if c := columns["address_consent"]; c.Readable || len(c.Clauses) == 0 {
		t.Errorf("someone else's address consent: %+v", c)
	}

	if code := getJSON(t, s, "rowan.ashdown@example.org", "/api/explain/pst00000000001", &out); code != http.StatusNotFound {
		t.Errorf("a row the viewer can't read: %d", code)
	}
	e, ok := s.Model().explain(Env{Viewer: parent, Now: testNow}, Env{Viewer: staff, Now: testNow}, "pst00000000001")
	if !ok || e.Readable {
		t.Errorf("viewing as someone who can't read a row the signed-in person can: %v %+v", ok, e)
	}
}
