package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"heliosian/internal/data"
)

type command struct {
	summary string
	run     func(args []string)
}

var commands = map[string]command{
	"create": {"create an empty spreadsheet in the same shared drive folder as --sheet", create},
	"tabs":   {"list the tabs with their sizes and header rows", tabs},
	"rows":   {"print the rows of one tab", rows},
	"dump":   {"copy one tab to a local csv", dump},
	"write":  {"write a local csv into a tab, header-checked", write},
	"sync":   {"sync a tab's cells to a local csv by key column", sync},
	"set":    {"set one cell by key column, appending the row if missing", set},
	"delete": {"delete the rows matching a column value", deleteRows},
	"rename": {"rename a tab", rename},
	"drop":   {"delete named columns from a tab, header and cells", drop},
}

func usage() {
	names := []string{}
	for name := range commands {
		names = append(names, name)
	}
	slices.Sort(names)
	fmt.Fprintln(os.Stderr, "usage: go run ./tools/sheets <command> --sheet <spreadsheet id> [flags]")
	for _, name := range names {
		fmt.Fprintf(os.Stderr, "  %-7s %s\n", name, commands[name].summary)
	}
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	c, ok := commands[os.Args[1]]
	if !ok {
		usage()
	}
	c.run(os.Args[2:])
}

func flags(name string, args []string, define func(fs *flag.FlagSet), required ...string) string {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	sheet := fs.String("sheet", "", "spreadsheet id")
	define(fs)
	fs.Parse(args)
	missing := []string{}
	for _, flagName := range append([]string{"sheet"}, required...) {
		if fs.Lookup(flagName).Value.String() == "" {
			missing = append(missing, "--"+flagName)
		}
	}
	if len(missing) > 0 {
		log.Fatalf("[ERROR] %s needs %s", name, strings.Join(missing, ", "))
	}
	return *sheet
}

func parse(name string, args []string, define func(fs *flag.FlagSet), required ...string) *data.Sheet {
	source, err := data.NewSheet(map[string]string{"sheet": flags(name, args, define, required...)})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	return source
}

func raw(source *data.Sheet, tab string) [][]string {
	all, err := source.Raw("sheet", tab)
	if err != nil {
		log.Fatalf("[ERROR] read tab %s: %v", tab, err)
	}
	return all
}

func readCSV(path string) [][]string {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("[ERROR] open %s: %v", path, err)
	}
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		log.Fatalf("[ERROR] read %s: %v", path, err)
	}
	if err := f.Close(); err != nil {
		log.Fatalf("[ERROR] close %s: %v", path, err)
	}
	if len(records) < 2 {
		log.Fatalf("[ERROR] %s has no data rows", path)
	}
	return records
}

func records(header []string, recs [][]string) []map[string]string {
	out := []map[string]string{}
	for _, rec := range recs {
		row := map[string]string{}
		for i, name := range header {
			if i < len(rec) {
				row[name] = rec[i]
			}
		}
		out = append(out, row)
	}
	return out
}

func create(args []string) {
	var title *string
	anchorID := flags("create", args, func(fs *flag.FlagSet) {
		title = fs.String("title", "", "spreadsheet title")
	}, "title")
	svc, err := drive.NewService(context.Background(), option.WithScopes(drive.DriveScope))
	if err != nil {
		log.Fatalf("[ERROR] create drive client: %v", err)
	}
	anchor, err := svc.Files.Get(anchorID).SupportsAllDrives(true).Fields("driveId, parents").Do()
	if err != nil {
		log.Fatalf("[ERROR] look up %s: %v", anchorID, err)
	}
	if anchor.DriveId == "" || len(anchor.Parents) == 0 {
		log.Fatalf("[ERROR] %s is not in a shared drive", anchorID)
	}
	existing, err := svc.Files.List().
		Q(fmt.Sprintf("name = '%s' and mimeType = 'application/vnd.google-apps.spreadsheet' and trashed = false", *title)).
		Corpora("drive").DriveId(anchor.DriveId).IncludeItemsFromAllDrives(true).SupportsAllDrives(true).
		Fields("files(id, name)").Do()
	if err != nil {
		log.Fatalf("[ERROR] list spreadsheets: %v", err)
	}
	if len(existing.Files) > 0 {
		log.Fatalf("[ERROR] a spreadsheet named %q already exists: %s", *title, existing.Files[0].Id)
	}
	created, err := svc.Files.Create(&drive.File{
		Name:     *title,
		MimeType: "application/vnd.google-apps.spreadsheet",
		Parents:  anchor.Parents,
	}).SupportsAllDrives(true).Fields("id").Do()
	if err != nil {
		log.Fatalf("[ERROR] create spreadsheet: %v", err)
	}
	log.Printf("created %q beside %s", *title, anchorID)
	fmt.Println(created.Id)
}

func tabs(args []string) {
	source := parse("tabs", args, func(fs *flag.FlagSet) {})
	title, sizes, err := source.Layout("sheet")
	if err != nil {
		log.Fatalf("[ERROR] get spreadsheet: %v", err)
	}
	fmt.Println("title:", title)
	names := []string{}
	for _, t := range sizes {
		names = append(names, t.Title)
	}
	headers, err := source.Tabs(context.Background(), "sheet", nil, names)
	if err != nil {
		log.Fatalf("[ERROR] read headers: %v", err)
	}
	for _, t := range sizes {
		fmt.Printf("\ntab %q: %d rows x %d cols\n", t.Title, t.Rows, t.Columns)
		fmt.Printf("  header: %v\n", headers[t.Title].Header)
	}
}

func rows(args []string) {
	var tab *string
	var count, from *int
	var cells *bool
	source := parse("rows", args, func(fs *flag.FlagSet) {
		tab = fs.String("tab", "", "tab title")
		count = fs.Int("rows", 0, "print only n rows")
		from = fs.Int("from", 1, "with --rows, first row to print")
		cells = fs.Bool("cells", false, "print each non-empty cell with its column index")
	}, "tab")
	all := raw(source, *tab)
	if *count > 0 {
		start := min(max(*from-1, 0), len(all))
		all = all[start:min(start+*count, len(all))]
	}
	for _, row := range all {
		if !*cells {
			fmt.Println(row)
			continue
		}
		for i, cell := range row {
			if cell != "" {
				fmt.Printf("  %d: %q\n", i, cell)
			}
		}
		fmt.Println("---")
	}
}

func dump(args []string) {
	var tab, out *string
	source := parse("dump", args, func(fs *flag.FlagSet) {
		tab = fs.String("tab", "", "tab title")
		out = fs.String("out", "", "output csv path")
	}, "tab", "out")
	all := raw(source, *tab)
	f, err := os.Create(*out)
	if err != nil {
		log.Fatalf("[ERROR] create %s: %v", *out, err)
	}
	w := csv.NewWriter(f)
	if err := w.WriteAll(all); err != nil {
		log.Fatalf("[ERROR] write csv: %v", err)
	}
	if err := f.Close(); err != nil {
		log.Fatalf("[ERROR] close %s: %v", *out, err)
	}
	log.Printf("wrote %d rows to %s", len(all), *out)
}

func write(args []string) {
	var tab, in *string
	var appendRows *bool
	source := parse("write", args, func(fs *flag.FlagSet) {
		tab = fs.String("tab", "", "tab title")
		in = fs.String("in", "", "input csv path (first row must match the tab header)")
		appendRows = fs.Bool("append", false, "append below existing rows instead of requiring an empty tab")
	}, "tab", "in")
	recs := readCSV(*in)
	existing := raw(source, *tab)
	if len(existing) == 0 {
		log.Fatalf("[ERROR] tab %s has no header row", *tab)
	}
	if !*appendRows && len(existing) != 1 {
		log.Fatalf("[ERROR] tab %s has %d rows; want exactly the header row", *tab, len(existing))
	}
	if !slices.Equal(existing[0], recs[0]) {
		log.Fatalf("[ERROR] header mismatch:\n tab: %q\n csv: %q", existing[0], recs[0])
	}
	added := records(recs[0], recs[1:])
	if err := source.Insert("sheet", *tab, added); err != nil {
		log.Fatalf("[ERROR] write rows: %v", err)
	}
	log.Printf("wrote %d rows to %s", len(added), *tab)
}

func sync(args []string) {
	var tab, in, keyCol *string
	var apply *bool
	source := parse("sync", args, func(fs *flag.FlagSet) {
		tab = fs.String("tab", "", "tab title")
		in = fs.String("in", "", "input csv, whose columns are the ones synced")
		keyCol = fs.String("keycol", "", "column matching csv rows to tab rows")
		apply = fs.Bool("apply", false, "write the changes; without it the run only reports them")
	}, "tab", "in", "keycol")
	recs := readCSV(*in)
	header, wanted := recs[0], records(recs[0], recs[1:])
	result, err := source.Sync("sheet", *tab, header, wanted, *keyCol, data.Merge, false)
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	for _, e := range result.Edits {
		log.Printf("  %s %s: %q -> %q", e.Key, e.Column, e.From, e.To)
	}
	log.Printf("%d cells differ across %d csv rows", len(result.Edits), len(wanted))
	if len(result.Added) > 0 {
		log.Fatalf("[ERROR] %d csv rows match no tab row, which this tool will not add: %v",
			len(result.Added), result.Added)
	}
	if len(result.Detached) > 0 {
		log.Printf("%d tab rows are absent from the csv and are left alone", len(result.Detached))
	}
	if !*apply {
		log.Printf("reporting only, pass --apply to write")
		return
	}
	if len(result.Edits) == 0 {
		return
	}
	if _, err := source.Sync("sheet", *tab, header, wanted, *keyCol, data.Merge, true); err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	log.Printf("wrote %d cells to %s", len(result.Edits), *tab)
}

func set(args []string) {
	var tab, keyCol, key, col, value *string
	source := parse("set", args, func(fs *flag.FlagSet) {
		tab = fs.String("tab", "", "tab title")
		keyCol = fs.String("keycol", "Email", "key column name")
		key = fs.String("key", "", "key value")
		col = fs.String("col", "", "column to set")
		value = fs.String("value", "", "value to write")
	}, "tab", "key", "col")
	if err := source.Set("sheet", *tab, map[string]string{*keyCol: *key}, map[string]string{*col: *value}); err != nil {
		log.Fatalf("[ERROR] set %s[%s=%s].%s: %v", *tab, *keyCol, *key, *col, err)
	}
	log.Printf("set %s[%s=%s].%s = %q", *tab, *keyCol, *key, *col, *value)
}

func deleteRows(args []string) {
	var tab, col, value *string
	source := parse("delete", args, func(fs *flag.FlagSet) {
		tab = fs.String("tab", "", "tab title")
		col = fs.String("col", "Email", "column to match")
		value = fs.String("value", "", "value to match")
	}, "tab", "value")
	if err := source.Delete("sheet", *tab, map[string]string{*col: *value}); err != nil {
		log.Fatalf("[ERROR] delete from %s where %s=%s: %v", *tab, *col, *value, err)
	}
	log.Printf("deleted rows from %s where %s = %q", *tab, *col, *value)
}

func rename(args []string) {
	var from, to *string
	source := parse("rename", args, func(fs *flag.FlagSet) {
		from = fs.String("from", "", "current tab title")
		to = fs.String("to", "", "new tab title")
	}, "from", "to")
	if err := source.RenameTab("sheet", *from, *to); err != nil {
		log.Fatalf("[ERROR] rename %q: %v", *from, err)
	}
	log.Printf("renamed %q to %q", *from, *to)
}

func drop(args []string) {
	var tab, columns *string
	var apply *bool
	source := parse("drop", args, func(fs *flag.FlagSet) {
		tab = fs.String("tab", "", "tab title")
		columns = fs.String("columns", "", "column headers to delete, comma-separated")
		apply = fs.Bool("apply", false, "delete the columns; without it the run only reports them")
	}, "tab", "columns")
	all := raw(source, *tab)
	if len(all) == 0 {
		log.Fatalf("[ERROR] tab %q has no header row", *tab)
	}
	names := []string{}
	for _, name := range strings.Split(*columns, ",") {
		name = strings.TrimSpace(name)
		i := slices.Index(all[0], name)
		if i < 0 {
			log.Fatalf("[ERROR] tab %q has no column %q", *tab, name)
		}
		filled := 0
		for _, row := range all[1:] {
			if i < len(row) && row[i] != "" {
				filled++
			}
		}
		log.Printf("column %q (%d) holds %d filled cells in %d rows", name, i, filled, len(all)-1)
		names = append(names, name)
	}
	if !*apply {
		log.Printf("dry run: pass --apply to delete %d columns from %q", len(names), *tab)
		return
	}
	if err := source.DropColumns("sheet", *tab, names); err != nil {
		log.Fatalf("[ERROR] delete columns from %q: %v", *tab, err)
	}
	log.Printf("deleted %d columns from %q: %s", len(names), *tab, *columns)
}
