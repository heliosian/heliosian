// Command columns prints the column names of each directory table in the configured source.
package main

import (
	"fmt"
	"log"
	"os"
	"sort"

	"heliosian/internal/data"
)

func main() {
	sheetID := os.Getenv("DIRECTORY_SHEET")
	var source data.Source
	if sheetID == "" {
		source = data.Dir{Root: "sampledata"}
	} else {
		s, err := data.NewSheet(map[string]string{"directory": sheetID})
		if err != nil {
			log.Fatalf("[ERROR] load directory sheet: %v", err)
		}
		source = s
	}
	for _, name := range []string{"Basic Directory", "Staff Details", "Classrooms", "Schedules", "Grade Lookup", "Room Parents", "Departments"} {
		rows, err := source.Table("directory", name)
		if err != nil {
			log.Fatalf("[ERROR] table %s: %v", name, err)
		}
		if len(rows) == 0 {
			fmt.Printf("%s: no rows\n", name)
			continue
		}
		seen := map[string]bool{}
		columns := []string{}
		for _, row := range rows {
			for column := range row {
				if !seen[column] {
					seen[column] = true
					columns = append(columns, column)
				}
			}
		}
		sort.Strings(columns)
		fmt.Printf("%s:\n", name)
		for _, column := range columns {
			fmt.Printf("  %s\n", column)
		}
	}
}
