// Command renametab renames one tab of a sheet.
package main

import (
	"context"
	"flag"
	"log"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func main() {
	sheet := flag.String("sheet", "", "spreadsheet id")
	from := flag.String("from", "", "current tab title")
	to := flag.String("to", "", "new tab title")
	flag.Parse()
	if *sheet == "" || *from == "" || *to == "" {
		log.Fatal("[ERROR] -sheet, -from, and -to are required")
	}
	svc, err := sheets.NewService(context.Background(),
		option.WithScopes(sheets.SpreadsheetsScope))
	if err != nil {
		log.Fatalf("[ERROR] create sheets client: %v", err)
	}
	meta, err := svc.Spreadsheets.Get(*sheet).Fields("sheets(properties(sheetId,title))").Do()
	if err != nil {
		log.Fatalf("[ERROR] get spreadsheet: %v", err)
	}
	var id int64 = -1
	for _, s := range meta.Sheets {
		if s.Properties.Title == *to {
			log.Fatalf("[ERROR] tab %q already exists", *to)
		}
		if s.Properties.Title == *from {
			id = s.Properties.SheetId
		}
	}
	if id < 0 {
		log.Fatalf("[ERROR] no tab titled %q", *from)
	}
	_, err = svc.Spreadsheets.BatchUpdate(*sheet, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{{UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
			Properties: &sheets.SheetProperties{SheetId: id, Title: *to},
			Fields:     "title",
		}}},
	}).Do()
	if err != nil {
		log.Fatalf("[ERROR] rename %q: %v", *from, err)
	}
	log.Printf("renamed %q to %q", *from, *to)
}
