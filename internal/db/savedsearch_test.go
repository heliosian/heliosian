package db

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
)

func TestASavedSearchsConditionAnswersItsWords(t *testing.T) {
	s := sample(t)
	row, ok := s.Model().Table("SAVED_SEARCH").Get("sav00000000001")
	if !ok || !strings.Contains(row["condition"], `(slug "hummingbirds-parents")`) {
		t.Fatalf("the sample's saved search reads %v", row)
	}
	events := func(condition string) []string {
		t.Helper()
		got := s.Model().Run(t.Context(), mustParse(t, `(from GROUP @e (where (in kind "event" "party" "activity") `+condition+`))`), Env{Viewer: parent, Now: testNow})
		if got.Err != nil {
			t.Fatal(got.Err)
		}
		slices.Sort(got.IDs)
		return got.IDs
	}
	if got := events(row["condition"]); !slices.Equal(got, []string{"grp00000000040"}) {
		t.Errorf("Juni's Calendar holds %v, want the Fall Picnic alone", got)
	}
	everything := events(searchCondition(nil))
	if len(everything) < 3 || !slices.Equal(everything, events("")) {
		t.Errorf("a search of no lists holds %v, want everything", everything)
	}
}

func TestAWordIsForAnEventsAudience(t *testing.T) {
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{
		{Insert: "GROUP", As: "audience", Row: map[string]any{"kind": "group", "parent": "grp00000000040", "name": "Fall Picnic Audience", "status": "open", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004", "posting": "none", "replying": "none"}},
		{Set: "grp00000000040", Cells: map[string]any{"audience": "@audience"}},
		{Insert: "RULE", Row: map[string]any{"group": "@audience", "order": "i", "target": "grp00000000011", "replace_with": "household"}},
	})
	for search, want := range map[string][]string{
		`[["hummingbirds"]]`:         {"grp00000000521"},
		`[["hummingbirds-parents"]]`: {"grp00000000040", "grp00000000521"},
		`[["community"]]`:            {"grp00000000040"},
	} {
		lists, err := searchWords(search)
		if err != nil {
			t.Fatal(err)
		}
		got := s.Model().Run(t.Context(), mustParse(t, `(from GROUP @e (where (in kind "event" "party") `+searchCondition(lists)+`))`), Env{Viewer: parent, Now: testNow})
		slices.Sort(got.IDs)
		if got.Err != nil || !slices.Equal(got.IDs, want) {
			t.Errorf("%s holds %v, %v; want %v", search, got.IDs, got.Err, want)
		}
	}
}

func TestWaitlistedIsAPartyTheHouseholdWaitsFor(t *testing.T) {
	const tacoNight = "grp00000000521"
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{
		{Delete: "mem00000000025"},
		{Insert: "GROUP", As: "w", Row: map[string]any{"kind": "group", "parent": tacoNight, "name": "Taco Night Waitlist", "status": "open", "visible_to": "grp00000000004", "posting": "none", "replying": "none"}},
		{Set: tacoNight, Cells: map[string]any{"waitlist": "@w"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@w", "person": student, "member": "yes"}},
	})
	holds := func(search, viewer string) []string {
		t.Helper()
		lists, err := searchWords(search)
		if err != nil {
			t.Fatal(err)
		}
		got := s.Model().Run(t.Context(), mustParse(t, `(from GROUP @e (where (in kind "event" "party" "activity") `+searchCondition(lists)+`))`), Env{Viewer: viewer, Now: testNow})
		if got.Err != nil {
			t.Fatal(got.Err)
		}
		return got.IDs
	}
	if got := holds(`[["waitlisted"]]`, parent); !slices.Equal(got, []string{tacoNight}) {
		t.Errorf("waitlisted holds %v for a parent whose child waits for Taco Night", got)
	}
	if got := holds(`[["going"]]`, parent); slices.Contains(got, tacoNight) {
		t.Errorf("going holds Taco Night, which the household only waits for: %v", got)
	}
	if got := holds(`[["waitlisted"]]`, staff); len(got) != 0 {
		t.Errorf("waitlisted holds %v for someone waiting for nothing", got)
	}
}

func TestASearchsWordsAreRunWithoutSavingThem(t *testing.T) {
	s := sample(t)
	mux := http.NewServeMux()
	registerSavedSearch(mux, s, testTokens, func() time.Time { return testNow })
	ask := func(body string) (int, []string) {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/api/do/saved-search", strings.NewReader(body))
		rec := httptest.NewRecorder()
		auth.Fixed("rowan.ashdown@example.org", mux).ServeHTTP(rec, r)
		var out answer
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
		}
		return rec.Code, out.Result
	}
	if code, got := ask(`{"search": [], "from": "2026-09-01", "to": "2027-08-31"}`); code != http.StatusOK || !slices.Equal(got, []string{"grp00000000040", "grp00000000512", "grp00000000521"}) {
		t.Fatalf("a search of no words over the year answered %d %v", code, got)
	}
	if code, got := ask(`{"search": [["community"]], "from": "2026-09-01", "to": "2027-08-31"}`); code != http.StatusOK || !slices.Equal(got, []string{"grp00000000040"}) {
		t.Fatalf("community over the year answered %d %v", code, got)
	}
	if code, got := ask(`{"search": [], "from": "2026-10-10", "to": "2027-02-05"}`); code != http.StatusOK || !slices.Equal(got, []string{"grp00000000040", "grp00000000512"}) {
		t.Fatalf("a window from the picnic's day to International Night's answered %d %v", code, got)
	}
	for body, want := range map[string]int{
		`{"search": [["nope"]], "from": "2026-09-01", "to": "2027-08-31"}`:      http.StatusBadRequest,
		`{"search": [["Community"]], "from": "2026-09-01", "to": "2027-08-31"}`: http.StatusBadRequest,
		`{"search": [], "from": "September", "to": "2027-08-31"}`:               http.StatusBadRequest,
		`{"search": [[]], "from": "2026-09-01", "to": "2027-08-31"}`:            http.StatusBadRequest,
	} {
		if code, _ := ask(body); code != want {
			t.Errorf("%s answered %d, want %d", body, code, want)
		}
	}
}

func TestASavedSearchsFeedIsItsEvents(t *testing.T) {
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Insert: "ALIAS", Row: map[string]any{"alias": "oldfeedtoken", "target": "sav00000000001", "hidden": true}}})
	for _, token := range []string{"fed00000000002", "oldfeedtoken"} {
		ics, ok, err := s.Model().SavedSearchICS(t.Context(), token, "https://when.test", testNow)
		if err != nil || !ok {
			t.Fatalf("%s: %v %v", token, ok, err)
		}
		text := string(ics)
		if !strings.Contains(text, "SUMMARY:Fall Picnic") || !strings.Contains(text, "X-WR-CALNAME:Juni's Calendar") || !strings.Contains(text, "https://when.test/e/fall-picnic") {
			t.Fatalf("%s's feed reads %s", token, text)
		}
		if strings.Contains(text, "Taco Night") {
			t.Fatalf("%s's feed carries what its words leave out: %s", token, text)
		}
	}
	if _, ok, _ := s.Model().SavedSearchICS(t.Context(), "nope", "https://when.test", testNow); ok {
		t.Fatal("a token no saved search has answered a feed")
	}
}

func TestASavedSearchsWordsAreChecked(t *testing.T) {
	s, queue := sampleWithQueue(t)
	save := func(search string) error {
		_, err := Write(context.Background(), s, queue, access.System(setupEnv.System), setupEnv, Batch{Batch: []Edit{{Insert: "SAVED_SEARCH", Row: map[string]any{"person": parent, "name": "Test", "search": search}}}})
		return err
	}
	if err := save(`[["hummingbirds"],["community","going","waitlisted"]]`); err != nil {
		t.Fatalf("a search of known words was refused: %v", err)
	}
	for search, want := range map[string]string{
		`[["hummingbirds","nope"]]`: `no group has the slug "nope"`,
		`[["Hummingbirds"]]`:        "is not a word",
		`[["hummingbirds"],[]]`:     "an empty list of words",
		`["hummingbirds"]`:          "is not a JSON list of lists of words",
	} {
		if err := save(search); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("saving %s: %v, want %q", search, err, want)
		}
	}
}
