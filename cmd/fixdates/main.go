// Command fixdates rewrites the Glide-style timestamps in the Events sheet into
// the forms the loader reads: Start and End on Activities become
// "2025-03-01 17:30", and Added on Activities and Volunteers becomes
// "2025-05-05". The clock is kept as written - Glide stamped local times with a
// Z - so five-thirty stays five-thirty. Without -write it only reports.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const glideForm = "2006-01-02T15:04:05.000Z"

var columns = map[string]map[string]string{
	"Activities": {"Start": "2006-01-02 15:04", "End": "2006-01-02 15:04", "Added": "2006-01-02"},
	"Volunteers": {"Added": "2006-01-02"},
}

func main() {
	write := flag.Bool("write", false, "write the converted cells; otherwise only report them")
	flag.Parse()
	id := os.Getenv("EVENTS_SHEET")
	if id == "" {
		log.Fatal("[ERROR] EVENTS_SHEET is required")
	}
	svc, err := sheets.NewService(context.Background(), option.WithScopes(sheets.SpreadsheetsScope))
	if err != nil {
		log.Fatalf("[ERROR] create sheets client: %v", err)
	}
	var updates []*sheets.ValueRange
	for tab, wanted := range columns {
		quoted := "'" + strings.ReplaceAll(tab, "'", "''") + "'"
		resp, err := svc.Spreadsheets.Values.Get(id, quoted).Do()
		if err != nil {
			log.Fatalf("[ERROR] read %s: %v", tab, err)
		}
		if len(resp.Values) == 0 {
			log.Fatalf("[ERROR] %s is empty", tab)
		}
		index := map[string]int{}
		for i, cell := range resp.Values[0] {
			index[strings.TrimSpace(fmt.Sprint(cell))] = i
		}
		converted := 0
		for column, form := range wanted {
			col, ok := index[column]
			if !ok {
				log.Fatalf("[ERROR] %s has no column %q", tab, column)
			}
			for i, row := range resp.Values[1:] {
				if col >= len(row) {
					continue
				}
				cell := strings.TrimSpace(fmt.Sprint(row[col]))
				t, err := time.Parse(glideForm, cell)
				if err != nil {
					continue
				}
				value := t.Format(form)
				fmt.Printf("%s!%s%d  %s -> %s\n", tab, columnName(col), i+2, cell, value)
				updates = append(updates, &sheets.ValueRange{
					Range:  fmt.Sprintf("%s!%s%d", quoted, columnName(col), i+2),
					Values: [][]interface{}{{value}},
				})
				converted++
			}
		}
		log.Printf("%s: %d cells to convert", tab, converted)
	}
	if !*write {
		log.Printf("dry run: %d cells would change; pass -write to change them", len(updates))
		return
	}
	if len(updates) == 0 {
		return
	}
	if _, err := svc.Spreadsheets.Values.BatchUpdate(id, &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "RAW", Data: updates,
	}).Do(); err != nil {
		log.Fatalf("[ERROR] write: %v", err)
	}
	log.Printf("wrote %d cells", len(updates))
}

func columnName(idx int) string {
	name := ""
	for idx >= 0 {
		name = string(rune('A'+idx%26)) + name
		idx = idx/26 - 1
	}
	return name
}
