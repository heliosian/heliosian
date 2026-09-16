// Command periodicsync runs every stage of the periodic sync in turn: the calendar import, then the Google Groups reconciliation.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	gapi "google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"heliosian/internal/app"
	"heliosian/internal/calendar"
	"heliosian/internal/calendarimport"
	"heliosian/internal/celebrate"
	"heliosian/internal/data"
	"heliosian/internal/events"
	"heliosian/internal/groups"
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

// trusting answers for the images an Events or Celebrate row names: the
// bucket's are taken on trust, since this tool carries no bucket client,
// and bundled files are checked on disk.
type trusting struct {
	folder string
	roots  []string
}

func (t trusting) Has(key string) (bool, error) {
	if strings.HasPrefix(key, t.folder) {
		return true, nil
	}
	for _, root := range t.roots {
		if found, _ := (staticFiles{root}).Has(key); found {
			return true, nil
		}
	}
	return false, nil
}

func (trusting) Prefetch([]string) error { return nil }

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
	raw, err := os.ReadFile("creds/anthropic.key")
	if err != nil {
		log.Fatalf("[ERROR] read creds/anthropic.key (or set ANTHROPIC_API_KEY): %v", err)
	}
	key := strings.TrimSpace(string(raw))
	if key == "" {
		log.Fatal("[ERROR] creds/anthropic.key is empty")
	}
	return key
}

// syncGroups is the groups stage: every group's members worked out from
// the sheets as they stand, and Google brought in step.
func syncGroups(ctx context.Context, source *data.Sheet, directory *who.Model, dryRun bool) error {
	whoTables, err := who.ReadTables(source)
	if err != nil {
		return err
	}
	portalTables, err := events.ReadTables(source)
	if err != nil {
		return err
	}
	portal, err := events.BuildModel(portalTables, trusting{"activity-images/", []string{"web/team", "web/public/team"}})
	if err != nil {
		return err
	}
	siteTables, err := celebrate.ReadTables(source)
	if err != nil {
		return err
	}
	site, err := celebrate.BuildModel(siteTables, trusting{"party-images/", []string{"web/celebrate", "web/public/celebrate"}})
	if err != nil {
		return err
	}
	tables, err := groups.ReadTables(source)
	if err != nil {
		return err
	}
	model, err := groups.BuildModel(tables)
	if err != nil {
		return err
	}
	now := time.Now().In(calendar.Location)
	sources := groups.Sources{
		Directory: directory,
		Tags:      func(owner string) map[string][]string { return who.TagsOf(whoTables.Tags, directory, owner) },
		Lists: func(owner string) []who.List {
			return append(directory.RoomParentLists(owner), app.SmartLists(directory, portal, site, owner, now)...)
		},
		Shared: func(email string) []who.SharedTag {
			return who.SharedTagsOf(whoTables.Tags, whoTables.Managers, directory, email)
		},
	}
	desired := groups.Plan(model, sources)
	if dryRun {
		for _, d := range desired {
			log.Printf("dry run: %s would hold %d members", d.Address(), len(d.Members))
		}
		return nil
	}
	google, err := groups.NewCloudIdentity(ctx)
	if err != nil {
		return err
	}
	result := groups.Reconcile(ctx, google, desired, log.Printf)
	log.Printf("groups: %d groups, %d members added, %d removed, %d orphans", result.Groups, result.Added, result.Removed, len(result.Orphans))
	if len(result.Errors) > 0 {
		names := []string{}
		for name := range result.Errors {
			names = append(names, name)
		}
		return &stageError{"groups failed for " + strings.Join(names, ", ")}
	}
	return nil
}

type stageError struct{ words string }

func (e *stageError) Error() string { return e.words }

func main() {
	dryRun := flag.Bool("dry-run", false, "report what the run would change, writing nothing to the sheets or to Google")
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
		"events":      requiredEnv("EVENTS_SHEET"),
		"celebrate":   requiredEnv("CELEBRATE_SHEET"),
		"groups":      requiredEnv("GROUPS_SHEET"),
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
	directory, err := who.LoadModel(source, nil, staticFiles{"web/who"})
	if err != nil {
		log.Fatalf("[ERROR] load directory model: %v", err)
	}
	// Each stage runs whatever the one before did: a failure is recorded
	// and the run exits non-zero at the end naming every stage that failed.
	failures := []string{}
	log.Printf("stage: calendar import")
	if err := calendarimport.Run(ctx, calendarimport.Options{Source: source, Sheets: svc, CalendarSheet: calendarSheet, Directory: directory, AnthropicKey: key, DryRun: *dryRun}); err != nil {
		log.Printf("[ERROR] calendar import: %v", err)
		failures = append(failures, "calendar import")
	}
	log.Printf("stage: groups")
	if err := syncGroups(ctx, source, directory, *dryRun); err != nil {
		log.Printf("[ERROR] groups: %v", err)
		failures = append(failures, "groups")
	}
	if len(failures) > 0 {
		log.Fatalf("[ERROR] %d stages failed: %s", len(failures), strings.Join(failures, "; "))
	}
	log.Printf("every stage completed")
}
