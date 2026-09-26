package team

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

func spec(images ImageChecker) store.Spec[*Model] {
	return store.Spec[*Model]{
		App: appName,
		Tabs: []store.Tab{
			{Name: categoriesTab, Columns: CategoryColumns, Key: []string{"Category ID"}},
			{Name: activitiesTab, Columns: ActivityColumns, Key: []string{"Event ID"}, Cascade: carryActivity},
			{Name: volunteersTab, Columns: VolunteerColumns, Key: []string{"Event ID", "Email"}},
			{Name: linksTab, Columns: LinkColumns, Key: []string{"Event ID", "Title"}},
			{Name: settingsTab, Columns: SettingColumns, Key: []string{"Key"}},
			{Name: adminsTab, Columns: AdminColumns, Key: []string{"Email"}},
			{Name: redirectsTab, Columns: RedirectColumns, Key: []string{"Old"}},
		},
		Build: func(ctx context.Context, tables store.Tables) (*Model, error) {
			return BuildModel(ctx, tables, images)
		},
		Loaded: func(model *Model, took time.Duration) {
			children, volunteers := 0, 0
			for _, a := range model.Activities {
				children += len(a.Descendants())
				for _, n := range append([]*Activity{a}, a.Descendants()...) {
					volunteers += len(n.Volunteers)
				}
			}
			slog.Info("loaded events model", "categories", len(model.Categories), "roots", len(model.Activities),
				"children", children, "volunteers", volunteers, "skipped", model.Skipped, "took", took.Round(time.Millisecond))
		},
	}
}

func carryActivity(before, after store.Row) []store.Op {
	switch {
	case before == nil || before["Event ID"] == "":
		return nil
	case after == nil:
		return []store.Op{store.Delete(linksTab, store.Row{"Event ID": before["Event ID"]})}
	case before["Year"] != after["Year"]:
		return []store.Op{store.Update(activitiesTab, store.Row{"Parent": after["Event ID"]}, store.Row{"Year": after["Year"]})}
	}
	return nil
}

func NewCache(source data.Source, writer data.Writer, images ImageChecker, superAdmin func(string) bool, queue *store.Queue) (*Cache, error) {
	s, err := store.New(spec(images), source, writer, queue)
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
