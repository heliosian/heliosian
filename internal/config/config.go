// Package config reads the Config sheet: the platform settings every app shares.
package config

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"heliosian/internal/data"
)

const (
	App                = "config"
	SettingsTab        = "Settings"
	SuperAdminsTab     = "Super Admins"
	GradeColorsTab     = "Grade Colors"
	ClassroomColorsTab = "Classroom Colors"
)

const (
	KeyColumn       = "Key"
	ValueColumn     = "Value"
	EmailColumn     = "Email"
	GradeColumn     = "Grade"
	ClassroomColumn = "Classroom"
	ColorColumn     = "Color"
)

const (
	PhotoStaleYears       = "Photo Stale Years"
	FactsStaleYears       = "Facts Stale Years"
	FamilyPhotoStaleYears = "Family Photo Stale Years"
	VeracrossPreferences  = "Veracross Preferences URL"
	HeliosWhoOptIn        = "Helios Who Opt-In URL"
	StaffColor            = "Staff Color"
)

var Keys = []string{
	PhotoStaleYears, FactsStaleYears, FamilyPhotoStaleYears,
	VeracrossPreferences, HeliosWhoOptIn, StaffColor,
}

var (
	settingsColumns       = []string{KeyColumn, ValueColumn}
	superAdminColumns     = []string{EmailColumn}
	gradeColorColumns     = []string{GradeColumn, ColorColumn}
	classroomColorColumns = []string{ClassroomColumn, ColorColumn}
)

var HexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// StaleYears is how old a photo or facts entry can get before the directory asks
// someone to refresh it. Admin-editable so the school can loosen or tighten the nag
// without a deploy.
type StaleYears struct {
	Photo       float64 `json:"photo"`
	Facts       float64 `json:"facts"`
	FamilyPhoto float64 `json:"familyPhoto"`
}

// PrivacyLinks are the two external URLs My Privacy sends someone to fix a mismatch
// between Veracross and their Helios Who opt-in. Admin-editable, since both belong to
// other systems (Veracross's own portal, the consent Google Form) this app doesn't
// control and can't guarantee will stay put.
type PrivacyLinks struct {
	VeracrossPreferences string `json:"veracrossPreferences"`
	HeliosWhoOptIn       string `json:"heliosWhoOptIn"`
}

// Settings is the sheet parsed and validated. SuperAdmins are platform-wide;
// regular admins are per app, in each app's own Admins tab. The two are deliberately
// separate, disjoint lists rather than one list with a role flag: a regular admin's
// view of an admin page is built from an API response that a super admin's request
// never shares a code path with, so there's no field to trim or hide — a regular
// admin's client simply never receives anything about super admins to begin with.
//
// SuperAdmins are never serialized: the config endpoint serves everyone, and who the
// super admins are is only ever revealed to a super admin.
type Settings struct {
	SuperAdmins     []string          `json:"-"`
	StaleYears      StaleYears        `json:"staleYears"`
	PrivacyLinks    PrivacyLinks      `json:"privacyLinks"`
	StaffColor      string            `json:"staffColor"`
	GradeColors     map[string]string `json:"gradeColors"`
	ClassroomColors map[string]string `json:"classroomColors"`
}

// Tables are the sheet's tabs as read, before validation, kept so a write can be
// mirrored into them and the result parsed before anything is persisted.
type Tables struct {
	Settings        []map[string]string
	SuperAdmins     []map[string]string
	GradeColors     []map[string]string
	ClassroomColors []map[string]string
}

func ReadTables(source data.Source) (*Tables, error) {
	type table struct {
		name   string
		want   []string
		header []string
		rows   []map[string]string
		err    error
	}
	settings := &table{name: SettingsTab, want: settingsColumns}
	superAdmins := &table{name: SuperAdminsTab, want: superAdminColumns}
	gradeColors := &table{name: GradeColorsTab, want: gradeColorColumns}
	classroomColors := &table{name: ClassroomColorsTab, want: classroomColorColumns}
	var wg sync.WaitGroup
	for _, t := range []*table{settings, superAdmins, gradeColors, classroomColors} {
		wg.Go(func() {
			t.header, t.rows, t.err = source.Table(App, t.name)
		})
	}
	wg.Wait()
	for _, t := range []*table{settings, superAdmins, gradeColors, classroomColors} {
		if t.err != nil {
			return nil, t.err
		}
		if err := data.CheckColumns(t.name, t.header, t.want); err != nil {
			return nil, err
		}
	}
	return &Tables{
		Settings:        settings.rows,
		SuperAdmins:     superAdmins.rows,
		GradeColors:     gradeColors.rows,
		ClassroomColors: classroomColors.rows,
	}, nil
}

// SuperAdminEmails is the Super Admins tab as written, normalized.
func (t *Tables) SuperAdminEmails() []string {
	emails := make([]string, 0, len(t.SuperAdmins))
	for _, row := range t.SuperAdmins {
		emails = append(emails, row[EmailColumn])
	}
	return NormalizeEmails(emails)
}

func parseSettings(rows []map[string]string) (map[string]string, error) {
	values := map[string]string{}
	for _, row := range rows {
		key := row[KeyColumn]
		if key == "" {
			return nil, fmt.Errorf("%s row %v has no key", SettingsTab, row)
		}
		if !slices.Contains(Keys, key) {
			return nil, fmt.Errorf("%s has unknown key %q", SettingsTab, key)
		}
		if _, dup := values[key]; dup {
			return nil, fmt.Errorf("%s has duplicate key %q", SettingsTab, key)
		}
		values[key] = row[ValueColumn]
	}
	for _, key := range Keys {
		if values[key] == "" {
			return nil, fmt.Errorf("%s is missing %q", SettingsTab, key)
		}
	}
	return values, nil
}

func parseYears(values map[string]string, key string) (float64, error) {
	years, err := strconv.ParseFloat(values[key], 64)
	if err != nil || years <= 0 {
		return 0, fmt.Errorf("setting %q must be a positive number of years, not %q", key, values[key])
	}
	return years, nil
}

func parseLink(values map[string]string, key string) (string, error) {
	link := values[key]
	if !strings.HasPrefix(link, "https://") {
		return "", fmt.Errorf("setting %q must be a full https:// url, not %q", key, link)
	}
	return link, nil
}

func parseColors(tab, keyColumn string, rows []map[string]string) (map[string]string, error) {
	colors := map[string]string{}
	for _, row := range rows {
		name := row[keyColumn]
		if name == "" {
			return nil, fmt.Errorf("%s row %v has no %s", tab, row, keyColumn)
		}
		if _, dup := colors[name]; dup {
			return nil, fmt.Errorf("%s has duplicate rows for %q", tab, name)
		}
		if !HexColor.MatchString(row[ColorColumn]) {
			return nil, fmt.Errorf("%s color for %q must be a #rrggbb hex value, not %q", tab, name, row[ColorColumn])
		}
		colors[name] = row[ColorColumn]
	}
	return colors, nil
}

// Parse validates every tab and refuses the whole sheet on the first problem, the
// same stance every app's own sheet takes: an edit that breaks a rule surfaces as a
// refused load, never as a setting quietly read as something else.
func Parse(t *Tables) (*Settings, error) {
	values, err := parseSettings(t.Settings)
	if err != nil {
		return nil, err
	}
	s := &Settings{}
	if s.StaleYears.Photo, err = parseYears(values, PhotoStaleYears); err != nil {
		return nil, err
	}
	if s.StaleYears.Facts, err = parseYears(values, FactsStaleYears); err != nil {
		return nil, err
	}
	if s.StaleYears.FamilyPhoto, err = parseYears(values, FamilyPhotoStaleYears); err != nil {
		return nil, err
	}
	if s.PrivacyLinks.VeracrossPreferences, err = parseLink(values, VeracrossPreferences); err != nil {
		return nil, err
	}
	if s.PrivacyLinks.HeliosWhoOptIn, err = parseLink(values, HeliosWhoOptIn); err != nil {
		return nil, err
	}
	if !HexColor.MatchString(values[StaffColor]) {
		return nil, fmt.Errorf("setting %q must be a #rrggbb hex value, not %q", StaffColor, values[StaffColor])
	}
	s.StaffColor = values[StaffColor]
	if s.GradeColors, err = parseColors(GradeColorsTab, GradeColumn, t.GradeColors); err != nil {
		return nil, err
	}
	if s.ClassroomColors, err = parseColors(ClassroomColorsTab, ClassroomColumn, t.ClassroomColors); err != nil {
		return nil, err
	}
	s.SuperAdmins = t.SuperAdminEmails()
	if len(s.SuperAdmins) == 0 {
		return nil, fmt.Errorf("%s has no super admins", SuperAdminsTab)
	}
	return s, nil
}

func FormatYears(years float64) string {
	return strconv.FormatFloat(years, 'f', -1, 64)
}

func NormalizeEmails(emails []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, e := range emails {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" || !strings.Contains(e, "@") || seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	return out
}

// upsertRow mirrors what data.Writer.Upsert is about to write to a tab keyed by one
// column, copying the rows it touches so the tables a current model was built from
// stay intact. A blank cell is dropped, since parseTable never holds "".
func upsertRow(rows []map[string]string, keyColumn, key string, cells map[string]string) []map[string]string {
	apply := func(row map[string]string) {
		for column, value := range cells {
			if value == "" {
				delete(row, column)
				continue
			}
			row[column] = value
		}
	}
	next := make([]map[string]string, len(rows))
	copy(next, rows)
	for i, row := range next {
		if row[keyColumn] != key {
			continue
		}
		clone := maps.Clone(row)
		apply(clone)
		next[i] = clone
		return next
	}
	row := map[string]string{keyColumn: key}
	apply(row)
	return append(next, row)
}

func (t *Tables) WithSettings(values map[string]string) *Tables {
	out := *t
	for key, value := range values {
		out.Settings = upsertRow(out.Settings, KeyColumn, key, map[string]string{ValueColumn: value})
	}
	return &out
}

func (t *Tables) WithGradeColor(name, color string) *Tables {
	out := *t
	out.GradeColors = upsertRow(t.GradeColors, GradeColumn, name, map[string]string{ColorColumn: color})
	return &out
}

func (t *Tables) WithClassroomColor(name, color string) *Tables {
	out := *t
	out.ClassroomColors = upsertRow(t.ClassroomColors, ClassroomColumn, name, map[string]string{ColorColumn: color})
	return &out
}

func (t *Tables) WithSuperAdmins(emails []string) *Tables {
	out := *t
	out.SuperAdmins = make([]map[string]string, 0, len(emails))
	for _, email := range emails {
		out.SuperAdmins = append(out.SuperAdmins, map[string]string{EmailColumn: email})
	}
	return &out
}

// WriteSettings persists one Key/Value row per entry in values, matching WithSettings.
func WriteSettings(writer data.Writer, values map[string]string) error {
	for key, value := range values {
		if err := writer.Upsert(App, SettingsTab, KeyColumn, key, map[string]string{ValueColumn: value}); err != nil {
			return fmt.Errorf("set %s %q: %w", SettingsTab, key, err)
		}
	}
	return nil
}

func WriteGradeColor(writer data.Writer, name, color string) error {
	return writer.Upsert(App, GradeColorsTab, GradeColumn, name, map[string]string{ColorColumn: color})
}

func WriteClassroomColor(writer data.Writer, name, color string) error {
	return writer.Upsert(App, ClassroomColorsTab, ClassroomColumn, name, map[string]string{ColorColumn: color})
}

// WriteSuperAdmins persists the difference between the tab as it is and as it should
// be, row by row, matching WithSuperAdmins.
func WriteSuperAdmins(writer data.Writer, current, next []string) error {
	is := map[string]bool{}
	for _, e := range next {
		is[e] = true
	}
	was := map[string]bool{}
	for _, e := range current {
		was[e] = true
		if is[e] {
			continue
		}
		if err := writer.Delete(App, SuperAdminsTab, map[string]string{EmailColumn: e}); err != nil {
			return fmt.Errorf("remove super admin %s: %w", e, err)
		}
	}
	for _, e := range next {
		if was[e] {
			continue
		}
		if err := writer.Append(App, SuperAdminsTab, []string{e}); err != nil {
			return fmt.Errorf("add super admin %s: %w", e, err)
		}
	}
	return nil
}
