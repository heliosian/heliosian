// Command import pulls a fresh Veracross export and applies it to the directory sheet.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/sheetsync"
	"heliosian/internal/who"
)

const (
	studentsTab = "Veracross Student Import"
	staffTab    = "Veracross Staff Import"
	namesTab    = "Name to Email"
	websiteFile = "Staff Bios.csv"
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

// lineBreak stands in for a break the markup asks for, so it survives the pass that
// collapses the newlines and runs of spaces the markup merely happens to contain.
const lineBreak = "\x00"

// blockTags close a run of text: the bios are paragraphs, and a bio flattened
// without them runs sentences together.
var blockTags = map[string]bool{
	"p": true, "div": true, "li": true, "blockquote": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
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

// flattenBio turns the school website's own markup into the text the About Me card
// renders: paragraphs separated by a blank line, entities decoded, everything else
// dropped. A link keeps its words and loses its address, since nothing downstream
// renders markup.
func flattenBio(fragment string) (string, error) {
	parent := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), parent)
	if err != nil {
		return "", err
	}
	text := &strings.Builder{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
		}
		if n.Type == html.ElementNode && n.Data == "br" {
			text.WriteString(lineBreak)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if n.Type == html.ElementNode && blockTags[n.Data] {
			text.WriteString(lineBreak + lineBreak)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
	lines := strings.Split(text.String(), lineBreak)
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	flat := strings.Join(lines, "\n")
	for strings.Contains(flat, "\n\n\n") {
		flat = strings.ReplaceAll(flat, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(flat), nil
}

// websiteRows reads webexport's export and flattens each bio, which is the whole of
// what this import does to it: everything else is carried through as exported.
func websiteRows(out string) ([]map[string]string, error) {
	_, rows, err := readCSV(filepath.Join(out, websiteFile))
	if err != nil {
		return nil, err
	}
	flattened := make([]map[string]string, 0, len(rows))
	bios := 0
	for _, row := range rows {
		bio, err := flattenBio(row["bio_html"])
		if err != nil {
			return nil, fmt.Errorf("flatten the bio of %s: %w", row[who.WebsiteName], err)
		}
		if bio != "" {
			bios++
		}
		next := map[string]string{who.WebsiteBio: bio}
		for _, column := range who.WebsiteColumns {
			if column != who.WebsiteBio {
				next[column] = row[column]
			}
		}
		flattened = append(flattened, next)
	}
	log.Printf("%s: %d staff, %d with a bio", who.WebsiteTable, len(flattened), bios)
	return flattened, nil
}

// straighten replaces the punctuation a web page renders with the punctuation a
// person types, so a bio and a hand-written override that say the same thing compare
// equal.
var straighten = strings.NewReplacer(
	"’", "'", "‘", "'", "“", `"`, "”", `"`,
	"–", "-", "—", "-", "…", "...", " ", " ",
)

func comparable(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(straighten.Replace(s)), " "))
}

// caughtUp reports whether the published text has overtaken the override: they say
// the same thing, or the override is an older, shorter version the bio now contains.
// Anything else is a person's own words and is left alone.
func caughtUp(override, published string) bool {
	a, b := comparable(override), comparable(published)
	return a == b || strings.Contains(b, a)
}

// clearCaughtUpOverrides deletes the Facts and Job Title overrides the staff page has
// caught up with. The directory refuses to load over an override that only restates
// what an import supplies, so the run that starts publishing bios is the run that has
// to clear them - the same bargain pruneNameToEmail strikes for addresses. An
// override that still says something of its own survives and keeps winning.
func clearCaughtUpOverrides(source *data.Sheet, bios []map[string]string, apply bool) error {
	_, nameRows, err := source.Table("directory", namesTab)
	if err != nil {
		return err
	}
	nameToEmail := map[string]string{}
	for _, row := range nameRows {
		nameToEmail[who.NormName(row["Name"])] = strings.ToLower(row["Email"])
	}
	_, overrideRows, err := source.Table("directory", "Overrides")
	if err != nil {
		return err
	}
	overrides := map[string]map[string]string{}
	for _, row := range overrideRows {
		overrides[strings.ToLower(row["Email"])] = row
	}

	columns := []struct{ override, published string }{
		{"Facts", who.WebsiteBio},
		{"Job Title", who.WebsiteTitle},
	}
	cleared, kept := 0, 0
	for _, bio := range bios {
		email := who.WebsiteEmail(bio, nameToEmail)
		row, ok := overrides[email]
		if !ok {
			continue
		}
		cells := map[string]string{}
		for _, column := range columns {
			current, published := row[column.override], bio[column.published]
			if current == "" || current == "-" || published == "" {
				continue
			}
			if !caughtUp(current, published) {
				kept++
				log.Printf("  kept %s for %s, which says something the staff page does not", column.override, email)
				continue
			}
			cells[column.override] = ""
			// The date is what the About Me card posts the facts under, so it goes with
			// the text it describes rather than outliving it above the school's copy.
			if column.override == "Facts" && row["Facts Updated"] != "" {
				cells["Facts Updated"] = ""
			}
		}
		if len(cells) == 0 {
			continue
		}
		cleared++
		if !apply {
			log.Printf("  would clear %v for %s, which the staff page has caught up with", slices.Sorted(maps.Keys(cells)), email)
			continue
		}
		if err := source.Upsert("directory", "Overrides", "Email", email, cells); err != nil {
			return err
		}
		log.Printf("  cleared %v for %s, which the staff page has caught up with", slices.Sorted(maps.Keys(cells)), email)
	}
	log.Printf("Overrides: %d rows the staff page has caught up with, %d left alone", cleared, kept)
	return nil
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
				hasEmail[who.NormName(row[s.name])] = true
			}
		}
	}
	_, rows, err := source.Table("directory", namesTab)
	if err != nil {
		return err
	}
	dropped := 0
	for _, row := range rows {
		if row["Email"] == "" || !hasEmail[who.NormName(row["Name"])] {
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

func syncTab(svc *sheets.Service, sheet, tab string, header []string, rows []map[string]string, keyCol string, policy sheetsync.Policy, apply bool) error {
	result, err := sheetsync.Sync(svc, sheet, tab, header, rows, keyCol, policy, apply)
	if err != nil {
		return err
	}
	log.Printf("%s: %d cells updated, %d rows added, %d removed",
		tab, len(result.Edits), len(result.Added), len(result.Removed))
	for _, key := range result.Added {
		log.Printf("  added %s", key)
	}
	for _, key := range result.Removed {
		log.Printf("  removed %s", key)
	}
	for _, key := range result.Detached {
		log.Printf("  kept, and not in the export: %s", key)
	}
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
	config := os.Getenv("CONFIG_SHEET")
	exporter := os.Getenv("VCEXPORT")
	if exporter == "" {
		exporter = "../vcexport"
	}
	website := os.Getenv("WEBEXPORT")
	if website == "" {
		website = "../webexport"
	}
	if sheet == "" || preferences == "" || config == "" {
		log.Fatal("[ERROR] DIRECTORY_SHEET, PREFERENCES_SHEET, and CONFIG_SHEET are required")
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
	// Both exports write their portraits into the same photos directory, so one
	// upload pass covers them: every name is the hash of its own bytes, which is
	// what makes merging them safe.
	log.Printf("exporting from the school website into %s", out)
	if err := run(website, "go", "run", ".", "--out", out); err != nil {
		log.Fatalf("[ERROR] website export: %v", err)
	}

	source, err := data.NewSheet(map[string]string{"directory": sheet, "preferences": preferences, "config": config})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	bios, err := websiteRows(out)
	if err != nil {
		log.Fatalf("[ERROR] read %s: %v", websiteFile, err)
	}

	if err := uploadPhotos(filepath.Join(out, "photos"), !*dryRun); err != nil {
		log.Fatalf("[ERROR] upload photos: %v", err)
	}

	svc, err := sheets.NewService(context.Background(), option.WithScopes(sheets.SpreadsheetsScope))
	if err != nil {
		log.Fatalf("[ERROR] create sheets client: %v", err)
	}
	if err := syncTab(svc, sheet, who.WebsiteTable, who.WebsiteColumns, bios, who.WebsiteID, sheetsync.Mirror, !*dryRun); err != nil {
		log.Fatalf("[ERROR] sync %s: %v", who.WebsiteTable, err)
	}
	for _, s := range sources {
		header, rows, err := readCSV(filepath.Join(out, s.file))
		if err != nil {
			log.Fatalf("[ERROR] read %s: %v", s.file, err)
		}
		if err := syncTab(svc, sheet, s.tab, header, rows, s.keyCol, s.policy, !*dryRun); err != nil {
			log.Fatalf("[ERROR] sync %s: %v", s.tab, err)
		}
	}

	if err := clearCaughtUpOverrides(source, bios, !*dryRun); err != nil {
		log.Fatalf("[ERROR] clear the overrides the staff page has caught up with: %v", err)
	}

	if err := pruneNameToEmail(out, source, !*dryRun); err != nil {
		log.Fatalf("[ERROR] prune %s: %v", namesTab, err)
	}

	log.Printf("rebuilding the model to check the result")
	model, err := who.LoadModel(source, nil, staticFiles{})
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
	_, err := os.Stat(filepath.Join("web/who", filepath.FromSlash(key)))
	return err == nil
}
