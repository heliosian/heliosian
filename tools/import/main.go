// Command import pulls a fresh Veracross export and applies it to the directory sheet.
package main

import (
	"context"
	"crypto/sha256"
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

	"heliosian/internal/blob"
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

// uploadPhotos puts the export's portraits in the bucket before any tab names one,
// so the sheet never indexes an object that is not there yet. The filename is the
// hash of the bytes, and checking that here is what keeps the two repositories from
// drifting into a directory full of broken photos.
func uploadPhotos(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	uploader, err := blob.NewUploader()
	if err != nil {
		return err
	}
	uploaded := 0
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return err
		}
		if want := fmt.Sprintf("%x%s", sha256.Sum256(content), filepath.Ext(entry.Name())); want != entry.Name() {
			return fmt.Errorf("photo %s is not named for its content, want %s", entry.Name(), want)
		}
		written, err := uploader.Put("photos", entry.Name(), http.DetectContentType(content), content)
		if err != nil {
			return err
		}
		if written {
			uploaded++
		}
	}
	log.Printf("photos: %d uploaded, %d already in the bucket", uploaded, len(entries)-uploaded)
	return nil
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
	log.Printf("exporting from Veracross into %s", out)
	if err := run(exporter, "go", "run", ".", "-out", out); err != nil {
		log.Fatalf("[ERROR] export: %v", err)
	}

	if err := uploadPhotos(filepath.Join(out, "photos")); err != nil {
		log.Fatalf("[ERROR] upload photos: %v", err)
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
