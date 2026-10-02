package model

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

const (
	sharedSheet         = "birthdayshared"
	sharedNewsletterTab = "Newsletter"
	exportActor         = "birthday@heliosian.com"
)

var SharedNewsletterColumns = []string{"Staff Name", "Staff Email", "Staff Birthday", "Target Newsletter Date", "Contacted On", "Charity Selected On", "Charity Name", "Charity Link", "Charity Blurb", "Note", "Preference"}

const (
	exportDay    = time.Thursday
	exportHour   = 23
	exportMinute = 59
)

func nextExport(t time.Time) time.Time {
	t = t.In(Location)
	at := time.Date(t.Year(), t.Month(), t.Day(), exportHour, exportMinute, 0, 0, Location)
	for at.Weekday() != exportDay || !at.After(t) {
		at = time.Date(at.Year(), at.Month(), at.Day()+1, exportHour, exportMinute, 0, 0, Location)
	}
	return at
}

func weekIssue(m *Birthdays, t time.Time) string {
	from := t.In(Location).Format(DateFormat)
	to := t.In(Location).AddDate(0, 0, 7).Format(DateFormat)
	for _, n := range m.NewsletterDates {
		if n.Date > from && n.Date <= to {
			return n.Date
		}
	}
	return ""
}

// Keeps the wall clock rather than now, which tests pin to a day long past.
func (a birthdaysApp) exportLoop() {
	for {
		at := nextExport(time.Now())
		time.Sleep(time.Until(at))
		ctx := context.Background()
		issue := weekIssue(a.store.Model().Birthdays, at)
		if issue == "" {
			slog.InfoContext(ctx, "birthday: no issue this week to copy to the shared sheet")
			continue
		}
		n, err := a.weeklyExport(ctx, issue)
		if err != nil {
			slog.ErrorContext(ctx, "birthday: weekly copy to the shared sheet", "issue", issue, "copied", n, "error", err)
			continue
		}
		slog.InfoContext(ctx, "birthday: weekly copy to the shared sheet", "issue", issue, "copied", n)
	}
}

type exported struct {
	email, year string
	row         map[string]string
	donation    map[string]string
}

func (m *Model) toExport(issue string, now time.Time) []exported {
	out := []exported{}
	b := m.Birthdays
	day := birthdayDayOf(now)
	for i := range b.Birthdays {
		sv := m.staffOf(b.Birthdays[i].Email, now)
		if !sv.InDirectory || sv.Level == LevelSkip || sv.NewsletterDate != issue {
			continue
		}
		if sv.Donation != nil && sv.Donation.UsedOn != "" {
			continue
		}
		key, note := b.fallback(sv.Email, sv.Year)
		selected := ""
		donation := map[string]string{}
		if sv.Donation != nil {
			key, note, selected = sv.Donation.Charity, sv.Donation.Note, sv.Donation.RecordedOn
		} else {
			donation["Charity"], donation["Note"], donation["Recorded On"], donation["Recorded By"] = key, note, day, ""
		}
		c := b.Charity(key)
		name, link, about := c.Name, c.DonationLink, c.About
		out = append(out, exported{
			email: sv.Email, year: sv.Year, donation: donation,
			row: map[string]string{
				"Staff Name": sv.Name, "Staff Email": sv.Email, "Staff Birthday": sv.BirthdayThisYear,
				"Target Newsletter Date": issue, "Contacted On": sv.ContactedOn, "Charity Selected On": selected,
				"Charity Name": name, "Charity Link": link, "Charity Blurb": about, "Note": note, "Preference": sv.Level,
			},
		})
	}
	return out
}

func (s *Store) stageExport(tx *store.Tx, rows, marks []store.Op) error {
	if err := s.Stage(tx, sharedSheet, rows...); err != nil {
		return fmt.Errorf("copy to the shared sheet: %w", err)
	}
	if err := s.Stage(tx, birthdaysAppName, marks...); err != nil {
		return fmt.Errorf("mark the copied birthdays done: %w", err)
	}
	return nil
}

func (a birthdaysApp) weeklyExport(ctx context.Context, issue string) (int, error) {
	actor := access.System(exportActor)
	at := now()
	rows, marks, err := weeklyExport(actor, a.store.Model().toExport(issue, at), at)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	if _, err := a.queue.Transact(ctx, actor, func(tx *store.Tx) error { return a.store.stageExport(tx, rows, marks) }); err != nil {
		return 0, err
	}
	return len(rows), nil
}
