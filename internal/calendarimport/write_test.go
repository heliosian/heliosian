package calendarimport

import (
	"maps"
	"slices"
	"testing"

	"heliosian/internal/calendar"
	"heliosian/internal/data"
	"heliosian/internal/store"
)

type syncQueue struct{}

func (syncQueue) Add(f func()) { f() }

func sampleCache(t *testing.T) (*data.Dir, *calendar.Cache) {
	t.Helper()
	sheet := &data.Dir{Root: "../../sampledata"}
	roster := calendar.Roster{}
	for _, name := range []string{"Hummingbirds", "Hawks", "Falcons", "Jays", "Ravens", "Condors", "Ospreys", "Egrets", "Herons"} {
		roster.Classrooms = append(roster.Classrooms, calendar.Classroom{Name: name})
	}
	cache, err := calendar.NewCache(sheet, sheet, func() calendar.Roster { return roster }, nil, func(string) bool { return false }, syncQueue{})
	if err != nil {
		t.Fatal(err)
	}
	return sheet, cache
}

func rowsOf(t *testing.T, sheet *data.Dir, tab string) []map[string]string {
	t.Helper()
	_, rows, err := sheet.Table("calendar", tab)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestWriteCommitsOnlyWhatChanged(t *testing.T) {
	sheet, cache := sampleCache(t)
	google, enrichment := rowsOf(t, sheet, calendar.GoogleTab), rowsOf(t, sheet, calendar.EnrichmentTab)
	rows := []map[string]string{}
	for _, row := range google {
		switch row["Key"] {
		case "a12@sample":
			continue
		case "a6@sample":
			row = maps.Clone(row)
			row["Title"] = "Hummingbird Coffee"
		}
		rows = append(rows, row)
	}
	rows = append(rows, map[string]string{"Key": "a13@sample", "Start": "2027-04-01", "End": "2027-04-01", "Title": "Spring Picnic", "Updated": "2026-09-01 09:00", "Sequence": "0"})
	enriched := append(slices.Clone(enrichment), map[string]string{"Event ID": "a13@sample", "Tags": "Jays, Community", "Input Hash": "h", "Model": modelName, "Enriched": "2026-09-01"})
	sync := []tabSync{
		{calendar.GoogleTab, calendar.GoogleColumns, rows, google, "Key", true},
		{calendar.EnrichmentTab, calendar.EnrichmentColumns, enriched, enrichment, "Event ID", false},
	}
	dry := &run{opts: Options{Cache: cache, DryRun: true}}
	if err := dry.write(sync); err != nil {
		t.Fatal(err)
	}
	if cache.Model().Event("a13@sample") != nil || len(rowsOf(t, sheet, store.ChangeLogTab)) != 0 {
		t.Fatal("a dry run committed")
	}
	r := &run{opts: Options{Cache: cache}}
	if err := r.write(sync); err != nil {
		t.Fatal(err)
	}
	m := cache.Model()
	if m.Event("a6@sample").Title != "Hummingbird Coffee" || m.Event("a13@sample") == nil || !slices.Contains(m.Event("a13@sample").Tags, "Community") {
		t.Errorf("model after the import: a6 %+v, a13 %+v", m.Event("a6@sample"), m.Event("a13@sample"))
	}
	keys := []string{}
	for _, row := range rowsOf(t, sheet, calendar.GoogleTab) {
		keys = append(keys, row["Key"]+"="+row["Title"])
	}
	if slices.ContainsFunc(keys, func(k string) bool { return k == "a12@sample=Spring Celebration" }) || !slices.Contains(keys, "a6@sample=Hummingbird Coffee") || !slices.Contains(keys, "a13@sample=Spring Picnic") || len(keys) != len(google) {
		t.Errorf("the sheet after the import: %v", keys)
	}
	log := []string{}
	for _, row := range rowsOf(t, sheet, store.ChangeLogTab) {
		log = append(log, row["Actor"]+"|"+row["Action"]+"|"+row["Tab"]+"|"+row["Key"]+"|"+row["Column"]+"|"+row["Previous"])
	}
	for _, want := range []string{
		"calendarimport|set|Google Import|Key=a6@sample|Title|Hummingbird CAFE",
		"calendarimport|insert|Google Import|Key=a13@sample||",
		"calendarimport|delete|Google Import|Key=a12@sample|Title|Spring Celebration",
		"calendarimport|insert|Enrichment|Event ID=a13@sample||",
	} {
		if !slices.Contains(log, want) {
			t.Errorf("the change log lacks %s: %v", want, log)
		}
	}
	for _, line := range log {
		if line == "calendarimport|set|Google Import|Key=a7@sample|Title|International Night" {
			t.Errorf("an unchanged row was written: %s", line)
		}
	}
	before := len(log)
	sync[0].before, sync[1].before = rowsOf(t, sheet, calendar.GoogleTab), rowsOf(t, sheet, calendar.EnrichmentTab)
	if err := r.write(sync); err != nil {
		t.Fatal(err)
	}
	if len(rowsOf(t, sheet, store.ChangeLogTab)) != before {
		t.Errorf("a second run over the same rows wrote something")
	}
}
