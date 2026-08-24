// Command import pulls a fresh Veracross export and applies it to the directory sheet.
package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"heliosian/internal/data"
	"heliosian/internal/directory"
	"heliosian/internal/sheetsync"
)

const (
	devtools    = "http://localhost:9222/json/version"
	studentsTab = "Veracross Student Import"
	staffTab    = "Veracross Staff Import"
	namesTab    = "Name to Email"
)

var sources = []struct {
	tab    string
	file   string
	keyCol string
	policy sheetsync.Policy
}{
	{studentsTab, "All Students Directory.csv", "entry_sort_name", sheetsync.Mirror},
	{staffTab, "All Faculty & Staff Directory.csv", "entry_sort_name", sheetsync.Mirror},
	// Merged, not mirrored: this tab also holds addresses for staff and the blank
	// rows recording who is deliberately left out, none of which the export knows.
	{namesTab, "Name to Email.csv", "Name", sheetsync.Merge},
}

func readCSV(path string) ([]string, []map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(records) < 2 {
		return nil, nil, fmt.Errorf("%s has no data rows", path)
	}
	rows := []map[string]string{}
	for _, rec := range records[1:] {
		row := map[string]string{}
		for i, name := range records[0] {
			if i < len(rec) {
				row[name] = rec[i]
			}
		}
		rows = append(rows, row)
	}
	return records[0], rows, nil
}

func browserRunning() bool {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(devtools)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

func run(dir string, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	sheet := os.Getenv("DIRECTORY_SHEET")
	preferences := os.Getenv("PREFERENCES_SHEET")
	exporter := os.Getenv("VCEXPORT")
	if exporter == "" {
		exporter = "../vcexport"
	}
	if sheet == "" || preferences == "" {
		log.Fatal("[ERROR] DIRECTORY_SHEET and PREFERENCES_SHEET are required")
	}
	out, err := os.MkdirTemp("", "vcexport")
	if err != nil {
		log.Fatalf("[ERROR] create output dir: %v", err)
	}

	if !browserRunning() {
		log.Printf("starting the capture browser")
		if err := run(exporter, "go", "run", "./tools/capturebrowser"); err != nil {
			log.Fatalf("[ERROR] start capture browser: %v", err)
		}
	}
	log.Printf("sign into Veracross in the browser window if you are not already, then press enter")
	bufio.NewReader(os.Stdin).ReadString('\n')

	log.Printf("exporting from Veracross into %s", out)
	if err := run(exporter, "go", "run", ".", "-out", out); err != nil {
		log.Fatalf("[ERROR] export: %v", err)
	}

	svc, err := sheets.NewService(context.Background(), option.WithScopes(sheets.SpreadsheetsScope))
	if err != nil {
		log.Fatalf("[ERROR] create sheets client: %v", err)
	}
	for _, s := range sources {
		header, rows, err := readCSV(filepath.Join(out, s.file))
		if err != nil {
			log.Fatalf("[ERROR] read %s: %v", s.file, err)
		}
		result, err := sheetsync.Sync(svc, sheet, s.tab, header, rows, s.keyCol, s.policy, true)
		if err != nil {
			log.Fatalf("[ERROR] sync %s: %v", s.tab, err)
		}
		log.Printf("%s: %d cells updated, %d rows added, %d removed",
			s.tab, len(result.Edits), len(result.Added), len(result.Removed))
		for _, key := range result.Added {
			log.Printf("  added %s", key)
		}
		for _, key := range result.Removed {
			log.Printf("  removed %s", key)
		}
		for _, key := range result.Detached {
			log.Printf("  kept, and not in the export: %s", key)
		}
	}

	log.Printf("photos are in %s, upload them separately", filepath.Join(out, "photos"))
	log.Printf("rebuilding the model to check the result")
	source, err := data.NewSheet(map[string]string{"directory": sheet, "preferences": preferences})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	model, err := directory.LoadModel(source, nil, staticFiles{})
	if err != nil {
		log.Fatalf("[ERROR] the sheet no longer loads: %v", err)
	}
	students, parents, staff := 0, 0, 0
	for _, p := range model.People {
		if p.IsStudent {
			students++
		}
		if p.IsParent {
			parents++
		}
		if p.IsStaff {
			staff++
		}
	}
	log.Printf("loaded %d people (students %d, parents %d, staff %d), %d families",
		len(model.People), students, parents, staff, len(model.Families))
}

type staticFiles struct{}

func (staticFiles) Has(key string) bool {
	_, err := os.Stat(filepath.Join("web/static", filepath.FromSlash(key)))
	return err == nil
}
