package home

import (
	"fmt"
	"maps"
	"net/url"
	"strings"
	"sync"
	"time"

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
	StyleCards = "cards"
	StyleTiles = "tiles"
)

var (
	categoryColumns  = []string{"Title", "Image", "Style"}
	linkColumns      = []string{"Title", "Description", "URL", "Image", "Category", "Visible", "Added By", "Added"}
	adminColumns     = []string{"Email"}
	changeLogColumns = []string{"Timestamp", "Actor", "Action", "Kind", "Title", "Description", "URL", "Image", "Category", "Visible", "Style"}
)

type ImageChecker interface {
	Has(key string) bool
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

type Category struct {
	Title    string `json:"title"`
	Image    string `json:"image,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
	Style    string `json:"style"`
	Links    []Link `json:"links"`
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

func exactColumns(table string, header, wanted []string) error {
	present := map[string]bool{}
	for _, h := range header {
		present[h] = true
	}
	for _, w := range wanted {
		if !present[w] {
			return fmt.Errorf("table %s is missing column %q", table, w)
		}
	}
	known := map[string]bool{}
	for _, w := range wanted {
		known[w] = true
	}
	for _, h := range header {
		if !known[h] {
			return fmt.Errorf("table %s has unexpected column %q", table, h)
		}
	}
	return nil
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
		if err := exactColumns(t.name, t.header, t.want); err != nil {
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

// cardsOrTiles is spelled exactly, the same stance yesNo takes: a blank or
// misspelled Style refuses the load rather than guessing a presentation.
func cardsOrTiles(cell string) (string, error) {
	switch cell {
	case StyleCards, StyleTiles:
		return cell, nil
	}
	return "", fmt.Errorf("%q is not %s or %s", cell, StyleCards, StyleTiles)
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
	if !images.Has(name) {
		return "", fmt.Errorf("image %q does not exist", name)
	}
	return "/" + name, nil
}

// BuildModel validates every row and refuses the whole set on the first
// problem, the same stance the directory takes: a sheet edit that breaks a
// rule surfaces as a refused load, never as a page quietly missing a link.
func BuildModel(tables *Tables, images ImageChecker) (*Model, error) {
	model := &Model{Categories: []Category{}}
	index := map[string]int{}
	for _, row := range tables.Categories {
		title := strings.TrimSpace(row["Title"])
		if title == "" {
			return nil, fmt.Errorf("category row %v has no title", row)
		}
		if _, dup := index[title]; dup {
			return nil, fmt.Errorf("duplicate category %q", title)
		}
		image, err := imageURL(images, row["Image"])
		if err != nil {
			return nil, fmt.Errorf("category %q: %w", title, err)
		}
		style, err := cardsOrTiles(row["Style"])
		if err != nil {
			return nil, fmt.Errorf("category %q: style %w", title, err)
		}
		index[title] = len(model.Categories)
		model.Categories = append(model.Categories, Category{Title: title, Image: row["Image"], ImageURL: image, Style: style, Links: []Link{}})
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
