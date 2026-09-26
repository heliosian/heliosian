package calendar

import (
	"context"
	"slices"
	"testing"
)

type recordingImages struct {
	prefetched []string
	asked      []string
	missing    map[string]bool
}

func (r *recordingImages) Has(key string) (bool, error) {
	r.asked = append(r.asked, key)
	return !r.missing[key], nil
}

func (r *recordingImages) Prefetch(_ context.Context, names []string) error {
	r.prefetched = append(r.prefetched, names...)
	return nil
}

func TestResolveImagesAsksForEveryPicture(t *testing.T) {
	rec := &recordingImages{missing: map[string]bool{"category-images/gone.jpg": true}}
	model := &Model{
		Tags:    []Tag{{Name: "Trip", Image: "sample/trip.jpg"}, {Name: "Plain"}},
		Events:  []*Event{{ID: "shown", Source: SourceSheet, Image: "/category-images/shown.jpg"}, {ID: "feed", Source: SourceGoogle}},
		Pending: []*Event{{ID: "waiting", Source: SourceSheet, Image: "/category-images/gone.jpg"}},
		Invitations: map[string]*Invitation{
			"shown": {EventID: "shown", Flyer: "category-images/flyer.jpg"},
			"bare":  {EventID: "bare"},
		},
	}
	resolveImages(context.Background(), rec, model)
	want := []string{"sample/trip.jpg", "category-images/shown.jpg", "category-images/gone.jpg", "category-images/flyer.jpg"}
	slices.Sort(want)
	prefetched := slices.Clone(rec.prefetched)
	slices.Sort(prefetched)
	if !slices.Equal(prefetched, want) {
		t.Errorf("prefetched %v, want %v", prefetched, want)
	}
	asked := slices.Clone(rec.asked)
	slices.Sort(asked)
	if !slices.Equal(asked, want) {
		t.Errorf("asked %v, want %v", asked, want)
	}
	if model.Tags[0].ImageURL != "/sample/trip.jpg" || model.Tags[1].ImageURL != "" {
		t.Errorf("tag addresses: %q %q", model.Tags[0].ImageURL, model.Tags[1].ImageURL)
	}
	if model.Pending[0].Image != "/category-images/gone.jpg" {
		t.Errorf("a missing picture's name was changed: %q", model.Pending[0].Image)
	}
}
