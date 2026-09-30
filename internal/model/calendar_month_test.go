package model

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/testkit"
)

func daysOf(t *testing.T, h http.Handler, as, feed string) map[string][]DayKind {
	t.Helper()
	var f feedResource
	raw := decoded[envelope](t, testkit.Call(t, h, as, "GET", "/api/calendar-feeds/"+feed, nil))
	if err := json.Unmarshal(raw.Resources["calendar-feeds"][feed], &f); err != nil {
		t.Fatalf("no calendar %s: %v", feed, err)
	}
	return f.Days
}

func TestMonth(t *testing.T) {
	_, h := sampleEventsServer(t)
	mine := feedKey(sam, MyHeliosianToken)
	days := daysOf(t, h, sam, mine)
	if _, weekend := days["2026-09-13"]; weekend {
		t.Errorf("a Sunday is a school day: %v", days["2026-09-13"])
	}
	if k, ok := days["2026-09-14"]; !ok || k == nil || len(k) != 0 {
		t.Errorf("a regular Monday is no school day or has kinds: %v %v", ok, k)
	}
	if k := days["2026-09-07"]; len(k) != 1 || k[0].Name != "No School" || k[0].Words != "No School" {
		t.Errorf("Labor Day = %+v", k)
	}
	if k := days["2026-09-29"]; len(k) != 1 || k[0].Words != "Early Dismissal" {
		t.Errorf("conference day = %+v", k)
	}
	month := func(feed, from, to string) []map[string]any {
		return listOf(t, h, sam, "/api/events?from="+from+"&to="+to+"&calendar="+feed)
	}
	got := month(mine, "2026-09-01", "2026-09-30")
	titles := titlesOf(got)
	for i, e := range got {
		start, end := e["start"].(string), e["end"].(string)
		if i > 0 && got[i-1]["start"].(string) > start {
			t.Errorf("out of order: %q after %q", e["title"], got[i-1]["title"])
		}
		if start[:10] > "2026-09-30" || end[:10] < "2026-09-01" {
			t.Errorf("outside the month: %+v", e)
		}
		if dates, _ := e["dates"].([]any); len(dates) == 0 || dates[0] != start[:10] {
			t.Errorf("event carries no days to sit on: %+v", e)
		}
	}
	for _, want := range []string{"Labor Day - No School", "Jays and Ravens Camping", "Fondue & Fort Night", "Returning Grade ILP Conference, half days"} {
		if !slices.Contains(titles, want) {
			t.Errorf("month lacks %q: %v", want, titles)
		}
	}
	if slices.Contains(titles, "Hummingbird CAFE") || slices.Contains(titles, "MS Back to School Night") || slices.Contains(titles, "Back to School Social") {
		t.Errorf("month lists another classroom's, the middle school's, or last month's: %v", titles)
	}
	everything := write(t, h, sam, "POST", "/api/calendar-feeds", map[string]any{"name": "Everything", "classrooms": []string{}, "tags": []string{}})
	before := month(everything, "2026-08-01", "2026-08-31")
	if social := eventTitled(before, "Back to School Social"); social == nil || social["app"] != "team" {
		t.Errorf("August lacks the social: %v", titlesOf(before))
	}
	if k := daysOf(t, h, sam, everything)["2026-08-19"]; len(k) != 1 || k[0].Words != "Early Dismissal · Hummingbirds" {
		t.Errorf("kindergarten's short day on a calendar of every classroom = %+v", k)
	}
	if k := days["2026-08-19"]; len(k) != 0 {
		t.Errorf("a Jays student sees the kindergarten's short day: %+v", k)
	}
	for date := range days {
		if strings.HasPrefix(date, "2027-07") {
			t.Errorf("July is in the school calendar: %s", date)
		}
	}
	if summer := month(mine, "2027-07-01", "2027-07-31"); len(summer) != 0 {
		t.Errorf("July = %v", titlesOf(summer))
	}
}
