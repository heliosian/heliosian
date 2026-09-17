package calendarimport

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

	"heliosian/internal/calendar"
)

func timed(at string) *gcal.EventDateTime {
	return &gcal.EventDateTime{DateTime: at}
}

func day(on string) *gcal.EventDateTime {
	return &gcal.EventDateTime{Date: on}
}

func fakeCalendar(t *testing.T) (*gcal.Service, *[]string) {
	t.Helper()
	pages := map[string]gcal.Events{
		"": {
			NextPageToken: "second",
			Items: []*gcal.Event{
				{ICalUID: "abc@google.com", Summary: "Beacon Wellness Chat in the Library", Start: timed("2026-09-18T09:00:00-07:00"), End: timed("2026-09-18T10:00:00-07:00"), Updated: "2026-09-15T17:08:10.591Z", Sequence: 1, Description: "Bring snacks"},
				{ICalUID: "rec@google.com", RecurringEventId: "rec", OriginalStartTime: timed("2027-04-14T11:00:00-07:00"), Start: timed("2027-04-14T10:30:00-07:00"), End: timed("2027-04-14T15:15:00-07:00"), Updated: "2026-03-16T21:37:00Z", Sequence: 2, Summary: "Passion Projects"},
				{ICalUID: "gone@google.com", Status: "cancelled", Start: timed("2026-10-01T09:00:00-07:00"), End: timed("2026-10-01T10:00:00-07:00"), Updated: "2026-03-16T21:37:00Z"},
				{ICalUID: "early@google.com", Summary: "Before the window", Start: timed("2026-06-30T23:00:00-07:00"), End: timed("2026-07-01T01:00:00-07:00"), Updated: "2026-03-16T21:37:00Z"},
			},
		},
		"second": {
			Items: []*gcal.Event{
				{ICalUID: "day@google.com", Summary: "Thanksgiving Break - No School", Start: day("2026-11-23"), End: day("2026-11-28"), Updated: "2026-08-06T21:19:00Z"},
				{ICalUID: "r2@google.com", RecurringEventId: "r2", OriginalStartTime: day("2026-12-01"), Start: day("2026-12-01"), End: day("2026-12-02"), Updated: "2026-08-06T21:19:00Z"},
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

func TestFeedRowsKeepTheSheetsKeys(t *testing.T) {
	svc, queries := fakeCalendar(t)
	from := time.Date(2026, time.July, 1, 0, 0, 0, 0, calendar.Location)
	rows, err := feedRows(context.Background(), svc, from, from.AddDate(3, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(*queries) != 2 {
		t.Fatalf("%d requests, want 2", len(*queries))
	}
	want := []map[string]string{
		{"Key": "abc@google.com", "Title": "Beacon Wellness Chat in the Library", "Location": "", "Description": "Bring snacks", "Start": "2026-09-18 09:00", "End": "2026-09-18 10:00", "Updated": "2026-09-15 10:08", "Sequence": "1"},
		{"Key": "rec@google.com/20270414T110000", "Title": "Passion Projects", "Location": "", "Description": "", "Start": "2027-04-14 10:30", "End": "2027-04-14 15:15", "Updated": "2026-03-16 14:37", "Sequence": "2"},
		{"Key": "day@google.com", "Title": "Thanksgiving Break - No School", "Location": "", "Description": "", "Start": "2026-11-23", "End": "2026-11-27", "Updated": "2026-08-06 14:19", "Sequence": "0"},
		{"Key": "r2@google.com/20261201", "Title": "(untitled)", "Location": "", "Description": "", "Start": "2026-12-01", "End": "2026-12-01", "Updated": "2026-08-06 14:19", "Sequence": "0"},
	}
	if len(rows) != len(want) {
		t.Fatalf("%d rows, want %d: %v", len(rows), len(want), rows)
	}
	for i := range want {
		for column, value := range want[i] {
			if rows[i][column] != value {
				t.Errorf("row %d %s = %q, want %q", i, column, rows[i][column], value)
			}
		}
	}
}

func TestFeedRowsAskForSingleEventsInTheWindow(t *testing.T) {
	svc, queries := fakeCalendar(t)
	from := time.Date(2026, time.July, 1, 0, 0, 0, 0, calendar.Location)
	if _, err := feedRows(context.Background(), svc, from, from.AddDate(3, 0, 0)); err != nil {
		t.Fatal(err)
	}
	first, err := url.ParseQuery((*queries)[0])
	if err != nil {
		t.Fatal(err)
	}
	if first.Get("singleEvents") != "true" || first.Get("timeMin") != "2026-07-01T00:00:00-07:00" || first.Get("timeMax") != "2029-07-01T00:00:00-07:00" {
		t.Fatalf("first query %q", (*queries)[0])
	}
	second, err := url.ParseQuery((*queries)[1])
	if err != nil {
		t.Fatal(err)
	}
	if second.Get("pageToken") != "second" {
		t.Fatalf("second query %q", (*queries)[1])
	}
}
