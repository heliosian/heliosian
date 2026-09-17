package calendarimport

import "testing"

func TestPlanKeepsCurrentRowsAndAsksAboutTheRest(t *testing.T) {
	vocabulary := "vocabulary"
	same := map[string]string{"Key": "same@google.com", "Title": "Same", "Start": "2026-09-18 09:00", "End": "2026-09-18 10:00"}
	moved := map[string]string{"Key": "moved@google.com", "Title": "Moved", "Start": "2026-09-19 09:00", "End": "2026-09-19 10:00"}
	fresh := map[string]string{"Key": "fresh@google.com", "Title": "Fresh", "Start": "2026-09-20", "End": "2026-09-20"}
	existing := map[string]map[string]string{
		"same@google.com":            {"Event ID": "same@google.com", "Tags": "Parents", "Input Hash": digest("Same", "", "2026-09-18 09:00", "2026-09-18 10:00", "", vocabulary)},
		"moved@google.com":           {"Event ID": "moved@google.com", "Tags": "Staff", "Input Hash": digest("Moved", "", "2026-09-18 09:00", "2026-09-18 10:00", "", vocabulary)},
		"2026-2026-09-07-labor-day":  {"Event ID": "2026-2026-09-07-labor-day", "Tags": "Schedule", "Input Hash": "pdf"},
		"gone@google.com":            {"Event ID": "gone@google.com", "Tags": "Community", "Input Hash": "gone"},
	}
	kept, pending, hashes := plan(existing, []map[string]string{same, moved, fresh}, vocabulary)
	if len(kept) != 1 || kept[0]["Event ID"] != "same@google.com" || kept[0]["Tags"] != "Parents" {
		t.Fatalf("kept %v", kept)
	}
	if len(pending) != 2 || pending[0].ID != "moved@google.com" || pending[1].ID != "fresh@google.com" {
		t.Fatalf("pending %v", pending)
	}
	if hashes["moved@google.com"] != digest("Moved", "", "2026-09-19 09:00", "2026-09-19 10:00", "", vocabulary) {
		t.Fatalf("hash %q", hashes["moved@google.com"])
	}
	if _, ok := hashes["same@google.com"]; ok {
		t.Fatal("hash for a kept row")
	}
}
