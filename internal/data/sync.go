package data

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"google.golang.org/api/sheets/v4"
)

type Edit struct {
	Key      string
	Column   string
	From, To string
}

type SyncResult struct {
	Edits    []Edit
	Added    []string
	Detached []string
}

func cellAt(row []any, i int) string {
	if i >= len(row) {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(row[i]))
}

func (s *Sheet) Sync(app, table string, header []string, rows []map[string]string, keyCol string, apply bool) (*SyncResult, error) {
	g, err := s.read(app, table)
	if err != nil {
		return nil, err
	}
	if err := requireColumns(table, g.index, append(slices.Clone(header), keyCol)...); err != nil {
		return nil, err
	}
	quoted := quoteTab(table)
	position := map[string]int{}
	for i, row := range g.values[1:] {
		key := cellAt(row, g.index[keyCol])
		if key == "" {
			continue
		}
		if _, dup := position[key]; dup {
			return nil, fmt.Errorf("table %s has duplicate key %q", table, key)
		}
		position[key] = i + 2
	}

	result := &SyncResult{}
	updates := []*sheets.ValueRange{}
	appends := [][]any{}
	seen := map[string]bool{}
	for _, row := range rows {
		key := row[keyCol]
		if key == "" {
			return nil, fmt.Errorf("row %v has no key", row)
		}
		if seen[key] {
			return nil, fmt.Errorf("rows have duplicate key %q", key)
		}
		seen[key] = true
		n, ok := position[key]
		if !ok {
			out := g.blankRow()
			for _, name := range header {
				out[g.index[name]] = strings.TrimSpace(row[name])
			}
			appends = append(appends, out)
			result.Added = append(result.Added, key)
			continue
		}
		for _, name := range header {
			want := strings.TrimSpace(row[name])
			if name == keyCol || want == cellAt(g.values[n-1], g.index[name]) {
				continue
			}
			updates = append(updates, &sheets.ValueRange{
				Range:  fmt.Sprintf("%s!%s%d", quoted, columnName(g.index[name]), n),
				Values: [][]any{{want}},
			})
			result.Edits = append(result.Edits, Edit{
				Key: key, Column: name, From: cellAt(g.values[n-1], g.index[name]), To: want,
			})
		}
	}
	for key := range position {
		if seen[key] {
			continue
		}
		result.Detached = append(result.Detached, key)
	}
	sort.Strings(result.Added)
	sort.Strings(result.Detached)
	if !apply {
		return result, nil
	}

	if len(updates) > 0 {
		_, err := call("sync "+table, s.service.Spreadsheets.Values.BatchUpdate(g.id, &sheets.BatchUpdateValuesRequest{
			ValueInputOption: "RAW", Data: updates,
		}).Do)
		if err != nil {
			return nil, fmt.Errorf("update cells of %s: %w", table, err)
		}
	}
	if len(appends) > 0 {
		if err := s.writeRows(g.id, table, len(g.values), appends); err != nil {
			return nil, fmt.Errorf("add rows to %s: %w", table, err)
		}
	}
	return result, nil
}
