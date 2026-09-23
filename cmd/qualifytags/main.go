package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"heliosian/internal/filter"
)

func main() {
	sheet := flag.String("sheet", "", "spreadsheet id")
	tab := flag.String("tab", "", "tab title: Rules, Audience or Invite Groups")
	apply := flag.Bool("apply", false, "write the cells; without it the run only reports them")
	flag.Parse()
	if *sheet == "" || *tab == "" {
		log.Fatal("[ERROR] --sheet and --tab are required")
	}
	svc, err := sheets.NewService(context.Background(), option.WithScopes(sheets.SpreadsheetsScope))
	if err != nil {
		log.Fatalf("[ERROR] create sheets client: %v", err)
	}
	quoted := "'" + strings.ReplaceAll(*tab, "'", "''") + "'"
	resp, err := svc.Spreadsheets.Values.Get(*sheet, quoted).Do()
	if err != nil {
		log.Fatalf("[ERROR] read tab %s: %v", *tab, err)
	}
	if len(resp.Values) == 0 {
		log.Fatalf("[ERROR] tab %q has no header row", *tab)
	}
	header := map[string]int{}
	for i, cell := range resp.Values[0] {
		header[strings.TrimSpace(fmt.Sprint(cell))] = i
	}
	tags, ok := header["Tags"]
	if !ok {
		log.Fatalf("[ERROR] tab %q has no Tags column", *tab)
	}
	owner, ok := header["Owner"]
	if !ok {
		log.Fatalf("[ERROR] tab %q has no Owner column", *tab)
	}
	cell := func(row []interface{}, i int) string {
		if i >= len(row) {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(row[i]))
	}
	data := []*sheets.ValueRange{}
	for i, row := range resp.Values[1:] {
		was := cell(row, tags)
		if was == "" {
			continue
		}
		now := []string{}
		for _, tag := range filter.SplitList(was) {
			now = append(now, qualified(tag, strings.ToLower(cell(row, owner)), i+2))
		}
		if next := filter.JoinList(now); next != was {
			log.Printf("row %d: %q -> %q", i+2, was, next)
			data = append(data, &sheets.ValueRange{Range: fmt.Sprintf("%s!%s%d", quoted, column(tags), i+2), Values: [][]interface{}{{next}}})
		}
	}
	if !*apply {
		log.Printf("dry run: pass --apply to write %d cells in %q", len(data), *tab)
		return
	}
	if len(data) == 0 {
		log.Printf("nothing to write in %q", *tab)
		return
	}
	if _, err := svc.Spreadsheets.Values.BatchUpdate(*sheet, &sheets.BatchUpdateValuesRequest{ValueInputOption: "RAW", Data: data}).Do(); err != nil {
		log.Fatalf("[ERROR] write %q: %v", *tab, err)
	}
	log.Printf("wrote %d cells in %q; drop its Owner column with cmd/dropcolumns", len(data), *tab)
}

var magic = []string{"party:", "activity:", "room:", "group:"}

func qualified(tag, owner string, row int) string {
	if rest, ok := strings.CutPrefix(tag, "shared:"); ok {
		return rest
	}
	at := strings.Index(tag, ":")
	if at > 0 && strings.Contains(tag[:at], "@") {
		return tag
	}
	for _, kind := range magic {
		if strings.HasPrefix(tag, kind) {
			return tag
		}
	}
	if owner == "" {
		log.Fatalf("[ERROR] row %d names tag %q and has no Owner to name it by", row, tag)
	}
	return filter.TagKey(owner, tag)
}

func column(i int) string {
	name := ""
	for i >= 0 {
		name = string(rune('A'+i%26)) + name
		i = i/26 - 1
	}
	return name
}
