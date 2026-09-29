package model

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/store"
)

type HomeCache struct {
	*store.Store[*Home]
	AdminList
	directory  *DirectoryCache
	parties    *PartiesCache
	activities *ActivitiesCache
}

func homeSpec(images blob.Checker) store.Spec[*Home] {
	return store.Spec[*Home]{
		App: homeAppName,
		Tabs: []store.Tab{
			{Name: homeCategoriesTab, Columns: HomeCategoryColumns, Key: []string{"Category ID"}, Cascade: dropCategoryAudience},
			{Name: homeLinksTab, Columns: HomeLinkColumns, Key: []string{"Link ID"}, Cascade: dropLinkAudience},
			AdminsTab,
			{Name: homeVisibilityTab, Columns: HomeVisibilityColumns, Key: []string{"App"}},
			{Name: homeAudienceTab, Columns: HomeAudienceColumns, Key: HomeAudienceColumns},
			{Name: homeWidgetsTab, Columns: HomeWidgetColumns, Key: []string{"Widget"}},
		},
		Build: func(ctx context.Context, tables store.Tables) (*Home, error) {
			return BuildHome(ctx, tables, images)
		},
		Loaded: func(m *Home, took time.Duration) {
			links := 0
			for _, category := range m.Categories {
				links += len(category.Links)
			}
			slog.Info("loaded apps model", "categories", len(m.Categories), "links", links, "took", took.Round(time.Millisecond))
		},
	}
}

func dropAudience(thing string, before, after store.Row) []store.Op {
	if before == nil || after != nil {
		return nil
	}
	return []store.Op{store.Delete(homeAudienceTab, store.Row{"Thing": thing})}
}

func dropLinkAudience(_ store.Tables, before, after store.Row) []store.Op {
	return dropAudience(thingLink+before["Link ID"], before, after)
}

func dropCategoryAudience(_ store.Tables, before, after store.Row) []store.Op {
	return dropAudience(thingCategory+before["Category ID"], before, after)
}

func NewHomeCache(source data.Source, writer data.Writer, images blob.Checker, superAdmins func() []string, directory *DirectoryCache, parties *PartiesCache, activities *ActivitiesCache, queue *store.Queue) (*HomeCache, error) {
	s, err := store.New(homeSpec(images), source, writer, queue)
	if err != nil {
		return nil, err
	}
	return &HomeCache{Store: s, AdminList: NewAdminList("home", HomeAdminAllowances, superAdmins, func() []string { return s.Model().admins }, s.Commit), directory: directory, parties: parties, activities: activities}, nil
}

func (c *HomeCache) sources() AudienceSources {
	return DirectoryAudience(c.directory.Model(), c.parties.Model(), c.activities.Model(), now())
}

func (c *HomeCache) linked(email string) []Linked {
	return LinkedEvents(c.directory.Model(), c.parties.Model(), c.activities.Model(), email, now())
}

func (c *HomeCache) includes(rules []Rule, email string) bool {
	return len(rules) > 0 && Audience{Rules: rules, Editors: c.Admins()}.Includes(c.sources(), email)
}

func (c *HomeCache) CategoriesFor(v access.Actor) []HomeCategory {
	forMe := func(rules []Rule) bool {
		return len(rules) == 0 || c.includes(rules, v.Email)
	}
	admin := v.May(ConfigureHome)
	full := c.Model()
	out := []HomeCategory{}
	for _, category := range full.Categories {
		sectionMine := forMe(category.Rules)
		if !sectionMine && !admin {
			continue
		}
		shown := HomeCategory{ID: category.ID, Title: category.Title, Emoji: category.Emoji, Style: category.Style, Max: category.Max, Links: []HomeLink{}, Virtual: category.Virtual, Rules: []Rule{}}
		if admin {
			shown.Rules = category.Rules
		}
		if !sectionMine {
			no := false
			shown.ForMe = &no
		}
		for _, link := range category.Links {
			mine := forMe(link.Rules)
			if !(link.Visible && mine) && !admin {
				continue
			}
			if !mine {
				no := false
				link.ForMe = &no
			}
			if !admin {
				link.Rules = []Rule{}
			}
			shown.Links = append(shown.Links, link)
		}
		out = append(out, shown)
	}
	return out
}

type AppVisibility struct {
	App
	Visibility string   `json:"visibility"`
	Emails     []string `json:"emails"`
	Rules      []Rule   `json:"rules"`
}

func appVisibilityOf(m *Home, app App) AppVisibilityRow {
	v, ok := m.Visibility[app.Key]
	if !ok {
		v = AppVisibilityRow{Mode: VisibleToList}
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

func orderedApps(m *Home) []App {
	out := slices.Clone(Apps)
	slices.SortStableFunc(out, func(a, b App) int {
		return store.CompareKeys(m.Visibility[a.Key].Order, m.Visibility[b.Key].Order)
	})
	return out
}

func (c *HomeCache) AppVisibilities() []AppVisibility {
	m := c.Model()
	out := []AppVisibility{}
	for _, app := range orderedApps(m) {
		v := appVisibilityOf(m, app)
		app.Name, app.Tagline, app.Mark = v.Name, v.Tagline, markVersion(app.Key)
		out = append(out, AppVisibility{App: app, Visibility: v.Mode, Emails: v.Emails, Rules: v.Rules})
	}
	return out
}

func (c *HomeCache) AppList() []App {
	m := c.Model()
	out := []App{}
	for _, app := range orderedApps(m) {
		v := appVisibilityOf(m, app)
		app.Name, app.Tagline, app.Mark = v.Name, v.Tagline, markVersion(app.Key)
		out = append(out, app)
	}
	return out
}

func (c *HomeCache) HiddenApps(email string) []string {
	email = c.sources().Directory.Resolve(strings.ToLower(strings.TrimSpace(email)))
	hidden := []string{}
	m := c.Model()
	for _, app := range Apps {
		v := appVisibilityOf(m, app)
		if v.Mode == VisibleToList && !slices.Contains(v.Emails, email) && !c.includes(v.Rules, email) {
			hidden = append(hidden, app.Key)
		}
	}
	return hidden
}

func (c *HomeCache) MissingVisibility() []App {
	m := c.Model()
	out := []App{}
	for _, app := range Apps {
		if _, ok := m.Visibility[app.Key]; !ok {
			out = append(out, app)
		}
	}
	return out
}

func GrantApp(ctx context.Context, cache *HomeCache, appKey, email string) error {
	actor := access.Actor{Email: strings.ToLower(strings.TrimSpace(email))}
	ops, err := cache.grant(actor, appKey)
	if err != nil {
		return err
	}
	return cache.Commit(ctx, actor, ops...)
}
