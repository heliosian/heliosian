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

func TestWhen(t *testing.T) {
	for _, c := range []struct {
		cell  string
		fails bool
	}{
		{"2026-09-24", false},
		{"2026-09-24 16:00", false},
		{"2026-09-24 16:00:05", false},
		{"", true},
		{"yesterday", true},
	} {
		if _, err := When(c.cell); (err != nil) != c.fails {
			t.Errorf("When(%q) = %v", c.cell, err)
		}
	}
}
