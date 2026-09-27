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
	"heliosian/internal/store"
)

type linkEdit struct {
	Original    string        `json:"original"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	URL         string        `json:"url"`
	Image       string        `json:"image"`
	Category    string        `json:"category"`
	Visible     bool          `json:"visible"`
	Rules       []filter.Rule `json:"rules"`
}

type categoryEdit struct {
	Original string        `json:"original"`
	Title    string        `json:"title"`
	Emoji    string        `json:"emoji"`
	Style    string        `json:"style"`
	Max      string        `json:"max"`
	Rules    []filter.Rule `json:"rules"`
}

type visibilityEdit struct {
	App        string         `json:"app"`
	Visibility string         `json:"visibility"`
	Emails     []string       `json:"emails"`
	Tagline    string         `json:"tagline"`
	Name       string         `json:"name"`
	Rules      *[]filter.Rule `json:"rules"`
}

func requireAdmin(actor access.Actor) error {
	if !actor.Admin {
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

func (m *Model) link(title string) *Link {
	for _, c := range m.Categories {
		for _, l := range c.Links {
			if strings.EqualFold(l.Title, strings.TrimSpace(title)) {
				return &l
			}
		}
	}
	return nil
}

func (m *Model) category(title string) *Category {
	for _, c := range m.Categories {
		if c.Title == title {
			return &c
		}
	}
	return nil
}

func (m *Model) styleOf(title string) string {
	for _, c := range m.Categories {
		if c.Title == title {
			return c.Style
		}
	}
	return ""
}

func (m *Model) titleOf(style string) string {
	for _, c := range m.Categories {
		if c.Style == style {
			return c.Title
		}
	}
	return ""
}

func (m *Model) virtualEvents(title string) bool {
	for _, c := range m.Categories {
		if c.Title == title {
			return c.Virtual
		}
	}
	return false
}

func categoryOrder(model *Model, titles []string, events store.Row) ([]store.Op, error) {
	byTitle := map[string]Category{}
	for _, c := range model.Categories {
		byTitle[c.Title] = c
	}
	if len(titles) != len(byTitle) {
		return nil, access.Invalid("the order must name every category exactly once")
	}
	current := []string{}
	virtual := []bool{}
	for _, title := range titles {
		c, ok := byTitle[title]
		if !ok {
			return nil, access.Invalid("unknown category %s", title)
		}
		delete(byTitle, title)
		current = append(current, c.order)
		virtual = append(virtual, c.Virtual)
	}
	keys := store.Order(current)
	ops := []store.Op{}
	for i, title := range titles {
		switch {
		case virtual[i]:
			row := maps.Clone(events)
			row[store.OrderColumn] = keys[i]
			ops = append(ops, store.Insert(categoriesTab, row))
		case keys[i] != current[i]:
			ops = append(ops, store.Update(categoriesTab, store.Row{"Title": title}, store.Row{store.OrderColumn: keys[i]}))
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
	existing := model.link(in.Original)
	if in.Original != "" && existing == nil {
		return "", "", nil, access.Missing("no such link")
	}
	was := rulesOf(model, thingLink+in.Original)
	rules, err := c.checkRules(was, in.Rules, actor.Email)
	if err != nil {
		return "", "", nil, err
	}
	row := store.Row{
		"Title": title, "Description": strings.TrimSpace(in.Description), "URL": strings.TrimSpace(in.URL),
		"Image": strings.TrimSpace(in.Image), "Category": strings.TrimSpace(in.Category), "Visible": cells.YesNoCell(in.Visible),
	}
	if existing != nil && existing.Category != row["Category"] {
		row[store.OrderColumn] = ""
	}
	action := "edit"
	op := store.Update(linksTab, store.Row{"Title": in.Original}, row)
	if in.Original == "" {
		action = "add"
		row["Added By"] = actor.Email
		row["Added"] = time.Now().Format(addedFormat)
		op = store.Insert(linksTab, row)
	}
	ops := []store.Op{op}
	if changed := audience(thingLink+title, was, rules); changed != nil {
		if in.Original != "" && in.Original != title {
			ops = append(ops, store.Delete(audienceTab, store.Row{"Thing": thingLink + in.Original}))
		}
		ops = append(ops, changed...)
	}
	return action, title, ops, nil
}

func (c *Cache) deleteLink(actor access.Actor, title string) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	return []store.Op{store.Delete(linksTab, store.Row{"Title": strings.TrimSpace(title)})}, nil
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
	if in.Original != "" && model.category(in.Original) == nil {
		return "", "", nil, access.Missing("no such category")
	}
	was := rulesOf(model, thingCategory+in.Original)
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
	virtual := model.virtualEvents(in.Original)
	if virtual {
		style = StyleEvents
	} else if in.Original != "" && model.styleOf(in.Original) == StyleEvents {
		style = StyleEvents
	} else if style == StyleEvents {
		return "", "", nil, access.Invalid("the events section is the one the page already has")
	}
	if style == StyleApps {
		if other := model.titleOf(StyleApps); other != "" && other != in.Original {
			return "", "", nil, access.Invalid("%q is already the community apps section", other)
		}
		if cat := model.category(in.Original); cat != nil && cat.Style != StyleApps && len(cat.Links) > 0 {
			return "", "", nil, access.Invalid("move or delete its links first: the community apps section holds no links")
		}
	}
	if _, err := checkMax(in.Max); err != nil {
		return "", "", nil, access.Invalid("%s", err)
	}
	cells := store.Row{"Title": title, "Emoji": emoji, "Style": style, "Max": strings.TrimSpace(in.Max)}
	action := "edit"
	var ops []store.Op
	switch {
	case virtual:
		titles := []string{}
		for _, cat := range model.Categories {
			titles = append(titles, cat.Title)
		}
		if ops, err = categoryOrder(model, titles, cells); err != nil {
			return "", "", nil, err
		}
	case in.Original == "":
		action = "add"
		ops = []store.Op{store.Insert(categoriesTab, cells)}
	default:
		ops = []store.Op{store.Update(categoriesTab, store.Row{"Title": in.Original}, cells)}
	}
	if changed := audience(thingCategory+title, was, rules); changed != nil {
		if in.Original != "" && in.Original != title {
			ops = append(ops, store.Delete(audienceTab, store.Row{"Thing": thingCategory + in.Original}))
		}
		ops = append(ops, changed...)
	}
	return action, title, ops, nil
}

func (c *Cache) reorderCategories(actor access.Actor, titles []string) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	return categoryOrder(c.Model(), titles, store.Row{"Title": EventsTitle, "Emoji": EventsEmoji, "Style": StyleEvents})
}

func (c *Cache) moveLink(actor access.Actor, title string, by int) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	if by != 1 && by != -1 {
		return nil, access.Invalid("by must be 1 or -1")
	}
	var titles, current []string
	at := -1
	for _, cat := range c.Model().Categories {
		for i, l := range cat.Links {
			if l.Title == title {
				at = i
			}
		}
		if at >= 0 {
			for _, l := range cat.Links {
				titles, current = append(titles, l.Title), append(current, l.order)
			}
			break
		}
	}
	if at < 0 {
		return nil, access.Invalid("unknown link %s", title)
	}
	to := at + by
	if to < 0 || to >= len(titles) {
		return nil, nil
	}
	titles[at], titles[to] = titles[to], titles[at]
	current[at], current[to] = current[to], current[at]
	keys := store.Order(current)
	ops := []store.Op{}
	for i, t := range titles {
		if keys[i] != current[i] {
			ops = append(ops, store.Update(linksTab, store.Row{"Title": t}, store.Row{store.OrderColumn: keys[i]}))
		}
	}
	return ops, nil
}

func (c *Cache) deleteCategory(actor access.Actor, title string) ([]store.Op, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	title = strings.TrimSpace(title)
	model := c.Model()
	if model.styleOf(title) == StyleEvents {
		return nil, access.Invalid("the events section can be renamed or moved, not deleted")
	}
	if cat := model.category(title); cat != nil && len(cat.Links) > 0 {
		return nil, access.Invalid("move or delete its links first")
	}
	return []store.Op{store.Delete(categoriesTab, store.Row{"Title": title})}, nil
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
