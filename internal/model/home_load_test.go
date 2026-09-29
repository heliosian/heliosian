package model

import (
	"context"
	"strings"
	"testing"

	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

func categoryRow(key, title, emoji, style string) store.Row {
	return store.Row{"Category ID": key, "Title": title, "Emoji": emoji, "Style": style}
}

func TestEventsSectionIsSynthesizedUntilTheSheetHasIt(t *testing.T) {
	tables := store.Tables{homeCategoriesTab: {categoryRow(schoolID, "School", "🏫", StyleCards)}}
	m, err := BuildHome(context.Background(), tables, testkit.All)
	if err != nil {
		t.Fatalf("build without an events row: %v", err)
	}
	if len(m.Categories) != 2 || m.Categories[0].ID != EventsCategoryID || m.Categories[0].Title != EventsCategoryTitle || m.Categories[0].Style != StyleEvents || !m.Categories[0].Virtual {
		t.Errorf("categories = %+v, want the synthesized events section first", m.Categories)
	}

	tables[homeCategoriesTab] = append(tables[homeCategoriesTab], categoryRow(eventsID, "What's On", "🎉", StyleEvents))
	m, err = BuildHome(context.Background(), tables, testkit.All)
	if err != nil {
		t.Fatalf("build with an events row: %v", err)
	}
	if len(m.Categories) != 2 || m.Categories[1].Title != "What's On" || m.Categories[1].Virtual {
		t.Errorf("categories = %+v, want the sheet's own events row second and not virtual", m.Categories)
	}

	tables[homeCategoriesTab] = append(tables[homeCategoriesTab], categoryRow(chatsID, "Another", "📅", StyleEvents))
	if _, err := BuildHome(context.Background(), tables, testkit.All); err == nil || !strings.Contains(err.Error(), "only one") {
		t.Errorf("two events rows built: %v", err)
	}
}

func TestTheEventsSectionsIDIsItsOwn(t *testing.T) {
	tables := store.Tables{homeCategoriesTab: {categoryRow(EventsCategoryID, "School", "", StyleCards)}}
	if _, err := BuildHome(context.Background(), tables, testkit.All); err == nil || !strings.Contains(err.Error(), "events section") {
		t.Errorf("a plain category holding the events section's id built: %v", err)
	}
	tables[homeCategoriesTab] = []store.Row{categoryRow(schoolID, EventsCategoryTitle, "", StyleCards)}
	if _, err := BuildHome(context.Background(), tables, testkit.All); err != nil {
		t.Errorf("a plain category titled as the events section refused: %v", err)
	}
}

func TestLinksCannotNameTheEventsSection(t *testing.T) {
	tables := store.Tables{
		homeCategoriesTab: {categoryRow(eventsID, "What's On", "🎉", StyleEvents)},
		homeLinksTab:      {{"Link ID": directoryID, "Title": "Gala", "URL": "https://example.org", "Category": eventsID, "Visible": "Yes"}},
	}
	if _, err := BuildHome(context.Background(), tables, testkit.All); err == nil || !strings.Contains(err.Error(), "events") {
		t.Errorf("a link under the events section built: %v", err)
	}
}

func TestIDsAreCheckedAtLoad(t *testing.T) {
	link := func(key, title, cat string) store.Row {
		return store.Row{"Link ID": key, "Title": title, "URL": "https://example.org", "Category": cat, "Visible": "Yes"}
	}
	for _, c := range []struct {
		name   string
		tables store.Tables
		want   string
	}{
		{"a category without an id", store.Tables{homeCategoriesTab: {categoryRow("", "School", "", StyleCards)}}, "not an id"},
		{"a category id with an i", store.Tables{homeCategoriesTab: {categoryRow("hcg000000000i", "School", "", StyleCards)}}, "not an id"},
		{"two categories with one id", store.Tables{homeCategoriesTab: {categoryRow(schoolID, "School", "", StyleCards), categoryRow(schoolID, "Clubs", "", StyleTiles)}}, "used twice"},
		{"a link without an id", store.Tables{homeCategoriesTab: {categoryRow(schoolID, "School", "", StyleCards)}, homeLinksTab: {link("", "Gala", schoolID)}}, "not an id"},
		{"a link sharing a category's id", store.Tables{homeCategoriesTab: {categoryRow(schoolID, "School", "", StyleCards)}, homeLinksTab: {link(schoolID, "Gala", schoolID)}}, "used twice"},
		{"two links with one id", store.Tables{homeCategoriesTab: {categoryRow(schoolID, "School", "", StyleCards)}, homeLinksTab: {link(directoryID, "Gala", schoolID), link(directoryID, "Fair", schoolID)}}, "used twice"},
		{"a link naming its category by title", store.Tables{homeCategoriesTab: {categoryRow(schoolID, "School", "", StyleCards)}, homeLinksTab: {link(directoryID, "Gala", "School")}}, "unknown category"},
	} {
		if _, err := BuildHome(context.Background(), c.tables, testkit.All); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s built: %v, want %q", c.name, err, c.want)
		}
	}
	tables := store.Tables{
		homeCategoriesTab: {categoryRow(schoolID, "School", "", StyleCards), categoryRow(chatsID, "School", "", StyleTiles)},
		homeLinksTab:      {link(directoryID, "Gala", strings.ToUpper(schoolID)), link(parentPortalID, "Gala", chatsID)},
	}
	m, err := BuildHome(context.Background(), tables, testkit.All)
	if err != nil {
		t.Fatalf("titles shared across rows refused: %v", err)
	}
	if got := m.category(schoolID).Links; len(got) != 1 || got[0].ID != directoryID || got[0].Category != schoolID {
		t.Errorf("school's links = %+v, want the one filed under its id in capitals", got)
	}
}
