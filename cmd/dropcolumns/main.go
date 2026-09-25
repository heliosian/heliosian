package main

import (
	"flag"
	"log"
	"slices"
	"strings"

	"heliosian/internal/data"
)

func main() {
	sheet := flag.String("sheet", "", "spreadsheet id")
	tab := flag.String("tab", "", "tab title")
	columns := flag.String("columns", "", "column headers to delete, comma-separated")
	apply := flag.Bool("apply", false, "delete the columns; without it the run only reports them")
	flag.Parse()
	if *sheet == "" || *tab == "" || *columns == "" {
		log.Fatal("[ERROR] --sheet, --tab, and --columns are required")
	}
	source, err := data.NewSheet(map[string]string{"sheet": *sheet})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	rows, err := source.Raw("sheet", *tab)
	if err != nil {
		log.Fatalf("[ERROR] read tab %s: %v", *tab, err)
	}
	if len(rows) == 0 {
		log.Fatalf("[ERROR] tab %q has no header row", *tab)
	}
	names := []string{}
	for _, name := range strings.Split(*columns, ",") {
		name = strings.TrimSpace(name)
		i := slices.Index(rows[0], name)
		if i < 0 {
			log.Fatalf("[ERROR] tab %q has no column %q", *tab, name)
		}
		filled := 0
		for _, row := range rows[1:] {
			if i < len(row) && row[i] != "" {
				filled++
			}
		}
		log.Printf("column %q (%d) holds %d filled cells in %d rows", name, i, filled, len(rows)-1)
		names = append(names, name)
	}
	if !*apply {
		log.Printf("dry run: pass --apply to delete %d columns from %q", len(names), *tab)
		return
	}
	if err := source.DropColumns("sheet", *tab, names); err != nil {
		log.Fatalf("[ERROR] delete columns from %q: %v", *tab, err)
	}
	log.Printf("deleted %d columns from %q: %s", len(names), *tab, *columns)
}
