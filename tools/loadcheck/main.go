package main

import (
	"flag"
	"fmt"
	"log"

	"heliosian/internal/app"
	"heliosian/internal/data"
	"heliosian/internal/db"
	"heliosian/internal/hosts"
	"heliosian/internal/spreadsheets"
	"heliosian/internal/store"
)

func main() {
	dir := flag.String("dir", "", "load from a directory of dumped tabs instead of the live sheets")
	flag.Parse()
	var source data.Source
	if *dir != "" {
		source = &data.Dir{Root: *dir}
	} else {
		live, err := data.NewSheet(spreadsheets.IDs(spreadsheets.All))
		if err != nil {
			log.Fatalf("sheet source: %v", err)
		}
		source = live
	}
	dataStore, err := db.NewStore(source, nil, store.NewQueue(), db.NewSearchIndex(), hosts.Origin(app.Domain))
	if err != nil {
		log.Fatalf("load the data sheets: %v", err)
	}
	fmt.Println("data sheets:")
	for _, t := range db.Tables {
		if !t.Generated {
			fmt.Printf("  %s: %d rows\n", t.Name, dataStore.Model().Table(t.Name).Len())
		}
	}
}
