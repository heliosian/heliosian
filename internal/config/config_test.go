package config

import (
	"testing"

	"heliosian/internal/data"
)

func sampleTables(t *testing.T) *Tables {
	t.Helper()
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample config: %v", err)
	}
	return tables
}

func TestSampleConfigParses(t *testing.T) {
	s, err := Parse(sampleTables(t))
	if err != nil {
		t.Fatalf("parse sample config: %v", err)
	}
	if s.StaleYears.Photo != 0.75 || s.StaleYears.Facts != 0.6 || s.StaleYears.FamilyPhoto != 1.5 {
		t.Errorf("stale years = %+v, want the Settings tab values", s.StaleYears)
	}
	if s.PrivacyLinks.HeliosWhoOptIn != "https://hca.run/optin" {
		t.Errorf("opt-in link = %q, want the Settings tab value", s.PrivacyLinks.HeliosWhoOptIn)
	}
	if s.StaffColor != "#1f4d53" {
		t.Errorf("staff color = %q, want the Settings tab value", s.StaffColor)
	}
	if s.GradeColors["Kindergarten"] != "#d20210" || s.ClassroomColors["Hummingbirds"] != "#d20210" {
		t.Errorf("colors = %v %v, want the color tabs' rows", s.GradeColors, s.ClassroomColors)
	}
	if len(s.SuperAdmins) != 1 || s.SuperAdmins[0] != "jordan.whitfield@heliosschool.org" {
		t.Errorf("super admins = %v, want the sample parent", s.SuperAdmins)
	}
}

// A key the app does not know, a malformed value, or an empty super admin list
// refuses the load rather than being read as a default.
func TestBadConfigIsFatal(t *testing.T) {
	tables := sampleTables(t)
	if _, err := Parse(tables.WithSettings(map[string]string{"Bogus": "x"})); err == nil {
		t.Error("an unknown Settings key should fail the load")
	}
	if _, err := Parse(tables.WithSettings(map[string]string{StaffColor: "teal"})); err == nil {
		t.Error("a non-hex Staff Color should fail the load")
	}
	if _, err := Parse(tables.WithGradeColor("Grade 1", "red")); err == nil {
		t.Error("a non-hex grade color should fail the load")
	}
	if _, err := Parse(tables.WithSuperAdmins(nil)); err == nil {
		t.Error("an empty Super Admins tab should fail the load")
	}
}

// A mirrored write leaves the tables it was mirrored into untouched.
func TestMirrorsCopy(t *testing.T) {
	tables := sampleTables(t)
	before := tables.Settings[0][ValueColumn]
	tables.WithSettings(map[string]string{tables.Settings[0][KeyColumn]: "changed"})
	if tables.Settings[0][ValueColumn] != before {
		t.Error("WithSettings changed the original rows")
	}
}
