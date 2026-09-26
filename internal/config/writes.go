package config

import (
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

func requireAdmin(actor access.Actor) error {
	if !actor.Admin {
		return access.Forbidden("admin access required")
	}
	return nil
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

func (s *Settings) setStaleYears(actor access.Actor, years StaleYears) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	if years.Photo <= 0 || years.Facts <= 0 || years.FamilyPhoto <= 0 {
		return nil, access.Invalid("thresholds must be positive numbers of years")
	}
	return setSettings(map[string]string{
		PhotoStaleYears:       FormatYears(years.Photo),
		FactsStaleYears:       FormatYears(years.Facts),
		FamilyPhotoStaleYears: FormatYears(years.FamilyPhoto),
	}), nil
}

func (s *Settings) setPrivacyLinks(actor access.Actor, links PrivacyLinks) (PrivacyLinks, []store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return PrivacyLinks{}, nil, err
	}
	links.VeracrossPreferences = strings.TrimSpace(links.VeracrossPreferences)
	links.HeliosWhoOptIn = strings.TrimSpace(links.HeliosWhoOptIn)
	if !strings.HasPrefix(links.VeracrossPreferences, "https://") || !strings.HasPrefix(links.HeliosWhoOptIn, "https://") {
		return PrivacyLinks{}, nil, access.Invalid("both links must be full https:// URLs")
	}
	return links, setSettings(map[string]string{
		VeracrossPreferences: links.VeracrossPreferences,
		HeliosWhoOptIn:       links.HeliosWhoOptIn,
	}), nil
}

func (s *Settings) setColor(actor access.Actor, kind, name, color string) (string, []store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return "", nil, err
	}
	if !HexColor.MatchString(color) {
		return "", nil, access.Invalid("color must be a #rrggbb hex value")
	}
	name = strings.TrimSpace(name)
	if kind != "staff" && name == "" {
		return "", nil, access.Invalid("missing name")
	}
	switch kind {
	case "classroom":
		return name, []store.Op{store.Set(ClassroomColorsTab, store.Row{ClassroomColumn: name}, store.Row{ColorColumn: color})}, nil
	case "grade":
		return name, []store.Op{store.Set(GradeColorsTab, store.Row{GradeColumn: name}, store.Row{ColorColumn: color})}, nil
	case "staff":
		return name, setSettings(map[string]string{StaffColor: color}), nil
	}
	return "", nil, access.Invalid("bad kind: must be classroom, grade, or staff")
}

func (s *Settings) setSuperAdmins(actor access.Actor, emails []string) ([]string, []store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, nil, err
	}
	admins := NormalizeEmails(emails)
	if len(admins) == 0 {
		return nil, nil, access.Invalid("the super admin list cannot be empty")
	}
	ops := []store.Op{}
	for _, e := range s.SuperAdmins {
		if !slices.Contains(admins, e) {
			ops = append(ops, store.Delete(SuperAdminsTab, store.Row{EmailColumn: e}))
		}
	}
	for _, e := range admins {
		if !slices.Contains(s.SuperAdmins, e) {
			ops = append(ops, store.Insert(SuperAdminsTab, store.Row{EmailColumn: e}))
		}
	}
	return admins, ops, nil
}

func signedOut(email string) []store.Op {
	at := time.Now().Truncate(time.Second)
	return []store.Op{store.Set(SignedOutTab, store.Row{EmailColumn: email}, store.Row{TimeColumn: at.Format(time.RFC3339)})}
}

func (s *Settings) signOut(actor access.Actor, email string) (string, []store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return "", nil, err
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") {
		return "", nil, access.Invalid("missing email")
	}
	return email, signedOut(email), nil
}

func (s *Settings) signOutSelf(actor access.Actor) []store.Op {
	return signedOut(actor.Email)
}
