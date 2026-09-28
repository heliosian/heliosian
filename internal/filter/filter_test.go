package filter_test

import (
	"slices"
	"strings"
	"testing"

	"heliosian/internal/data"
	"heliosian/internal/filter"
	"heliosian/internal/testkit"
	"heliosian/internal/who"
)

const (
	jordan = "jordan.whitfield@heliosschool.org"
	asha   = "asha.chandra@heliosschool.org"
	abena  = "abena.osei@heliosschool.org"
	colin  = "colin.quinn@heliosschool.org"
)

const (
	carpoolID    = "dtg0000000001"
	soccerTeamID = "dtg0000000002"
	bookClubID   = "dtg0000000003"
)

var (
	carpool    = filter.TagKey(carpoolID)
	soccerTeam = filter.TagKey(soccerTeamID)
	bookClub   = filter.TagKey(bookClubID)
	gone       = filter.TagKey("dtg0000000099")
)

func sample(t *testing.T) (filter.Sources, *who.Model) {
	t.Helper()
	model, err := who.LoadModel(&data.Dir{Root: "../../sampledata"}, nil, testkit.Files("../../web/who"), []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	return filter.Sources{
		Directory: model,
		Tags:      model.Tags,
		Lists: func(owner string) []who.List {
			if owner != jordan {
				return nil
			}
			return []who.List{{Key: "party:p1", Name: "Pizza Night", Kind: who.ListParty, People: []string{abena, colin}}}
		},
		Shared: model.SharedTags,
	}, model
}

func people(t *testing.T, model *who.Model, key string) []string {
	t.Helper()
	tag, ok := model.Tag(key)
	if !ok || len(tag.People) == 0 {
		t.Fatalf("the sample has no tag %s", key)
	}
	return tag.People
}

func include(tags ...string) filter.Rule {
	return filter.Rule{Kind: filter.KindInclude, Tags: tags}
}

func members(s filter.Sources, editors []string, rules ...filter.Rule) []string {
	return filter.Members(filter.List{Rules: rules, Editors: editors}, s)
}

func TestTagsReadForTheEditors(t *testing.T) {
	s, model := sample(t)
	if got, want := members(s, []string{jordan}, include(carpool)), people(t, model, carpoolID); !slices.Equal(got, want) {
		t.Errorf("the owner among the editors: got %v, want %v", got, want)
	}
	if got := members(s, []string{colin}, include(carpool)); len(got) != 0 {
		t.Errorf("nobody among the editors may read it, yet: %v", got)
	}
	if got, want := members(s, []string{asha}, include(soccerTeam)), people(t, model, soccerTeamID); !slices.Equal(got, want) {
		t.Errorf("a manager among the editors: got %v, want %v", got, want)
	}
	if got, want := members(s, []string{colin, jordan}, include(bookClub)), people(t, model, bookClubID); !slices.Equal(got, want) {
		t.Errorf("a manager among several editors: got %v, want %v", got, want)
	}
	if got := members(s, []string{jordan}, include("party:p1")); !slices.Equal(got, []string{abena, colin}) {
		t.Errorf("a Magic Tag of an editor's: %v", got)
	}
	if got := members(s, []string{asha}, include("party:p1")); len(got) != 0 {
		t.Errorf("a Magic Tag none of the editors has: %v", got)
	}
	if got := members(s, []string{jordan}, include(gone)); len(got) != 0 {
		t.Errorf("a tag that does not exist: %v", got)
	}
}

func TestTagLabels(t *testing.T) {
	s, _ := sample(t)
	r := include(carpool, bookClub, "party:p1", gone, "activity:abc")
	want := []string{
		"Carpool",
		"Book Club (Abena Osei's)",
		"Pizza Night",
		"Deleted tag",
		"activity:abc (no longer a tag)",
	}
	if got := s.TagLabels(r, []string{jordan, abena, colin}, jordan); !slices.Equal(got, want) {
		t.Errorf("TagLabels = %q, want %q", got, want)
	}
	if got := s.TagLabels(include(soccerTeam), []string{colin}, jordan); got[0] != "Soccer Team (no longer shared)" {
		t.Errorf("a tag no editor may read reads %q", got[0])
	}
	if got := s.TagLabels(include(carpool), []string{jordan}, colin); got[0] != "Carpool (Jordan Whitfield's)" {
		t.Errorf("another viewer reads %q", got[0])
	}
}

func TestWritable(t *testing.T) {
	s, _ := sample(t)
	editors := []string{jordan}
	if err := filter.Writable(s, jordan, editors, nil, []filter.Rule{include(carpool, "party:p1")}); err != nil {
		t.Errorf("the owner's own: %v", err)
	}
	if err := filter.Writable(s, jordan, editors, nil, []filter.Rule{include(bookClub)}); err != nil {
		t.Errorf("a tag shared with the saver: %v", err)
	}
	theirs := filter.Writable(s, colin, editors, nil, []filter.Rule{include(carpool)})
	vanished := filter.Writable(s, colin, editors, nil, []filter.Rule{include(gone)})
	if theirs == nil || vanished == nil || theirs.Error() != vanished.Error() {
		t.Errorf("someone else's tag: %v; a tag that does not exist: %v; want the same refusal", theirs, vanished)
	}
	if err := filter.Writable(s, jordan, editors, nil, []filter.Rule{include(gone)}); err == nil {
		t.Error("a new rule naming a tag that does not exist passed")
	}
	if err := filter.Writable(s, colin, editors, nil, []filter.Rule{include("party:p1")}); err == nil || err.Error() != theirs.Error() {
		t.Errorf("someone else's Magic Tag: %v", err)
	}
	existing := []filter.Rule{include(carpool), include(gone)}
	if err := filter.Writable(s, colin, editors, existing, existing); err != nil {
		t.Errorf("unchanged rules of someone else's, one naming a vanished tag: %v", err)
	}
	changed := include(carpool)
	changed.Grades = []string{"Grade 3"}
	if err := filter.Writable(s, colin, editors, existing, []filter.Rule{changed}); err == nil {
		t.Error("a changed rule of someone else's passed")
	}
	if err := filter.Writable(s, abena, editors, nil, []filter.Rule{include(bookClub)}); err != nil {
		t.Errorf("a tag one of the editors manages: %v", err)
	}
	if err := filter.Writable(s, asha, []string{colin}, nil, []filter.Rule{include(soccerTeam)}); err == nil || !strings.Contains(err.Error(), "edit this") {
		t.Errorf("a tag none of the editors can read: %v", err)
	}
}

func TestCleanAndCheckTagReferences(t *testing.T) {
	got := filter.Clean(filter.Rule{Kind: "Include", Tags: []string{" tag:" + strings.ToUpper(soccerTeamID) + " ", "party:p1"}})
	if !slices.Equal(got.Tags, []string{soccerTeam, "party:p1"}) {
		t.Errorf("cleaned tags = %q", got.Tags)
	}
	for _, bad := range []string{"Carpool", ":Carpool", "tag:", "tag:Carpool", "tag:dtg000000000i", jordan + ":Carpool"} {
		if err := filter.CheckFacets(include(bad)); err == nil {
			t.Errorf("%q passed", bad)
		}
	}
	if err := filter.CheckFacets(include(carpool, gone, "room:K-2")); err != nil {
		t.Errorf("good references refused: %v", err)
	}
}

func TestOptionsOfferOwnAndSharedTagsByKey(t *testing.T) {
	s, _ := sample(t)
	got := filter.OptionsFor(s, jordan).Tags
	want := []filter.TagOption{
		{Key: carpool, Name: "Carpool"},
		{Key: soccerTeam, Name: "Soccer Team"},
		{Key: bookClub, Name: "Book Club (Abena Osei's)"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("tags = %+v, want %+v", got, want)
	}
}
