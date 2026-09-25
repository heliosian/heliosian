package birthday

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"heliosian/internal/auth"
)

// The weekly export copies the birthdays an issue of the newsletter carries
// into the Newsletter tab of the Staff Birthday List (Shared) spreadsheet -
// the association's own record of the week's donations - and marks each of
// them done: every Thursday at 11:59 at night, school time, for the issue in
// the week ahead, and whenever the team presses Copy to Shared Sheet on the
// Newsletters page. Everyone the issue carries goes, No Newsletter included
// with that as their preference; whoever has no charity recorded by then is
// given the default charity, recorded with nobody as its recorder, since the
// default stands when they do not answer. Marking done is the donation's
// Used On, so a birthday already marked is not copied again.

const (
	// sharedSheet is the spreadsheet's key among the data sources
	// (BIRTHDAY_SHARED_SHEET), and sharedNewsletterTab the tab the rows go on.
	sharedSheet         = "birthdayshared"
	sharedNewsletterTab = "Newsletter"

	// exportActor is who the Thursday run marks the donations used by, the
	// Used By column wanting an address.
	exportActor = "birthday@heliosian.com"
)

// SharedNewsletterColumns is the shared Newsletter tab's header.
var SharedNewsletterColumns = []string{"Staff Name", "Staff Email", "Staff Birthday", "Target Newsletter Date", "Contacted On", "Charity Selected On", "Charity Name", "Charity Link", "Charity Blurb", "Note", "Preference"}

// exportDay, exportHour and exportMinute are when the weekly run goes:
// Thursday, at 23:59.
const (
	exportDay    = time.Thursday
	exportHour   = 23
	exportMinute = 59
)

// nextExport is the first Thursday 23:59 after t, school time.
func nextExport(t time.Time) time.Time {
	t = t.In(local)
	at := time.Date(t.Year(), t.Month(), t.Day(), exportHour, exportMinute, 0, 0, local)
	for at.Weekday() != exportDay || !at.After(t) {
		at = time.Date(at.Year(), at.Month(), at.Day()+1, exportHour, exportMinute, 0, 0, local)
	}
	return at
}

// weekIssue is the issue a run at t is for: the first newsletter date in the
// seven days after t's day, or "" when the week has none.
func weekIssue(model *Model, t time.Time) string {
	from := t.In(local).Format(DateFormat)
	to := t.In(local).AddDate(0, 0, 7).Format(DateFormat)
	for _, d := range model.NewsletterDates {
		if d > from && d <= to {
			return d
		}
	}
	return ""
}

// exportLoop runs the export every Thursday night for the week's issue. It
// keeps the wall clock rather than now, which tests pin to a day long past.
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
		n, err := a.exportIssue(ctx, issue, exportActor, exportActor)
		if err != nil {
			slog.ErrorContext(ctx, "birthday: weekly copy to the shared sheet", "issue", issue, "copied", n, "error", err)
			continue
		}
		slog.InfoContext(ctx, "birthday: weekly copy to the shared sheet", "issue", issue, "copied", n)
	}
}

// exported is one birthday on its way to the shared sheet: its row there,
// and what marking it done writes to its donation.
type exported struct {
	email, year string
	row         map[string]string
	donation    map[string]string
}

// toExport is every birthday an issue carries that is not yet done.
func (a app) toExport(model *Model, issue, actor string) []exported {
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
		donation := map[string]string{"Used On": day, "Used By": actor}
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

// exportIssue copies an issue's birthdays that are not yet done to the
// shared sheet, then marks each done, and says how many went. A row is
// marked only once it is on the shared sheet, so a failure part way leaves
// the rest to go on the next try, never a birthday marked and not copied.
// real is who is really acting, for the Change Log.
func (a app) exportIssue(ctx context.Context, issue, actor, real string) (int, error) {
	items := a.toExport(a.cache.Model(), issue, actor)
	var copied []exported
	var failed error
	for _, it := range items {
		if err := a.writer.AppendCells(sharedSheet, sharedNewsletterTab, it.row); err != nil {
			failed = fmt.Errorf("copy %s to the shared sheet: %w", it.email, err)
			break
		}
		copied = append(copied, it)
	}
	if len(copied) == 0 {
		return 0, failed
	}
	done := make(chan error, 1)
	a.queue.Add(func() {
		tables := a.cache.Tables()
		for _, it := range copied {
			tables = tables.with(donationsTab, map[string]string{"Email": it.email, "Year": it.year}, it.donation)
		}
		model, err := BuildModel(tables)
		if err != nil {
			done <- err
			return
		}
		a.cache.set(tables, model)
		done <- nil
		for _, it := range copied {
			if err := a.writer.Set(appName, donationsTab, map[string]string{"Email": it.email, "Year": it.year}, it.donation); err != nil {
				slog.ErrorContext(ctx, "birthday: mark copied donation used", "email", it.email, "error", err)
			}
			if err := a.writer.Append(appName, changeLogTab, []string{time.Now().Format(time.RFC3339), actor, "used", "donation", it.email, it.year, "copied to the shared sheet for " + issue, real}); err != nil {
				slog.ErrorContext(ctx, "birthday: log copied donation", "email", it.email, "error", err)
			}
		}
	})
	if err := <-done; err != nil {
		return 0, fmt.Errorf("mark the copied birthdays done: %w", err)
	}
	return len(copied), failed
}

// shareIssue is POST /api/birthday/newsletter/share: anyone on the team
// running the export by hand for an issue, the Newsletters page's Copy to
// Shared Sheet. It answers how many birthdays went.
func (a app) shareIssue(w http.ResponseWriter, r *http.Request) {
	actor, _, ok := a.requireTeam(w, r)
	if !ok {
		return
	}
	var body struct {
		Date string `json:"date"`
	}
	if !decode(w, r, &body) {
		return
	}
	issue := strings.TrimSpace(body.Date)
	if !slices.Contains(a.cache.Model().NewsletterDates, issue) {
		http.Error(w, "that is not a newsletter date", http.StatusBadRequest)
		return
	}
	n, err := a.exportIssue(r.Context(), issue, actor, auth.RealEmail(r))
	if err != nil {
		slog.ErrorContext(r.Context(), "birthday: copy to the shared sheet", "actor", actor, "issue", issue, "copied", n, "error", err)
		http.Error(w, fmt.Sprintf("copied %d, then: %v", n, err), http.StatusBadGateway)
		return
	}
	slog.InfoContext(r.Context(), "birthday: copied to the shared sheet", "actor", actor, "issue", issue, "copied", n)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"copied": n})
}
