package digest

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"heliosian/internal/model"
)

func TestClean(t *testing.T) {
	markdown := "Sign up [here](https://example.org/sign-up) by Friday."
	got := clean([]answer{
		{Title: " Sign  up for clubs. ", Summary: "Fall clubs", Details: "Sign-ups close Friday.", Link: "https://example.org/sign-up", Due: "2026-09-25", Point: 2},
		{Title: "Sign up for clubs", Summary: "Again", Details: "A repeat."},
		{Title: "Pay for the trip", Summary: "Trip fee", Details: "Pay online.", Link: "https://example.org/made-up", Due: "next Friday", Point: 4},
		{Title: "  ", Summary: "No title"},
		{Title: "placeholder"},
		{Title: "", Summary: "", Details: ""},
	}, markdown, 3)
	if len(got) != 2 {
		t.Fatalf("clean kept %+v", got)
	}
	if got[0].Title != "Sign up for clubs" || got[0].Link != "https://example.org/sign-up" || got[0].Due != "2026-09-25" || got[0].Point != 2 {
		t.Errorf("the first to-do = %+v", got[0])
	}
	if got[1].Link != "" || got[1].Due != "" || got[1].Point != 0 {
		t.Errorf("a link the email lacks, a day that is no date or a point past the points was kept: %+v", got[1])
	}
}

func TestPointsAndAudience(t *testing.T) {
	if got := cleanPoints([]string{"  a   b ", "", "c"}); !slices.Equal(got, []string{"a b", "c"}) {
		t.Errorf("cleanPoints: %v", got)
	}
	if got := known([]string{"condors", "Nowhere", "Condors"}, []string{"Condors", "Jays"}); !slices.Equal(got, []string{"Condors"}) {
		t.Errorf("known: %v", got)
	}
	if got := audience([]string{"Condors"}, []string{"Grade 6"}); got != "Condors, Grade 6" {
		t.Errorf("audience: %q", got)
	}
	if got := audience(nil, nil); got != model.DocumentForEveryone {
		t.Errorf("an email to nobody named is for %q", got)
	}
}

func TestTheSchemaListsPointsBeforeToDosAndTitlesFirst(t *testing.T) {
	body, err := json.Marshal(anthropic.OutputConfigParam{Format: anthropic.JSONOutputFormatParam{Schema: schema}})
	if err != nil {
		t.Fatal(err)
	}
	order := []int{}
	for _, name := range []string{`"points"`, `"classrooms"`, `"grades"`, `"asks"`, `"todos"`, `"title"`, `"summary"`, `"details"`, `"link"`, `"due"`, `"point"`} {
		order = append(order, strings.Index(string(body), name+`:{`))
	}
	if !slices.IsSorted(order) || order[0] < 0 {
		t.Errorf("the reading's fields go to Claude out of order: %s", body)
	}
}

func TestMissing(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, model.Location)
	m := &model.Documents{
		Documents: []*model.Document{
			{Key: "new", Date: "2026-09-29", Kind: model.DocumentKindNewsletter},
			{Key: "group", Date: "2026-09-28", Kind: model.DocumentKindGroup, Channel: "soccer-team"},
			{Key: "read", Date: "2026-09-27", Kind: model.DocumentKindList, Channel: "jays.parents"},
			{Key: "stale", Date: "2026-09-26", Kind: model.DocumentKindList, Channel: "jays.parents"},
			{Key: "chat", Date: "2026-09-25", Kind: model.DocumentKindList, Channel: "chat"},
			{Key: "page", Date: "2026-09-24", Kind: model.DocumentKindPage, Channel: "website"},
			{Key: "old", Date: "2026-09-01", Kind: model.DocumentKindNewsletter},
		},
		Read: map[string]string{"read": Revision, "stale": "2026-01-01"},
	}
	got := []string{}
	for _, d := range Missing(m, now) {
		got = append(got, d.Key)
	}
	if !slices.Equal(got, []string{"stale", "group", "new"}) {
		t.Errorf("missing = %v, want the unread and stale school and group mail of the window, oldest first", got)
	}
}
