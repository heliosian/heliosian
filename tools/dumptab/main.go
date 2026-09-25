package main

import (
	"encoding/csv"
	"flag"
	"log"
	"os"

	"heliosian/internal/data"
)

func main() {
	sheet := flag.String("sheet", "", "spreadsheet id")
	tab := flag.String("tab", "", "tab title")
	out := flag.String("out", "", "output csv path")
	flag.Parse()
	if *sheet == "" || *tab == "" || *out == "" {
		log.Fatal("[ERROR] --sheet, --tab, and --out are required")
	}
	source, err := data.NewSheet(map[string]string{"sheet": *sheet})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	rows, err := source.Raw("sheet", *tab)
	if err != nil {
		log.Fatalf("[ERROR] read tab %s: %v", *tab, err)
	}
	f, err := os.Create(*out)
	if err != nil {
		log.Fatalf("[ERROR] create %s: %v", *out, err)
	}
	w := csv.NewWriter(f)
	if err := w.WriteAll(rows); err != nil {
		log.Fatalf("[ERROR] write csv: %v", err)
	}
	if err := f.Close(); err != nil {
		log.Fatalf("[ERROR] close %s: %v", *out, err)
	}
	log.Printf("wrote %d rows to %s", len(rows), *out)
}
