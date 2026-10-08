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
	"create":   {"create an empty spreadsheet in the same shared drive folder as --sheet", create},
	"tabs":     {"list the tabs with their sizes and header rows", tabs},
	"rows":     {"print the rows of one tab", rows},
	"dump":     {"copy one tab to a local csv", dump},
	"write":    {"write a local csv into a tab, header-checked", write},
	"sync":     {"sync a tab's cells to a local csv by key column", sync},
	"set":      {"set one cell by key column, appending the row if missing", set},
	"delete":   {"delete the rows matching a key column value", deleteRows},
	"rename":   {"rename a tab", rename},
	"drop":     {"delete named columns from a tab, header and cells", drop},
	"drop-tab": {"delete a tab that holds no rows beyond its header", dropTab},
}

func usage() {
	names := []string{}
	for name := range commands {
		names = append(names, name)
	}
	slices.Sort(names)
	fmt.Fprintln(os.Stderr, "usage: go run ./tools/sheets <command> --sheet <spreadsheet id> [flags]")
	for _, name := range names {
		fmt.Fprintf(os.Stderr, "  %-8s %s\n", name, commands[name].summary)
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
		log.Fatalf("%s needs %s", name, strings.Join(missing, ", "))
	}
	return *sheet
}

func parse(name string, args []string, define func(fs *flag.FlagSet), required ...string) *data.Sheet {
	source, err := data.NewSheet(map[string]string{"sheet": flags(name, args, define, required...)})
	if err != nil {
		log.Fatalf("sheet source: %v", err)
	}
	return source
}

func raw(source *data.Sheet, tab string) [][]string {
	all, err := source.Raw("sheet", tab)
	if err != nil {
		log.Fatalf("read tab %s: %v", tab, err)
	}
	return all
}

func readCSV(path string) [][]string {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("open %s: %v", path, err)
	}
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		log.Fatalf("read %s: %v", path, err)
	}
	if err := f.Close(); err != nil {
		log.Fatalf("close %s: %v", path, err)
	}
	if len(records) < 2 {
		log.Fatalf("%s has no data rows", path)
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
		log.Fatalf("create drive client: %v", err)
	}
	anchor, err := svc.Files.Get(anchorID).SupportsAllDrives(true).Fields("driveId, parents").Do()
	if err != nil {
		log.Fatalf("look up %s: %v", anchorID, err)
	}
	if anchor.DriveId == "" || len(anchor.Parents) == 0 {
		log.Fatalf("%s is not in a shared drive", anchorID)
	}
	existing, err := svc.Files.List().
		Q(fmt.Sprintf("name = '%s' and mimeType = 'application/vnd.google-apps.spreadsheet' and trashed = false", *title)).
		Corpora("drive").DriveId(anchor.DriveId).IncludeItemsFromAllDrives(true).SupportsAllDrives(true).
		Fields("files(id, name)").Do()
	if err != nil {
		log.Fatalf("list spreadsheets: %v", err)
	}
	if len(existing.Files) > 0 {
		log.Fatalf("a spreadsheet named %q already exists: %s", *title, existing.Files[0].Id)
	}
	created, err := svc.Files.Create(&drive.File{
		Name:     *title,
		MimeType: "application/vnd.google-apps.spreadsheet",
		Parents:  anchor.Parents,
	}).SupportsAllDrives(true).Fields("id").Do()
	if err != nil {
		log.Fatalf("create spreadsheet: %v", err)
	}
	log.Printf("created %q beside %s", *title, anchorID)
	fmt.Println(created.Id)
}

func tabs(args []string) {
	source := parse("tabs", args, func(fs *flag.FlagSet) {})
	title, sizes, err := source.Layout("sheet")
	if err != nil {
		log.Fatalf("get spreadsheet: %v", err)
	}
	fmt.Println("title:", title)
	names := []string{}
	for _, t := range sizes {
		names = append(names, t.Title)
	}
	headers, err := source.Tabs(context.Background(), "sheet", nil, names)
	if err != nil {
		log.Fatalf("read headers: %v", err)
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
		log.Fatalf("create %s: %v", *out, err)
	}
	w := csv.NewWriter(f)
	if err := w.WriteAll(all); err != nil {
		log.Fatalf("write csv: %v", err)
	}
	if err := f.Close(); err != nil {
		log.Fatalf("close %s: %v", *out, err)
	}
	log.Printf("wrote %d rows to %s", len(all), *out)
}

func write(args []string) {
	var tab, in *string
	var appendRows, apply *bool
	source := parse("write", args, func(fs *flag.FlagSet) {
		tab = fs.String("tab", "", "tab title")
		in = fs.String("in", "", "input csv path (first row must match the tab header)")
		appendRows = fs.Bool("append", false, "append below existing rows instead of requiring an empty tab")
		apply = fs.Bool("apply", false, "write the rows; without it the run only reports them")
	}, "tab", "in")
	recs := readCSV(*in)
	existing := raw(source, *tab)
	if len(existing) == 0 {
		log.Fatalf("tab %s has no header row", *tab)
	}
	if !*appendRows && len(existing) != 1 {
		log.Fatalf("tab %s has %d rows; want exactly the header row", *tab, len(existing))
	}
	if !slices.Equal(existing[0], recs[0]) {
		log.Fatalf("header mismatch:\n tab: %q\n csv: %q", existing[0], recs[0])
	}
	added := records(recs[0], recs[1:])
	log.Printf("%d csv rows go below the %d rows in %s", len(added), len(existing), *tab)
	if !*apply {
		log.Printf("reporting only, pass --apply to write")
		return
	}
	if err := source.Insert("sheet", *tab, added); err != nil {
		log.Fatalf("write rows: %v", err)
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
	result, err := source.Sync("sheet", *tab, header, wanted, *keyCol, false)
	if err != nil {
		log.Fatalf("%v", err)
	}
	for _, e := range result.Edits {
		log.Printf("  %s %s: %q -> %q", e.Key, e.Column, e.From, e.To)
	}
	log.Printf("%d cells differ across %d csv rows", len(result.Edits), len(wanted))
	if len(result.Added) > 0 {
		log.Fatalf("%d csv rows match no tab row, which this tool will not add: %v",
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
	if _, err := source.Sync("sheet", *tab, header, wanted, *keyCol, true); err != nil {
		log.Fatalf("%v", err)
	}
	log.Printf("wrote %d cells to %s", len(result.Edits), *tab)
}

func column(all [][]string, tab, name string) int {
	if len(all) == 0 {
		log.Fatalf("tab %q has no header row", tab)
	}
	i := slices.Index(all[0], name)
	if i < 0 {
		log.Fatalf("tab %q has no column %q", tab, name)
	}
	return i
}

func matching(all [][]string, keyIndex int, keyCol, key string) []int {
	out := []int{}
	for i, row := range all[1:] {
		cell := ""
		if keyIndex < len(row) {
			cell = row[keyIndex]
		}
		if data.Matches(map[string]string{keyCol: cell}, map[string]string{keyCol: key}) {
			out = append(out, i+1)
		}
	}
	return out
}

func set(args []string) {
	var tab, keyCol, col, value *string
	var apply *bool
	keys := keyList{}
	source := parse("set", args, func(fs *flag.FlagSet) {
		tab = fs.String("tab", "", "tab title")
		keyCol = fs.String("keycol", "", "column whose value picks the row")
		fs.Var(&keys, "key", "value of --keycol in the row to set; repeat for several")
		col = fs.String("col", "", "column to write")
		value = fs.String("value", "", "value to write")
		apply = fs.Bool("apply", false, "write the cell; without it the run only reports it")
	}, "tab", "keycol", "key", "col")
	all := raw(source, *tab)
	keyIndex, colIndex := column(all, *tab, *keyCol), column(all, *tab, *col)
	present := map[string]map[string]string{}
	absent := []string{}
	for _, key := range keys {
		found := matching(all, keyIndex, *keyCol, key)
		for _, i := range found {
			from := ""
			if colIndex < len(all[i]) {
				from = all[i][colIndex]
			}
			log.Printf("  row %d %s: %q -> %q", i+1, *col, from, *value)
		}
		if len(found) == 0 {
			log.Printf("no row has %s = %q, so a row is appended", *keyCol, key)
			absent = append(absent, key)
			continue
		}
		present[key] = map[string]string{*col: *value}
	}
	if !*apply {
		log.Printf("reporting only, pass --apply to write")
		return
	}
	if len(present) > 0 {
		if err := source.SetMany("sheet", *tab, *keyCol, present); err != nil {
			log.Fatalf("set %s.%s where %s in %q: %v", *tab, *col, *keyCol, keys, err)
		}
	}
	for _, key := range absent {
		if err := source.Upsert("sheet", *tab, map[string]string{*keyCol: key}, map[string]string{*col: *value}); err != nil {
			log.Fatalf("set %s[%s=%s].%s: %v", *tab, *keyCol, key, *col, err)
		}
	}
	log.Printf("set %s.%s = %q where %s in %q", *tab, *col, *value, *keyCol, keys)
}

type keyList []string

func (k *keyList) String() string {
	return strings.Join(*k, ",")
}

func (k *keyList) Set(v string) error {
	*k = append(*k, v)
	return nil
}

func deleteRows(args []string) {
	var tab, keyCol *string
	var apply *bool
	keys := keyList{}
	source := parse("delete", args, func(fs *flag.FlagSet) {
		tab = fs.String("tab", "", "tab title")
		keyCol = fs.String("keycol", "", "column whose value picks the rows")
		fs.Var(&keys, "key", "value of --keycol in the rows to delete; repeat for several")
		apply = fs.Bool("apply", false, "delete the rows; without it the run only reports them")
	}, "tab", "keycol", "key")
	all := raw(source, *tab)
	index := column(all, *tab, *keyCol)
	found := 0
	for _, key := range keys {
		rows := matching(all, index, *keyCol, key)
		if len(rows) == 0 {
			log.Fatalf("tab %q has no row with %s = %q", *tab, *keyCol, key)
		}
		for _, i := range rows {
			log.Printf("  row %d: %q", i+1, all[i])
		}
		found += len(rows)
	}
	if !*apply {
		log.Printf("reporting only, pass --apply to delete %d rows", found)
		return
	}
	if err := source.DeleteMany("sheet", *tab, *keyCol, keys); err != nil {
		log.Fatalf("delete from %s where %s in %q: %v", *tab, *keyCol, keys, err)
	}
	log.Printf("deleted %d rows from %s where %s in %q", found, *tab, *keyCol, keys)
}

func rename(args []string) {
	var from, to *string
	var apply *bool
	source := parse("rename", args, func(fs *flag.FlagSet) {
		from = fs.String("from", "", "current tab title")
		to = fs.String("to", "", "new tab title")
		apply = fs.Bool("apply", false, "rename the tab; without it the run only reports it")
	}, "from", "to")
	_, sizes, err := source.Layout("sheet")
	if err != nil {
		log.Fatalf("get spreadsheet: %v", err)
	}
	titles := []string{}
	for _, t := range sizes {
		titles = append(titles, t.Title)
	}
	if !slices.Contains(titles, *from) {
		log.Fatalf("spreadsheet has no tab %q", *from)
	}
	if slices.Contains(titles, *to) {
		log.Fatalf("spreadsheet already has a tab %q", *to)
	}
	if !*apply {
		log.Printf("reporting only, pass --apply to rename %q to %q", *from, *to)
		return
	}
	if err := source.RenameTab("sheet", *from, *to); err != nil {
		log.Fatalf("rename %q: %v", *from, err)
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
		log.Fatalf("tab %q has no header row", *tab)
	}
	names := []string{}
	for _, name := range strings.Split(*columns, ",") {
		name = strings.TrimSpace(name)
		i := slices.Index(all[0], name)
		if i < 0 {
			log.Fatalf("tab %q has no column %q", *tab, name)
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
		log.Fatalf("delete columns from %q: %v", *tab, err)
	}
	log.Printf("deleted %d columns from %q: %s", len(names), *tab, *columns)
}

func dropTab(args []string) {
	var tab *string
	var apply *bool
	source := parse("drop-tab", args, func(fs *flag.FlagSet) {
		tab = fs.String("tab", "", "tab title")
		apply = fs.Bool("apply", false, "delete the tab; without it the run only reports it")
	}, "tab")
	all := raw(source, *tab)
	if len(all) > 1 {
		log.Fatalf("tab %q holds %d rows beyond its header", *tab, len(all)-1)
	}
	if !*apply {
		log.Printf("reporting only, pass --apply to delete tab %q", *tab)
		return
	}
	if err := source.DeleteTab("sheet", *tab); err != nil {
		log.Fatalf("delete tab %q: %v", *tab, err)
	}
	log.Printf("deleted tab %q", *tab)
}
