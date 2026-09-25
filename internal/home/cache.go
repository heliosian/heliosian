package home

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/filter"
	"heliosian/internal/store"
)

type Cache struct {
	*store.Store[*Model]
	superAdmins func() []string
	directory   Directory
}

func spec(images ImageChecker) store.Spec[*Model] {
	return store.Spec[*Model]{
		App: appName,
		Tabs: []store.Tab{
			{Name: categoriesTab, Columns: categoryColumns, Key: []string{"Title"}, Cascade: carryCategory},
			{Name: linksTab, Columns: linkColumns, Key: []string{"Title"}, Cascade: carryLink},
			{Name: adminsTab, Columns: adminColumns, Key: []string{"Email"}},
			{Name: visibilityTab, Columns: visibilityColumns, Key: []string{"App"}},
			{Name: audienceTab, Columns: AudienceColumns, Key: AudienceColumns},
		},
		Build: func(tables store.Tables) (*Model, error) {
			return BuildModel(tables, images)
		},
		Loaded: func(model *Model, took time.Duration) {
			links := 0
			for _, category := range model.Categories {
				links += len(category.Links)
			}
			slog.Info("loaded apps model", "categories", len(model.Categories), "links", links, "took", took.Round(time.Millisecond))
		},
	}
}

func carryAudience(thing string, before, after store.Row) []store.Op {
	switch {
	case before == nil:
		return nil
	case after == nil:
		return []store.Op{store.Delete(audienceTab, store.Row{"Thing": thing + before["Title"]})}
	case before["Title"] != after["Title"]:
		return []store.Op{store.Update(audienceTab, store.Row{"Thing": thing + before["Title"]}, store.Row{"Thing": thing + after["Title"]})}
	}
	return nil
}

func carryLink(before, after store.Row) []store.Op {
	return carryAudience(thingLink, before, after)
}

func carryCategory(before, after store.Row) []store.Op {
	ops := carryAudience(thingCategory, before, after)
	if before != nil && after != nil && before["Title"] != after["Title"] {
		ops = append(ops, store.Update(linksTab, store.Row{"Category": before["Title"]}, store.Row{"Category": after["Title"]}))
	}
	return ops
}

func NewCache(source data.Source, writer data.Writer, images ImageChecker, superAdmins func() []string, queue store.Enqueuer) (*Cache, error) {
	s, err := store.New(spec(images), source, writer, queue)
	if err != nil {
		return nil, err
	}
	return &Cache{Store: s, superAdmins: superAdmins}, nil
}

func (c *Cache) includes(rules []filter.Rule, email string) bool {
	return c.directory != nil && len(rules) > 0 && filter.OnList(filter.List{Rules: rules, Editors: c.Admins()}, c.directory.Sources(), email)
}

func normalizeEmails(emails []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, e := range emails {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" || !strings.Contains(e, "@") || seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	return out
}

type Person struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type AppVisibility struct {
	App
	Visibility string        `json:"visibility"`
	Emails     []string      `json:"emails"`
	Rules      []filter.Rule `json:"rules"`
}

func visibilityOf(model *Model, app App) Visibility {
	v, ok := model.Visibility[app.Key]
	if !ok {
		v = Visibility{Mode: VisibleToList}
	}
	if v.Emails == nil {
		v.Emails = []string{}
	}
	if v.Tagline == "" {
		v.Tagline = app.Tagline
	}
	if v.Name == "" {
		v.Name = app.Name
	}
	return v
}

func orderedApps(model *Model) []App {
	out := slices.Clone(Apps)
	place := func(app App) int {
		if v, ok := model.Visibility[app.Key]; ok && v.Order > 0 {
			return v.Order
		}
		return len(Apps) + 1 + slices.Index(Apps, app)
	}
	slices.SortStableFunc(out, func(a, b App) int { return place(a) - place(b) })
	return out
}

func (c *Cache) AppVisibilities() []AppVisibility {
	model := c.Model()
	out := make([]AppVisibility, 0, len(Apps))
	for _, app := range orderedApps(model) {
		v := visibilityOf(model, app)
		app.Name, app.Tagline, app.Mark = v.Name, v.Tagline, markVersion(app.Key)
		out = append(out, AppVisibility{App: app, Visibility: v.Mode, Emails: v.Emails, Rules: v.Rules})
	}
	return out
}

func (c *Cache) AppList() []App {
	model := c.Model()
	out := make([]App, 0, len(Apps))
	for _, app := range orderedApps(model) {
		v := visibilityOf(model, app)
		app.Name, app.Tagline, app.Mark = v.Name, v.Tagline, markVersion(app.Key)
		out = append(out, app)
	}
	return out
}

func (c *Cache) HiddenApps(email string) []string {
	email = strings.ToLower(strings.TrimSpace(email))
	hidden := []string{}
	model := c.Model()
	for _, app := range Apps {
		v := visibilityOf(model, app)
		if v.Mode == VisibleToList && !slices.Contains(v.Emails, email) && !c.includes(v.Rules, email) {
			hidden = append(hidden, app.Key)
		}
	}
	return hidden
}

func (c *Cache) MissingVisibility() []App {
	model := c.Model()
	out := []App{}
	for _, app := range Apps {
		if _, ok := model.Visibility[app.Key]; !ok {
			out = append(out, app)
		}
	}
	return out
}

func (c *Cache) IsSuperAdmin(email string) bool {
	return slices.Contains(normalizeEmails(c.superAdmins()), strings.ToLower(strings.TrimSpace(email)))
}

func (c *Cache) IsAdmin(email string) bool {
	return slices.Contains(c.Admins(), strings.ToLower(strings.TrimSpace(email)))
}

func (c *Cache) Admins() []string {
	admins := normalizeEmails(append(slices.Clone(c.Model().admins), c.superAdmins()...))
	sort.Strings(admins)
	return admins
}

func Grant(ctx context.Context, cache *Cache, appKey, email string) error {
	app, ok := appByKey(appKey)
	if !ok {
		return fmt.Errorf("no app %q", appKey)
	}
	email = strings.ToLower(strings.TrimSpace(email))
	v := visibilityOf(cache.Model(), app)
	if slices.Contains(v.Emails, email) {
		return nil
	}
	return cache.Commit(ctx, email, store.Set(visibilityTab, store.Row{"App": app.Key}, store.Row{"Visibility": v.Mode, "Emails": joinEmails(normalizeEmails(append(v.Emails, email)))}))
}
