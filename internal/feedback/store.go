package feedback

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/logging"
)

const (
	appName         = "feedback"
	reportsTab      = "Reports"
	refreshInterval = 5 * time.Minute

	StatusNew       = "New"
	StatusFiled     = "Filed"
	StatusDismissed = "Dismissed"
)

// ReportColumns is the Reports tab, the inbox every report lands in and the
// admin page works from.
var ReportColumns = []string{
	"ID", "Received", "App", "App Name", "Kind", "Status", "Summary", "Details",
	"Email", "Role", "URL", "Page", "Browser", "Viewport", "Screen",
	"Language", "Time Zone", "Errors", "Issue", "Handled", "Handled By",
}

// Enqueuer serializes sheet writes; the directory's write queue is shared here.
type Enqueuer interface {
	Add(func())
}

func newID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		logging.Fatal("feedback: read random bytes", "error", err)
	}
	return hex.EncodeToString(b)
}

func (r Report) cells() map[string]string {
	return map[string]string{
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

func reportFromRow(row map[string]string) Report {
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

// Store is the Reports tab: every report kept, newest first, refreshed on the
// same five minutes as every other model and updated in place as the admin
// page files and dismisses.
type Store struct {
	source  data.Source
	writer  data.Writer
	queue   Enqueuer
	mu      sync.RWMutex
	reports []Report
	edits   int
}

func NewStore(source data.Source, writer data.Writer, queue Enqueuer) (*Store, error) {
	s := &Store{source: source, writer: writer, queue: queue}
	if err := s.refresh(); err != nil {
		return nil, err
	}
	go s.refreshLoop()
	return s, nil
}

func (s *Store) refreshLoop() {
	for range time.Tick(refreshInterval) {
		s.Refresh()
	}
}

func (s *Store) Refresh() {
	s.queue.Add(func() {
		if err := s.refresh(); err != nil {
			slog.Error("feedback model refresh", "error", err)
		}
	})
}

func (s *Store) refresh() error {
	start := time.Now()
	s.mu.RLock()
	before := s.edits
	s.mu.RUnlock()
	header, rows, err := s.source.Table(appName, reportsTab)
	if err != nil {
		return err
	}
	if err := data.CheckColumns(reportsTab, header, ReportColumns); err != nil {
		return err
	}
	reports := make([]Report, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row["ID"]) == "" {
			continue
		}
		reports = append(reports, reportFromRow(row))
	}
	s.mu.Lock()
	if s.edits != before {
		s.mu.Unlock()
		slog.Info("feedback model refresh skipped: edited while reading")
		return nil
	}
	s.reports = reports
	s.mu.Unlock()
	slog.Info("loaded feedback model", "reports", len(reports), "took", time.Since(start).Round(time.Millisecond))
	return nil
}

// Reports is every report, newest first.
func (s *Store) Reports() []Report {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := slices.Clone(s.reports)
	slices.SortStableFunc(out, func(a, b Report) int { return b.At.Compare(a.At) })
	return out
}

func (s *Store) Report(id string) (Report, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.reports {
		if r.ID == id {
			return r, true
		}
	}
	return Report{}, false
}

// Save appends a freshly submitted report, giving it its id and New status.
func (s *Store) Save(r Report) (Report, error) {
	r.ID = newID()
	r.Status = StatusNew
	done := make(chan error, 1)
	s.queue.Add(func() {
		s.mu.Lock()
		s.reports = append(s.reports, r)
		s.edits++
		s.mu.Unlock()
		done <- s.writer.Insert(appName, reportsTab, []map[string]string{r.cells()})
	})
	return r, <-done
}

// update writes one report's cells and keeps the loaded copy in step.
func (s *Store) update(r Report) error {
	s.mu.Lock()
	for i, have := range s.reports {
		if have.ID == r.ID {
			s.reports[i] = r
		}
	}
	s.edits++
	s.mu.Unlock()
	s.queue.Add(func() {
		if err := s.writer.Set(appName, reportsTab, map[string]string{"ID": r.ID}, r.cells()); err != nil {
			slog.Error("feedback write", "id", r.ID, "error", err)
		}
	})
	return nil
}

// Filed marks a report filed, against the issue it became.
func (s *Store) Filed(id, issue, actor string, now time.Time) error {
	r, ok := s.Report(id)
	if !ok {
		return fmt.Errorf("feedback: no report %q", id)
	}
	r.Status, r.Issue, r.Handled, r.HandledBy = StatusFiled, issue, now, actor
	return s.update(r)
}

// Dismissed marks a report handled without an issue.
func (s *Store) Dismissed(id, actor string, now time.Time) error {
	r, ok := s.Report(id)
	if !ok {
		return fmt.Errorf("feedback: no report %q", id)
	}
	r.Status, r.Handled, r.HandledBy = StatusDismissed, now, actor
	return s.update(r)
}
