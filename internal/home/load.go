package home

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"heliosian/internal/data"
	"heliosian/internal/filter"
)

const (
	appName        = "apps"
	categoriesTab  = "Categories"
	linksTab       = "Links"
	adminsTab      = "Admins"
	visibilityTab  = "Visibility"
	audienceTab    = "Audience"
	changeLogTab   = "Change Log"
	addedFormat    = "2006-01-02"
	maxTitleLength = 80
	maxDescLength  = 300
	maxURLLength   = 1000

	StyleCards  = "cards"
	StyleTiles  = "tiles"
	StyleEvents = "events"
	StyleApps   = "apps"

	EventsTitle = "Upcoming Events"
	EventsEmoji = "icon:when"
)

var (
	categoryColumns   = []string{"Title", "Emoji", "Style", "Max"}
	linkColumns       = []string{"Title", "Description", "URL", "Image", "Category", "Visible", "Added By", "Added"}
	AudienceColumns   = append([]string{"Thing"}, filter.RuleColumns...)
	adminColumns      = []string{"Email"}
	visibilityColumns = []string{"App", "Visibility", "Emails", "Tagline", "Name", "Order"}
	changeLogColumns  = []string{"Timestamp", "Actor", "Action", "Kind", "Title", "Description", "URL", "Image", "Category", "Visible", "Style", "Real Actor"}
	CategoryColumns   = categoryColumns
	LinkColumns       = linkColumns
	VisibilityColumns = visibilityColumns
	ChangeLogColumns  = changeLogColumns
)

type App struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Tagline string `json:"tagline"`
	Host    string `json:"host,omitempty"`
	Mark    string `json:"mark,omitempty"`
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

var Home = App{Key: "home", Name: "Heliosian", Tagline: "Helios Community Apps"}

var Apps = []App{
	{Key: "who", Name: "Helios Who?", Tagline: "A visual directory"},
	{Key: "team", Name: "HCA-Team", Tagline: "HCA Volunteer Portal"},
	{Key: "celebrate", Name: "Helios Celebrate", Tagline: "Fun(d)raiser Parties"},
	{Key: "birthday", Name: "Helios Birthday Team", Tagline: "Staff birthday donations"},
	{Key: "calendar", Name: "Helios Calendar", Tagline: "The school year, day by day", Host: "when"},
	{Key: "loop", Name: "Helios Loop", Tagline: "Email groups drawn from the directory"},
	{Key: "ask", Name: "Helios Ask", Tagline: "Ask about the school, your family and what's on"},
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

type Visibility struct {
	Mode    string
	Emails  []string
	Tagline string
	Name    string
	Rules   []filter.Rule
	Order   int
}

func (v Visibility) cells() map[string]string {
	order := ""
	if v.Order > 0 {
		order = strconv.Itoa(v.Order)
	}
	return map[string]string{"Visibility": v.Mode, "Emails": joinEmails(v.Emails), "Tagline": v.Tagline, "Name": v.Name, "Order": order}
}

func appKnown(key string) bool {
	_, ok := appByKey(key)
	return ok
}

type ImageChecker interface {
	Has(key string) (bool, error)
	Prefetch(names []string) error
}

type Link struct {
	Title       string        `json:"title"`
	Description string        `json:"description,omitempty"`
	URL         string        `json:"url"`
	Image       string        `json:"image,omitempty"`
	ImageURL    string        `json:"imageUrl,omitempty"`
	Category    string        `json:"category"`
	Visible     bool          `json:"visible"`
	AddedBy     string        `json:"addedBy,omitempty"`
	Added       string        `json:"added,omitempty"`
	Rules       []filter.Rule `json:"rules"`
	ForMe       *bool         `json:"forMe,omitempty"`
}

func splitList(cell string) []string {
	out := []string{}
	for _, part := range strings.Split(cell, ",") {
		if part = strings.TrimSpace(part); part != "" && !slices.Contains(out, part) {
			out = append(out, part)
		}
	}
	return out
}

type Category struct {
	Title   string        `json:"title"`
	Emoji   string        `json:"emoji,omitempty"`
	Style   string        `json:"style"`
	Max     int           `json:"max,omitempty"`
	Links   []Link        `json:"links"`
	Virtual bool          `json:"virtual,omitempty"`
	Rules   []filter.Rule `json:"rules"`
	ForMe   *bool         `json:"forMe,omitempty"`
}

type Model struct {
	Categories []Category            `json:"categories"`
	Visibility map[string]Visibility `json:"-"`
}

type Tables struct {
	Categories []map[string]string
	Links      []map[string]string
	Admins     []map[string]string
	Visibility []map[string]string
	Audience   []map[string]string
}

const (
	thingApp      = "app:"
	thingCategory = "category:"
	thingLink     = "link:"
)

func rulesFor(rows []map[string]string, key string) ([]filter.Rule, error) {
	out := []filter.Rule{}
	for _, row := range rows {
		if strings.TrimSpace(row["Thing"]) != key {
			continue
		}
		r := filter.Clean(filter.RuleFromRow(row))
		if err := filter.Check(r); err != nil {
			return nil, fmt.Errorf("%s rule for %s: %w", audienceTab, key, err)
		}
		out = append(out, r)
	}
	return out, nil
}

func (t *Tables) withAudience(key string, rules []filter.Rule) *Tables {
	out := *t
	out.Audience = []map[string]string{}
	for _, row := range t.Audience {
		if strings.TrimSpace(row["Thing"]) != key {
			out.Audience = append(out.Audience, row)
		}
	}
	for _, r := range rules {
		cells := filter.RuleCells(r)
		cells["Thing"] = key
		out.Audience = append(out.Audience, cells)
	}
	return &out
}

func audienceRows(key string, rules []filter.Rule) [][]string {
	rows := [][]string{}
	for _, r := range rules {
		cells := filter.RuleCells(r)
		cells["Thing"] = key
		row := make([]string, 0, len(AudienceColumns))
		for _, c := range AudienceColumns {
			row = append(row, cells[c])
		}
		rows = append(rows, row)
	}
	return rows
}

func ReadTables(source data.Source) (*Tables, error) {
	type table struct {
		name   string
		want   []string
		header []string
		rows   []map[string]string
	}
	categories := &table{name: categoriesTab, want: categoryColumns}
	links := &table{name: linksTab, want: linkColumns}
	admins := &table{name: adminsTab, want: adminColumns}
	visibility := &table{name: visibilityTab, want: visibilityColumns}
	audience := &table{name: audienceTab, want: AudienceColumns}
	changeLog := &table{name: changeLogTab, want: changeLogColumns}
	read := []*table{categories, links, admins, visibility, audience}
	names := []string{}
	for _, t := range read {
		names = append(names, t.name)
	}
	tabs, err := source.Tabs(appName, names, []string{changeLog.name})
	if err != nil {
		return nil, err
	}
	for _, t := range append(read, changeLog) {
		t.header, t.rows = tabs[t.name].Header, tabs[t.name].Rows
		if err := data.CheckColumns(t.name, t.header, t.want); err != nil {
			return nil, err
		}
	}
	return &Tables{Categories: categories.rows, Links: links.rows, Admins: admins.rows, Visibility: visibility.rows, Audience: audience.rows}, nil
}

func yesNo(cell string) (bool, error) {
	switch cell {
	case "Yes":
		return true, nil
	case "No":
		return false, nil
	}
	return false, fmt.Errorf("%q is not Yes or No", cell)
}

func checkMax(cell string) (int, error) {
	cell = strings.TrimSpace(cell)
	if cell == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(cell)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("max %q is not a whole number of one or more", cell)
	}
	return n, nil
}

func checkStyle(cell string) (string, error) {
	switch cell {
	case StyleCards, StyleTiles, StyleEvents, StyleApps:
		return cell, nil
	}
	return "", fmt.Errorf("%q is not %s, %s, %s or %s", cell, StyleCards, StyleTiles, StyleEvents, StyleApps)
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

func checkURL(raw string) error {
	if len(raw) > maxURLLength {
		return fmt.Errorf("url is too long")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("url %q must be an absolute http(s) url", raw)
	}
	return nil
}

func imageURL(images ImageChecker, name string) (string, error) {
	if name == "" {
		return "", nil
	}
	found, err := images.Has(name)
	if err != nil {
		return "", fmt.Errorf("image %q: %w", name, err)
	}
	if !found {
		return "", fmt.Errorf("image %q does not exist", name)
	}
	return "/" + name, nil
}

func imageNames(rows ...[]map[string]string) []string {
	names := []string{}
	for _, table := range rows {
		for _, row := range table {
			if row["Image"] != "" {
				names = append(names, row["Image"])
			}
		}
	}
	return names
}

func BuildModel(tables *Tables, images ImageChecker) (*Model, error) {
	if err := images.Prefetch(imageNames(tables.Links)); err != nil {
		return nil, err
	}
	model := &Model{Categories: []Category{}}
	index := map[string]int{}
	events, apps := false, false
	for _, row := range tables.Categories {
		title := strings.TrimSpace(row["Title"])
		if title == "" {
			return nil, fmt.Errorf("category row %v has no title", row)
		}
		if _, dup := index[title]; dup {
			return nil, fmt.Errorf("duplicate category %q", title)
		}
		emoji := strings.TrimSpace(row["Emoji"])
		if err := checkEmoji(emoji); err != nil {
			return nil, fmt.Errorf("category %q: %w", title, err)
		}
		style, err := checkStyle(row["Style"])
		if err != nil {
			return nil, fmt.Errorf("category %q: style %w", title, err)
		}
		if style == StyleEvents {
			if events {
				return nil, fmt.Errorf("category %q: only one category can be the %s section", title, StyleEvents)
			}
			events = true
		}
		if style == StyleApps {
			if apps {
				return nil, fmt.Errorf("category %q: only one category can be the %s section", title, StyleApps)
			}
			apps = true
		}
		max, err := checkMax(row["Max"])
		if err != nil {
			return nil, fmt.Errorf("category %q: %w", title, err)
		}
		rules, err := rulesFor(tables.Audience, thingCategory+title)
		if err != nil {
			return nil, err
		}
		index[title] = len(model.Categories)
		model.Categories = append(model.Categories, Category{Title: title, Emoji: emoji, Style: style, Max: max, Links: []Link{}, Rules: rules})
	}
	if !events {
		if _, taken := index[EventsTitle]; taken {
			return nil, fmt.Errorf("category %q is the events section's name; give it the %s style or another title", EventsTitle, StyleEvents)
		}
		model.Categories = append([]Category{{Title: EventsTitle, Emoji: EventsEmoji, Style: StyleEvents, Links: []Link{}, Virtual: true}}, model.Categories...)
		for title := range index {
			index[title]++
		}
		index[EventsTitle] = 0
	}
	titles := map[string]bool{}
	for _, row := range tables.Links {
		title := strings.TrimSpace(row["Title"])
		if title == "" {
			return nil, fmt.Errorf("link row %v has no title", row)
		}
		if titles[title] {
			return nil, fmt.Errorf("duplicate link %q", title)
		}
		titles[title] = true
		if err := checkURL(row["URL"]); err != nil {
			return nil, fmt.Errorf("link %q: %w", title, err)
		}
		at, ok := index[row["Category"]]
		if !ok {
			return nil, fmt.Errorf("link %q names unknown category %q", title, row["Category"])
		}
		if model.Categories[at].Style == StyleEvents {
			return nil, fmt.Errorf("link %q sits under %q, which holds HCA-Team's events rather than links", title, row["Category"])
		}
		if model.Categories[at].Style == StyleApps {
			return nil, fmt.Errorf("link %q sits under %q, which holds the community apps rather than links", title, row["Category"])
		}
		visible, err := yesNo(row["Visible"])
		if err != nil {
			return nil, fmt.Errorf("link %q: visible %w", title, err)
		}
		if row["Added"] != "" {
			if _, err := time.Parse(addedFormat, row["Added"]); err != nil {
				return nil, fmt.Errorf("link %q has invalid added date %q", title, row["Added"])
			}
		}
		image, err := imageURL(images, row["Image"])
		if err != nil {
			return nil, fmt.Errorf("link %q: %w", title, err)
		}
		rules, err := rulesFor(tables.Audience, thingLink+title)
		if err != nil {
			return nil, err
		}
		model.Categories[at].Links = append(model.Categories[at].Links, Link{
			Title: title, Description: row["Description"], URL: row["URL"],
			Image: row["Image"], ImageURL: image, Category: row["Category"],
			Visible: visible, AddedBy: row["Added By"], Added: row["Added"],
			Rules: rules,
		})
	}
	visibility, err := buildVisibility(tables.Visibility)
	if err != nil {
		return nil, err
	}
	for key, v := range visibility {
		rules, err := rulesFor(tables.Audience, thingApp+key)
		if err != nil {
			return nil, err
		}
		v.Rules = rules
		visibility[key] = v
	}
	model.Visibility = visibility
	return model, nil
}

func buildVisibility(rows []map[string]string) (map[string]Visibility, error) {
	visibility := map[string]Visibility{}
	for _, row := range rows {
		app := strings.ToLower(strings.TrimSpace(row["App"]))
		if !appKnown(app) {
			slog.Warn("visibility row names an app this build does not know, skipped", "app", app, "known", appKeys())
			continue
		}
		if _, dup := visibility[app]; dup {
			return nil, fmt.Errorf("%s has two rows for %q", visibilityTab, app)
		}
		mode := row["Visibility"]
		if mode != VisibleToEveryone && mode != VisibleToList {
			return nil, fmt.Errorf("%s row for %q: visibility %q is not %s or %s", visibilityTab, app, mode, VisibleToEveryone, VisibleToList)
		}
		tagline := strings.TrimSpace(row["Tagline"])
		if len(tagline) > maxDescLength {
			return nil, fmt.Errorf("%s row for %q: tagline is too long", visibilityTab, app)
		}
		name := strings.TrimSpace(row["Name"])
		if len(name) > maxTitleLength {
			return nil, fmt.Errorf("%s row for %q: name is too long", visibilityTab, app)
		}
		order := 0
		if cell := strings.TrimSpace(row["Order"]); cell != "" {
			n, err := strconv.Atoi(cell)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("%s row for %q: order %q is not a whole number of one or more", visibilityTab, app, cell)
			}
			order = n
		}
		visibility[app] = Visibility{Mode: mode, Emails: splitEmails(row["Emails"]), Tagline: tagline, Name: name, Order: order}
	}
	return visibility, nil
}

func splitEmails(cell string) []string {
	return normalizeEmails(strings.FieldsFunc(cell, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == ' '
	}))
}

func joinEmails(emails []string) string {
	return strings.Join(emails, ", ")
}

func appKeys() []string {
	keys := make([]string, 0, len(Apps))
	for _, app := range Apps {
		keys = append(keys, app.Key)
	}
	return keys
}

func cloneRows(rows []map[string]string) []map[string]string {
	out := make([]map[string]string, len(rows))
	for i, row := range rows {
		out[i] = maps.Clone(row)
	}
	return out
}

func applyCells(row, cells map[string]string) {
	for column, value := range cells {
		if value == "" {
			delete(row, column)
			continue
		}
		row[column] = value
	}
}

func (t *Tables) withRow(tab, key string, cells map[string]string) *Tables {
	out := *t
	rows := cloneRows(t.tab(tab))
	if key != "" {
		for _, row := range rows {
			if row["Title"] == key {
				applyCells(row, cells)
				out.setTab(tab, rows)
				return &out
			}
		}
	}
	row := map[string]string{}
	applyCells(row, cells)
	out.setTab(tab, append(rows, row))
	return &out
}

func (t *Tables) withoutRow(tab, key string) *Tables {
	out := *t
	rows := []map[string]string{}
	for _, row := range t.tab(tab) {
		if row["Title"] != key {
			rows = append(rows, row)
		}
	}
	out.setTab(tab, rows)
	return &out
}

func (t *Tables) withVisibility(app string, v Visibility) *Tables {
	out := *t
	out.Visibility = cloneRows(t.Visibility)
	cells := v.cells()
	cells["App"] = app
	for _, row := range out.Visibility {
		if strings.EqualFold(strings.TrimSpace(row["App"]), app) {
			applyCells(row, cells)
			return &out
		}
	}
	row := map[string]string{}
	applyCells(row, cells)
	out.Visibility = append(out.Visibility, row)
	return &out
}

func (t *Tables) withAdmins(emails []string) *Tables {
	out := *t
	out.Admins = make([]map[string]string, 0, len(emails))
	for _, email := range emails {
		out.Admins = append(out.Admins, map[string]string{"Email": email})
	}
	return &out
}

func (t *Tables) tab(name string) []map[string]string {
	if name == categoriesTab {
		return t.Categories
	}
	return t.Links
}

func (t *Tables) setTab(name string, rows []map[string]string) {
	if name == categoriesTab {
		t.Categories = rows
		return
	}
	t.Links = rows
}
