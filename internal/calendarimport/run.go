package calendarimport

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	gcal "google.golang.org/api/calendar/v3"

	"heliosian/internal/cells"
	"heliosian/internal/data"
	"heliosian/internal/model"
	"heliosian/internal/store"
)

var legendDayTypes = []string{model.NoSchoolDayType, model.EarlyDismissalDayType}

type Options struct {
	Source       data.Source
	Cache        *model.CalendarCache
	Calendar     *gcal.Service
	Roster       func() model.Roster
	AnthropicKey string
	DryRun       bool
}

type run struct {
	opts       Options
	roster     model.Roster
	client     anthropic.Client
	tables     store.Tables
	dayTypes   []string
	dayTypeIDs map[string]string
	tags       []model.CalendarTag
	failures   []string
}

func begin(ctx context.Context, opts Options) (*run, error) {
	if opts.DryRun {
		slog.InfoContext(ctx, "calendar import: dry run, nothing will be written to the sheet")
	}
	roster := opts.Roster()
	slog.InfoContext(ctx, "calendar import: roster", "classrooms", len(roster.Classrooms))
	names := []string{model.GoogleTab, model.PDFTab, model.EnrichmentTab, model.DayTypesTab, model.TagsTab}
	tabs, err := opts.Source.Tabs(context.Background(), "calendar", names, nil)
	if err != nil {
		return nil, fmt.Errorf("read calendar tables: %w", err)
	}
	tables := store.Tables{}
	for _, name := range names {
		tables[name] = tabs[name].Rows
	}
	r, err := vocabulary(roster, tables)
	if err != nil {
		return nil, err
	}
	r.opts, r.client = opts, anthropic.NewClient(option.WithAPIKey(opts.AnthropicKey))
	return r, nil
}

func vocabulary(roster model.Roster, tables store.Tables) (*run, error) {
	dayTypes := []string{}
	dayTypeIDs := map[string]string{}
	for _, row := range tables[model.DayTypesTab] {
		dayTypes = append(dayTypes, row["Day Type"])
		dayTypeIDs[row["Day Type"]] = row["Day Type ID"]
	}
	tags := []model.CalendarTag{}
	for _, row := range tables[model.TagsTab] {
		if model.BuiltInTag(row["Tag ID"]) {
			continue
		}
		tags = append(tags, model.CalendarTag{ID: row["Tag ID"], Name: row["Tag"], Description: row["Description"]})
	}
	if len(tags) == 0 {
		return nil, fmt.Errorf("%s needs rows before the import can run", model.TagsTab)
	}
	for _, key := range append([]string{model.RegularDayType}, legendDayTypes...) {
		if !slices.Contains(slices.Collect(maps.Values(dayTypeIDs)), key) {
			return nil, fmt.Errorf("%s needs a row with the id %s before the import can run", model.DayTypesTab, key)
		}
	}
	return &run{roster: roster, tables: tables, dayTypes: dayTypes, dayTypeIDs: dayTypeIDs, tags: tags}, nil
}

func (r *run) tagCell(names []string) (string, error) {
	out := []string{}
	for _, name := range names {
		key := r.roster.IDOf(name)
		if i := slices.IndexFunc(r.tags, func(t model.CalendarTag) bool { return t.Name == name }); key == "" && i >= 0 {
			key = r.tags[i].ID
		}
		if key == "" {
			return "", fmt.Errorf("%q is neither a classroom nor a tag", name)
		}
		out = append(out, key)
	}
	return cells.JoinList(out), nil
}

func (r *run) dayTypeID(name string) (string, error) {
	key, ok := r.dayTypeIDs[name]
	if !ok {
		return "", fmt.Errorf("%q is not a day type", name)
	}
	return key, nil
}

func RunGoogle(ctx context.Context, opts Options) error {
	r, err := begin(ctx, opts)
	if err != nil {
		return err
	}
	from, to := window(time.Now().In(model.Location))
	google, err := feedRows(ctx, opts.Calendar, from, to)
	if err != nil {
		return fmt.Errorf("read the school calendar: %w", err)
	}
	slog.InfoContext(ctx, "calendar import: feed read", "events", len(google), "from", from.Format(model.DateFormat))
	inWindow := func(row map[string]string) bool {
		start, err := time.ParseInLocation(model.DateFormat, row["Start"][:min(len(row["Start"]), len(model.DateFormat))], model.Location)
		return err == nil && !start.Before(from) && start.Before(to)
	}
	rows := slices.Clone(google)
	for _, row := range r.tables[model.GoogleTab] {
		if !inWindow(row) {
			rows = append(rows, row)
		}
	}
	r.identify(rows, r.tables[model.GoogleTab])
	enrichment := r.enrich(ctx, google, false)
	slog.InfoContext(ctx, "calendar import: rows", "feed", len(rows), "enriched", len(enrichment))
	return r.write(ctx, []tabSync{
		{model.GoogleTab, model.GoogleColumns, rows, r.tables[model.GoogleTab], "Key", true},
		{model.EnrichmentTab, model.EnrichmentColumns, enrichment, r.tables[model.EnrichmentTab], "Event ID", false},
	})
}

func RunPDF(ctx context.Context, opts Options) error {
	r, err := begin(ctx, opts)
	if err != nil {
		return err
	}
	page, err := fetch(ctx, pageURL)
	if err != nil {
		return fmt.Errorf("fetch the school calendar page: %w", err)
	}
	pdfURL, err := findPDF(page)
	if err != nil {
		return err
	}
	pdf, err := fetch(ctx, pdfURL)
	if err != nil {
		return fmt.Errorf("fetch the year calendar pdf: %w", err)
	}
	pdfHash := digest(string(pdf), legendSystem, entriesSystem(r.roster), monthSystem(""))[:12]
	rows := r.tables[model.PDFTab]
	known := false
	for _, row := range r.tables[model.PDFTab] {
		if row["PDF"] == pdfHash {
			known = true
		}
	}
	if known {
		slog.InfoContext(ctx, "calendar import: pdf unchanged", "url", pdfURL, "hash", pdfHash, "kept", len(r.tables[model.PDFTab]))
	} else {
		slog.InfoContext(ctx, "calendar import: pdf is new, reading it", "url", pdfURL, "hash", pdfHash)
		fresh, year, err := r.readPDF(ctx, pdf, pdfHash)
		if err != nil {
			slog.ErrorContext(ctx, "calendar import: read the year calendar pdf, keeping the rows already there", "kept", len(r.tables[model.PDFTab]), "error", err)
			r.failures = append(r.failures, "the year calendar pdf")
		} else {
			rows = []map[string]string{}
			for _, row := range r.tables[model.PDFTab] {
				if row["Year"] != year {
					rows = append(rows, row)
				}
			}
			rows = append(rows, fresh...)
			slog.InfoContext(ctx, "calendar import: pdf entries", "entries", len(fresh), "year", year)
		}
	}
	r.identify(rows, r.tables[model.PDFTab])
	enrichment := r.enrich(ctx, rows, true)
	slog.InfoContext(ctx, "calendar import: rows", "pdf", len(rows), "enriched", len(enrichment))
	return r.write(ctx, []tabSync{
		{model.PDFTab, model.PDFColumns, rows, r.tables[model.PDFTab], "Key", true},
		{model.EnrichmentTab, model.EnrichmentColumns, enrichment, r.tables[model.EnrichmentTab], "Event ID", false},
	})
}

func (r *run) identify(rows, before []map[string]string) {
	known := map[string]string{}
	for _, row := range before {
		known[row["Key"]] = row["Event ID"]
	}
	mint := r.opts.Cache.Model().Minter()
	for _, row := range rows {
		switch {
		case row["Event ID"] != "":
		case known[row["Key"]] != "":
			row["Event ID"] = known[row["Key"]]
		default:
			row["Event ID"] = mint()
		}
	}
}

type tabSync struct {
	tab    string
	header []string
	rows   []map[string]string
	before []map[string]string
	keyCol string
	mirror bool
}

func (r *run) write(ctx context.Context, tabs []tabSync) error {
	ops := []store.Op{}
	for _, s := range tabs {
		tabOps, added, changed, removed := syncOps(importer, s)
		slog.InfoContext(ctx, "calendar import: tab changes", "tab", s.tab, "added", added, "changed", changed, "removed", removed)
		ops = append(ops, tabOps...)
	}
	if r.opts.DryRun {
		slog.InfoContext(ctx, "calendar import: dry run, row changes not committed", "changes", len(ops))
	} else if err := r.opts.Cache.CommitAndWait(context.Background(), importer, ops...); err != nil {
		return fmt.Errorf("commit the import: %w", err)
	}
	if len(r.failures) > 0 {
		return fmt.Errorf("%d stages failed and will be retried next run: %s", len(r.failures), strings.Join(r.failures, "; "))
	}
	return nil
}
