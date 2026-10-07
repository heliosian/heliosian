package main

import (
	"encoding/csv"
	"log/slog"
	"os"
	"path/filepath"

	"heliosian/internal/data"
	"heliosian/internal/logging"
	"heliosian/internal/spreadsheets"
)

const root = "local/snapshot"

func main() {
	slog.SetDefault(logging.Console())
	sheet, err := data.NewSheet(spreadsheets.IDs(spreadsheets.All))
	if err != nil {
		logging.Fatal("sheets client", "error", err)
	}
	if err := os.RemoveAll(root); err != nil {
		logging.Fatal("clear the old snapshot", "dir", root, "error", err)
	}
	for _, s := range spreadsheets.All {
		tabs, err := sheet.RawTabs(s.Source)
		if err != nil {
			logging.Fatal("read spreadsheet", "sheet", s.Source, "error", err)
		}
		dir := filepath.Join(root, s.Source)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			logging.Fatal("create snapshot directory", "dir", dir, "error", err)
		}
		total := 0
		for title, rows := range tabs {
			write(filepath.Join(dir, title+".csv"), rows)
			total += len(rows)
		}
		slog.Info("snapshot: wrote", "sheet", s.Source, "tabs", len(tabs), "rows", total)
	}
	slog.Info("snapshot: done, serve it with go run ./tools/startserver --snapshot --email <address>, delete it with rm -r "+root, "dir", root)
}

func write(path string, rows [][]string) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		logging.Fatal("create snapshot file", "path", path, "error", err)
	}
	w := csv.NewWriter(f)
	if err := w.WriteAll(rows); err != nil {
		logging.Fatal("write snapshot file", "path", path, "error", err)
	}
	if err := f.Close(); err != nil {
		logging.Fatal("close snapshot file", "path", path, "error", err)
	}
}
