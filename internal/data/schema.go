package data

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/sheets/v4"
)

type TabSize struct {
	Title   string
	Rows    int64
	Columns int64
}

func (s *Sheet) spreadsheet(app string) (string, error) {
	id, ok := s.spreadsheets[app]
	if !ok {
		return "", fmt.Errorf("no spreadsheet configured for app %q", app)
	}
	return id, nil
}

func (s *Sheet) Layout(app string) (string, []TabSize, error) {
	id, err := s.spreadsheet(app)
	if err != nil {
		return "", nil, err
	}
	meta, err := call("layout "+app, s.service.Spreadsheets.Get(id).Fields("properties(title),sheets(properties(sheetId,title,gridProperties(rowCount,columnCount)))").Do)
	if err != nil {
		return "", nil, err
	}
	tabs := []TabSize{}
	for _, sh := range meta.Sheets {
		size := TabSize{Title: sh.Properties.Title}
		if sh.Properties.GridProperties != nil {
			size.Rows = sh.Properties.GridProperties.RowCount
			size.Columns = sh.Properties.GridProperties.ColumnCount
		}
		tabs = append(tabs, size)
	}
	return meta.Properties.Title, tabs, nil
}

func (s *Sheet) AddTab(app, title string, header []string) error {
	id, err := s.spreadsheet(app)
	if err != nil {
		return err
	}
	_, err = call("add tab "+title, s.service.Spreadsheets.BatchUpdate(id, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{{AddSheet: &sheets.AddSheetRequest{
			Properties: &sheets.SheetProperties{Title: title},
		}}},
	}).Do)
	if err != nil {
		return fmt.Errorf("create tab %q: %w", title, err)
	}
	s.mu.Lock()
	s.ids = nil
	s.mu.Unlock()
	cells := make([]interface{}, len(header))
	for i, name := range header {
		cells[i] = name
	}
	_, err = call("header "+title, s.service.Spreadsheets.Values.Update(id, quoteTab(title)+"!1:1", &sheets.ValueRange{
		Values: [][]interface{}{cells},
	}).ValueInputOption("RAW").Do)
	if err != nil {
		return fmt.Errorf("write header of %q: %w", title, err)
	}
	return nil
}

func (s *Sheet) AddColumns(app, table string, names []string) error {
	id, err := s.spreadsheet(app)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return nil
	}
	resp, err := call("header "+table, s.service.Spreadsheets.Values.Get(id, quoteTab(table)+"!1:1").Do)
	if err != nil {
		return err
	}
	width := 0
	if len(resp.Values) > 0 {
		width = len(resp.Values[0])
	}
	tab, err := s.tabID(id, table)
	if err != nil {
		return err
	}
	meta, err := call("grid "+table, s.service.Spreadsheets.Get(id).Fields("sheets(properties(sheetId,gridProperties(columnCount)))").Do)
	if err != nil {
		return err
	}
	for _, sh := range meta.Sheets {
		if sh.Properties.SheetId != tab {
			continue
		}
		if short := int64(width+len(names)) - sh.Properties.GridProperties.ColumnCount; short > 0 {
			_, err := call("widen "+table, s.service.Spreadsheets.BatchUpdate(id, &sheets.BatchUpdateSpreadsheetRequest{
				Requests: []*sheets.Request{{AppendDimension: &sheets.AppendDimensionRequest{
					SheetId: tab, Dimension: "COLUMNS", Length: short,
				}}},
			}).Do)
			if err != nil {
				return fmt.Errorf("widen %q: %w", table, err)
			}
		}
	}
	cells := make([]interface{}, len(names))
	for i, name := range names {
		cells[i] = name
	}
	_, err = call("add columns "+table, s.service.Spreadsheets.Values.Update(id, fmt.Sprintf("%s!%s1", quoteTab(table), columnName(width)),
		&sheets.ValueRange{Values: [][]interface{}{cells}}).ValueInputOption("RAW").Do)
	if err != nil {
		return fmt.Errorf("add columns to %q: %w", table, err)
	}
	return nil
}

func (s *Sheet) DropColumns(app, table string, names []string) error {
	id, err := s.spreadsheet(app)
	if err != nil {
		return err
	}
	resp, err := call("header "+table, s.service.Spreadsheets.Values.Get(id, quoteTab(table)+"!1:1").Do)
	if err != nil {
		return err
	}
	if len(resp.Values) == 0 {
		return fmt.Errorf("table %s has no header row", table)
	}
	index := map[string]int{}
	for i, cell := range resp.Values[0] {
		index[strings.TrimSpace(fmt.Sprint(cell))] = i
	}
	indexes := []int{}
	for _, name := range names {
		i, ok := index[name]
		if !ok {
			return fmt.Errorf("table %s is missing column %q", table, name)
		}
		indexes = append(indexes, i)
	}
	tab, err := s.tabID(id, table)
	if err != nil {
		return err
	}
	sort.Sort(sort.Reverse(sort.IntSlice(indexes)))
	requests := []*sheets.Request{}
	for _, i := range indexes {
		requests = append(requests, &sheets.Request{DeleteDimension: &sheets.DeleteDimensionRequest{
			Range: &sheets.DimensionRange{SheetId: tab, Dimension: "COLUMNS", StartIndex: int64(i), EndIndex: int64(i + 1)},
		}})
	}
	_, err = call("drop columns "+table, s.service.Spreadsheets.BatchUpdate(id, &sheets.BatchUpdateSpreadsheetRequest{Requests: requests}).Do)
	return err
}

func (s *Sheet) RenameTab(app, from, to string) error {
	id, err := s.spreadsheet(app)
	if err != nil {
		return err
	}
	_, tabs, err := s.Layout(app)
	if err != nil {
		return err
	}
	for _, t := range tabs {
		if t.Title == to {
			return fmt.Errorf("tab %q already exists", to)
		}
	}
	tab, err := s.tabID(id, from)
	if err != nil {
		return err
	}
	_, err = call("rename "+from, s.service.Spreadsheets.BatchUpdate(id, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{{UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
			Properties: &sheets.SheetProperties{SheetId: tab, Title: to},
			Fields:     "title",
		}}},
	}).Do)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.ids = nil
	s.mu.Unlock()
	return nil
}
