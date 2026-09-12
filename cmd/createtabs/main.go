// Command createtabs creates each spreadsheet's tabs with their header rows, taking
// the spreadsheet ids from the environment like every other tool here.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"heliosian/internal/birthday"
	"heliosian/internal/events"
	"heliosian/internal/home"
)

// spreadsheets pairs each layout with the variable naming the spreadsheet it belongs
// to. Preferences is a Google Form's own response sheet and Invites is authored by
// hand, so neither has a layout to apply.
var spreadsheets = []struct{ env, layout string }{
	{"DIRECTORY_SHEET", "Directory"},
	{"APPS_SHEET", "Apps"},
	{"EVENTS_SHEET", "Events"},
	{"BIRTHDAY_SHEET", "Birthdays"},
	{"CONFIG_SHEET", "Config"},
}

type tab struct {
	title  string
	header []string
}

// seeds are the rows a fresh tab starts with, where the app expects a row to
// exist: the Apps sheet's events section (see docs/home/data.md).
var seeds = map[string]map[string][]string{
	"Apps": {"Categories": {home.EventsTitle, home.EventsEmoji, home.StyleEvents}},
}

var layouts = map[string][]tab{
	"Directory": {
		// person_photo and person_department are spliced in by vcexport rather than
		// exported by Veracross, so the tab needs the columns before an import can
		// mirror it. Nothing reads the department; it is carried to be compared with
		// the one the school files someone under, which lives in Overrides.
		{"Veracross Staff Import", []string{
			"entry_sort_name", "person_full_name", "person_job_title", "person_room",
			"person_classifications", "person_biography",
			"person_email", "person_email_2", "person_phone_business", "person_photo",
			"person_department",
		}},
		// student_photo is spliced in by vcexport rather than exported by Veracross, so the
		// tab needs the column before an import can mirror it.
		{"Veracross Student Import", []string{
			"entry_sort_name", "student_full_name", "student_classifications", "student_email",
			"student_phone_mobile",
			"household_1_phone", "household_1_address",
			"household_1_person_1_full_name", "household_1_person_1_email",
			"household_1_person_1_email_2", "household_1_person_1_phone_mobile",
			"household_1_person_1_phone_business",
			"household_1_person_2_full_name", "household_1_person_2_email",
			"household_1_person_2_email_2", "household_1_person_2_phone_mobile",
			"household_1_person_2_phone_business",
			"household_2_phone", "household_2_address",
			"household_2_person_1_full_name", "household_2_person_1_email",
			"household_2_person_1_email_2", "household_2_person_1_phone_mobile",
			"household_2_person_1_phone_business",
			"household_2_person_2_full_name", "household_2_person_2_email",
			"household_2_person_2_email_2", "household_2_person_2_phone_mobile",
			"household_2_person_2_phone_business",
			"student_photo",
		}},
		{"Name to Email", []string{"Name", "Email"}},
		{"Email Aliases", []string{"Alias", "Email"}},
		{"Overrides", []string{
			"Email", "Added",
			"Full Name", "Legal Name", "Preferred Name",
			"Is Student", "Is Parent", "Is Staff",
			"New to Helios", "Pronouns", "Facts",
			"Grade", "Classroom", "Crew",
			"Phone", "Job Title", "Department", "Grade Band", "Room Parent",
			"Opted Out", "Photo Updated", "Facts Updated",
			"Veracross Photo", "Primary Photo", "Pronunciation",
		}},
		{"Families", []string{
			"Email", "Address", "Family Phone", "Family Photo Caption",
			"Family Photo Updated", "Family Photo", "Family Photo Crop", "Family Pronunciation",
		}},
		{"Change Log", []string{
			"Timestamp", "Actor",
			"Email", "Added",
			"Full Name", "Legal Name", "Preferred Name",
			"Is Student", "Is Parent", "Is Staff",
			"New to Helios", "Pronouns", "Facts",
			"Grade", "Classroom", "Crew",
			"Phone", "Job Title", "Department", "Grade Band", "Room Parent",
			"Address", "Family Phone", "Family Photo Caption", "Opted Out",
			"Photo Updated", "Facts Updated", "Family Photo Updated",
			"Veracross Photo", "Primary Photo", "Pronunciation",
			"Family Photo", "Family Pronunciation",
		}},
		{"Website Staff Import", []string{
			"constituent_id", "full_name", "title", "departments", "email", "bio", "photo",
		}},
		{"Tags", []string{"Owner Email", "Tag", "Person Email"}},
		{"Photos", []string{"Email", "Photo Name"}},
		{"Admins", []string{"Email"}},
	},
	"Apps": {
		{"Categories", []string{"Title", "Emoji", "Style", "Max"}},
		{"Links", []string{"Title", "Description", "URL", "Image", "Category", "Visible", "Added By", "Added"}},
		{"Admins", []string{"Email"}},
		{"Change Log", []string{"Timestamp", "Actor", "Action", "Kind", "Title", "Description", "URL", "Image", "Category", "Visible", "Style"}},
	},
	"Events": {
		{"Categories", events.CategoryColumns},
		{"Activities", append(events.ActivityColumns, events.OrderColumn)},
		{"Volunteers", events.VolunteerColumns},
		{"Links", events.LinkColumns},
		{"Settings", events.SettingColumns},
		{"Admins", events.AdminColumns},
		{"Redirects", events.RedirectColumns},
		{"Change Log", events.ChangeLogColumns},
	},
	"Birthdays": {
		{"Birthdays", birthday.BirthdayColumns},
		{"Assignments", birthday.AssignmentColumns},
		{"Outreach", birthday.OutreachColumns},
		{"Donations", birthday.DonationColumns},
		{"Notes", birthday.NoteColumns},
		{"Charities", birthday.CharityColumns},
		{"Newsletter Dates", birthday.NewsletterDateColumns},
		{"Settings", birthday.SettingColumns},
		{"Admins", birthday.AdminColumns},
		{"Change Log", birthday.ChangeLogColumns},
	},
	"Config": {
		{"Settings", []string{"Key", "Value"}},
		{"Super Admins", []string{"Email"}},
		{"Grade Colors", []string{"Grade", "Color"}},
		{"Classroom Colors", []string{"Classroom", "Color"}},
	},
}

func column(i int) string {
	name := ""
	for i >= 0 {
		name = string(rune('A'+i%26)) + name
		i = i/26 - 1
	}
	return name
}

// addMissingColumns appends headings a tab does not have yet, widening the grid first
// since a tab is only as wide as it was created. Existing columns are never moved, so
// every row's data stays under the heading it was written for.
func addMissingColumns(svc *sheets.Service, sheet, title string, id, grid int64, header []string) (int, error) {
	quoted := "'" + strings.ReplaceAll(title, "'", "''") + "'"
	resp, err := svc.Spreadsheets.Values.Get(sheet, quoted+"!1:1").Do()
	if err != nil {
		return 0, fmt.Errorf("read header of %q: %w", title, err)
	}
	present := map[string]bool{}
	width := 0
	if len(resp.Values) > 0 {
		width = len(resp.Values[0])
		for _, cell := range resp.Values[0] {
			present[strings.TrimSpace(fmt.Sprint(cell))] = true
		}
	}
	added := []interface{}{}
	for _, name := range header {
		if !present[name] {
			added = append(added, name)
		}
	}
	if len(added) == 0 {
		return 0, nil
	}
	if short := int64(width+len(added)) - grid; short > 0 {
		_, err := svc.Spreadsheets.BatchUpdate(sheet, &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{{AppendDimension: &sheets.AppendDimensionRequest{
				SheetId: id, Dimension: "COLUMNS", Length: short,
			}}},
		}).Do()
		if err != nil {
			return 0, fmt.Errorf("widen %q: %w", title, err)
		}
	}
	_, err = svc.Spreadsheets.Values.Update(sheet, fmt.Sprintf("%s!%s1", quoted, column(width)),
		&sheets.ValueRange{Values: [][]interface{}{added}}).ValueInputOption("RAW").Do()
	if err != nil {
		return 0, fmt.Errorf("add columns to %q: %w", title, err)
	}
	log.Printf("added %d columns to %q: %v", len(added), title, added)
	return len(added), nil
}

// applyLayout brings one spreadsheet up to its layout. The title is checked against
// the layout the variable promised: an id pointing at the wrong document would
// otherwise have tabs added to it before anyone noticed.
// applyLayout reports how many tabs it created and columns it added, so a run that
// changes nothing - which is most of them - says so in one line instead of naming
// every tab it left alone.
func applyLayout(svc *sheets.Service, sheet, env, layout string) (tabs, columns int, err error) {
	meta, err := svc.Spreadsheets.Get(sheet).Fields("properties(title),sheets(properties(sheetId,title,gridProperties(columnCount)))").Do()
	if err != nil {
		return 0, 0, fmt.Errorf("get the spreadsheet %s names: %w", env, err)
	}
	if meta.Properties.Title != layout {
		return 0, 0, fmt.Errorf("%s names spreadsheet %q, want the one titled %q", env, meta.Properties.Title, layout)
	}
	type tabInfo struct{ id, columns int64 }
	existing := map[string]tabInfo{}
	for _, s := range meta.Sheets {
		info := tabInfo{id: s.Properties.SheetId}
		if s.Properties.GridProperties != nil {
			info.columns = s.Properties.GridProperties.ColumnCount
		}
		existing[s.Properties.Title] = info
	}
	for _, t := range layouts[layout] {
		if info, ok := existing[t.title]; ok {
			added, err := addMissingColumns(svc, sheet, t.title, info.id, info.columns, t.header)
			if err != nil {
				return 0, 0, err
			}
			columns += added
			continue
		}
		_, err := svc.Spreadsheets.BatchUpdate(sheet, &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{{AddSheet: &sheets.AddSheetRequest{
				Properties: &sheets.SheetProperties{Title: t.title},
			}}},
		}).Do()
		if err != nil {
			return 0, 0, fmt.Errorf("create tab %q: %w", t.title, err)
		}
		rows := [][]interface{}{cells(t.header)}
		if seed := seeds[layout][t.title]; seed != nil {
			rows = append(rows, cells(seed))
		}
		quoted := "'" + strings.ReplaceAll(t.title, "'", "''") + "'"
		_, err = svc.Spreadsheets.Values.Update(sheet, quoted+"!1:1", &sheets.ValueRange{
			Values: rows,
		}).ValueInputOption("RAW").Do()
		if err != nil {
			return 0, 0, fmt.Errorf("write header of %q: %w", t.title, err)
		}
		log.Printf("created tab %q in %q with %d columns", t.title, layout, len(t.header))
		tabs++
	}
	return tabs, columns, nil
}

func cells(row []string) []interface{} {
	out := make([]interface{}, len(row))
	for i, c := range row {
		out[i] = c
	}
	return out
}

func main() {
	ids := map[string]string{}
	for _, s := range spreadsheets {
		id := os.Getenv(s.env)
		if id == "" {
			log.Fatalf("[ERROR] %s is required", s.env)
		}
		ids[s.env] = id
	}
	svc, err := sheets.NewService(context.Background(),
		option.WithScopes(sheets.SpreadsheetsScope))
	if err != nil {
		log.Fatalf("[ERROR] create sheets client: %v", err)
	}
	tabs, columns := 0, 0
	for _, s := range spreadsheets {
		created, added, err := applyLayout(svc, ids[s.env], s.env, s.layout)
		if err != nil {
			log.Fatalf("[ERROR] %v", err)
		}
		tabs += created
		columns += added
	}
	log.Printf("checked %d spreadsheets: %d tabs created, %d columns added", len(spreadsheets), tabs, columns)
}
