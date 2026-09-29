package model

import (
	"context"
	"log/slog"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/id"
	"heliosian/internal/store"
)

type EmailListsCache struct {
	*store.Store[*EmailLists]
	AdminList
}

var groupTabs = []string{managersTab, rulesTab, additionsTab, excludedTab, archivedTab, messagesTab, deliveriesTab}

func emailListsSpec(idKey []byte) store.Spec[*EmailLists] {
	return store.Spec[*EmailLists]{
		App: emailListsAppName,
		Tabs: []store.Tab{
			{Name: groupsTab, Columns: GroupColumns, Key: []string{idColumn}, Cascade: carryGroup},
			{Name: managersTab, Columns: ManagerColumns, Key: []string{"Group", "Email"}},
			{Name: rulesTab, Columns: ListRuleColumns, Key: ListRuleColumns},
			{Name: additionsTab, Columns: AdditionColumns, Key: []string{"Group", "Email"}},
			{Name: excludedTab, Columns: ExcludedColumns, Key: []string{"Group", "Email"}},
			{Name: id.AliasesTab, Columns: id.AliasColumns, Key: []string{id.AliasColumn}},
			{Name: messagesTab, Columns: ListMessageColumns, Key: []string{"ID", "Group"}},
			{Name: deliveriesTab, Columns: DeliveryColumns, Key: []string{"Timestamp", "Group", "Email", "Event"}, AppendOnly: true},
			AdminsTab,
			{Name: archivedTab, Columns: ArchivedColumns, Key: []string{"Group", "Email"}},
		},
		Build: func(_ context.Context, tables store.Tables) (*EmailLists, error) {
			return BuildEmailLists(tables, idKey)
		},
		Loaded: func(m *EmailLists, took time.Duration) {
			rules := 0
			for _, g := range m.Groups {
				rules += len(g.Rules)
			}
			slog.Info("loaded groups model", "groups", len(m.Groups), "rules", rules, "messages", len(m.Messages), "deliveries", len(m.Deliveries), "took", took.Round(time.Millisecond))
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

func NewEmailListsCache(source data.Source, writer data.Writer, superAdmins func() []string, queue *store.Queue, idKey []byte) (*EmailListsCache, error) {
	s, err := store.New(emailListsSpec(idKey), source, writer, queue)
	if err != nil {
		return nil, err
	}
	return &EmailListsCache{Store: s, AdminList: NewAdminList("loop", EmailListsAdminAllowances, superAdmins, func() []string { return s.Model().admins }, s.Commit)}, nil
}
