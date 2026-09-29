package model

import (
	"context"
	"log/slog"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/store"
)

type BirthdaysCache struct {
	*store.Store[*Birthdays]
	AdminList
	shared *store.Store[int]
}

var sharedSpec = store.Spec[int]{
	App:  sharedSheet,
	Tabs: []store.Tab{{Name: sharedNewsletterTab, Columns: SharedNewsletterColumns, Key: []string{"Staff Email", "Target Newsletter Date"}, AppendOnly: true}},
	Build: func(_ context.Context, tables store.Tables) (int, error) {
		return len(tables[sharedNewsletterTab]), nil
	},
	Loaded: func(rows int, took time.Duration) {
		slog.Info("loaded shared newsletter", "rows", rows, "took", took.Round(time.Millisecond))
	},
}

func birthdaysSpec(idKey []byte) store.Spec[*Birthdays] {
	return store.Spec[*Birthdays]{
		App: birthdaysAppName,
		Tabs: []store.Tab{
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
		},
		Build: func(_ context.Context, tables store.Tables) (*Birthdays, error) {
			return BuildBirthdays(tables, idKey)
		},
		Loaded: func(m *Birthdays, took time.Duration) {
			slog.Info("loaded birthday model", "birthdays", len(m.Birthdays), "charities", len(m.Charities),
				"donations", len(m.Donations), "newsletters", len(m.NewsletterDates), "took", took.Round(time.Millisecond))
		},
	}
}

func NewBirthdaysCache(source data.Source, writer data.Writer, superAdmins func() []string, queue *store.Queue, idKey []byte) (*BirthdaysCache, error) {
	s, err := store.New(birthdaysSpec(idKey), source, writer, queue)
	if err != nil {
		return nil, err
	}
	shared, err := store.New(sharedSpec, source, writer, queue)
	if err != nil {
		return nil, err
	}
	return &BirthdaysCache{Store: s, AdminList: NewAdminList("birthday", BirthdaysAdminAllowances, superAdmins, func() []string { return s.Model().admins }, s.Commit), shared: shared}, nil
}
