package when

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/api"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
	"heliosian/internal/testkit/mailtest"
	"heliosian/internal/who"
)

func apiApp(t *testing.T) (http.Handler, *Cache, *mailtest.Recorder) {
	t.Helper()
	mux, cache, kept, _ := invitesAppWith(t)
	sources := func(email string, _ time.Time) []Linked { return testDeps.Linked(email) }
	world := func(tx *store.Tx) World {
		return testHooks.World(cache.In(tx), testDirectory, noSettings(), sampleKey, sources)
	}
	reg := api.New(api.Config[World]{
		Actor:  func(r *http.Request, w World) access.Actor { return w.Directory.Actor(r, cache.Held) },
		Held:   cache.Held,
		Now:    func() time.Time { return now() },
		Queue:  queue,
		Staged: world,
		Scope:  func(w World, q api.Query) World { return w.At(q.Now) },
	})
	for _, rt := range who.Resources() {
		reg.Add(api.Lift(rt, func(w World) *who.Model { return w.Directory }))
	}
	for _, rt := range testHooks.Resources() {
		reg.Add(rt)
	}
	queue.OnSwap(func() { reg.Publish(world(nil)) })
	api := http.NewServeMux()
	reg.Register(api)
	api.Handle("/", mux)
	return api, cache, kept
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

func TestEventsAsResources(t *testing.T) {
	h, cache, kept := apiApp(t)
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
	if cache.Model().Event(made) == nil || cache.Model().AnswerOf(host, made) != AnswerYes {
		t.Fatalf("the new event %s: %+v, host's answer %q", made, cache.Model().Event(made), cache.Model().AnswerOf(host, made))
	}
	if rec := testkit.Call(t, h, robin, "POST", "/api/events/"+made+"/send", map[string]any{}); rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
		t.Errorf("a stranger sending another's invites: %d %s", rec.Code, rec.Body)
	}
	write(t, h, host, "POST", "/api/act", []map[string]any{
		{"method": "POST", "path": "/api/events/" + made + "/invite", "body": map[string]any{"people": []map[string]string{{"email": robin}}}},
		{"method": "POST", "path": "/api/events/" + made + "/send", "body": map[string]any{}},
	})
	if inv := cache.Model().InviteOf(made, robin); inv == nil || inv.Requested == "" {
		t.Fatalf("robin after the batch: %+v", inv)
	}
	eventually(t, "robin is sent the invitation", func() bool { return len(mailTo(kept, robin)) == 1 })

	write(t, h, robin, "POST", "/api/events/"+made+"/answer", map[string]string{"answer": "yes"})
	if cache.Model().AnswerOf(robin, made) != AnswerYes {
		t.Errorf("robin's answer = %q", cache.Model().AnswerOf(robin, made))
	}

	before := len(changeLog(t))
	write(t, h, host, "POST", "/api/events/"+made+"/edit", map[string]string{"title": "Meetup!"})
	after := changeLog(t)[before:]
	if cache.Model().Event(made).Title != "Meetup!" || len(after) != 1 || !strings.Contains(after[0], "|Title|Meetup") {
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
	if len(cache.Model().MyCalendars(host)) != 2 {
		t.Errorf("calendars after deleting: %+v", cache.Model().MyCalendars(host))
	}
}
