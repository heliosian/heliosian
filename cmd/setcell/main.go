// Command setcell sets one cell in a sheet tab by key column, appending the row if missing.
package main

import (
	"flag"
	"log"

	"heliosian/internal/data"
)

func main() {
	sheet := flag.String("sheet", "", "spreadsheet id")
	tab := flag.String("tab", "", "tab title")
	keyCol := flag.String("keycol", "Email", "key column name")
	key := flag.String("key", "", "key value")
	col := flag.String("col", "", "column to set")
	value := flag.String("value", "", "value to write")
	flag.Parse()
	if *sheet == "" || *tab == "" || *key == "" || *col == "" {
		log.Fatal("[ERROR] -sheet, -tab, -key, and -col are required")
	}
	source, err := data.NewSheet(map[string]string{"directory": *sheet})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	if err := source.Upsert("directory", *tab, *keyCol, *key, map[string]string{*col: *value}); err != nil {
		log.Fatalf("[ERROR] set %s[%s=%s].%s: %v", *tab, *keyCol, *key, *col, err)
	}
	log.Printf("set %s[%s=%s].%s = %q", *tab, *keyCol, *key, *col, *value)
}
