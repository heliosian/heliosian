package home

import (
	"context"
	"strings"
	"testing"

	"heliosian/internal/store"
)

type noImages struct{}

func (noImages) Has(string) (bool, error) { return true, nil }

func (noImages) Prefetch(context.Context, []string) error { return nil }

func category(title, emoji, style string) store.Row {
	return store.Row{"Title": title, "Emoji": emoji, "Style": style}
}

func TestEventsSectionIsSynthesizedUntilTheSheetHasIt(t *testing.T) {
	tables := store.Tables{categoriesTab: {category("School", "🏫", StyleCards)}}
	m, err := BuildModel(context.Background(), tables, noImages{})
	if err != nil {
		t.Fatalf("build without an events row: %v", err)
	}
	if len(m.Categories) != 2 || m.Categories[0].Title != EventsTitle || m.Categories[0].Style != StyleEvents || !m.Categories[0].Virtual {
		t.Errorf("categories = %+v, want the synthesized events section first", m.Categories)
	}

	tables[categoriesTab] = append(tables[categoriesTab], category("What's On", "🎉", StyleEvents))
	m, err = BuildModel(context.Background(), tables, noImages{})
	if err != nil {
		t.Fatalf("build with an events row: %v", err)
	}
	if len(m.Categories) != 2 || m.Categories[1].Title != "What's On" || m.Categories[1].Virtual {
		t.Errorf("categories = %+v, want the sheet's own events row second and not virtual", m.Categories)
	}

	tables[categoriesTab] = append(tables[categoriesTab], category("Another", "📅", StyleEvents))
	if _, err := BuildModel(context.Background(), tables, noImages{}); err == nil || !strings.Contains(err.Error(), "only one") {
		t.Errorf("two events rows built: %v", err)
	}
}

func TestLinksCannotNameTheEventsSection(t *testing.T) {
	tables := store.Tables{
		categoriesTab: {category("What's On", "🎉", StyleEvents)},
		linksTab:      {{"Title": "Gala", "URL": "https://example.org", "Category": "What's On", "Visible": "Yes"}},
	}
	if _, err := BuildModel(context.Background(), tables, noImages{}); err == nil || !strings.Contains(err.Error(), "events") {
		t.Errorf("a link under the events section built: %v", err)
	}
}
