package main

import (
	"context"
	"fmt"
	"log"
	"slices"

	"heliosian/internal/data"
	"heliosian/internal/db"
	"heliosian/internal/env"
	"heliosian/internal/id"
	"heliosian/internal/model"
	"heliosian/internal/spreadsheets"
	"heliosian/internal/store"
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
	"Apps": {"Categories": {"Category ID": model.EventsCategoryID, "Title": model.EventsCategoryTitle, "Emoji": model.EventsCategoryEmoji, "Style": model.StyleEvents, store.OrderColumn: "i"}},
}

var layouts = map[string][]tab{
	"Artifacts": {
		{"Documents", model.DocumentColumns},
		{"To Dos", model.ToDoColumns},
		{"Reads", model.ReadColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Feedback": {
		{"Reports", model.ReportColumns},
		{id.AliasesTab, id.AliasColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Directory": withChangeLog(model.DirectoryTabs),
	"Invite List Builder": {
		{"Services", model.ServiceColumns},
		{"Templates", model.TemplateColumns},
		{"Greetings", model.GreetingColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Apps": {
		{"Categories", model.HomeCategoryColumns},
		{"Links", model.HomeLinkColumns},
		{model.AdminsTab.Name, model.AdminsTab.Columns},
		{"Visibility", model.HomeVisibilityColumns},
		{"Audience", model.HomeAudienceColumns},
		{"Widgets", model.HomeWidgetColumns},
		{"Layout", model.HomeLayoutColumns},
		{"To Dos", model.HomeToDoColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Events": {
		{"Categories", model.ActivityCategoryColumns},
		{"Activities", model.ActivityColumns},
		{"Volunteers", model.VolunteerColumns},
		{"Links", model.LinkColumns},
		{"Settings", model.KeyValueColumns},
		{"Notifications", model.NotificationColumns},
		{model.AdminsTab.Name, model.AdminsTab.Columns},
		{"Redirects", model.RedirectColumns},
		{id.AliasesTab, id.AliasColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Birthdays": {
		{"Birthdays", model.BirthdayColumns},
		{"Assignments", model.AssignmentColumns},
		{"Outreach", model.OutreachColumns},
		{"Donations", model.DonationColumns},
		{"Notes", model.NoteColumns},
		{"Charities", model.CharityColumns},
		{"Newsletter Dates", model.NewsletterDateColumns},
		{"Settings", model.KeyValueColumns},
		{model.AdminsTab.Name, model.AdminsTab.Columns},
		{"Team", model.TeamColumns},
		{"Reminders", model.ReminderColumns},
		{"Invites", model.BirthdayInviteColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Staff Birthday List (Shared)": {
		{"Newsletter", model.SharedNewsletterColumns},
	},
	"Celebrate": {
		{"Celebrations", model.CelebrationColumns},
		{"Categories", model.PartyCategoryColumns},
		{"Parties", model.PartyColumns},
		{"Hosts", model.HostColumns},
		{"Tickets", model.TicketColumns},
		{"Settings", model.KeyValueColumns},
		{model.AdminsTab.Name, model.AdminsTab.Columns},
		{"Redirects", model.RedirectColumns},
		{"INVOICING", model.InvoicingColumns},
		{"Former Addresses", model.FormerColumns},
		{id.AliasesTab, id.AliasColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Calendar": {
		{model.GoogleTab, model.GoogleColumns},
		{model.PDFTab, model.PDFColumns},
		{model.EventsTab, model.EventColumns},
		{model.EnrichmentTab, model.EnrichmentColumns},
		{model.OverridesTab, model.OverrideColumns},
		{model.DayTypesTab, model.DayTypeColumns},
		{model.DayOverridesTab, model.DayOverrideColumns},
		{model.TagsTab, model.TagColumns},
		{model.AdminsTab.Name, model.AdminsTab.Columns},
		{model.FeedsTab, model.FeedColumns},
		{model.SettingsTab, model.SettingColumns},
		{model.RSVPsTab, model.RSVPColumns},
		{model.InvitationsTab, model.InvitationColumns},
		{model.InvitesTab, model.InviteColumns},
		{model.InviteGroupsTab, model.InviteGroupColumns},
		{model.BouncesTab, model.BounceColumns},
		{model.MessagesTab, model.MessageColumns},
		{id.AliasesTab, id.AliasColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Config": withChangeLog(model.ConfigTabs),
	"Groups": {
		{"Groups", model.GroupColumns},
		{"Managers", model.ManagerColumns},
		{"Rules", model.ListRuleColumns},
		{"Additions", model.AdditionColumns},
		{"Excluded", model.ExcludedColumns},
		{id.AliasesTab, id.AliasColumns},
		{"Messages", model.ListMessageColumns},
		{"Deliveries", model.DeliveryColumns},
		{model.AdminsTab.Name, model.AdminsTab.Columns},
		{"Archived", model.ArchivedColumns},
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

func addDataLayouts() {
	for _, s := range spreadsheets.All {
		tabs := []tab{}
		for _, t := range db.Tables {
			if t.Sheet == s.Source {
				tabs = append(tabs, tab{t.Name, t.Stored()})
			}
		}
		if len(tabs) > 0 {
			layouts[s.Title] = append(tabs, tab{store.ChangeLogTab, store.ChangeLogColumns})
		}
	}
}

func main() {
	addDataLayouts()
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
