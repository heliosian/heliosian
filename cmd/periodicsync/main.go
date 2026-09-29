package main

import (
	"context"
	"flag"
	"log/slog"

	"heliosian/internal/calendarimport"
	"heliosian/internal/data"
	"heliosian/internal/env"
	"heliosian/internal/logging"
	"heliosian/internal/model"
	"heliosian/internal/spreadsheets"
	"heliosian/internal/static"
	"heliosian/internal/store"
	"heliosian/internal/when"
)

func main() {
	slog.SetDefault(logging.Cloud())
	dryRun := flag.Bool("dry-run", false, "report what the run would change, writing nothing to the sheets")
	permitted := flag.Bool("i-have-user-permission-to-spend-money", false, "every run that reaches Claude costs real money; pass this only when the person paying has said to run it")
	flag.Parse()
	if !*permitted {
		logging.Fatal("periodicsync: this run spends money on Claude; pass --i-have-user-permission-to-spend-money only when the user has said to run it")
	}
	key := env.Required("ANTHROPIC_API_KEY")
	ctx := context.Background()
	source, err := data.NewSheet(spreadsheets.IDs(spreadsheets.Of(spreadsheets.SyncSources)))
	if err != nil {
		logging.Fatal("periodicsync: sheet source", "error", err)
	}
	directory, err := model.LoadDirectory(source, nil, static.Files{Root: "web/who"}, []byte(env.Required("ID_KEY")))
	if err != nil {
		logging.Fatal("periodicsync: load directory model", "error", err)
	}
	roster := func() when.Roster { return when.RosterOf(directory) }
	cache, err := when.NewCache(source, source, roster, nil, func() []string { return nil }, store.NewQueue())
	if err != nil {
		logging.Fatal("periodicsync: load calendar model", "error", err)
	}
	opts := calendarimport.Options{Source: source, Cache: cache, Roster: roster, AnthropicKey: key, DryRun: *dryRun}
	if err := calendarimport.RunPDF(ctx, opts); err != nil {
		logging.Fatal("periodicsync: year calendar pdf", "error", err)
	}
	slog.InfoContext(ctx, "periodicsync: completed")
}
