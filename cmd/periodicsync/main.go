package main

import (
	"context"
	"flag"
	"log/slog"
	"strings"

	"heliosian/internal/app"
	"heliosian/internal/calendarimport"
	"heliosian/internal/data"
	"heliosian/internal/env"
	"heliosian/internal/logging"
	"heliosian/internal/store"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

func main() {
	slog.SetDefault(logging.Cloud())
	dryRun := flag.Bool("dry-run", false, "report what the run would change, writing nothing to the sheets")
	permitted := flag.Bool("i-have-user-permission-to-spend-money", false, "every run that reaches Claude costs real money; pass this only when the person paying has said to run it")
	flag.Parse()
	if !*permitted {
		logging.Fatal("periodicsync: this run spends money on Claude; pass --i-have-user-permission-to-spend-money only when the user has said to run it")
	}
	spreadsheets := app.SheetIDs(app.SpreadsheetsOf(app.SyncSources))
	key := env.Key("ANTHROPIC_API_KEY", "local/creds/anthropic.key")
	ctx := context.Background()
	source, err := data.NewSheet(spreadsheets)
	if err != nil {
		logging.Fatal("periodicsync: sheet source", "error", err)
	}
	directory, err := who.LoadModel(source, nil, app.StaticFiles{Root: "web/who"}, []byte("periodicsync"))
	if err != nil {
		logging.Fatal("periodicsync: load directory model", "error", err)
	}
	roster := func() when.Roster { return app.CalendarRoster(directory) }
	cache, err := when.NewCache(source, source, roster, nil, func(string) bool { return false }, store.NewQueue())
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
