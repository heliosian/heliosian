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

	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/store"
	"heliosian/internal/who"
)

const (
	directory   = "directory"
	studentsTab = "Veracross Student Import"
	staffTab    = "Veracross Staff Import"
	namesTab    = "Name to Email"
	websiteFile = "Staff Bios.csv"
	actor       = "import"
)

var sources = []struct {
	tab    string
	file   string
	keyCol string
	mirror bool
}{
	{studentsTab, "All Students Directory.csv", "entry_sort_name", true},
	{staffTab, "All Faculty & Staff Directory.csv", "entry_sort_name", true},
	{namesTab, "Name to Email.csv", "Name", false},
}

const lineBreak = "\x00"

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

var straighten = strings.NewReplacer(
	"’", "'", "‘", "'", "“", `"`, "”", `"`,
	"–", "-", "—", "-", "…", "...", " ", " ",
)

func comparable(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(straighten.Replace(s)), " "))
}

func caughtUp(override, published string) bool {
	a, b := comparable(override), comparable(published)
	return a == b || strings.Contains(b, a)
}

func clearCaughtUpOverrides(out string, source *data.Sheet, bios []map[string]string) ([]store.Op, error) {
	_, aliasRows, err := source.Table(directory, who.AliasesTable)
	if err != nil {
		return nil, err
	}
	aliases, err := who.ParseAliases(aliasRows)
	if err != nil {
		return nil, err
	}
	bios, _ = aliases.Rewrite(bios, who.WebsiteEmailColumn)
	_, staffRows, err := readCSV(filepath.Join(out, "All Faculty & Staff Directory.csv"))
	if err != nil {
		return nil, err
	}
	staffRows, _ = aliases.Rewrite(staffRows, "person_email")
	staffByName := who.StaffByName(staffRows)
	_, nameRows, err := source.Table(directory, namesTab)
	if err != nil {
		return nil, err
	}
	nameToEmail := map[string]string{}
	for _, row := range nameRows {
		nameToEmail[who.NormName(row["Name"])] = strings.ToLower(row["Email"])
	}
	_, overrideRows, err := source.Table(directory, "Overrides")
	if err != nil {
		return nil, err
	}
	overrides := map[string]map[string]string{}
	for _, row := range overrideRows {
		overrides[strings.ToLower(row["Email"])] = row
	}

	columns := []struct{ override, published string }{
		{"Facts", who.WebsiteBio},
		{"Job Title", who.WebsiteTitle},
	}
	ops := []store.Op{}
	kept := 0
	for _, bio := range bios {
		email, err := who.WebsiteEmail(bio, nameToEmail, staffByName)
		if err != nil {
			return nil, err
		}
		row, ok := overrides[email]
		if !ok {
			continue
		}
		cells := store.Row{}
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
			if column.override == "Facts" && row["Facts Updated"] != "" {
				cells["Facts Updated"] = ""
			}
		}
		if len(cells) == 0 {
			continue
		}
		log.Printf("  clearing %v for %s, which the staff page has caught up with", slices.Sorted(maps.Keys(cells)), email)
		ops = append(ops, store.Update("Overrides", store.Row{"Email": email}, cells))
	}
	log.Printf("Overrides: %d rows the staff page has caught up with, %d left alone", len(ops), kept)
	return ops, nil
}

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
			present, err := uploader.Has("photos/" + entry.Name())
			if err != nil {
				return err
			}
			if !present {
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

func pruneNameToEmail(out string, source *data.Sheet, exported []map[string]string) ([]store.Op, error) {
	hasEmail := map[string]bool{}
	for _, s := range []struct{ file, name, email string }{
		{"All Students Directory.csv", "student_full_name", "student_email"},
		{"All Faculty & Staff Directory.csv", "person_full_name", "person_email"},
	} {
		_, rows, err := readCSV(filepath.Join(out, s.file))
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row[s.email] != "" {
				hasEmail[who.NormName(row[s.name])] = true
			}
		}
	}
	_, rows, err := source.Table(directory, namesTab)
	if err != nil {
		return nil, err
	}
	ops := []store.Op{}
	dropped := map[string]bool{}
	for _, row := range slices.Concat(rows, exported) {
		name := strings.TrimSpace(row["Name"])
		if row["Email"] == "" || !hasEmail[who.NormName(name)] || dropped[who.NormName(name)] {
			continue
		}
		dropped[who.NormName(name)] = true
		log.Printf("  dropping %s, veracross now has an address for them", name)
		ops = append(ops, store.Delete(namesTab, store.Row{"Name": name}))
	}
	log.Printf("%s: %d entries the export has caught up with", namesTab, len(ops))
	return ops, nil
}

type tabSync struct {
	tab    string
	header []string
	rows   []map[string]string
	keyCol string
	mirror bool
}

func (s tabSync) ops(source *data.Sheet) ([]store.Op, error) {
	header, before, err := source.Table(directory, s.tab)
	if err != nil {
		return nil, err
	}
	for _, column := range append(slices.Clone(s.header), s.keyCol) {
		if !slices.Contains(header, column) {
			return nil, fmt.Errorf("table %s is missing column %q", s.tab, column)
		}
	}
	had := map[string]map[string]string{}
	for _, row := range before {
		key := strings.TrimSpace(row[s.keyCol])
		if key == "" {
			continue
		}
		if _, dup := had[key]; dup {
			return nil, fmt.Errorf("table %s has duplicate key %q", s.tab, key)
		}
		had[key] = row
	}
	ops := []store.Op{}
	seen := map[string]bool{}
	added, changed := 0, 0
	for _, row := range s.rows {
		key := strings.TrimSpace(row[s.keyCol])
		if key == "" {
			return nil, fmt.Errorf("row %v has no key", row)
		}
		if seen[key] {
			return nil, fmt.Errorf("rows have duplicate key %q", key)
		}
		seen[key] = true
		old, ok := had[key]
		cells := store.Row{}
		for _, column := range s.header {
			want := strings.TrimSpace(row[column])
			if !ok {
				if want != "" {
					cells[column] = want
				}
				continue
			}
			if column != s.keyCol && want != strings.TrimSpace(old[column]) {
				cells[column] = want
			}
		}
		if !ok {
			log.Printf("  added %s", key)
			ops = append(ops, store.Insert(s.tab, cells))
			added++
			continue
		}
		if len(cells) > 0 {
			ops = append(ops, store.Update(s.tab, store.Row{s.keyCol: key}, cells))
			changed++
		}
	}
	removed := 0
	for _, key := range slices.Sorted(maps.Keys(had)) {
		if seen[key] {
			continue
		}
		if !s.mirror {
			log.Printf("  kept, and not in the export: %s", key)
			continue
		}
		log.Printf("  removed %s", key)
		ops = append(ops, store.Delete(s.tab, store.Row{s.keyCol: key}))
		removed++
	}
	log.Printf("%s: %d rows added, %d changed, %d removed", s.tab, added, changed, removed)
	return ops, nil
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
	if err := run(exporter, "go", "run", ".", "--out", out); err != nil {
		log.Fatalf("[ERROR] export: %v", err)
	}
	log.Printf("exporting from the school website into %s", out)
	if err := run(website, "go", "run", ".", "--out", out); err != nil {
		log.Fatalf("[ERROR] website export: %v", err)
	}

	source, err := data.NewSheet(map[string]string{directory: sheet, "preferences": preferences, "config": config})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	directoryStore, err := who.Open(source, source, nil, staticFiles{}, []byte(actor), store.NewQueue())
	if err != nil {
		log.Fatalf("[ERROR] the directory does not load: %v", err)
	}
	bios, err := websiteRows(out)
	if err != nil {
		log.Fatalf("[ERROR] read %s: %v", websiteFile, err)
	}

	if err := uploadPhotos(filepath.Join(out, "photos"), !*dryRun); err != nil {
		log.Fatalf("[ERROR] upload photos: %v", err)
	}

	ops, err := tabSync{tab: who.WebsiteTable, header: who.WebsiteColumns, rows: bios, keyCol: who.WebsiteID, mirror: true}.ops(source)
	if err != nil {
		log.Fatalf("[ERROR] plan %s: %v", who.WebsiteTable, err)
	}
	exportedNames := []map[string]string{}
	for _, s := range sources {
		header, rows, err := readCSV(filepath.Join(out, s.file))
		if err != nil {
			log.Fatalf("[ERROR] read %s: %v", s.file, err)
		}
		if s.tab == namesTab {
			exportedNames = rows
		}
		tabOps, err := tabSync{tab: s.tab, header: header, rows: rows, keyCol: s.keyCol, mirror: s.mirror}.ops(source)
		if err != nil {
			log.Fatalf("[ERROR] plan %s: %v", s.tab, err)
		}
		ops = append(ops, tabOps...)
	}

	cleared, err := clearCaughtUpOverrides(out, source, bios)
	if err != nil {
		log.Fatalf("[ERROR] plan clearing the overrides the staff page has caught up with: %v", err)
	}
	pruned, err := pruneNameToEmail(out, source, exportedNames)
	if err != nil {
		log.Fatalf("[ERROR] plan pruning %s: %v", namesTab, err)
	}
	ops = slices.Concat(ops, cleared, pruned)

	if *dryRun {
		log.Printf("dry run: %d row changes not committed", len(ops))
		return
	}
	if err := directoryStore.CommitAndWait(context.Background(), actor, ops...); err != nil {
		log.Fatalf("[ERROR] commit the import: %v", err)
	}
	model := directoryStore.Model()
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

func (staticFiles) Has(key string) (bool, error) {
	_, err := os.Stat(filepath.Join("web/who", filepath.FromSlash(key)))
	return err == nil, nil
}

func (staticFiles) Prefetch([]string) error { return nil }
