package home

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/admins"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/filter"
	"heliosian/internal/store"
)

type Cache struct {
	*store.Store[*Model]
	admins.List
	sources func() filter.Sources
}

func spec(images blob.Checker) store.Spec[*Model] {
	return store.Spec[*Model]{
		App: appName,
		Tabs: []store.Tab{
			{Name: categoriesTab, Columns: categoryColumns, Key: []string{"Category ID"}, Cascade: dropCategoryAudience},
			{Name: linksTab, Columns: linkColumns, Key: []string{"Link ID"}, Cascade: dropLinkAudience},
			admins.Spec,
			{Name: visibilityTab, Columns: visibilityColumns, Key: []string{"App"}},
			{Name: audienceTab, Columns: AudienceColumns, Key: AudienceColumns},
			{Name: widgetsTab, Columns: widgetColumns, Key: []string{"Widget"}},
		},
		Build: func(ctx context.Context, tables store.Tables) (*Model, error) {
			return BuildModel(ctx, tables, images)
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

func dropAudience(thing string, before, after store.Row) []store.Op {
	if before == nil || after != nil {
		return nil
	}
	return []store.Op{store.Delete(audienceTab, store.Row{"Thing": thing})}
}

func dropLinkAudience(_ store.Tables, before, after store.Row) []store.Op {
	return dropAudience(thingLink+before["Link ID"], before, after)
}

func dropCategoryAudience(_ store.Tables, before, after store.Row) []store.Op {
	return dropAudience(thingCategory+before["Category ID"], before, after)
}

func NewCache(source data.Source, writer data.Writer, images blob.Checker, superAdmins func() []string, sources func() filter.Sources, queue *store.Queue) (*Cache, error) {
	s, err := store.New(spec(images), source, writer, queue)
	if err != nil {
		return nil, err
	}
	return &Cache{Store: s, List: admins.New("home", AdminAllowances, superAdmins, func() []string { return s.Model().admins }, s.Commit), sources: sources}, nil
}

func (c *Cache) includes(rules []filter.Rule, email string) bool {
	return len(rules) > 0 && filter.OnList(filter.List{Rules: rules, Editors: c.Admins()}, c.sources(), email)
}

func (c *Cache) CategoriesFor(v access.Actor) []Category {
	forMe := func(rules []filter.Rule) bool {
		return len(rules) == 0 || c.includes(rules, v.Email)
	}
	admin := v.May(Configure)
	full := c.Model()
	out := []Category{}
	for _, category := range full.Categories {
		sectionMine := forMe(category.Rules)
		if !sectionMine && !admin {
			continue
		}
		shown := Category{ID: category.ID, Title: category.Title, Emoji: category.Emoji, Style: category.Style, Max: category.Max, Links: []Link{}, Virtual: category.Virtual, Rules: []filter.Rule{}}
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
				link.Rules = []filter.Rule{}
			}
			shown.Links = append(shown.Links, link)
		}
		out = append(out, shown)
	}
	return out
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
	slices.SortStableFunc(out, func(a, b App) int {
		return store.CompareKeys(model.Visibility[a.Key].Order, model.Visibility[b.Key].Order)
	})
	return out
}

func (c *Cache) AppVisibilities() []AppVisibility {
	model := c.Model()
	out := []AppVisibility{}
	for _, app := range orderedApps(model) {
		v := visibilityOf(model, app)
		app.Name, app.Tagline, app.Mark = v.Name, v.Tagline, markVersion(app.Key)
		out = append(out, AppVisibility{App: app, Visibility: v.Mode, Emails: v.Emails, Rules: v.Rules})
	}
	return out
}

func (c *Cache) AppList() []App {
	model := c.Model()
	out := []App{}
	for _, app := range orderedApps(model) {
		v := visibilityOf(model, app)
		app.Name, app.Tagline, app.Mark = v.Name, v.Tagline, markVersion(app.Key)
		out = append(out, app)
	}
	return out
}

func (c *Cache) HiddenApps(email string) []string {
	email = c.sources().Directory.Resolve(strings.ToLower(strings.TrimSpace(email)))
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

func Grant(ctx context.Context, cache *Cache, appKey, email string) error {
	actor := access.Actor{Email: strings.ToLower(strings.TrimSpace(email))}
	ops, err := cache.grant(actor, appKey)
	if err != nil {
		return err
	}
	return cache.Commit(ctx, actor, ops...)
}
