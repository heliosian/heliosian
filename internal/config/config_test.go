package config

import (
	"context"
	"testing"
	"time"

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
	bad := *tables
	bad.SignedOut = append(bad.SignedOut, map[string]string{EmailColumn: "parent@heliosschool.org", TimeColumn: "yesterday"})
	if _, err := Parse(&bad); err == nil {
		t.Error("a Signed Out time that is not RFC 3339 should fail the load")
	}
}

type inline struct{}

func (inline) Add(f func()) { f() }

// Signing an address out records the moment in memory and in the tab, one
// row per address, overwritten by a later sign-out; a lookup reads it
// whatever the address's case.
func TestSignOutIsRecordedOncePerAddress(t *testing.T) {
	dir := &data.Dir{Root: "../../sampledata"}
	cache, err := NewCache(dir, dir, inline{})
	if err != nil {
		t.Fatalf("load sample config: %v", err)
	}
	if _, ok := cache.SignedOut("parent@heliosschool.org"); ok {
		t.Fatal("the sample tab should hold no sign-out")
	}
	before := time.Now().Add(-time.Second)
	if err := cache.SignOut(context.Background(), "Parent@heliosschool.org "); err != nil {
		t.Fatalf("sign out: %v", err)
	}
	first, ok := cache.SignedOut("PARENT@heliosschool.org")
	if !ok || first.Before(before.Truncate(time.Second)) || first.After(time.Now()) {
		t.Errorf("signed out at %v %v, want about now", first, ok)
	}
	if err := cache.SignOut(context.Background(), "parent@heliosschool.org"); err != nil {
		t.Fatalf("sign out again: %v", err)
	}
	second, _ := cache.SignedOut("parent@heliosschool.org")
	if second.Before(first) {
		t.Errorf("second sign-out at %v, want no earlier than the first at %v", second, first)
	}
	_, rows, err := dir.Table(App, SignedOutTab)
	if err != nil {
		t.Fatalf("read the tab: %v", err)
	}
	if len(rows) != 1 || rows[0][EmailColumn] != "parent@heliosschool.org" || rows[0][TimeColumn] != second.Format(time.RFC3339) {
		t.Errorf("tab holds %v, want one row for the address at the second sign-out", rows)
	}
	if tables, err := ReadTables(dir); err != nil {
		t.Errorf("re-read: %v", err)
	} else if s, err := Parse(tables); err != nil || !s.SignedOut["parent@heliosschool.org"].Equal(second) {
		t.Errorf("re-read sign-out = %v %v, want %v", s.SignedOut, err, second)
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
