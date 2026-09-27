package birthday

import (
	"context"
	"log/slog"
	"time"

	"heliosian/internal/admins"
	"heliosian/internal/data"
	"heliosian/internal/store"
)

type Cache struct {
	*store.Store[*Model]
	admins.List
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

var spec = store.Spec[*Model]{
	App: appName,
	Tabs: []store.Tab{
		{Name: birthdaysTab, Columns: BirthdayColumns, Key: []string{"Email"}},
		{Name: assignmentsTab, Columns: AssignmentColumns, Key: []string{"Email", "Year"}},
		{Name: outreachTab, Columns: OutreachColumns, Key: []string{"Email", "Year"}},
		{Name: donationsTab, Columns: DonationColumns, Key: []string{"Email", "Year"}},
		{Name: notesTab, Columns: NoteColumns, Key: []string{"Email", "Added By", "Added"}},
		{Name: charitiesTab, Columns: CharityColumns, Key: []string{"Name"}, Cascade: renameCharity},
		{Name: newsletterDatesTab, Columns: NewsletterDateColumns, Key: []string{"Date"}, Cascade: moveNewsletterDate},
		{Name: settingsTab, Columns: SettingColumns, Key: []string{"Key"}},
		admins.Spec,
		{Name: teamTab, Columns: TeamColumns, Key: []string{"Email", "Role"}},
		{Name: remindersTab, Columns: ReminderColumns, Key: []string{"Email", "Year", "Kind"}},
	},
	Build: func(_ context.Context, tables store.Tables) (*Model, error) {
		return BuildModel(tables)
	},
	Loaded: func(model *Model, took time.Duration) {
		slog.Info("loaded birthday model", "birthdays", len(model.Birthdays), "charities", len(model.Charities),
			"donations", len(model.Donations), "newsletters", len(model.NewsletterDates), "took", took.Round(time.Millisecond))
	},
}

func renameCharity(_ store.Tables, before, after store.Row) []store.Op {
	if before == nil || after == nil || before["Name"] == after["Name"] {
		return nil
	}
	return []store.Op{
		store.Update(donationsTab, store.Row{"Charity": before["Name"]}, store.Row{"Charity": after["Name"]}),
		store.Update(settingsTab, store.Row{"Key": DefaultCharityKey, "Value": before["Name"]}, store.Row{"Value": after["Name"]}),
	}
}

func moveNewsletterDate(_ store.Tables, before, after store.Row) []store.Op {
	if before == nil || after == nil || before["Date"] == after["Date"] {
		return nil
	}
	return []store.Op{store.Update(birthdaysTab, store.Row{"Newsletter Override": before["Date"]}, store.Row{"Newsletter Override": after["Date"]})}
}

func NewCache(source data.Source, writer data.Writer, superAdmins func() []string, queue *store.Queue) (*Cache, error) {
	s, err := store.New(spec, source, writer, queue)
	if err != nil {
		return nil, err
	}
	shared, err := store.New(sharedSpec, source, writer, queue)
	if err != nil {
		return nil, err
	}
	return &Cache{Store: s, List: admins.New(superAdmins, func() []string { return s.Model().admins }, s.Commit), shared: shared}, nil
}
