package data

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

var retryWaits = []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second}

func call[T any](what string, do func(...googleapi.CallOption) (T, error)) (T, error) {
	var out T
	var err error
	for attempt := 0; ; attempt++ {
		out, err = do()
		var apiErr *googleapi.Error
		if err == nil || !errors.As(err, &apiErr) || apiErr.Code != 429 || attempt >= len(retryWaits) {
			return out, err
		}
		slog.Warn("sheets quota refused a call; waiting to retry", "call", what, "wait", retryWaits[attempt])
		time.Sleep(retryWaits[attempt])
	}
}

type Sheet struct {
	service      *sheets.Service
	spreadsheets map[string]string
	mu           sync.Mutex
	ids          map[string]int64
}

func NewSheet(spreadsheets map[string]string) (*Sheet, error) {
	service, err := sheets.NewService(context.Background(),
		option.WithScopes(sheets.SpreadsheetsScope))
	if err != nil {
		return nil, err
	}
	return &Sheet{service: service, spreadsheets: spreadsheets}, nil
}

func (s *Sheet) Table(app, name string) ([]string, []map[string]string, error) {
	id, err := s.spreadsheet(app)
	if err != nil {
		return nil, nil, err
	}
	resp, err := call("get "+name, s.service.Spreadsheets.Values.Get(id, quoteTab(name)).Do)
	if err != nil {
		return nil, nil, err
	}
	return parseTable(name, resp.Values)
}

func (s *Sheet) Header(app, name string) ([]string, error) {
	id, err := s.spreadsheet(app)
	if err != nil {
		return nil, err
	}
	resp, err := call("header "+name, s.service.Spreadsheets.Values.Get(id, quoteTab(name)+"!1:1").Do)
	if err != nil {
		return nil, err
	}
	return parseHeader(name, resp.Values)
}

func (s *Sheet) Tabs(ctx context.Context, app string, tables, headers []string) (map[string]Tab, error) {
	id, err := s.spreadsheet(app)
	if err != nil {
		return nil, err
	}
	ranges := []string{}
	for _, name := range tables {
		ranges = append(ranges, quoteTab(name))
	}
	for _, name := range headers {
		ranges = append(ranges, quoteTab(name)+"!1:1")
	}
	resp, err := call("batch get "+app, s.service.Spreadsheets.Values.BatchGet(id).Ranges(ranges...).Context(ctx).Do)
	if err != nil {
		return nil, err
	}
	if len(resp.ValueRanges) != len(ranges) {
		return nil, fmt.Errorf("spreadsheet %s answered %d ranges for %d asked", app, len(resp.ValueRanges), len(ranges))
	}
	out := map[string]Tab{}
	for i, name := range tables {
		header, rows, err := parseTable(name, resp.ValueRanges[i].Values)
		if err != nil {
			return nil, err
		}
		out[name] = Tab{Header: header, Rows: rows}
	}
	for i, name := range headers {
		header, err := parseHeader(name, resp.ValueRanges[len(tables)+i].Values)
		if err != nil {
			return nil, err
		}
		out[name] = Tab{Header: header}
	}
	return out, nil
}

func (s *Sheet) Raw(app, name string) ([][]string, error) {
	id, err := s.spreadsheet(app)
	if err != nil {
		return nil, err
	}
	resp, err := call("get "+name, s.service.Spreadsheets.Values.Get(id, quoteTab(name)).Do)
	if err != nil {
		return nil, err
	}
	rows := make([][]string, len(resp.Values))
	for i, row := range resp.Values {
		cells := make([]string, len(row))
		for j, cell := range row {
			cells[j] = strings.TrimSpace(fmt.Sprint(cell))
		}
		rows[i] = cells
	}
	return rows, nil
}

func (s *Sheet) Upsert(app, table string, match, cells map[string]string) error {
	return s.set(app, table, match, cells, true)
}

func (s *Sheet) Update(app, table string, match, cells map[string]string) error {
	return s.set(app, table, match, cells, false)
}

func (s *Sheet) set(app, table string, match, cells map[string]string, upsert bool) error {
	g, err := s.read(app, table)
	if err != nil {
		return err
	}
	if err := requireColumns(table, g.index, slices.Collect(maps.Keys(match))...); err != nil {
		return err
	}
	if err := requireColumns(table, g.index, slices.Collect(maps.Keys(cells))...); err != nil {
		return err
	}
	quoted := quoteTab(table)
	ranges := []*sheets.ValueRange{}
	matched := false
	for i, row := range g.values[1:] {
		if !valuesMatch(row, g.index, match) {
			continue
		}
		matched = true
		for column, value := range cells {
			ranges = append(ranges, &sheets.ValueRange{
				Range:  fmt.Sprintf("%s!%s%d", quoted, columnName(g.index[column]), i+2),
				Values: [][]interface{}{{value}},
			})
		}
	}
	if !matched && !upsert {
		return fmt.Errorf("table %s has no row matching %v", table, match)
	}
	if !matched {
		row := g.blankRow()
		for column, value := range match {
			row[g.index[column]] = value
		}
		for column, value := range cells {
			row[g.index[column]] = value
		}
		return s.writeRows(g.id, table, len(g.values), [][]interface{}{row})
	}
	if len(ranges) == 0 {
		return nil
	}
	_, err = call("set "+table, s.service.Spreadsheets.Values.BatchUpdate(g.id, &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "RAW",
		Data:             ranges,
	}).Do)
	return err
}

func (s *Sheet) SetMany(app, table, keyColumn string, cells map[string]map[string]string) error {
	g, err := s.read(app, table)
	if err != nil {
		return err
	}
	if err := requireColumns(table, g.index, keyColumn); err != nil {
		return err
	}
	for _, c := range cells {
		if err := requireColumns(table, g.index, slices.Collect(maps.Keys(c))...); err != nil {
			return err
		}
	}
	quoted := quoteTab(table)
	keys := map[string]string{}
	for key := range cells {
		keys[Key(key)] = key
	}
	ranges := []*sheets.ValueRange{}
	seen := map[string]bool{}
	for i, row := range g.values[1:] {
		k := g.index[keyColumn]
		if k >= len(row) {
			continue
		}
		key, ok := keys[Key(fmt.Sprint(row[k]))]
		if !ok {
			continue
		}
		seen[key] = true
		for column, value := range cells[key] {
			ranges = append(ranges, &sheets.ValueRange{
				Range:  fmt.Sprintf("%s!%s%d", quoted, columnName(g.index[column]), i+2),
				Values: [][]interface{}{{value}},
			})
		}
	}
	for key := range cells {
		if !seen[key] {
			return fmt.Errorf("table %s has no row with %s %q", table, keyColumn, key)
		}
	}
	if len(ranges) == 0 {
		return nil
	}
	_, err = call("set "+table, s.service.Spreadsheets.Values.BatchUpdate(g.id, &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "RAW",
		Data:             ranges,
	}).Do)
	return err
}

func (s *Sheet) Insert(app, table string, rows []map[string]string) error {
	if len(rows) == 0 {
		return nil
	}
	g, err := s.read(app, table)
	if err != nil {
		return err
	}
	values := [][]interface{}{}
	for _, cells := range rows {
		if err := requireColumns(table, g.index, slices.Collect(maps.Keys(cells))...); err != nil {
			return err
		}
		row := g.blankRow()
		for column, value := range cells {
			row[g.index[column]] = value
		}
		values = append(values, row)
	}
	return s.writeRows(g.id, table, len(g.values), values)
}

// An explicit address, not the append call: append's table detection starts a
// row in the wrong column past a blank row.
func (s *Sheet) writeRows(id, table string, used int, rows [][]interface{}) error {
	quoted := quoteTab(table)
	tab, err := s.tabID(id, table)
	if err != nil {
		return err
	}
	meta, err := call("grid "+table, s.service.Spreadsheets.Get(id).Fields("sheets(properties(sheetId,gridProperties(rowCount)))").Do)
	if err != nil {
		return err
	}
	for _, sh := range meta.Sheets {
		if sh.Properties.SheetId != tab {
			continue
		}
		if short := int64(used+len(rows)) - sh.Properties.GridProperties.RowCount; short > 0 {
			_, err := call("grow "+table, s.service.Spreadsheets.BatchUpdate(id, &sheets.BatchUpdateSpreadsheetRequest{
				Requests: []*sheets.Request{{AppendDimension: &sheets.AppendDimensionRequest{
					SheetId: tab, Dimension: "ROWS", Length: short + 100,
				}}},
			}).Do)
			if err != nil {
				return err
			}
		}
	}
	_, err = call("append "+table, s.service.Spreadsheets.Values.Update(id, fmt.Sprintf("%s!A%d", quoted, used+1), &sheets.ValueRange{
		Values: rows,
	}).ValueInputOption("RAW").Do)
	return err
}

func (s *Sheet) Delete(app, table string, match map[string]string) error {
	g, err := s.read(app, table)
	if err != nil {
		return err
	}
	if err := requireColumns(table, g.index, slices.Collect(maps.Keys(match))...); err != nil {
		return err
	}
	tab, err := s.tabID(g.id, table)
	if err != nil {
		return err
	}
	requests := []*sheets.Request{}
	// Descending, so deleting a row never shifts one still queued behind it.
	for i := len(g.values) - 1; i >= 1; i-- {
		if !valuesMatch(g.values[i], g.index, match) {
			continue
		}
		requests = append(requests, &sheets.Request{DeleteDimension: &sheets.DeleteDimensionRequest{
			Range: &sheets.DimensionRange{
				SheetId:    tab,
				Dimension:  "ROWS",
				StartIndex: int64(i),
				EndIndex:   int64(i + 1),
			},
		}})
	}
	if len(requests) == 0 {
		return fmt.Errorf("table %s has no row matching %v", table, match)
	}
	_, err = call("delete "+table, s.service.Spreadsheets.BatchUpdate(g.id, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}).Do)
	return err
}

func valuesMatch(row []interface{}, index map[string]int, match map[string]string) bool {
	cells := map[string]string{}
	for column := range match {
		if i := index[column]; i < len(row) {
			cells[column] = fmt.Sprint(row[i])
		}
	}
	return Matches(cells, match)
}

func (s *Sheet) tabID(id, title string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := id + "!" + title
	if tab, ok := s.ids[key]; ok {
		return tab, nil
	}
	meta, err := call("tabs", s.service.Spreadsheets.Get(id).Fields("sheets(properties(sheetId,title))").Do)
	if err != nil {
		return 0, err
	}
	if s.ids == nil {
		s.ids = map[string]int64{}
	}
	for _, sh := range meta.Sheets {
		s.ids[id+"!"+sh.Properties.Title] = sh.Properties.SheetId
	}
	tab, ok := s.ids[key]
	if !ok {
		return 0, fmt.Errorf("spreadsheet has no tab %q", title)
	}
	return tab, nil
}

func quoteTab(title string) string {
	return "'" + strings.ReplaceAll(title, "'", "''") + "'"
}

func columnName(idx int) string {
	name := ""
	for idx >= 0 {
		name = string(rune('A'+idx%26)) + name
		idx = idx/26 - 1
	}
	return name
}

func parseHeader(name string, values [][]interface{}) ([]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("table %s has no header row", name)
	}
	header := make([]string, len(values[0]))
	seen := map[string]bool{}
	for i, cell := range values[0] {
		h := strings.TrimSpace(fmt.Sprint(cell))
		if h != "" && seen[h] {
			return nil, fmt.Errorf("table %s has duplicate header %q", name, h)
		}
		seen[h] = true
		header[i] = h
	}
	return header, nil
}

func indexOf(header []string) map[string]int {
	index := map[string]int{}
	for i, name := range header {
		if name != "" {
			index[name] = i
		}
	}
	return index
}

type grid struct {
	id     string
	values [][]interface{}
	index  map[string]int
}

func (s *Sheet) read(app, table string) (*grid, error) {
	id, err := s.spreadsheet(app)
	if err != nil {
		return nil, err
	}
	resp, err := call("get "+table, s.service.Spreadsheets.Values.Get(id, quoteTab(table)).Do)
	if err != nil {
		return nil, fmt.Errorf("read tab %s: %w", table, err)
	}
	header, err := parseHeader(table, resp.Values)
	if err != nil {
		return nil, err
	}
	return &grid{id: id, values: resp.Values, index: indexOf(header)}, nil
}

func requireColumns(table string, index map[string]int, columns ...string) error {
	for _, column := range columns {
		if _, ok := index[column]; !ok {
			return fmt.Errorf("table %s is missing column %q", table, column)
		}
	}
	return nil
}

func (g *grid) blankRow() []interface{} {
	row := []interface{}{}
	for range g.values[0] {
		row = append(row, "")
	}
	return row
}

func parseTable(name string, values [][]interface{}) ([]string, []map[string]string, error) {
	header, err := parseHeader(name, values)
	if err != nil {
		return nil, nil, err
	}
	records := []map[string]string{}
	for _, row := range values[1:] {
		record := map[string]string{}
		for i, cell := range row {
			if i >= len(header) || header[i] == "" {
				continue
			}
			value := strings.TrimSpace(fmt.Sprint(cell))
			if value == "" {
				continue
			}
			record[header[i]] = value
		}
		if len(record) > 0 {
			records = append(records, record)
		}
	}
	return header, records, nil
}
