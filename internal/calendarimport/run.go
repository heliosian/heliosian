package calendarimport

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	gcal "google.golang.org/api/calendar/v3"

	"heliosian/internal/data"
	"heliosian/internal/store"
	"heliosian/internal/when"
)

var legendDayTypes = []string{"No School", "Early Dismissal"}

type Options struct {
	Source       data.Source
	Cache        *when.Cache
	Calendar     *gcal.Service
	Roster       func() when.Roster
	AnthropicKey string
	DryRun       bool
}

type run struct {
	opts     Options
	roster   when.Roster
	client   anthropic.Client
	tables   store.Tables
	dayTypes []string
	tags     []when.Tag
	failures []string
}

func begin(ctx context.Context, opts Options) (*run, error) {
	if opts.DryRun {
		slog.InfoContext(ctx, "calendar import: dry run, nothing will be written to the sheet")
	}
	roster := opts.Roster()
	slog.InfoContext(ctx, "calendar import: roster", "classrooms", len(roster.Classrooms))
	names := []string{when.GoogleTab, when.PDFTab, when.EnrichmentTab, when.DayTypesTab, when.TagsTab}
	tabs, err := opts.Source.Tabs(context.Background(), "calendar", names, nil)
	if err != nil {
		return nil, fmt.Errorf("read calendar tables: %w", err)
	}
	tables := store.Tables{}
	for _, name := range names {
		tables[name] = tabs[name].Rows
	}
	dayTypes := []string{}
	for _, row := range tables[when.DayTypesTab] {
		dayTypes = append(dayTypes, row["Day Type"])
	}
	tags := []when.Tag{}
	for _, row := range tables[when.TagsTab] {
		tags = append(tags, when.Tag{Name: row["Tag"], Description: row["Description"]})
	}
	if len(tags) == 0 {
		return nil, fmt.Errorf("%s needs rows before the import can run", when.TagsTab)
	}
	for _, name := range append([]string{when.RegularDayType}, legendDayTypes...) {
		if !slices.Contains(dayTypes, name) {
			return nil, fmt.Errorf("%s needs a %q row before the import can run", when.DayTypesTab, name)
		}
	}
	return &run{opts: opts, roster: roster, client: anthropic.NewClient(option.WithAPIKey(opts.AnthropicKey)), tables: tables, dayTypes: dayTypes, tags: tags}, nil
}

func RunGoogle(ctx context.Context, opts Options) error {
	r, err := begin(ctx, opts)
	if err != nil {
		return err
	}
	from, to := window(time.Now().In(when.Location))
	google, err := feedRows(ctx, opts.Calendar, from, to)
	if err != nil {
		return fmt.Errorf("read the school calendar: %w", err)
	}
	slog.InfoContext(ctx, "calendar import: feed read", "events", len(google), "from", from.Format(when.DateFormat))
	inWindow := func(row map[string]string) bool {
		start, err := time.ParseInLocation(when.DateFormat, row["Start"][:min(len(row["Start"]), len(when.DateFormat))], when.Location)
		return err == nil && !start.Before(from) && start.Before(to)
	}
	rows := slices.Clone(google)
	for _, row := range r.tables[when.GoogleTab] {
		if !inWindow(row) {
			rows = append(rows, row)
		}
	}
	enrichment := r.enrich(ctx, google, false)
	slog.InfoContext(ctx, "calendar import: rows", "feed", len(rows), "enriched", len(enrichment))
	return r.write(ctx, []tabSync{
		{when.GoogleTab, when.GoogleColumns, rows, r.tables[when.GoogleTab], "Key", true},
		{when.EnrichmentTab, when.EnrichmentColumns, enrichment, r.tables[when.EnrichmentTab], "Event ID", false},
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
	rows := r.tables[when.PDFTab]
	known := false
	for _, row := range r.tables[when.PDFTab] {
		if row["PDF"] == pdfHash {
			known = true
		}
	}
	if known {
		slog.InfoContext(ctx, "calendar import: pdf unchanged", "url", pdfURL, "hash", pdfHash, "kept", len(r.tables[when.PDFTab]))
	} else {
		slog.InfoContext(ctx, "calendar import: pdf is new, reading it", "url", pdfURL, "hash", pdfHash)
		fresh, year, err := readPDF(ctx, r.client, pdf, pdfHash, r.roster, r.dayTypes)
		if err != nil {
			slog.ErrorContext(ctx, "calendar import: read the year calendar pdf, keeping the rows already there", "kept", len(r.tables[when.PDFTab]), "error", err)
			r.failures = append(r.failures, "the year calendar pdf")
		} else {
			rows = []map[string]string{}
			for _, row := range r.tables[when.PDFTab] {
				if row["Year"] != year {
					rows = append(rows, row)
				}
			}
			rows = append(rows, fresh...)
			slog.InfoContext(ctx, "calendar import: pdf entries", "entries", len(fresh), "year", year)
		}
	}
	enrichment := r.enrich(ctx, rows, true)
	slog.InfoContext(ctx, "calendar import: rows", "pdf", len(rows), "enriched", len(enrichment))
	return r.write(ctx, []tabSync{
		{when.PDFTab, when.PDFColumns, rows, r.tables[when.PDFTab], "Key", true},
		{when.EnrichmentTab, when.EnrichmentColumns, enrichment, r.tables[when.EnrichmentTab], "Event ID", false},
	})
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
