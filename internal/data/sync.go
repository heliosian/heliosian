package data

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/sheets/v4"
)

type Policy int

const (
	Mirror Policy = iota
	Merge
)

type Edit struct {
	Key      string
	Column   string
	From, To string
}

type SyncResult struct {
	Edits    []Edit
	Added    []string
	Removed  []string
	Detached []string
}

func cellAt(row []interface{}, i int) string {
	if i >= len(row) {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(row[i]))
}

func (s *Sheet) Sync(app, table string, header []string, rows []map[string]string, keyCol string, policy Policy, apply bool) (*SyncResult, error) {
	id, ok := s.spreadsheets[app]
	if !ok {
		return nil, fmt.Errorf("no spreadsheet configured for app %q", app)
	}
	quoted := quoteTab(table)
	resp, err := call("get "+table, s.service.Spreadsheets.Values.Get(id, quoted).Do)
	if err != nil {
		return nil, fmt.Errorf("read tab %s: %w", table, err)
	}
	if len(resp.Values) == 0 {
		return nil, fmt.Errorf("table %s has no header row", table)
	}
	index := map[string]int{}
	for i, c := range resp.Values[0] {
		index[strings.TrimSpace(fmt.Sprint(c))] = i
	}
	for _, name := range header {
		if _, ok := index[name]; !ok {
			return nil, fmt.Errorf("table %s is missing column %q", table, name)
		}
	}
	if _, ok := index[keyCol]; !ok {
		return nil, fmt.Errorf("table %s is missing column %q", table, keyCol)
	}

	position := map[string]int{}
	for i, row := range resp.Values[1:] {
		key := cellAt(row, index[keyCol])
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
	appends := [][]interface{}{}
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
			out := make([]interface{}, len(resp.Values[0]))
			for i := range out {
				out[i] = ""
			}
			for _, name := range header {
				out[index[name]] = strings.TrimSpace(row[name])
			}
			appends = append(appends, out)
			result.Added = append(result.Added, key)
			continue
		}
		for _, name := range header {
			// Trimmed, as the tab reads back: an untrimmed value would differ forever and
			// rewrite itself on every run.
			want := strings.TrimSpace(row[name])
			if name == keyCol || want == cellAt(resp.Values[n-1], index[name]) {
				continue
			}
			updates = append(updates, &sheets.ValueRange{
				Range:  fmt.Sprintf("%s!%s%d", quoted, columnName(index[name]), n),
				Values: [][]interface{}{{want}},
			})
			result.Edits = append(result.Edits, Edit{
				Key: key, Column: name, From: cellAt(resp.Values[n-1], index[name]), To: want,
			})
		}
	}
	for key := range position {
		if seen[key] {
			continue
		}
		if policy == Mirror {
			result.Removed = append(result.Removed, key)
			continue
		}
		result.Detached = append(result.Detached, key)
	}
	sort.Strings(result.Added)
	sort.Strings(result.Removed)
	sort.Strings(result.Detached)
	if !apply {
		return result, nil
	}

	// Cells while every row is where it was read, deletions from the bottom, then new
	// rows past everything the first two touched.
	if len(updates) > 0 {
		_, err := call("sync "+table, s.service.Spreadsheets.Values.BatchUpdate(id, &sheets.BatchUpdateValuesRequest{
			ValueInputOption: "RAW", Data: updates,
		}).Do)
		if err != nil {
			return nil, fmt.Errorf("update cells of %s: %w", table, err)
		}
	}
	if len(result.Removed) > 0 {
		tab, err := s.tabID(id, table)
		if err != nil {
			return nil, err
		}
		doomed := []int{}
		for _, key := range result.Removed {
			doomed = append(doomed, position[key])
		}
		sort.Sort(sort.Reverse(sort.IntSlice(doomed)))
		requests := []*sheets.Request{}
		for _, n := range doomed {
			requests = append(requests, &sheets.Request{DeleteDimension: &sheets.DeleteDimensionRequest{
				Range: &sheets.DimensionRange{
					SheetId: tab, Dimension: "ROWS",
					StartIndex: int64(n - 1), EndIndex: int64(n),
				},
			}})
		}
		if _, err := call("delete "+table, s.service.Spreadsheets.BatchUpdate(id, &sheets.BatchUpdateSpreadsheetRequest{Requests: requests}).Do); err != nil {
			return nil, fmt.Errorf("delete rows of %s: %w", table, err)
		}
	}
	if len(appends) > 0 {
		if err := s.writeRows(id, table, len(resp.Values)-len(result.Removed), appends); err != nil {
			return nil, fmt.Errorf("add rows to %s: %w", table, err)
		}
	}
	return result, nil
}
