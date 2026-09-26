package keypoints

import (
	"context"
	"testing"
	"time"

	"heliosian/internal/artifacts"
)

// TestMissing checks which emails want points: school mail within the
// window that has none yet - not the chat, not an older email, not one
// already done.
func TestMissing(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	m := &artifacts.Model{
		Documents: []*artifacts.Document{
			{Key: "a", Date: "2026-09-25", Kind: artifacts.KindNewsletter},
			{Key: "b", Date: "2026-09-24", Kind: artifacts.KindList, Channel: "jays.parents"},
			{Key: "c", Date: "2026-09-24", Kind: artifacts.KindList, Channel: "chat"},
			{Key: "d", Date: "2026-09-01", Kind: artifacts.KindNewsletter},
			{Key: "e", Date: "2026-09-23", Kind: artifacts.KindNewsletter},
		},
		Points: map[string][]string{"e": {"done"}},
	}
	got := []string{}
	for _, d := range Missing(m, now) {
		got = append(got, d.Key)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("missing: %v", got)
	}
}

// TestFake checks the sample server's stand-in: an email's headings, else
// its first sentence.
func TestFake(t *testing.T) {
	points, _ := Fake{}.Points(context.Background(), "", "", "Hello all.\n\n## Picture Day\n\nTuesday.\n\n## Book Fair\n")
	if len(points) != 2 || points[0] != "Picture Day" || points[1] != "Book Fair" {
		t.Errorf("headings: %v", points)
	}
	points, _ = Fake{}.Points(context.Background(), "", "", "Please send snacks. Thanks!")
	if len(points) != 1 || points[0] != "Please send snacks" {
		t.Errorf("first sentence: %v", points)
	}
	if got := clean([]string{"  a   b ", "", "c"}); len(got) != 2 || got[0] != "a b" {
		t.Errorf("clean: %v", got)
	}
}
