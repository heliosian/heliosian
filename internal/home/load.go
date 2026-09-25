package home

import (
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
	"time"
	"unicode"
	"unicode/utf8"

	"heliosian/internal/filter"
	"heliosian/internal/store"
)

const (
	appName        = "apps"
	categoriesTab  = "Categories"
	linksTab       = "Links"
	adminsTab      = "Admins"
	visibilityTab  = "Visibility"
	audienceTab    = "Audience"
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
	categoryColumns   = []string{"Title", "Emoji", "Style", "Max", store.OrderColumn}
	linkColumns       = []string{"Title", "Description", "URL", "Image", "Category", "Visible", "Added By", "Added", store.OrderColumn}
	AudienceColumns   = append([]string{"Thing"}, filter.RuleColumns...)
	adminColumns      = []string{"Email"}
	visibilityColumns = []string{"App", "Visibility", "Emails", "Tagline", "Name", store.OrderColumn}
	CategoryColumns   = categoryColumns
	LinkColumns       = linkColumns
	AdminColumns      = adminColumns
	VisibilityColumns = visibilityColumns
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
	Order   string
}

func (v Visibility) cells() store.Row {
	return store.Row{"Visibility": v.Mode, "Emails": joinEmails(v.Emails), "Tagline": v.Tagline, "Name": v.Name, store.OrderColumn: v.Order}
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
	order       string
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
	order   string
}

type Model struct {
	Categories []Category            `json:"categories"`
	Visibility map[string]Visibility `json:"-"`
	admins     []string
}

func compareOrder(a, b, aTitle, bTitle string) int {
	if c := store.CompareKeys(a, b); c != 0 || a == "" {
		return c
	}
	return strings.Compare(aTitle, bTitle)
}

const (
	thingApp      = "app:"
	thingCategory = "category:"
	thingLink     = "link:"
)

func rulesFor(rows []store.Row, key string) ([]filter.Rule, error) {
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

func imageNames(rows []store.Row) []string {
	names := []string{}
	for _, row := range rows {
		if row["Image"] != "" {
			names = append(names, row["Image"])
		}
	}
	return names
}

func BuildModel(tables store.Tables, images ImageChecker) (*Model, error) {
	if err := images.Prefetch(imageNames(tables[linksTab])); err != nil {
		return nil, err
	}
	audience := tables[audienceTab]
	model := &Model{Categories: []Category{}}
	for _, row := range tables[adminsTab] {
		model.admins = append(model.admins, row["Email"])
	}
	model.admins = normalizeEmails(model.admins)
	index := map[string]int{}
	events, apps := false, false
	for _, row := range tables[categoriesTab] {
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
		order := strings.TrimSpace(row[store.OrderColumn])
		if err := store.CheckKey(order); err != nil {
			return nil, fmt.Errorf("category %q: %w", title, err)
		}
		rules, err := rulesFor(audience, thingCategory+title)
		if err != nil {
			return nil, err
		}
		index[title] = len(model.Categories)
		model.Categories = append(model.Categories, Category{Title: title, Emoji: emoji, Style: style, Max: max, Links: []Link{}, Rules: rules, order: order})
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
	for _, row := range tables[linksTab] {
		title := strings.TrimSpace(row["Title"])
		if title == "" {
			return nil, fmt.Errorf("link row %v has no title", row)
		}
		if titles[title] {
			return nil, fmt.Errorf("duplicate link %q", title)
		}
		titles[title] = true
		order := strings.TrimSpace(row[store.OrderColumn])
		if err := store.CheckKey(order); err != nil {
			return nil, fmt.Errorf("link %q: %w", title, err)
		}
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
		rules, err := rulesFor(audience, thingLink+title)
		if err != nil {
			return nil, err
		}
		model.Categories[at].Links = append(model.Categories[at].Links, Link{
			Title: title, Description: row["Description"], URL: row["URL"],
			Image: row["Image"], ImageURL: image, Category: row["Category"],
			Visible: visible, AddedBy: row["Added By"], Added: row["Added"],
			Rules: rules, order: order,
		})
	}
	for i := range model.Categories {
		slices.SortStableFunc(model.Categories[i].Links, func(a, b Link) int { return compareOrder(a.order, b.order, a.Title, b.Title) })
	}
	stored := model.Categories
	if !events {
		stored = model.Categories[1:]
	}
	slices.SortStableFunc(stored, func(a, b Category) int { return compareOrder(a.order, b.order, a.Title, b.Title) })
	visibility, err := buildVisibility(tables[visibilityTab])
	if err != nil {
		return nil, err
	}
	for key, v := range visibility {
		rules, err := rulesFor(audience, thingApp+key)
		if err != nil {
			return nil, err
		}
		v.Rules = rules
		visibility[key] = v
	}
	model.Visibility = visibility
	return model, nil
}

func buildVisibility(rows []store.Row) (map[string]Visibility, error) {
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
		order := strings.TrimSpace(row[store.OrderColumn])
		if err := store.CheckKey(order); err != nil {
			return nil, fmt.Errorf("%s row for %q: %w", visibilityTab, app, err)
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
