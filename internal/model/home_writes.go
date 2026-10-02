package model

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

type linkEdit struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Image       string `json:"image"`
	Category    string `json:"category"`
	Visible     bool   `json:"visible"`
	Order       string `json:"order"`
	Rules       []Rule `json:"rules"`
}

type categoryEdit struct {
	Title        string `json:"title"`
	Emoji        string `json:"emoji"`
	Style        string `json:"style"`
	Descriptions bool   `json:"descriptions"`
	Sidebar      bool   `json:"sidebar"`
	Order        string `json:"order"`
	Rules        []Rule `json:"rules"`
}

type visibilityEdit struct {
	Visibility string   `json:"visibility"`
	Emails     []string `json:"emails"`
	Tagline    string   `json:"tagline"`
	Name       string   `json:"name"`
	Order      string   `json:"order"`
	Rules      []Rule   `json:"rules"`
}

type widgetEdit struct {
	Order   string `json:"order"`
	Sidebar bool   `json:"sidebar"`
	Rules   []Rule `json:"rules"`
}

type layoutEdit struct {
	Rows []string `json:"rows"`
}

var ConfigureHome = access.Named("home.configure")

var HomeAdminAllowances = []access.Allowance{ConfigureHome}

func requireHomeAdmin(actor access.Actor) error {
	if !actor.May(ConfigureHome) {
		return access.Forbidden("admin access required")
	}
	return nil
}

func sentOrder(cell string) (string, error) {
	order, err := checkOrder(cell)
	if err != nil {
		return "", access.Invalid("order %s", err)
	}
	return order, nil
}

func (m *Home) appOrders() []string {
	keys := []string{}
	for _, app := range orderedApps(m) {
		if v, ok := m.Visibility[app.Key]; ok {
			keys = append(keys, v.Order)
		}
	}
	return keys
}

func (m *Home) discover() ([]store.Op, []App) {
	found := m.MissingVisibility()
	ops := []store.Op{}
	keys := m.appOrders()
	for _, app := range found {
		order := keyAfter(keys)
		keys = append(keys, order)
		ops = append(ops, store.Insert(homeVisibilityTab, store.Row{"App": app.Key, "Visibility": VisibleToList, "Tagline": app.Tagline, "Name": app.Name, store.OrderColumn: order}))
	}
	widgets := m.widgetOrders()
	for _, name := range m.WidgetOrder {
		if m.widgetKeys[name] != "" {
			continue
		}
		order := keyAfter(widgets)
		widgets = append(widgets, order)
		ops = append(ops, store.Insert(homeWidgetsTab, store.Row{"Widget": name, store.OrderColumn: order}))
	}
	return ops, found
}

func (m *Home) grant(actor access.Actor, appKey string) ([]store.Op, error) {
	app, ok := appByKey(appKey)
	if !ok {
		return nil, access.Invalid("no app %q", appKey)
	}
	v := appVisibilityOf(m, app)
	if slices.Contains(v.Emails, actor.Email) {
		return nil, nil
	}
	return []store.Op{store.Update(homeVisibilityTab, store.Row{"App": app.Key}, store.Row{"Emails": joinVisibilityEmails(mail.NormalizeAll(append(v.Emails, actor.Email)))})}, nil
}

func (m *Model) checkHomeRules(existing, rules []Rule, actor string) ([]Rule, error) {
	sources := m.Audience(now())
	options := sources.Options(actor)
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
	if err := sources.Writable(actor, m.AdminList("home").Admins(), existing, out); err != nil {
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

func (m *Model) saveHomeWidget(actor access.Actor, widget string, in widgetEdit) ([]store.Op, error) {
	if err := requireHomeAdmin(actor); err != nil {
		return nil, err
	}
	if !slices.Contains(m.Home.WidgetOrder, widget) {
		return nil, access.Missing("no such widget")
	}
	key := thingWidget + widget
	if strings.HasPrefix(widget, thingCategory) {
		key = widget
	}
	was := rulesOf(m.Home, key)
	checked, err := m.checkHomeRules(was, in.Rules, actor.Email)
	if err != nil {
		return nil, err
	}
	ops := audienceOps(key, was, checked)
	changed := store.Row{}
	if in.Order != m.Home.widgetKeys[widget] {
		order, err := sentOrder(in.Order)
		if err != nil {
			return nil, err
		}
		changed[store.OrderColumn] = order
	}
	if in.Sidebar != m.Home.sidebar[widget] {
		changed["Sidebar"] = cells.YesNoCell(in.Sidebar)
	}
	if len(changed) > 0 {
		ops = append(ops, m.Home.widgetRowOp(widget, changed))
	}
	return ops, nil
}

func (m *Home) widgetRowOp(widget string, changed store.Row) store.Op {
	if m.widgetKeys[widget] == "" && changed[store.OrderColumn] == "" {
		changed[store.OrderColumn] = keyAfter(m.widgetOrders())
	}
	return store.Upsert(homeWidgetsTab, store.Row{"Widget": widget}, changed)
}

func (m *Home) saveLayout(actor access.Actor, in layoutEdit) ([]store.Op, error) {
	if err := requireHomeAdmin(actor); err != nil {
		return nil, err
	}
	if limit := max(len(m.WidgetOrder), len(m.Layout)); len(in.Rows) > limit {
		return nil, access.Invalid("%d rows is more than the page's %d", len(in.Rows), limit)
	}
	rows := []string{}
	for _, cell := range in.Rows {
		columns, err := checkRowLayout(cell)
		if err != nil {
			return nil, access.Invalid("%s", err)
		}
		rows = append(rows, columns)
	}
	ops := []store.Op{}
	for i, columns := range rows {
		row := store.Row{"Row": strconv.Itoa(i + 1)}
		if i >= len(m.Layout) {
			row["Columns"] = columns
			ops = append(ops, store.Insert(homeLayoutTab, row))
			continue
		}
		if columns != m.Layout[i] {
			ops = append(ops, store.Update(homeLayoutTab, row, store.Row{"Columns": columns}))
		}
	}
	for i := len(rows); i < len(m.Layout); i++ {
		ops = append(ops, store.Delete(homeLayoutTab, store.Row{"Row": strconv.Itoa(i + 1)}))
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

func (m *Home) appsSection() *HomeCategory {
	for _, c := range m.Categories {
		if holdsApps(c.Style) {
			return &c
		}
	}
	return nil
}

func (m *Home) taken(key string) bool {
	return m.category(key) != nil || m.link(key) != nil
}

func linkOrders(c *HomeCategory) []string {
	keys := []string{}
	for _, l := range c.Links {
		keys = append(keys, l.Order)
	}
	return keys
}

func (all *Model) saveHomeLink(actor access.Actor, key string, in linkEdit, taken func(string) bool) (string, string, []store.Op, error) {
	if err := requireHomeAdmin(actor); err != nil {
		return "", "", nil, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" || len(title) > maxHomeTitleLength || len(in.Description) > maxHomeDescLength {
		return "", "", nil, access.Invalid("title is required and fields must be short")
	}
	m := all.Home
	var existing *HomeLink
	var was []Rule
	if key != "" {
		if existing = m.link(key); existing == nil {
			return "", "", nil, access.Missing("no such link")
		}
		was = existing.Rules
	}
	rules, err := all.checkHomeRules(was, in.Rules, actor.Email)
	if err != nil {
		return "", "", nil, err
	}
	category := m.category(in.Category)
	if category == nil || category.Style == StyleEvents || holdsApps(category.Style) {
		return "", "", nil, access.Invalid("no such category for links")
	}
	row := store.Row{
		"Title": title, "Description": strings.TrimSpace(in.Description), "URL": strings.TrimSpace(in.URL),
		"Image": strings.TrimSpace(in.Image), "Category": category.ID, "Visible": cells.YesNoCell(in.Visible),
	}
	switch {
	case existing != nil && existing.Category == category.ID:
		order, err := sentOrder(in.Order)
		if err != nil {
			return "", "", nil, err
		}
		row[store.OrderColumn] = order
	default:
		row[store.OrderColumn] = keyAfter(linkOrders(category))
	}
	if existing == nil {
		key := id.New(taken)
		row["Link ID"] = key
		row["Added By"] = actor.Email
		row["Added"] = time.Now().Format(linkAddedFormat)
		return "add", key, append([]store.Op{store.Insert(homeLinksTab, row)}, audienceOps(thingLink+key, nil, rules)...), nil
	}
	return "edit", existing.ID, append([]store.Op{store.Update(homeLinksTab, store.Row{"Link ID": existing.ID}, row)}, audienceOps(thingLink+existing.ID, was, rules)...), nil
}

func (m *Home) deleteLink(actor access.Actor, key string) ([]store.Op, error) {
	if err := requireHomeAdmin(actor); err != nil {
		return nil, err
	}
	link := m.link(key)
	if link == nil {
		return nil, access.Missing("no such link")
	}
	return []store.Op{store.Delete(homeLinksTab, store.Row{"Link ID": link.ID})}, nil
}

func (all *Model) saveHomeCategory(actor access.Actor, key string, in categoryEdit, taken func(string) bool) (string, string, []store.Op, error) {
	if err := requireHomeAdmin(actor); err != nil {
		return "", "", nil, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" || len(title) > maxHomeTitleLength {
		return "", "", nil, access.Invalid("title is required and must be short")
	}
	m := all.Home
	var existing *HomeCategory
	var was []Rule
	if key != "" {
		if existing = m.category(key); existing == nil {
			return "", "", nil, access.Missing("no such category")
		}
		was = existing.Rules
	}
	rules, err := all.checkHomeRules(was, in.Rules, actor.Email)
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
	if holdsApps(style) {
		if other := m.appsSection(); other != nil && (existing == nil || other.ID != existing.ID) {
			return "", "", nil, access.Invalid("%q is already the community apps section", other.Title)
		}
		if existing != nil && !holdsApps(existing.Style) && len(existing.Links) > 0 {
			return "", "", nil, access.Invalid("move or delete its links first: the community apps section holds no links")
		}
	}
	row := store.Row{"Title": title, "Emoji": emoji, "Style": style}
	if (existing == nil && !in.Descriptions) || (existing != nil && existing.Descriptions != in.Descriptions) {
		row["Descriptions"] = cells.YesNoCell(in.Descriptions)
	}
	if existing == nil {
		orders := []string{}
		for _, c := range m.Categories {
			orders = append(orders, c.Order)
		}
		key := id.New(taken)
		row["Category ID"] = key
		row[store.OrderColumn] = keyAfter(orders)
		ops := append([]store.Op{store.Insert(homeCategoriesTab, row)}, audienceOps(thingCategory+key, nil, rules)...)
		return "add", key, append(ops, m.newWidgetRow(key, in.Sidebar)), nil
	}
	order, err := sentOrder(in.Order)
	if err != nil {
		return "", "", nil, err
	}
	row[store.OrderColumn] = order
	ops := append([]store.Op{store.Update(homeCategoriesTab, store.Row{"Category ID": existing.ID}, row)}, audienceOps(thingCategory+existing.ID, was, rules)...)
	if widget := thingCategory + existing.ID; in.Sidebar != m.sidebar[widget] {
		ops = append(ops, m.widgetRowOp(widget, store.Row{"Sidebar": cells.YesNoCell(in.Sidebar)}))
	}
	return "edit", existing.ID, ops, nil
}

func (m *Home) widgetOrders() []string {
	orders := []string{}
	for _, name := range m.WidgetOrder {
		if key := m.widgetKeys[name]; key != "" {
			orders = append(orders, key)
		}
	}
	return orders
}

func (m *Home) newWidgetRow(category string, sidebar bool) store.Op {
	row := store.Row{"Widget": thingCategory + category, store.OrderColumn: keyAfter(m.widgetOrders())}
	if sidebar {
		row["Sidebar"] = cells.YesNoCell(true)
	}
	return store.Insert(homeWidgetsTab, row)
}

func (m *Home) deleteCategory(actor access.Actor, key string) ([]store.Op, error) {
	if err := requireHomeAdmin(actor); err != nil {
		return nil, err
	}
	cat := m.category(key)
	if cat == nil {
		return nil, access.Missing("no such category")
	}
	if cat.Style == StyleEvents {
		return nil, access.Invalid("the events section can be renamed or moved, not deleted")
	}
	if holdsApps(cat.Style) {
		return nil, access.Invalid("the community apps section can be renamed or moved, not deleted")
	}
	if len(cat.Links) > 0 {
		return nil, access.Invalid("move or delete its links first")
	}
	return []store.Op{store.Delete(homeCategoriesTab, store.Row{"Category ID": cat.ID})}, nil
}

func (m *Model) setHomeVisibility(actor access.Actor, key string, in visibilityEdit, sent map[string]bool) (AppVisibilityRow, []store.Op, error) {
	if err := requireHomeAdmin(actor); err != nil {
		return AppVisibilityRow{}, nil, err
	}
	app, ok := appByKey(key)
	if !ok {
		return AppVisibilityRow{}, nil, access.Invalid("app must be one of %s", strings.Join(appKeys(), ", "))
	}
	if in.Visibility != VisibleToEveryone && in.Visibility != VisibleToList {
		return AppVisibilityRow{}, nil, access.Invalid("visibility must be %s or %s", VisibleToEveryone, VisibleToList)
	}
	tagline := strings.TrimSpace(in.Tagline)
	if (sent["tagline"] && tagline == "") || len(tagline) > maxHomeDescLength {
		return AppVisibilityRow{}, nil, access.Invalid("a short tagline is required")
	}
	name := strings.TrimSpace(in.Name)
	if (sent["name"] && name == "") || len(name) > maxHomeTitleLength {
		return AppVisibilityRow{}, nil, access.Invalid("a short name is required")
	}
	order, err := sentOrder(in.Order)
	if err != nil {
		return AppVisibilityRow{}, nil, err
	}
	was := appVisibilityOf(m.Home, app)
	rules, err := m.checkHomeRules(was.Rules, in.Rules, actor.Email)
	if err != nil {
		return AppVisibilityRow{}, nil, err
	}
	v := AppVisibilityRow{Mode: in.Visibility, Emails: mail.NormalizeAll(in.Emails), Tagline: tagline, Name: name, Order: order, Rules: rules}
	ops := append([]store.Op{store.Upsert(homeVisibilityTab, store.Row{"App": key}, v.cells())}, audienceOps(thingApp+key, was.Rules, rules)...)
	return v, ops, nil
}
