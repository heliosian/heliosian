package loop

import (
	"context"
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

var groupTabs = []string{managersTab, rulesTab, additionsTab, excludedTab, aliasesTab, archivedTab, messagesTab, deliveriesTab}

func spec() store.Spec[*Model] {
	return store.Spec[*Model]{
		App: appName,
		Tabs: []store.Tab{
			{Name: groupsTab, Columns: GroupColumns, Key: []string{"Name"}, Cascade: carryGroup},
			{Name: managersTab, Columns: ManagerColumns, Key: []string{"Group", "Email"}},
			{Name: rulesTab, Columns: RuleColumns, Key: RuleColumns},
			{Name: additionsTab, Columns: AdditionColumns, Key: []string{"Group", "Email"}},
			{Name: excludedTab, Columns: ExcludedColumns, Key: []string{"Group", "Email"}},
			{Name: aliasesTab, Columns: AliasColumns, Key: []string{"Group", "Alias"}},
			{Name: messagesTab, Columns: MessageColumns, Key: []string{"ID", "Group"}},
			{Name: deliveriesTab, Columns: DeliveryColumns, Key: []string{"Timestamp", "Group", "Email", "Event"}, AppendOnly: true},
			{Name: adminsTab, Columns: AdminColumns, Key: []string{"Email"}},
			{Name: archivedTab, Columns: ArchivedColumns, Key: []string{"Group", "Email"}},
		},
		Build: func(_ context.Context, tables store.Tables) (*Model, error) {
			return BuildModel(tables)
		},
		Loaded: func(model *Model, took time.Duration) {
			rules := 0
			for _, g := range model.Groups {
				rules += len(g.Rules)
			}
			slog.Info("loaded groups model", "groups", len(model.Groups), "rules", rules, "messages", len(model.Messages), "deliveries", len(model.Deliveries), "took", took.Round(time.Millisecond))
		},
	}
}

func carryGroup(before, after store.Row) []store.Op {
	if before == nil || after != nil || before["Name"] == "" {
		return nil
	}
	ops := []store.Op{}
	for _, tab := range groupTabs {
		ops = append(ops, store.Delete(tab, store.Row{"Group": before["Name"]}))
	}
	return ops
}

func NewCache(source data.Source, writer data.Writer, superAdmin func(string) bool, queue *store.Queue) (*Cache, error) {
	s, err := store.New(spec(), source, writer, queue)
	if err != nil {
		return nil, err
	}
	return &Cache{Store: s, superAdmin: superAdmin}, nil
}

func (c *Cache) IsSuperAdmin(email string) bool {
	return c.superAdmin(strings.ToLower(strings.TrimSpace(email)))
}

func (c *Cache) IsAdmin(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	return slices.Contains(c.Model().admins, email) || c.superAdmin(email)
}

func (c *Cache) Admins(superAdmins []string) []string {
	admins := config.NormalizeEmails(append(slices.Clone(c.Model().admins), superAdmins...))
	sort.Strings(admins)
	return admins
}
