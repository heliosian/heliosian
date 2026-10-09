package spreadsheets

import "heliosian/internal/env"

type Spreadsheet struct {
	Source string
	Env    string
	Title  string
}

var All = []Spreadsheet{
	{"directory", "DIRECTORY_SHEET", "Directory"},
	{"preferences", "PREFERENCES_SHEET", "Preferences"},
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
	{"datapeople", "DATA_PEOPLE_SHEET", "Data: People"},
	{"datagroups", "DATA_GROUPS_SHEET", "Data: Groups"},
	{"datadocuments", "DATA_DOCUMENTS_SHEET", "Data: Documents"},
	{"datamail", "DATA_MAIL_SHEET", "Data: Mail"},
	{"dataconfig", "DATA_CONFIG_SHEET", "Data: Config"},
}

func IDs(sheets []Spreadsheet) map[string]string {
	ids := map[string]string{}
	for _, s := range sheets {
		ids[s.Source] = env.Required(s.Env)
	}
	return ids
}
