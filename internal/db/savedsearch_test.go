package db

import (
	"context"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/access"
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

func TestASavedSearchsWordsAreChecked(t *testing.T) {
	s, queue := sampleWithQueue(t)
	save := func(search string) error {
		_, err := Write(context.Background(), s, queue, access.System(setupEnv.System), setupEnv, Batch{Batch: []Edit{{Insert: "SAVED_SEARCH", Row: map[string]any{"person": parent, "name": "Test", "search": search}}}})
		return err
	}
	if err := save(`[["hummingbirds"],["community","going"]]`); err != nil {
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
