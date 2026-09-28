package cells

import "testing"

func TestYesNo(t *testing.T) {
	for _, c := range []struct {
		cell  string
		blank bool
		want  bool
		fails bool
	}{
		{"Yes", false, true, false},
		{" yes ", false, true, false},
		{"No", true, false, false},
		{"no", true, false, false},
		{" No ", true, false, false},
		{"", true, true, false},
		{"", false, false, false},
		{"maybe", true, false, true},
		{"TRUE", true, false, true},
	} {
		got, err := YesNo(c.cell, c.blank)
		if (err != nil) != c.fails || got != c.want {
			t.Errorf("YesNo(%q, %v) = %v, %v", c.cell, c.blank, got, err)
		}
	}
}

func TestURL(t *testing.T) {
	for _, c := range []struct {
		raw      string
		optional bool
		fails    bool
	}{
		{"https://example.com/x", false, false},
		{"http://example.com", false, false},
		{"", true, false},
		{"", false, true},
		{"https://", true, true},
		{"example.com", false, true},
		{"ftp://example.com", false, true},
	} {
		if err := URL(c.raw, c.optional); (err != nil) != c.fails {
			t.Errorf("URL(%q, %v) = %v", c.raw, c.optional, err)
		}
	}
}

func TestTitle(t *testing.T) {
	for _, c := range []struct {
		title string
		fails bool
	}{
		{"Field Day", false},
		{"", true},
		{"   ", true},
		{" Field Day", true},
		{"Field Day ", true},
		{"a much longer title than ten", true},
	} {
		if err := Title("thing", c.title, 20); (err != nil) != c.fails {
			t.Errorf("Title(%q) = %v", c.title, err)
		}
	}
}

func TestCheckPretty(t *testing.T) {
	for _, c := range []struct {
		pretty string
		fails  bool
	}{
		{"", false},
		{"fondue-night", false},
		{"fondue0000001", false},
		{"Fondue", true},
		{"pty0000000001", true},
	} {
		if err := CheckPretty(c.pretty); (err != nil) != c.fails {
			t.Errorf("CheckPretty(%q) = %v", c.pretty, err)
		}
	}
}

func TestAdded(t *testing.T) {
	for _, c := range []struct {
		cell  string
		fails bool
	}{
		{"", false},
		{"2026-09-24", false},
		{"2026-09-24 16:00", false},
		{"yesterday", true},
	} {
		if err := Added(c.cell); (err != nil) != c.fails {
			t.Errorf("Added(%q) = %v", c.cell, err)
		}
	}
}
