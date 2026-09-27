package spreadsheets

import (
	"slices"

	"heliosian/internal/env"
	"heliosian/internal/logging"
)

type Spreadsheet struct {
	Source string
	Env    string
	Title  string
}

var All = []Spreadsheet{
	{"directory", "DIRECTORY_SHEET", "Directory"},
	{"preferences", "PREFERENCES_SHEET", "Preferences"},
	{"invites", "INVITES_SHEET", "Invite List Builder"},
	{"apps", "APPS_SHEET", "Apps"},
	{"events", "EVENTS_SHEET", "Events"},
	{"birthdays", "BIRTHDAY_SHEET", "Birthdays"},
	{"birthdayshared", "BIRTHDAY_SHARED_SHEET", "Staff Birthday List (Shared)"},
	{"celebrate", "CELEBRATE_SHEET", "Celebrate"},
	{"calendar", "CALENDAR_SHEET", "Calendar"},
	{"config", "CONFIG_SHEET", "Config"},
	{"groups", "GROUPS_SHEET", "Groups"},
	{"artifacts", "ARTIFACTS_SHEET", "Artifacts"},
	{"feedback", "FEEDBACK_SHEET", "Feedback"},
}

var SyncSources = []string{"calendar", "directory", "preferences", "config"}

func Of(sources []string) []Spreadsheet {
	out := []Spreadsheet{}
	for _, source := range sources {
		i := slices.IndexFunc(All, func(s Spreadsheet) bool { return s.Source == source })
		if i < 0 {
			logging.Fatal("unknown spreadsheet source", "source", source)
		}
		out = append(out, All[i])
	}
	return out
}

func IDs(sheets []Spreadsheet) map[string]string {
	ids := map[string]string{}
	for _, s := range sheets {
		ids[s.Source] = env.Required(s.Env)
	}
	return ids
}
