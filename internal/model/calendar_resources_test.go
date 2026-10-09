package model

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/id"
	"heliosian/internal/testkit"
)

func served(mux *http.ServeMux, s *Store, hooks CalendarHooks) *http.ServeMux {
	typedRegistry(s, queue, DirectoryResources(), hooks.Resources(), MagicTagResources()).Register(mux)
	return mux
}

func feedKey(email, token string) string {
	return id.Of(sampleKey, kindCalendarFeed, email+"\x00"+token)
}

func settingsKey() string {
	return id.Of(sampleKey, kindSettings, "")
}

func decoded[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if rec.Code != http.StatusOK {
		t.Fatalf("read: %d %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

type envelope struct {
	Result    json.RawMessage                       `json:"result"`
	Resources map[string]map[string]json.RawMessage `json:"resources"`
}

func calendarOf(t *testing.T, h http.Handler) CalendarView {
	t.Helper()
	out := decoded[envelope](t, call(t, h, "POST", "/api/query", `{"events":{"path":"/api/events"},"settings":{"path":"/api/when-settings?include=feeds"}}`))
	var result map[string]json.RawMessage
	json.Unmarshal(out.Result, &result)
	ids, settingsIDs := []string{}, []string{}
	json.Unmarshal(result["events"], &ids)
	json.Unmarshal(result["settings"], &settingsIDs)
	var s settingsResource
	json.Unmarshal(out.Resources["when-settings"][settingsIDs[0]], &s)
	v := CalendarView{User: s.User, Today: s.Today, Classrooms: s.Classrooms, Tags: s.Tags, DayTypes: s.DayTypes, Events: []*Event{}, Feeds: []Feed{}}
	for _, key := range ids {
		var e eventResource
		json.Unmarshal(out.Resources["events"][key], &e)
		v.Events = append(v.Events, e.Event)
		if e.Provenance != nil {
			if v.Provenance == nil {
				v.Provenance = map[string]*Provenance{}
			}
			v.Provenance[key] = e.Provenance
		}
		if e.Responses != nil && (len(e.Responses.Yes)+len(e.Responses.Maybe)+len(e.Responses.No) > 0) {
			if v.Responses == nil {
				v.Responses = map[string]*Responses{}
			}
			v.Responses[key] = e.Responses
		}
	}
	var feeds struct {
		Feeds []string `json:"feeds"`
	}
	json.Unmarshal(out.Resources["when-settings"][settingsIDs[0]], &feeds)
	for _, key := range feeds.Feeds {
		var f feedResource
		json.Unmarshal(out.Resources["calendar-feeds"][key], &f)
		v.Feeds = append(v.Feeds, f.Feed)
	}
	return v
}

func feedOf(t *testing.T, h http.Handler, key string) feedResource {
	t.Helper()
	out := decoded[envelope](t, call(t, h, "GET", "/api/calendar-feeds/"+key, ""))
	var f feedResource
	json.Unmarshal(out.Resources["calendar-feeds"][key], &f)
	return f
}

func settingsPath(action string) string {
	return "/api/when-settings/" + settingsKey() + "/" + action
}

func eventOf(t *testing.T, h http.Handler, key string) *httptest.ResponseRecorder {
	t.Helper()
	return call(t, h, "GET", "/api/events/"+key, "")
}

func inviteView(t *testing.T, h http.Handler, key string) InviteView {
	t.Helper()
	return pickerOf(t, h, key).InviteView
}

func pickerOf(t *testing.T, h http.Handler, key string) guestListResource {
	t.Helper()
	out := decoded[envelope](t, call(t, h, "GET", "/api/events/"+key+"?include=guest-list", ""))
	var e struct {
		GuestList string `json:"guest-list"`
	}
	for _, raw := range out.Resources["events"] {
		json.Unmarshal(raw, &e)
	}
	var v guestListResource
	if err := json.Unmarshal(out.Resources["guest-lists"][e.GuestList], &v); err != nil {
		t.Fatalf("no guest list on %s: %s", key, out.Resources["events"])
	}
	return v
}

func settingsOf(t *testing.T, h http.Handler) settingsResource {
	t.Helper()
	out := decoded[envelope](t, call(t, h, "GET", "/api/when-settings/"+settingsKey(), ""))
	var s settingsResource
	if err := json.Unmarshal(out.Resources["when-settings"][settingsKey()], &s); err != nil {
		t.Fatalf("no settings: %v", err)
	}
	return s
}

func opened(t *testing.T, h http.Handler, eventID string) *httptest.ResponseRecorder {
	t.Helper()
	return call(t, h, "POST", "/api/guest-lists/"+id.Of(sampleKey, kindGuestList, eventID)+"/opened", "")
}

func act(t *testing.T, h http.Handler, key, action, body string) *httptest.ResponseRecorder {
	t.Helper()
	return call(t, h, "POST", "/api/events/"+key+"/"+action, body)
}

func created(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var made struct {
		ID string `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &made)
	return made.ID
}

type reply struct {
	Result    json.RawMessage                      `json:"result"`
	Resources map[string]map[string]map[string]any `json:"resources"`
}

func read(t *testing.T, h http.Handler, as, path string) reply {
	t.Helper()
	rec := testkit.Call(t, h, as, "GET", path, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body)
	}
	var out reply
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return out
}

func (r reply) one(t *testing.T, kind, key string) map[string]any {
	t.Helper()
	obj := r.Resources[kind][key]
	if obj == nil {
		t.Fatalf("no %s %s in %v", kind, key, r.Resources[kind])
	}
	return obj
}

func can(obj map[string]any, action string) bool {
	allowed, _ := obj["can"].(map[string]any)
	return allowed[action] == true
}

func write(t *testing.T, h http.Handler, as, method, path string, body any) string {
	t.Helper()
	rec := testkit.Call(t, h, as, method, path, body)
	if rec.Code >= 300 {
		t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body)
	}
	var made struct {
		ID string `json:"id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &made)
	return made.ID
}

func listOf(t *testing.T, h http.Handler, as, path string) []map[string]any {
	t.Helper()
	r := read(t, h, as, path)
	ids := []string{}
	if err := json.Unmarshal(r.Result, &ids); err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	kind, _, _ := strings.Cut(strings.TrimPrefix(path, "/api/"), "?")
	out := []map[string]any{}
	for _, key := range ids {
		out = append(out, r.one(t, kind, key))
	}
	return out
}

func defaultFeedOf(t *testing.T, h http.Handler, as string) string {
	t.Helper()
	r := read(t, h, as, "/api/when-settings/"+settingsKey()+"?include=feeds")
	feeds, _ := r.one(t, "when-settings", settingsKey())["feeds"].([]any)
	if len(feeds) == 0 {
		t.Fatalf("%s has no calendars", as)
	}
	return feeds[0].(string)
}

func upcomingOf(t *testing.T, h http.Handler, as, feed string) []map[string]any {
	t.Helper()
	return listOf(t, h, as, "/api/events?from="+now().Format(DateFormat)+"&calendar="+feed)
}

func answerOf(e map[string]any) string {
	me, _ := e["me"].(map[string]any)
	answer, _ := me["answer"].(string)
	return answer
}

func TestEventsAsResources(t *testing.T) {
	h, cache, kept := calendarInvitesApp(t)
	const meeting = "evt0000000002"
	list := read(t, h, host, "/api/events")
	ids := []string{}
	json.Unmarshal(list.Result, &ids)
	if !slices.Contains(ids, meeting) || !slices.Contains(ids, "gev0000000007") {
		t.Fatalf("the host's calendar lacks events: %v", ids)
	}
	mine := read(t, h, host, "/api/events/"+meeting+"?include=guest-list")
	e := mine.one(t, "events", meeting)
	if !can(e, "settings") || !can(e, "send") || e["title"] != "HCA Meeting" || e["path"] != "/e/"+meeting || e["app"] != "when" {
		t.Errorf("the host's event: %v", e)
	}
	list0 := mine.one(t, "guest-lists", e["guest-list"].(string))
	if list0["host"] != true {
		t.Errorf("the host's guest list: %v", list0)
	}
	theirs := read(t, h, robin, "/api/events/"+meeting)
	if e := theirs.one(t, "events", meeting); can(e, "settings") || can(e, "send") || !can(e, "answer") {
		t.Errorf("a guest's can on the host's event: %v", e["can"])
	}

	made := write(t, h, host, "POST", "/api/events", map[string]any{"title": "Meetup", "start": "2026-10-10 15:00", "tags": []string{}, "sharing": "Link", "address": "meetup"})
	if cache.Model().Calendar.Event(made) == nil || cache.Model().Calendar.AnswerOf(host, made) != AnswerYes {
		t.Fatalf("the new event %s: %+v, host's answer %q", made, cache.Model().Calendar.Event(made), cache.Model().Calendar.AnswerOf(host, made))
	}
	if rec := testkit.Call(t, h, robin, "POST", "/api/events/"+made+"/send", map[string]any{}); rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
		t.Errorf("a stranger sending another's invites: %d %s", rec.Code, rec.Body)
	}
	write(t, h, host, "POST", "/api/act", []map[string]any{
		{"method": "POST", "path": "/api/events/" + made + "/invite", "body": map[string]any{"people": []map[string]string{{"email": robin}}}},
		{"method": "POST", "path": "/api/events/" + made + "/send", "body": map[string]any{}},
	})
	if inv := cache.Model().Calendar.InviteOf(made, robin); inv == nil || inv.Requested == "" {
		t.Fatalf("robin after the batch: %+v", inv)
	}
	eventually(t, "robin is sent the invitation", func() bool { return len(mailTo(kept, robin)) == 1 })

	write(t, h, robin, "POST", "/api/events/"+made+"/answer", map[string]string{"answer": "yes"})
	if cache.Model().Calendar.AnswerOf(robin, made) != AnswerYes {
		t.Errorf("robin's answer = %q", cache.Model().Calendar.AnswerOf(robin, made))
	}

	before := len(changeLog(t))
	write(t, h, host, "POST", "/api/events/"+made+"/edit", map[string]string{"title": "Meetup!"})
	after := changeLog(t)[before:]
	if cache.Model().Calendar.Event(made).Title != "Meetup!" || len(after) != 1 || !strings.Contains(after[0], "|Title|Meetup") {
		t.Errorf("a title edit wrote %v", after)
	}

	feed := write(t, h, host, "POST", "/api/calendar-feeds", map[string]any{"name": "Trips", "classrooms": []string{"Jays"}, "tags": []string{}})
	settings := read(t, h, host, "/api/when-settings?include=feeds")
	var one []string
	json.Unmarshal(settings.Result, &one)
	if feeds := settings.one(t, "when-settings", one[0])["feeds"].([]any); !slices.Contains(feeds, any(feed)) {
		t.Errorf("the new feed %s is not among %v", feed, feeds)
	}
	write(t, h, host, "POST", "/api/calendar-feeds/"+feed+"/edit", map[string]string{"name": "Jays trips"})
	if got := read(t, h, host, "/api/calendar-feeds/"+feed).one(t, "calendar-feeds", feed); got["name"] != "Jays trips" || got["classrooms"].([]any)[0] != "Jays" {
		t.Errorf("the feed after renaming: %v", got)
	}
	if rec := testkit.Call(t, h, robin, "GET", "/api/calendar-feeds/"+feed, nil); rec.Code != http.StatusNotFound {
		t.Errorf("someone else's feed: %d", rec.Code)
	}
	write(t, h, host, "DELETE", "/api/calendar-feeds/"+feed, nil)
	if len(cache.Model().Calendar.MyCalendars(host)) != 2 {
		t.Errorf("calendars after deleting: %+v", cache.Model().Calendar.MyCalendars(host))
	}
}
