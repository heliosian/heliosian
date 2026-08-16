// Command sheets explores a google sheet via the service account: tabs, sizes, and header rows.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	"heliosian/internal/data"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)


func main() {
	sheet := flag.String("sheet", "", "spreadsheet id")
	tab := flag.String("tab", "", "print rows of this tab instead of the overview")
	rows := flag.Int("rows", 0, "with -tab, print only n rows")
	from := flag.Int("from", 1, "with -tab and -rows, first row to print")
	cells := flag.Bool("cells", false, "with -tab, print each non-empty cell with its column index")
	flag.Parse()
	if *sheet == "" {
		log.Fatal("[ERROR] -sheet <spreadsheet id> is required")
	}
	svc, err := sheets.NewService(context.Background(),
		option.WithCredentialsFile(data.KeyFile),
		option.WithScopes(sheets.SpreadsheetsReadonlyScope))
	if err != nil {
		log.Fatalf("[ERROR] create sheets client: %v", err)
	}
	if *tab != "" {
		rng := quoteTab(*tab)
		if *rows > 0 {
			rng = fmt.Sprintf("%s!%d:%d", quoteTab(*tab), *from, *from+*rows-1)
		}
		resp, err := svc.Spreadsheets.Values.Get(*sheet, rng).Do()
		if err != nil {
			log.Fatalf("[ERROR] read tab %s: %v", *tab, err)
		}
		for _, row := range resp.Values {
			if *cells {
				for i, cell := range row {
					if s := fmt.Sprint(cell); s != "" {
						fmt.Printf("  %d: %q\n", i, s)
					}
				}
				fmt.Println("---")
				continue
			}
			fmt.Println(row)
		}
		return
	}
	ss, err := svc.Spreadsheets.Get(*sheet).Do()
	if err != nil {
		log.Fatalf("[ERROR] get spreadsheet: %v", err)
	}
	fmt.Println("title:", ss.Properties.Title)
	for _, s := range ss.Sheets {
		p := s.Properties
		fmt.Printf("\ntab %q: %d rows x %d cols\n", p.Title, p.GridProperties.RowCount, p.GridProperties.ColumnCount)
		resp, err := svc.Spreadsheets.Values.Get(*sheet, quoteTab(p.Title)+"!1:1").Do()
		if err != nil {
			log.Printf("[ERROR] read header of %s: %v", p.Title, err)
			continue
		}
		if len(resp.Values) > 0 {
			fmt.Printf("  header: %v\n", resp.Values[0])
		}
	}
}

func quoteTab(title string) string {
	return "'" + strings.ReplaceAll(title, "'", "''") + "'"
}
