// Package data provides tabular app data sources.
package data

import (
	"encoding/csv"
	"os"
	"path/filepath"
)

type Source interface {
	Table(app, name string) ([]string, []map[string]string, error)
}

type Dir struct {
	Root string
}

func (d Dir) Table(app, name string) ([]string, []map[string]string, error) {
	f, err := os.Open(filepath.Join(d.Root, app, name+".csv"))
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, nil, err
	}
	values := make([][]interface{}, len(rows))
	for i, row := range rows {
		cells := make([]interface{}, len(row))
		for j, cell := range row {
			cells[j] = cell
		}
		values[i] = cells
	}
	return parseTable(name, values)
}
