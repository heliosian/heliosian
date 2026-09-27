package birthday

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/store"
	"heliosian/internal/when"
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
	t = t.In(when.Location)
	at := time.Date(t.Year(), t.Month(), t.Day(), exportHour, exportMinute, 0, 0, when.Location)
	for at.Weekday() != exportDay || !at.After(t) {
		at = time.Date(at.Year(), at.Month(), at.Day()+1, exportHour, exportMinute, 0, 0, when.Location)
	}
	return at
}

func weekIssue(model *Model, t time.Time) string {
	from := t.In(when.Location).Format(DateFormat)
	to := t.In(when.Location).AddDate(0, 0, 7).Format(DateFormat)
	for _, d := range model.NewsletterDates {
		if d > from && d <= to {
			return d
		}
	}
	return ""
}

// Keeps the wall clock rather than now, which tests pin to a day long past.
func (a app) exportLoop() {
	for {
		at := nextExport(time.Now())
		time.Sleep(time.Until(at))
		ctx := context.Background()
		issue := weekIssue(a.cache.Model(), at)
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

func (a app) toExport(model *Model, issue string) []exported {
	out := []exported{}
	day := today()
	for i := range model.Birthdays {
		sv, ok := a.staffView(model, model.Birthdays[i].Email)
		if !ok || !sv.InDirectory || sv.Level == LevelSkip || sv.NewsletterDate != issue {
			continue
		}
		if sv.Donation != nil && sv.Donation.UsedOn != "" {
			continue
		}
		name, note, selected := model.Settings.DefaultCharity, "", ""
		donation := map[string]string{}
		if sv.Donation != nil {
			name, note, selected = sv.Donation.Charity, sv.Donation.Note, sv.Donation.RecordedOn
		} else {
			donation["Charity"], donation["Note"], donation["Recorded On"], donation["Recorded By"] = name, "", day, ""
		}
		link, about := "", ""
		if c := model.Charity(name); c != nil {
			link, about = c.DonationLink, c.About
		}
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

func (a app) commitExport(ctx context.Context, actor access.Actor, rows, marks []store.Op) error {
	if len(rows) == 0 {
		return nil
	}
	if err := a.cache.shared.Commit(ctx, actor, rows...); err != nil {
		return fmt.Errorf("copy to the shared sheet: %w", err)
	}
	if err := a.cache.Commit(ctx, actor, marks...); err != nil {
		return fmt.Errorf("mark the copied birthdays done: %w", err)
	}
	return nil
}

func (a app) weeklyExport(ctx context.Context, issue string) (int, error) {
	actor := access.System(exportActor)
	model := a.cache.Model()
	rows, marks, err := weeklyExport(actor, a.toExport(model, issue))
	if err != nil {
		return 0, err
	}
	if err := a.commitExport(ctx, actor, rows, marks); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func (a app) shareIssue(r *http.Request, body dateRef) (map[string]int, error) {
	actor := a.actor(r)
	issue := strings.TrimSpace(body.Date)
	model := a.cache.Model()
	rows, marks, err := model.shareIssue(actor, issue, a.toExport(model, issue))
	if err != nil {
		return nil, err
	}
	if err := a.commitExport(r.Context(), actor, rows, marks); err != nil {
		slog.ErrorContext(r.Context(), "birthday: copy to the shared sheet", "actor", actor.Email, "issue", issue, "copied", 0, "error", err)
		return nil, access.Refuse(http.StatusBadGateway, "copied 0, then: %v", err)
	}
	slog.InfoContext(r.Context(), "birthday: copied to the shared sheet", "actor", actor.Email, "issue", issue, "copied", len(rows))
	return map[string]int{"copied": len(rows)}, nil
}
