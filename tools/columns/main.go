// Command columns prints the column names of each directory table in the configured source.
package main

import (
	"fmt"
	"log"
	"os"

	"heliosian/internal/data"
)

func main() {
	sheetID := os.Getenv("DIRECTORY_SHEET")
	var source data.Source
	if sheetID == "" {
		source = &data.Dir{Root: "sampledata"}
	} else {
		s, err := data.NewSheet(map[string]string{"directory": sheetID})
		if err != nil {
			log.Fatalf("[ERROR] load directory sheet: %v", err)
		}
		source = s
	}
	for _, name := range []string{"Veracross Import", "Name to Email", "Overrides", "Change Log"} {
		header, rows, err := source.Table("directory", name)
		if err != nil {
			log.Fatalf("[ERROR] table %s: %v", name, err)
		}
		fmt.Printf("%s (%d rows):\n", name, len(rows))
		for _, column := range header {
			fmt.Printf("  %s\n", column)
		}
	}
}
