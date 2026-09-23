package filter_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/data"
	"heliosian/internal/filter"
	"heliosian/internal/who"
)

type staticFiles struct{}

func (staticFiles) Has(key string) (bool, error) {
	_, err := os.Stat(filepath.Join("../../web/who", filepath.FromSlash(key)))
	return err == nil, nil
}

func (staticFiles) Prefetch([]string) error { return nil }

const (
	jordan = "jordan.whitfield@heliosschool.org"
	asha   = "asha.chandra@heliosschool.org"
	abena  = "abena.osei@heliosschool.org"
	colin  = "colin.quinn@heliosschool.org"
)

func sample(t *testing.T) (filter.Sources, *who.Tables) {
	t.Helper()
	tables, err := who.ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatal(err)
	}
	model, err := who.BuildModel(tables, nil, staticFiles{}, []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	return filter.Sources{
		Directory: model,
		Tags:      func(owner string) map[string][]string { return who.TagsOf(tables.Tags, model, owner) },
		Lists: func(owner string) []who.List {
			if owner != jordan {
				return nil
			}
			return []who.List{{Key: "party:p1", Name: "Pizza Night", Kind: who.ListParty, People: []string{abena, colin}}}
		},
		Shared: func(email string) []who.SharedTag {
			return who.SharedTagsOf(tables.Tags, tables.Managers, model, email)
		},
	}, tables
}

func include(tags ...string) filter.Rule {
	return filter.Rule{Kind: filter.KindInclude, Tags: tags}
}

func members(s filter.Sources, editors []string, rules ...filter.Rule) []string {
	return filter.Members(filter.List{Rules: rules, Editors: editors}, s)
}

func TestTagsReadForTheEditors(t *testing.T) {
	s, tables := sample(t)
	carpool := who.TagsOf(tables.Tags, s.Directory, jordan)["Carpool"]
	if got := members(s, []string{jordan}, include(filter.TagKey(jordan, "Carpool"))); !slices.Equal(got, carpool) {
		t.Errorf("the owner among the editors: got %v, want %v", got, carpool)
	}
	if got := members(s, []string{colin}, include(filter.TagKey(jordan, "Carpool"))); len(got) != 0 {
		t.Errorf("nobody among the editors may read it, yet: %v", got)
	}
	soccer := who.TagsOf(tables.Tags, s.Directory, jordan)["Soccer Team"]
	if got := members(s, []string{asha}, include(filter.TagKey(jordan, "Soccer Team"))); !slices.Equal(got, soccer) {
		t.Errorf("a manager among the editors: got %v, want %v", got, soccer)
	}
	bookClub := who.TagsOf(tables.Tags, s.Directory, abena)["Book Club"]
	if got := members(s, []string{colin, jordan}, include(filter.TagKey(abena, "Book Club"))); !slices.Equal(got, bookClub) {
		t.Errorf("a manager among several editors: got %v, want %v", got, bookClub)
	}
	if got := members(s, []string{jordan}, include("party:p1")); !slices.Equal(got, []string{abena, colin}) {
		t.Errorf("a Magic Tag of an editor's: %v", got)
	}
	if got := members(s, []string{asha}, include("party:p1")); len(got) != 0 {
		t.Errorf("a Magic Tag none of the editors has: %v", got)
	}
	if got := members(s, []string{jordan}, include(filter.TagKey(jordan, "Gone"))); len(got) != 0 {
		t.Errorf("a tag that does not exist: %v", got)
	}
}

func TestTagLabels(t *testing.T) {
	s, _ := sample(t)
	r := include(
		filter.TagKey(jordan, "Carpool"),
		filter.TagKey(abena, "Book Club"),
		"party:p1",
		filter.TagKey(jordan, "Gone"),
		"activity:abc",
	)
	want := []string{
		"Carpool",
		"Book Club (Abena Osei's)",
		"Pizza Night",
		"Gone (no longer a tag)",
		"activity:abc (no longer a tag)",
	}
	if got := s.TagLabels(r, []string{jordan, abena, colin}, jordan); !slices.Equal(got, want) {
		t.Errorf("TagLabels = %q, want %q", got, want)
	}
	if got := s.TagLabels(include(filter.TagKey(jordan, "Soccer Team")), []string{colin}, jordan); got[0] != "Soccer Team (no longer shared)" {
		t.Errorf("a tag no editor may read reads %q", got[0])
	}
	if got := s.TagLabels(include(filter.TagKey(jordan, "Carpool")), []string{jordan}, colin); got[0] != "Carpool (Jordan Whitfield's)" {
		t.Errorf("another viewer reads %q", got[0])
	}
}

func TestWritable(t *testing.T) {
	s, _ := sample(t)
	editors := []string{jordan}
	if err := filter.Writable(s, jordan, editors, nil, []filter.Rule{include(filter.TagKey(jordan, "Carpool"), "party:p1")}); err != nil {
		t.Errorf("the owner's own: %v", err)
	}
	if err := filter.Writable(s, jordan, editors, nil, []filter.Rule{include(filter.TagKey(abena, "Book Club"))}); err != nil {
		t.Errorf("a tag shared with the saver: %v", err)
	}
	theirs := filter.Writable(s, colin, editors, nil, []filter.Rule{include(filter.TagKey(jordan, "Carpool"))})
	gone := filter.Writable(s, colin, editors, nil, []filter.Rule{include(filter.TagKey(jordan, "Gone"))})
	if theirs == nil || gone == nil || theirs.Error() != gone.Error() {
		t.Errorf("someone else's tag: %v; a tag that does not exist: %v; want the same refusal", theirs, gone)
	}
	if err := filter.Writable(s, colin, editors, nil, []filter.Rule{include("party:p1")}); err == nil || err.Error() != theirs.Error() {
		t.Errorf("someone else's Magic Tag: %v", err)
	}
	existing := []filter.Rule{include(filter.TagKey(jordan, "Carpool"))}
	if err := filter.Writable(s, colin, editors, existing, existing); err != nil {
		t.Errorf("an unchanged rule of someone else's: %v", err)
	}
	changed := include(filter.TagKey(jordan, "Carpool"))
	changed.Grades = []string{"Grade 3"}
	if err := filter.Writable(s, colin, editors, existing, []filter.Rule{changed}); err == nil {
		t.Error("a changed rule of someone else's passed")
	}
	if err := filter.Writable(s, abena, editors, nil, []filter.Rule{include(filter.TagKey(abena, "Book Club"))}); err != nil {
		t.Errorf("a tag one of the editors manages: %v", err)
	}
	if err := filter.Writable(s, asha, []string{colin}, nil, []filter.Rule{include(filter.TagKey(jordan, "Soccer Team"))}); err == nil || !strings.Contains(err.Error(), "edit this") {
		t.Errorf("a tag none of the editors can read: %v", err)
	}
}

func TestCleanAndCheckTagReferences(t *testing.T) {
	got := filter.Clean(filter.Rule{Kind: "Include", Tags: []string{" Jordan.Whitfield@HeliosSchool.org:Soccer Team ", "party:p1"}})
	if !slices.Equal(got.Tags, []string{filter.TagKey(jordan, "Soccer Team"), "party:p1"}) {
		t.Errorf("cleaned tags = %q", got.Tags)
	}
	for _, bad := range []string{"Carpool", ":Carpool", "jordan.whitfield@heliosschool.org:"} {
		if err := filter.CheckFacets(include(bad)); err == nil {
			t.Errorf("%q passed", bad)
		}
	}
	if err := filter.CheckFacets(include(filter.TagKey(jordan, "Carpool"), "room:K-2")); err != nil {
		t.Errorf("good references refused: %v", err)
	}
}

func TestOptionsOfferOwnAndSharedTagsByKey(t *testing.T) {
	s, _ := sample(t)
	got := filter.OptionsFor(s, jordan).Tags
	want := []filter.TagOption{
		{Key: filter.TagKey(jordan, "Carpool"), Name: "Carpool"},
		{Key: filter.TagKey(jordan, "Soccer Team"), Name: "Soccer Team"},
		{Key: filter.TagKey(abena, "Book Club"), Name: "Book Club (Abena Osei's)"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("tags = %+v, want %+v", got, want)
	}
}
