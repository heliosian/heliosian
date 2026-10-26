package db

import (
	"context"
	"encoding/json"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"heliosian/internal/testkit"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	gcal "google.golang.org/api/calendar/v3"
	gapi "google.golang.org/api/option"
)

func timed(at string) *gcal.EventDateTime {
	return &gcal.EventDateTime{DateTime: at}
}

func onDay(on string) *gcal.EventDateTime {
	return &gcal.EventDateTime{Date: on}
}

func fakeCalendar(t *testing.T) (*gcal.Service, *[]string) {
	t.Helper()
	pages := map[string]gcal.Events{
		"": {
			NextPageToken: "second",
			Items: []*gcal.Event{
				{ICalUID: "abc@google.com", Summary: "Beacon Wellness Chat in the Library", Start: timed("2026-09-18T09:00:00-07:00"), End: timed("2026-09-18T10:00:00-07:00"), Description: "<p>Bring <a href=\"https://example.org/snacks\">snacks</a></p>"},
				{ICalUID: "rec@google.com", RecurringEventId: "rec", OriginalStartTime: timed("2027-04-14T11:00:00-07:00"), Start: timed("2027-04-14T10:30:00-07:00"), End: timed("2027-04-14T15:15:00-07:00"), Summary: "Passion Projects"},
				{ICalUID: "gone@google.com", Status: "cancelled", Start: timed("2026-10-01T09:00:00-07:00"), End: timed("2026-10-01T10:00:00-07:00")},
				{ICalUID: "early@google.com", Summary: "Before the window", Start: timed("2026-06-30T23:00:00-07:00"), End: timed("2026-07-01T01:00:00-07:00")},
			},
		},
		"second": {
			Items: []*gcal.Event{
				{ICalUID: "day@google.com", Summary: "Thanksgiving Break - No School", Start: onDay("2026-11-23"), End: onDay("2026-11-28")},
				{ICalUID: "r2@google.com", RecurringEventId: "r2", OriginalStartTime: onDay("2026-12-01"), Start: onDay("2026-12-01"), End: onDay("2026-12-02")},
			},
		},
	}
	queries := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		page, ok := pages[r.URL.Query().Get("pageToken")]
		if !ok {
			http.Error(w, "no such page", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(page)
	}))
	t.Cleanup(server.Close)
	svc, err := gcal.NewService(context.Background(), gapi.WithEndpoint(server.URL+"/"), gapi.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	return svc, &queries
}

func TestReadFeed(t *testing.T) {
	svc, queries := fakeCalendar(t)
	from := time.Date(2026, time.July, 1, 0, 0, 0, 0, School)
	feed, err := readFeed(context.Background(), svc, SchoolCalendarID, from, from.AddDate(3, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	want := []GoogleEvent{
		{CalendarItem: CalendarItem{Key: "abc@google.com", Title: "Beacon Wellness Chat in the Library", Description: "Bring snacks https://example.org/snacks", Start: "2026-09-18 09:00", End: "2026-09-18 10:00"}},
		{Series: "rec@google.com", CalendarItem: CalendarItem{Key: "rec@google.com/20270414T110000", Title: "Passion Projects", Start: "2027-04-14 10:30", End: "2027-04-14 15:15"}},
		{CalendarItem: CalendarItem{Key: "day@google.com", Title: "Thanksgiving Break - No School", Start: "2026-11-23", End: "2026-11-27", AllDay: true}},
		{Series: "r2@google.com", CalendarItem: CalendarItem{Key: "r2@google.com/20261201", Title: "(untitled)", Start: "2026-12-01", End: "2026-12-01", AllDay: true}},
	}
	if len(feed) != len(want) {
		t.Fatalf("%d events, want %d: %+v", len(feed), len(want), feed)
	}
	for i := range want {
		if feed[i] != want[i] {
			t.Errorf("event %d = %+v, want %+v", i, feed[i], want[i])
		}
	}
	first, err := url.ParseQuery((*queries)[0])
	if err != nil {
		t.Fatal(err)
	}
	if first.Get("singleEvents") != "true" || first.Get("timeMin") != "2026-07-01T00:00:00-07:00" || first.Get("timeMax") != "2029-07-01T00:00:00-07:00" {
		t.Fatalf("first query %q", (*queries)[0])
	}
	if len(*queries) != 2 {
		t.Fatalf("%d requests, want 2", len(*queries))
	}
}

func post(w *CalendarWatcher, token, state string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", CalendarHookPath, nil)
	r.Header.Set("X-Goog-Channel-Token", token)
	r.Header.Set("X-Goog-Resource-State", state)
	rec := httptest.NewRecorder()
	w.ServeHTTP(rec, r)
	return rec
}

func TestCalendarHook(t *testing.T) {
	for _, c := range []struct {
		name, token, state string
		status             int
		kicked             bool
	}{
		{"another channel's token", "theirs", "exists", http.StatusForbidden, false},
		{"the opening post", "ours", "sync", http.StatusOK, false},
		{"a change", "ours", "exists", http.StatusOK, true},
	} {
		w := &CalendarWatcher{token: "ours", kick: make(chan struct{}, 1)}
		if rec := post(w, c.token, c.state); rec.Code != c.status {
			t.Errorf("%s: status %d", c.name, rec.Code)
		}
		if kicked := len(w.kick) == 1; kicked != c.kicked {
			t.Errorf("%s: kicked %v", c.name, kicked)
		}
	}
}

func TestCounselingFeedKeys(t *testing.T) {
	svc, _ := fakeCalendar(t)
	from := time.Date(2026, time.July, 1, 0, 0, 0, 0, School)
	feed, err := readFeed(context.Background(), svc, CounselingCalendarID, from, from.AddDate(3, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range feed {
		if e.Calendar != CounselingCategory || !strings.HasPrefix(e.Key, CounselingCalendarID+"/") {
			t.Fatalf("counseling event lost source identity: %+v", e)
		}
		if e.Series != "" && !strings.HasPrefix(e.Series, CounselingCalendarID+"/") {
			t.Fatalf("series lost source identity: %+v", e)
		}
	}
}

func TestCalendarWatchesBothFeeds(t *testing.T) {
	watches := map[string]int{}
	stopped := 0
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/channels/stop" {
			stopped++
			rw.WriteHeader(http.StatusNoContent)
			return
		}
		var ch gcal.Channel
		if err := json.NewDecoder(r.Body).Decode(&ch); err != nil {
			t.Error(err)
			return
		}
		watches[r.URL.Path]++
		ch.ResourceId = r.URL.Path
		json.NewEncoder(rw).Encode(ch)
	}))
	defer server.Close()
	svc, err := gcal.NewService(context.Background(), gapi.WithEndpoint(server.URL+"/"), gapi.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	w := NewCalendarWatcher(nil, svc, "unused", "token")
	w.renew()
	w.renew()
	if len(watches) != 2 || len(w.channels) != 2 {
		t.Fatalf("watches = %v; channels = %v", watches, w.channels)
	}
	for path, count := range watches {
		if count != 1 {
			t.Errorf("%s watched %d times", path, count)
		}
	}
	w.channels[CounselingCalendarID].Expiration = time.Now().UnixMilli()
	w.renew()
	if stopped != 1 {
		t.Fatalf("renewal stopped %d channels", stopped)
	}
	w.Stop()
	if stopped != 3 || len(w.channels) != 0 {
		t.Fatalf("stop left channels: stopped=%d, channels=%v", stopped, w.channels)
	}
}

func TestCounselingCategory(t *testing.T) {
	s, _, _ := calendarSample(t)
	w := NewCalendarWatcher(s, nil, "unused", "token")
	for range 2 {
		if err := w.ensureCounselingCategory(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	count := 0
	for _, g := range s.Model().Table("GROUP").All() {
		if g["name"] == CounselingCategory {
			count++
			if g["kind"] != "category" || g["slug"] != "high-school-counseling" || g["visible_to"] == "" || g["default"] != "Yes" {
				t.Fatalf("category: %v", g)
			}
		}
	}
	if count != 1 {
		t.Fatalf("%d counseling categories", count)
	}
}

func TestCounselingSync(t *testing.T) {
	s, _, _ := calendarSample(t)
	school := []*gcal.Event{{ICalUID: "shared@google", Summary: "School presentation", Start: timed("2026-11-04T18:00:00-08:00"), End: timed("2026-11-04T19:00:00-08:00")}}
	counseling := []*gcal.Event{
		{ICalUID: "shared@google", Summary: "High school options evening", Start: timed("2026-11-04T18:00:00-08:00"), End: timed("2026-11-04T19:00:00-08:00")},
		{ICalUID: "private@google", Summary: "Alexander meeting", Start: timed("2026-11-04T17:00:00-08:00"), End: timed("2026-11-04T17:30:00-08:00")},
	}
	categoryTitle := CounselingCategory
	failRead, failClassify, excludeCommunity := false, false, false
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/calendars/") {
			events := school
			if strings.Contains(r.URL.Path, CounselingCalendarID) {
				if failRead {
					http.Error(rw, "calendar unavailable", http.StatusForbidden)
					return
				}
				events = counseling
			}
			json.NewEncoder(rw).Encode(gcal.Events{Items: events})
			return
		}
		calls++
		body, _ := io.ReadAll(r.Body)
		text := testkit.UserText(string(body))
		var items []struct{ ID, Title, Calendar string }
		if err := json.Unmarshal([]byte(text[strings.Index(text, "["):]), &items); err != nil {
			t.Error(err)
			return
		}
		answer := map[string]classifyAnswer{}
		for _, it := range items {
			if failClassify {
				continue
			}
			answer[it.ID] = classifyAnswer{Title: it.Title, Category: categoryTitle, Who: "families", Classrooms: []string{"Condors", "Ospreys", "Herons", "Egrets"}, DayType: noDayType, Exclude: it.Calendar == CounselingCategory && (it.Title == "Alexander meeting" || excludeCommunity)}
		}
		raw, _ := json.Marshal(answer)
		testkit.ClaudeStream(rw, string(raw), "end_turn")
	}))
	defer server.Close()
	svc, err := gcal.NewService(context.Background(), gapi.WithEndpoint(server.URL+"/"), gapi.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	w := NewCalendarWatcher(s, svc, "unused", "token")
	w.client = anthropic.NewClient(option.WithAPIKey("unused"), option.WithBaseURL(server.URL))
	if err := w.sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	counselingKey := CounselingCalendarID + "/shared@google"
	if len(sourcesKeyed(s.Model(), "shared@google")) != 1 || len(sourcesKeyed(s.Model(), counselingKey)) != 1 || len(sourcesKeyed(s.Model(), CounselingCalendarID+"/private@google")) != 0 {
		t.Fatal("sources did not isolate calendars or exclude appointment")
	}
	group := sourcesKeyed(s.Model(), counselingKey)[0]["group"]
	g, _ := s.Model().Table("GROUP").Get(group)
	if len(s.Model().Table("RULE").Referencing("group", g["audience"])) != 4 {
		t.Fatalf("audience: %v", g)
	}
	firstCalls := calls
	if err := w.sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != firstCalls {
		t.Fatal("unchanged appointment was classified again")
	}
	category := g["parent"]
	categoryTitle = "High School Planning"
	apply(t, s, []Edit{{Set: category, Cells: map[string]any{"name": "High School Planning"}}})
	if err := w.sync(context.Background()); err != nil {
		t.Fatalf("sync after category rename: %v", err)
	}
	if current, _ := s.Model().Table("GROUP").Get(group); current["parent"] != category {
		t.Fatalf("rename lost category identity: %v", current)
	}
	categories := 0
	for _, category := range s.Model().Table("GROUP").All() {
		if category["slug"] == counselingSlug {
			categories++
		}
	}
	if categories != 1 {
		t.Fatalf("rename made %d counseling categories", categories)
	}
	counseling[0].Description = "changed"
	failClassify = true
	if err := w.sync(context.Background()); err == nil {
		t.Fatal("failed classification accepted")
	}
	if !s.Model().Has(group) {
		t.Fatal("classification failure removed existing event")
	}
	failClassify = false
	excludeCommunity = true
	if err := w.sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Model().Has(group) || len(sourcesKeyed(s.Model(), "shared@google")) != 1 {
		t.Fatal("exclusion did not remove only the counseling event")
	}
	school = nil
	failRead = true
	if err := w.sync(context.Background()); err == nil {
		t.Fatal("failed calendar read accepted")
	}
	if len(sourcesKeyed(s.Model(), "shared@google")) != 1 {
		t.Fatal("failed read removed school event")
	}
}
