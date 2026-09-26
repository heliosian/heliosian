package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"heliosian/internal/data"
)

func main() {
	sheet := flag.String("sheet", "", "spreadsheet id")
	tab := flag.String("tab", "", "print rows of this tab instead of the overview")
	rows := flag.Int("rows", 0, "with --tab, print only n rows")
	from := flag.Int("from", 1, "with --tab and --rows, first row to print")
	cells := flag.Bool("cells", false, "with --tab, print each non-empty cell with its column index")
	flag.Parse()
	if *sheet == "" {
		log.Fatal("[ERROR] --sheet <spreadsheet id> is required")
	}
	source, err := data.NewSheet(map[string]string{"sheet": *sheet})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	if *tab != "" {
		all, err := source.Raw("sheet", *tab)
		if err != nil {
			log.Fatalf("[ERROR] read tab %s: %v", *tab, err)
		}
		if *rows > 0 {
			start := min(max(*from-1, 0), len(all))
			all = all[start:min(start+*rows, len(all))]
		}
		for _, row := range all {
			if *cells {
				for i, cell := range row {
					if cell != "" {
						fmt.Printf("  %d: %q\n", i, cell)
					}
				}
				fmt.Println("---")
				continue
			}
			fmt.Println(row)
		}
		return
	}
	title, tabs, err := source.Layout("sheet")
	if err != nil {
		log.Fatalf("[ERROR] get spreadsheet: %v", err)
	}
	fmt.Println("title:", title)
	names := []string{}
	for _, t := range tabs {
		names = append(names, t.Title)
	}
	headers, err := source.Tabs(context.Background(), "sheet", nil, names)
	if err != nil {
		log.Fatalf("[ERROR] read headers: %v", err)
	}
	for _, t := range tabs {
		fmt.Printf("\ntab %q: %d rows x %d cols\n", t.Title, t.Rows, t.Columns)
		fmt.Printf("  header: %v\n", headers[t.Title].Header)
	}
}
