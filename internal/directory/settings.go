package directory

import (
	"encoding/json"
	"fmt"
	"strings"

	"heliosian/internal/blob"
)

const settingsFolder = "config"
const settingsName = "settings.json"

// Settings holds everything an admin can change without a deploy. It lives as one
// JSON blob in the media bucket rather than a sheet tab, since neither admins nor
// stale-photo thresholds are rows tied to a person or family.
//
// Admins and SuperAdmins are deliberately separate, disjoint lists rather than one
// list with a role flag: a regular admin's view of the admin page is built from an API
// response that a super admin's request never shares a code path with, so there's no
// field to trim or hide — a regular admin's client simply never receives anything
// about super admins to begin with.
type Settings struct {
	SuperAdmins  []string     `json:"superAdmins"`
	Admins       []string     `json:"admins"`
	StaleYears   StaleYears   `json:"staleYears"`
	PrivacyLinks PrivacyLinks `json:"privacyLinks"`

	// GradeColors and ClassroomColors key on the grade/grade-band and classroom names
	// themselves (e.g. "Grade 3", "Egrets") rather than a stable id, matching how
	// Classroom/Grade are recomputed from scratch on every model rebuild rather than
	// persisted rows - there's no id to key on that would survive a rebuild anyway.
	// StaffColor is the fallback for anyone (staff, or a parent whose kids have no
	// grade on record) with no grade-band color to inherit.
	GradeColors     map[string]string `json:"gradeColors,omitempty"`
	ClassroomColors map[string]string `json:"classroomColors,omitempty"`
	StaffColor      string            `json:"staffColor,omitempty"`
}

// defaultSettings seeds a fresh deploy (or sample mode, which never persists) with a
// starting super admin and the thresholds the app shipped with.
func defaultSettings() Settings {
	return Settings{
		SuperAdmins: []string{"gayle.mcdowell@heliosschool.org", "ian.gulliver@heliosschool.org"},
		Admins:      []string{},
		StaleYears:  StaleYears{Photo: 0.75, Facts: 0.6, FamilyPhoto: 1.5},
		PrivacyLinks: PrivacyLinks{
			VeracrossPreferences: "https://portals.veracross.com/heliosschool/parent/directory-preferences",
			HeliosWhoOptIn:       "https://hca.run/optin",
		},
		GradeColors:     map[string]string{},
		ClassroomColors: map[string]string{},
		StaffColor:      "#1f4d53",
	}
}

// loadSettings reads the settings blob if one exists, falling back to defaults when
// there is no store (sample mode), nothing has been saved yet, or the blob is corrupt.
func loadSettings(store *blob.Store) Settings {
	settings := defaultSettings()
	if store == nil {
		return settings
	}
	data, ok := store.Get(settingsFolder, settingsName)
	if !ok {
		return settings
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return defaultSettings()
	}
	return settings
}

func saveSettings(store *blob.Store, settings Settings) error {
	if store == nil {
		return nil
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	return store.PutNamed(settingsFolder, settingsName, "application/json", data)
}

func normalizeEmails(emails []string) []string {
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

// mergedAdmins is what the Admins tab actually shows: every admin, either tier,
// indistinguishable, sorted together. A regular admin's client never learns which
// names came from which list, because there's only one list here.
func mergedAdmins(settings Settings) []string {
	return normalizeEmails(append(append([]string{}, settings.Admins...), settings.SuperAdmins...))
}

// withoutSuperAdmins drops any email that's currently a super admin, so the merged
// list the Admins tab submits back can never write a super admin into settings.Admins
// (or, by their absence from an edited list, be mistaken for removed from
// settings.SuperAdmins — that list is untouched here regardless).
func withoutSuperAdmins(emails, superAdmins []string) []string {
	super := map[string]bool{}
	for _, e := range superAdmins {
		super[e] = true
	}
	out := []string{}
	for _, e := range emails {
		if !super[e] {
			out = append(out, e)
		}
	}
	return out
}
