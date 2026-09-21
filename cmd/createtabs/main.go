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

	"heliosian/internal/artifacts"
	"heliosian/internal/birthday"
	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
	"heliosian/internal/feedback"
	"heliosian/internal/home"
	"heliosian/internal/loop"
	"heliosian/internal/team"
)

// spreadsheets pairs each layout with the variable naming the spreadsheet it belongs
// to. Preferences is a Google Form's own response sheet and Invites is authored by
// hand, so neither has a layout to apply.
var spreadsheets = []struct{ env, layout string }{
	{"DIRECTORY_SHEET", "Directory"},
	{"APPS_SHEET", "Apps"},
	{"EVENTS_SHEET", "Events"},
	{"BIRTHDAY_SHEET", "Birthdays"},
	{"CALENDAR_SHEET", "Calendar"},
	{"CELEBRATE_SHEET", "Celebrate"},
	{"CONFIG_SHEET", "Config"},
	{"GROUPS_SHEET", "Groups"},
	{"ARTIFACTS_SHEET", "Artifacts"},
	{"FEEDBACK_SHEET", "Feedback"},
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
	"Artifacts": {
		{"Documents", artifacts.DocumentColumns},
	},
	"Feedback": {
		{"Reports", feedback.ReportColumns},
	},
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
		{"Geocode", []string{"Address", "Lat", "Lng"}},
	},
	"Apps": {
		{"Categories", home.CategoryColumns},
		{"Links", home.LinkColumns},
		{"Admins", []string{"Email"}},
		{"Visibility", home.VisibilityColumns},
		{"Audience", home.AudienceColumns},
		{"Change Log", home.ChangeLogColumns},
	},
	"Events": {
		{"Categories", team.CategoryColumns},
		{"Activities", append(team.ActivityColumns, team.OrderColumn, team.CompleteColumn)},
		{"Volunteers", team.VolunteerColumns},
		{"Links", team.LinkColumns},
		{"Settings", team.SettingColumns},
		{"Admins", team.AdminColumns},
		{"Redirects", team.RedirectColumns},
		{"Change Log", team.ChangeLogColumns},
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
		{"Team", birthday.TeamColumns},
		{"Reminders", birthday.ReminderColumns},
		{"Change Log", birthday.ChangeLogColumns},
	},
	"Celebrate": {
		{"Celebrations", celebrate.CelebrationColumns},
		{"Categories", celebrate.CategoryColumns},
		{"Parties", celebrate.PartyColumns},
		{"Hosts", celebrate.HostColumns},
		{"Tickets", celebrate.TicketColumns},
		{"Settings", celebrate.SettingColumns},
		{"Admins", celebrate.AdminColumns},
		{"Redirects", celebrate.RedirectColumns},
		{"Change Log", celebrate.ChangeLogColumns},
		{"INVOICING", celebrate.InvoicingColumns},
	},
	"Calendar": {
		{calendar.GoogleTab, calendar.GoogleColumns},
		{calendar.PDFTab, calendar.PDFColumns},
		{calendar.EventsTab, calendar.EventColumns},
		{calendar.EnrichmentTab, calendar.EnrichmentColumns},
		{calendar.OverridesTab, calendar.OverrideColumns},
		{calendar.DayTypesTab, calendar.DayTypeColumns},
		{calendar.DayOverridesTab, calendar.DayOverrideColumns},
		{calendar.TagsTab, calendar.TagColumns},
		{calendar.AdminsTab, calendar.AdminColumns},
		{calendar.FeedsTab, calendar.FeedColumns},
		{calendar.SettingsTab, calendar.SettingColumns},
		{calendar.RSVPsTab, calendar.RSVPColumns},
		{calendar.InvitationsTab, calendar.InvitationColumns},
		{calendar.InvitesTab, calendar.InviteColumns},
		{calendar.InviteGroupsTab, calendar.InviteGroupColumns},
		{calendar.BouncesTab, calendar.BounceColumns},
		{calendar.ChangeLogTab, calendar.ChangeLogColumns},
	},
	"Config": {
		{"Settings", []string{"Key", "Value"}},
		{"Super Admins", []string{"Email"}},
		{"Grade Colors", []string{"Grade", "Color"}},
		{"Classroom Colors", []string{"Classroom", "Color"}},
	},
	"Groups": {
		{"Groups", loop.GroupColumns},
		{"Managers", loop.ManagerColumns},
		{"Rules", loop.RuleColumns},
		{"Additions", loop.AdditionColumns},
		{"Excluded", loop.ExcludedColumns},
		{"Aliases", loop.AliasColumns},
		{"Messages", loop.MessageColumns},
		{"Deliveries", loop.DeliveryColumns},
		{"Admins", loop.AdminColumns},
		{"Archived", loop.ArchivedColumns},
		{"Change Log", loop.ChangeLogColumns},
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

func quoteTab(title string) string {
	return "'" + strings.ReplaceAll(title, "'", "''") + "'"
}

// addMissingColumns appends headings a tab does not have yet, given the header row
// it has now, widening the grid first since a tab is only as wide as it was created.
// Existing columns are never moved, so every row's data stays under the heading it
// was written for.
func addMissingColumns(svc *sheets.Service, sheet, title string, id, grid int64, current []interface{}, header []string) (int, error) {
	quoted := quoteTab(title)
	present := map[string]bool{}
	width := len(current)
	for _, cell := range current {
		present[strings.TrimSpace(fmt.Sprint(cell))] = true
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
	if _, err := svc.Spreadsheets.Values.Update(sheet, fmt.Sprintf("%s!%s1", quoted, column(width)),
		&sheets.ValueRange{Values: [][]interface{}{added}}).ValueInputOption("RAW").Do(); err != nil {
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
// every tab it left alone. The tabs' headers come in one batched read per
// spreadsheet, the way the server's startup reads, so a run stays inside the
// Sheets API's sixty reads a minute however many tabs the layouts hold.
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
	ranges := []string{}
	for _, t := range layouts[layout] {
		if _, ok := existing[t.title]; ok {
			ranges = append(ranges, quoteTab(t.title)+"!1:1")
		}
	}
	headers := map[string][]interface{}{}
	if len(ranges) > 0 {
		resp, err := svc.Spreadsheets.Values.BatchGet(sheet).Ranges(ranges...).Do()
		if err != nil {
			return 0, 0, fmt.Errorf("read the headers of %s: %w", env, err)
		}
		if len(resp.ValueRanges) != len(ranges) {
			return 0, 0, fmt.Errorf("read the headers of %s: %d ranges asked, %d answered", env, len(ranges), len(resp.ValueRanges))
		}
		i := 0
		for _, t := range layouts[layout] {
			if _, ok := existing[t.title]; !ok {
				continue
			}
			if len(resp.ValueRanges[i].Values) > 0 {
				headers[t.title] = resp.ValueRanges[i].Values[0]
			}
			i++
		}
	}
	for _, t := range layouts[layout] {
		if info, ok := existing[t.title]; ok {
			added, err := addMissingColumns(svc, sheet, t.title, info.id, info.columns, headers[t.title], t.header)
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
		_, err = svc.Spreadsheets.Values.Update(sheet, quoteTab(t.title)+"!1:1", &sheets.ValueRange{
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
