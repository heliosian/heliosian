// Command deleterow removes rows of a sheet tab matching a column value.
package main

import (
	"flag"
	"log"

	"heliosian/internal/data"
)

func main() {
	sheet := flag.String("sheet", "", "spreadsheet id")
	tab := flag.String("tab", "", "tab title")
	col := flag.String("col", "Email", "column to match")
	value := flag.String("value", "", "value to match")
	flag.Parse()
	if *sheet == "" || *tab == "" || *value == "" {
		log.Fatal("[ERROR] -sheet, -tab, and -value are required")
	}
	source, err := data.NewSheet(map[string]string{"directory": *sheet})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	if err := source.Delete("directory", *tab, map[string]string{*col: *value}); err != nil {
		log.Fatalf("[ERROR] delete from %s where %s=%s: %v", *tab, *col, *value, err)
	}
	log.Printf("deleted rows from %s where %s = %q", *tab, *col, *value)
}
