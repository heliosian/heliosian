package model

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/data"
	"heliosian/internal/id"
	"heliosian/internal/store"
)

type ActivitiesCache struct {
	*store.Store[*Activities]
	AdminList
}

func activitiesSpec(images blob.Checker) store.Spec[*Activities] {
	return store.Spec[*Activities]{
		App: activitiesAppName,
		Tabs: []store.Tab{
			{Name: activityCategoriesTab, Columns: ActivityCategoryColumns, Key: []string{"Category ID"}},
			{Name: activitiesTab, Columns: ActivityColumns, Key: []string{"Event ID"}, Cascade: carryActivity},
			{Name: volunteersTab, Columns: VolunteerColumns, Key: []string{"Event ID", "Email"}},
			{Name: linksTab, Columns: LinkColumns, Key: []string{"Link ID"}},
			{Name: activitySettingsTab, Columns: KeyValueColumns, Key: []string{"Key"}},
			{Name: notificationsTab, Columns: NotificationColumns, Key: []string{"Email"}},
			AdminsTab,
			{Name: activityRedirectsTab, Columns: RedirectColumns, Key: []string{"Old"}},
			{Name: id.AliasesTab, Columns: id.AliasColumns, Key: []string{id.AliasColumn}},
		},
		Build: func(ctx context.Context, tables store.Tables) (*Activities, error) {
			return BuildActivities(ctx, tables, images)
		},
		Loaded: func(m *Activities, took time.Duration) {
			children, volunteers := 0, 0
			for _, a := range m.Activities {
				children += len(a.Descendants())
				for _, n := range append([]*Activity{a}, a.Descendants()...) {
					volunteers += len(n.Volunteers)
				}
			}
			slog.Info("loaded events model", "categories", len(m.Categories), "roots", len(m.Activities),
				"children", children, "volunteers", volunteers, "skipped", m.Skipped, "took", took.Round(time.Millisecond))
		},
	}
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

func NewActivitiesCache(source data.Source, writer data.Writer, images blob.Checker, superAdmins func() []string, queue *store.Queue) (*ActivitiesCache, error) {
	s, err := store.New(activitiesSpec(images), source, writer, queue)
	if err != nil {
		return nil, err
	}
	return &ActivitiesCache{Store: s, AdminList: NewAdminList("team", ActivitiesAdminAllowances, superAdmins, func() []string { return s.Model().admins }, s.Commit)}, nil
}
