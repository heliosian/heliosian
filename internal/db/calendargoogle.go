package db

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"golang.org/x/net/html"
	htmlatom "golang.org/x/net/html/atom"
	gcal "google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

const (
	CalendarHookPath = "/hooks/calendar"
	hookAddress      = "https://when.heliosian.com" + CalendarHookPath
	sweepInterval    = time.Hour
	renewWithin      = 2 * time.Hour
	channelLife      = 7 * 24 * time.Hour
	feedFields       = googleapi.Field("nextPageToken,items(iCalUID,recurringEventId,originalStartTime,start,end,summary,location,description,status)")
)

var calendarTables = []string{"GROUP", "GROUP_SOURCE", "RULE", "MEMBER", "DOCUMENT_GROUP", "PERSON", "ALIAS"}

func (m *Model) calendarRows() CalendarRows {
	out := CalendarRows{}
	for _, name := range calendarTables {
		rows := []map[string]string{}
		for _, row := range m.Table(name).All() {
			rows = append(rows, row)
		}
		out[name] = rows
	}
	return out
}

func feedWindow(now time.Time) (time.Time, time.Time) {
	start := now.Year() - 1
	if now.Month() < time.July {
		start--
	}
	from := time.Date(start, time.July, 1, 0, 0, 0, 0, School)
	return from, from.AddDate(3, 0, 0)
}

func eventTime(t *gcal.EventDateTime) (time.Time, bool, error) {
	if t == nil {
		return time.Time{}, false, fmt.Errorf("event with no time")
	}
	if t.Date != "" {
		day, err := time.ParseInLocation(DateLayout, t.Date, School)
		return day, true, err
	}
	at, err := time.Parse(time.RFC3339, t.DateTime)
	return at.In(School), false, err
}

func instanceKey(uid string, t time.Time, allDay bool) string {
	if allDay {
		return uid + "/" + t.Format("20060102")
	}
	return uid + "/" + t.Format("20060102T150405")
}

func googleEvent(e *gcal.Event) (GoogleEvent, time.Time, error) {
	key := e.ICalUID
	series := ""
	if e.RecurringEventId != "" {
		orig, origAllDay, err := eventTime(e.OriginalStartTime)
		if err != nil {
			return GoogleEvent{}, time.Time{}, fmt.Errorf("event %s original start: %w", key, err)
		}
		series = e.ICalUID
		key = instanceKey(key, orig, origAllDay)
	}
	start, allDay, err := eventTime(e.Start)
	if err != nil {
		return GoogleEvent{}, time.Time{}, fmt.Errorf("event %s start: %w", key, err)
	}
	end, _, err := eventTime(e.End)
	if err != nil {
		return GoogleEvent{}, time.Time{}, fmt.Errorf("event %s end: %w", key, err)
	}
	description, err := flatten(e.Description)
	if err != nil {
		return GoogleEvent{}, time.Time{}, fmt.Errorf("event %s description: %w", key, err)
	}
	out := GoogleEvent{Series: series, CalendarItem: CalendarItem{Key: key, Title: Collapse(e.Summary), Location: Collapse(e.Location), Description: description, AllDay: allDay}}
	if allDay {
		end = end.AddDate(0, 0, -1)
		if end.Before(start) {
			end = start
		}
		out.Start, out.End = start.Format(DateLayout), end.Format(DateLayout)
	} else {
		out.Start, out.End = start.Format(MomentLayout), end.Format(MomentLayout)
	}
	if out.Title == "" {
		out.Title = "(untitled)"
	}
	return out, start, nil
}

func readFeed(ctx context.Context, svc *gcal.Service, from, to time.Time) ([]GoogleEvent, error) {
	out := []GoogleEvent{}
	call := svc.Events.List(SchoolCalendarID).Context(ctx).
		SingleEvents(true).OrderBy("startTime").MaxResults(2500).
		TimeMin(from.Format(time.RFC3339)).TimeMax(to.Format(time.RFC3339)).
		Fields(feedFields)
	for {
		page, err := call.Do()
		if err != nil {
			return nil, err
		}
		for _, e := range page.Items {
			event, start, err := googleEvent(e)
			if err != nil {
				return nil, err
			}
			if e.Status == "cancelled" || start.Before(from) || !start.Before(to) {
				continue
			}
			out = append(out, event)
		}
		if page.NextPageToken == "" {
			return out, nil
		}
		call.PageToken(page.NextPageToken)
	}
}

const lineBreak = "\x00"

var blockTags = map[string]bool{
	"p": true, "div": true, "li": true, "blockquote": true, "tr": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

func flatten(text string) (string, error) {
	if strings.Contains(text, "<") {
		parent := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: htmlatom.Div}
		nodes, err := html.ParseFragment(strings.NewReader(text), parent)
		if err != nil {
			return "", err
		}
		out := &strings.Builder{}
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.TextNode {
				out.WriteString(n.Data)
			}
			if n.Type == html.ElementNode && n.Data == "br" {
				out.WriteString(lineBreak)
			}
			before := out.Len()
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
			if n.Type == html.ElementNode && n.Data == "a" {
				for _, attr := range n.Attr {
					if attr.Key == "href" && attr.Val != "" && !strings.Contains(out.String()[before:], attr.Val) {
						out.WriteString(" " + attr.Val)
					}
				}
			}
			if n.Type == html.ElementNode && blockTags[n.Data] {
				out.WriteString(lineBreak)
			}
		}
		for _, n := range nodes {
			walk(n)
		}
		text = strings.ReplaceAll(out.String(), lineBreak, "\n")
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = Collapse(line)
	}
	flat := strings.Join(lines, "\n")
	for strings.Contains(flat, "\n\n\n") {
		flat = strings.ReplaceAll(flat, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(flat), nil
}

type CalendarWatcher struct {
	s        *Store
	queue    *store.Queue
	pics     *Pictures
	calendar *gcal.Service
	client   anthropic.Client
	token    string
	kick     chan struct{}
	mu       sync.Mutex
	channel  *gcal.Channel
}

func NewCalendarWatcher(s *Store, queue *store.Queue, pics *Pictures, calendar *gcal.Service, anthropicKey, token string) *CalendarWatcher {
	return &CalendarWatcher{s: s, queue: queue, pics: pics, calendar: calendar, client: anthropic.NewClient(option.WithAPIKey(anthropicKey)), token: token, kick: make(chan struct{}, 1)}
}

func (w *CalendarWatcher) Start() {
	go w.work()
	w.renew()
	w.Kick()
	go w.sweep()
}

func (w *CalendarWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.channel == nil {
		return
	}
	w.stop(w.channel)
	w.channel = nil
}

func (w *CalendarWatcher) Kick() {
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

func (w *CalendarWatcher) work() {
	for range w.kick {
		start := time.Now()
		if err := w.sync(context.Background()); err != nil {
			slog.Error("calendar import", "error", err, "took", time.Since(start).Round(time.Millisecond))
			continue
		}
		slog.Info("calendar import", "took", time.Since(start).Round(time.Millisecond))
	}
}

func (w *CalendarWatcher) sync(ctx context.Context) error {
	from, to := feedWindow(time.Now().In(School))
	feed, err := readFeed(ctx, w.calendar, from, to)
	if err != nil {
		return fmt.Errorf("read the school calendar: %w", err)
	}
	rows := w.s.Model().calendarRows()
	v, err := NewVocabulary(rows)
	if err != nil {
		return err
	}
	pending := GoogleToClassify(rows, v, feed)
	slog.InfoContext(ctx, "calendar import: feed read", "events", len(feed), "to classify", len(pending))
	classified := v.Classify(ctx, w.client, pending)
	rows = w.s.Model().calendarRows()
	edits := GooglePlan(rows, v, feed, from, to, classified)
	if len(edits) == 0 {
		return nil
	}
	env := Env{System: importReader, Now: time.Now()}
	if _, err := Write(ctx, w.s, w.queue, w.pics, access.System(importReader), env, Batch{Batch: edits}); err != nil {
		return fmt.Errorf("write the calendar: %w", err)
	}
	slog.InfoContext(ctx, "calendar import: written", "edits", len(edits))
	return nil
}

func (w *CalendarWatcher) sweep() {
	for range time.Tick(sweepInterval) {
		w.renew()
		w.Kick()
	}
}

func expiry(ch *gcal.Channel) time.Time {
	return time.UnixMilli(ch.Expiration)
}

func (w *CalendarWatcher) renew() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.channel != nil && time.Until(expiry(w.channel)) > renewWithin {
		return
	}
	id := make([]byte, 16)
	rand.Read(id)
	ch, err := w.calendar.Events.Watch(SchoolCalendarID, &gcal.Channel{
		Id: hex.EncodeToString(id), Type: "web_hook", Address: hookAddress, Token: w.token,
		Expiration: time.Now().Add(channelLife).UnixMilli(),
	}).Do()
	if err != nil {
		slog.Error("calendar watch", "error", err)
		return
	}
	slog.Info("calendar watch opened", "channel", ch.Id, "expires", expiry(ch).In(School).Format(MomentLayout))
	old := w.channel
	w.channel = ch
	if old != nil {
		w.stop(old)
	}
}

func (w *CalendarWatcher) stop(ch *gcal.Channel) {
	if err := w.calendar.Channels.Stop(&gcal.Channel{Id: ch.Id, ResourceId: ch.ResourceId}).Do(); err != nil {
		slog.Error("calendar watch stop", "channel", ch.Id, "error", err)
		return
	}
	slog.Info("calendar watch closed", "channel", ch.Id)
}

func (w *CalendarWatcher) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Goog-Channel-Token")), []byte(w.token)) != 1 {
		http.Error(rw, "unknown channel", http.StatusForbidden)
		return
	}
	state := r.Header.Get("X-Goog-Resource-State")
	slog.InfoContext(r.Context(), "calendar changed", "channel", r.Header.Get("X-Goog-Channel-ID"), "state", state, "number", r.Header.Get("X-Goog-Message-Number"))
	if state != "sync" {
		w.Kick()
	}
	rw.WriteHeader(http.StatusOK)
}
