package home

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/config"
	"heliosian/internal/filter"
	"heliosian/internal/id"
	"heliosian/internal/store"
)

type linkEdit struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	URL         string        `json:"url"`
	Image       string        `json:"image"`
	Category    string        `json:"category"`
	Visible     bool          `json:"visible"`
	Rules       []filter.Rule `json:"rules"`
}

type categoryEdit struct {
	ID    string        `json:"id"`
	Title string        `json:"title"`
	Emoji string        `json:"emoji"`
	Style string        `json:"style"`
	Max   string        `json:"max"`
	Rules []filter.Rule `json:"rules"`
}

type visibilityEdit struct {
	App        string         `json:"app"`
	Visibility string         `json:"visibility"`
	Emails     []string       `json:"emails"`
	Tagline    string         `json:"tagline"`
	Name       string         `json:"name"`
	Rules      *[]filter.Rule `json:"rules"`
}

var Configure = access.Named("home.configure")

var AdminAllowances = []access.Allowance{Configure}

func requireAdmin(actor access.Actor) error {
	if !actor.May(Configure) {
		return access.Forbidden("admin access required")
	}
	return nil
}

func (c *Cache) discover(actor access.Actor) ([]store.Op, []App) {
	found := c.MissingVisibility()
	ops := []store.Op{}
	for _, app := range found {
		ops = append(ops, store.Insert(visibilityTab, store.Row{"App": app.Key, "Visibility": VisibleToList, "Tagline": app.Tagline, "Name": app.Name}))
	}
	return ops, found
}

func (c *Cache) grant(actor access.Actor, appKey string) ([]store.Op, error) {
	app, ok := appByKey(appKey)
	if !ok {
		return nil, fmt.Errorf("no app %q", appKey)
	}
	v := visibilityOf(c.Model(), app)
	if slices.Contains(v.Emails, actor.Email) {
		return nil, nil
	}
	return []store.Op{store.Upsert(visibilityTab, store.Row{"App": app.Key}, store.Row{"Visibility": v.Mode, "Emails": joinEmails(config.NormalizeEmails(append(v.Emails, actor.Email)))})}, nil
}

func (c *Cache) checkRules(existing, rules []filter.Rule, actor string) ([]filter.Rule, error) {
	options := filter.OptionsFor(c.sources(), actor)
	out := []filter.Rule{}
	for _, r := range rules {
		r = filter.Clean(r)
		if err := filter.Check(r); err != nil {
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
	if err := filter.Writable(c.sources(), actor, c.Admins(), existing, out); err != nil {
		return nil, access.Invalid("%s", err)
	}
	return out, nil
}

func audience(key string, was, rules []filter.Rule) []store.Op {
	if slices.EqualFunc(was, rules, func(x, y filter.Rule) bool { return maps.Equal(filter.RuleCells(x), filter.RuleCells(y)) }) {
		return nil
	}
	ops := []store.Op{store.Delete(audienceTab, store.Row{"Thing": key})}
	for _, r := range rules {
		cells := filter.RuleCells(r)
		cells["Thing"] = key
		ops = append(ops, store.Insert(audienceTab, cells))
	}
	return ops
}

func (c *Cache) saveWidgetAudience(actor access.Actor, widget string, rules []filter.Rule) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	if !slices.Contains(Widgets, widget) {
		return nil, access.Missing("no such widget")
	}
	key := thingWidget + widget
	was := rulesOf(c.Model(), key)
	checked, err := c.checkRules(was, rules, actor.Email)
	if err != nil {
		return nil, err
	}
	return audience(key, was, checked), nil
}

func (c *Cache) setWidgetOrder(actor access.Actor, widgets []string) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	got, want := slices.Clone(widgets), slices.Clone(Widgets)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		return nil, access.Invalid("the order must list every widget once: %s", strings.Join(Widgets, ", "))
	}
	model := c.Model()
	current := []string{}
	for _, name := range widgets {
		current = append(current, model.widgetKeys[name])
	}
	next := store.Order(current)
	ops := []store.Op{}
	for i, name := range widgets {
		if next[i] != current[i] {
			ops = append(ops, store.Upsert(widgetsTab, store.Row{"Widget": name}, store.Row{store.OrderColumn: next[i]}))
		}
	}
	return ops, nil
}

func (m *Model) link(key string) *Link {
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

func (m *Model) category(key string) *Category {
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

func (m *Model) styled(style string) *Category {
	for _, c := range m.Categories {
		if c.Style == style {
			return &c
		}
	}
	return nil
}

func (m *Model) taken(key string) bool {
	return m.category(key) != nil || m.link(key) != nil
}

func categoryOrder(model *Model, order []string, events store.Row) ([]store.Op, error) {
	byID := map[string]Category{}
	for _, c := range model.Categories {
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
			ops = append(ops, store.Insert(categoriesTab, row))
		case placed[i] != current[i]:
			ops = append(ops, store.Update(categoriesTab, store.Row{"Category ID": key}, store.Row{store.OrderColumn: placed[i]}))
		}
	}
	return ops, nil
}

func (c *Cache) saveLink(actor access.Actor, in linkEdit) (string, string, []store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return "", "", nil, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" || len(title) > maxTitleLength || len(in.Description) > maxDescLength {
		return "", "", nil, access.Invalid("title is required and fields must be short")
	}
	model := c.Model()
	var existing *Link
	var was []filter.Rule
	if strings.TrimSpace(in.ID) != "" {
		if existing = model.link(in.ID); existing == nil {
			return "", "", nil, access.Missing("no such link")
		}
		was = existing.Rules
	}
	rules, err := c.checkRules(was, in.Rules, actor.Email)
	if err != nil {
		return "", "", nil, err
	}
	category := model.category(in.Category)
	if category == nil {
		return "", "", nil, access.Invalid("no such category")
	}
	row := store.Row{
		"Title": title, "Description": strings.TrimSpace(in.Description), "URL": strings.TrimSpace(in.URL),
		"Image": strings.TrimSpace(in.Image), "Category": category.ID, "Visible": cells.YesNoCell(in.Visible),
	}
	if existing == nil {
		key := id.New(model.taken)
		row["Link ID"] = key
		row["Added By"] = actor.Email
		row["Added"] = time.Now().Format(addedFormat)
		return "add", key, append([]store.Op{store.Insert(linksTab, row)}, audience(thingLink+key, nil, rules)...), nil
	}
	if existing.Category != category.ID {
		row[store.OrderColumn] = ""
	}
	return "edit", existing.ID, append([]store.Op{store.Update(linksTab, store.Row{"Link ID": existing.ID}, row)}, audience(thingLink+existing.ID, was, rules)...), nil
}

func (c *Cache) deleteLink(actor access.Actor, key string) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	link := c.Model().link(key)
	if link == nil {
		return nil, access.Missing("no such link")
	}
	return []store.Op{store.Delete(linksTab, store.Row{"Link ID": link.ID})}, nil
}

func (c *Cache) saveCategory(actor access.Actor, in categoryEdit) (string, string, []store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return "", "", nil, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" || len(title) > maxTitleLength {
		return "", "", nil, access.Invalid("title is required and must be short")
	}
	model := c.Model()
	var existing *Category
	var was []filter.Rule
	if strings.TrimSpace(in.ID) != "" {
		if existing = model.category(in.ID); existing == nil {
			return "", "", nil, access.Missing("no such category")
		}
		was = existing.Rules
	}
	rules, err := c.checkRules(was, in.Rules, actor.Email)
	if err != nil {
		return "", "", nil, err
	}
	style, err := checkStyle(strings.TrimSpace(in.Style))
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
		if other := model.styled(StyleApps); other != nil && (existing == nil || other.ID != existing.ID) {
			return "", "", nil, access.Invalid("%q is already the community apps section", other.Title)
		}
		if existing != nil && existing.Style != StyleApps && len(existing.Links) > 0 {
			return "", "", nil, access.Invalid("move or delete its links first: the community apps section holds no links")
		}
	}
	if _, err := checkMax(in.Max); err != nil {
		return "", "", nil, access.Invalid("%s", err)
	}
	cells := store.Row{"Title": title, "Emoji": emoji, "Style": style, "Max": strings.TrimSpace(in.Max)}
	if existing == nil {
		key := id.New(model.taken)
		cells["Category ID"] = key
		return "add", key, append([]store.Op{store.Insert(categoriesTab, cells)}, audience(thingCategory+key, nil, rules)...), nil
	}
	changed := audience(thingCategory+existing.ID, was, rules)
	if !existing.Virtual {
		return "edit", existing.ID, append([]store.Op{store.Update(categoriesTab, store.Row{"Category ID": existing.ID}, cells)}, changed...), nil
	}
	cells["Category ID"] = existing.ID
	order := []string{}
	for _, cat := range model.Categories {
		order = append(order, cat.ID)
	}
	ops, err := categoryOrder(model, order, cells)
	if err != nil {
		return "", "", nil, err
	}
	return "edit", existing.ID, append(ops, changed...), nil
}

func (c *Cache) reorderCategories(actor access.Actor, order []string) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	return categoryOrder(c.Model(), order, store.Row{"Category ID": EventsID, "Title": EventsTitle, "Emoji": EventsEmoji, "Style": StyleEvents})
}

func (c *Cache) moveLink(actor access.Actor, key string, by int) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	if by == 0 {
		return nil, access.Invalid("by must not be 0")
	}
	model := c.Model()
	link := model.link(key)
	if link == nil {
		return nil, access.Invalid("unknown link %s", key)
	}
	var keys, current []string
	at := -1
	for i, l := range model.category(link.Category).Links {
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
			ops = append(ops, store.Update(linksTab, store.Row{"Link ID": k}, store.Row{store.OrderColumn: placed[i]}))
		}
	}
	return ops, nil
}

func (c *Cache) deleteCategory(actor access.Actor, key string) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
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
	return []store.Op{store.Delete(categoriesTab, store.Row{"Category ID": cat.ID})}, nil
}

func (c *Cache) setVisibility(actor access.Actor, in visibilityEdit) (string, Visibility, []store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return "", Visibility{}, nil, err
	}
	key := strings.ToLower(strings.TrimSpace(in.App))
	if !appKnown(key) {
		return "", Visibility{}, nil, access.Invalid("app must be one of %s", strings.Join(appKeys(), ", "))
	}
	if in.Visibility != VisibleToEveryone && in.Visibility != VisibleToList {
		return "", Visibility{}, nil, access.Invalid("visibility must be %s or %s", VisibleToEveryone, VisibleToList)
	}
	tagline := strings.TrimSpace(in.Tagline)
	if tagline == "" || len(tagline) > maxDescLength {
		return "", Visibility{}, nil, access.Invalid("a short tagline is required")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > maxTitleLength {
		return "", Visibility{}, nil, access.Invalid("a short name is required")
	}
	app, _ := appByKey(key)
	was := visibilityOf(c.Model(), app)
	rules := was.Rules
	if in.Rules != nil {
		checked, err := c.checkRules(was.Rules, *in.Rules, actor.Email)
		if err != nil {
			return "", Visibility{}, nil, err
		}
		rules = checked
	}
	v := Visibility{Mode: in.Visibility, Emails: config.NormalizeEmails(in.Emails), Tagline: tagline, Name: name, Order: was.Order, Rules: rules}
	ops := append([]store.Op{store.Upsert(visibilityTab, store.Row{"App": key}, v.cells())}, audience(thingApp+key, was.Rules, rules)...)
	return key, v, ops, nil
}

func (c *Cache) setAppOrder(actor access.Actor, order []string) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
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
	model := c.Model()
	apps := []App{}
	current := []string{}
	for _, key := range order {
		app, _ := appByKey(strings.ToLower(strings.TrimSpace(key)))
		apps, current = append(apps, app), append(current, visibilityOf(model, app).Order)
	}
	next := store.Order(current)
	ops := []store.Op{}
	for i, app := range apps {
		if next[i] != current[i] {
			ops = append(ops, store.Upsert(visibilityTab, store.Row{"App": app.Key}, store.Row{"Visibility": visibilityOf(model, app).Mode, store.OrderColumn: next[i]}))
		}
	}
	return ops, nil
}
