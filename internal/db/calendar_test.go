package db

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestClassifyRequestListsEventsInOrderWithTheirHandles(t *testing.T) {
	items := []CalendarItem{}
	for i := range 11 {
		items = append(items, CalendarItem{Key: "k" + strconv.Itoa(i), Title: "Event " + strconv.Itoa(i)})
	}
	request, sent, err := classifyRequest(items)
	if err != nil {
		t.Fatal(err)
	}
	var listed []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(request[strings.Index(request, "["):]), &listed); err != nil {
		t.Fatalf("the request is not a list: %v", err)
	}
	for i, l := range listed {
		handle := "e" + strconv.Itoa(i+1)
		if l.ID != handle || l.Title != items[i].Title || sent[handle].Key != items[i].Key {
			t.Errorf("entry %d is %+v, sent as %+v, want %s with %q", i, l, sent[handle], handle, items[i].Title)
		}
	}
	if len(listed) != len(items) {
		t.Fatalf("%d listed, want %d", len(listed), len(items))
	}
}

func TestCounselingClassification(t *testing.T) {
	v := &Vocabulary{Counseling: "counseling", Events: []Category{{ID: "counseling", Title: "High School Planning"}}, Classrooms: []Classroom{
		{ID: "lower", Name: "Lower", Grades: []string{"3", "4"}},
		{ID: "middle", Name: "Middle", Grades: []string{"5", "6"}},
		{ID: "upper", Name: "Upper", Grades: []string{"7", "8"}},
	}}
	if !strings.Contains(v.classifierSystem(), `use category "High School Planning"`) {
		t.Fatal("classifier prompt lost the renamed counseling category")
	}
	for _, tc := range []struct {
		title      string
		exclude    bool
		classrooms []string
		want       int
	}{
		{`Khan Lab School "Fireside Chat" at Helios`, false, []string{"Lower", "Middle", "Upper"}, 2},
		{"8th Grade Parent Coffee: HS Game Plan and Decisions", false, []string{"Upper"}, 1},
		{"Alexander meeting", true, []string{"Upper"}, 1},
		{"BASSC Fall Meeting (Zoom)", true, []string{"Upper"}, 1},
	} {
		t.Run(tc.title, func(t *testing.T) {
			item := CalendarItem{Key: "key", Calendar: CounselingCategory, Title: tc.title, AllDay: true}
			answers, err := v.classified(map[string]CalendarItem{"e1": item}, map[string]classifyAnswer{"e1": {
				Title: tc.title, Exclude: tc.exclude, Category: "Misc", Classrooms: tc.classrooms, Who: "parents", DayType: "No School",
			}}, map[string]string{"High School Planning": "counseling", "Misc": "misc"})
			if err != nil {
				t.Fatal(err)
			}
			c := answers["key"]
			if c.Exclude != tc.exclude || c.Category != "counseling" || len(c.Classrooms) != tc.want || c.DayType != "" || c.Who != "parents" {
				t.Fatalf("classification = %+v", c)
			}
			if v.InputHash(item) == v.InputHash(CalendarItem{Key: "key", Title: tc.title, AllDay: true}) {
				t.Fatal("source missing from hash")
			}
		})
	}
}
