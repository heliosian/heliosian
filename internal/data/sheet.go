package data

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// retryWaits paces a call the Sheets API has refused for quota: its read and
// write quotas are per minute per user, and a server starting up reads every
// tab of every sheet at once - or deploys twice in a minute - so a burst runs
// into them. Waiting out the minute and asking again is the whole fix; giving
// up would drop a write or refuse to start.
var retryWaits = []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second}

// call runs one Sheets API request - its Do method - retrying only a quota
// refusal (429).
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
	id, ok := s.spreadsheets[app]
	if !ok {
		return nil, nil, fmt.Errorf("no spreadsheet configured for app %q", app)
	}
	resp, err := call("get "+name, s.service.Spreadsheets.Values.Get(id, quoteTab(name)).Do)
	if err != nil {
		return nil, nil, err
	}
	return parseTable(name, resp.Values)
}

// Header reads only the first row. The change log grows without bound and is never
// read into the model, so validating its columns must not drag every audit row over
// the wire on each reload.
func (s *Sheet) Header(app, name string) ([]string, error) {
	id, ok := s.spreadsheets[app]
	if !ok {
		return nil, fmt.Errorf("no spreadsheet configured for app %q", app)
	}
	resp, err := call("header "+name, s.service.Spreadsheets.Values.Get(id, quoteTab(name)+"!1:1").Do)
	if err != nil {
		return nil, err
	}
	return parseHeader(name, resp.Values)
}

// Raw returns every row, header included, with no dedup check and no
// name-keyed record conversion - see Source.Raw.
func (s *Sheet) Raw(app, name string) ([][]string, error) {
	id, ok := s.spreadsheets[app]
	if !ok {
		return nil, fmt.Errorf("no spreadsheet configured for app %q", app)
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

func (s *Sheet) Upsert(app, table, keyColumn, keyValue string, cells map[string]string) error {
	id, ok := s.spreadsheets[app]
	if !ok {
		return fmt.Errorf("no spreadsheet configured for app %q", app)
	}
	quoted := quoteTab(table)
	resp, err := call("get "+table, s.service.Spreadsheets.Values.Get(id, quoted).Do)
	if err != nil {
		return err
	}
	if len(resp.Values) == 0 {
		return fmt.Errorf("table %s is empty", table)
	}
	keyIdx := -1
	colIdx := map[string]int{}
	for i, cell := range resp.Values[0] {
		name := strings.TrimSpace(fmt.Sprint(cell))
		if name == keyColumn {
			keyIdx = i
		}
		if _, ok := cells[name]; ok {
			colIdx[name] = i
		}
	}
	if keyIdx < 0 {
		return fmt.Errorf("table %s is missing column %q", table, keyColumn)
	}
	for column := range cells {
		if _, ok := colIdx[column]; !ok {
			return fmt.Errorf("table %s is missing column %q", table, column)
		}
	}
	ranges := []*sheets.ValueRange{}
	for i, row := range resp.Values[1:] {
		if keyIdx >= len(row) || !strings.EqualFold(strings.TrimSpace(fmt.Sprint(row[keyIdx])), keyValue) {
			continue
		}
		for column, value := range cells {
			ranges = append(ranges, &sheets.ValueRange{
				Range:  fmt.Sprintf("%s!%s%d", quoted, columnName(colIdx[column]), i+2),
				Values: [][]interface{}{{value}},
			})
		}
	}
	if len(ranges) == 0 {
		width := keyIdx + 1
		for _, idx := range colIdx {
			width = max(width, idx+1)
		}
		row := make([]interface{}, width)
		for i := range row {
			row[i] = ""
		}
		row[keyIdx] = keyValue
		for column, value := range cells {
			row[colIdx[column]] = value
		}
		return s.writeRows(id, table, len(resp.Values), [][]interface{}{row})
	}
	_, err = call("set "+table, s.service.Spreadsheets.Values.BatchUpdate(id, &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "RAW",
		Data:             ranges,
	}).Do)
	return err
}

func (s *Sheet) Set(app, table string, match, cells map[string]string) error {
	id, ok := s.spreadsheets[app]
	if !ok {
		return fmt.Errorf("no spreadsheet configured for app %q", app)
	}
	quoted := quoteTab(table)
	resp, err := call("get "+table, s.service.Spreadsheets.Values.Get(id, quoted).Do)
	if err != nil {
		return err
	}
	if len(resp.Values) == 0 {
		return fmt.Errorf("table %s is empty", table)
	}
	index := map[string]int{}
	for i, cell := range resp.Values[0] {
		index[strings.TrimSpace(fmt.Sprint(cell))] = i
	}
	for column := range match {
		if _, ok := index[column]; !ok {
			return fmt.Errorf("table %s is missing column %q", table, column)
		}
	}
	for column := range cells {
		if _, ok := index[column]; !ok {
			return fmt.Errorf("table %s is missing column %q", table, column)
		}
	}
	ranges := []*sheets.ValueRange{}
	for i, row := range resp.Values[1:] {
		if !valuesMatch(row, index, match) {
			continue
		}
		for column, value := range cells {
			ranges = append(ranges, &sheets.ValueRange{
				Range:  fmt.Sprintf("%s!%s%d", quoted, columnName(index[column]), i+2),
				Values: [][]interface{}{{value}},
			})
		}
	}
	if len(ranges) == 0 {
		row := make([]interface{}, len(resp.Values[0]))
		for i := range row {
			row[i] = ""
		}
		for column, value := range match {
			row[index[column]] = value
		}
		for column, value := range cells {
			row[index[column]] = value
		}
		return s.writeRows(id, table, len(resp.Values), [][]interface{}{row})
	}
	_, err = call("set "+table, s.service.Spreadsheets.Values.BatchUpdate(id, &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "RAW",
		Data:             ranges,
	}).Do)
	return err
}

func (s *Sheet) Append(app, table string, row []string) error {
	return s.AppendAll(app, table, [][]string{row})
}

// AppendAll adds many rows in one call, which is the difference between a bulk load
// finishing and it spending minutes being throttled a row at a time.
func (s *Sheet) AppendAll(app, table string, rows [][]string) error {
	id, ok := s.spreadsheets[app]
	if !ok {
		return fmt.Errorf("no spreadsheet configured for app %q", app)
	}
	if len(rows) == 0 {
		return nil
	}
	values := make([][]interface{}, len(rows))
	for i, row := range rows {
		cells := make([]interface{}, len(row))
		for j, cell := range row {
			cells[j] = cell
		}
		values[i] = cells
	}
	used, err := s.rowCount(id, table)
	if err != nil {
		return err
	}
	return s.writeRows(id, table, used, values)
}

// rowCount is how many rows of the tab hold anything, header included: the
// next row is where an appended row goes.
func (s *Sheet) rowCount(id, table string) (int, error) {
	resp, err := call("get "+table, s.service.Spreadsheets.Values.Get(id, quoteTab(table)).Do)
	if err != nil {
		return 0, err
	}
	return len(resp.Values), nil
}

// writeRows puts rows at an explicit address - the first row after the `used`
// rows the tab already has, starting in column A - growing the grid first when
// it is too short. The Sheets append call is not used: it "detects a table"
// from the range it is given, and a blank row, or a column that is empty for a
// stretch, makes it pick a different table and start the row in that table's
// first column, which lands cells under the wrong headers.
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

func (s *Sheet) AppendCells(app, table string, cells map[string]string) error {
	id, ok := s.spreadsheets[app]
	if !ok {
		return fmt.Errorf("no spreadsheet configured for app %q", app)
	}
	quoted := quoteTab(table)
	// The whole tab is read: the header to place the cells, the row count to
	// place the row.
	resp, err := call("get "+table, s.service.Spreadsheets.Values.Get(id, quoted).Do)
	if err != nil {
		return err
	}
	if len(resp.Values) == 0 {
		return fmt.Errorf("table %s is empty", table)
	}
	index := map[string]int{}
	for i, cell := range resp.Values[0] {
		index[strings.TrimSpace(fmt.Sprint(cell))] = i
	}
	row := make([]interface{}, len(resp.Values[0]))
	for i := range row {
		row[i] = ""
	}
	for column, value := range cells {
		i, ok := index[column]
		if !ok {
			return fmt.Errorf("table %s is missing column %q", table, column)
		}
		row[i] = value
	}
	return s.writeRows(id, table, len(resp.Values), [][]interface{}{row})
}

func (s *Sheet) Delete(app, table string, match map[string]string) error {
	id, ok := s.spreadsheets[app]
	if !ok {
		return fmt.Errorf("no spreadsheet configured for app %q", app)
	}
	resp, err := call("get "+table, s.service.Spreadsheets.Values.Get(id, quoteTab(table)).Do)
	if err != nil {
		return err
	}
	if len(resp.Values) == 0 {
		return fmt.Errorf("table %s is empty", table)
	}
	index := map[string]int{}
	for i, cell := range resp.Values[0] {
		index[strings.TrimSpace(fmt.Sprint(cell))] = i
	}
	for column := range match {
		if _, ok := index[column]; !ok {
			return fmt.Errorf("table %s is missing column %q", table, column)
		}
	}
	tab, err := s.tabID(id, table)
	if err != nil {
		return err
	}
	requests := []*sheets.Request{}
	// Descending, so deleting a row never shifts one still queued behind it.
	for i := len(resp.Values) - 1; i >= 1; i-- {
		if !valuesMatch(resp.Values[i], index, match) {
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
		return nil
	}
	_, err = call("delete "+table, s.service.Spreadsheets.BatchUpdate(id, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}).Do)
	return err
}

// Reorder rewrites the tab's data block in one Values.Update. It permutes the
// raw rows rather than the parsed ones, so cells in columns this app never
// models - and any trailing columns - move with their row untouched.
func (s *Sheet) Reorder(app, table, keyColumn string, keys []string) error {
	id, ok := s.spreadsheets[app]
	if !ok {
		return fmt.Errorf("no spreadsheet configured for app %q", app)
	}
	quoted := quoteTab(table)
	resp, err := call("get "+table, s.service.Spreadsheets.Values.Get(id, quoted).Do)
	if err != nil {
		return err
	}
	if len(resp.Values) == 0 {
		return fmt.Errorf("table %s is empty", table)
	}
	key := -1
	for i, cell := range resp.Values[0] {
		if strings.TrimSpace(fmt.Sprint(cell)) == keyColumn {
			key = i
			break
		}
	}
	if key < 0 {
		return fmt.Errorf("table %s is missing column %q", table, keyColumn)
	}
	width := len(resp.Values[0])
	rows := make([]map[string]string, 0, len(resp.Values)-1)
	raw := map[string][]interface{}{}
	for _, row := range resp.Values[1:] {
		name := ""
		if key < len(row) {
			name = strings.TrimSpace(fmt.Sprint(row[key]))
		}
		rows = append(rows, map[string]string{keyColumn: name})
		raw[name] = row
		if len(row) > width {
			width = len(row)
		}
	}
	ordered, err := orderRows(table, keyColumn, keys, rows, func(row map[string]string) string {
		return row[keyColumn]
	})
	if err != nil {
		return err
	}
	values := make([][]interface{}, 0, len(ordered))
	for _, row := range ordered {
		cells := raw[row[keyColumn]]
		// Pad, so a short row cannot leave the row it displaced showing through.
		padded := make([]interface{}, width)
		for i := range padded {
			if i < len(cells) {
				padded[i] = cells[i]
				continue
			}
			padded[i] = ""
		}
		values = append(values, padded)
	}
	if len(values) == 0 {
		return nil
	}
	rng := fmt.Sprintf("%s!A2:%s%d", quoted, columnName(width-1), len(values)+1)
	_, err = call("reorder "+table, s.service.Spreadsheets.Values.Update(id, rng, &sheets.ValueRange{Values: values}).
		ValueInputOption("RAW").Do)
	return err
}

func valuesMatch(row []interface{}, index map[string]int, match map[string]string) bool {
	for column, value := range match {
		i := index[column]
		if i >= len(row) || !strings.EqualFold(strings.TrimSpace(fmt.Sprint(row[i])), value) {
			return false
		}
	}
	return true
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
