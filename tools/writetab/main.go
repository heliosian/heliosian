// Command writetab writes a local CSV into an empty sheet tab below its matching header row.
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func main() {
	sheet := flag.String("sheet", "", "spreadsheet id")
	tab := flag.String("tab", "", "tab title")
	in := flag.String("in", "", "input csv path (first row must match the tab header)")
	appendRows := flag.Bool("append", false, "append below existing rows instead of requiring an empty tab")
	flag.Parse()
	if *sheet == "" || *tab == "" || *in == "" {
		log.Fatal("[ERROR] -sheet, -tab, and -in are required")
	}
	f, err := os.Open(*in)
	if err != nil {
		log.Fatalf("[ERROR] open %s: %v", *in, err)
	}
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		log.Fatalf("[ERROR] read %s: %v", *in, err)
	}
	if err := f.Close(); err != nil {
		log.Fatalf("[ERROR] close %s: %v", *in, err)
	}
	if len(records) < 2 {
		log.Fatalf("[ERROR] %s has no data rows", *in)
	}
	svc, err := sheets.NewService(context.Background(),
		option.WithScopes(sheets.SpreadsheetsScope))
	if err != nil {
		log.Fatalf("[ERROR] create sheets client: %v", err)
	}
	quoted := "'" + strings.ReplaceAll(*tab, "'", "''") + "'"
	resp, err := svc.Spreadsheets.Values.Get(*sheet, quoted).Do()
	if err != nil {
		log.Fatalf("[ERROR] read tab %s: %v", *tab, err)
	}
	if len(resp.Values) == 0 {
		log.Fatalf("[ERROR] tab %s has no header row", *tab)
	}
	if !*appendRows && len(resp.Values) != 1 {
		log.Fatalf("[ERROR] tab %s has %d rows; want exactly the header row", *tab, len(resp.Values))
	}
	header := make([]string, len(resp.Values[0]))
	for i, cell := range resp.Values[0] {
		header[i] = strings.TrimSpace(fmt.Sprint(cell))
	}
	if strings.Join(header, "\x00") != strings.Join(records[0], "\x00") {
		log.Fatalf("[ERROR] header mismatch:\n tab: %q\n csv: %q", header, records[0])
	}
	values := make([][]interface{}, len(records)-1)
	for i, rec := range records[1:] {
		row := make([]interface{}, len(rec))
		for j, cell := range rec {
			row[j] = cell
		}
		values[i] = row
	}
	if *appendRows {
		_, err = svc.Spreadsheets.Values.Append(*sheet, quoted, &sheets.ValueRange{
			Values: values,
		}).ValueInputOption("RAW").InsertDataOption("INSERT_ROWS").Do()
	} else {
		_, err = svc.Spreadsheets.Values.Update(*sheet, quoted+"!A2", &sheets.ValueRange{
			Values: values,
		}).ValueInputOption("RAW").Do()
	}
	if err != nil {
		log.Fatalf("[ERROR] write rows: %v", err)
	}
	log.Printf("wrote %d rows to %s", len(values), *tab)
}
