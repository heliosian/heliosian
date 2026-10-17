package spreadsheets

import "heliosian/internal/env"

type Spreadsheet struct {
	Source string
	Env    string
	Title  string
}

var All = []Spreadsheet{
	{"preferences", "PREFERENCES_SHEET", "Preferences"},
	{"birthdayshared", "BIRTHDAY_SHARED_SHEET", "Staff Birthday List (Shared)"},
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
