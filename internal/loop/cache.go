package loop

import (
	"context"
	"log/slog"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/id"
	"heliosian/internal/model"
	"heliosian/internal/store"
)

type Cache struct {
	*store.Store[*Model]
	model.AdminList
}

var groupTabs = []string{managersTab, rulesTab, additionsTab, excludedTab, archivedTab, messagesTab, deliveriesTab}

func spec(idKey []byte) store.Spec[*Model] {
	return store.Spec[*Model]{
		App: appName,
		Tabs: []store.Tab{
			{Name: groupsTab, Columns: GroupColumns, Key: []string{idColumn}, Cascade: carryGroup},
			{Name: managersTab, Columns: ManagerColumns, Key: []string{"Group", "Email"}},
			{Name: rulesTab, Columns: RuleColumns, Key: RuleColumns},
			{Name: additionsTab, Columns: AdditionColumns, Key: []string{"Group", "Email"}},
			{Name: excludedTab, Columns: ExcludedColumns, Key: []string{"Group", "Email"}},
			{Name: id.AliasesTab, Columns: id.AliasColumns, Key: []string{id.AliasColumn}},
			{Name: messagesTab, Columns: MessageColumns, Key: []string{"ID", "Group"}},
			{Name: deliveriesTab, Columns: DeliveryColumns, Key: []string{"Timestamp", "Group", "Email", "Event"}, AppendOnly: true},
			model.AdminsTab,
			{Name: archivedTab, Columns: ArchivedColumns, Key: []string{"Group", "Email"}},
		},
		Build: func(_ context.Context, tables store.Tables) (*Model, error) {
			return BuildModel(tables, idKey)
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

func carryGroup(_ store.Tables, before, after store.Row) []store.Op {
	if before == nil || after != nil || before[idColumn] == "" {
		return nil
	}
	ops := []store.Op{store.Delete(id.AliasesTab, store.Row{id.IDColumn: before[idColumn]})}
	for _, tab := range groupTabs {
		ops = append(ops, store.Delete(tab, store.Row{"Group": before[idColumn]}))
	}
	return ops
}

func NewCache(source data.Source, writer data.Writer, superAdmins func() []string, queue *store.Queue, idKey []byte) (*Cache, error) {
	s, err := store.New(spec(idKey), source, writer, queue)
	if err != nil {
		return nil, err
	}
	return &Cache{Store: s, AdminList: model.NewAdminList("loop", AdminAllowances, superAdmins, func() []string { return s.Model().admins }, s.Commit)}, nil
}
