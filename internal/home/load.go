package home

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"heliosian/internal/theme"
	"log/slog"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"heliosian/internal/data"
)

const (
	appName        = "apps"
	categoriesTab  = "Categories"
	linksTab       = "Links"
	adminsTab      = "Admins"
	visibilityTab  = "Visibility"
	settingsTab    = "Settings"
	changeLogTab   = "Change Log"
	addedFormat    = "2006-01-02"
	maxTitleLength = 80
	maxDescLength  = 300
	maxURLLength   = 1000

	// StyleCards renders a category as large feature cards, StyleTiles as a
	// row of compact tiles. Every category picks one; see docs/home/data.md.
	// StyleEvents and StyleApps are the two sections that hold no links:
	// Helios When's upcoming events, and the community apps themselves - the
	// same list the toolbar switches between, each with its mark and
	// tagline, less any the Visibility tab keeps from the viewer. The sheet
	// may carry one events row, to name, mark and place the section; without
	// one the page synthesizes it at the top (see BuildModel). The apps
	// section is on the page only while a row carries its style.
	StyleCards  = "cards"
	StyleTiles  = "tiles"
	StyleEvents = "events"
	StyleApps   = "apps"

	// The events section as it stands until the sheet says otherwise.
	EventsTitle = "Upcoming Events"
	EventsEmoji = "📅"
)

var (
	categoryColumns   = []string{"Title", "Emoji", "Style", "Max"}
	linkColumns       = []string{"Title", "Description", "URL", "Image", "Category", "Visible", "Added By", "Added"}
	adminColumns      = []string{"Email"}
	visibilityColumns = []string{"App", "Visibility", "Emails", "Tagline", "Name", "Order"}
	settingColumns    = []string{"Key", "Value"}
	changeLogColumns  = []string{"Timestamp", "Actor", "Action", "Kind", "Title", "Description", "URL", "Image", "Category", "Visible", "Style"}
)

// The Settings tab holds the front page's theme (internal/theme): the
// rail's and the page's colours. Any other key refuses the load, as a
// misspelled one would otherwise colour nothing, silently.
// App is one of the community apps the shared toolbar switches between,
// keyed by its hostname's first label; its mark is served from
// web/public/common/brand/apps/<key>.png. Apps is the registry: every app
// the Visibility tab has a row for and the front page's apps section can
// list, in the order the switch starts with. Key is the code's - each app is
// a package here and a hostname routed in internal/app - and Name and
// Tagline are the defaults the sheet's row starts with; the row's own win
// once edited, and its Order cell moves the app in the switch and on the
// front page (AppList). Heliosian itself, Home, heads the switch but is not
// among them: it is the front page, and the switch's way home.
type App struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Tagline string `json:"tagline"`
	// Host is the hostname's first label the switch links to, when it is
	// not the key: the calendar is the package and the sheet, when. the
	// address (internal/app sends its other addresses there).
	Host string `json:"host,omitempty"`
	// Mark is a fingerprint of the app's mark file, for the switch and the
	// front page to fetch the file by (?v=), so a redrawn mark reaches a
	// browser that cached the old one under the day-long brand caching.
	Mark string `json:"mark,omitempty"`
}

var marks sync.Map

// markVersion fingerprints web/public/common/brand/apps/<key>.png, once per
// process: the file is part of the build, so it changes only with a deploy.
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
	{Key: "groups", Name: "Helios Groups", Tagline: "Email groups drawn from the directory"},
}

func appByKey(key string) (App, bool) {
	for _, app := range Apps {
		if app.Key == key {
			return app, true
		}
	}
	return App{}, false
}

// An app's Visibility is VisibleToEveryone or VisibleToList, when only the
// people in its Emails cell see it, in the toolbar's switch and on the front
// page. Never access: a direct link still opens the app. The list is kept
// while the app is everyone's, so switching back to it finds the list as it
// was. An app with no row is a new one, and starts as VisibleToList with
// nobody on it - out of sight until an admin lets people in - and the
// server writes it that row when it finds it (Register).
const (
	VisibleToEveryone = "everyone"
	VisibleToList     = "list"
)

type Visibility struct {
	Mode    string
	Emails  []string
	Tagline string
	Name    string
	// Order is the app's place in the switch, counted from one; zero is a
	// row that has none, which keeps the registry's place after every row
	// that has one.
	Order int
}

// cells is the row as the sheet holds it.
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
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
	Image       string `json:"image,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
	Category    string `json:"category"`
	Visible     bool   `json:"visible"`
	AddedBy     string `json:"addedBy,omitempty"`
	Added       string `json:"added,omitempty"`
}

// A category goes by an emoji rather than a picture: it heads the section,
// marks the rail, and stands in for a link that has no image of its own.
// Max is how many of its things the page shows before a See More; zero
// shows them all. Virtual marks the events section when the sheet has no row
// for it yet: it shows and edits like any other, and the first rename, emoji
// or move writes its row.
type Category struct {
	Title   string `json:"title"`
	Emoji   string `json:"emoji,omitempty"`
	Style   string `json:"style"`
	Max     int    `json:"max,omitempty"`
	Links   []Link `json:"links"`
	Virtual bool   `json:"virtual,omitempty"`
}

// Model is the portal as the sheet orders it: categories in row order, each
// holding its links in row order. Visibility is the Visibility tab by app,
// absent for an app with no row.
type Model struct {
	Categories []Category            `json:"categories"`
	Visibility map[string]Visibility `json:"-"`
	Theme      theme.Theme           `json:"theme"`
}

type Tables struct {
	Categories []map[string]string
	Links      []map[string]string
	Admins     []map[string]string
	Visibility []map[string]string
	Settings   []map[string]string
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
	settings := &table{name: settingsTab, want: settingColumns}
	changeLog := &table{name: changeLogTab, want: changeLogColumns}
	read := []*table{categories, links, admins, visibility, settings}
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
	return &Tables{Categories: categories.rows, Links: links.rows, Admins: admins.rows, Visibility: visibility.rows, Settings: settings.rows}, nil
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

// checkMax reads a Max cell: blank for no limit, else a whole number of one
// or more.
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

// checkStyle is spelled exactly, the same stance yesNo takes: a blank or
// misspelled Style refuses the load rather than guessing a presentation.
func checkStyle(cell string) (string, error) {
	switch cell {
	case StyleCards, StyleTiles, StyleEvents, StyleApps:
		return cell, nil
	}
	return "", fmt.Errorf("%q is not %s, %s, %s or %s", cell, StyleCards, StyleTiles, StyleEvents, StyleApps)
}

// checkEmoji accepts a blank cell or one emoji - a short run of symbol runes,
// joiners and variation selectors, so a flag or a skin-toned face passes and
// a word does not.
func checkEmoji(cell string) error {
	if cell == "" {
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

// BuildModel validates every row and refuses the whole set on the first
// problem, the same stance the directory takes: a sheet edit that breaks a
// rule surfaces as a refused load, never as a page quietly missing a link.
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
		index[title] = len(model.Categories)
		model.Categories = append(model.Categories, Category{Title: title, Emoji: emoji, Style: style, Max: max, Links: []Link{}})
	}
	// The events section is always on the page: at the top, under its own
	// name, until a row places and names it.
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
		model.Categories[at].Links = append(model.Categories[at].Links, Link{
			Title: title, Description: row["Description"], URL: row["URL"],
			Image: row["Image"], ImageURL: image, Category: row["Category"],
			Visible: visible, AddedBy: row["Added By"], Added: row["Added"],
		})
	}
	visibility, err := buildVisibility(tables.Visibility)
	if err != nil {
		return nil, err
	}
	model.Visibility = visibility
	if model.Theme, err = buildTheme(tables.Settings); err != nil {
		return nil, err
	}
	return model, nil
}

// buildTheme reads the Settings tab, which holds nothing but the theme.
func buildTheme(rows []map[string]string) (theme.Theme, error) {
	for _, row := range rows {
		if key := strings.TrimSpace(row["Key"]); !theme.IsKey(key) {
			return theme.Theme{}, fmt.Errorf("%s has unknown key %q", settingsTab, key)
		}
	}
	t, err := theme.FromRows(rows)
	if err != nil {
		return theme.Theme{}, fmt.Errorf("%s: %w", settingsTab, err)
	}
	return t, nil
}

// withSetting is Tables with one Settings row set, for the model to be
// rebuilt and checked before the row is written.
func (t *Tables) withSetting(key, value string) *Tables {
	out := *t
	out.Settings = cloneRows(t.Settings)
	for _, row := range out.Settings {
		if strings.EqualFold(strings.TrimSpace(row["Key"]), key) {
			row["Value"] = value
			return &out
		}
	}
	out.Settings = append(out.Settings, map[string]string{"Key": key, "Value": value})
	return &out
}

// buildVisibility reads the Visibility tab: one row per app, spelling its
// Visibility exactly, and listing in Emails whoever sees it when that is
// list - separated by commas or line breaks, in the order written. A row for
// an app this build does not know is logged and skipped: a newer build has
// written it, and a partial deploy must not die over it. Anything else
// refuses the load, since a misspelled mode would narrow nothing, silently.
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

// splitEmails reads an Emails cell, normalized and deduplicated, in order.
func splitEmails(cell string) []string {
	return normalizeEmails(strings.FieldsFunc(cell, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == ' '
	}))
}

// joinEmails writes an Emails cell the way the sheet is read back.
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

// withRow mirrors what data.Writer.Upsert (or Append, when key is "") is
// about to write to one tab, so the model can be rebuilt and checked before
// anything is persisted.
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

// withVisibility mirrors what setVisibility upserts: the app's row takes the
// mode and the list, appended when the app had none. Its own tab is keyed
// by App rather than Title, so withRow does not serve it.
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
