package todos

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
		{Title: " Sign  up for clubs. ", Summary: "Fall clubs", Details: "Sign-ups close Friday.", Link: "https://example.org/sign-up", Due: "2026-09-25"},
		{Title: "Sign up for clubs", Summary: "Again", Details: "A repeat."},
		{Title: "Pay for the trip", Summary: "Trip fee", Details: "Pay online.", Link: "https://example.org/made-up", Due: "next Friday"},
		{Title: "  ", Summary: "No title"},
		{Title: "placeholder"},
		{Title: "", Summary: "", Details: ""},
	}, markdown)
	if len(got) != 2 {
		t.Fatalf("clean kept %+v", got)
	}
	if got[0].Title != "Sign up for clubs" || got[0].Link != "https://example.org/sign-up" || got[0].Due != "2026-09-25" {
		t.Errorf("the first to-do = %+v", got[0])
	}
	if got[1].Link != "" || got[1].Due != "" {
		t.Errorf("a link the email lacks or a day that is no date was kept: %+v", got[1])
	}
}

func TestTheSchemaListsTheTitleFirst(t *testing.T) {
	body, err := json.Marshal(anthropic.OutputConfigParam{Format: anthropic.JSONOutputFormatParam{Schema: schema}})
	if err != nil {
		t.Fatal(err)
	}
	order := []int{}
	for _, name := range []string{`"title"`, `"summary"`, `"details"`, `"link"`, `"due"`} {
		order = append(order, strings.Index(string(body), name+`:{`))
	}
	if !slices.IsSorted(order) || order[0] < 0 {
		t.Errorf("the to-do's fields go to Claude out of order: %s", body)
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
		ToDosRead: map[string]string{"read": Revision, "stale": "2026-01-01"},
	}
	got := []string{}
	for _, d := range Missing(m, now) {
		got = append(got, d.Key)
	}
	if !slices.Equal(got, []string{"stale", "group", "new"}) {
		t.Errorf("missing = %v, want the unread and stale school and group mail of the window, oldest first", got)
	}
}
