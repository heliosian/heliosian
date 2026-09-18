package filter_test

import (
	"slices"
	"testing"

	"heliosian/internal/filter"
	"heliosian/internal/who"
)

func TestTagLabels(t *testing.T) {
	owner := "ian.gulliver@heliosschool.org"
	s := filter.Sources{
		Tags: func(o string) map[string][]string {
			if o != owner {
				return nil
			}
			return map[string][]string{"Soccer": {"a@heliosschool.org"}}
		},
		Lists: func(o string) []who.List {
			if o != owner {
				return nil
			}
			return []who.List{{Key: "activity:abc", Name: "Fondue & Fort Night"}}
		},
		Shared: func(o string) []who.SharedTag {
			if o != owner {
				return nil
			}
			return []who.SharedTag{{Owner: "gayle.mcdowell@heliosschool.org", OwnerName: "Gayle", Name: "Book Club"}}
		},
	}
	r := filter.Rule{Owner: owner, Tags: []string{
		"Soccer",
		"activity:abc",
		filter.SharedKey("gayle.mcdowell@heliosschool.org", "Book Club"),
		filter.SharedKey("kris.donhowe@heliosschool.org", "Chess"),
		"Gone",
	}}
	want := []string{
		"Soccer",
		"Fondue & Fort Night",
		"Book Club (Gayle's)",
		"kris.donhowe@heliosschool.org:Chess (no longer shared)",
		"Gone (no longer a tag)",
	}
	if got := s.TagLabels(r); !slices.Equal(got, want) {
		t.Errorf("TagLabels = %q, want %q", got, want)
	}
	r.Owner = "someone.else@heliosschool.org"
	if got := s.TagLabels(r); got[1] != "activity:abc (no longer a tag)" {
		t.Errorf("another owner's Magic Tag read as %q", got[1])
	}
}
