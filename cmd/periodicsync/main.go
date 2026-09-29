package main

import (
	"context"
	"flag"
	"log/slog"

	"heliosian/internal/artifacts"
	"heliosian/internal/blob"
	"heliosian/internal/calendarimport"
	"heliosian/internal/data"
	"heliosian/internal/env"
	"heliosian/internal/logging"
	"heliosian/internal/model"
	"heliosian/internal/spreadsheets"
	"heliosian/internal/static"
	"heliosian/internal/store"
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
	source, err := data.NewSheet(spreadsheets.IDs(spreadsheets.All))
	if err != nil {
		logging.Fatal("periodicsync: sheet source", "error", err)
	}
	bucket, err := blob.Open(blob.MediaBucket)
	if err != nil {
		logging.Fatal("periodicsync: media bucket", "error", err)
	}
	embedder, err := artifacts.NewVertex()
	if err != nil {
		logging.Fatal("periodicsync: vertex embedder", "error", err)
	}
	deps := model.Deps{IDKey: []byte(env.Required("ID_KEY")), Static: static.Files{Root: "web/who"}, Objects: bucket, Embedder: embedder}
	models, err := model.NewStore(source, source, store.NewQueue(), deps)
	if err != nil {
		logging.Fatal("periodicsync: load the models", "error", err)
	}
	opts := calendarimport.Options{Source: source, Store: models, AnthropicKey: key, DryRun: *dryRun}
	if err := calendarimport.RunPDF(ctx, opts); err != nil {
		logging.Fatal("periodicsync: year calendar pdf", "error", err)
	}
	slog.InfoContext(ctx, "periodicsync: completed")
}
