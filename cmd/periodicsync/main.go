package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"heliosian/internal/app"
	"heliosian/internal/calendar"
	"heliosian/internal/calendarimport"
	"heliosian/internal/data"
	"heliosian/internal/logging"
	"heliosian/internal/store"
	"heliosian/internal/who"
)

type staticFiles struct {
	root string
}

func (s staticFiles) Has(key string) (bool, error) {
	_, err := os.Stat(filepath.Join(s.root, filepath.FromSlash(key)))
	return err == nil, nil
}

func (staticFiles) Prefetch(context.Context, []string) error { return nil }

func apiKey() string {
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return key
	}
	raw, err := os.ReadFile("local/creds/anthropic.key")
	if err != nil {
		logging.Fatal("periodicsync: read local/creds/anthropic.key (or set ANTHROPIC_API_KEY)", "error", err)
	}
	key := strings.TrimSpace(string(raw))
	if key == "" {
		logging.Fatal("periodicsync: local/creds/anthropic.key is empty")
	}
	return key
}

func main() {
	slog.SetDefault(logging.Cloud())
	dryRun := flag.Bool("dry-run", false, "report what the run would change, writing nothing to the sheets")
	permitted := flag.Bool("i-have-user-permission-to-spend-money", false, "every run that reaches Claude costs real money; pass this only when the person paying has said to run it")
	flag.Parse()
	if !*permitted {
		logging.Fatal("periodicsync: this run spends money on Claude; pass --i-have-user-permission-to-spend-money only when the user has said to run it")
	}
	spreadsheets := app.SheetIDs(app.SpreadsheetsOf(app.SyncSources))
	key := apiKey()
	ctx := context.Background()
	source, err := data.NewSheet(spreadsheets)
	if err != nil {
		logging.Fatal("periodicsync: sheet source", "error", err)
	}
	directory, err := who.LoadModel(source, nil, staticFiles{"web/who"}, []byte("periodicsync"))
	if err != nil {
		logging.Fatal("periodicsync: load directory model", "error", err)
	}
	roster := func() calendar.Roster { return app.CalendarRoster(directory) }
	cache, err := calendar.NewCache(source, source, roster, nil, func(string) bool { return false }, store.NewQueue())
	if err != nil {
		logging.Fatal("periodicsync: load calendar model", "error", err)
	}
	failures := []string{}
	slog.InfoContext(ctx, "periodicsync: stage", "stage", "year calendar pdf")
	opts := calendarimport.Options{Source: source, Cache: cache, Roster: roster, AnthropicKey: key, DryRun: *dryRun}
	if err := calendarimport.RunPDF(ctx, opts); err != nil {
		slog.ErrorContext(ctx, "periodicsync: stage failed", "stage", "year calendar pdf", "error", err)
		failures = append(failures, "year calendar pdf")
	}
	if len(failures) > 0 {
		logging.Fatal("periodicsync: stages failed", "failed", strings.Join(failures, "; "))
	}
	slog.InfoContext(ctx, "periodicsync: every stage completed")
}
