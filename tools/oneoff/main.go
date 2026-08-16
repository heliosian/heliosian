// Command oneoff drops the duplicated trailing import column and reshapes the Change Log header.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"heliosian/internal/data"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

var changeLogHeader = []string{
	"Timestamp", "Actor",
	"Email", "Added",
	"Full Name", "Legal Name", "Preferred Name",
	"Is Student", "Is Parent", "Is Staff",
	"New to Helios", "Pronouns", "Facts",
	"Grade", "Classroom", "Crew",
	"Phone", "Job Title", "Department", "Grade Band", "Room Parent",
	"Address", "Family Phone", "Family Photo Caption",
}

func main() {
	sheet := flag.String("sheet", "", "spreadsheet id")
	flag.Parse()
	if *sheet == "" {
		log.Fatal("[ERROR] -sheet <spreadsheet id> is required")
	}
	svc, err := sheets.NewService(context.Background(),
		option.WithCredentialsFile(data.KeyFile),
		option.WithScopes(sheets.SpreadsheetsScope))
	if err != nil {
		log.Fatalf("[ERROR] create sheets client: %v", err)
	}

	meta, err := svc.Spreadsheets.Get(*sheet).Fields("sheets(properties(sheetId,title))").Do()
	if err != nil {
		log.Fatalf("[ERROR] get spreadsheet: %v", err)
	}
	importID := int64(-1)
	for _, s := range meta.Sheets {
		if s.Properties.Title == "Veracross Import" {
			importID = s.Properties.SheetId
		}
	}
	if importID < 0 {
		log.Fatal("[ERROR] no Veracross Import tab")
	}

	resp, err := svc.Spreadsheets.Values.Get(*sheet, "'Veracross Import'!AD1:AD1000").Do()
	if err != nil {
		log.Fatalf("[ERROR] read column AD: %v", err)
	}
	for i, row := range resp.Values {
		if i == 0 {
			if len(row) == 0 || fmt.Sprint(row[0]) != "household_2_person_2_phone_business" {
				log.Fatalf("[ERROR] column AD header is %v, not the expected duplicate", row)
			}
			continue
		}
		if len(row) > 0 && fmt.Sprint(row[0]) != "" {
			log.Fatalf("[ERROR] column AD row %d has data %q; not deleting", i+1, row[0])
		}
	}
	_, err = svc.Spreadsheets.BatchUpdate(*sheet, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{{DeleteDimension: &sheets.DeleteDimensionRequest{
			Range: &sheets.DimensionRange{
				SheetId:    importID,
				Dimension:  "COLUMNS",
				StartIndex: 29,
				EndIndex:   30,
			},
		}}},
	}).Do()
	if err != nil {
		log.Fatalf("[ERROR] delete column AD: %v", err)
	}
	log.Print("deleted duplicate import column AD")

	values := make([]interface{}, len(changeLogHeader))
	for i, h := range changeLogHeader {
		values[i] = h
	}
	_, err = svc.Spreadsheets.Values.Update(*sheet, "'Change Log'!1:1", &sheets.ValueRange{
		Values: [][]interface{}{values},
	}).ValueInputOption("RAW").Do()
	if err != nil {
		log.Fatalf("[ERROR] rewrite change log header: %v", err)
	}
	log.Printf("rewrote Change Log header with %d columns", len(changeLogHeader))
}
