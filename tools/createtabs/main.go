// Command createtabs creates the directory sheet's local-layer tabs with their header rows.
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

var tabs = []struct {
	title  string
	header []string
}{
	{"Veracross Staff Import", []string{
		"entry_sort_name", "person_full_name", "person_job_title", "person_room",
		"person_classifications", "person_biography",
		"person_email", "person_email_2", "person_phone_business",
	}},
	{"Name to Email", []string{"Name", "Email"}},
	{"Overrides", []string{
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
	meta, err := svc.Spreadsheets.Get(*sheet).Fields("sheets(properties(sheetId,title,gridProperties(columnCount)))").Do()
	if err != nil {
		log.Fatalf("[ERROR] get spreadsheet: %v", err)
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
