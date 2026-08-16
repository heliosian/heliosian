package data

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

const KeyFile = "creds/service-account.json"

type Sheet struct {
	service      *sheets.Service
	spreadsheets map[string]string
}

func NewSheet(spreadsheets map[string]string) (*Sheet, error) {
	service, err := sheets.NewService(context.Background(),
		option.WithCredentialsFile(KeyFile),
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
	resp, err := s.service.Spreadsheets.Values.Get(id, quoteTab(name)).Do()
	if err != nil {
		return nil, nil, err
	}
	return parseTable(name, resp.Values)
}

func (s *Sheet) Upsert(app, table, keyColumn, keyValue, column, value string) error {
	id, ok := s.spreadsheets[app]
	if !ok {
		return fmt.Errorf("no spreadsheet configured for app %q", app)
	}
	quoted := quoteTab(table)
	resp, err := s.service.Spreadsheets.Values.Get(id, quoted).Do()
	if err != nil {
		return err
	}
	if len(resp.Values) == 0 {
		return fmt.Errorf("table %s is empty", table)
	}
	keyIdx, colIdx := -1, -1
	for i, cell := range resp.Values[0] {
		switch strings.TrimSpace(fmt.Sprint(cell)) {
		case keyColumn:
			keyIdx = i
		case column:
			colIdx = i
		}
	}
	if keyIdx < 0 || colIdx < 0 {
		return fmt.Errorf("table %s is missing column %q or %q", table, keyColumn, column)
	}
	ranges := []*sheets.ValueRange{}
	for i, row := range resp.Values[1:] {
		if keyIdx >= len(row) || !strings.EqualFold(strings.TrimSpace(fmt.Sprint(row[keyIdx])), keyValue) {
			continue
		}
		ranges = append(ranges, &sheets.ValueRange{
			Range:  fmt.Sprintf("%s!%s%d", quoted, columnName(colIdx), i+2),
			Values: [][]interface{}{{value}},
		})
	}
	if len(ranges) == 0 {
		width := max(keyIdx, colIdx) + 1
		row := make([]interface{}, width)
		for i := range row {
			row[i] = ""
		}
		row[keyIdx] = keyValue
		row[colIdx] = value
		_, err := s.service.Spreadsheets.Values.Append(id, quoted, &sheets.ValueRange{
			Values: [][]interface{}{row},
		}).ValueInputOption("RAW").InsertDataOption("INSERT_ROWS").Do()
		return err
	}
	_, err = s.service.Spreadsheets.Values.BatchUpdate(id, &sheets.BatchUpdateValuesRequest{
		ValueInputOption: "RAW",
		Data:             ranges,
	}).Do()
	return err
}

func (s *Sheet) Append(app, table string, header, row []string) error {
	id, ok := s.spreadsheets[app]
	if !ok {
		return fmt.Errorf("no spreadsheet configured for app %q", app)
	}
	meta, err := s.service.Spreadsheets.Get(id).Fields("sheets(properties(title))").Do()
	if err != nil {
		return err
	}
	exists := false
	for _, sh := range meta.Sheets {
		if sh.Properties.Title == table {
			exists = true
			break
		}
	}
	quoted := quoteTab(table)
	if !exists {
		_, err := s.service.Spreadsheets.BatchUpdate(id, &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{{AddSheet: &sheets.AddSheetRequest{
				Properties: &sheets.SheetProperties{Title: table},
			}}},
		}).Do()
		if err != nil {
			return fmt.Errorf("create table %s: %w", table, err)
		}
		if err := s.appendRow(id, quoted, header); err != nil {
			return err
		}
	}
	return s.appendRow(id, quoted, row)
}

func (s *Sheet) appendRow(id, quotedTable string, row []string) error {
	values := make([]interface{}, len(row))
	for i, cell := range row {
		values[i] = cell
	}
	_, err := s.service.Spreadsheets.Values.Append(id, quotedTable, &sheets.ValueRange{
		Values: [][]interface{}{values},
	}).ValueInputOption("RAW").InsertDataOption("INSERT_ROWS").Do()
	return err
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

func parseTable(name string, values [][]interface{}) ([]string, []map[string]string, error) {
	if len(values) == 0 {
		return nil, nil, fmt.Errorf("table %s has no header row", name)
	}
	header := make([]string, len(values[0]))
	seen := map[string]bool{}
	for i, cell := range values[0] {
		h := strings.TrimSpace(fmt.Sprint(cell))
		if h != "" && seen[h] {
			return nil, nil, fmt.Errorf("table %s has duplicate header %q", name, h)
		}
		seen[h] = true
		header[i] = h
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
