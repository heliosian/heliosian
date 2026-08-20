// Command dumptab writes one sheet tab to a local CSV file.
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
	out := flag.String("out", "", "output csv path")
	flag.Parse()
	if *sheet == "" || *tab == "" || *out == "" {
		log.Fatal("[ERROR] -sheet, -tab, and -out are required")
	}
	svc, err := sheets.NewService(context.Background(),
		option.WithScopes(sheets.SpreadsheetsReadonlyScope))
	if err != nil {
		log.Fatalf("[ERROR] create sheets client: %v", err)
	}
	resp, err := svc.Spreadsheets.Values.Get(*sheet, "'"+strings.ReplaceAll(*tab, "'", "''")+"'").Do()
	if err != nil {
		log.Fatalf("[ERROR] read tab %s: %v", *tab, err)
	}
	f, err := os.Create(*out)
	if err != nil {
		log.Fatalf("[ERROR] create %s: %v", *out, err)
	}
	w := csv.NewWriter(f)
	for _, row := range resp.Values {
		record := make([]string, len(row))
		for i, cell := range row {
			record[i] = fmt.Sprint(cell)
		}
		if err := w.Write(record); err != nil {
			log.Fatalf("[ERROR] write row: %v", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		log.Fatalf("[ERROR] flush csv: %v", err)
	}
	if err := f.Close(); err != nil {
		log.Fatalf("[ERROR] close %s: %v", *out, err)
	}
	log.Printf("wrote %d rows to %s", len(resp.Values), *out)
}
