package db

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	feed, err := readFeed(context.Background(), svc, from, from.AddDate(3, 0, 0))
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
