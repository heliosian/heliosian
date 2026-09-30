package model

import (
	"slices"
	"strings"
	"testing"

	"heliosian/internal/data"
	"heliosian/internal/testkit"
)

var (
	carpoolKey    = TagKey(carpool)
	soccerTeamKey = TagKey(soccerTeam)
	bookClubKey   = TagKey(bookClub)
	goneKey       = TagKey("dtg0000000099")
)

func audienceSample(t *testing.T) (AudienceSources, *Directory) {
	t.Helper()
	d := loadDirectory(t, &data.Dir{Root: "../../sampledata"}, nil, testkit.Files("../../web/who"), sampleKey)
	return AudienceSources{
		Directory: d,
		MagicTags: func(owner string) []MagicTag {
			if owner != jordan {
				return nil
			}
			return []MagicTag{{Key: "party:p1", Name: "Pizza Night", Kind: MagicTagParty, People: []string{abena, colin}}}
		},
		EmailLists: &EmailLists{},
	}, d
}

func tagPeople(t *testing.T, d *Directory, key string) []string {
	t.Helper()
	tag, ok := d.Tag(key)
	if !ok || len(tag.People) == 0 {
		t.Fatalf("the sample has no tag %s", key)
	}
	return tag.People
}

func include(tags ...string) Rule {
	return Rule{Kind: RuleInclude, Tags: tags}
}

func audienceMembers(s AudienceSources, editors []string, rules ...Rule) []string {
	return Audience{Rules: rules, Editors: editors}.Members(s)
}

func TestTagsReadForTheEditors(t *testing.T) {
	s, d := audienceSample(t)
	if got, want := audienceMembers(s, []string{jordan}, include(carpoolKey)), tagPeople(t, d, carpool); !slices.Equal(got, want) {
		t.Errorf("the owner among the editors: got %v, want %v", got, want)
	}
	if got := audienceMembers(s, []string{colin}, include(carpoolKey)); len(got) != 0 {
		t.Errorf("nobody among the editors may read it, yet: %v", got)
	}
	if got, want := audienceMembers(s, []string{asha}, include(soccerTeamKey)), tagPeople(t, d, soccerTeam); !slices.Equal(got, want) {
		t.Errorf("a manager among the editors: got %v, want %v", got, want)
	}
	if got, want := audienceMembers(s, []string{colin, jordan}, include(bookClubKey)), tagPeople(t, d, bookClub); !slices.Equal(got, want) {
		t.Errorf("a manager among several editors: got %v, want %v", got, want)
	}
	if got := audienceMembers(s, []string{jordan}, include("party:p1")); !slices.Equal(got, []string{abena, colin}) {
		t.Errorf("a Magic Tag of an editor's: %v", got)
	}
	if got := audienceMembers(s, []string{asha}, include("party:p1")); len(got) != 0 {
		t.Errorf("a Magic Tag none of the editors has: %v", got)
	}
	if got := audienceMembers(s, []string{jordan}, include(goneKey)); len(got) != 0 {
		t.Errorf("a tag that does not exist: %v", got)
	}
}

func TestTagLabels(t *testing.T) {
	s, _ := audienceSample(t)
	r := include(carpoolKey, bookClubKey, "party:p1", goneKey, "activity:abc")
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
	if got := s.TagLabels(include(soccerTeamKey), []string{colin}, jordan); got[0] != "Soccer Team (no longer shared)" {
		t.Errorf("a tag no editor may read reads %q", got[0])
	}
	if got := s.TagLabels(include(carpoolKey), []string{jordan}, colin); got[0] != "Carpool (Jordan Whitfield's)" {
		t.Errorf("another viewer reads %q", got[0])
	}
}

func TestWritable(t *testing.T) {
	s, _ := audienceSample(t)
	editors := []string{jordan}
	if err := s.Writable(jordan, editors, nil, []Rule{include(carpoolKey, "party:p1")}); err != nil {
		t.Errorf("the owner's own: %v", err)
	}
	if err := s.Writable(jordan, editors, nil, []Rule{include(bookClubKey)}); err != nil {
		t.Errorf("a tag shared with the saver: %v", err)
	}
	theirs := s.Writable(colin, editors, nil, []Rule{include(carpoolKey)})
	vanished := s.Writable(colin, editors, nil, []Rule{include(goneKey)})
	if theirs == nil || vanished == nil || theirs.Error() != vanished.Error() {
		t.Errorf("someone else's tag: %v; a tag that does not exist: %v; want the same refusal", theirs, vanished)
	}
	if err := s.Writable(jordan, editors, nil, []Rule{include(goneKey)}); err == nil {
		t.Error("a new rule naming a tag that does not exist passed")
	}
	if err := s.Writable(colin, editors, nil, []Rule{include("party:p1")}); err == nil || err.Error() != theirs.Error() {
		t.Errorf("someone else's Magic Tag: %v", err)
	}
	existing := []Rule{include(carpoolKey), include(goneKey)}
	if err := s.Writable(colin, editors, existing, existing); err != nil {
		t.Errorf("unchanged rules of someone else's, one naming a vanished tag: %v", err)
	}
	changed := include(carpoolKey)
	changed.Grades = []string{"Grade 3"}
	if err := s.Writable(colin, editors, existing, []Rule{changed}); err == nil {
		t.Error("a changed rule of someone else's passed")
	}
	if err := s.Writable(abena, editors, nil, []Rule{include(bookClubKey)}); err != nil {
		t.Errorf("a tag one of the editors manages: %v", err)
	}
	if err := s.Writable(asha, []string{colin}, nil, []Rule{include(soccerTeamKey)}); err == nil || !strings.Contains(err.Error(), "edit this") {
		t.Errorf("a tag none of the editors can read: %v", err)
	}
}

func TestCleanAndCheckTagReferences(t *testing.T) {
	got := Rule{Kind: "Include", Tags: []string{" tag:" + strings.ToUpper(soccerTeam) + " ", "party:p1"}}.Clean()
	if !slices.Equal(got.Tags, []string{soccerTeamKey, "party:p1"}) {
		t.Errorf("cleaned tags = %q", got.Tags)
	}
	for _, bad := range []string{"Carpool", ":Carpool", "tag:", "tag:Carpool", "tag:dtg000000000i", jordan + ":Carpool"} {
		if err := include(bad).CheckFacets(); err == nil {
			t.Errorf("%q passed", bad)
		}
	}
	if err := include(carpoolKey, goneKey, "room:K-2").CheckFacets(); err != nil {
		t.Errorf("good references refused: %v", err)
	}
}

func TestOptionsOfferOwnAndSharedTagsByKey(t *testing.T) {
	s, _ := audienceSample(t)
	got := s.Options(jordan).Tags
	want := []TagOption{
		{Key: carpoolKey, Name: "Carpool"},
		{Key: soccerTeamKey, Name: "Soccer Team"},
		{Key: bookClubKey, Name: "Book Club (Abena Osei's)"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("tags = %+v, want %+v", got, want)
	}
}
