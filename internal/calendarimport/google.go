package calendarimport

import (
	"context"
	"fmt"
	"strconv"
	"time"

	gcal "google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"

	"heliosian/internal/model"
)

func window(now time.Time) (time.Time, time.Time) {
	start := now.Year() - 1
	if now.Month() < time.July {
		start--
	}
	from := time.Date(start, time.July, 1, 0, 0, 0, 0, model.Location)
	return from, from.AddDate(3, 0, 0)
}

const feedFields = googleapi.Field("nextPageToken,items(iCalUID,recurringEventId,originalStartTime,start,end,summary,location,description,updated,sequence,status)")

func feedRows(ctx context.Context, svc *gcal.Service, from, to time.Time) ([]map[string]string, error) {
	rows := []map[string]string{}
	call := svc.Events.List(model.SchoolCalendarID).Context(ctx).
		SingleEvents(true).OrderBy("startTime").MaxResults(2500).
		TimeMin(from.Format(time.RFC3339)).TimeMax(to.Format(time.RFC3339)).
		Fields(feedFields)
	for {
		page, err := call.Do()
		if err != nil {
			return nil, err
		}
		for _, e := range page.Items {
			row, start, err := feedRow(e)
			if err != nil {
				return nil, err
			}
			if e.Status == "cancelled" || start.Before(from) || !start.Before(to) {
				continue
			}
			rows = append(rows, row)
		}
		if page.NextPageToken == "" {
			return rows, nil
		}
		call.PageToken(page.NextPageToken)
	}
}

func eventTime(t *gcal.EventDateTime) (time.Time, bool, error) {
	if t == nil {
		return time.Time{}, false, fmt.Errorf("event with no time")
	}
	if t.Date != "" {
		day, err := time.ParseInLocation(model.DateFormat, t.Date, model.Location)
		return day, true, err
	}
	at, err := time.Parse(time.RFC3339, t.DateTime)
	return at.In(model.Location), false, err
}

func instanceKey(uid string, t time.Time, allDay bool) string {
	if allDay {
		return uid + "/" + t.Format("20060102")
	}
	return uid + "/" + t.Format("20060102T150405")
}

func feedRow(e *gcal.Event) (map[string]string, time.Time, error) {
	key := e.ICalUID
	if e.RecurringEventId != "" {
		orig, origAllDay, err := eventTime(e.OriginalStartTime)
		if err != nil {
			return nil, time.Time{}, fmt.Errorf("event %s original start: %w", key, err)
		}
		key = instanceKey(key, orig, origAllDay)
	}
	start, allDay, err := eventTime(e.Start)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("event %s start: %w", key, err)
	}
	end, _, err := eventTime(e.End)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("event %s end: %w", key, err)
	}
	updated, err := time.Parse(time.RFC3339, e.Updated)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("event %s updated: %w", key, err)
	}
	description, err := flatten(e.Description)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("event %s description: %w", key, err)
	}
	row := map[string]string{
		"Key":         key,
		"Title":       collapse(e.Summary),
		"Location":    collapse(e.Location),
		"Description": description,
		"Updated":     updated.In(model.Location).Format(model.DateTimeFormat),
		"Sequence":    strconv.FormatInt(e.Sequence, 10),
	}
	if allDay {
		end = end.AddDate(0, 0, -1)
		if end.Before(start) {
			end = start
		}
		row["Start"], row["End"] = start.Format(model.DateFormat), end.Format(model.DateFormat)
	} else {
		row["Start"], row["End"] = start.Format(model.DateTimeFormat), end.Format(model.DateTimeFormat)
	}
	if row["Title"] == "" {
		row["Title"] = "(untitled)"
	}
	return row, start, nil
}
