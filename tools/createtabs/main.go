// Command createtabs creates the directory sheet's local-layer tabs with their header rows.
package main

import (
	"context"
	"flag"
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
	}},
	{"Tags", []string{"Owner Email", "Tag", "Person Email"}},
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
	meta, err := svc.Spreadsheets.Get(*sheet).Fields("sheets(properties(title))").Do()
	if err != nil {
		log.Fatalf("[ERROR] get spreadsheet: %v", err)
	}
	existing := map[string]bool{}
	for _, s := range meta.Sheets {
		existing[s.Properties.Title] = true
	}
	for _, t := range tabs {
		if existing[t.title] {
			log.Printf("tab %q already exists", t.title)
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
