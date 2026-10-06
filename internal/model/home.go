package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

const (
	homeAppName        = "apps"
	homeCategoriesTab  = "Categories"
	homeLinksTab       = "Links"
	homeVisibilityTab  = "Visibility"
	homeAudienceTab    = "Audience"
	homeWidgetsTab     = "Widgets"
	homeLayoutTab      = "Layout"
	linkAddedFormat    = "2006-01-02"
	maxHomeTitleLength = 80
	maxHomeDescLength  = 300

	StyleCards    = "cards"
	StyleTiles    = "tiles"
	StyleEvents   = "events"
	StyleApps     = "apps"
	StyleAppsGrid = "apps-grid"

	EventsCategoryID    = "hcg0000000000"
	EventsCategoryTitle = "Upcoming Events"
	EventsCategoryEmoji = "icon:when"
)

var (
	HomeCategoryColumns   = []string{"Category ID", "Title", "Emoji", "Style", "Descriptions", store.OrderColumn}
	HomeLinkColumns       = []string{"Link ID", "Title", "Description", "URL", "Image", "Category", "Visible", "Added By", "Added", store.OrderColumn}
	HomeAudienceColumns   = append([]string{"Thing"}, RuleColumns...)
	HomeVisibilityColumns = []string{"App", "Visibility", "Emails", "Tagline", "Name", store.OrderColumn}
	HomeWidgetColumns     = []string{"Widget", "Sidebar", store.OrderColumn}
	HomeLayoutColumns     = []string{"Row", "Columns"}
	RowLayouts            = []string{"1/3 1/3 1/3", "2/3 1/3", "1/3 2/3", "1/2 1/2", "1"}
)

type App struct {
	Key     string   `json:"key"`
	Name    string   `json:"name"`
	Tagline string   `json:"tagline"`
	Hosts   []string `json:"-"`
	Mark    string   `json:"mark,omitempty"`
}

var marks sync.Map

func markVersion(key string) string {
	if v, ok := marks.Load(key); ok {
		return v.(string)
	}
	version := ""
	if data, err := os.ReadFile(filepath.Join("web", "public", "common", "brand", "apps", key+".png")); err == nil {
		sum := sha256.Sum256(data)
		version = hex.EncodeToString(sum[:4])
	}
	marks.Store(key, version)
	return version
}

var HomeApp = App{Key: "home", Name: "Heliosian", Tagline: "Helios Community Apps", Hosts: []string{"", "www", "home"}}

var Apps = []App{
	{Key: "who", Name: "Helios Who?", Tagline: "A visual directory", Hosts: []string{"who"}},
	{Key: "team", Name: "HCA-Team", Tagline: "HCA Volunteer Portal", Hosts: []string{"team", "hca"}},
	{Key: "celebrate", Name: "Helios Celebrate", Tagline: "Fun(d)raiser Parties", Hosts: []string{"celebrate"}},
	{Key: "birthday", Name: "Helios Birthday Team", Tagline: "Staff birthday donations", Hosts: []string{"birthday"}},
	{Key: "when", Name: "Helios Calendar", Tagline: "The school year, day by day", Hosts: []string{"when", "calendar", "cal"}},
	{Key: "loop", Name: "Helios Loop", Tagline: "Email lists drawn from the directory", Hosts: []string{"loop"}},
	{Key: "ask", Name: "Helios Ask", Tagline: "Ask about the school, your family and what's on", Hosts: []string{"ask"}},
	{Key: "admin", Name: "Helios Admin", Tagline: "The data behind the apps", Hosts: []string{"admin"}},
	{Key: "wiki", Name: "Helios Wiki", Tagline: "Parent-to-parent info", Hosts: []string{"wiki"}},
}

func Qualify(label, domain string) string {
	if label == "" {
		return domain
	}
	return label + "." + domain
}

func appByKey(key string) (App, bool) {
	for _, app := range Apps {
		if app.Key == key {
			return app, true
		}
	}
	return App{}, false
}

const (
	VisibleToEveryone = "everyone"
	VisibleToList     = "list"
)

type AppVisibilityRow struct {
	Mode    string
	Emails  []string
	Tagline string
	Name    string
	Rules   []Rule
	Order   string
}

func (v AppVisibilityRow) cells() store.Row {
	return store.Row{"Visibility": v.Mode, "Emails": joinVisibilityEmails(v.Emails), "Tagline": v.Tagline, "Name": v.Name, store.OrderColumn: v.Order}
}

func appKnown(key string) bool {
	_, ok := appByKey(key)
	return ok
}

type HomeLink struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
	Image       string `json:"image,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
	Category    string `json:"category"`
	Visible     bool   `json:"visible"`
	AddedBy     string `json:"addedBy,omitempty"`
	Added       string `json:"added,omitempty"`
	Order       string `json:"order"`
	Rules       []Rule `json:"rules"`
	ForMe       *bool  `json:"forMe,omitempty"`
	app         string
}

type HomeCategory struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Emoji        string     `json:"emoji,omitempty"`
	Style        string     `json:"style"`
	Descriptions bool       `json:"descriptions"`
	Order        string     `json:"order"`
	Links        []HomeLink `json:"links"`
	Rules        []Rule     `json:"rules"`
	ForMe        *bool      `json:"forMe,omitempty"`
}

type Home struct {
	Categories  []HomeCategory               `json:"categories"`
	Visibility  map[string]AppVisibilityRow  `json:"-"`
	WidgetRules map[string][]Rule            `json:"-"`
	WidgetOrder []string                     `json:"-"`
	ToDoStates  map[string]map[string]string `json:"-"`
	Layout      []string                     `json:"-"`
	widgetKeys  map[string]string
	sidebar     map[string]bool
	admins      []string
}

var HomeWidgets = []string{"when", "team", "celebrate", "school", "birthday", "todo"}

var Domains = []string{"heliosian.com", "heliosiandev.com"}

func compareOrder(a, b, aTitle, bTitle string) int {
	if c := store.CompareKeys(a, b); c != 0 {
		return c
	}
	return strings.Compare(aTitle, bTitle)
}

func checkOrder(cell string) (string, error) {
	order := strings.TrimSpace(cell)
	if order == "" {
		return "", fmt.Errorf("has no order")
	}
	return order, store.CheckKey(order)
}

func keyAfter(keys []string) string {
	return store.Order(append(slices.Clone(keys), ""))[len(keys)]
}

func appOfLink(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	for _, domain := range Domains {
		for _, app := range Apps {
			for _, label := range app.Hosts {
				if host == Qualify(label, domain) {
					return app.Key
				}
			}
		}
	}
	return ""
}

const (
	thingApp      = "app:"
	thingCategory = "category:"
	thingLink     = "link:"
	thingWidget   = "widget:"
)

func drawnAsWidget(style string) bool {
	return style != StyleEvents
}

func (m *Home) widgetNames() []string {
	out := slices.Clone(HomeWidgets)
	for _, c := range m.Categories {
		if drawnAsWidget(c.Style) {
			out = append(out, thingCategory+c.ID)
		}
	}
	return out
}

func buildWidgetOrder(m *Home, rows []store.Row) error {
	m.widgetKeys = map[string]string{}
	m.sidebar = map[string]bool{}
	names := m.widgetNames()
	for _, row := range rows {
		name := strings.TrimSpace(row["Widget"])
		if !slices.Contains(names, name) {
			return fmt.Errorf("%s has no widget %q; the widgets are %s", homeWidgetsTab, name, strings.Join(names, ", "))
		}
		if _, dup := m.widgetKeys[name]; dup {
			return fmt.Errorf("%s has two rows for %q", homeWidgetsTab, name)
		}
		order, err := checkOrder(row[store.OrderColumn])
		if err != nil {
			return fmt.Errorf("widget %q: %w", name, err)
		}
		m.widgetKeys[name] = order
		sidebar, err := cells.YesNo(row["Sidebar"], false)
		if err != nil {
			return fmt.Errorf("widget %q: sidebar %w", name, err)
		}
		m.sidebar[name] = sidebar
	}
	m.WidgetOrder = names
	slices.SortStableFunc(m.WidgetOrder, func(a, b string) int { return store.CompareKeys(m.widgetKeys[a], m.widgetKeys[b]) })
	return nil
}

func buildLayout(rows []store.Row) ([]string, error) {
	byRow := map[int]string{}
	for _, row := range rows {
		cell := strings.TrimSpace(row["Row"])
		n, err := strconv.Atoi(cell)
		if err != nil || n < 1 || strconv.Itoa(n) != cell {
			return nil, fmt.Errorf("%s row %q is not a row number", homeLayoutTab, row["Row"])
		}
		if _, dup := byRow[n]; dup {
			return nil, fmt.Errorf("%s has two rows numbered %d", homeLayoutTab, n)
		}
		columns, err := checkRowLayout(row["Columns"])
		if err != nil {
			return nil, fmt.Errorf("%s row %d: %w", homeLayoutTab, n, err)
		}
		byRow[n] = columns
	}
	out := []string{}
	for n := 1; n <= len(byRow); n++ {
		columns, ok := byRow[n]
		if !ok {
			return nil, fmt.Errorf("%s has no row %d, but has %d rows; they are numbered from 1 without gaps", homeLayoutTab, n, len(byRow))
		}
		out = append(out, columns)
	}
	return out, nil
}

func audienceRulesFor(rows []store.Row, key string) ([]Rule, error) {
	out := []Rule{}
	for _, row := range rows {
		if strings.TrimSpace(row["Thing"]) != key {
			continue
		}
		r := RuleFromRow(row).Clean()
		if err := r.Check(); err != nil {
			return nil, fmt.Errorf("%s rule for %s: %w", homeAudienceTab, key, err)
		}
		out = append(out, r)
	}
	return out, nil
}

func checkRowLayout(cell string) (string, error) {
	columns := strings.Join(strings.Fields(cell), " ")
	if !slices.Contains(RowLayouts, columns) {
		return "", fmt.Errorf("columns %q are not one of %s", cell, strings.Join(RowLayouts, ", "))
	}
	return columns, nil
}

func checkCategoryStyle(cell string) (string, error) {
	switch cell {
	case StyleCards, StyleTiles, StyleEvents, StyleApps, StyleAppsGrid:
		return cell, nil
	}
	return "", fmt.Errorf("%q is not %s, %s, %s, %s or %s", cell, StyleCards, StyleTiles, StyleEvents, StyleApps, StyleAppsGrid)
}

func holdsApps(style string) bool {
	return style == StyleApps || style == StyleAppsGrid
}

var iconValue = regexp.MustCompile(`^icon:[a-z]+$`)

func checkEmoji(cell string) error {
	if cell == "" || iconValue.MatchString(cell) {
		return nil
	}
	if utf8.RuneCountInString(cell) > 10 {
		return fmt.Errorf("emoji %q is too long", cell)
	}
	for _, r := range cell {
		joiner := r == 0x200d || r == 0xfe0f || r == 0xfe0e || (r >= 0x1f3fb && r <= 0x1f3ff) || r == 0x20e3
		if !joiner && (unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) || r < 0x2000) {
			return fmt.Errorf("%q is not an emoji", cell)
		}
	}
	return nil
}

func BuildHome(ctx context.Context, tables store.Tables, images blob.Checker) (*Home, error) {
	if err := images.Prefetch(ctx, cells.ImageNames([]string{"Image"}, tables[homeLinksTab])); err != nil {
		return nil, err
	}
	audience := tables[homeAudienceTab]
	m := &Home{Categories: []HomeCategory{}, WidgetRules: map[string][]Rule{}}
	for _, key := range HomeWidgets {
		rules, err := audienceRulesFor(audience, thingWidget+key)
		if err != nil {
			return nil, err
		}
		m.WidgetRules[key] = rules
	}
	m.admins = ReadAdmins(tables)
	toDos, err := buildHomeToDos(tables[homeToDosTab])
	if err != nil {
		return nil, err
	}
	m.ToDoStates = toDos
	index := map[string]int{}
	ids := map[string]bool{}
	events, apps := false, false
	for _, row := range tables[homeCategoriesTab] {
		title := strings.TrimSpace(row["Title"])
		if title == "" {
			return nil, fmt.Errorf("category row %v has no title", row)
		}
		key, ok := id.Parse(row["Category ID"])
		if !ok {
			return nil, fmt.Errorf("category %q: category id %q is not an id", title, row["Category ID"])
		}
		if ids[key] {
			return nil, fmt.Errorf("category %q: id %s is used twice", title, key)
		}
		ids[key] = true
		emoji := strings.TrimSpace(row["Emoji"])
		if err := checkEmoji(emoji); err != nil {
			return nil, fmt.Errorf("category %q: %w", title, err)
		}
		style, err := checkCategoryStyle(row["Style"])
		if err != nil {
			return nil, fmt.Errorf("category %q: style %w", title, err)
		}
		if style == StyleEvents {
			if events {
				return nil, fmt.Errorf("category %q: only one category can be the %s section", title, StyleEvents)
			}
			events = true
		}
		if holdsApps(style) {
			if apps {
				return nil, fmt.Errorf("category %q: only one category can be the %s section", title, StyleApps)
			}
			apps = true
		}
		order, err := checkOrder(row[store.OrderColumn])
		if err != nil {
			return nil, fmt.Errorf("category %q: %w", title, err)
		}
		descriptions, err := cells.YesNo(row["Descriptions"], true)
		if err != nil {
			return nil, fmt.Errorf("category %q: descriptions %w", title, err)
		}
		rules, err := audienceRulesFor(audience, thingCategory+key)
		if err != nil {
			return nil, err
		}
		index[key] = len(m.Categories)
		m.Categories = append(m.Categories, HomeCategory{ID: key, Title: title, Emoji: emoji, Style: style, Descriptions: descriptions, Order: order, Links: []HomeLink{}, Rules: rules})
	}
	if !events {
		return nil, fmt.Errorf("%s has no %s section; its row is %s (tools/createtabs seeds it)", homeCategoriesTab, StyleEvents, EventsCategoryID)
	}
	for _, row := range tables[homeLinksTab] {
		title := strings.TrimSpace(row["Title"])
		if title == "" {
			return nil, fmt.Errorf("link row %v has no title", row)
		}
		key, ok := id.Parse(row["Link ID"])
		if !ok {
			return nil, fmt.Errorf("link %q: link id %q is not an id", title, row["Link ID"])
		}
		if ids[key] {
			return nil, fmt.Errorf("link %q: id %s is used twice", title, key)
		}
		ids[key] = true
		order, err := checkOrder(row[store.OrderColumn])
		if err != nil {
			return nil, fmt.Errorf("link %q: %w", title, err)
		}
		if err := cells.URL(row["URL"], false); err != nil {
			return nil, fmt.Errorf("link %q: %w", title, err)
		}
		category, _ := id.Parse(row["Category"])
		at, ok := index[category]
		if !ok {
			return nil, fmt.Errorf("link %q names unknown category %q", title, row["Category"])
		}
		if m.Categories[at].Style == StyleEvents {
			return nil, fmt.Errorf("link %q sits under %q, which holds HCA-Team's events rather than links", title, m.Categories[at].Title)
		}
		if holdsApps(m.Categories[at].Style) {
			return nil, fmt.Errorf("link %q sits under %q, which holds the community apps rather than links", title, m.Categories[at].Title)
		}
		visible, err := cells.YesNo(row["Visible"], false)
		if err != nil {
			return nil, fmt.Errorf("link %q: visible %w", title, err)
		}
		if err := cells.Added(row["Added"]); err != nil {
			return nil, fmt.Errorf("link %q: %w", title, err)
		}
		image, err := cells.ImageURL(images, row["Image"])
		if err != nil {
			return nil, fmt.Errorf("link %q: %w", title, err)
		}
		rules, err := audienceRulesFor(audience, thingLink+key)
		if err != nil {
			return nil, err
		}
		m.Categories[at].Links = append(m.Categories[at].Links, HomeLink{
			ID: key, Title: title, Description: row["Description"], URL: row["URL"],
			Image: row["Image"], ImageURL: image, Category: category,
			Visible: visible, AddedBy: row["Added By"], Added: row["Added"],
			Order: order, Rules: rules, app: appOfLink(row["URL"]),
		})
	}
	for i := range m.Categories {
		slices.SortStableFunc(m.Categories[i].Links, func(a, b HomeLink) int { return compareOrder(a.Order, b.Order, a.Title, b.Title) })
	}
	slices.SortStableFunc(m.Categories, func(a, b HomeCategory) int { return compareOrder(a.Order, b.Order, a.Title, b.Title) })
	for _, c := range m.Categories {
		if drawnAsWidget(c.Style) {
			m.WidgetRules[thingCategory+c.ID] = c.Rules
		}
	}
	if err := buildWidgetOrder(m, tables[homeWidgetsTab]); err != nil {
		return nil, err
	}
	if m.Layout, err = buildLayout(tables[homeLayoutTab]); err != nil {
		return nil, err
	}
	visibility, err := buildVisibility(tables[homeVisibilityTab])
	if err != nil {
		return nil, err
	}
	for key, v := range visibility {
		rules, err := audienceRulesFor(audience, thingApp+key)
		if err != nil {
			return nil, err
		}
		v.Rules = rules
		visibility[key] = v
	}
	m.Visibility = visibility
	return m, nil
}

func buildVisibility(rows []store.Row) (map[string]AppVisibilityRow, error) {
	visibility := map[string]AppVisibilityRow{}
	for _, row := range rows {
		app := strings.ToLower(strings.TrimSpace(row["App"]))
		if !appKnown(app) {
			slog.Warn("visibility row names an app this build does not know, skipped", "app", app, "known", appKeys())
			continue
		}
		if _, dup := visibility[app]; dup {
			return nil, fmt.Errorf("%s has two rows for %q", homeVisibilityTab, app)
		}
		mode := row["Visibility"]
		if mode != VisibleToEveryone && mode != VisibleToList {
			return nil, fmt.Errorf("%s row for %q: visibility %q is not %s or %s", homeVisibilityTab, app, mode, VisibleToEveryone, VisibleToList)
		}
		tagline := strings.TrimSpace(row["Tagline"])
		if len(tagline) > maxHomeDescLength {
			return nil, fmt.Errorf("%s row for %q: tagline is too long", homeVisibilityTab, app)
		}
		name := strings.TrimSpace(row["Name"])
		if len(name) > maxHomeTitleLength {
			return nil, fmt.Errorf("%s row for %q: name is too long", homeVisibilityTab, app)
		}
		order, err := checkOrder(row[store.OrderColumn])
		if err != nil {
			return nil, fmt.Errorf("%s row for %q: %w", homeVisibilityTab, app, err)
		}
		visibility[app] = AppVisibilityRow{Mode: mode, Emails: splitVisibilityEmails(row["Emails"]), Tagline: tagline, Name: name, Order: order}
	}
	return visibility, nil
}

func splitVisibilityEmails(cell string) []string {
	return mail.NormalizeAll(strings.FieldsFunc(cell, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == ' '
	}))
}

func joinVisibilityEmails(emails []string) string {
	return strings.Join(emails, ", ")
}

func appKeys() []string {
	keys := []string{}
	for _, app := range Apps {
		keys = append(keys, app.Key)
	}
	return keys
}
