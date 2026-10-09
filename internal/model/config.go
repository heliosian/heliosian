package model

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/mail"
	"heliosian/internal/store"
)

const (
	ConfigApp          = "config"
	configSettingsTab  = "Settings"
	superAdminsTab     = "Super Admins"
	gradeColorsTab     = "Grade Colors"
	classroomColorsTab = "Classroom Colors"
	signedOutTab       = "Signed Out"
)

const (
	configKeyColumn   = "Key"
	configValueColumn = "Value"
	configEmailColumn = "Email"
	gradeColumn       = "Grade"
	classroomColumn   = "Classroom"
	colorColumn       = "Color"
	signedOutColumn   = "Time"
)

const (
	PhotoStaleYears       = "Photo Stale Years"
	FactsStaleYears       = "Facts Stale Years"
	FamilyPhotoStaleYears = "Family Photo Stale Years"
	VeracrossPreferences  = "Veracross Preferences URL"
	HeliosWhoOptIn        = "Helios Who Opt-In URL"
	StaffColor            = "Staff Color"
)

var configKeys = []string{
	PhotoStaleYears, FactsStaleYears, FamilyPhotoStaleYears,
	VeracrossPreferences, HeliosWhoOptIn, StaffColor,
}

var ConfigTabs = []store.Tab{
	{Name: configSettingsTab, Columns: []string{configKeyColumn, configValueColumn}, Key: []string{configKeyColumn}},
	{Name: superAdminsTab, Columns: []string{configEmailColumn}, Key: []string{configEmailColumn}},
	{Name: gradeColorsTab, Columns: []string{gradeColumn, colorColumn}, Key: []string{gradeColumn}},
	{Name: classroomColorsTab, Columns: []string{classroomColumn, colorColumn}, Key: []string{classroomColumn}},
	{Name: signedOutTab, Columns: []string{configEmailColumn, signedOutColumn}, Key: []string{configEmailColumn}},
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type StaleYears struct {
	Photo       float64 `json:"photo"`
	Facts       float64 `json:"facts"`
	FamilyPhoto float64 `json:"familyPhoto"`
}

type PrivacyLinks struct {
	VeracrossPreferences string `json:"veracrossPreferences"`
	HeliosWhoOptIn       string `json:"heliosWhoOptIn"`
}

type Config struct {
	SuperAdmins     []string             `json:"-"`
	SignedOut       map[string]time.Time `json:"-"`
	StaleYears      StaleYears           `json:"staleYears"`
	PrivacyLinks    PrivacyLinks         `json:"privacyLinks"`
	StaffColor      string               `json:"staffColor"`
	GradeColors     map[string]string    `json:"gradeColors"`
	ClassroomColors map[string]string    `json:"classroomColors"`
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
		if !hexColor.MatchString(row[colorColumn]) {
			return nil, fmt.Errorf("%s color for %q must be a #rrggbb hex value, not %q", tab, name, row[colorColumn])
		}
		colors[name] = row[colorColumn]
	}
	return colors, nil
}

func parseSignedOut(rows []store.Row) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	for _, row := range rows {
		email := mail.Normalize(row[configEmailColumn])
		if email == "" {
			return nil, fmt.Errorf("%s row %v has no %s", signedOutTab, row, configEmailColumn)
		}
		if _, dup := out[email]; dup {
			return nil, fmt.Errorf("%s has duplicate rows for %q", signedOutTab, email)
		}
		at, err := time.Parse(time.RFC3339, row[signedOutColumn])
		if err != nil {
			return nil, fmt.Errorf("%s time for %q must be RFC 3339, not %q", signedOutTab, email, row[signedOutColumn])
		}
		out[email] = at
	}
	return out, nil
}

func parseConfig(tables store.Tables) (*Config, error) {
	values, err := store.ParseSettings(tables[configSettingsTab], configKeys, nil)
	if err != nil {
		return nil, err
	}
	s := &Config{}
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
	if !hexColor.MatchString(values[StaffColor]) {
		return nil, fmt.Errorf("setting %q must be a #rrggbb hex value, not %q", StaffColor, values[StaffColor])
	}
	s.StaffColor = values[StaffColor]
	if s.GradeColors, err = parseColors(gradeColorsTab, gradeColumn, tables[gradeColorsTab]); err != nil {
		return nil, err
	}
	if s.ClassroomColors, err = parseColors(classroomColorsTab, classroomColumn, tables[classroomColorsTab]); err != nil {
		return nil, err
	}
	if s.SignedOut, err = parseSignedOut(tables[signedOutTab]); err != nil {
		return nil, err
	}
	emails := []string{}
	for _, row := range tables[superAdminsTab] {
		emails = append(emails, row[configEmailColumn])
	}
	s.SuperAdmins = mail.NormalizeAll(emails)
	if len(s.SuperAdmins) == 0 {
		return nil, fmt.Errorf("%s has no super admins", superAdminsTab)
	}
	return s, nil
}
