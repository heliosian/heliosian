package home

import (
	"strings"
	"testing"
)

type noImages struct{}

func (noImages) Has(string) (bool, error) { return true, nil }

func (noImages) Prefetch([]string) error { return nil }

func category(title, emoji, style string) map[string]string {
	return map[string]string{"Title": title, "Emoji": emoji, "Style": style}
}

// The events section is always on the page: synthesized at the top when the
// sheet has no row for it, the sheet's own row where the sheet puts it, and
// never more than one.
func TestEventsSectionIsSynthesizedUntilTheSheetHasIt(t *testing.T) {
	tables := &Tables{Categories: []map[string]string{category("School", "🏫", StyleCards)}}
	m, err := BuildModel(tables, noImages{})
	if err != nil {
		t.Fatalf("build without an events row: %v", err)
	}
	if len(m.Categories) != 2 || m.Categories[0].Title != EventsTitle || m.Categories[0].Style != StyleEvents || !m.Categories[0].Virtual {
		t.Errorf("categories = %+v, want the synthesized events section first", m.Categories)
	}

	tables.Categories = append(tables.Categories, category("What's On", "🎉", StyleEvents))
	m, err = BuildModel(tables, noImages{})
	if err != nil {
		t.Fatalf("build with an events row: %v", err)
	}
	if len(m.Categories) != 2 || m.Categories[1].Title != "What's On" || m.Categories[1].Virtual {
		t.Errorf("categories = %+v, want the sheet's own events row second and not virtual", m.Categories)
	}

	tables.Categories = append(tables.Categories, category("Another", "📅", StyleEvents))
	if _, err := BuildModel(tables, noImages{}); err == nil || !strings.Contains(err.Error(), "only one") {
		t.Errorf("two events rows built: %v", err)
	}
}

// A link cannot sit under the events section, which the portal fills.
func TestLinksCannotNameTheEventsSection(t *testing.T) {
	tables := &Tables{
		Categories: []map[string]string{category("What's On", "🎉", StyleEvents)},
		Links:      []map[string]string{{"Title": "Gala", "URL": "https://example.org", "Category": "What's On", "Visible": "Yes"}},
	}
	if _, err := BuildModel(tables, noImages{}); err == nil || !strings.Contains(err.Error(), "events") {
		t.Errorf("a link under the events section built: %v", err)
	}
}
