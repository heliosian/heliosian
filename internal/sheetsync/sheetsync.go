// Package sheetsync updates a sheet tab from parsed CSV rows, cell by cell.
package sheetsync

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/sheets/v4"
)

// Policy decides what happens to tab rows the CSV does not mention. Mirror tabs are
// wholesale copies of a Veracross export, so a row that stops appearing means the
// person left. Merge tabs carry hand-authored rows alongside generated ones, where
// the same absence means only that the generator has nothing to say.
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

type Result struct {
	Edits    []Edit
	Added    []string
	Removed  []string
	Detached []string
}

func quoteTab(tab string) string {
	return "'" + strings.ReplaceAll(tab, "'", "''") + "'"
}

func column(i int) string {
	name := ""
	for i >= 0 {
		name = string(rune('A'+i%26)) + name
		i = i/26 - 1
	}
	return name
}

func cell(row []interface{}, i int) string {
	if i >= len(row) {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(row[i]))
}

// Sync writes the difference between rows and the tab. Cells are updated in place,
// new rows appended, and departed rows deleted only under Mirror. Nothing is cleared
// wholesale, so a failure part way leaves the tab readable rather than empty.
func Sync(svc *sheets.Service, id, tab string, header []string, rows []map[string]string, keyCol string, policy Policy, apply bool) (*Result, error) {
	resp, err := svc.Spreadsheets.Values.Get(id, quoteTab(tab)).Do()
	if err != nil {
		return nil, fmt.Errorf("read tab %s: %w", tab, err)
	}
	if len(resp.Values) == 0 {
		return nil, fmt.Errorf("tab %s has no header row", tab)
	}
	index := map[string]int{}
	for i, c := range resp.Values[0] {
		index[strings.TrimSpace(fmt.Sprint(c))] = i
	}
	for _, name := range header {
		if _, ok := index[name]; !ok {
			return nil, fmt.Errorf("tab %s has no column %q", tab, name)
		}
	}
	if _, ok := index[keyCol]; !ok {
		return nil, fmt.Errorf("csv has no column %q", keyCol)
	}

	position := map[string]int{}
	for i, row := range resp.Values[1:] {
		key := cell(row, index[keyCol])
		if key == "" {
			continue
		}
		if _, dup := position[key]; dup {
			return nil, fmt.Errorf("tab %s has duplicate key %q", tab, key)
		}
		position[key] = i + 2
	}

	result := &Result{}
	updates := []*sheets.ValueRange{}
	appends := [][]interface{}{}
	seen := map[string]bool{}
	for _, row := range rows {
		key := row[keyCol]
		if key == "" {
			return nil, fmt.Errorf("csv row %v has no key", row)
		}
		if seen[key] {
			return nil, fmt.Errorf("csv has duplicate key %q", key)
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
			// Trimmed on the way in, because that is how the tab reads back. Comparing
			// a raw export value against a trimmed one makes any cell with stray
			// whitespace differ forever, and rewrite itself on every single import.
			want := strings.TrimSpace(row[name])
			if name == keyCol || want == cell(resp.Values[n-1], index[name]) {
				continue
			}
			updates = append(updates, &sheets.ValueRange{
				Range:  fmt.Sprintf("%s!%s%d", quoteTab(tab), column(index[name]), n),
				Values: [][]interface{}{{want}},
			})
			result.Edits = append(result.Edits, Edit{
				Key: key, Column: name, From: cell(resp.Values[n-1], index[name]), To: want,
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

	// Cells first, while every row is still where it was read. Deletions next, from
	// the bottom so earlier positions stay valid. Appends last, since they land past
	// everything either step touched.
	if len(updates) > 0 {
		_, err := svc.Spreadsheets.Values.BatchUpdate(id, &sheets.BatchUpdateValuesRequest{
			ValueInputOption: "RAW", Data: updates,
		}).Do()
		if err != nil {
			return nil, fmt.Errorf("update cells of %s: %w", tab, err)
		}
	}
	if len(result.Removed) > 0 {
		tabID, err := sheetID(svc, id, tab)
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
					SheetId: tabID, Dimension: "ROWS",
					StartIndex: int64(n - 1), EndIndex: int64(n),
				},
			}})
		}
		if _, err := svc.Spreadsheets.BatchUpdate(id, &sheets.BatchUpdateSpreadsheetRequest{Requests: requests}).Do(); err != nil {
			return nil, fmt.Errorf("delete rows of %s: %w", tab, err)
		}
	}
	if len(appends) > 0 {
		_, err := svc.Spreadsheets.Values.Append(id, quoteTab(tab), &sheets.ValueRange{Values: appends}).
			ValueInputOption("RAW").InsertDataOption("INSERT_ROWS").Do()
		if err != nil {
			return nil, fmt.Errorf("append rows to %s: %w", tab, err)
		}
	}
	return result, nil
}

func sheetID(svc *sheets.Service, id, tab string) (int64, error) {
	meta, err := svc.Spreadsheets.Get(id).Fields("sheets(properties(sheetId,title))").Do()
	if err != nil {
		return 0, fmt.Errorf("get spreadsheet: %w", err)
	}
	for _, s := range meta.Sheets {
		if s.Properties.Title == tab {
			return s.Properties.SheetId, nil
		}
	}
	return 0, fmt.Errorf("no tab titled %q", tab)
}
