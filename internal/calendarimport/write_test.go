package calendarimport

import (
	"maps"
	"slices"
	"testing"

	"heliosian/internal/data"
	"heliosian/internal/model"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

func sampleStore(t *testing.T) (*data.Dir, *model.Store) {
	t.Helper()
	sheet := &data.Dir{Root: "../../sampledata"}
	s, err := model.NewStore(sheet, sheet, store.NewQueue(), model.Deps{IDKey: []byte("sample"), Static: testkit.None, Parties: testkit.All, Activities: testkit.All, Home: testkit.All})
	if err != nil {
		t.Fatal(err)
	}
	return sheet, s
}

func rowsOf(t *testing.T, sheet *data.Dir, tab string) []map[string]string {
	t.Helper()
	_, rows, err := sheet.Table("calendar", tab)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestIdentifyKeepsAKnownKeysEventIDAndMintsForANewOne(t *testing.T) {
	sheet, s := sampleStore(t)
	google := rowsOf(t, sheet, model.GoogleTab)
	rows := []map[string]string{{"Key": "a7@sample", "Title": "International Night"}, {"Key": "a13@sample", "Title": "Spring Picnic"}}
	(&run{opts: Options{Store: s}}).identify(rows, google)
	if rows[0]["Event ID"] != "gev0000000007" {
		t.Errorf("a known key's event id is %q", rows[0]["Event ID"])
	}
	if fresh := rows[1]["Event ID"]; fresh == "" || s.Model().Calendar.Event(fresh) != nil || fresh == rows[0]["Event ID"] {
		t.Errorf("a new key's event id is %q", fresh)
	}
}

func TestWriteCommitsOnlyWhatChanged(t *testing.T) {
	sheet, s := sampleStore(t)
	google, enrichment := rowsOf(t, sheet, model.GoogleTab), rowsOf(t, sheet, model.EnrichmentTab)
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
	rows = append(rows, map[string]string{"Key": "a13@sample", "Event ID": "gev0000000013", "Start": "2027-04-01", "End": "2027-04-01", "Title": "Spring Picnic", "Updated": "2026-09-01 09:00", "Sequence": "0"})
	jays, community := model.ClassroomID([]byte("sample"), "Jays"), "tag0000000105"
	enriched := append(slices.Clone(enrichment), map[string]string{"Event ID": "gev0000000013", "Tags": jays + ", " + community, "Input Hash": "h", "Model": modelName, "Enriched": "2026-09-01"})
	sync := []tabSync{
		{model.GoogleTab, model.GoogleColumns, rows, google, "Key", true},
		{model.EnrichmentTab, model.EnrichmentColumns, enriched, enrichment, "Event ID", false},
	}
	dry := &run{opts: Options{Store: s, DryRun: true}}
	if err := dry.write(t.Context(), sync); err != nil {
		t.Fatal(err)
	}
	if s.Model().Calendar.Event("a13@sample") != nil || len(rowsOf(t, sheet, store.ChangeLogTab)) != 0 {
		t.Fatal("a dry run committed")
	}
	r := &run{opts: Options{Store: s}}
	if err := r.write(t.Context(), sync); err != nil {
		t.Fatal(err)
	}
	m := s.Model().Calendar
	if m.Event("a6@sample").Title != "Hummingbird Coffee" || m.Event("a13@sample") == nil || !slices.Contains(m.Event("a13@sample").Tags, community) || m.TagName(community) != "Community" {
		t.Errorf("model after the import: a6 %+v, a13 %+v", m.Event("a6@sample"), m.Event("a13@sample"))
	}
	keys := []string{}
	for _, row := range rowsOf(t, sheet, model.GoogleTab) {
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
		"calendarimport|insert|Enrichment|Event ID=gev0000000013||",
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
	sync[0].before, sync[1].before = rowsOf(t, sheet, model.GoogleTab), rowsOf(t, sheet, model.EnrichmentTab)
	if err := r.write(t.Context(), sync); err != nil {
		t.Fatal(err)
	}
	if len(rowsOf(t, sheet, store.ChangeLogTab)) != before {
		t.Errorf("a second run over the same rows wrote something")
	}
}
