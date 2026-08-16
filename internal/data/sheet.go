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
		option.WithScopes(sheets.SpreadsheetsReadonlyScope))
	if err != nil {
		return nil, err
	}
	return &Sheet{service: service, spreadsheets: spreadsheets}, nil
}

func (s *Sheet) Table(app, name string) ([]map[string]string, error) {
	id, ok := s.spreadsheets[app]
	if !ok {
		return nil, fmt.Errorf("no spreadsheet configured for app %q", app)
	}
	resp, err := s.service.Spreadsheets.Values.Get(id, "'"+strings.ReplaceAll(name, "'", "''")+"'").Do()
	if err != nil {
		return nil, err
	}
	return toRecords(resp.Values), nil
}

func toRecords(values [][]interface{}) []map[string]string {
	if len(values) == 0 {
		return nil
	}
	header := make([]string, len(values[0]))
	for i, cell := range values[0] {
		header[i] = strings.TrimSpace(fmt.Sprint(cell))
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
	return records
}
