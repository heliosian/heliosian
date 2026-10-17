package model

import (
	"context"
	"maps"
	"slices"
	"testing"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

func sampleConfig(t *testing.T) (*data.Dir, *store.Queue, *Store) {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	queue := store.NewQueue()
	return dir, queue, sampleDirectory(t, dir, queue)
}

func configTables(t *testing.T) store.Tables {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	tables := store.Tables{}
	for _, tab := range ConfigTabs {
		_, rows, err := dir.Table(ConfigApp, tab.Name)
		if err != nil {
			t.Fatal(err)
		}
		tables[tab.Name] = rows
	}
	return tables
}

func withRows(tables store.Tables, tab string, rows ...store.Row) store.Tables {
	out := maps.Clone(tables)
	out[tab] = append(slices.Clone(tables[tab]), rows...)
	return out
}

func TestSampleConfigParses(t *testing.T) {
	s, err := parseConfig(configTables(t))
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

func TestBadConfigIsFatal(t *testing.T) {
	tables := configTables(t)
	for name, bad := range map[string]store.Tables{
		"unknown key":      withRows(tables, configSettingsTab, store.Row{configKeyColumn: "Bogus", configValueColumn: "x"}),
		"grade color":      withRows(tables, gradeColorsTab, store.Row{gradeColumn: "Grade 9", colorColumn: "red"}),
		"no super admins":  func() store.Tables { t := maps.Clone(tables); t[superAdminsTab] = nil; return t }(),
		"sign-out time":    withRows(tables, signedOutTab, store.Row{configEmailColumn: "parent@heliosschool.org", signedOutColumn: "yesterday"}),
		"duplicate colour": withRows(tables, classroomColorsTab, store.Row{classroomColumn: "Hummingbirds", colorColumn: "#000000"}),
	} {
		if _, err := parseConfig(bad); err == nil {
			t.Errorf("%s: the load was not refused", name)
		}
	}
}

func TestSignOutIsRecordedOncePerAddress(t *testing.T) {
	dir, queue, cache := sampleConfig(t)
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
	time.Sleep(1100 * time.Millisecond)
	if err := cache.SignOut(context.Background(), "parent@heliosschool.org"); err != nil {
		t.Fatalf("sign out again: %v", err)
	}
	second, _ := cache.SignedOut("parent@heliosschool.org")
	if !second.After(first) {
		t.Errorf("second sign-out at %v, want after the first at %v", second, first)
	}
	queue.Flush()
	_, rows, err := dir.Table(ConfigApp, signedOutTab)
	if err != nil {
		t.Fatalf("read the tab: %v", err)
	}
	if len(rows) != 1 || rows[0][configEmailColumn] != "parent@heliosschool.org" || rows[0][signedOutColumn] != second.Format(time.RFC3339) {
		t.Errorf("tab holds %v, want one row for the address at the second sign-out", rows)
	}
	log := testkit.ChangeLines(t, dir, queue, ConfigApp)
	want := []string{
		"parent@heliosschool.org|insert|Signed Out|Email=parent@heliosschool.org||",
		"parent@heliosschool.org|set|Signed Out|Email=parent@heliosschool.org|Time|" + first.Format(time.RFC3339),
	}
	if !slices.Equal(log, want) {
		t.Errorf("change log %q, want %q", log, want)
	}
}
