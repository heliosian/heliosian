// Package data provides tabular app data sources.
package data

import (
	"encoding/csv"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

type Source interface {
	Table(app, name string) ([]string, []map[string]string, error)
	Header(app, name string) ([]string, error)
}

type Writer interface {
	Upsert(app, table, keyColumn, keyValue string, cells map[string]string) error
	Append(app, table string, row []string) error
	Delete(app, table string, match map[string]string) error
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
