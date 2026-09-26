package keypoints

import (
	"context"
	"testing"
	"time"

	"heliosian/internal/artifacts"
)

// TestMissing checks which emails want reading: school mail within the
// window whose audience is not yet judged, then any judged before Revision
// - not the chat, not an older email, not one judged since.
func TestMissing(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	m := &artifacts.Model{
		Documents: []*artifacts.Document{
			{Key: "a", Date: "2026-09-25", Kind: artifacts.KindNewsletter},
			{Key: "b", Date: "2026-09-24", Kind: artifacts.KindList, Channel: "jays.parents"},
			{Key: "c", Date: "2026-09-24", Kind: artifacts.KindList, Channel: "chat"},
			{Key: "d", Date: "2026-09-01", Kind: artifacts.KindNewsletter},
			{Key: "e", Date: "2026-09-23", Kind: artifacts.KindNewsletter},
			{Key: "f", Date: "2026-09-25", Kind: artifacts.KindNewsletter},
			{Key: "g", Date: "2026-09-22", Kind: artifacts.KindNewsletter},
		},
		Points:   map[string][]string{"e": {"done"}, "f": {"old"}, "g": {"old"}},
		Audience: map[string]string{"e": artifacts.Everyone, "f": "Condors", "g": artifacts.Everyone},
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

// TestFake checks the sample server's stand-in: an email's headings, else
// its first sentence.
func TestFake(t *testing.T) {
	points := fakePoints("Hello all.\n\n## Picture Day\n\nTuesday.\n\n## Book Fair\n")
	if len(points) != 2 || points[0] != "Picture Day" || points[1] != "Book Fair" {
		t.Errorf("headings: %v", points)
	}
	points = fakePoints("Please send snacks. Thanks!")
	if len(points) != 1 || points[0] != "Please send snacks" {
		t.Errorf("first sentence: %v", points)
	}
	if got := clean([]string{"  a   b ", "", "c"}); len(got) != 2 || got[0] != "a b" {
		t.Errorf("clean: %v", got)
	}
	if got := known([]string{"condors", "Nowhere", "Condors"}, []string{"Condors", "Jays"}); len(got) != 1 || got[0] != "Condors" {
		t.Errorf("known: %v", got)
	}
	r, _ := Fake{}.Read(context.Background(), Email{Markdown: "Hi.", Teaches: []string{"Condors"}})
	if len(r.Classrooms) != 1 || r.Classrooms[0] != "Condors" {
		t.Errorf("fake reading: %+v", r)
	}
}
