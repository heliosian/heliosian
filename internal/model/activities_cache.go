package model

import (
	"strings"

	"heliosian/internal/cells"
	"heliosian/internal/id"
	"heliosian/internal/store"
)

var activitiesTabs = []store.Tab{
	{Name: activityCategoriesTab, Columns: ActivityCategoryColumns, Key: []string{"Category ID"}},
	{Name: activitiesTab, Columns: ActivityColumns, Key: []string{"Event ID"}, Cascade: carryActivity},
	{Name: volunteersTab, Columns: VolunteerColumns, Key: []string{"Event ID", "Email"}},
	{Name: linksTab, Columns: LinkColumns, Key: []string{"Link ID"}},
	{Name: activitySettingsTab, Columns: KeyValueColumns, Key: []string{"Key"}},
	{Name: notificationsTab, Columns: NotificationColumns, Key: []string{"Email"}},
	AdminsTab,
	{Name: activityRedirectsTab, Columns: RedirectColumns, Key: []string{"Old"}},
	{Name: id.AliasesTab, Columns: id.AliasColumns, Key: []string{id.AliasColumn}},
}

func carryActivity(tables store.Tables, before, after store.Row) []store.Op {
	switch {
	case before == nil || before["Event ID"] == "":
		return nil
	case after == nil:
		return []store.Op{store.Delete(linksTab, store.Row{"Event ID": before["Event ID"]})}
	}
	ops := []store.Op{}
	if before["Year"] != after["Year"] {
		ops = append(ops, store.Update(activitiesTab, store.Row{"Parent": after["Event ID"]}, store.Row{"Year": after["Year"]}))
	}
	rows := tables[activitiesTab]
	was, now := rowPath(rows, before), rowPath(rows, after)
	if was == now || heldBy(rows, was, after) {
		return ops
	}
	return append(ops, store.Insert(activityRedirectsTab, store.Row{"Type": RedirectActivity, "Old": was, "New": now, "Date": todayLocal()}))
}

func rowPath(rows []store.Row, row store.Row) string {
	return activityPath(strings.TrimSpace(row["Event ID"]), strings.TrimSpace(row["Parent"]), cells.NormalizePretty(row["Pretty ID"]), func(parent string) string {
		return rowPath(rows, activityRow(rows, parent))
	})
}

func activityRow(rows []store.Row, id string) store.Row {
	for _, row := range rows {
		if strings.TrimSpace(row["Event ID"]) == id {
			return row
		}
	}
	return nil
}

func heldBy(rows []store.Row, path string, self store.Row) bool {
	for _, row := range rows {
		id := strings.TrimSpace(row["Event ID"])
		if id != "" && id != strings.TrimSpace(self["Event ID"]) && rowPath(rows, row) == path {
			return true
		}
	}
	return false
}
