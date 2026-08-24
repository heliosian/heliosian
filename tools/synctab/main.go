// Command synctab updates a sheet tab from a CSV cell by cell, matching rows by a key column.
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"heliosian/internal/sheetsync"
)

func readCSV(path string) ([]string, []map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(records) < 2 {
		return nil, nil, fmt.Errorf("%s has no data rows", path)
	}
	rows := []map[string]string{}
	for _, rec := range records[1:] {
		row := map[string]string{}
		for i, name := range records[0] {
			if i < len(rec) {
				row[name] = rec[i]
			}
		}
		rows = append(rows, row)
	}
	return records[0], rows, nil
}

func main() {
	sheet := flag.String("sheet", "", "spreadsheet id")
	tab := flag.String("tab", "", "tab title")
	in := flag.String("in", "", "input csv, whose columns are the ones synced")
	keyCol := flag.String("keycol", "", "column matching csv rows to tab rows")
	apply := flag.Bool("apply", false, "write the changes; without it the run only reports them")
	flag.Parse()
	if *sheet == "" || *tab == "" || *in == "" || *keyCol == "" {
		log.Fatal("[ERROR] -sheet, -tab, -in, and -keycol are required")
	}
	header, rows, err := readCSV(*in)
	if err != nil {
		log.Fatalf("[ERROR] read %s: %v", *in, err)
	}
	svc, err := sheets.NewService(context.Background(), option.WithScopes(sheets.SpreadsheetsScope))
	if err != nil {
		log.Fatalf("[ERROR] create sheets client: %v", err)
	}
	// Reporting first, always, so the same call that would write shows its work.
	result, err := sheetsync.Sync(svc, *sheet, *tab, header, rows, *keyCol, sheetsync.Merge, false)
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	for _, e := range result.Edits {
		log.Printf("  %s %s: %q -> %q", e.Key, e.Column, e.From, e.To)
	}
	log.Printf("%d cells differ across %d csv rows", len(result.Edits), len(rows))
	if len(result.Added) > 0 {
		log.Fatalf("[ERROR] %d csv rows match no tab row, which this tool will not add: %v",
			len(result.Added), result.Added)
	}
	if len(result.Detached) > 0 {
		log.Printf("%d tab rows are absent from the csv and are left alone", len(result.Detached))
	}
	if !*apply {
		log.Printf("reporting only, pass -apply to write")
		return
	}
	if len(result.Edits) == 0 {
		return
	}
	if _, err := sheetsync.Sync(svc, *sheet, *tab, header, rows, *keyCol, sheetsync.Merge, true); err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	log.Printf("wrote %d cells to %s", len(result.Edits), *tab)
}
