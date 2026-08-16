// Command oneoff appends the Opted Out column to the Overrides and Change Log tabs.
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
	meta, err := svc.Spreadsheets.Get(*sheet).Fields("sheets(properties(sheetId,title,gridProperties(columnCount)))").Do()
	if err != nil {
		log.Fatalf("[ERROR] get spreadsheet: %v", err)
	}
	for _, tab := range []string{"Overrides", "Change Log"} {
		var props *sheets.SheetProperties
		for _, s := range meta.Sheets {
			if s.Properties.Title == tab {
				props = s.Properties
			}
		}
		if props == nil {
			log.Fatalf("[ERROR] no %s tab", tab)
		}
		resp, err := svc.Spreadsheets.Values.Get(*sheet, fmt.Sprintf("'%s'!1:1", tab)).Do()
		if err != nil {
			log.Fatalf("[ERROR] read %s header: %v", tab, err)
		}
		width := len(resp.Values[0])
		for _, cell := range resp.Values[0] {
			if fmt.Sprint(cell) == "Opted Out" {
				log.Fatalf("[ERROR] %s already has an Opted Out column", tab)
			}
		}
		if int64(width) >= props.GridProperties.ColumnCount {
			_, err = svc.Spreadsheets.BatchUpdate(*sheet, &sheets.BatchUpdateSpreadsheetRequest{
				Requests: []*sheets.Request{{AppendDimension: &sheets.AppendDimensionRequest{
					SheetId:   props.SheetId,
					Dimension: "COLUMNS",
					Length:    1,
				}}},
			}).Do()
			if err != nil {
				log.Fatalf("[ERROR] widen %s: %v", tab, err)
			}
		}
		cell := fmt.Sprintf("'%s'!%s1", tab, columnName(width))
		_, err = svc.Spreadsheets.Values.Update(*sheet, cell, &sheets.ValueRange{
			Values: [][]interface{}{{"Opted Out"}},
		}).ValueInputOption("RAW").Do()
		if err != nil {
			log.Fatalf("[ERROR] write %s header: %v", tab, err)
		}
		log.Printf("added Opted Out to %s at %s", tab, cell)
	}
}

func columnName(idx int) string {
	name := ""
	for idx >= 0 {
		name = string(rune('A'+idx%26)) + name
		idx = idx/26 - 1
	}
	return name
}
