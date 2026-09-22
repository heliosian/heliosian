// Command periodicsync runs every stage of the periodic sync in turn: the year calendar PDF.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"

	gapi "google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"heliosian/internal/app"
	"heliosian/internal/calendar"
	"heliosian/internal/calendarimport"
	"heliosian/internal/data"
	"heliosian/internal/who"
)

type staticFiles struct {
	root string
}

func (s staticFiles) Has(key string) (bool, error) {
	_, err := os.Stat(filepath.Join(s.root, filepath.FromSlash(key)))
	return err == nil, nil
}

func (staticFiles) Prefetch([]string) error { return nil }

func requiredEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("[ERROR] %s is required", name)
	}
	return value
}

func apiKey() string {
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return key
	}
	raw, err := os.ReadFile("local/creds/anthropic.key")
	if err != nil {
		log.Fatalf("[ERROR] read local/creds/anthropic.key (or set ANTHROPIC_API_KEY): %v", err)
	}
	key := strings.TrimSpace(string(raw))
	if key == "" {
		log.Fatal("[ERROR] local/creds/anthropic.key is empty")
	}
	return key
}

func main() {
	dryRun := flag.Bool("dry-run", false, "report what the run would change, writing nothing to the sheets")
	permitted := flag.Bool("i-have-user-permission-to-spend-money", false, "every run that reaches Claude costs real money; pass this only when the person paying has said to run it")
	flag.Parse()
	if !*permitted {
		log.Fatal("[ERROR] this run spends money on Claude; pass --i-have-user-permission-to-spend-money only when the user has said to run it")
	}
	calendarSheet := requiredEnv("CALENDAR_SHEET")
	spreadsheets := map[string]string{
		"calendar":    calendarSheet,
		"directory":   requiredEnv("DIRECTORY_SHEET"),
		"preferences": requiredEnv("PREFERENCES_SHEET"),
		"config":      requiredEnv("CONFIG_SHEET"),
	}
	key := apiKey()
	ctx := context.Background()
	source, err := data.NewSheet(spreadsheets)
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	svc, err := sheets.NewService(ctx, gapi.WithScopes(sheets.SpreadsheetsScope))
	if err != nil {
		log.Fatalf("[ERROR] create sheets client: %v", err)
	}
	directory, err := who.LoadModel(source, nil, staticFiles{"web/who"}, []byte("periodicsync"))
	if err != nil {
		log.Fatalf("[ERROR] load directory model: %v", err)
	}
	// Each stage runs whatever the one before did: a failure is recorded
	// and the run exits non-zero at the end naming every stage that failed.
	failures := []string{}
	log.Printf("stage: year calendar pdf")
	opts := calendarimport.Options{Source: source, Sheets: svc, CalendarSheet: calendarSheet, Roster: func() calendar.Roster { return app.CalendarRoster(directory) }, AnthropicKey: key, DryRun: *dryRun}
	if err := calendarimport.RunPDF(ctx, opts); err != nil {
		log.Printf("[ERROR] year calendar pdf: %v", err)
		failures = append(failures, "year calendar pdf")
	}
	if len(failures) > 0 {
		log.Fatalf("[ERROR] %d stages failed: %s", len(failures), strings.Join(failures, "; "))
	}
	log.Printf("every stage completed")
}
