package keypoints

import (
	"testing"
	"time"

	"heliosian/internal/model"
)

func TestMissing(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	m := &model.Documents{
		Documents: []*model.Document{
			{Key: "a", Date: "2026-09-25", Kind: model.DocumentKindNewsletter},
			{Key: "b", Date: "2026-09-24", Kind: model.DocumentKindList, Channel: "jays.parents"},
			{Key: "c", Date: "2026-09-24", Kind: model.DocumentKindList, Channel: "chat"},
			{Key: "d", Date: "2026-09-01", Kind: model.DocumentKindNewsletter},
			{Key: "e", Date: "2026-09-23", Kind: model.DocumentKindNewsletter},
			{Key: "f", Date: "2026-09-25", Kind: model.DocumentKindNewsletter},
			{Key: "g", Date: "2026-09-22", Kind: model.DocumentKindNewsletter},
		},
		Points:   map[string][]string{"e": {"done"}, "f": {"old"}, "g": {"old"}},
		Audience: map[string]string{"e": model.DocumentForEveryone, "f": "Condors", "g": model.DocumentForEveryone},
		Judged:   map[string]string{"e": Revision, "g": "2026-09-20"},
	}
	got := []string{}
	for _, d := range Missing(m, now) {
		got = append(got, d.Key)
	}
	if len(got) != 4 || got[0] != "a" || got[1] != "b" || got[2] != "f" || got[3] != "g" {
		t.Errorf("missing: %v", got)
	}
}

func TestCleanAndKnown(t *testing.T) {
	if got := clean([]string{"  a   b ", "", "c"}); len(got) != 2 || got[0] != "a b" {
		t.Errorf("clean: %v", got)
	}
	if got := known([]string{"condors", "Nowhere", "Condors"}, []string{"Condors", "Jays"}); len(got) != 1 || got[0] != "Condors" {
		t.Errorf("known: %v", got)
	}
}
