package config

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/store"
)

const (
	App                = "config"
	SettingsTab        = "Settings"
	SuperAdminsTab     = "Super Admins"
	GradeColorsTab     = "Grade Colors"
	ClassroomColorsTab = "Classroom Colors"
	SignedOutTab       = "Signed Out"
)

const (
	KeyColumn       = "Key"
	ValueColumn     = "Value"
	EmailColumn     = "Email"
	GradeColumn     = "Grade"
	ClassroomColumn = "Classroom"
	ColorColumn     = "Color"
	TimeColumn      = "Time"
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
	SettingsColumns       = []string{KeyColumn, ValueColumn}
	SuperAdminColumns     = []string{EmailColumn}
	GradeColorColumns     = []string{GradeColumn, ColorColumn}
	ClassroomColorColumns = []string{ClassroomColumn, ColorColumn}
	SignedOutColumns      = []string{EmailColumn, TimeColumn}
)

var HexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type StaleYears struct {
	Photo       float64 `json:"photo"`
	Facts       float64 `json:"facts"`
	FamilyPhoto float64 `json:"familyPhoto"`
}

type PrivacyLinks struct {
	VeracrossPreferences string `json:"veracrossPreferences"`
	HeliosWhoOptIn       string `json:"heliosWhoOptIn"`
}

type Settings struct {
	SuperAdmins     []string             `json:"-"`
	SignedOut       map[string]time.Time `json:"-"`
	StaleYears      StaleYears           `json:"staleYears"`
	PrivacyLinks    PrivacyLinks         `json:"privacyLinks"`
	StaffColor      string               `json:"staffColor"`
	GradeColors     map[string]string    `json:"gradeColors"`
	ClassroomColors map[string]string    `json:"classroomColors"`
}

func parseSettings(rows []store.Row) (map[string]string, error) {
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

func parseColors(tab, keyColumn string, rows []store.Row) (map[string]string, error) {
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

func parseSignedOut(rows []store.Row) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	for _, row := range rows {
		email := strings.ToLower(strings.TrimSpace(row[EmailColumn]))
		if email == "" {
			return nil, fmt.Errorf("%s row %v has no %s", SignedOutTab, row, EmailColumn)
		}
		if _, dup := out[email]; dup {
			return nil, fmt.Errorf("%s has duplicate rows for %q", SignedOutTab, email)
		}
		at, err := time.Parse(time.RFC3339, row[TimeColumn])
		if err != nil {
			return nil, fmt.Errorf("%s time for %q must be RFC 3339, not %q", SignedOutTab, email, row[TimeColumn])
		}
		out[email] = at
	}
	return out, nil
}

func Parse(tables store.Tables) (*Settings, error) {
	values, err := parseSettings(tables[SettingsTab])
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
	if s.GradeColors, err = parseColors(GradeColorsTab, GradeColumn, tables[GradeColorsTab]); err != nil {
		return nil, err
	}
	if s.ClassroomColors, err = parseColors(ClassroomColorsTab, ClassroomColumn, tables[ClassroomColorsTab]); err != nil {
		return nil, err
	}
	if s.SignedOut, err = parseSignedOut(tables[SignedOutTab]); err != nil {
		return nil, err
	}
	emails := []string{}
	for _, row := range tables[SuperAdminsTab] {
		emails = append(emails, row[EmailColumn])
	}
	s.SuperAdmins = NormalizeEmails(emails)
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

func setSettings(values map[string]string) []store.Op {
	ops := []store.Op{}
	for _, key := range Keys {
		if value, ok := values[key]; ok {
			ops = append(ops, store.Set(SettingsTab, store.Row{KeyColumn: key}, store.Row{ValueColumn: value}))
		}
	}
	return ops
}
