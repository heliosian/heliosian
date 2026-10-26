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
		out[name] = append([]map[string]string{}, m.Table(name).All()...)
	}
	return out
}

func feedWindow(now time.Time) (time.Time, time.Time) {
	start := SchoolYearStart(now) - 1
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

func readFeed(ctx context.Context, svc *gcal.Service, calendarID string, from, to time.Time) ([]GoogleEvent, error) {
	out := []GoogleEvent{}
	call := svc.Events.List(calendarID).Context(ctx).
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
			if calendarID == CounselingCalendarID {
				event.Calendar = CounselingCategory
				event.Key = CounselingCalendarID + "/" + event.Key
				if event.Series != "" {
					event.Series = CounselingCalendarID + "/" + event.Series
				}
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
	calendar *gcal.Service
	client   anthropic.Client
	token    string
	kick     wake
	mu       sync.Mutex
	channels map[string]*gcal.Channel
	excluded map[string]string
}

func NewCalendarWatcher(s *Store, calendar *gcal.Service, anthropicKey, token string) *CalendarWatcher {
	return &CalendarWatcher{s: s, calendar: calendar, client: anthropic.NewClient(option.WithAPIKey(anthropicKey)), token: token, channels: map[string]*gcal.Channel{}, excluded: map[string]string{}, kick: newWake()}
}

func (w *CalendarWatcher) Start() {
	go w.work()
	w.renew()
	w.kick.poke()
	go w.sweep()
}

func (w *CalendarWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for calendarID, ch := range w.channels {
		w.stop(ch)
		delete(w.channels, calendarID)
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
	feed := []GoogleEvent{}
	for _, calendarID := range []string{SchoolCalendarID, CounselingCalendarID} {
		events, err := readFeed(ctx, w.calendar, calendarID, from, to)
		if err != nil {
			return fmt.Errorf("read calendar %s: %w", calendarID, err)
		}
		feed = append(feed, events...)
	}
	if err := w.ensureCounselingCategory(ctx); err != nil {
		return err
	}
	rows := w.s.Model().calendarRows()
	v, err := NewVocabulary(rows)
	if err != nil {
		return err
	}
	pending := []CalendarItem{}
	awaiting := map[string]bool{}
	classified := map[string]Classification{}
	for _, it := range GoogleToClassify(rows, v, feed) {
		if w.excluded[it.Key] == v.InputHash(it) {
			classified[it.Key] = Classification{Exclude: true}
			continue
		}
		pending = append(pending, it)
		awaiting[it.Key] = true
	}
	slog.InfoContext(ctx, "calendar import: feed read", "events", len(feed), "to classify", len(pending))
	for key, c := range v.Classify(ctx, w.client, pending) {
		classified[key] = c
	}
	included := []GoogleEvent{}
	for _, e := range feed {
		c, answered := classified[e.Key]
		if c.Exclude {
			w.excluded[e.Key] = v.InputHash(e.CalendarItem)
			continue
		}
		if e.Calendar == CounselingCategory && awaiting[e.Key] && !answered {
			return fmt.Errorf("counseling event %q was not classified", e.Title)
		}
		delete(w.excluded, e.Key)
		included = append(included, e)
	}
	seen := map[string]bool{}
	for _, e := range feed {
		seen[e.Key] = true
	}
	for key := range w.excluded {
		if !seen[key] {
			delete(w.excluded, key)
		}
	}
	feed = included
	rows = w.s.Model().calendarRows()
	matched, err := v.MatchGoogle(ctx, w.client, rows, GoogleToMatch(rows, v, feed))
	if err != nil {
		return err
	}
	edits := GooglePlan(rows, v, feed, from, to, classified, matched)
	if len(edits) == 0 {
		return nil
	}
	env := Env{System: "calendar", Whole: true, Now: time.Now()}
	if _, err := Write(ctx, w.s, access.System(env.System), env, Batch{Batch: edits}); err != nil {
		return fmt.Errorf("write the calendar: %w", err)
	}
	slog.InfoContext(ctx, "calendar import: written", "edits", len(edits))
	return nil
}

func (w *CalendarWatcher) sweep() {
	for range time.Tick(sweepInterval) {
		w.renew()
		w.kick.poke()
	}
}

func expiry(ch *gcal.Channel) time.Time {
	return time.UnixMilli(ch.Expiration)
}

func (w *CalendarWatcher) renew() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, calendarID := range []string{SchoolCalendarID, CounselingCalendarID} {
		w.renewCalendar(calendarID)
	}
}

func (w *CalendarWatcher) renewCalendar(calendarID string) {
	if ch := w.channels[calendarID]; ch != nil && time.Until(expiry(ch)) > renewWithin {
		return
	}
	id := make([]byte, 16)
	rand.Read(id)
	ch, err := w.calendar.Events.Watch(calendarID, &gcal.Channel{
		Id: hex.EncodeToString(id), Type: "web_hook", Address: hookAddress, Token: w.token,
		Expiration: time.Now().Add(channelLife).UnixMilli(),
	}).Do()
	if err != nil {
		slog.Error("calendar watch", "error", err)
		return
	}
	slog.Info("calendar watch opened", "channel", ch.Id, "expires", expiry(ch).In(School).Format(MomentLayout))
	old := w.channels[calendarID]
	w.channels[calendarID] = ch
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
		w.kick.poke()
	}
	rw.WriteHeader(http.StatusOK)
}

func (w *CalendarWatcher) ensureCounselingCategory(ctx context.Context) error {
	everyone := ""
	for _, g := range w.s.Model().Table("GROUP").All() {
		if g["slug"] == "everyone" {
			everyone = g["id"]
		}
		if g["kind"] == "category" && g["slug"] == counselingSlug {
			return nil
		}
	}
	if everyone == "" {
		return fmt.Errorf("no everyone group")
	}
	env := Env{System: "calendar", Whole: true, Now: time.Now()}
	_, err := Write(ctx, w.s, access.System(env.System), env, Batch{Batch: []Edit{{Insert: "GROUP", Row: map[string]any{
		"kind": "category", "name": CounselingCategory, "slug": counselingSlug, "default": true,
		"description": "High school admissions, counseling, family presentations and deadlines for grades 5–8.",
		"status":      "open", "visible_to": everyone,
	}}}})
	return err
}
