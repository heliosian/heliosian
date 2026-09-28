package calendarimport

import (
	"strings"
	"testing"

	"heliosian/internal/cells"
	"heliosian/internal/store"
	"heliosian/internal/when"
)

func sampleRun(t *testing.T) (*run, *when.Model) {
	t.Helper()
	sheet, cache := sampleCache(t)
	tables := store.Tables{}
	for _, tab := range []string{when.DayTypesTab, when.TagsTab} {
		tables[tab] = rowsOf(t, sheet, tab)
	}
	r, err := vocabulary(cache.Model().Roster, tables)
	if err != nil {
		t.Fatal(err)
	}
	return r, cache.Model()
}

func TestVocabularyLeavesOutTheBuiltIns(t *testing.T) {
	r, _ := sampleRun(t)
	if len(r.tags) != 15 || r.tags[0].Name != "Schedule" || r.tags[0].ID != when.TagSchedule {
		t.Errorf("vocabulary = %+v", r.tags)
	}
	for _, tag := range r.tags {
		if when.BuiltInTag(tag.ID) {
			t.Errorf("a built-in tag is in the vocabulary: %+v", tag)
		}
	}
	system := classifierSystem(r.roster, r.tags)
	if !strings.Contains(system, "- Schedule: ") || strings.Contains(system, "- Misc: ") || strings.Contains(system, "- Going: ") {
		t.Errorf("the classifier's vocabulary:\n%s", system)
	}
	if strings.Join(r.dayTypes, ",") != "Regular,No Aftercare,Early Dismissal,No School" {
		t.Errorf("day types offered to Claude = %v", r.dayTypes)
	}
}

func TestImportWritesIDsForClaudesNames(t *testing.T) {
	r, m := sampleRun(t)
	jays, ravens, hummingbirds := m.Roster.IDOf("Jays"), m.Roster.IDOf("Ravens"), m.Roster.IDOf("Hummingbirds")
	row, err := r.enrichmentRow(enrichOutput{ID: "a13@sample", Tags: []string{"Jays", "Ravens", "Trip", "Parents"}, DayType: "Early Dismissal", Keywords: []string{"beach"}}, "h", "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	if want := cells.JoinList([]string{jays, ravens, "tag0000000107", "tag0000000007"}); row["Tags"] != want || row["Day Type"] != when.EarlyDismissalDayType || row["Keywords"] != "beach" {
		t.Errorf("enrichment row = %v, want tags %s", row, want)
	}
	if strings.Join(m.TagNames(cells.SplitList(row["Tags"])), ",") != "Jays,Ravens,Trip,Parents" {
		t.Errorf("the row's tags read back as %v", m.TagNames(cells.SplitList(row["Tags"])))
	}
	row, err = r.enrichmentRow(enrichOutput{ID: "a14@sample", Tags: []string{"Hummingbirds"}, DayType: noDayType}, "h", "2026-09-01")
	if _, has := row["Day Type"]; err != nil || has || row["Tags"] != hummingbirds {
		t.Errorf("a row with no day type = %v, %v", row, err)
	}
	if _, err := r.enrichmentRow(enrichOutput{ID: "a15@sample", Tags: []string{"Penguins"}, DayType: noDayType}, "h", "2026-09-01"); err == nil {
		t.Errorf("a name the sheet does not have was written")
	}
	extraction := pdfExtraction{
		Year: "2026-2027",
		Entries: []pdfEntry{
			{Title: "First Day of School", Start: "2026-08-18", End: "2026-08-18", DayType: noDayType, Classrooms: m.Roster.Names(), Marker: when.MarkerFirstDay},
			{Title: "Last Day (K only)", Start: "2027-06-04", End: "2027-06-04", DayType: "Early Dismissal", Classrooms: []string{"Hummingbirds"}, Marker: when.MarkerLastDay},
		},
		Shaded: []shadedDay{{Date: "2026-09-07", Legend: "Holiday", DayType: "No School"}},
	}
	rows, err := r.pdfRows(t.Context(), extraction, "hash")
	if err != nil {
		t.Fatal(err)
	}
	everyone := []string{}
	for _, c := range m.Roster.Classrooms {
		everyone = append(everyone, c.ID)
	}
	if len(rows) != 3 || rows[0]["Tags"] != cells.JoinList(everyone) || rows[0]["Day Type"] != "" {
		t.Errorf("first day row = %v", rows)
	}
	if rows[1]["Tags"] != hummingbirds || rows[1]["Day Type"] != when.EarlyDismissalDayType {
		t.Errorf("last day row = %v", rows[1])
	}
	if rows[2]["Tags"] != cells.JoinList(everyone) || rows[2]["Day Type"] != when.NoSchoolDayType || rows[2]["Title"] != "Holiday" {
		t.Errorf("shaded day row = %v", rows[2])
	}
}
