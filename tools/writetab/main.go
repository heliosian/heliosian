package main

import (
	"encoding/csv"
	"flag"
	"log"
	"os"
	"slices"

	"heliosian/internal/data"
)

func main() {
	sheet := flag.String("sheet", "", "spreadsheet id")
	tab := flag.String("tab", "", "tab title")
	in := flag.String("in", "", "input csv path (first row must match the tab header)")
	appendRows := flag.Bool("append", false, "append below existing rows instead of requiring an empty tab")
	flag.Parse()
	if *sheet == "" || *tab == "" || *in == "" {
		log.Fatal("[ERROR] --sheet, --tab, and --in are required")
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
	source, err := data.NewSheet(map[string]string{"sheet": *sheet})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	existing, err := source.Raw("sheet", *tab)
	if err != nil {
		log.Fatalf("[ERROR] read tab %s: %v", *tab, err)
	}
	if len(existing) == 0 {
		log.Fatalf("[ERROR] tab %s has no header row", *tab)
	}
	if !*appendRows && len(existing) != 1 {
		log.Fatalf("[ERROR] tab %s has %d rows; want exactly the header row", *tab, len(existing))
	}
	if !slices.Equal(existing[0], records[0]) {
		log.Fatalf("[ERROR] header mismatch:\n tab: %q\n csv: %q", existing[0], records[0])
	}
	rows := make([]map[string]string, len(records)-1)
	for i, rec := range records[1:] {
		row := map[string]string{}
		for j, cell := range rec {
			if j < len(records[0]) {
				row[records[0][j]] = cell
			}
		}
		rows[i] = row
	}
	if err := source.Insert("sheet", *tab, rows); err != nil {
		log.Fatalf("[ERROR] write rows: %v", err)
	}
	log.Printf("wrote %d rows to %s", len(rows), *tab)
}
