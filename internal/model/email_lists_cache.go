package model

import (
	"heliosian/internal/id"
	"heliosian/internal/store"
)

var groupTabs = []string{managersTab, rulesTab, additionsTab, excludedTab, archivedTab, messagesTab, deliveriesTab}

var emailListsTabs = []store.Tab{
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
