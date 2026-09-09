// Command import pulls a fresh Veracross export and applies it to the directory sheet.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/directory"
	"heliosian/internal/sheetsync"
)

const (
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
func uploadPhotos(dir string, apply bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	uploader, err := blob.NewUploader()
	if err != nil {
		return err
	}
	uploaded, pending := 0, 0
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return err
		}
		if want := fmt.Sprintf("%x%s", sha256.Sum256(content), filepath.Ext(entry.Name())); want != entry.Name() {
			return fmt.Errorf("photo %s is not named for its content, want %s", entry.Name(), want)
		}
		if !apply {
			if !uploader.Has("photos/" + entry.Name()) {
				pending++
			}
			continue
		}
		written, err := uploader.Put("photos", entry.Name(), http.DetectContentType(content), content)
		if err != nil {
			return err
		}
		if written {
			uploaded++
		}
	}
	if !apply {
		log.Printf("photos: %d in the export, %d not in the bucket yet", len(entries), pending)
		return nil
	}
	log.Printf("photos: %d uploaded, %d already in the bucket", uploaded, len(entries)-uploaded)
	return nil
}

// pruneNameToEmail drops the entries the export has caught up with. The tab supplies
// what Veracross omits and never overrides it, so an entry naming somebody the export
// now carries an address for is one the model refuses to load: the import that learns
// the new address is the run that has to remove the row.
func pruneNameToEmail(out string, source *data.Sheet, apply bool) error {
	hasEmail := map[string]bool{}
	for _, s := range []struct{ file, name, email string }{
		{"All Students Directory.csv", "student_full_name", "student_email"},
		{"All Faculty & Staff Directory.csv", "person_full_name", "person_email"},
	} {
		_, rows, err := readCSV(filepath.Join(out, s.file))
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row[s.email] != "" {
				hasEmail[directory.NormName(row[s.name])] = true
			}
		}
	}
	_, rows, err := source.Table("directory", namesTab)
	if err != nil {
		return err
	}
	dropped := 0
	for _, row := range rows {
		if row["Email"] == "" || !hasEmail[directory.NormName(row["Name"])] {
			continue
		}
		dropped++
		if !apply {
			log.Printf("  would drop %s, veracross now has an address for them", row["Name"])
			continue
		}
		if err := source.Delete("directory", namesTab, map[string]string{"Name": row["Name"]}); err != nil {
			return err
		}
		log.Printf("  dropped %s, veracross now has an address for them", row["Name"])
	}
	log.Printf("%s: %d entries the export has caught up with", namesTab, dropped)
	return nil
}

func run(dir string, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	dryRun := flag.Bool("dry-run", false, "report what the import would change, writing nothing to the sheet or the bucket")
	flag.Parse()

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

	if *dryRun {
		log.Printf("dry run: nothing will be written to the sheet or the bucket")
	}
	log.Printf("exporting from Veracross into %s", out)
	if err := run(exporter, "go", "run", ".", "-out", out); err != nil {
		log.Fatalf("[ERROR] export: %v", err)
	}

	if err := uploadPhotos(filepath.Join(out, "photos"), !*dryRun); err != nil {
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
		result, err := sheetsync.Sync(svc, sheet, s.tab, header, rows, s.keyCol, s.policy, !*dryRun)
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

	source, err := data.NewSheet(map[string]string{"directory": sheet, "preferences": preferences})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	if err := pruneNameToEmail(out, source, !*dryRun); err != nil {
		log.Fatalf("[ERROR] prune %s: %v", namesTab, err)
	}

	log.Printf("rebuilding the model to check the result")
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
