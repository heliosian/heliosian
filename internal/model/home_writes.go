package model

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

type linkEdit struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Image       string `json:"image"`
	Category    string `json:"category"`
	Visible     bool   `json:"visible"`
	Rules       []Rule `json:"rules"`
}

type categoryEdit struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Emoji string `json:"emoji"`
	Style string `json:"style"`
	Max   string `json:"max"`
	Rules []Rule `json:"rules"`
}

type visibilityEdit struct {
	App        string   `json:"app"`
	Visibility string   `json:"visibility"`
	Emails     []string `json:"emails"`
	Tagline    string   `json:"tagline"`
	Name       string   `json:"name"`
	Rules      *[]Rule  `json:"rules"`
}

var ConfigureHome = access.Named("home.configure")

var HomeAdminAllowances = []access.Allowance{ConfigureHome}

func requireHomeAdmin(actor access.Actor) error {
	if !actor.May(ConfigureHome) {
		return access.Forbidden("admin access required")
	}
	return nil
}

func (c *HomeCache) discover(actor access.Actor) ([]store.Op, []App) {
	found := c.MissingVisibility()
	ops := []store.Op{}
	for _, app := range found {
		ops = append(ops, store.Insert(homeVisibilityTab, store.Row{"App": app.Key, "Visibility": VisibleToList, "Tagline": app.Tagline, "Name": app.Name}))
	}
	return ops, found
}

func (c *HomeCache) grant(actor access.Actor, appKey string) ([]store.Op, error) {
	app, ok := appByKey(appKey)
	if !ok {
		return nil, fmt.Errorf("no app %q", appKey)
	}
	v := appVisibilityOf(c.Model(), app)
	if slices.Contains(v.Emails, actor.Email) {
		return nil, nil
	}
	return []store.Op{store.Upsert(homeVisibilityTab, store.Row{"App": app.Key}, store.Row{"Visibility": v.Mode, "Emails": joinVisibilityEmails(mail.NormalizeAll(append(v.Emails, actor.Email)))})}, nil
}

func (c *HomeCache) checkRules(existing, rules []Rule, actor string) ([]Rule, error) {
	options := c.sources().Options(actor)
	out := []Rule{}
	for _, r := range rules {
		r = r.Clean()
		if err := r.Check(); err != nil {
			return nil, access.Invalid("%s", err)
		}
		for _, g := range r.Grades {
			if !slices.Contains(options.Grades, g) {
				return nil, access.Invalid("the directory has no grade %s", g)
			}
		}
		for _, room := range r.Classrooms {
			if !slices.Contains(options.Classrooms, room) {
				return nil, access.Invalid("the directory has no classroom %s", room)
			}
		}
		out = append(out, r)
	}
	if err := c.sources().Writable(actor, c.Admins(), existing, out); err != nil {
		return nil, access.Invalid("%s", err)
	}
	return out, nil
}

func audienceOps(key string, was, rules []Rule) []store.Op {
	if slices.EqualFunc(was, rules, func(x, y Rule) bool { return maps.Equal(x.Cells(), y.Cells()) }) {
		return nil
	}
	ops := []store.Op{store.Delete(homeAudienceTab, store.Row{"Thing": key})}
	for _, r := range rules {
		cells := r.Cells()
		cells["Thing"] = key
		ops = append(ops, store.Insert(homeAudienceTab, cells))
	}
	return ops
}

func (c *HomeCache) saveWidgetAudience(actor access.Actor, widget string, rules []Rule) ([]store.Op, error) {
	if err := requireHomeAdmin(actor); err != nil {
		return nil, err
	}
	if !slices.Contains(HomeWidgets, widget) {
		return nil, access.Missing("no such widget")
	}
	key := thingWidget + widget
	was := rulesOf(c.Model(), key)
	checked, err := c.checkRules(was, rules, actor.Email)
	if err != nil {
		return nil, err
	}
	return audienceOps(key, was, checked), nil
}

func (c *HomeCache) setWidgetOrder(actor access.Actor, widgets []string) ([]store.Op, error) {
	if err := requireHomeAdmin(actor); err != nil {
		return nil, err
	}
	got, want := slices.Clone(widgets), slices.Clone(HomeWidgets)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		return nil, access.Invalid("the order must list every widget once: %s", strings.Join(HomeWidgets, ", "))
	}
	m := c.Model()
	current := []string{}
	for _, name := range widgets {
		current = append(current, m.widgetKeys[name])
	}
	next := store.Order(current)
	ops := []store.Op{}
	for i, name := range widgets {
		if next[i] != current[i] {
			ops = append(ops, store.Upsert(homeWidgetsTab, store.Row{"Widget": name}, store.Row{store.OrderColumn: next[i]}))
		}
	}
	return ops, nil
}

func (m *Home) link(key string) *HomeLink {
	key, ok := id.Parse(key)
	if !ok {
		return nil
	}
	for _, c := range m.Categories {
		for _, l := range c.Links {
			if l.ID == key {
				return &l
			}
		}
	}
	return nil
}

func (m *Home) category(key string) *HomeCategory {
	key, ok := id.Parse(key)
	if !ok {
		return nil
	}
	for _, c := range m.Categories {
		if c.ID == key {
			return &c
		}
	}
	return nil
}

func (m *Home) styled(style string) *HomeCategory {
	for _, c := range m.Categories {
		if c.Style == style {
			return &c
		}
	}
	return nil
}

func (m *Home) taken(key string) bool {
	return m.category(key) != nil || m.link(key) != nil
}

func categoryOrder(m *Home, order []string, events store.Row) ([]store.Op, error) {
	byID := map[string]HomeCategory{}
	for _, c := range m.Categories {
		byID[c.ID] = c
	}
	if len(order) != len(byID) {
		return nil, access.Invalid("the order must name every category exactly once")
	}
	named := []string{}
	current := []string{}
	virtual := []bool{}
	for _, raw := range order {
		key, _ := id.Parse(raw)
		c, ok := byID[key]
		if !ok {
			return nil, access.Invalid("unknown category %s", raw)
		}
		delete(byID, key)
		named = append(named, key)
		current = append(current, c.order)
		virtual = append(virtual, c.Virtual)
	}
	placed := store.Order(current)
	ops := []store.Op{}
	for i, key := range named {
		switch {
		case virtual[i]:
			row := maps.Clone(events)
			row[store.OrderColumn] = placed[i]
			ops = append(ops, store.Insert(homeCategoriesTab, row))
		case placed[i] != current[i]:
			ops = append(ops, store.Update(homeCategoriesTab, store.Row{"Category ID": key}, store.Row{store.OrderColumn: placed[i]}))
		}
	}
	return ops, nil
}

func (c *HomeCache) saveLink(actor access.Actor, in linkEdit) (string, string, []store.Op, error) {
	if err := requireHomeAdmin(actor); err != nil {
		return "", "", nil, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" || len(title) > maxHomeTitleLength || len(in.Description) > maxHomeDescLength {
		return "", "", nil, access.Invalid("title is required and fields must be short")
	}
	m := c.Model()
	var existing *HomeLink
	var was []Rule
	if strings.TrimSpace(in.ID) != "" {
		if existing = m.link(in.ID); existing == nil {
			return "", "", nil, access.Missing("no such link")
		}
		was = existing.Rules
	}
	rules, err := c.checkRules(was, in.Rules, actor.Email)
	if err != nil {
		return "", "", nil, err
	}
	category := m.category(in.Category)
	if category == nil {
		return "", "", nil, access.Invalid("no such category")
	}
	row := store.Row{
		"Title": title, "Description": strings.TrimSpace(in.Description), "URL": strings.TrimSpace(in.URL),
		"Image": strings.TrimSpace(in.Image), "Category": category.ID, "Visible": cells.YesNoCell(in.Visible),
	}
	if existing == nil {
		key := id.New(m.taken)
		row["Link ID"] = key
		row["Added By"] = actor.Email
		row["Added"] = time.Now().Format(linkAddedFormat)
		return "add", key, append([]store.Op{store.Insert(homeLinksTab, row)}, audienceOps(thingLink+key, nil, rules)...), nil
	}
	if existing.Category != category.ID {
		row[store.OrderColumn] = ""
	}
	return "edit", existing.ID, append([]store.Op{store.Update(homeLinksTab, store.Row{"Link ID": existing.ID}, row)}, audienceOps(thingLink+existing.ID, was, rules)...), nil
}

func (c *HomeCache) deleteLink(actor access.Actor, key string) ([]store.Op, error) {
	if err := requireHomeAdmin(actor); err != nil {
		return nil, err
	}
	link := c.Model().link(key)
	if link == nil {
		return nil, access.Missing("no such link")
	}
	return []store.Op{store.Delete(homeLinksTab, store.Row{"Link ID": link.ID})}, nil
}

func (c *HomeCache) saveCategory(actor access.Actor, in categoryEdit) (string, string, []store.Op, error) {
	if err := requireHomeAdmin(actor); err != nil {
		return "", "", nil, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" || len(title) > maxHomeTitleLength {
		return "", "", nil, access.Invalid("title is required and must be short")
	}
	m := c.Model()
	var existing *HomeCategory
	var was []Rule
	if strings.TrimSpace(in.ID) != "" {
		if existing = m.category(in.ID); existing == nil {
			return "", "", nil, access.Missing("no such category")
		}
		was = existing.Rules
	}
	rules, err := c.checkRules(was, in.Rules, actor.Email)
	if err != nil {
		return "", "", nil, err
	}
	style, err := checkCategoryStyle(strings.TrimSpace(in.Style))
	if err != nil {
		return "", "", nil, access.Invalid("style %s", err.Error())
	}
	emoji := strings.TrimSpace(in.Emoji)
	if err := checkEmoji(emoji); err != nil {
		return "", "", nil, access.Invalid("%s", err)
	}
	switch {
	case existing != nil && existing.Style == StyleEvents:
		style = StyleEvents
	case style == StyleEvents:
		return "", "", nil, access.Invalid("the events section is the one the page already has")
	}
	if style == StyleApps {
		if other := m.styled(StyleApps); other != nil && (existing == nil || other.ID != existing.ID) {
			return "", "", nil, access.Invalid("%q is already the community apps section", other.Title)
		}
		if existing != nil && existing.Style != StyleApps && len(existing.Links) > 0 {
			return "", "", nil, access.Invalid("move or delete its links first: the community apps section holds no links")
		}
	}
	if _, err := checkCategoryMax(in.Max); err != nil {
		return "", "", nil, access.Invalid("%s", err)
	}
	cells := store.Row{"Title": title, "Emoji": emoji, "Style": style, "Max": strings.TrimSpace(in.Max)}
	if existing == nil {
		key := id.New(m.taken)
		cells["Category ID"] = key
		return "add", key, append([]store.Op{store.Insert(homeCategoriesTab, cells)}, audienceOps(thingCategory+key, nil, rules)...), nil
	}
	changed := audienceOps(thingCategory+existing.ID, was, rules)
	if !existing.Virtual {
		return "edit", existing.ID, append([]store.Op{store.Update(homeCategoriesTab, store.Row{"Category ID": existing.ID}, cells)}, changed...), nil
	}
	cells["Category ID"] = existing.ID
	order := []string{}
	for _, cat := range m.Categories {
		order = append(order, cat.ID)
	}
	ops, err := categoryOrder(m, order, cells)
	if err != nil {
		return "", "", nil, err
	}
	return "edit", existing.ID, append(ops, changed...), nil
}

func (c *HomeCache) reorderCategories(actor access.Actor, order []string) ([]store.Op, error) {
	if err := requireHomeAdmin(actor); err != nil {
		return nil, err
	}
	return categoryOrder(c.Model(), order, store.Row{"Category ID": EventsCategoryID, "Title": EventsCategoryTitle, "Emoji": EventsCategoryEmoji, "Style": StyleEvents})
}

func (c *HomeCache) moveLink(actor access.Actor, key string, by int) ([]store.Op, error) {
	if err := requireHomeAdmin(actor); err != nil {
		return nil, err
	}
	if by == 0 {
		return nil, access.Invalid("by must not be 0")
	}
	m := c.Model()
	link := m.link(key)
	if link == nil {
		return nil, access.Invalid("unknown link %s", key)
	}
	var keys, current []string
	at := -1
	for i, l := range m.category(link.Category).Links {
		if l.ID == link.ID {
			at = i
		}
		keys, current = append(keys, l.ID), append(current, l.order)
	}
	to := at + by
	if to < 0 || to >= len(keys) {
		return nil, nil
	}
	movedKey, movedOrder := keys[at], current[at]
	keys = slices.Insert(slices.Delete(keys, at, at+1), to, movedKey)
	current = slices.Insert(slices.Delete(current, at, at+1), to, movedOrder)
	placed := store.Order(current)
	ops := []store.Op{}
	for i, k := range keys {
		if placed[i] != current[i] {
			ops = append(ops, store.Update(homeLinksTab, store.Row{"Link ID": k}, store.Row{store.OrderColumn: placed[i]}))
		}
	}
	return ops, nil
}

func (c *HomeCache) deleteCategory(actor access.Actor, key string) ([]store.Op, error) {
	if err := requireHomeAdmin(actor); err != nil {
		return nil, err
	}
	cat := c.Model().category(key)
	if cat == nil {
		return nil, access.Missing("no such category")
	}
	if cat.Style == StyleEvents {
		return nil, access.Invalid("the events section can be renamed or moved, not deleted")
	}
	if len(cat.Links) > 0 {
		return nil, access.Invalid("move or delete its links first")
	}
	return []store.Op{store.Delete(homeCategoriesTab, store.Row{"Category ID": cat.ID})}, nil
}

func (c *HomeCache) setVisibility(actor access.Actor, in visibilityEdit) (string, AppVisibilityRow, []store.Op, error) {
	if err := requireHomeAdmin(actor); err != nil {
		return "", AppVisibilityRow{}, nil, err
	}
	key := strings.ToLower(strings.TrimSpace(in.App))
	if !appKnown(key) {
		return "", AppVisibilityRow{}, nil, access.Invalid("app must be one of %s", strings.Join(appKeys(), ", "))
	}
	if in.Visibility != VisibleToEveryone && in.Visibility != VisibleToList {
		return "", AppVisibilityRow{}, nil, access.Invalid("visibility must be %s or %s", VisibleToEveryone, VisibleToList)
	}
	tagline := strings.TrimSpace(in.Tagline)
	if tagline == "" || len(tagline) > maxHomeDescLength {
		return "", AppVisibilityRow{}, nil, access.Invalid("a short tagline is required")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > maxHomeTitleLength {
		return "", AppVisibilityRow{}, nil, access.Invalid("a short name is required")
	}
	app, _ := appByKey(key)
	was := appVisibilityOf(c.Model(), app)
	rules := was.Rules
	if in.Rules != nil {
		checked, err := c.checkRules(was.Rules, *in.Rules, actor.Email)
		if err != nil {
			return "", AppVisibilityRow{}, nil, err
		}
		rules = checked
	}
	v := AppVisibilityRow{Mode: in.Visibility, Emails: mail.NormalizeAll(in.Emails), Tagline: tagline, Name: name, Order: was.Order, Rules: rules}
	ops := append([]store.Op{store.Upsert(homeVisibilityTab, store.Row{"App": key}, v.cells())}, audienceOps(thingApp+key, was.Rules, rules)...)
	return key, v, ops, nil
}

func (c *HomeCache) setAppOrder(actor access.Actor, order []string) ([]store.Op, error) {
	if err := requireHomeAdmin(actor); err != nil {
		return nil, err
	}
	keys := []string{}
	for _, key := range order {
		keys = append(keys, strings.ToLower(strings.TrimSpace(key)))
	}
	slices.Sort(keys)
	want := appKeys()
	slices.Sort(want)
	if !slices.Equal(keys, want) {
		return nil, access.Invalid("the order must list every app once: %s", strings.Join(appKeys(), ", "))
	}
	m := c.Model()
	apps := []App{}
	current := []string{}
	for _, key := range order {
		app, _ := appByKey(strings.ToLower(strings.TrimSpace(key)))
		apps, current = append(apps, app), append(current, appVisibilityOf(m, app).Order)
	}
	next := store.Order(current)
	ops := []store.Op{}
	for i, app := range apps {
		if next[i] != current[i] {
			ops = append(ops, store.Upsert(homeVisibilityTab, store.Row{"App": app.Key}, store.Row{"Visibility": appVisibilityOf(m, app).Mode, store.OrderColumn: next[i]}))
		}
	}
	return ops, nil
}
