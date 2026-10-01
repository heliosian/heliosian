package db

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/store"
)

func send(t *testing.T, s *Store, method, as, query string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	Register(mux, s, func() time.Time { return testNow })
	rec := httptest.NewRecorder()
	auth.Fixed(as, mux).ServeHTTP(rec, httptest.NewRequest(method, "/api/q", strings.NewReader(query)))
	return rec
}

func ask(t *testing.T, s *Store, as, query string) (int, answer, string) {
	t.Helper()
	rec := send(t, s, "QUERY", as, query)
	var out answer
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, out, rec.Body.String()
}

func TestServeQuery(t *testing.T) {
	s := sample(t)
	code, out, body := ask(t, s, "Rowan.Hockin@example.org", `(from MEMBER (where (= group "grp00000000040") (= role "member")) (order person.name_sort asc) (include person))`)
	if code != http.StatusOK {
		t.Fatalf("%d %s", code, body)
	}
	if len(out.Result) != 3 || out.Now != "2026-09-30 12:00" || len(out.Resources["MEMBER"]) != 3 || len(out.Resources["PERSON"]) != 3 {
		t.Fatalf("answer %+v", out)
	}
	if !strings.HasPrefix(out.Query, "(from MEMBER") {
		t.Fatalf("query %q", out.Query)
	}
	last := out.Resources["MEMBER"][out.Result[2]]
	if out.Result[2] != "mem00000000015" || last["guest_of"] != "per00000000002" || last["price"] != "" {
		t.Fatalf("the guest's row %s reads %v", out.Result[2], last)
	}
}

func TestServeQueryWantsQuery(t *testing.T) {
	if code := send(t, sample(t), http.MethodPost, "rowan.hockin@example.org", `(from PERSON)`).Code; code != http.StatusMethodNotAllowed {
		t.Fatalf("a POST got %d", code)
	}
}

func TestServeQueryAsTheViewer(t *testing.T) {
	s := sample(t)
	q := `(from SAVED_VIEW)`
	if _, out, _ := ask(t, s, "rowan@example.com", q); len(out.Result) != 1 {
		t.Fatalf("Rowan's second address sees %d saved views", len(out.Result))
	}
	if _, out, _ := ask(t, s, "maya.lindqvist@example.org", q); len(out.Result) != 0 {
		t.Fatalf("Maya sees %d of Rowan's saved views", len(out.Result))
	}
	if _, out, _ := ask(t, s, "stranger@example.org", `(from PERSON)`); len(out.Result) != 3 {
		t.Fatalf("an address the sheets don't hold sees %d people", len(out.Result))
	}
}

func TestServeQueryRefuses(t *testing.T) {
	s := sample(t)
	code, _, body := ask(t, s, "rowan.hockin@example.org", `(from PERSON (where (= grde "6")))`)
	if code != http.StatusBadRequest || !strings.Contains(body, "did you mean grade?") {
		t.Fatalf("%d %s", code, body)
	}
	code, _, _ = ask(t, s, "rowan.hockin@example.org", "(from PERSON "+strings.Repeat(" ", queryLimit)+")")
	if code != http.StatusRequestEntityTooLarge {
		t.Fatalf("a long query got %d", code)
	}
}

func TestEmailsAreLowerCase(t *testing.T) {
	s := sample(t)
	err := commit(s, PeopleSheet, store.Insert("PERSON_EMAIL", store.Row{"id": "eml00000000099", "address": "Maya@Example.org", "person": staff, "source": "manual"}))
	if err == nil || !strings.Contains(err.Error(), "lower case") {
		t.Fatalf("a capitalised address: %v", err)
	}
}
