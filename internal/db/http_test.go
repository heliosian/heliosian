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
	"heliosian/internal/trace"
)

const testImportKey = "test-import-key"

func send(t *testing.T, s *Store, queue *store.Queue, method, kind, as, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	Register(mux, s, queue, newPictures(s, queue), []byte(testImportKey), func() time.Time { return testNow })
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
	if _, out, _ := ask(t, s, "application/json", "bearer:"+testImportKey, `{"from": "DOCUMENT_GROUP"}`); len(out.Result) != 1 {
		t.Fatalf("the import sees %d of the one document tie, which it reads to find mail not yet sent to its groups", len(out.Result))
	}
	rec := send(t, s, queue, http.MethodPost, "application/json", "bearer:"+testImportKey, `{"batch": [{"set": "per00000000001", "cells": {"vc_name": "June Ashdown"}}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("the import setting a vc_ column got %d %s", rec.Code, rec.Body.String())
	}
	rec = send(t, s, queue, http.MethodPost, "application/json", "bearer:"+testImportKey, `{"batch": [{"set": "per00000000001", "cells": {"department_override": "grp00000000010"}}]}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("the import setting a department override got %d %s", rec.Code, rec.Body.String())
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

const picnicText = `(from MEMBER (where (= group "grp00000000040") (= member "yes")) (order person.name_sort asc) (include person))`

const picnicJSON = `{"from": "MEMBER",
  "where": [{"=": [{"path": "group"}, "grp00000000040"]}, {"=": [{"path": "member"}, "yes"]}],
  "order": [{"path": "person.name_sort", "dir": "asc"}],
  "include": ["person"]}`

func TestServeQuery(t *testing.T) {
	s := sample(t)
	for kind, query := range map[string]string{"text/plain": picnicText, "application/json": picnicJSON} {
		code, out, body := ask(t, s, kind, "Rowan.Ashdown@example.org", query)
		if code != http.StatusOK {
			t.Fatalf("%s: %d %s", kind, code, body)
		}
		if len(out.Result) != 3 || out.Now != "2026-09-30 12:00:00" || len(out.Resources["MEMBER"]) != 3 || len(out.Resources["PERSON"]) != 3 {
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

func child(t *testing.T, s *trace.Span, name string) *trace.Span {
	t.Helper()
	for _, c := range s.Children {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("%s has no %s among %d children", s.Name, name, len(s.Children))
	return nil
}

func TestServeQueryTrace(t *testing.T) {
	rec := send(t, sample(t), nil, "QUERY", "text/plain", "Rowan.Ashdown@example.org", picnicText)
	var root trace.Span
	if err := json.Unmarshal([]byte(rec.Header().Get("Trace")), &root); err != nil {
		t.Fatalf("Trace header %q: %v", rec.Header().Get("Trace"), err)
	}
	if root.Name != "request" || root.CPU == nil {
		t.Fatalf("root span %q", rec.Header().Get("Trace"))
	}
	child(t, &root, "parse")
	child(t, &root, "encode")
	run := child(t, &root, "run")
	if run.Attrs["rows"] != float64(3) {
		t.Fatalf("run attrs %v", run.Attrs)
	}
	if scan := child(t, run, "scan"); scan.Attrs["table"] != "MEMBER" {
		t.Fatalf("scan attrs %v", scan.Attrs)
	}
	child(t, run, "sort")
	if include := child(t, run, "include"); include.Count != 3 {
		t.Fatalf("include ran %d times, want once per row", include.Count)
	}
	policy := child(t, run, "policy")
	member := child(t, policy, "MEMBER")
	if member.Count == 0 || member.Counts["held"] != 3 || len(member.Children) == 0 {
		t.Fatalf("MEMBER policy tally %+v", member)
	}
	clauses := 0
	for _, c := range member.Children {
		clauses += c.Count
	}
	if clauses < member.Count {
		t.Fatalf("%d clause runs under %d MEMBER checks", clauses, member.Count)
	}
	if policy.Count < member.Count {
		t.Fatalf("policy counts %d checks, fewer than MEMBER's %d", policy.Count, member.Count)
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
	q := `{"from": "COLLECTION"}`
	if _, out, _ := ask(t, s, "application/json", "rowan@example.com", q); len(out.Result) != 1 {
		t.Fatalf("Rowan's second address sees %d collections", len(out.Result))
	}
	if _, out, _ := ask(t, s, "application/json", "maya.lindqvist@example.org", q); len(out.Result) != 0 {
		t.Fatalf("Maya sees %d of Rowan's collections", len(out.Result))
	}
}

func TestOnlyListedActivePeopleGetIn(t *testing.T) {
	s, queue := sampleWithQueue(t)
	refused := func(as, why string) {
		t.Helper()
		if code, _, body := ask(t, s, "application/json", as, `{"from": "PERSON"}`); code != http.StatusForbidden {
			t.Errorf("%s reading: %d %s", why, code, body)
		}
		rec := send(t, s, queue, http.MethodPost, "application/json", as, `{"batch": [{"insert": "MEMBER", "row": {"group": "grp00000000043", "person": "per00000000002", "member": "yes"}}]}`)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s writing: %d %s", why, rec.Code, rec.Body.String())
		}
	}
	if code, _, body := ask(t, s, "application/json", "rowan@example.com", `{"from": "PERSON"}`); code != http.StatusOK {
		t.Fatalf("a listed parent: %d %s", code, body)
	}
	refused("stranger@example.org", "an address the sheets don't hold")
	for column, value := range map[string]string{"hidden": "Yes", "deactivated": "2026-09-30 12:00", "consent": "withheld"} {
		if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": parent}, store.Row{column: value})); err != nil {
			t.Fatal(err)
		}
		refused("rowan@example.com", "a parent with "+column+" "+value)
		if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": parent}, store.Row{column: map[string]string{"consent": "listed"}[column]})); err != nil {
			t.Fatal(err)
		}
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

func TestServeWriteTrace(t *testing.T) {
	s, queue := sampleWithQueue(t)
	rec := send(t, s, queue, http.MethodPost, "application/json", "rowan.ashdown@example.org", `{"batch": [
		{"insert": "MEMBER", "row": {"group": "grp00000000043", "person": "per00000000002", "member": "yes"}},
		{"insert": "MEMBER", "row": {"group": "grp00000000042", "person": "per00000000004", "member": "yes"}}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("write: %d %s", rec.Code, rec.Body.String())
	}
	var root trace.Span
	if err := json.Unmarshal([]byte(rec.Header().Get("Trace")), &root); err != nil {
		t.Fatalf("Trace header %q: %v", rec.Header().Get("Trace"), err)
	}
	if root.Name != "request" || root.CPU == nil {
		t.Fatalf("root span %q", rec.Header().Get("Trace"))
	}
	child(t, &root, "lock")
	child(t, &root, "commit")
	for _, name := range []string{"authorize", "stage", "fire"} {
		if tally := child(t, &root, name); tally.Count != 2 {
			t.Errorf("%s ran %d times, want once per write", name, tally.Count)
		}
	}
}

func TestServeWrites(t *testing.T) {
	s, queue := sampleWithQueue(t)
	write := func(as, batch string) (int, string) {
		rec := send(t, s, queue, http.MethodPost, "application/json", as, batch)
		return rec.Code, rec.Body.String()
	}
	answer := func(group, person string) bool {
		_, ok := s.Model().Table("MEMBER").Find(group, person)
		return ok
	}
	code, body := write("rowan.ashdown@example.org", `{"batch": [
		{"insert": "MEMBER", "row": {"group": "grp00000000043", "person": "per00000000002", "member": "yes"}},
		{"insert": "MEMBER", "row": {"group": "grp00000000042", "person": "per00000000004", "member": "yes"}}]}`)
	if code != http.StatusOK {
		t.Fatalf("answering for oneself and a guest: %d %s", code, body)
	}
	if answer("grp00000000042", parent) || !answer("grp00000000043", parent) || !answer("grp00000000042", guest) {
		t.Fatal("the answers are not where they were put, or the parent's yes outlived their no")
	}
	for _, c := range []struct {
		as, batch, want string
		code            int
	}{
		{"rowan.ashdown@example.org", `{"batch": [{"insert": "MEMBER", "row": {"group": "grp00000000042", "person": "per00000000002", "member": "yes"}}, {"set": "mem00000000014", "cells": {"note": "hi"}}]}`, "may not change MEMBER.note", http.StatusForbidden},
		{"rowan.ashdown@example.org", `{"batch": [{"insert": "MEMBER", "row": {"group": "grp00000000040", "person": "per00000000001", "member": "yes"}}]}`, "may not add to MEMBER", http.StatusForbidden},
		{"rowan.ashdown@example.org", `{"batch": [{"delete": "mem00000000014"}]}`, "may not remove from MEMBER", http.StatusForbidden},
		{"rowan.ashdown@example.org", `{"batch": [{"set": "mem99999999999", "cells": {"member":"cancelled"}}]}`, "no MEMBER", http.StatusNotFound},
		{"rowan.ashdown@example.org", `{"batch": [{"set": "mem00000000014", "cells": {"colour": "red"}}]}`, "no column colour", http.StatusBadRequest},
		{"rowan.ashdown@example.org", `{"batch": [{"set": "mem00000000014", "cells": {"member":"sure"}}]}`, "not one of", http.StatusBadRequest},
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
	if row, _ := s.Model().Table("MEMBER").Get("mem00000000014"); row["note"] != "" || answer("grp00000000042", parent) || !answer("grp00000000043", parent) {
		t.Fatalf("a refused batch left %v", row)
	}
}

func TestEmailsAreLowerCase(t *testing.T) {
	s := sample(t)
	err := commit(s, PeopleSheet, store.Insert("PERSON_EMAIL", store.Row{"id": "eml00000000099", "address": "Maya@Example.org", "person": staff, "source": "manual"}))
	if err == nil || !strings.Contains(err.Error(), "lower case") {
		t.Fatalf("a capitalised address: %v", err)
	}
	err = commit(s, PeopleSheet, store.Insert("PERSON_EMAIL", store.Row{"id": "eml00000000099", "address": "juni.ashdown.noemail@example.org", "person": student, "source": "manual"}))
	if err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("a placeholder address: %v", err)
	}
}
