// Command dropcolumns deletes named columns from one tab of a sheet, header
// and cells alike, for a column an app has stopped reading - so nobody takes
// what is left in it for a setting still in force. Without -apply it only
// says what it would delete.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sort"
	"strings"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func main() {
	sheet := flag.String("sheet", "", "spreadsheet id")
	tab := flag.String("tab", "", "tab title")
	columns := flag.String("columns", "", "column headers to delete, comma-separated")
	apply := flag.Bool("apply", false, "delete the columns; without it the run only reports them")
	flag.Parse()
	if *sheet == "" || *tab == "" || *columns == "" {
		log.Fatal("[ERROR] -sheet, -tab, and -columns are required")
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
		if s.Properties.Title == *tab {
			id = s.Properties.SheetId
		}
	}
	if id < 0 {
		log.Fatalf("[ERROR] no tab titled %q", *tab)
	}
	quoted := "'" + strings.ReplaceAll(*tab, "'", "''") + "'"
	resp, err := svc.Spreadsheets.Values.Get(*sheet, quoted).Do()
	if err != nil {
		log.Fatalf("[ERROR] read tab %s: %v", *tab, err)
	}
	if len(resp.Values) == 0 {
		log.Fatalf("[ERROR] tab %q has no header row", *tab)
	}
	header := map[string]int{}
	for i, cell := range resp.Values[0] {
		header[strings.TrimSpace(fmt.Sprint(cell))] = i
	}
	var indexes []int
	for _, name := range strings.Split(*columns, ",") {
		name = strings.TrimSpace(name)
		i, ok := header[name]
		if !ok {
			log.Fatalf("[ERROR] tab %q has no column %q", *tab, name)
		}
		filled := 0
		for _, row := range resp.Values[1:] {
			if i < len(row) && strings.TrimSpace(fmt.Sprint(row[i])) != "" {
				filled++
			}
		}
		log.Printf("column %q (%d) holds %d filled cells in %d rows", name, i, filled, len(resp.Values)-1)
		indexes = append(indexes, i)
	}
	if !*apply {
		log.Printf("dry run: pass -apply to delete %d columns from %q", len(indexes), *tab)
		return
	}
	// Descending, so deleting a column never shifts one still queued behind it.
	sort.Sort(sort.Reverse(sort.IntSlice(indexes)))
	requests := []*sheets.Request{}
	for _, i := range indexes {
		requests = append(requests, &sheets.Request{DeleteDimension: &sheets.DeleteDimensionRequest{
			Range: &sheets.DimensionRange{SheetId: id, Dimension: "COLUMNS", StartIndex: int64(i), EndIndex: int64(i + 1)},
		}})
	}
	if _, err := svc.Spreadsheets.BatchUpdate(*sheet, &sheets.BatchUpdateSpreadsheetRequest{Requests: requests}).Do(); err != nil {
		log.Fatalf("[ERROR] delete columns from %q: %v", *tab, err)
	}
	log.Printf("deleted %d columns from %q: %s", len(indexes), *tab, *columns)
}
