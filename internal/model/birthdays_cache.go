package model

import "heliosian/internal/store"

var sharedTabs = []store.Tab{{Name: sharedNewsletterTab, Columns: SharedNewsletterColumns, Key: []string{"Staff Email", "Target Newsletter Date"}, AppendOnly: true}}

var birthdaysTabs = []store.Tab{
	{Name: birthdaysTab, Columns: BirthdayColumns, Key: []string{"Email"}},
	{Name: assignmentsTab, Columns: AssignmentColumns, Key: []string{"Email", "Year"}},
	{Name: outreachTab, Columns: OutreachColumns, Key: []string{"Email", "Year"}},
	{Name: donationsTab, Columns: DonationColumns, Key: []string{"Email", "Year"}},
	{Name: birthdayNotesTab, Columns: NoteColumns, Key: []string{"Email", "Added By", "Added"}},
	{Name: charitiesTab, Columns: CharityColumns, Key: []string{"Charity ID"}},
	{Name: newsletterDatesTab, Columns: NewsletterDateColumns, Key: []string{"Newsletter Date ID"}},
	{Name: birthdaySettingsTab, Columns: KeyValueColumns, Key: []string{"Key"}},
	AdminsTab,
	{Name: teamTab, Columns: TeamColumns, Key: []string{"Email", "Role"}},
	{Name: remindersTab, Columns: ReminderColumns, Key: []string{"Email", "Year", "Kind"}},
	{Name: birthdayInvitesTab, Columns: BirthdayInviteColumns, Key: []string{"Invite ID"}},
}
