// Package data provides tabular app data sources.
package data

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
)

type Source interface {
	Table(app, name string) ([]map[string]string, error)
}

type Dir struct {
	Root string
}

func (d Dir) Table(app, name string) ([]map[string]string, error) {
	f, err := os.Open(filepath.Join(d.Root, app, name+".csv"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("table %s/%s has no header row", app, name)
	}
	header := rows[0]
	records := []map[string]string{}
	for _, row := range rows[1:] {
		record := map[string]string{}
		for i, column := range header {
			record[column] = row[i]
		}
		records = append(records, record)
	}
	return records, nil
}
