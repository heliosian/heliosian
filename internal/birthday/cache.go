package birthday

import (
	"log/slog"
	"slices"
	"sort"
	"strings"
	"time"

	"heliosian/internal/config"
	"heliosian/internal/data"
	"heliosian/internal/store"
)

type Cache struct {
	*store.Store[*Model]
	superAdmin func(email string) bool
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
		{Name: adminsTab, Columns: AdminColumns, Key: []string{"Email"}},
		{Name: teamTab, Columns: TeamColumns, Key: []string{"Email", "Role"}},
		{Name: remindersTab, Columns: ReminderColumns, Key: []string{"Email", "Year", "Kind"}},
	},
	Build: BuildModel,
	Loaded: func(model *Model, took time.Duration) {
		slog.Info("loaded birthday model", "birthdays", len(model.Birthdays), "charities", len(model.Charities),
			"donations", len(model.Donations), "newsletters", len(model.NewsletterDates), "took", took.Round(time.Millisecond))
	},
}

func renameCharity(before, after store.Row) []store.Op {
	if before == nil || after == nil || before["Name"] == after["Name"] {
		return nil
	}
	return []store.Op{
		store.Update(donationsTab, store.Row{"Charity": before["Name"]}, store.Row{"Charity": after["Name"]}),
		store.Update(settingsTab, store.Row{"Key": DefaultCharityKey, "Value": before["Name"]}, store.Row{"Value": after["Name"]}),
	}
}

func moveNewsletterDate(before, after store.Row) []store.Op {
	if before == nil || after == nil || before["Date"] == after["Date"] {
		return nil
	}
	return []store.Op{store.Update(birthdaysTab, store.Row{"Newsletter Override": before["Date"]}, store.Row{"Newsletter Override": after["Date"]})}
}

func NewCache(source data.Source, writer data.Writer, superAdmin func(string) bool, queue store.Enqueuer) (*Cache, error) {
	s, err := store.New(spec, source, writer, queue)
	if err != nil {
		return nil, err
	}
	return &Cache{Store: s, superAdmin: superAdmin}, nil
}

// IsSuperAdmin reports whether email is one of the platform's super admins
// (docs/config.md) - the tier that colours the app in Appearance.
func (c *Cache) IsSuperAdmin(email string) bool {
	return c.superAdmin(strings.ToLower(strings.TrimSpace(email)))
}

// IsAdmin reports whether email runs the app: a row in the Admins tab, or a
// platform super admin.
func (c *Cache) IsAdmin(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	return slices.Contains(c.Model().Admins, email) || c.superAdmin(email)
}

// Admins is every admin as the admin page lists them: the tab plus the super
// admins, indistinguishable, sorted together.
func (c *Cache) Admins(superAdmins []string) []string {
	admins := config.NormalizeEmails(append(slices.Clone(c.Model().Admins), superAdmins...))
	sort.Strings(admins)
	return admins
}
