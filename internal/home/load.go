package home

import (
	"fmt"
	"maps"
	"net/url"
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
	changeLogTab   = "Change Log"
	addedFormat    = "2006-01-02"
	maxTitleLength = 80
	maxDescLength  = 300
	maxURLLength   = 1000

	// StyleCards renders a category as large feature cards, StyleTiles as a
	// row of compact tiles. Every category picks one; see docs/home/data.md.
	// StyleEvents is the one section that holds no links: HCA-Team's upcoming
	// events. The sheet may carry one such row, to name, mark and place it;
	// without one the page synthesizes it at the top (see BuildModel).
	StyleCards  = "cards"
	StyleTiles  = "tiles"
	StyleEvents = "events"

	// The events section as it stands until the sheet says otherwise.
	EventsTitle = "Upcoming Events"
	EventsEmoji = "📅"
)

var (
	categoryColumns  = []string{"Title", "Emoji", "Style"}
	linkColumns      = []string{"Title", "Description", "URL", "Image", "Category", "Visible", "Added By", "Added"}
	adminColumns     = []string{"Email"}
	changeLogColumns = []string{"Timestamp", "Actor", "Action", "Kind", "Title", "Description", "URL", "Image", "Category", "Visible", "Style"}
)

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
// Virtual marks the events section when the sheet has no row for it yet: it
// shows and edits like any other, and the first rename, emoji or move writes
// its row.
type Category struct {
	Title   string `json:"title"`
	Emoji   string `json:"emoji,omitempty"`
	Style   string `json:"style"`
	Links   []Link `json:"links"`
	Virtual bool   `json:"virtual,omitempty"`
}

// Model is the portal as the sheet orders it: categories in row order, each
// holding its links in row order.
type Model struct {
	Categories []Category `json:"categories"`
}

type Tables struct {
	Categories []map[string]string
	Links      []map[string]string
	Admins     []map[string]string
}

func ReadTables(source data.Source) (*Tables, error) {
	type table struct {
		name   string
		want   []string
		header []string
		rows   []map[string]string
		err    error
	}
	categories := &table{name: categoriesTab, want: categoryColumns}
	links := &table{name: linksTab, want: linkColumns}
	admins := &table{name: adminsTab, want: adminColumns}
	changeLog := &table{name: changeLogTab, want: changeLogColumns}
	var wg sync.WaitGroup
	for _, t := range []*table{categories, links, admins} {
		wg.Go(func() {
			t.header, t.rows, t.err = source.Table(appName, t.name)
		})
	}
	wg.Go(func() {
		changeLog.header, changeLog.err = source.Header(appName, changeLog.name)
	})
	wg.Wait()
	for _, t := range []*table{categories, links, admins, changeLog} {
		if t.err != nil {
			return nil, t.err
		}
		if err := data.CheckColumns(t.name, t.header, t.want); err != nil {
			return nil, err
		}
	}
	return &Tables{Categories: categories.rows, Links: links.rows, Admins: admins.rows}, nil
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

// checkStyle is spelled exactly, the same stance yesNo takes: a blank or
// misspelled Style refuses the load rather than guessing a presentation.
func checkStyle(cell string) (string, error) {
	switch cell {
	case StyleCards, StyleTiles, StyleEvents:
		return cell, nil
	}
	return "", fmt.Errorf("%q is not %s, %s or %s", cell, StyleCards, StyleTiles, StyleEvents)
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
	events := false
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
		index[title] = len(model.Categories)
		model.Categories = append(model.Categories, Category{Title: title, Emoji: emoji, Style: style, Links: []Link{}})
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
	return model, nil
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
