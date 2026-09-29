package model

import (
	"context"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/id"
	"heliosian/internal/store"
)

const (
	feedbackAppName = "feedback"
	reportsTab      = "Reports"

	ReportStatusNew       = "New"
	ReportStatusFiled     = "Filed"
	ReportStatusDismissed = "Dismissed"
)

var ReportColumns = []string{
	"ID", "Received", "App", "App Name", "Kind", "Status", "Summary", "Details",
	"Email", "Role", "URL", "Page", "Browser", "Viewport", "Screen",
	"Language", "Time Zone", "Errors", "Issue", "Handled", "Handled By", "Screenshot",
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
		"Role":       reportRole(r.SuperAdmin),
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
		"Screenshot": r.Screenshot,
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
		status = ReportStatusNew
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
		Screenshot: row["Screenshot"],
		Issue:      row["Issue"],
		Handled:    handled,
		HandledBy:  row["Handled By"],
	}
}

type Feedback struct {
	reports []Report
	aliases id.Aliases
}

func buildFeedback(_ context.Context, tables store.Tables) (*Feedback, error) {
	aliases, err := id.ParseAliases(tables[id.AliasesTab])
	if err != nil {
		return nil, err
	}
	m := &Feedback{reports: []Report{}, aliases: aliases}
	for _, row := range tables[reportsTab] {
		if strings.TrimSpace(row["ID"]) == "" {
			continue
		}
		m.reports = append(m.reports, reportFromRow(row))
	}
	slices.SortStableFunc(m.reports, func(a, b Report) int { return b.At.Compare(a.At) })
	return m, nil
}

var feedbackTabs = []store.Tab{
	{Name: reportsTab, Columns: ReportColumns, Key: []string{"ID"}},
	{Name: id.AliasesTab, Columns: id.AliasColumns, Key: []string{id.AliasColumn}},
}

func (m *Feedback) Reports() []Report {
	return m.reports
}

func (m *Feedback) report(key string) (Report, bool) {
	key = m.aliases.Resolve(key)
	for _, r := range m.reports {
		if r.ID == key {
			return r, true
		}
	}
	return Report{}, false
}

func (m *Feedback) taken(key string) bool {
	_, ok := m.report(key)
	return ok || m.aliases[key] != ""
}

func (m *Feedback) Report(id string) (Report, bool) {
	return m.report(id)
}

func (s *Store) saveReport(ctx context.Context, actor access.Actor, r Report) (Report, error) {
	saved, ops := s.Model().Feedback.submit(actor, r)
	if err := s.Commit(ctx, actor, feedbackAppName, ops...); err != nil {
		return Report{}, err
	}
	return saved, nil
}
