package model

import (
	"context"
	"strings"
	"testing"

	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

func categoryRow(key, title, emoji, style, order string) store.Row {
	return store.Row{"Category ID": key, "Title": title, "Emoji": emoji, "Style": style, store.OrderColumn: order}
}

func eventsRow(order string) store.Row {
	return categoryRow(EventsCategoryID, EventsCategoryTitle, EventsCategoryEmoji, StyleEvents, order)
}

func linkRow(key, title, category, order string) store.Row {
	return store.Row{"Link ID": key, "Title": title, "URL": "https://example.org", "Category": category, "Visible": "Yes", store.OrderColumn: order}
}

func TestTheLoadWantsAnEventsRow(t *testing.T) {
	tables := store.Tables{homeCategoriesTab: {categoryRow(schoolID, "School", "🏫", StyleCards, "4")}}
	if _, err := BuildHome(context.Background(), tables, testkit.All); err == nil || !strings.Contains(err.Error(), "no events section") || !strings.Contains(err.Error(), EventsCategoryID) {
		t.Errorf("a Categories tab without the events row built: %v", err)
	}
	tables[homeCategoriesTab] = append(tables[homeCategoriesTab], categoryRow(eventsID, "What's On", "🎉", StyleEvents, "8"))
	m, err := BuildHome(context.Background(), tables, testkit.All)
	if err != nil {
		t.Fatalf("build with an events row: %v", err)
	}
	if len(m.Categories) != 2 || m.Categories[1].Title != "What's On" || m.Categories[1].Style != StyleEvents || m.Categories[1].Order != "8" {
		t.Errorf("categories = %+v, want the sheet's own events row second", m.Categories)
	}
	tables[homeCategoriesTab] = append(tables[homeCategoriesTab], categoryRow(chatsID, "Another", "📅", StyleEvents, "c"))
	if _, err := BuildHome(context.Background(), tables, testkit.All); err == nil || !strings.Contains(err.Error(), "only one") {
		t.Errorf("two events rows built: %v", err)
	}
}

func TestTheLoadRefusesABlankOrder(t *testing.T) {
	good := func() store.Tables {
		return store.Tables{
			homeCategoriesTab: {eventsRow("2"), categoryRow(schoolID, "School", "", StyleCards, "4")},
			homeLinksTab:      {linkRow(directoryID, "Directory", schoolID, "2")},
			homeVisibilityTab: {{"App": "who", "Visibility": "everyone", store.OrderColumn: "2"}},
			homeWidgetsTab:    {{"Widget": "when", store.OrderColumn: "2"}},
		}
	}
	if _, err := BuildHome(context.Background(), good(), testkit.All); err != nil {
		t.Fatalf("every row keyed refused: %v", err)
	}
	for _, c := range []struct {
		tab  string
		row  int
		want string
	}{
		{homeCategoriesTab, 1, `category "School"`},
		{homeLinksTab, 0, `link "Directory"`},
		{homeVisibilityTab, 0, `"who"`},
		{homeWidgetsTab, 0, `widget "when"`},
	} {
		for _, order := range []string{"", "  ", "a b"} {
			tables := good()
			tables[c.tab][c.row][store.OrderColumn] = order
			_, err := BuildHome(context.Background(), tables, testkit.All)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("%s with order %q built: %v", c.tab, order, err)
			}
			if strings.TrimSpace(order) == "" && (err == nil || !strings.Contains(err.Error(), "has no order")) {
				t.Errorf("%s with a blank order: %v, want it named", c.tab, err)
			}
		}
		tables := good()
		delete(tables[c.tab][c.row], store.OrderColumn)
		if _, err := BuildHome(context.Background(), tables, testkit.All); err == nil || !strings.Contains(err.Error(), "has no order") {
			t.Errorf("%s with no order cell built: %v", c.tab, err)
		}
	}
}

func TestTheEventsSectionIsFoundByItsStyle(t *testing.T) {
	tables := store.Tables{homeCategoriesTab: {eventsRow("2"), categoryRow(schoolID, EventsCategoryTitle, "", StyleCards, "4")}}
	m, err := BuildHome(context.Background(), tables, testkit.All)
	if err != nil {
		t.Fatalf("a plain category titled as the events section refused: %v", err)
	}
	if events := m.category(EventsCategoryID); events == nil || events.Style != StyleEvents {
		t.Errorf("the events section = %+v", events)
	}
}

func TestLinksCannotNameTheEventsSection(t *testing.T) {
	tables := store.Tables{
		homeCategoriesTab: {categoryRow(eventsID, "What's On", "🎉", StyleEvents, "2")},
		homeLinksTab:      {linkRow(directoryID, "Gala", eventsID, "2")},
	}
	if _, err := BuildHome(context.Background(), tables, testkit.All); err == nil || !strings.Contains(err.Error(), "events") {
		t.Errorf("a link under the events section built: %v", err)
	}
}

func TestIDsAreCheckedAtLoad(t *testing.T) {
	school := categoryRow(schoolID, "School", "", StyleCards, "4")
	for _, c := range []struct {
		name   string
		tables store.Tables
		want   string
	}{
		{"a category without an id", store.Tables{homeCategoriesTab: {eventsRow("2"), categoryRow("", "School", "", StyleCards, "4")}}, "not an id"},
		{"a category id with an i", store.Tables{homeCategoriesTab: {eventsRow("2"), categoryRow("hcg000000000i", "School", "", StyleCards, "4")}}, "not an id"},
		{"two categories with one id", store.Tables{homeCategoriesTab: {eventsRow("2"), school, categoryRow(schoolID, "Clubs", "", StyleTiles, "6")}}, "used twice"},
		{"a link without an id", store.Tables{homeCategoriesTab: {eventsRow("2"), school}, homeLinksTab: {linkRow("", "Gala", schoolID, "2")}}, "not an id"},
		{"a link sharing a category's id", store.Tables{homeCategoriesTab: {eventsRow("2"), school}, homeLinksTab: {linkRow(schoolID, "Gala", schoolID, "2")}}, "used twice"},
		{"two links with one id", store.Tables{homeCategoriesTab: {eventsRow("2"), school}, homeLinksTab: {linkRow(directoryID, "Gala", schoolID, "2"), linkRow(directoryID, "Fair", schoolID, "4")}}, "used twice"},
		{"a link naming its category by title", store.Tables{homeCategoriesTab: {eventsRow("2"), school}, homeLinksTab: {linkRow(directoryID, "Gala", "School", "2")}}, "unknown category"},
	} {
		if _, err := BuildHome(context.Background(), c.tables, testkit.All); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s built: %v, want %q", c.name, err, c.want)
		}
	}
	tables := store.Tables{
		homeCategoriesTab: {eventsRow("2"), school, categoryRow(chatsID, "School", "", StyleTiles, "6")},
		homeLinksTab:      {linkRow(directoryID, "Gala", strings.ToUpper(schoolID), "2"), linkRow(parentPortalID, "Gala", chatsID, "2")},
	}
	m, err := BuildHome(context.Background(), tables, testkit.All)
	if err != nil {
		t.Fatalf("titles shared across rows refused: %v", err)
	}
	if got := m.category(schoolID).Links; len(got) != 1 || got[0].ID != directoryID || got[0].Category != schoolID {
		t.Errorf("school's links = %+v, want the one filed under its id in capitals", got)
	}
}
