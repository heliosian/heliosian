package home

import (
	"slices"
	"testing"

	"heliosian/internal/data"
	"heliosian/internal/filter"
	"heliosian/internal/who"
)

type noFiles struct{}

func (noFiles) Has(string) (bool, error) { return false, nil }

func (noFiles) Prefetch([]string) error { return nil }

type sampleDirectory struct{ model *who.Model }

func (d sampleDirectory) Sources() filter.Sources {
	return filter.Sources{Directory: d.model}
}

func directoryOf(t *testing.T) sampleDirectory {
	t.Helper()
	tables, err := who.ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatal(err)
	}
	model, err := who.BuildModel(tables, nil, noFiles{}, []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	return sampleDirectory{model}
}

func TestAudienceIsAListOfRules(t *testing.T) {
	c := sampleCache(t)
	c.directory = directoryOf(t)
	links := map[string]Link{}
	var chats Category
	for _, cat := range c.Model().Categories {
		if cat.Title == "Chats" {
			chats = cat
		}
		for _, l := range cat.Links {
			links[l.Title] = l
		}
	}
	if got := links["Hawks and Falcons Chat"].Rules; len(got) != 1 || got[0].Kind != filter.KindInclude || got[0].Roles[0] != "Parent" || len(got[0].Classrooms) != 2 {
		t.Fatalf("the chat's rules = %+v", got)
	}
	const (
		jordan = "jordan.whitfield@heliosschool.org" // parent: Jays, Ospreys
		sam    = "sam.whitfield@heliosschool.org"    // student: Jays
		ruth   = "ruth.amari@heliosschool.org"       // staff, teaching the Hummingbirds
	)
	sees := func(rules []filter.Rule, email string) bool {
		return len(rules) == 0 || c.includes(rules, email)
	}
	for _, tc := range []struct {
		title string
		email string
		want  bool
	}{
		{"Directory", sam, true},
		{"Parent Portal", jordan, true},
		{"Parent Portal", sam, false},
		{"Staff Room", ruth, true},
		{"Staff Room", jordan, false},
		{"Hummingbirds Chat", ruth, false},
		{"Hawks and Falcons Chat", jordan, false},
		{"Jays Chat", jordan, true},
		{"Jays Chat", sam, false},
	} {
		if got := sees(links[tc.title].Rules, tc.email); got != tc.want {
			t.Errorf("%s for %s = %v, want %v", tc.title, tc.email, got, tc.want)
		}
	}
	if len(chats.Rules) != 1 || sees(chats.Rules, sam) || !sees(chats.Rules, ruth) || !sees(chats.Rules, jordan) {
		t.Errorf("Chats = %+v, want kept to parents and staff", chats.Rules)
	}
	if hidden := c.HiddenApps(jordan); slices.Contains(hidden, "celebrate") {
		t.Errorf("a Jays parent is kept from the celebration: %v", hidden)
	}
	if hidden := c.HiddenApps(sam); !slices.Contains(hidden, "celebrate") {
		t.Errorf("a student sees the celebration: %v", hidden)
	}
	tables := c.Tables().withAudience(thingLink+"Directory", []filter.Rule{{Kind: filter.KindExclude, Roles: []string{"Student"}}})
	model, err := BuildModel(tables, noImages{})
	if err != nil {
		t.Fatal(err)
	}
	if got := model.Categories[2].Links[0].Rules; len(got) != 1 || got[0].Kind != filter.KindExclude {
		t.Errorf("the Directory's rules after a save = %+v", got)
	}
	for _, bad := range []map[string]string{
		{"Thing": "link:Directory", "Kind": "include", "Roles": "Teachers"},
		{"Thing": "link:Directory", "Kind": "include"},
		{"Thing": "link:Directory", "Kind": "include", "Tags": "Carpool"},
	} {
		tables := c.Tables().withAudience(thingLink+"Directory", nil)
		tables.Audience = append(tables.Audience, bad)
		if _, err := BuildModel(tables, noImages{}); err == nil {
			t.Errorf("a bad rule %v loaded", bad)
		}
	}
}
