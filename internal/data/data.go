// Package data provides tabular app data sources.
package data

import (
	"encoding/csv"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// CheckColumns refuses a tab missing a column the caller reads. A column it does
// not read is somebody else's business.
func CheckColumns(table string, header, wanted []string) error {
	present := map[string]bool{}
	for _, h := range header {
		present[h] = true
	}
	for _, w := range wanted {
		if !present[w] {
			return fmt.Errorf("table %s is missing column %q", table, w)
		}
	}
	return nil
}

type Source interface {
	Table(app, name string) ([]string, []map[string]string, error)
	Header(app, name string) ([]string, error)
	// Raw returns every row, header included, exactly as the tab holds it - no
	// dedup check, no name-keyed records. Table/Header assume one row is one
	// record and reject a tab whose columns repeat; a few tabs (the Invite List
	// Builder's per-system templates) are read positionally instead and may
	// deliberately repeat a column name to match a destination system's own
	// quirky template, so they need the columns exactly as entered instead.
	Raw(app, name string) ([][]string, error)
}

type Writer interface {
	Upsert(app, table, keyColumn, keyValue string, cells map[string]string) error
	// Set is Upsert for a tab keyed by more than one column: every row matching all
	// of match takes cells, and when none does a row holding match plus cells is
	// appended.
	Set(app, table string, match, cells map[string]string) error
	// SetMany is Set for several rows of one tab at once, keyed by one column:
	// every row whose keyColumn is a key in cells takes that key's cells. One
	// read and one write, however many rows - a row at a time, the same edit
	// spends its Sheets quota many times over. A key no row carries is an error.
	SetMany(app, table, keyColumn string, cells map[string]map[string]string) error
	Append(app, table string, row []string) error
	// AppendCells adds a row placing each cell under the column of that name,
	// wherever the tab keeps it, and rejects a column the tab does not have. It
	// is Append for a tab whose column order people may have rearranged by hand.
	AppendCells(app, table string, cells map[string]string) error
	Delete(app, table string, match map[string]string) error
	// Reorder rewrites a tab's data rows into the order the keys give. The keys
	// must be exactly the tab's existing keyColumn values, each once, so rows
	// only ever move - nothing is created, dropped, or edited, and columns the
	// caller does not model travel with their row.
	Reorder(app, table, keyColumn string, keys []string) error
}

// orderRows returns rows sorted into the order keys gives, or an error when
// keys is not a permutation of the rows' keyColumn values. Shared by every
// Writer so the two backends refuse identically.
func orderRows(table, keyColumn string, keys []string, rows []map[string]string, keyOf func(map[string]string) string) ([]map[string]string, error) {
	if len(keys) != len(rows) {
		return nil, fmt.Errorf("table %s has %d rows but %d were ordered", table, len(rows), len(keys))
	}
	byKey := make(map[string]map[string]string, len(rows))
	for _, row := range rows {
		byKey[keyOf(row)] = row
	}
	ordered := make([]map[string]string, 0, len(rows))
	for _, key := range keys {
		row, ok := byKey[key]
		if !ok {
			return nil, fmt.Errorf("table %s has no row with %s %q", table, keyColumn, key)
		}
		delete(byKey, key)
		ordered = append(ordered, row)
	}
	return ordered, nil
}

type table struct {
	header []string
	rows   []map[string]string
}

// Dir is a fake spreadsheet over a directory of CSVs: tabs load on first read and
// every write lands in memory, so the files on disk stay as fixtures.
type Dir struct {
	Root   string
	mu     sync.Mutex
	tables map[string]*table
}

func (d *Dir) load(app, name string) (*table, error) {
	key := app + "/" + name
	if t, ok := d.tables[key]; ok {
		return t, nil
	}
	f, err := os.Open(filepath.Join(d.Root, app, name+".csv"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	values := make([][]interface{}, len(records))
	for i, row := range records {
		cells := make([]interface{}, len(row))
		for j, cell := range row {
			cells[j] = cell
		}
		values[i] = cells
	}
	header, rows, err := parseTable(name, values)
	if err != nil {
		return nil, err
	}
	t := &table{header: header, rows: rows}
	if d.tables == nil {
		d.tables = map[string]*table{}
	}
	d.tables[key] = t
	return t, nil
}

// Header reads the whole CSV, since a local file costs nothing to parse and the
// cached table serves every later read.
func (d *Dir) Header(app, name string) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, err := d.load(app, name)
	if err != nil {
		return nil, err
	}
	return slices.Clone(t.header), nil
}

// Raw re-reads the CSV fresh rather than going through the header-deduped cache,
// so it stays correct even for a tab whose columns repeat.
func (d *Dir) Raw(app, name string) ([][]string, error) {
	f, err := os.Open(filepath.Join(d.Root, app, name+".csv"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return csv.NewReader(f).ReadAll()
}

func (d *Dir) Table(app, name string) ([]string, []map[string]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, err := d.load(app, name)
	if err != nil {
		return nil, nil, err
	}
	rows := make([]map[string]string, len(t.rows))
	for i, row := range t.rows {
		rows[i] = maps.Clone(row)
	}
	return slices.Clone(t.header), rows, nil
}

func (d *Dir) Upsert(app, name, keyColumn, keyValue string, cells map[string]string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, err := d.load(app, name)
	if err != nil {
		return err
	}
	found := false
	for _, row := range t.rows {
		if !strings.EqualFold(row[keyColumn], keyValue) {
			continue
		}
		setCells(row, cells)
		found = true
	}
	if !found {
		row := map[string]string{keyColumn: keyValue}
		setCells(row, cells)
		t.rows = append(t.rows, row)
	}
	return nil
}

func (d *Dir) Set(app, name string, match, cells map[string]string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, err := d.load(app, name)
	if err != nil {
		return err
	}
	found := false
	for _, row := range t.rows {
		if !rowMatches(row, match) {
			continue
		}
		setCells(row, cells)
		found = true
	}
	if !found {
		row := map[string]string{}
		setCells(row, match)
		setCells(row, cells)
		t.rows = append(t.rows, row)
	}
	return nil
}

func (d *Dir) SetMany(app, name, keyColumn string, cells map[string]map[string]string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, err := d.load(app, name)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, row := range t.rows {
		if c, ok := cells[row[keyColumn]]; ok {
			setCells(row, c)
			seen[row[keyColumn]] = true
		}
	}
	for key := range cells {
		if !seen[key] {
			return fmt.Errorf("table %s has no row with %s %q", name, keyColumn, key)
		}
	}
	return nil
}

func (d *Dir) Append(app, name string, row []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, err := d.load(app, name)
	if err != nil {
		return err
	}
	record := map[string]string{}
	for i, cell := range row {
		if i >= len(t.header) || t.header[i] == "" || cell == "" {
			continue
		}
		record[t.header[i]] = cell
	}
	t.rows = append(t.rows, record)
	return nil
}

func (d *Dir) AppendCells(app, name string, cells map[string]string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, err := d.load(app, name)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, column := range t.header {
		known[column] = true
	}
	record := map[string]string{}
	for column, cell := range cells {
		if !known[column] {
			return fmt.Errorf("table %s is missing column %q", name, column)
		}
		if cell != "" {
			record[column] = cell
		}
	}
	t.rows = append(t.rows, record)
	return nil
}

func (d *Dir) Delete(app, name string, match map[string]string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, err := d.load(app, name)
	if err != nil {
		return err
	}
	kept := []map[string]string{}
	for _, row := range t.rows {
		if !rowMatches(row, match) {
			kept = append(kept, row)
		}
	}
	t.rows = kept
	return nil
}

func (d *Dir) Reorder(app, name, keyColumn string, keys []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, err := d.load(app, name)
	if err != nil {
		return err
	}
	ordered, err := orderRows(name, keyColumn, keys, t.rows, func(row map[string]string) string {
		return row[keyColumn]
	})
	if err != nil {
		return err
	}
	t.rows = ordered
	return nil
}

func rowMatches(row, match map[string]string) bool {
	for column, value := range match {
		if !strings.EqualFold(row[column], value) {
			return false
		}
	}
	return true
}

// parseTable drops blank cells, so a cleared column vanishes rather than holding "".
func setCells(row, cells map[string]string) {
	for column, value := range cells {
		if value == "" {
			delete(row, column)
			continue
		}
		row[column] = value
	}
}
