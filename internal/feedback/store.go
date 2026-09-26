package feedback

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/logging"
	"heliosian/internal/store"
)

const (
	appName    = "feedback"
	reportsTab = "Reports"

	StatusNew       = "New"
	StatusFiled     = "Filed"
	StatusDismissed = "Dismissed"
)

var ReportColumns = []string{
	"ID", "Received", "App", "App Name", "Kind", "Status", "Summary", "Details",
	"Email", "Role", "URL", "Page", "Browser", "Viewport", "Screen",
	"Language", "Time Zone", "Errors", "Issue", "Handled", "Handled By",
}

func newID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		logging.Fatal("feedback: read random bytes", "error", err)
	}
	return hex.EncodeToString(b)
}

func (r Report) cells() store.Row {
	return store.Row{
		"ID":         r.ID,
		"Received":   r.At.UTC().Format(time.RFC3339),
		"App":        r.App,
		"App Name":   r.AppName,
		"Kind":       r.Kind,
		"Status":     r.Status,
		"Summary":    r.Summary,
		"Details":    r.Details,
		"Email":      r.Email,
		"Role":       role(r.SuperAdmin),
		"URL":        r.URL,
		"Page":       r.Page,
		"Browser":    r.UserAgent,
		"Viewport":   r.Viewport,
		"Screen":     r.Screen,
		"Language":   r.Language,
		"Time Zone":  r.Timezone,
		"Errors":     strings.Join(r.Errors, "\n"),
		"Issue":      r.Issue,
		"Handled":    handledCell(r.Handled),
		"Handled By": r.HandledBy,
	}
}

func handledCell(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(time.RFC3339)
}

func reportFromRow(row store.Row) Report {
	errors := []string{}
	for _, line := range strings.Split(row["Errors"], "\n") {
		if line = strings.TrimSpace(line); line != "" {
			errors = append(errors, line)
		}
	}
	at, _ := time.Parse(time.RFC3339, row["Received"])
	handled, _ := time.Parse(time.RFC3339, row["Handled"])
	status := row["Status"]
	if status == "" {
		status = StatusNew
	}
	return Report{
		ID:         row["ID"],
		At:         at,
		App:        row["App"],
		AppName:    row["App Name"],
		Kind:       row["Kind"],
		Status:     status,
		Summary:    row["Summary"],
		Details:    row["Details"],
		Email:      row["Email"],
		SuperAdmin: row["Role"] == "super admin",
		URL:        row["URL"],
		Page:       row["Page"],
		UserAgent:  row["Browser"],
		Viewport:   row["Viewport"],
		Screen:     row["Screen"],
		Language:   row["Language"],
		Timezone:   row["Time Zone"],
		Errors:     errors,
		Issue:      row["Issue"],
		Handled:    handled,
		HandledBy:  row["Handled By"],
	}
}

type Model struct {
	reports []Report
}

func build(_ context.Context, tables store.Tables) (*Model, error) {
	m := &Model{reports: []Report{}}
	for _, row := range tables[reportsTab] {
		if strings.TrimSpace(row["ID"]) == "" {
			continue
		}
		m.reports = append(m.reports, reportFromRow(row))
	}
	slices.SortStableFunc(m.reports, func(a, b Report) int { return b.At.Compare(a.At) })
	return m, nil
}

type Cache struct {
	*store.Store[*Model]
}

func NewCache(source data.Source, writer data.Writer, queue *store.Queue) (*Cache, error) {
	s, err := store.New(store.Spec[*Model]{
		App:   appName,
		Tabs:  []store.Tab{{Name: reportsTab, Columns: ReportColumns, Key: []string{"ID"}}},
		Build: build,
		Loaded: func(m *Model, took time.Duration) {
			slog.Info("loaded feedback model", "reports", len(m.reports), "took", took.Round(time.Millisecond))
		},
	}, source, writer, queue)
	if err != nil {
		return nil, err
	}
	return &Cache{Store: s}, nil
}

func (c *Cache) Reports() []Report {
	return c.Model().reports
}

func (c *Cache) Report(id string) (Report, bool) {
	for _, r := range c.Model().reports {
		if r.ID == id {
			return r, true
		}
	}
	return Report{}, false
}

func (c *Cache) save(ctx context.Context, r Report) (Report, error) {
	r.ID = newID()
	r.Status = StatusNew
	if err := c.Commit(ctx, r.Email, store.Insert(reportsTab, r.cells())); err != nil {
		return Report{}, err
	}
	return r, nil
}

func (c *Cache) handle(ctx context.Context, id, actor string, cells store.Row) error {
	if _, ok := c.Report(id); !ok {
		return fmt.Errorf("feedback: no report %q", id)
	}
	return c.Commit(ctx, actor, store.Update(reportsTab, store.Row{"ID": id}, cells))
}

func (c *Cache) Filed(ctx context.Context, id, issue, actor string, now time.Time) error {
	return c.handle(ctx, id, actor, store.Row{"Status": StatusFiled, "Issue": issue, "Handled": handledCell(now), "Handled By": actor})
}

func (c *Cache) Dismissed(ctx context.Context, id, actor string, now time.Time) error {
	return c.handle(ctx, id, actor, store.Row{"Status": StatusDismissed, "Handled": handledCell(now), "Handled By": actor})
}
