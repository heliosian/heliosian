package model

import (
	"context"
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

var homeTabs = []store.Tab{
	{Name: homeCategoriesTab, Columns: HomeCategoryColumns, Key: []string{"Category ID"}, Cascade: dropCategoryAudience},
	{Name: homeLinksTab, Columns: HomeLinkColumns, Key: []string{"Link ID"}, Cascade: dropLinkAudience},
	AdminsTab,
	{Name: homeVisibilityTab, Columns: HomeVisibilityColumns, Key: []string{"App"}},
	{Name: homeAudienceTab, Columns: HomeAudienceColumns, Key: HomeAudienceColumns},
	{Name: homeWidgetsTab, Columns: HomeWidgetColumns, Key: []string{"Widget"}},
	{Name: homeToDosTab, Columns: HomeToDoColumns, Key: []string{"Email", "To Do"}},
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

func (m *Model) homeIncludes(rules []Rule, email string) bool {
	return len(rules) > 0 && Audience{Rules: rules, Editors: m.AdminList("home").Admins()}.Includes(m.Audience(now()), email)
}

func (m *Model) HomeCategoriesFor(v access.Actor) []HomeCategory {
	forMe := func(rules []Rule) bool {
		return len(rules) == 0 || m.homeIncludes(rules, v.Email)
	}
	admin := v.May(ConfigureHome)
	hidden := m.HiddenApps(v.Email)
	out := []HomeCategory{}
	for _, category := range m.Home.Categories {
		sectionMine := forMe(category.Rules)
		if !sectionMine && !admin {
			continue
		}
		shown := HomeCategory{ID: category.ID, Title: category.Title, Emoji: category.Emoji, Style: category.Style, Max: category.Max, Order: category.Order, Links: []HomeLink{}, Rules: []Rule{}}
		if admin {
			shown.Rules = category.Rules
		}
		if !sectionMine {
			no := false
			shown.ForMe = &no
		}
		for _, link := range category.Links {
			if link.app != "" && slices.Contains(hidden, link.app) {
				continue
			}
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
	Order      string   `json:"order"`
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

func (m *Home) AppVisibilities() []AppVisibility {
	out := []AppVisibility{}
	for _, app := range orderedApps(m) {
		v := appVisibilityOf(m, app)
		app.Name, app.Tagline, app.Mark = v.Name, v.Tagline, markVersion(app.Key)
		out = append(out, AppVisibility{App: app, Visibility: v.Mode, Emails: v.Emails, Order: v.Order, Rules: v.Rules})
	}
	return out
}

func (m *Home) AppList() []App {
	out := []App{}
	for _, app := range orderedApps(m) {
		v := appVisibilityOf(m, app)
		app.Name, app.Tagline, app.Mark = v.Name, v.Tagline, markVersion(app.Key)
		out = append(out, app)
	}
	return out
}

func (m *Model) HiddenApps(email string) []string {
	email = m.Directory.Resolve(strings.ToLower(strings.TrimSpace(email)))
	hidden := []string{}
	for _, app := range Apps {
		v := appVisibilityOf(m.Home, app)
		if v.Mode == VisibleToList && !slices.Contains(v.Emails, email) && !m.homeIncludes(v.Rules, email) {
			hidden = append(hidden, app.Key)
		}
	}
	return hidden
}

func (m *Home) MissingVisibility() []App {
	out := []App{}
	for _, app := range Apps {
		if _, ok := m.Visibility[app.Key]; !ok {
			out = append(out, app)
		}
	}
	return out
}

func (s *Store) GrantApp(ctx context.Context, appKey, email string) error {
	actor := access.Actor{Email: strings.ToLower(strings.TrimSpace(email))}
	ops, err := s.Model().Home.grant(actor, appKey)
	if err != nil {
		return err
	}
	return s.Commit(ctx, actor, homeAppName, ops...)
}
