package main

import (
	"fmt"
	"log"
	"os"
	"slices"

	"heliosian/internal/artifacts"
	"heliosian/internal/birthday"
	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
	"heliosian/internal/data"
	"heliosian/internal/feedback"
	"heliosian/internal/home"
	"heliosian/internal/loop"
	"heliosian/internal/store"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

var spreadsheets = []struct{ env, layout string }{
	{"DIRECTORY_SHEET", "Directory"},
	{"INVITES_SHEET", "Invite List Builder"},
	{"APPS_SHEET", "Apps"},
	{"EVENTS_SHEET", "Events"},
	{"BIRTHDAY_SHEET", "Birthdays"},
	{"BIRTHDAY_SHARED_SHEET", "Staff Birthday List (Shared)"},
	{"CALENDAR_SHEET", "Calendar"},
	{"CELEBRATE_SHEET", "Celebrate"},
	{"CONFIG_SHEET", "Config"},
	{"GROUPS_SHEET", "Groups"},
	{"ARTIFACTS_SHEET", "Artifacts"},
	{"FEEDBACK_SHEET", "Feedback"},
}

type tab struct {
	title  string
	header []string
}

var seeds = map[string]map[string]map[string]string{
	"Apps": {"Categories": {"Title": home.EventsTitle, "Emoji": home.EventsEmoji, "Style": home.StyleEvents}},
}

var layouts = map[string][]tab{
	"Artifacts": {
		{"Documents", artifacts.DocumentColumns},
	},
	"Feedback": {
		{"Reports", feedback.ReportColumns},
	},
	"Directory": {
		{"Veracross Staff Import", []string{
			"entry_sort_name", "person_full_name", "person_job_title", "person_room",
			"person_classifications", "person_biography",
			"person_email", "person_email_2", "person_phone_business", "person_photo",
			"person_department",
		}},
		{"Veracross Student Import", []string{
			"entry_sort_name", "student_full_name", "student_classifications", "student_email",
			"student_phone_mobile",
			"household_1_phone", "household_1_address",
			"household_1_person_1_full_name", "household_1_person_1_email",
			"household_1_person_1_email_2", "household_1_person_1_phone_mobile",
			"household_1_person_1_phone_business",
			"household_1_person_2_full_name", "household_1_person_2_email",
			"household_1_person_2_email_2", "household_1_person_2_phone_mobile",
			"household_1_person_2_phone_business",
			"household_2_phone", "household_2_address",
			"household_2_person_1_full_name", "household_2_person_1_email",
			"household_2_person_1_email_2", "household_2_person_1_phone_mobile",
			"household_2_person_1_phone_business",
			"household_2_person_2_full_name", "household_2_person_2_email",
			"household_2_person_2_email_2", "household_2_person_2_phone_mobile",
			"household_2_person_2_phone_business",
			"student_photo",
		}},
		{"Name to Email", []string{"Name", "Email"}},
		{"Email Aliases", []string{"Alias", "Email"}},
		{"Overrides", []string{
			"Email", "Added",
			"Full Name", "Legal Name", "Preferred Name",
			"Is Student", "Is Parent", "Is Staff",
			"New to Helios", "Pronouns", "Facts",
			"Grade", "Classroom", "Crew",
			"Phone", "Job Title", "Department", "Grade Band", "Room Parent",
			"Opted Out", "Photo Updated", "Facts Updated",
			"Veracross Photo", "Primary Photo", "Pronunciation",
		}},
		{"Families", []string{
			"Email", "Address", "Family Phone", "Family Photo Caption",
			"Family Photo Updated", "Family Photo", "Family Photo Crop", "Family Pronunciation",
		}},
		{store.ChangeLogTab, store.ChangeLogColumns},
		{"Website Staff Import", []string{
			"constituent_id", "full_name", "title", "departments", "email", "bio", "photo",
		}},
		{"Tags", []string{"Owner Email", "Tag", "Person Email"}},
		{"Tag Managers", []string{"Owner Email", "Tag", "Manager Email"}},
		{"Photos", []string{"Email", "Photo Name", "Crop Name", store.OrderColumn}},
		{"Images", []string{"Kind", "Name", "Image"}},
		{"Admins", []string{"Email"}},
		{"Geocode", []string{"Address", "Lat", "Lng"}},
	},
	"Invite List Builder": {
		{"_Greetings", who.GreetingColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Apps": {
		{"Categories", home.CategoryColumns},
		{"Links", home.LinkColumns},
		{"Admins", home.AdminColumns},
		{"Visibility", home.VisibilityColumns},
		{"Audience", home.AudienceColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Events": {
		{"Categories", team.CategoryColumns},
		{"Activities", team.ActivityColumns},
		{"Volunteers", team.VolunteerColumns},
		{"Links", team.LinkColumns},
		{"Settings", team.SettingColumns},
		{"Admins", team.AdminColumns},
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
		{"Admins", birthday.AdminColumns},
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
		{"Admins", celebrate.AdminColumns},
		{"Redirects", celebrate.RedirectColumns},
		{"INVOICING", celebrate.InvoicingColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Calendar": {
		{calendar.GoogleTab, calendar.GoogleColumns},
		{calendar.PDFTab, calendar.PDFColumns},
		{calendar.EventsTab, calendar.EventColumns},
		{calendar.EnrichmentTab, calendar.EnrichmentColumns},
		{calendar.OverridesTab, calendar.OverrideColumns},
		{calendar.DayTypesTab, calendar.DayTypeColumns},
		{calendar.DayOverridesTab, calendar.DayOverrideColumns},
		{calendar.TagsTab, calendar.TagColumns},
		{calendar.AdminsTab, calendar.AdminColumns},
		{calendar.FeedsTab, calendar.FeedColumns},
		{calendar.SettingsTab, calendar.SettingColumns},
		{calendar.RSVPsTab, calendar.RSVPColumns},
		{calendar.InvitationsTab, calendar.InvitationColumns},
		{calendar.InvitesTab, calendar.InviteColumns},
		{calendar.InviteGroupsTab, calendar.InviteGroupColumns},
		{calendar.BouncesTab, calendar.BounceColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Config": {
		{"Settings", []string{"Key", "Value"}},
		{"Super Admins", []string{"Email"}},
		{"Grade Colors", []string{"Grade", "Color"}},
		{"Classroom Colors", []string{"Classroom", "Color"}},
		{"Signed Out", []string{"Email", "Time"}},
	},
	"Groups": {
		{"Groups", loop.GroupColumns},
		{"Managers", loop.ManagerColumns},
		{"Rules", loop.RuleColumns},
		{"Additions", loop.AdditionColumns},
		{"Excluded", loop.ExcludedColumns},
		{"Aliases", loop.AliasColumns},
		{"Messages", loop.MessageColumns},
		{"Deliveries", loop.DeliveryColumns},
		{"Admins", loop.AdminColumns},
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
	headers, err := source.Tabs(layout, nil, present)
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
	ids := map[string]string{}
	for _, s := range spreadsheets {
		id := os.Getenv(s.env)
		if id == "" {
			log.Fatalf("[ERROR] %s is required", s.env)
		}
		ids[s.layout] = id
	}
	source, err := data.NewSheet(ids)
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	tabs, columns := 0, 0
	for _, s := range spreadsheets {
		created, added, err := applyLayout(source, s.layout)
		if err != nil {
			log.Fatalf("[ERROR] %s: %v", s.env, err)
		}
		tabs += created
		columns += added
	}
	log.Printf("checked %d spreadsheets: %d tabs created, %d columns added", len(spreadsheets), tabs, columns)
}
