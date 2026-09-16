package groups

import (
	"context"
	"slices"
	"testing"
)

func quiet(string, ...any) {}

func TestReconcileMakesAndCorrectsGroups(t *testing.T) {
	fake := NewFake()
	fake.Seed("old@loop.heliosian.com", "Old", []string{"a@x.org", "gone@x.org"})
	fake.Seed("stray@loop.heliosian.com", "Stray", nil)
	desired := []Desired{
		{Name: "old", Title: "Old", Members: []string{"a@x.org", "b@x.org"}},
		{Name: "new", Title: "New", Description: "fresh", Members: []string{"c@x.org"}},
	}
	result := Reconcile(context.Background(), fake, desired, quiet)
	if len(result.Errors) != 0 {
		t.Fatalf("errors %v", result.Errors)
	}
	if result.Added != 2 || result.Removed != 1 {
		t.Fatalf("added %d removed %d", result.Added, result.Removed)
	}
	if !fake.Has("old@loop.heliosian.com", "b@x.org") || fake.Has("old@loop.heliosian.com", "gone@x.org") {
		t.Fatal("old's members are wrong")
	}
	if !fake.Has("new@loop.heliosian.com", "c@x.org") {
		t.Fatal("new was not made")
	}
	if !slices.Equal(result.Orphans, []string{"stray@loop.heliosian.com"}) {
		t.Fatalf("orphans %v", result.Orphans)
	}
	if _, err := fake.Members(context.Background(), "stray@loop.heliosian.com"); err != nil {
		t.Fatal("the orphan was deleted")
	}
}

func TestSyncerSendsOnlyTheDifference(t *testing.T) {
	fake := NewFake()
	fake.Seed("a@loop.heliosian.com", "A", []string{"one@x.org"})
	plan := []Desired{{Name: "a", Title: "A", Members: []string{"one@x.org", "two@x.org"}}}
	s := NewSyncer(fake, func() []Desired { return slices.Clone(plan) })
	s.Run(context.Background())
	if fake.Calls["Members"] != 1 || fake.Calls["Add"] != 1 {
		t.Fatalf("first pass: %v", fake.Calls)
	}
	if st := s.Status("a"); st.Error != "" || st.Synced.IsZero() || st.Members != 2 {
		t.Fatalf("status %+v", st)
	}
	plan[0].Members = []string{"two@x.org", "three@x.org"}
	plan[0].Title = "A renamed"
	s.Run(context.Background())
	if fake.Calls["Members"] != 1 {
		t.Fatal("the second pass read the members back")
	}
	if fake.Calls["Add"] != 2 || fake.Calls["Remove"] != 1 || fake.Calls["Ensure"] != 2 {
		t.Fatalf("second pass: %v", fake.Calls)
	}
	if fake.Titles()["a@loop.heliosian.com"] != "A renamed" {
		t.Fatal("the rename did not land")
	}
	plan = append(plan, Desired{Name: "b", Title: "B", Members: []string{"one@x.org"}})
	s.Run(context.Background())
	if !fake.Has("b@loop.heliosian.com", "one@x.org") {
		t.Fatal("b was not made")
	}
	plan = plan[1:]
	s.Run(context.Background())
	if fake.Calls["Delete"] != 1 {
		t.Fatalf("a was not deleted: %v", fake.Calls)
	}
	if st := s.Status("a"); st.Members != 0 || !st.Synced.IsZero() {
		t.Fatalf("a still has a status: %+v", st)
	}
}

func TestSyncerKeepsTheLastSentOnFailure(t *testing.T) {
	failing := &failingGoogle{Fake: NewFake(), fail: true}
	plan := []Desired{{Name: "a", Title: "A", Members: []string{"one@x.org"}}}
	s := NewSyncer(failing, func() []Desired { return slices.Clone(plan) })
	s.Run(context.Background())
	if st := s.Status("a"); st.Error == "" {
		t.Fatal("no error recorded")
	}
	failing.fail = false
	s.Run(context.Background())
	if st := s.Status("a"); st.Error != "" || !failing.Has("a@loop.heliosian.com", "one@x.org") {
		t.Fatalf("the retry did not land: %+v", st)
	}
}

type failingGoogle struct {
	*Fake
	fail bool
}

func (f *failingGoogle) Ensure(ctx context.Context, address, title, description string) error {
	if f.fail {
		return context.DeadlineExceeded
	}
	return f.Fake.Ensure(ctx, address, title, description)
}
