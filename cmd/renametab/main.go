package main

import (
	"flag"
	"log"

	"heliosian/internal/data"
)

func main() {
	sheet := flag.String("sheet", "", "spreadsheet id")
	from := flag.String("from", "", "current tab title")
	to := flag.String("to", "", "new tab title")
	flag.Parse()
	if *sheet == "" || *from == "" || *to == "" {
		log.Fatal("[ERROR] --sheet, --from, and --to are required")
	}
	source, err := data.NewSheet(map[string]string{"sheet": *sheet})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	if err := source.RenameTab("sheet", *from, *to); err != nil {
		log.Fatalf("[ERROR] rename %q: %v", *from, err)
	}
	log.Printf("renamed %q to %q", *from, *to)
}
