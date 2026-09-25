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

type Tab struct {
	Header []string
	Rows   []map[string]string
}

type Source interface {
	Table(app, name string) ([]string, []map[string]string, error)
	Header(app, name string) ([]string, error)
	Tabs(app string, tables, headers []string) (map[string]Tab, error)
	Raw(app, name string) ([][]string, error)
}

type Writer interface {
	Insert(app, table string, rows []map[string]string) error
	Set(app, table string, match, cells map[string]string) error
	SetMany(app, table, keyColumn string, cells map[string]map[string]string) error
	Delete(app, table string, match map[string]string) error
}

type table struct {
	header []string
	rows   []map[string]string
}

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

func (d *Dir) Header(app, name string) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, err := d.load(app, name)
	if err != nil {
		return nil, err
	}
	return slices.Clone(t.header), nil
}

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

func (d *Dir) Tabs(app string, tables, headers []string) (map[string]Tab, error) {
	out := map[string]Tab{}
	for _, name := range tables {
		header, rows, err := d.Table(app, name)
		if err != nil {
			return nil, err
		}
		out[name] = Tab{Header: header, Rows: rows}
	}
	for _, name := range headers {
		header, err := d.Header(app, name)
		if err != nil {
			return nil, err
		}
		out[name] = Tab{Header: header}
	}
	return out, nil
}

func (t *table) has(name string, columns ...map[string]string) error {
	for _, cells := range columns {
		for column := range cells {
			if !slices.Contains(t.header, column) {
				return fmt.Errorf("table %s is missing column %q", name, column)
			}
		}
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
	if err := t.has(name, match, cells); err != nil {
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
	if err := t.has(name, map[string]string{keyColumn: ""}); err != nil {
		return err
	}
	for _, c := range cells {
		if err := t.has(name, c); err != nil {
			return err
		}
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

func (d *Dir) Insert(app, name string, rows []map[string]string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, err := d.load(app, name)
	if err != nil {
		return err
	}
	for _, cells := range rows {
		if err := t.has(name, cells); err != nil {
			return err
		}
	}
	for _, cells := range rows {
		record := map[string]string{}
		setCells(record, cells)
		t.rows = append(t.rows, record)
	}
	return nil
}

func (d *Dir) Delete(app, name string, match map[string]string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	t, err := d.load(app, name)
	if err != nil {
		return err
	}
	if err := t.has(name, match); err != nil {
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
