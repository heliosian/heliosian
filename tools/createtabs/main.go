package main

import (
	"context"
	"fmt"
	"log"
	"slices"

	"heliosian/internal/admins"
	"heliosian/internal/artifacts"
	"heliosian/internal/birthday"
	"heliosian/internal/celebrate"
	"heliosian/internal/config"
	"heliosian/internal/data"
	"heliosian/internal/env"
	"heliosian/internal/feedback"
	"heliosian/internal/home"
	"heliosian/internal/loop"
	"heliosian/internal/spreadsheets"
	"heliosian/internal/store"
	"heliosian/internal/team"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

type tab struct {
	title  string
	header []string
}

func withChangeLog(tabs []store.Tab) []tab {
	out := []tab{}
	for _, t := range tabs {
		out = append(out, tab{t.Name, t.Columns})
	}
	return append(out, tab{store.ChangeLogTab, store.ChangeLogColumns})
}

var seeds = map[string]map[string]map[string]string{
	"Apps": {"Categories": {"Title": home.EventsTitle, "Emoji": home.EventsEmoji, "Style": home.StyleEvents}},
}

var layouts = map[string][]tab{
	"Artifacts": {
		{"Documents", artifacts.DocumentColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Feedback": {
		{"Reports", feedback.ReportColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Directory": withChangeLog(who.Tabs),
	"Invite List Builder": {
		{"_Greetings", who.GreetingColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Apps": {
		{"Categories", home.CategoryColumns},
		{"Links", home.LinkColumns},
		{admins.Tab, admins.Columns},
		{"Visibility", home.VisibilityColumns},
		{"Audience", home.AudienceColumns},
		{"Widgets", home.WidgetColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Events": {
		{"Categories", team.CategoryColumns},
		{"Activities", team.ActivityColumns},
		{"Volunteers", team.VolunteerColumns},
		{"Links", team.LinkColumns},
		{"Settings", team.SettingColumns},
		{"Notifications", team.NotificationColumns},
		{admins.Tab, admins.Columns},
		{"Redirects", team.RedirectColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Birthdays": {
		{"Birthdays", birthday.BirthdayColumns},
		{"Assignments", birthday.AssignmentColumns},
		{"Outreach", birthday.OutreachColumns},
		{"Donations", birthday.DonationColumns},
		{"Notes", birthday.NoteColumns},
		{"Charities", birthday.CharityColumns},
		{"Newsletter Dates", birthday.NewsletterDateColumns},
		{"Settings", birthday.SettingColumns},
		{admins.Tab, admins.Columns},
		{"Team", birthday.TeamColumns},
		{"Reminders", birthday.ReminderColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Staff Birthday List (Shared)": {
		{"Newsletter", birthday.SharedNewsletterColumns},
	},
	"Celebrate": {
		{"Celebrations", celebrate.CelebrationColumns},
		{"Categories", celebrate.CategoryColumns},
		{"Parties", celebrate.PartyColumns},
		{"Hosts", celebrate.HostColumns},
		{"Tickets", celebrate.TicketColumns},
		{"Settings", celebrate.SettingColumns},
		{admins.Tab, admins.Columns},
		{"Redirects", celebrate.RedirectColumns},
		{"INVOICING", celebrate.InvoicingColumns},
		{"Former Addresses", celebrate.FormerColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Calendar": {
		{when.GoogleTab, when.GoogleColumns},
		{when.PDFTab, when.PDFColumns},
		{when.EventsTab, when.EventColumns},
		{when.EnrichmentTab, when.EnrichmentColumns},
		{when.OverridesTab, when.OverrideColumns},
		{when.DayTypesTab, when.DayTypeColumns},
		{when.DayOverridesTab, when.DayOverrideColumns},
		{when.TagsTab, when.TagColumns},
		{admins.Tab, admins.Columns},
		{when.FeedsTab, when.FeedColumns},
		{when.SettingsTab, when.SettingColumns},
		{when.RSVPsTab, when.RSVPColumns},
		{when.InvitationsTab, when.InvitationColumns},
		{when.InvitesTab, when.InviteColumns},
		{when.InviteGroupsTab, when.InviteGroupColumns},
		{when.BouncesTab, when.BounceColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Config": withChangeLog(config.Tabs),
	"Groups": {
		{"Groups", loop.GroupColumns},
		{"Managers", loop.ManagerColumns},
		{"Rules", loop.RuleColumns},
		{"Additions", loop.AdditionColumns},
		{"Excluded", loop.ExcludedColumns},
		{"Aliases", loop.AliasColumns},
		{"Messages", loop.MessageColumns},
		{"Deliveries", loop.DeliveryColumns},
		{admins.Tab, admins.Columns},
		{"Archived", loop.ArchivedColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
}

func applyLayout(source *data.Sheet, layout string) (tabs, columns int, err error) {
	title, sizes, err := source.Layout(layout)
	if err != nil {
		return 0, 0, err
	}
	if title != layout {
		return 0, 0, fmt.Errorf("spreadsheet is titled %q, want %q", title, layout)
	}
	existing := []string{}
	for _, size := range sizes {
		existing = append(existing, size.Title)
	}
	present := []string{}
	for _, t := range layouts[layout] {
		if slices.Contains(existing, t.title) {
			present = append(present, t.title)
		}
	}
	headers, err := source.Tabs(context.Background(), layout, nil, present)
	if err != nil {
		return 0, 0, err
	}
	for _, t := range layouts[layout] {
		if slices.Contains(present, t.title) {
			missing := []string{}
			for _, name := range t.header {
				if !slices.Contains(headers[t.title].Header, name) {
					missing = append(missing, name)
				}
			}
			if len(missing) == 0 {
				continue
			}
			if err := source.AddColumns(layout, t.title, missing); err != nil {
				return 0, 0, err
			}
			log.Printf("added %d columns to %q: %v", len(missing), t.title, missing)
			columns += len(missing)
			continue
		}
		if err := source.AddTab(layout, t.title, t.header); err != nil {
			return 0, 0, err
		}
		if seed := seeds[layout][t.title]; seed != nil {
			if err := source.Insert(layout, t.title, []map[string]string{seed}); err != nil {
				return 0, 0, err
			}
		}
		log.Printf("created tab %q in %q with %d columns", t.title, layout, len(t.header))
		tabs++
	}
	return tabs, columns, nil
}

func main() {
	sheets := []spreadsheets.Spreadsheet{}
	for _, s := range spreadsheets.All {
		if _, ok := layouts[s.Title]; ok {
			sheets = append(sheets, s)
		}
	}
	ids := map[string]string{}
	for _, s := range sheets {
		ids[s.Title] = env.Required(s.Env)
	}
	source, err := data.NewSheet(ids)
	if err != nil {
		log.Fatalf("sheet source: %v", err)
	}
	tabs, columns := 0, 0
	for _, s := range sheets {
		created, added, err := applyLayout(source, s.Title)
		if err != nil {
			log.Fatalf("%s: %v", s.Env, err)
		}
		tabs += created
		columns += added
	}
	log.Printf("checked %d spreadsheets: %d tabs created, %d columns added", len(sheets), tabs, columns)
}
