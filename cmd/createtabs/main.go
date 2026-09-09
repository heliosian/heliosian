// Command createtabs creates a sheet's tabs with their header rows, picking the layout by the spreadsheet's title.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

type tab struct {
	title  string
	header []string
}

var layouts = map[string][]tab{
	"Directory": {
		{"Veracross Staff Import", []string{
			"entry_sort_name", "person_full_name", "person_job_title", "person_room",
			"person_classifications", "person_biography",
			"person_email", "person_email_2", "person_phone_business", "person_photo",
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
		{"Tags", []string{"Owner Email", "Tag", "Person Email"}},
		{"Photos", []string{"Email", "Photo Name"}},
		{"Admins", []string{"Email"}},
	},
	"Apps": {
		{"Categories", []string{"Title", "Image"}},
		{"Links", []string{"Title", "Description", "URL", "Image", "Category", "Visible", "Added By", "Added"}},
		{"Admins", []string{"Email"}},
		{"Change Log", []string{"Timestamp", "Actor", "Action", "Kind", "Title", "Description", "URL", "Image", "Category", "Visible"}},
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
func addMissingColumns(svc *sheets.Service, sheet, title string, id, grid int64, header []string) error {
	quoted := "'" + strings.ReplaceAll(title, "'", "''") + "'"
	resp, err := svc.Spreadsheets.Values.Get(sheet, quoted+"!1:1").Do()
	if err != nil {
		return fmt.Errorf("read header of %q: %w", title, err)
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
		log.Printf("tab %q already has every column", title)
		return nil
	}
	if short := int64(width+len(added)) - grid; short > 0 {
		_, err := svc.Spreadsheets.BatchUpdate(sheet, &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{{AppendDimension: &sheets.AppendDimensionRequest{
				SheetId: id, Dimension: "COLUMNS", Length: short,
			}}},
		}).Do()
		if err != nil {
			return fmt.Errorf("widen %q: %w", title, err)
		}
	}
	_, err = svc.Spreadsheets.Values.Update(sheet, fmt.Sprintf("%s!%s1", quoted, column(width)),
		&sheets.ValueRange{Values: [][]interface{}{added}}).ValueInputOption("RAW").Do()
	if err != nil {
		return fmt.Errorf("add columns to %q: %w", title, err)
	}
	log.Printf("added %d columns to %q: %v", len(added), title, added)
	return nil
}

func main() {
	sheet := flag.String("sheet", "", "spreadsheet id")
	flag.Parse()
	if *sheet == "" {
		log.Fatal("[ERROR] -sheet <spreadsheet id> is required")
	}
	svc, err := sheets.NewService(context.Background(),
		option.WithScopes(sheets.SpreadsheetsScope))
	if err != nil {
		log.Fatalf("[ERROR] create sheets client: %v", err)
	}
	meta, err := svc.Spreadsheets.Get(*sheet).Fields("properties(title),sheets(properties(sheetId,title,gridProperties(columnCount)))").Do()
	if err != nil {
		log.Fatalf("[ERROR] get spreadsheet: %v", err)
	}
	tabs, ok := layouts[meta.Properties.Title]
	if !ok {
		log.Fatalf("[ERROR] spreadsheet %q has no layout here; it must be titled Directory, Apps, or Config", meta.Properties.Title)
	}
	log.Printf("spreadsheet %q: applying the %s layout", meta.Properties.Title, meta.Properties.Title)
	type tabInfo struct{ id, columns int64 }
	existing := map[string]tabInfo{}
	for _, s := range meta.Sheets {
		info := tabInfo{id: s.Properties.SheetId}
		if s.Properties.GridProperties != nil {
			info.columns = s.Properties.GridProperties.ColumnCount
		}
		existing[s.Properties.Title] = info
	}
	for _, t := range tabs {
		if info, ok := existing[t.title]; ok {
			if err := addMissingColumns(svc, *sheet, t.title, info.id, info.columns, t.header); err != nil {
				log.Fatalf("[ERROR] %v", err)
			}
			continue
		}
		_, err := svc.Spreadsheets.BatchUpdate(*sheet, &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{{AddSheet: &sheets.AddSheetRequest{
				Properties: &sheets.SheetProperties{Title: t.title},
			}}},
		}).Do()
		if err != nil {
			log.Fatalf("[ERROR] create tab %q: %v", t.title, err)
		}
		values := make([]interface{}, len(t.header))
		for i, h := range t.header {
			values[i] = h
		}
		quoted := "'" + strings.ReplaceAll(t.title, "'", "''") + "'"
		_, err = svc.Spreadsheets.Values.Update(*sheet, quoted+"!1:1", &sheets.ValueRange{
			Values: [][]interface{}{values},
		}).ValueInputOption("RAW").Do()
		if err != nil {
			log.Fatalf("[ERROR] write header of %q: %v", t.title, err)
		}
		log.Printf("created tab %q with %d columns", t.title, len(t.header))
	}
}
