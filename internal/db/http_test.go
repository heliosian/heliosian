package db

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/store"
)

const testImportKey = "test-import-key"

func send(t *testing.T, s *Store, queue *store.Queue, method, kind, as, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	Register(mux, s, queue, blob.New(blob.NewMemoryBucket()), []byte(testImportKey), func() time.Time { return testNow })
	r := httptest.NewRequest(method, "/api/q", strings.NewReader(body))
	if kind != "" {
		r.Header.Set("Content-Type", kind)
	}
	rec := httptest.NewRecorder()
	if key, ok := strings.CutPrefix(as, "bearer:"); ok {
		r.Header.Set("Authorization", "Bearer "+key)
		mux.ServeHTTP(rec, r)
		return rec
	}
	auth.Fixed(as, mux).ServeHTTP(rec, r)
	return rec
}

func TestImportKey(t *testing.T) {
	s, queue := sampleWithQueue(t)
	code, out, body := ask(t, s, "application/json", "bearer:"+testImportKey, `{"from": "PERSON"}`)
	if code != http.StatusOK || len(out.Result) != 4 {
		t.Fatalf("the import sees every person, but got %d %s", code, body)
	}
	if _, out, _ := ask(t, s, "application/json", "bearer:"+testImportKey, `{"from": "SAVED_VIEW"}`); len(out.Result) != 0 {
		t.Fatalf("the import sees %d saved views", len(out.Result))
	}
	rec := send(t, s, queue, http.MethodPost, "application/json", "bearer:"+testImportKey, `{"batch": [{"set": "per00000000001", "cells": {"vc_name": "June Ashdown"}}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("the import setting a vc_ column got %d %s", rec.Code, rec.Body.String())
	}
	rec = send(t, s, queue, http.MethodPost, "application/json", "bearer:"+testImportKey, `{"batch": [{"set": "per00000000001", "cells": {"birthday": "2016-04-12"}}]}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("the import setting a birthday got %d %s", rec.Code, rec.Body.String())
	}
	if code, _, body := ask(t, s, "application/json", "bearer:wrong", `{"from": "PERSON"}`); code != http.StatusUnauthorized {
		t.Fatalf("a wrong key got %d %s", code, body)
	}
	if code, _, _ := ask(t, s, "application/json", "bearer:", `{"from": "PERSON"}`); code != http.StatusUnauthorized {
		t.Fatalf("an empty key got %d", code)
	}
}

func ask(t *testing.T, s *Store, kind, as, query string) (int, answer, string) {
	t.Helper()
	rec := send(t, s, nil, "QUERY", kind, as, query)
	var out answer
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, out, rec.Body.String()
}

const picnicText = `(from MEMBER (where (= group "grp00000000040") (= role "member")) (order person.name_sort asc) (include person))`

const picnicJSON = `{"from": "MEMBER",
  "where": [{"=": [{"path": "group"}, "grp00000000040"]}, {"=": [{"path": "role"}, "member"]}],
  "order": [{"path": "person.name_sort", "dir": "asc"}],
  "include": ["person"]}`

func TestServeQuery(t *testing.T) {
	s := sample(t)
	for kind, query := range map[string]string{"text/plain": picnicText, "application/json": picnicJSON} {
		code, out, body := ask(t, s, kind, "Rowan.Ashdown@example.org", query)
		if code != http.StatusOK {
			t.Fatalf("%s: %d %s", kind, code, body)
		}
		if len(out.Result) != 3 || out.Now != "2026-09-30 12:00" || len(out.Resources["MEMBER"]) != 3 || len(out.Resources["PERSON"]) != 3 {
			t.Fatalf("%s: answer %+v", kind, out)
		}
		if out.Query != mustParse(t, picnicText).String() {
			t.Fatalf("%s: query %q", kind, out.Query)
		}
		if (out.Tree != nil) != (kind == "text/plain") {
			t.Fatalf("%s: tree %v", kind, out.Tree)
		}
		last := out.Resources["MEMBER"][out.Result[2]]
		if out.Result[2] != "mem00000000015" || last["guest_of"] != "per00000000002" || last["price"] != "" {
			t.Fatalf("%s: the guest's row %s reads %v", kind, out.Result[2], last)
		}
	}
}

func TestTextAndJSONAreOneTree(t *testing.T) {
	for _, src := range append(designQueries, picnicText,
		`(from PERSON (where (in id (select MEMBER.person (= group "grp00000000020"))) (not hidden) (blank consent) true) (limit 3))`,
		`(from GROUP @g (where (= (sum price MEMBER (= group @g)) 19.75) (>= start now)))`) {
		q := mustParse(t, src)
		raw, err := json.Marshal(q.Tree())
		if err != nil {
			t.Fatal(err)
		}
		back, err := ParseJSON(raw)
		if err != nil {
			t.Fatalf("%s as JSON %s: %v", src, raw, err)
		}
		if !q.tree.equal(back.tree) {
			t.Fatalf("%s came back as %s", src, back.String())
		}
	}
}

func TestServeQueryAsTheViewer(t *testing.T) {
	s := sample(t)
	q := `{"from": "SAVED_VIEW"}`
	if _, out, _ := ask(t, s, "application/json", "rowan@example.com", q); len(out.Result) != 1 {
		t.Fatalf("Rowan's second address sees %d saved views", len(out.Result))
	}
	if _, out, _ := ask(t, s, "application/json", "maya.lindqvist@example.org", q); len(out.Result) != 0 {
		t.Fatalf("Maya sees %d of Rowan's saved views", len(out.Result))
	}
	if _, out, _ := ask(t, s, "application/json", "stranger@example.org", `{"from": "PERSON"}`); len(out.Result) != 3 {
		t.Fatalf("an address the sheets don't hold sees %d people", len(out.Result))
	}
}

func TestServeQueryRefuses(t *testing.T) {
	s := sample(t)
	for _, c := range []struct {
		kind, query, want string
		code              int
	}{
		{"text/plain", `(from PERSON (where (= grde "6")))`, "did you mean grade?", http.StatusBadRequest},
		{"application/json", `{"from": "PERSON", "where": [{"=": [{"path": "grde"}, "6"]}]}`, "did you mean grade?", http.StatusBadRequest},
		{"application/json", `{"from": "PERSON", "wher": []}`, `no field "wher"`, http.StatusBadRequest},
		{"application/json", `{"from": "PERSON", "where": [{"like": []}]}`, "no condition like", http.StatusBadRequest},
		{"application/json", `{"from": "PERSON", "where": [{"=": [{"path": "@row.id"}, "x"]}]}`, "no row named @row", http.StatusBadRequest},
		{"application/json", `(from PERSON)`, "not JSON", http.StatusBadRequest},
		{"", `(from PERSON)`, "application/json", http.StatusUnsupportedMediaType},
		{"text/plain", "(from PERSON " + strings.Repeat(" ", queryLimit) + ")", "too large", http.StatusRequestEntityTooLarge},
	} {
		code, _, body := ask(t, s, c.kind, "rowan.ashdown@example.org", c.query)
		if code != c.code || !strings.Contains(body, c.want) {
			t.Errorf("%s %.40s: %d %s", c.kind, c.query, code, body)
		}
	}
}

func TestServeWrites(t *testing.T) {
	s, queue := sampleWithQueue(t)
	write := func(as, batch string) (int, string) {
		rec := send(t, s, queue, http.MethodPost, "application/json", as, batch)
		return rec.Code, rec.Body.String()
	}
	code, body := write("rowan.ashdown@example.org", `{"batch": [
		{"set": "mem00000000014", "cells": {"status": "no"}},
		{"set": "mem00000000015", "cells": {"status": "maybe"}}]}`)
	if code != http.StatusOK || !strings.Contains(body, `"mem00000000014","mem00000000015"`) {
		t.Fatalf("answering for oneself and a guest: %d %s", code, body)
	}
	for id, want := range map[string]string{"mem00000000014": "no", "mem00000000015": "maybe"} {
		if row, _ := s.Model().Table("MEMBER").Get(id); row["status"] != want {
			t.Fatalf("%s reads %v", id, row)
		}
	}
	for _, c := range []struct {
		as, batch, want string
		code            int
	}{
		{"juni@example.org", `{"batch": [{"set": "mem00000000014", "cells": {"status": "yes"}}]}`, "may not change MEMBER.status", http.StatusForbidden},
		{"rowan.ashdown@example.org", `{"batch": [{"set": "mem00000000014", "cells": {"status": "yes"}}, {"set": "mem00000000014", "cells": {"note": "hi"}}]}`, "may not change MEMBER.note", http.StatusForbidden},
		{"rowan.ashdown@example.org", `{"batch": [{"insert": "MEMBER", "row": {"group": "grp00000000040", "person": "per00000000001", "role": "member"}}]}`, "may not add to MEMBER", http.StatusForbidden},
		{"rowan.ashdown@example.org", `{"batch": [{"delete": "mem00000000014"}]}`, "may not remove from MEMBER", http.StatusForbidden},
		{"rowan.ashdown@example.org", `{"batch": [{"set": "mem99999999999", "cells": {"status": "yes"}}]}`, "no MEMBER", http.StatusNotFound},
		{"rowan.ashdown@example.org", `{"batch": [{"set": "mem00000000014", "cells": {"colour": "red"}}]}`, "no column colour", http.StatusBadRequest},
		{"rowan.ashdown@example.org", `{"batch": [{"set": "mem00000000014", "cells": {"status": "sure"}}]}`, "not one of", http.StatusBadRequest},
		{"rowan.ashdown@example.org", `{"batch": [{"set": "mem00000000014", "delete": "mem00000000014"}]}`, "one of insert", http.StatusBadRequest},
		{"rowan.ashdown@example.org", `{"batch": []}`, "empty", http.StatusBadRequest},
		{"bearer:" + testImportKey, `{"batch": [{"insert": "GROUP", "row": {"id": "grp00000000099", "kind": "family"}}]}`, "minted by the server", http.StatusBadRequest},
		{"bearer:" + testImportKey, `{"batch": [{"set": "per00000000001", "cells": {"vc_classroom": "@nowhere"}}]}`, "@nowhere names no earlier insert", http.StatusBadRequest},
		{"bearer:" + testImportKey, `{"batch": [{"delete": "@nowhere"}]}`, "@nowhere names no earlier insert", http.StatusBadRequest},
		{"bearer:" + testImportKey, `{"batch": [{"set": "per00000000001", "as": "x", "cells": {"vc_name": "x"}}]}`, "as names an insert", http.StatusBadRequest},
		{"bearer:" + testImportKey, `{"batch": [{"delete": "` + Derive(EffectiveMemberPrefix, "grp00000000020", "per00000000001") + `"}]}`, "EFFECTIVE_MEMBER is generated", http.StatusBadRequest},
		{"rowan.ashdown@example.org", `{"writes": []}`, "shape", http.StatusBadRequest},
	} {
		code, body := write(c.as, c.batch)
		if code != c.code || !strings.Contains(body, c.want) {
			t.Errorf("%s: %d %s", c.batch, code, body)
		}
	}
	if row, _ := s.Model().Table("MEMBER").Get("mem00000000014"); row["status"] != "no" || row["note"] != "" {
		t.Fatalf("a refused batch left %v", row)
	}
}

func TestEmailsAreLowerCase(t *testing.T) {
	s := sample(t)
	err := commit(s, PeopleSheet, store.Insert("PERSON_EMAIL", store.Row{"id": "eml00000000099", "address": "Maya@Example.org", "person": staff, "source": "manual"}))
	if err == nil || !strings.Contains(err.Error(), "lower case") {
		t.Fatalf("a capitalised address: %v", err)
	}
}
