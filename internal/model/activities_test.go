package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
	"heliosian/internal/testkit/mailtest"
)

const (
	activitiesParent = "robin.whitfield@heliosschool.org"
	chair            = "mina.park@heliosschool.org"

	activitiesKid = "ella.whitfield@heliosschool.org"
	student       = "sam.whitfield@heliosschool.org"
)

var (
	directory *Directory
	settings  = &Config{}
)

func activityViewerOf(email string, admin bool) access.Actor {
	var held []access.Allowance
	if admin {
		held = ActivitiesAdminAllowances
	}
	return directory.ActorOf(email, held)
}

var activitiesBundled = testkit.Images(func(key string) bool { return strings.HasPrefix(key, "brand/") })

func activitiesServeWith(t *testing.T, mailer *mail.Mailgun) (*Store, *http.ServeMux) {
	t.Helper()
	sampleSheet(t)
	listRows(t, nil)
	deps := sampleDeps(sampleKey)
	deps.Activities = activitiesBundled
	s := sampleStore(t, sheet, queue, deps)
	directory = s.Model().Directory
	mux := http.NewServeMux()
	calendar := calendarOver(s)
	hooks := RegisterActivities(mux, ActivitiesDeps{
		Store:    s,
		Images:   blob.NewImages(blob.New(blob.NewMemoryBucket()), "team"),
		Calendar: calendar,
		Mailer:   mailer,
		Style:    activitiesTestStyle,
	})
	typedRegistry(s, queue, DirectoryResources(), calendar.Resources(), hooks.Resources()).Register(mux)
	return s, mux
}

func volunteerKeyOf(activityID, email string) string {
	return id.Of(sampleKey, kindVolunteer, activityID+"\x00"+email)
}

func teamSettingsKeyOf() string {
	return id.Of(sampleKey, kindActivitiesSettings, "")
}

func redirectKeyOf(old string) string {
	return id.Of(sampleKey, kindActivityRedirect, strings.ToLower(old))
}

func activityAction(activityID, action string) string {
	return "/api/activities/" + activityID + "/" + action
}

func volunteerPath(activityID, email string) string {
	return "/api/volunteers/" + volunteerKeyOf(activityID, email)
}

func madeBy(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("make: %d %s", rec.Code, rec.Body)
	}
	made := created(t, rec)
	if made == "" {
		t.Fatalf("make answered no id: %s", rec.Body)
	}
	return made
}

func teamSettingsAction(action string) string {
	return "/api/team-settings/" + teamSettingsKeyOf() + "/" + action
}

type activityRead struct {
	activityResource
	Can        map[string]bool `json:"can"`
	Volunteers []string        `json:"volunteers"`
	Children   []string        `json:"children"`
	Links      []string        `json:"links"`
}

type teamRead struct {
	activities map[string]activityRead
	order      []string
	volunteers map[string]volunteerResource
	links      map[string]activityLinkResource
	categories map[string]ActivityCategory
	settings   activitiesSettingsResource
}

func teamAs(t *testing.T, mux http.Handler, as string) teamRead {
	t.Helper()
	out := decoded[envelope](t, testkit.Call(t, mux, as, "POST", "/api/query", map[string]any{
		"activities": map[string]string{"path": "/api/activities?include=volunteers,children,links"},
		"categories": map[string]string{"path": "/api/activity-categories"},
		"settings":   map[string]string{"path": "/api/team-settings"},
	}))
	var result map[string][]string
	if err := json.Unmarshal(out.Result, &result); err != nil {
		t.Fatal(err)
	}
	read := teamRead{activities: map[string]activityRead{}, order: result["activities"], volunteers: map[string]volunteerResource{},
		links: map[string]activityLinkResource{}, categories: map[string]ActivityCategory{}}
	for key, raw := range out.Resources["activities"] {
		var a activityRead
		if err := json.Unmarshal(raw, &a); err != nil {
			t.Fatal(err)
		}
		read.activities[key] = a
	}
	for key, raw := range out.Resources["volunteers"] {
		var v volunteerResource
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		read.volunteers[key] = v
	}
	for key, raw := range out.Resources["activity-links"] {
		var l activityLinkResource
		if err := json.Unmarshal(raw, &l); err != nil {
			t.Fatal(err)
		}
		read.links[key] = l
	}
	for key, raw := range out.Resources["activity-categories"] {
		var c ActivityCategory
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatal(err)
		}
		read.categories[key] = c
	}
	if err := json.Unmarshal(out.Resources["team-settings"][teamSettingsKeyOf()], &read.settings); err != nil {
		t.Fatal(err)
	}
	return read
}

func (r teamRead) volunteersOf(activityID string) []volunteerResource {
	out := []volunteerResource{}
	for _, key := range r.activities[activityID].Volunteers {
		out = append(out, r.volunteers[key])
	}
	return out
}

func activityAs(t *testing.T, mux http.Handler, as, activityID string) (activityRead, bool) {
	t.Helper()
	rec := testkit.Call(t, mux, as, "GET", "/api/activities/"+activityID+"?include=volunteers,children,links", nil)
	if rec.Code == http.StatusNotFound {
		return activityRead{}, false
	}
	out := decoded[envelope](t, rec)
	var keys []string
	if err := json.Unmarshal(out.Result, &keys); err != nil {
		var key string
		if err := json.Unmarshal(out.Result, &key); err != nil {
			t.Fatal(err)
		}
		keys = []string{key}
	}
	var a activityRead
	if err := json.Unmarshal(out.Resources["activities"][keys[0]], &a); err != nil {
		t.Fatal(err)
	}
	return a, true
}

var activitiesTestStyle = ActivitiesCardStyle(func() string { return "HCA-Team" }, func() string { return "HCA Volunteer Portal" })

func activitiesServer(t *testing.T) (*Store, *http.ServeMux) {
	t.Helper()
	return activitiesServeWith(t, mailtest.Discard())
}

func activitiesTables(t *testing.T) store.Tables {
	t.Helper()
	return testkit.Tables(t, sheet, queue, activitiesAppName, activityCategoriesTab, activitiesTab, volunteersTab, linksTab, activitySettingsTab, AdminsTab.Name, activityRedirectsTab)
}

func activitiesChangeLog(t *testing.T) []store.Row {
	t.Helper()
	return testkit.ChangeLog(t, sheet, queue, activitiesAppName)
}

func TestAdminApproves(t *testing.T) {
	for status, action := range map[string]string{StatusOpen: "approve", StatusHidden: "decline"} {
		t.Run(status, func(t *testing.T) {
			cache, mux := activitiesServer(t)
			if rec := testkit.Call(t, mux, activitiesParent, "POST", activityAction("act0000000012", action), nil); rec.Code != http.StatusNotFound {
				t.Fatalf("a parent changed an activity's status: %d", rec.Code)
			}
			before := len(activitiesChangeLog(t))
			if rec := testkit.Call(t, mux, jordan, "POST", activityAction("act0000000012", action), nil); rec.Code != http.StatusNoContent {
				t.Fatalf("making act0000000012 %s: %d %s", status, rec.Code, rec.Body)
			}
			log := activitiesChangeLog(t)[before:]
			if got := cache.Model().Activities.Activity("act0000000012").Status; got != status || len(log) != 1 || log[0]["Column"] != "Status" {
				t.Fatalf("act0000000012 after making it %s: status %s, change log %v", status, got, log)
			}
		})
	}
}

func TestASaveWritesOnlyWhatItNames(t *testing.T) {
	cache, mux := activitiesServer(t)
	was := *cache.Model().Activities.Activity("act0000000002")
	before := len(activitiesChangeLog(t))
	if rec := testkit.Call(t, mux, chair, "POST", activityAction("act0000000002", "edit"), map[string]any{"title": "Renamed"}); rec.Code != http.StatusNoContent {
		t.Fatalf("the co-chair's rename: %d %s", rec.Code, rec.Body)
	}
	log := activitiesChangeLog(t)[before:]
	got := cache.Model().Activities.Activity("act0000000002")
	if len(log) != 1 || log[0]["Column"] != "Title" || got.Title != "Renamed" || got.Description != was.Description || got.Status != was.Status || got.Category != was.Category {
		t.Fatalf("after a rename: %+v, change log %v", got, log)
	}
}

func TestApproveAndDecline(t *testing.T) {
	cache, mux := activitiesServer(t)
	status := func(id string) string { return cache.Model().Activities.Activity(id).Status }
	for _, c := range []struct {
		as, id           string
		approve, decline bool
	}{
		{jordan, "act0000000012", true, true},
		{jordan, "act0000000022", true, true},
		{chair, "act0000000022", true, false},
		{"elena.torres@heliosschool.org", "act0000000012", false, false},
		{jordan, "act0000000001", false, false},
		{chair, "act0000000017", false, false},
	} {
		a, ok := activityAs(t, mux, c.as, c.id)
		if !ok || a.Can["approve"] != c.approve || a.Can["decline"] != c.decline {
			t.Errorf("%s on %s: seen %v, can %v, want approve %v decline %v", c.as, c.id, ok, a.Can, c.approve, c.decline)
		}
	}
	if rec := testkit.Call(t, mux, "elena.torres@heliosschool.org", "POST", activityAction("act0000000012", "approve"), nil); rec.Code != http.StatusForbidden || status("act0000000012") != StatusPending {
		t.Fatalf("the suggester approved their own suggestion: %d, %s", rec.Code, status("act0000000012"))
	}
	if rec := testkit.Call(t, mux, chair, "POST", activityAction("act0000000022", "decline"), nil); rec.Code != http.StatusForbidden || status("act0000000022") != StatusPending {
		t.Fatalf("a co-chair declined a suggestion: %d, %s", rec.Code, status("act0000000022"))
	}
	if rec := testkit.Call(t, mux, jordan, "POST", activityAction("act0000000001", "approve"), nil); rec.Code != http.StatusForbidden || status("act0000000001") != StatusOpen {
		t.Fatalf("approved what was not pending: %d, %s", rec.Code, status("act0000000001"))
	}
	if rec := testkit.Call(t, mux, chair, "POST", activityAction("act0000000022", "approve"), nil); rec.Code != http.StatusNoContent || status("act0000000022") != StatusOpen {
		t.Fatalf("the event's co-chair could not approve: %d, %s", rec.Code, status("act0000000022"))
	}
	if a, _ := activityAs(t, mux, chair, "act0000000022"); a.Can["approve"] || a.Can["decline"] {
		t.Errorf("an approved suggestion can still be approved: %v", a.Can)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", activityAction("act0000000012", "decline"), nil); rec.Code != http.StatusNoContent || status("act0000000012") != StatusHidden {
		t.Fatalf("an admin could not decline: %d, %s", rec.Code, status("act0000000012"))
	}
	if _, ok := activityAs(t, mux, "elena.torres@heliosschool.org", "act0000000012"); ok {
		t.Error("the suggester still sees a declined suggestion")
	}
}

func byTitle(m *Activities, year, title string) *Activity {
	for _, root := range m.Activities {
		for _, node := range append([]*Activity{root}, root.Descendants()...) {
			if node.Year == year && node.Title == title {
				return node
			}
		}
	}
	return nil
}

func TestActivitiesSampleLoads(t *testing.T) {
	cache, _ := activitiesServer(t)
	m := cache.Model().Activities
	if len(m.Categories) != 6 || len(m.Activities) != 15 {
		t.Fatalf("got %d categories, %d root activities", len(m.Categories), len(m.Activities))
	}
	night := m.Activity("act0000000001")
	perf := byTitle(m, "2026 - 2027", "India Performance")
	if night == nil || len(night.Children) != 6 || perf == nil || perf.Parent != "act0000000020" {
		t.Fatalf("international night did not load as expected: %+v", night)
	}
	if perf.Category != "" {
		t.Fatalf("a child should inherit its root's category, got %q", perf.Category)
	}
	if night.Highlight == nil || night.Highlight.Headline != "Performances" || night.Highlight.Icon != "📣" || perf.Highlight != nil {
		t.Fatalf("highlight: night %+v, perf %+v", night.Highlight, perf.Highlight)
	}
	if len(night.CoChairs()) != 2 {
		t.Fatalf("co-chairs: %v", night.CoChairs())
	}
}

func TestVisibleToIsWhatRenderShows(t *testing.T) {
	cache, mux := activitiesServer(t)
	m := cache.Model().Activities
	if !m.Activity("act0000000001").IsCoChair(chair) {
		t.Fatalf("%s is not a co-chair of international night", chair)
	}
	all := []*Activity{}
	for _, root := range m.Activities {
		all = append(append(all, root), root.Descendants()...)
	}
	for _, c := range []struct {
		email string
		admin bool
	}{{activitiesParent, false}, {"elena.torres@heliosschool.org", false}, {chair, false}, {jordan, true}} {
		as := directory.ActorOf(c.email, cache.Held(c.email))
		if as.May(SeeAllActivities) != c.admin {
			t.Fatalf("%s: sees all %v, want %v", c.email, as.May(SeeAllActivities), c.admin)
		}
		var listed []string
		if err := json.Unmarshal(decoded[envelope](t, testkit.Call(t, mux, c.email, "GET", "/api/activities", nil)).Result, &listed); err != nil {
			t.Fatal(err)
		}
		shown := map[string]bool{}
		for _, key := range listed {
			shown[key] = true
		}
		hidden := 0
		for _, a := range all {
			if m.VisibleTo(a, as) != shown[a.ID] {
				t.Errorf("%s (admin %v): %q (%s) VisibleTo %v, rendered %v", c.email, c.admin, a.Title, a.Status, m.VisibleTo(a, as), shown[a.ID])
			}
			if !shown[a.ID] {
				hidden++
			}
		}
		if !c.admin && hidden == 0 {
			t.Errorf("%s: nothing is hidden, so the test proves nothing", c.email)
		}
	}
}

func TestActivitiesRenderHidesWhatItShould(t *testing.T) {
	_, mux := activitiesServer(t)
	view := teamAs(t, mux, activitiesParent)
	rooms := false
	for _, key := range view.order {
		a := view.activities[key]
		if a.Status == StatusHidden || a.Status == StatusPending {
			t.Errorf("%s reached a parent as %s", a.Title, a.Status)
		}
		if a.Title == "Room Parents" {
			rooms = true
			for _, v := range view.volunteersOf(key) {
				if v.Position != PositionCoChair {
					t.Errorf("room parents list leaked %s", v.Email)
				}
			}
			if a.Taken != 1 {
				t.Errorf("room parents taken %d", a.Taken)
			}
		}
	}
	if !rooms {
		t.Error("room parents did not reach a parent")
	}
	user := view.settings.User
	if user.Name != "Robin Whitfield" || len(user.Spouses) != 1 || user.Spouses[0].Email != jordan || len(user.Children) != 2 || user.Children[0] != (Child{Email: student, Name: "Sam Whitfield", Grade: "Grade 3"}) || view.settings.People != nil {
		t.Errorf("user %+v, people %v", user, view.settings.People)
	}
	suggester := teamAs(t, mux, "elena.torres@heliosschool.org")
	found := false
	for _, a := range suggester.activities {
		if a.Title == "Family Escape Room Night" {
			found = true
		}
	}
	if !found {
		t.Error("a suggester cannot see their own pending suggestion")
	}
	if got := teamAs(t, mux, "someone.new@heliosschool.org").settings.User.Name; got != "Someone New" {
		t.Errorf("display name %q", got)
	}
}

func TestHiddenVolunteersThroughTheRelation(t *testing.T) {
	cache, mux := activitiesServer(t)
	const hummingbirds, carmen, dana = "act0000000033", "carmen.alvarez@heliosschool.org", "dana.hawkins@heliosschool.org"
	if a := cache.Model().Activities.Activity(hummingbirds); !a.VolunteersHidden || len(a.Volunteers) != 1 || a.Volunteers[0].Email != carmen {
		t.Fatalf("the sample's hidden list: %+v", a)
	}
	for _, c := range []struct {
		as   string
		want []string
	}{{activitiesParent, []string{}}, {carmen, []string{carmen}}, {dana, []string{carmen}}, {jordan, []string{carmen}}} {
		a, ok := activityAs(t, mux, c.as, hummingbirds)
		if !ok {
			t.Fatalf("%s cannot see the hummingbirds", c.as)
		}
		want := []string{}
		for _, email := range c.want {
			want = append(want, volunteerKeyOf(hummingbirds, email))
		}
		if !slices.Equal(a.Volunteers, want) || a.Taken != 1 {
			t.Errorf("%s: volunteers %v taken %d, want %v", c.as, a.Volunteers, a.Taken, want)
		}
	}
	if rec := testkit.Call(t, mux, activitiesParent, "GET", volunteerPath(hummingbirds, carmen), nil); rec.Code != http.StatusNotFound {
		t.Errorf("a hidden sign-up was read directly: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "GET", "/api/volunteers", nil); strings.Contains(rec.Body.String(), carmen) {
		t.Errorf("the volunteer list leaked a hidden sign-up: %s", rec.Body)
	}
	own := decoded[envelope](t, testkit.Call(t, mux, carmen, "GET", volunteerPath(hummingbirds, carmen), nil))
	var mine volunteerResource
	if err := json.Unmarshal(own.Resources["volunteers"][volunteerKeyOf(hummingbirds, carmen)], &mine); err != nil {
		t.Fatal(err)
	}
	if mine.Email != carmen || mine.AddedBy != "" {
		t.Errorf("a volunteer's own hidden sign-up: %+v", mine)
	}
	seen := decoded[envelope](t, testkit.Call(t, mux, dana, "GET", volunteerPath(hummingbirds, carmen), nil))
	var theirs volunteerResource
	if err := json.Unmarshal(seen.Resources["volunteers"][volunteerKeyOf(hummingbirds, carmen)], &theirs); err != nil {
		t.Fatal(err)
	}
	if theirs.AddedBy != carmen {
		t.Errorf("the co-chair's view of a sign-up: %+v", theirs)
	}
}

func TestActivityForPrivateList(t *testing.T) {
	cache, _ := activitiesServer(t)
	m := cache.Model().Activities
	a := &Activity{ID: "X1", Status: StatusOpen, VolunteersHidden: true, Volunteers: []Volunteer{
		{Email: chair, Position: PositionCoChair},
		{Email: activitiesParent, Position: PositionOpen, AddedBy: activitiesParent},
		{Email: activitiesKid, Position: PositionOpen, AddedBy: activitiesParent},
		{Email: "elena.torres@heliosschool.org", Position: PositionOpen},
	}}
	for _, c := range []struct {
		name string
		as   access.Actor
		want []string
	}{
		{"stranger", activityViewerOf("someone.new@heliosschool.org", false), []string{chair}},
		{"student", activityViewerOf(activitiesKid, false), []string{chair, activitiesKid}},
		{"parent", activityViewerOf(activitiesParent, false), []string{chair, activitiesParent, activitiesKid}},
		{"co-chair", activityViewerOf(chair, false), []string{chair, activitiesParent, activitiesKid, "elena.torres@heliosschool.org"}},
		{"admin", activityViewerOf(jordan, true), []string{chair, activitiesParent, activitiesKid, "elena.torres@heliosschool.org"}},
	} {
		got := m.ActivityFor(a, c.as)
		emails := []string{}
		for _, v := range got.Volunteers {
			emails = append(emails, v.Email)
			if v.AddedBy != "" && !m.Edits(a, c.as) {
				t.Errorf("%s: added by leaked on %s", c.name, v.Email)
			}
		}
		if !slices.Equal(emails, c.want) || got.Taken != 4 {
			t.Errorf("%s: got %v taken %d, want %v", c.name, emails, got.Taken, c.want)
		}
	}
}

func TestSignUpAndRemove(t *testing.T) {
	cache, mux := activitiesServer(t)
	signUp := activityAction("act0000000017", "sign-up")
	body := map[string]any{"position": PositionOpen, "note": "happy to help"}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", signUp, body); rec.Code != http.StatusNoContent {
		t.Fatalf("sign up: %d %s", rec.Code, rec.Body)
	}
	role := cache.Model().Activities.Activity("act0000000017")
	if len(role.Volunteers) != 1 || role.Volunteers[0].Email != activitiesParent || role.Volunteers[0].AddedBy != activitiesParent {
		t.Fatalf("volunteers after sign up: %+v", role.Volunteers)
	}
	body["position"] = PositionCoChair
	if rec := testkit.Call(t, mux, activitiesParent, "POST", signUp, body); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent named themselves co-chair: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, chair, "POST", signUp, map[string]any{"email": activitiesParent, "position": PositionCoChair}); rec.Code != http.StatusNoContent {
		t.Fatalf("a co-chair could not promote: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", signUp, map[string]any{"position": PositionCoChair, "note": "here to lead"}); rec.Code != http.StatusNoContent {
		t.Fatalf("a co-chair could not edit their own note: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", signUp, map[string]any{"position": PositionVolunteer}); rec.Code != http.StatusNoContent {
		t.Fatalf("a co-chair could not step down: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", signUp, map[string]any{"position": PositionCoChair}); rec.Code != http.StatusForbidden {
		t.Fatalf("a volunteer named themselves co-chair: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", signUp, map[string]any{"email": activitiesParent, "position": PositionCoChair}); rec.Code != http.StatusNoContent {
		t.Fatalf("an admin could not promote: %d %s", rec.Code, rec.Body)
	}
	if a, _ := activityAs(t, mux, activitiesParent, "act0000000026"); a.Can["sign-up"] || !a.Full {
		t.Fatalf("a full role offers a sign-up: can %v, full %v", a.Can, a.Full)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", activityAction("act0000000026", "sign-up"), map[string]any{"position": PositionVolunteer}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a full role took a sign-up: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", activityAction("act0000000001", "sign-up"), map[string]any{"email": "Facilities@heliosschool.org", "position": PositionVolunteer}); rec.Code != http.StatusNoContent {
		t.Fatalf("an admin could not sign up an alias: %d %s", rec.Code, rec.Body)
	}
	if v := cache.Model().Activities.Activity("act0000000001").volunteer("hank.morrow@heliosschool.org"); v == nil {
		t.Fatalf("a sign-up by alias was not stored as the directory's address: %+v", cache.Model().Activities.Activity("act0000000001").Volunteers)
	}
	for _, as := range []string{activitiesParent, jordan} {
		if rec := testkit.Call(t, mux, as, "POST", signUp, map[string]any{"email": "x@elsewhere.example", "position": PositionVolunteer}); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s signed up an address outside the directory: %d", as, rec.Code)
		}
	}
	if a, _ := activityAs(t, mux, activitiesParent, "act0000000001"); a.Can["sign-up"] {
		t.Fatalf("an activity without direct sign-up offers one: %v", a.Can)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", activityAction("act0000000001", "sign-up"), map[string]any{"position": PositionVolunteer}); rec.Code != http.StatusBadRequest {
		t.Fatalf("an activity without direct sign-up took one: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, "someone.else@heliosschool.org", "DELETE", volunteerPath("act0000000017", activitiesParent), nil); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger removed someone: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", signUp, map[string]any{"email": activitiesKid, "position": PositionVolunteer}); rec.Code != http.StatusNoContent {
		t.Fatalf("a parent could not sign their child up: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", volunteerPath("act0000000017", activitiesKid)+"/edit", map[string]any{"position": PositionVolunteer, "note": "after school only"}); rec.Code != http.StatusNoContent {
		t.Fatalf("a parent could not edit their child's sign-up: %d %s", rec.Code, rec.Body)
	}
	if v := cache.Model().Activities.Activity("act0000000017").volunteer(activitiesKid); v == nil || v.Note != "after school only" || v.Position != PositionVolunteer {
		t.Fatalf("the child's sign-up after the edit: %+v", v)
	}
	if rec := testkit.Call(t, mux, "someone.else@heliosschool.org", "POST", volunteerPath("act0000000017", activitiesKid)+"/edit", map[string]any{"position": PositionVolunteer, "note": "not mine"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger edited someone's sign-up: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, activitiesKid, "DELETE", volunteerPath("act0000000017", activitiesParent), nil); rec.Code != http.StatusForbidden {
		t.Fatalf("a child removed their parent: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "DELETE", volunteerPath("act0000000017", activitiesKid), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("a parent could not remove their child: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "DELETE", volunteerPath("act0000000017", activitiesParent), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("remove self: %d %s", rec.Code, rec.Body)
	}
	if n := len(cache.Model().Activities.Activity("act0000000017").Volunteers); n != 0 {
		t.Fatalf("%d volunteers left after removal", n)
	}
}

func TestOfferToCoChairWithoutDirectSignUp(t *testing.T) {
	cache, mux := activitiesServer(t)
	const crew = "act0000000017"
	if rec := testkit.Call(t, mux, jordan, "POST", activityAction(crew, "edit"), map[string]any{"directSignUp": "No", "coLeaderNeeded": true}); rec.Code != http.StatusNoContent {
		t.Fatalf("close direct sign-up: %d %s", rec.Code, rec.Body)
	}
	if a, _ := activityAs(t, mux, activitiesParent, crew); a.Can["sign-up"] {
		t.Fatalf("a crew without direct sign-up offers one: %v", a.Can)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", activityAction(crew, "sign-up"), map[string]any{"position": PositionVolunteer}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a plain sign-up went through: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", activityAction(crew, "sign-up"), map[string]any{"position": PositionOpen}); rec.Code != http.StatusNoContent {
		t.Fatalf("an offer to co-chair was refused: %d %s", rec.Code, rec.Body)
	}
	if v := cache.Model().Activities.Activity(crew).volunteer(activitiesParent); v == nil || v.Position != PositionOpen {
		t.Fatalf("the offer: %+v", v)
	}
}

func TestVolunteersCompleteClosesSignUp(t *testing.T) {
	cache, mux := activitiesServer(t)
	const crew = "act0000000017"
	if a, _ := activityAs(t, mux, activitiesParent, crew); !a.Can["sign-up"] || a.Full || a.VolunteersComplete {
		t.Fatalf("an open crew: can %v, full %v, complete %v", a.Can, a.Full, a.VolunteersComplete)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", activityAction(crew, "edit"), map[string]any{"volunteersComplete": true}); rec.Code != http.StatusNoContent {
		t.Fatalf("mark complete: %d %s", rec.Code, rec.Body)
	}
	if a, _ := activityAs(t, mux, activitiesParent, crew); a.Can["sign-up"] || !a.Full || !a.VolunteersComplete {
		t.Fatalf("a complete crew: can %v, full %v, complete %v", a.Can, a.Full, a.VolunteersComplete)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", activityAction(crew, "sign-up"), map[string]any{"position": PositionVolunteer}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a complete crew took a sign-up: %d %s", rec.Code, rec.Body)
	}
	if v := cache.Model().Activities.Activity(crew).volunteer(activitiesParent); v != nil {
		t.Fatalf("the refused sign-up was stored: %+v", v)
	}
}

func TestFullAndPast(t *testing.T) {
	_, mux := activitiesServer(t)
	for _, c := range []struct {
		id         string
		full, past bool
	}{
		{"act0000000026", true, false},
		{"act0000000016", false, false},
		{"act0000000001", false, false},
		{"act0000000020", false, false},
		{"act0000000005", false, true},
		{"act0000000013", false, true},
		{"act0000000038", false, true},
		{"act0000000003", false, false},
		{"act0000000027", false, false},
	} {
		a, ok := activityAs(t, mux, jordan, c.id)
		if !ok || a.Full != c.full || a.Past != c.past {
			t.Errorf("%s: seen %v, full %v past %v, want %v %v", c.id, ok, a.Full, a.Past, c.full, c.past)
		}
	}
	if rec := testkit.Call(t, mux, jordan, "POST", activityAction("act0000000003", "edit"), map[string]any{"start": "2026-09-01 18:00", "end": "2026-09-01 20:00"}); rec.Code != http.StatusNoContent {
		t.Fatalf("move the movie night back: %d %s", rec.Code, rec.Body)
	}
	for _, key := range []string{"act0000000003", "act0000000027"} {
		if a, _ := activityAs(t, mux, jordan, key); !a.Past || a.Status != StatusOpen {
			t.Errorf("%s after its date went by: past %v, status %s", key, a.Past, a.Status)
		}
	}
}

func TestTeamNotify(t *testing.T) {
	_, mux := activitiesServer(t)
	if got := teamAs(t, mux, activitiesParent).settings.Notify; len(got) != 0 {
		t.Fatalf("a parent reads notification kinds: %v", got)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", teamSettingsAction("notify"), map[string]any{"kinds": []string{"signups"}}); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent set notifications: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", teamSettingsAction("notify"), map[string]any{"kinds": []string{"offers", "signups", "nonsense"}}); rec.Code != http.StatusNoContent {
		t.Fatalf("notify prefs: %d %s", rec.Code, rec.Body)
	}
	if got := teamAs(t, mux, jordan).settings.Notify; !slices.Equal(got, []string{"signups", "offers"}) {
		t.Fatalf("notify after setting: %v", got)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", teamSettingsAction("notify"), map[string]any{"kinds": []string{}}); rec.Code != http.StatusNoContent {
		t.Fatalf("clear notify prefs: %d %s", rec.Code, rec.Body)
	}
	if got := teamAs(t, mux, jordan).settings.Notify; len(got) != 0 {
		t.Fatalf("notify after clearing: %v", got)
	}
}

func TestActivitiesMail(t *testing.T) {
	rec := mailtest.NewRecorder(mailtest.From)
	_, mux := activitiesServeWith(t, rec.Mailgun)
	signUp := activityAction("act0000000017", "sign-up")
	if r := testkit.Call(t, mux, jordan, "POST", teamSettingsAction("notify"), map[string]any{"kinds": []string{"signups", "offers"}}); r.Code != http.StatusNoContent {
		t.Fatalf("notify prefs: %d %s", r.Code, r.Body)
	}
	if r := testkit.Call(t, mux, activitiesParent, "POST", signUp, map[string]any{"position": PositionOpen, "note": "happy to help"}); r.Code != http.StatusNoContent {
		t.Fatalf("sign up: %d %s", r.Code, r.Body)
	}
	bySubject := map[string]mail.Message{}
	for range 4 {
		m := rec.Next(t)
		bySubject[m.Subject] = m
	}
	thanks := bySubject["Thanks for volunteering for Clean Up Crew"]
	if !slices.Equal(thanks.To, []string{activitiesParent}) || !slices.Equal(thanks.CC, []string{jordan, chair}) || len(thanks.Attachments) != 0 || !strings.Contains(thanks.HTML, "Hi Robin") || !strings.Contains(thanks.HTML, "/open/share/act0000000017.png") || !strings.Contains(thanks.HTML, "Add to Calendar") {
		t.Fatalf("thank-you: %+v (subjects %v)", thanks, keys(bySubject))
	}
	if !slices.Equal(thanks.ReplyTo, []string{jordan, chair}) {
		t.Errorf("thank-you reply-to: %v", thanks.ReplyTo)
	}
	invite := bySubject["Calendar invite: Clean Up Crew"]
	if !slices.Equal(invite.To, []string{activitiesParent}) || len(invite.CC) != 0 || !slices.Equal(invite.ReplyTo, []string{jordan, chair}) || len(invite.Attachments) != 1 || !strings.Contains(invite.HTML, "Add it to your calendar") {
		t.Fatalf("invite note: %+v (subjects %v)", invite, keys(bySubject))
	}
	ics := strings.ReplaceAll(string(invite.Attachments[0].Content), "\r\n ", "")
	for _, want := range []string{"METHOD:REQUEST", "UID:team-act0000000017-" + activitiesParent + "@heliosian.com", "SUMMARY:Clean Up Crew (International Night)", "DTSTART:20260924T230000Z", "DTEND:20260925T010000Z", "ORGANIZER;CN=HCA-Team:mailto:" + mail.AddressOf(mailtest.From), "ATTENDEE;CN=Robin Whitfield;ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED;RSVP=FALSE:mailto:" + activitiesParent, "STATUS:CONFIRMED"} {
		if !strings.Contains(ics, want) {
			t.Errorf("invite lacks %q:\n%s", want, ics)
		}
	}
	if strings.Contains(ics, "mailto:"+chair) {
		t.Errorf("a chair is on the volunteer's invite:\n%s", ics)
	}
	if !strings.Contains(thanks.Text, "Your note: happy to help") {
		t.Errorf("thank-you note label: %q", thanks.Text)
	}
	if !strings.Contains(thanks.Text, "Event Chairs: ") || strings.Contains(thanks.Text, "Leads") {
		t.Fatalf("thank-you chairs: %q", thanks.Text)
	}
	if n, ok := bySubject["New sign-up: Robin Whitfield for Clean Up Crew"]; !ok || !slices.Equal(n.To, []string{jordan}) {
		t.Fatalf("sign-up notice: %+v", n)
	}
	if n, ok := bySubject["Co-chair offer: Robin Whitfield for Clean Up Crew"]; !ok || !slices.Equal(n.To, []string{jordan}) {
		t.Fatalf("offer notice: %+v", n)
	}
	if r := testkit.Call(t, mux, chair, "POST", signUp, map[string]any{"email": activitiesParent, "position": PositionCoChair}); r.Code != http.StatusNoContent {
		t.Fatalf("promote: %d %s", r.Code, r.Body)
	}
	m := rec.Next(t)
	if m.Subject != "You're a co-chair of Clean Up Crew" || !slices.Equal(m.To, []string{activitiesParent}) || !slices.Contains(m.CC, chair) {
		t.Fatalf("co-chair note: %+v", m)
	}
	other := student
	if r := testkit.Call(t, mux, other, "POST", signUp, map[string]any{"position": PositionVolunteer}); r.Code != http.StatusNoContent {
		t.Fatalf("second sign up: %d %s", r.Code, r.Body)
	}
	family := append([]string{other}, directory.Person(other).ParentContactEmails...)
	if !slices.Contains(family, activitiesParent) || !slices.Contains(family, jordan) {
		t.Fatalf("the sample student's parents: %v", family)
	}
	for range 3 {
		m := rec.Next(t)
		switch {
		case strings.HasPrefix(m.Subject, "Thanks for volunteering"):
			if !strings.Contains(m.Text, "Clean Up Crew Leads: Robin Whitfield\n") || !strings.Contains(m.Text, "Event Chairs: ") || !slices.Equal(m.To, family) || !slices.Equal(m.CC, []string{chair}) {
				t.Fatalf("thank-you under a lead: %q to %v cc %v", m.Text, m.To, m.CC)
			}
		case strings.HasPrefix(m.Subject, "Calendar invite"):
			if !slices.Equal(m.To, family) || len(m.CC) != 0 || !strings.Contains(string(m.Attachments[0].Content), "mailto:"+other) {
				t.Fatalf("a student's invite: to %v cc %v", m.To, m.CC)
			}
		}
	}
	if r := testkit.Call(t, mux, chair, "DELETE", volunteerPath("act0000000017", other), nil); r.Code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", r.Code, r.Body)
	}
	m = rec.Next(t)
	cancel := strings.ReplaceAll(string(m.Attachments[0].Content), "\r\n ", "")
	if m.Subject != "Removed: Clean Up Crew" || !slices.Equal(m.To, family) || !strings.HasPrefix(m.Attachments[0].ContentType, "text/calendar; method=CANCEL") || !strings.Contains(cancel, "METHOD:CANCEL") || !strings.Contains(cancel, "STATUS:CANCELLED") || !strings.Contains(cancel, "UID:team-act0000000017-"+other+"@heliosian.com") {
		t.Fatalf("cancellation: %+v\n%s", m, cancel)
	}
	if !strings.Contains(m.Text, "Mina Park removed your sign-up for Clean Up Crew") {
		t.Errorf("cancellation does not name who removed it: %q", m.Text)
	}
	if r := testkit.Call(t, mux, activitiesParent, "POST", signUp, map[string]any{"email": activitiesKid, "position": PositionOpen, "note": "bring snacks"}); r.Code != http.StatusNoContent {
		t.Fatalf("sign up a child: %d %s", r.Code, r.Body)
	}
	bySubject = map[string]mail.Message{}
	for range 4 {
		m := rec.Next(t)
		bySubject[m.Subject] = m
	}
	if thanks := bySubject["Thanks for volunteering for Clean Up Crew"]; !strings.Contains(thanks.Text, "Note from Robin Whitfield: bring snacks") {
		t.Errorf("a note someone else wrote is not theirs: %q", thanks.Text)
	}
	if offer := bySubject["Co-chair offer: Ella Whitfield for Clean Up Crew"]; !strings.Contains(offer.Text, "Robin Whitfield added Ella Whitfield as co-chair of Clean Up Crew") || !strings.Contains(offer.Text, "Added by: Robin Whitfield") {
		t.Errorf("offer notice does not name who made it: %q (subjects %v)", offer.Text, keys(bySubject))
	}
}

func keys(m map[string]mail.Message) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestMoveSignUp(t *testing.T) {
	cache, mux := activitiesServer(t)
	on := func(id string) bool {
		for _, v := range cache.Model().Activities.Activity(id).Volunteers {
			if v.Email == activitiesParent {
				return true
			}
		}
		return false
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", activityAction("act0000000017", "sign-up"), map[string]any{"position": PositionVolunteer, "note": "evenings"}); rec.Code != http.StatusNoContent {
		t.Fatalf("sign up: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, "someone.else@heliosschool.org", "POST", activityAction("act0000000018", "sign-up"), map[string]any{"email": activitiesParent, "position": PositionVolunteer, "from": "act0000000017"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger moved someone: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", activityAction("act0000000018", "sign-up"), map[string]any{"position": PositionVolunteer, "note": "evenings", "from": "act0000000017"}); rec.Code != http.StatusNoContent {
		t.Fatalf("move: %d %s", rec.Code, rec.Body)
	}
	if on("act0000000017") || !on("act0000000018") {
		t.Fatalf("after the move: on act0000000017 %v, on act0000000018 %v", on("act0000000017"), on("act0000000018"))
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", activityAction("act0000000019", "sign-up"), map[string]any{"position": PositionVolunteer, "from": "act0000000017"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("moved from a thing not signed up for: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", activityAction("act0000000016", "sign-up"), map[string]any{"position": PositionOpen}); rec.Code != http.StatusBadRequest {
		t.Fatalf("offered to co-chair where none is wanted: %d", rec.Code)
	}
}

func TestMoveNeedsBothEnds(t *testing.T) {
	cache, mux := activitiesServer(t)
	move := func(who, from, to string) int {
		return testkit.Call(t, mux, who, "POST", activityAction(to, "sign-up"), map[string]any{"email": marco, "position": PositionVolunteer, "from": from}).Code
	}
	if code := move(chair, "act0000000016", "act0000000003"); code != http.StatusForbidden || cache.Model().Activities.Activity("act0000000016").volunteer(marco) == nil {
		t.Fatalf("a co-chair moved someone into a thing they do not run: %d", code)
	}
	if code := move(chair, "act0000000016", "act0000000017"); code != http.StatusNoContent || cache.Model().Activities.Activity("act0000000017").volunteer(marco) == nil {
		t.Fatalf("a co-chair could not move someone between two things they run: %d", code)
	}
	if code := move(jordan, "act0000000017", "act0000000003"); code != http.StatusNoContent || cache.Model().Activities.Activity("act0000000003").volunteer(marco) == nil {
		t.Fatalf("an admin could not move someone: %d", code)
	}
}

func TestReorderChildren(t *testing.T) {
	cache, mux := activitiesServer(t)
	titles := func() []string {
		out := []string{}
		for _, c := range cache.Model().Activities.Activity("act0000000002").Children {
			out = append(out, c.Title)
		}
		return out
	}
	if got := titles(); !slices.Equal(got, []string{"Decor", "Childcare", "Marketing"}) {
		t.Fatalf("row order to start: %v", got)
	}
	order := activityAction("act0000000002", "order")
	body := map[string]any{"ids": []string{"act0000000025", "act0000000023", "act0000000024"}}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", order, body); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent reordered: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, chair, "POST", order, body); rec.Code != http.StatusNoContent {
		t.Fatalf("the chair could not reorder: %d %s", rec.Code, rec.Body)
	}
	if got := titles(); !slices.Equal(got, []string{"Marketing", "Decor", "Childcare"}) {
		t.Fatalf("order after: %v", got)
	}
	before := len(activitiesChangeLog(t))
	if rec := testkit.Call(t, mux, chair, "POST", order, map[string]any{"ids": []string{"act0000000023", "act0000000025", "act0000000024"}}); rec.Code != http.StatusNoContent {
		t.Fatalf("second reorder: %d %s", rec.Code, rec.Body)
	}
	if got := titles(); !slices.Equal(got, []string{"Decor", "Marketing", "Childcare"}) || len(activitiesChangeLog(t))-before != 1 {
		t.Fatalf("order after one move: %v, %d log rows", got, len(activitiesChangeLog(t))-before)
	}
	if rec := testkit.Call(t, mux, chair, "POST", order, map[string]any{"ids": []string{"act0000000025", "act0000000001"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a stranger's id was taken: %d", rec.Code)
	}
}

func TestSuggestApproveRenameDelete(t *testing.T) {
	cache, mux := activitiesServer(t)
	suggestion := map[string]any{"year": "2026 - 2027", "title": "Kite Day", "status": StatusOpen, "description": "Fly kites", "signUp": PositionOpen, "directSignUp": "Yes"}
	made := madeBy(t, testkit.Call(t, mux, activitiesParent, "POST", "/api/activity-categories/tcg0000000006/add", suggestion))
	kite := byTitle(cache.Model().Activities, "2026 - 2027", "Kite Day")
	if kite == nil || kite.ID != made || kite.Category != "tcg0000000006" || kite.Status != StatusPending || kite.AddedBy != activitiesParent || len(kite.Volunteers) != 1 || kite.Volunteers[0].Position != PositionOpen {
		t.Fatalf("suggestion %s landed as %+v", made, kite)
	}
	edit := map[string]any{"year": "2026 - 2027", "title": "Kite Festival", "category": "tcg0000000002", "status": StatusOpen, "directSignUp": "Yes"}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", activityAction(kite.ID, "edit"), edit); rec.Code != http.StatusForbidden {
		t.Fatalf("a non-chair edited an activity: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", activityAction(kite.ID, "edit"), edit); rec.Code != http.StatusNoContent {
		t.Fatalf("approve and rename: %d %s", rec.Code, rec.Body)
	}
	if byTitle(cache.Model().Activities, "2026 - 2027", "Kite Day") != nil {
		t.Fatal("the old title survived the rename")
	}
	festival := cache.Model().Activities.Activity(kite.ID)
	if festival == nil || festival.Status != StatusOpen || len(festival.Volunteers) != 1 {
		t.Fatalf("renamed activity: %+v", festival)
	}
	if a, _ := activityAs(t, mux, jordan, kite.ID); a.Can["delete"] {
		t.Fatalf("an activity with a volunteer on it offers delete: %v", a.Can)
	}
	if rec := testkit.Call(t, mux, jordan, "DELETE", "/api/activities/"+kite.ID, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("delete with a volunteer on it: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, jordan, "DELETE", volunteerPath(kite.ID, activitiesParent), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, jordan, "DELETE", "/api/activities/"+kite.ID, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Activities.Activity(kite.ID) != nil {
		t.Fatal("the activity survived deletion")
	}
}

func TestYearMoveCarriesTheTreeAndDeleteTakesTheLinks(t *testing.T) {
	cache, mux := activitiesServer(t)
	spring := cache.Model().Activities.Activity("act0000000002")
	edit := map[string]any{"year": "2027 - 2028", "title": spring.Title, "category": spring.Category, "status": spring.Status, "directSignUp": "Yes", "prettyId": spring.PrettyID}
	if rec := testkit.Call(t, mux, jordan, "POST", activityAction("act0000000002", "edit"), edit); rec.Code != http.StatusNoContent {
		t.Fatalf("move year: %d %s", rec.Code, rec.Body)
	}
	for _, c := range cache.Model().Activities.Activity("act0000000002").Children {
		if c.Year != "2027 - 2028" {
			t.Fatalf("%s stayed in %s", c.Title, c.Year)
		}
	}
	moved := 0
	for _, row := range activitiesTables(t)[activitiesTab] {
		if row["Parent"] == "act0000000002" && row["Year"] == "2027 - 2028" {
			moved++
		}
	}
	years := map[string]int{}
	for _, row := range activitiesChangeLog(t) {
		if row["Column"] == "Year" {
			years[row["Key"]+" "+row["Previous"]]++
			if row["Actor"] != jordan || row["Action"] != "set" {
				t.Errorf("log row %v", row)
			}
		}
	}
	if moved != 3 || len(years) != 4 || years["Event ID=act0000000023 2026 - 2027"] != 1 {
		t.Fatalf("sheet moved %d children, logged %v", moved, years)
	}
	made := madeBy(t, testkit.Call(t, mux, jordan, "POST", "/api/activities", map[string]any{"year": "2026 - 2027", "title": "Bake Sale", "category": "tcg0000000002", "status": StatusOpen, "directSignUp": "Yes"}))
	sale := byTitle(cache.Model().Activities, "2026 - 2027", "Bake Sale")
	if sale == nil || sale.ID != made {
		t.Fatalf("the add answered %s for %+v", made, sale)
	}
	link := madeBy(t, testkit.Call(t, mux, jordan, "POST", "/api/activity-links", map[string]any{"activity": sale.ID, "title": "Menu", "url": "https://example.org/menu"}))
	if links := cache.Model().Activities.Activity(sale.ID).Links; len(links) != 1 || links[0].ID != link {
		t.Fatalf("the link answered %s for %+v", link, links)
	}
	if rec := testkit.Call(t, mux, jordan, "DELETE", "/api/activities/"+sale.ID, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if cache.Count(activitiesAppName, linksTab, store.Row{"Event ID": sale.ID}) != 0 {
		t.Fatal("the link outlived its activity in memory")
	}
	for _, row := range activitiesTables(t)[linksTab] {
		if row["Event ID"] == sale.ID {
			t.Fatalf("the sheet kept %v", row)
		}
	}
	gone := 0
	for _, row := range activitiesChangeLog(t) {
		if row["Action"] == "delete" && row["Tab"] == linksTab && row["Column"] == "URL" && row["Previous"] == "https://example.org/menu" {
			gone++
		}
	}
	if gone != 1 {
		t.Fatalf("the link's delete was not logged: %v", activitiesChangeLog(t))
	}
}

func TestRenameKeepsTheTree(t *testing.T) {
	cache, mux := activitiesServer(t)
	edit := map[string]any{
		"year": "2026 - 2027", "title": "India Booth", "parent": "act0000000001",
		"category": "tcg0000000008", "status": StatusOpen, "coLeaderNeeded": true, "directSignUp": "Yes",
	}
	if rec := testkit.Call(t, mux, chair, "POST", activityAction("act0000000020", "edit"), edit); rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	booth := cache.Model().Activities.Activity("act0000000020")
	if booth == nil || booth.Title != "India Booth" || len(booth.Volunteers) != 1 || len(booth.Links) != 1 || len(booth.Children) != 1 || booth.Children[0].Parent != "act0000000020" {
		t.Fatalf("renamed: %+v", booth)
	}
	if rec := testkit.Call(t, mux, jordan, "DELETE", "/api/activities/act0000000020", nil); rec.Code != http.StatusForbidden || cache.Model().Activities.Activity("act0000000020") == nil {
		t.Fatalf("deleted something with children and volunteers: %d", rec.Code)
	}
	loop := map[string]any{"year": "2026 - 2027", "title": "International Night", "parent": "act0000000020", "category": "", "status": StatusOpen}
	if rec := testkit.Call(t, mux, jordan, "POST", activityAction("act0000000001", "edit"), loop); rec.Code != http.StatusBadRequest {
		t.Fatalf("a parent loop was accepted: %d", rec.Code)
	}
}

func TestCoChairApproves(t *testing.T) {
	cache, mux := activitiesServer(t)
	save := func(who, id, parentID, category, status string) int {
		act := cache.Model().Activities.Activity(id)
		return testkit.Call(t, mux, who, "POST", activityAction(id, "edit"), map[string]any{
			"year": act.Year, "title": act.Title, "parent": parentID, "category": category, "status": status, "directSignUp": act.DirectSignUpOwn,
		}).Code
	}
	status := func(id string) string { return cache.Model().Activities.Activity(id).Status }
	if code := save(chair, "act0000000022", "act0000000001", "tcg0000000008", StatusHidden); code != http.StatusBadRequest {
		t.Fatalf("a co-chair hid a suggestion: %d", code)
	}
	if code := testkit.Call(t, mux, chair, "POST", activityAction("act0000000022", "decline"), nil).Code; code != http.StatusForbidden || status("act0000000022") != StatusPending {
		t.Fatalf("a co-chair declined a suggestion: %d, %s", code, status("act0000000022"))
	}
	if code := save(chair, "act0000000022", "act0000000001", "tcg0000000008", StatusPending); code != http.StatusNoContent || status("act0000000022") != StatusPending {
		t.Fatalf("a co-chair's save that leaves it pending: %d, %s", code, status("act0000000022"))
	}
	if code := testkit.Call(t, mux, chair, "POST", activityAction("act0000000022", "approve"), nil).Code; code != http.StatusNoContent || status("act0000000022") != StatusOpen {
		t.Fatalf("the event's co-chair could not approve a suggestion: %d, %s", code, status("act0000000022"))
	}
	if code := testkit.Call(t, mux, jordan, "POST", activityAction("act0000000012", "sign-up"), map[string]any{"email": chair, "position": PositionCoChair}).Code; code != http.StatusNoContent {
		t.Fatalf("make a co-chair: %d", code)
	}
	if a, ok := activityAs(t, mux, chair, "act0000000012"); !ok || a.Can["approve"] || !a.Can["edit"] || !a.Me.Runs {
		t.Fatalf("a co-chair's own pending event: seen %v, can %v, runs %v", ok, a.Can, a.Me.Runs)
	}
	if code := testkit.Call(t, mux, chair, "POST", activityAction("act0000000012", "approve"), nil).Code; code != http.StatusForbidden || status("act0000000012") != StatusPending {
		t.Fatalf("a co-chair approved their own pending event: %d, %s", code, status("act0000000012"))
	}
	if code := save(chair, "act0000000012", "", "tcg0000000001", StatusOpen); code != http.StatusNoContent || status("act0000000012") != StatusPending {
		t.Fatalf("a co-chair approved their own pending event by saving it: %d, %s", code, status("act0000000012"))
	}
}

func TestCoChairMovesOnlyUnderTheirOwn(t *testing.T) {
	cache, mux := activitiesServer(t)
	const india = "deepa.natarajan@heliosschool.org"
	move := func(who, id, parentID string) int {
		act := cache.Model().Activities.Activity(id)
		return testkit.Call(t, mux, who, "POST", activityAction(id, "edit"), map[string]any{
			"year": act.Year, "title": act.Title, "parent": parentID, "category": "", "status": act.Status, "directSignUp": act.DirectSignUpOwn,
		}).Code
	}
	if code := move(india, "act0000000020", "act0000000002"); code != http.StatusForbidden {
		t.Fatalf("a co-chair moved their booth under an event they do not run: %d", code)
	}
	if code := move(india, "act0000000020", ""); code != http.StatusForbidden {
		t.Fatalf("a co-chair made their booth an event of its own: %d", code)
	}
	if code := move(chair, "act0000000016", "act0000000003"); code != http.StatusForbidden {
		t.Fatalf("a co-chair moved a crew under an event they do not run: %d", code)
	}
	if code := move(chair, "act0000000019", "act0000000002"); code != http.StatusNoContent || cache.Model().Activities.Activity("act0000000019").Parent != "act0000000002" {
		t.Fatalf("a co-chair could not move a booth between their own events: %d", code)
	}
	if code := move(jordan, "act0000000016", "act0000000003"); code != http.StatusNoContent {
		t.Fatalf("an admin could not move a crew: %d", code)
	}
}

func TestCopyToNextYear(t *testing.T) {
	cache, mux := activitiesServer(t)
	if rec := testkit.Call(t, mux, chair, "POST", activityAction("act0000000001", "copy"), nil); rec.Code != http.StatusForbidden {
		t.Fatalf("a co-chair copied: %d", rec.Code)
	}
	if a, _ := activityAs(t, mux, jordan, "act0000000001"); !a.Can["copy"] {
		t.Fatalf("an admin is not offered the copy: %v", a.Can)
	}
	made := madeBy(t, testkit.Call(t, mux, jordan, "POST", activityAction("act0000000001", "copy"), nil))
	next := byTitle(cache.Model().Activities, "2027 - 2028", "International Night")
	if next == nil || next.ID != made || next.ID == "act0000000001" || next.Status != StatusOpen || next.Start != "" || len(next.Volunteers) != 0 || len(next.Descendants()) != 6 || byTitle(cache.Model().Activities, "2027 - 2028", "Cybertron") != nil || len(next.Links) != 2 {
		t.Fatalf("copied activity: %+v", next)
	}
	for _, c := range next.Descendants() {
		if p := cache.Model().Activities.Activity(c.Parent); p == nil || p.Year != "2027 - 2028" {
			t.Fatalf("copied child %q points at parent %q in the wrong year", c.Title, c.Parent)
		}
	}
	if a, _ := activityAs(t, mux, jordan, "act0000000001"); a.Can["copy"] {
		t.Fatalf("a copied activity offers another copy: %v", a.Can)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", activityAction("act0000000001", "copy"), nil); rec.Code != http.StatusForbidden {
		t.Fatalf("copied twice: %d", rec.Code)
	}
}

func TestYears(t *testing.T) {
	if got := ActivityYear(testkit.MustTime("2026-09-09")); got != "2026 - 2027" {
		t.Errorf("september: %s", got)
	}
	if got := ActivityYear(testkit.MustTime("2027-03-01")); got != "2026 - 2027" {
		t.Errorf("march: %s", got)
	}
	if got := ShiftYearSpan("2026 - 2027", -1); got != "2025 - 2026" {
		t.Errorf("shift: %s", got)
	}
	if err := CheckYearSpan("2026 - 2028"); err == nil {
		t.Error("a two-year span passed")
	}
}

func TestEventCategories(t *testing.T) {
	cache, mux := activitiesServer(t)
	m := cache.Model().Activities
	night := m.Activity("act0000000001")
	if len(night.Categories) != 2 || night.Categories[1].ID != "tcg0000000008" || night.Categories[1].Adding != AddingYes {
		t.Fatalf("international night's categories: %+v", night.Categories)
	}
	if len(m.Categories) != 6 {
		t.Fatalf("the page should see only the six headings, got %d", len(m.Categories))
	}
	booth := map[string]any{"year": "2026 - 2027", "title": "Sweden", "status": StatusOpen, "directSignUp": "Yes"}
	propose := func(who, category string) int {
		return testkit.Call(t, mux, who, "POST", "/api/activities", map[string]any{
			"year": "2026 - 2027", "title": "Sweden", "parent": "act0000000001", "category": category, "status": StatusOpen, "directSignUp": "Yes",
		}).Code
	}
	sweden := madeBy(t, testkit.Call(t, mux, activitiesParent, "POST", "/api/activity-categories/tcg0000000008/add", booth))
	if got := cache.Model().Activities.Activity(sweden); got == nil || got.Title != "Sweden" || got.Parent != "act0000000001" || got.Category != "tcg0000000008" || got.Status != StatusOpen {
		t.Fatalf("a booth added under a Yes category should be open: %+v", got)
	}
	loose := madeBy(t, testkit.Call(t, mux, activitiesParent, "POST", activityAction("act0000000001", "add"), map[string]any{
		"year": "2026 - 2027", "title": "Loose Booth", "category": "", "status": StatusOpen, "directSignUp": "Yes",
	}))
	if got := cache.Model().Activities.Activity(loose); got == nil || got.Title != "Loose Booth" || got.Parent != "act0000000001" || got.Status != StatusPending {
		t.Fatalf("an uncategorised proposal should wait for approval: %+v", got)
	}
	if c, ok := activityAs(t, mux, activitiesParent, "act0000000001"); !ok || !c.Can["add"] {
		t.Fatalf("an Approval Needed event does not offer add: %v", c.Can)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/activity-categories/tcg0000000007/add", booth); rec.Code != http.StatusBadRequest {
		t.Fatalf("a proposal into a closed category went through: %d", rec.Code)
	}
	if code := propose(activitiesParent, "tcg0000000007"); code != http.StatusBadRequest {
		t.Fatalf("a proposal into a closed category went through: %d", code)
	}
	if code := propose(chair, "tcg0000000007"); code != http.StatusOK {
		t.Fatalf("the co-chair could not add into a closed category: %d", code)
	}
	if code := propose(chair, "tcg0000000001"); code != http.StatusBadRequest {
		t.Fatalf("a child took a page heading as its category: %d", code)
	}
	own := map[string]any{"eventId": "act0000000001", "title": "Performances", "allowAdding": AddingYes}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/activity-categories", own); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent made an event category: %d", rec.Code)
	}
	performances := madeBy(t, testkit.Call(t, mux, chair, "POST", "/api/activity-categories", own))
	if c := cache.Model().Activities.Category(performances); c == nil || c.Title != "Performances" || c.EventID != "act0000000001" {
		t.Fatalf("the new event category %s: %+v", performances, c)
	}
	if rec := testkit.Call(t, mux, chair, "POST", "/api/activity-categories", map[string]any{"title": "New Heading"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a co-chair made a page heading: %d", rec.Code)
	}
	if n := len(cache.Model().Activities.Activity("act0000000001").Categories); n != 3 {
		t.Fatalf("event categories after adding: %d", n)
	}
	ids := []string{}
	for _, c := range cache.Model().Activities.Activity("act0000000001").Categories {
		ids = append(ids, c.ID)
	}
	ids[0], ids[1] = ids[1], ids[0]
	if rec := testkit.Call(t, mux, activitiesParent, "POST", activityAction("act0000000001", "order-categories"), map[string]any{"ids": ids}); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent reordered an event's categories: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, chair, "POST", activityAction("act0000000001", "order-categories"), map[string]any{"ids": ids}); rec.Code != http.StatusNoContent {
		t.Fatalf("reorder: %d %s", rec.Code, rec.Body)
	}
	after := cache.Model().Activities
	got := []string{}
	for _, c := range after.Activity("act0000000001").Categories {
		got = append(got, c.ID)
	}
	if !slices.Equal(got, ids) || after.Categories[0].ID != "tcg0000000001" || after.Activity("act0000000013").Categories[0].ID != "tcg0000000009" {
		t.Fatalf("reorder: %v, or it leaked out of its scope", got)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", activityAction("act0000000001", "order-categories"), map[string]any{"ids": ids[1:]}); rec.Code != http.StatusBadRequest {
		t.Fatalf("an order missing one was taken: %d", rec.Code)
	}
	made := madeBy(t, testkit.Call(t, mux, jordan, "POST", activityAction("act0000000001", "copy"), nil))
	next := byTitle(cache.Model().Activities, "2027 - 2028", "International Night")
	if next == nil || next.ID != made || len(next.Categories) != 3 || next.Categories[0].ID == ids[0] || next.Categories[0].Title != after.Activity("act0000000001").Categories[0].Title {
		t.Fatalf("copied categories: %+v", next.Categories)
	}
	norway := byTitle(cache.Model().Activities, "2027 - 2028", "Norway")
	if norway == nil || cache.Model().Activities.Category(norway.Category).EventID != next.ID {
		t.Fatalf("copied child still names the old event's category: %+v", norway)
	}
}

func TestUncategorizedFallback(t *testing.T) {
	cache, mux := activitiesServer(t)
	if len(cache.Model().Activities.Categories) != 6 {
		t.Fatalf("the heading appeared without anything in it")
	}
	next := activitiesTables(t)
	next[activitiesTab] = append(next[activitiesTab],
		store.Row{"Event ID": "E900", CalendarEventColumn: "tev0000000900", "Year": "2026 - 2027", "Title": "Blank", "Status": StatusOpen},
		store.Row{"Event ID": "E901", CalendarEventColumn: "tev0000000901", "Year": "2026 - 2027", "Title": "Unknown", "Category": "nope", "Status": StatusOpen},
		store.Row{"Event ID": "E902", CalendarEventColumn: "tev0000000902", "Year": "2026 - 2027", "Title": "Borrowed", "Category": "tcg0000000007", "Status": StatusOpen},
		store.Row{"Event ID": "E903", CalendarEventColumn: "tev0000000903", "Year": "2026 - 2027", "Title": "Child", "Parent": "act0000000001", "Category": "tcg0000000009", "Status": StatusOpen},
	)
	m, err := BuildActivities(context.Background(), next, activitiesBundled)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"E900", "E901", "E902"} {
		if got := m.Activity(id).Category; got != UncategorizedID {
			t.Fatalf("%s: category %q", id, got)
		}
	}
	if last := m.Categories[len(m.Categories)-1]; last.ID != UncategorizedID || !last.BuiltIn || len(m.Categories) != 7 {
		t.Fatalf("categories: %+v", m.Categories)
	}
	if got := m.Activity("E903").Category; got != "" {
		t.Fatalf("child category %q", got)
	}
	add := map[string]any{"year": "2026 - 2027", "title": "Loose End", "category": UncategorizedID, "status": StatusOpen}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/activities", add); rec.Code != http.StatusBadRequest {
		t.Fatalf("a proposal without a category went through: %d", rec.Code)
	}
	made := madeBy(t, testkit.Call(t, mux, jordan, "POST", "/api/activities", add))
	loose := byTitle(cache.Model().Activities, "2026 - 2027", "Loose End")
	if loose == nil || loose.ID != made || loose.Category != UncategorizedID || cache.Count(activitiesAppName, activitiesTab, store.Row{"Title": "Loose End", "Category": ""}) != 1 {
		t.Fatalf("loose end: %+v", loose)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/activity-categories/"+UncategorizedID+"/edit", map[string]any{"title": "Misc"}); rec.Code != http.StatusNotFound {
		t.Fatalf("edited the built-in heading: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, jordan, "DELETE", "/api/activity-categories/"+UncategorizedID, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("deleted the built-in heading: %d", rec.Code)
	}
	if c := cache.Model().Activities.Category(UncategorizedID); c == nil || c.Title == "Misc" {
		t.Fatalf("the built-in heading after the refusals: %+v", c)
	}
	if got := teamAs(t, mux, jordan).settings.Uncategorized; got == nil || got.ID != UncategorizedID || !got.BuiltIn {
		t.Fatalf("the built-in heading is not in the settings: %+v", got)
	}
}

func TestReorderPageCategories(t *testing.T) {
	cache, mux := activitiesServer(t)
	ids := []string{}
	for _, c := range cache.Model().Activities.Categories {
		if !c.BuiltIn {
			ids = append(ids, c.ID)
		}
	}
	slices.Reverse(ids)
	for _, who := range []string{activitiesParent, chair} {
		if rec := testkit.Call(t, mux, who, "POST", teamSettingsAction("order-categories"), map[string]any{"ids": ids}); rec.Code != http.StatusForbidden {
			t.Fatalf("%s reordered the page's categories: %d", who, rec.Code)
		}
	}
	if rec := testkit.Call(t, mux, jordan, "POST", teamSettingsAction("order-categories"), map[string]any{"ids": ids[1:]}); rec.Code != http.StatusBadRequest {
		t.Fatalf("an order missing one was taken: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", teamSettingsAction("order-categories"), map[string]any{"ids": ids}); rec.Code != http.StatusNoContent {
		t.Fatalf("reorder: %d %s", rec.Code, rec.Body)
	}
	got := []string{}
	for _, c := range cache.Model().Activities.Categories {
		if !c.BuiltIn {
			got = append(got, c.ID)
		}
	}
	if !slices.Equal(got, ids) || cache.Model().Activities.Activity("act0000000001").Categories[0].ID != "tcg0000000007" {
		t.Fatalf("page order %v, want %v, or it leaked into an event", got, ids)
	}
}

func TestActivitiesBrokenSheetRefusesToLoad(t *testing.T) {
	t.Chdir("../..")
	broken := t.TempDir()
	if err := os.CopyFS(broken, os.DirFS("sampledata")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "events", "Volunteers.csv"), []byte("Year,Activity,Role,Email,Position,Note,Added By,Added\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := &data.Dir{Root: broken}
	deps := sampleDeps(sampleKey)
	deps.Activities = activitiesBundled
	if _, err := NewStore(dir, dir, store.NewQueue(), deps); err == nil || !strings.Contains(err.Error(), `missing column "Event ID"`) {
		t.Fatalf("a broken sheet loaded: %v", err)
	}
}

func TestVolunteerSettingsInherit(t *testing.T) {
	activitiesServer(t)
	next := activitiesTables(t)
	set := func(tab, key, id, column, value string) {
		for _, row := range next[tab] {
			if row[key] == id {
				row[column] = value
				return
			}
		}
		t.Fatalf("no %s row %s", tab, id)
	}
	for _, id := range []string{"act0000000001", "act0000000016", "act0000000019", "act0000000020"} {
		set(activitiesTab, "Event ID", id, "Direct Sign-Up", "")
		set(activitiesTab, "Event ID", id, "Volunteers Hidden", "")
	}
	set(activitiesTab, "Event ID", "act0000000001", "Direct Sign-Up", "No")
	set(activitiesTab, "Event ID", "act0000000001", "Volunteers Hidden", "Yes")
	set(activityCategoriesTab, "Category ID", "tcg0000000007", "Direct Sign-Up", "Yes")
	set(activityCategoriesTab, "Category ID", "tcg0000000008", "Hidden", "Yes")
	set(activitiesTab, "Event ID", "act0000000020", "Direct Sign-Up", "Yes")
	next[activitiesTab] = append(next[activitiesTab], store.Row{"Event ID": "E920", CalendarEventColumn: "tev0000000920", "Title": "Chai Stand", "Parent": "act0000000020", "Status": StatusOpen})
	m, err := BuildActivities(context.Background(), next, activitiesBundled)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		id             string
		direct, hidden bool
	}{
		{"act0000000001", false, true},
		{"act0000000016", true, true},
		{"act0000000019", false, true},
		{"act0000000020", true, true},
		{"E920", true, true},
	}
	for _, c := range cases {
		a := m.Activity(c.id)
		if a.DirectSignUp != c.direct || a.VolunteersHidden != c.hidden {
			t.Errorf("%s %q: allow volunteers %v, hidden %v; want %v, %v", c.id, a.Title, a.DirectSignUp, a.VolunteersHidden, c.direct, c.hidden)
		}
	}
	if c := m.Category("tcg0000000008"); c.DirectSignUp || !c.VolunteersHidden || !c.Hidden {
		t.Errorf("booths category resolved %+v", c)
	}
	norway := m.Activity("act0000000019")
	if m.VisibleTo(norway, access.Actor{}) || m.VisibleTo(m.Activity("E920"), access.Actor{}) {
		t.Errorf("an activity in a hidden category is visible to a stranger")
	}
	if !m.VisibleTo(norway, activityViewerOf(jordan, true)) {
		t.Errorf("an activity in a hidden category is hidden from an admin")
	}
	if !m.VisibleTo(m.Activity("act0000000016"), access.Actor{}) {
		t.Errorf("an activity in a shown category is hidden")
	}
}

func TestChairSetsAllowAddingOnTheirEvent(t *testing.T) {
	cache, _ := activitiesServer(t)
	patch := func(policy string) activityPatch {
		return activityPatch{"id": json.RawMessage(`"act0000000001"`), "allowAdding": json.RawMessage(`"` + policy + `"`)}
	}
	s, err := cache.Model().Activities.saveActivity(activityViewerOf(chair, false), patch(AddingApproval))
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Commit(context.Background(), activityViewerOf(chair, false), activitiesAppName, s.ops...); err != nil {
		t.Fatal(err)
	}
	if got := cache.Model().Activities.Activity("act0000000001").AllowAdding; got != AddingApproval {
		t.Fatalf("the event's chair set allow adding to %q, want %q", got, AddingApproval)
	}
	if _, err := cache.Model().Activities.saveActivity(activityViewerOf(activitiesParent, false), patch(AddingYes)); testkit.Status(t, err) != http.StatusForbidden {
		t.Fatalf("a parent changed allow adding: %v", err)
	}
}

func TestCategorySettings(t *testing.T) {
	cache, mux := activitiesServer(t)
	m := cache.Model().Activities
	body := categoryFlagsBody{ID: "tcg0000000008", Flags: map[string]string{"directSignUp": "No", "hidden": "", "allowAdding": AddingApproval}}
	if _, _, err := m.saveCategoryFlags(activityViewerOf(activitiesParent, false), body); testkit.Status(t, err) != http.StatusForbidden {
		t.Fatalf("a parent changed a category's settings: %v", err)
	}
	if _, _, err := m.saveCategoryFlags(activityViewerOf(jordan, true), categoryFlagsBody{ID: "tcg0000000001", Flags: body.Flags}); testkit.Status(t, err) != http.StatusBadRequest {
		t.Fatalf("a page heading took volunteer settings: %v", err)
	}
	if _, _, err := m.saveCategoryFlags(activityViewerOf(jordan, true), categoryFlagsBody{ID: "tcg0000000008", Flags: map[string]string{"directSignUp": "Maybe"}}); testkit.Status(t, err) != http.StatusBadRequest {
		t.Fatalf("a bad value was taken: %v", err)
	}
	op, _, err := m.saveCategoryFlags(activityViewerOf(chair, false), body)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Commit(context.Background(), activityViewerOf(chair, false), activitiesAppName, op); err != nil {
		t.Fatal(err)
	}
	if c := cache.Model().Activities.Category("tcg0000000008"); c.DirectSignUpOwn != "No" || c.DirectSignUp || c.Adding != AddingApproval {
		t.Fatalf("after saving: %+v", c)
	}
	flags := map[string]any{"flags": map[string]string{"volunteersHidden": "Yes"}}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/activity-categories/tcg0000000008/settings", flags); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent changed a category's settings: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/activity-categories/tcg0000000001/settings", flags); rec.Code != http.StatusBadRequest {
		t.Fatalf("a page heading took volunteer settings: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, chair, "POST", "/api/activity-categories/tcg0000000008/settings", flags); rec.Code != http.StatusNoContent {
		t.Fatalf("the co-chair's settings: %d %s", rec.Code, rec.Body)
	}
	if c := cache.Model().Activities.Category("tcg0000000008"); !c.VolunteersHidden {
		t.Fatalf("after saving through the API: %+v", c)
	}
}

func TestHandWrittenRows(t *testing.T) {
	cache, _ := activitiesServer(t)
	next := activitiesTables(t)
	next[activitiesTab] = append(next[activitiesTab],
		store.Row{"Event ID": "E900", CalendarEventColumn: "tev0000000900", "Title": "Bare Child", "Parent": "act0000000020", "Status": StatusOpen},
		store.Row{"Event ID": "", "Title": "Gone", "Year": "nonsense", "Status": "Active", "Parent": "nope"},
	)
	next[volunteersTab] = append(next[volunteersTab], store.Row{"Event ID": "", "Email": "not an email", "Position": "Boss"})
	next[linksTab] = append(next[linksTab], store.Row{"Link ID": "hrf0000000901", "Event ID": "", "Title": "Old", "URL": "https://example.com"})
	m, err := BuildActivities(context.Background(), next, activitiesBundled)
	if err != nil {
		t.Fatal(err)
	}
	bare := m.Activity("E900")
	if bare == nil || bare.Year != "2026 - 2027" || !bare.CoLeaderNeeded || !bare.DirectSignUp || bare.VolunteersHidden {
		t.Fatalf("bare child: %+v", bare)
	}
	if byTitle(m, "2026 - 2027", "Gone") != nil {
		t.Fatalf("a row without an id was loaded")
	}
	next[activitiesTab] = append(next[activitiesTab],
		store.Row{"Event ID": "E910", CalendarEventColumn: "tev0000000910", "Title": "Orphan", "Parent": "gone-id", "Status": StatusOpen},
		store.Row{"Event ID": "E911", CalendarEventColumn: "tev0000000911", "Title": "Orphan's Child", "Parent": "E910", "Status": StatusOpen},
	)
	next[volunteersTab] = append(next[volunteersTab],
		store.Row{"Event ID": "gone-id", "Email": activitiesParent, "Position": PositionVolunteer},
		store.Row{"Event ID": "act0000000001", "Email": chair, "Position": PositionVolunteer},
	)
	next[linksTab] = append(next[linksTab], store.Row{"Link ID": "hrf0000000902", "Event ID": "E910", "Title": "Lost", "URL": "https://example.com"})
	m, err = BuildActivities(context.Background(), next, activitiesBundled)
	if err != nil {
		t.Fatal(err)
	}
	if m.Activity("E910") != nil || m.Activity("E911") != nil {
		t.Fatalf("orphans were loaded")
	}
	want := Skipped{Deleted: 1, Orphans: 2, Volunteers: 2, Links: 2, Duplicates: 1}
	if m.Skipped != want {
		t.Fatalf("skipped %+v, want %+v", m.Skipped, want)
	}
	if n := len(m.Activity("act0000000001").Volunteers); n != len(cache.Model().Activities.Activity("act0000000001").Volunteers) {
		t.Fatalf("the duplicate sign-up was added: %d volunteers", n)
	}
	next[activitiesTab] = append(next[activitiesTab], store.Row{"Event ID": "E901", CalendarEventColumn: "tev0000000901", "Title": "Lost", "Year": "2025 - 2026", "Parent": "act0000000020", "Status": StatusOpen})
	if _, err := BuildActivities(context.Background(), next, activitiesBundled); err == nil || !strings.Contains(err.Error(), "has its parent") {
		t.Fatalf("a child in another year loaded: %v", err)
	}
	next = activitiesTables(t)
	next[activitiesTab][20][store.OrderColumn] = "10"
	if _, err := BuildActivities(context.Background(), next, activitiesBundled); err == nil || !strings.Contains(err.Error(), "ends in 0") {
		t.Fatalf("an order ending in 0 loaded: %v", err)
	}
	next = activitiesTables(t)
	next[activitiesTab][1][CalendarEventColumn] = next[activitiesTab][0][CalendarEventColumn]
	if _, err := BuildActivities(context.Background(), next, activitiesBundled); err == nil || !strings.Contains(err.Error(), "share calendar event id") {
		t.Fatalf("two activities with one calendar event id loaded: %v", err)
	}
	next = activitiesTables(t)
	next[activitiesTab][1][CalendarEventColumn] = ""
	if _, err := BuildActivities(context.Background(), next, activitiesBundled); err == nil || !strings.Contains(err.Error(), "calendar event id") {
		t.Fatalf("an activity with no calendar event id loaded: %v", err)
	}
}

func TestShowOnMainPage(t *testing.T) {
	cache, mux := activitiesServer(t)
	if c := cache.Model().Activities.Category("tcg0000000001"); !c.ShowOnMain {
		t.Fatalf("a heading defaults to being shown: %+v", c)
	}
	next := activitiesTables(t)
	next[activityCategoriesTab][0]["Show On Main Page"] = ""
	next[activityCategoriesTab][6]["Show On Main Page"] = "No"
	m, err := BuildActivities(context.Background(), next, activitiesBundled)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Category("tcg0000000001").ShowOnMain || !m.Category("tcg0000000007").ShowOnMain {
		t.Fatalf("blank or event-scoped categories should be shown")
	}
	off := false
	edit := map[string]any{"title": "Activities", "allowAdding": AddingNo, "showOnMain": off}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/activity-categories/tcg0000000002/edit", edit); rec.Code != http.StatusNoContent {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Activities.Category("tcg0000000002").ShowOnMain || cache.Count(activitiesAppName, activityCategoriesTab, store.Row{"Category ID": "tcg0000000002", "Show On Main Page": "No"}) != 1 {
		t.Fatalf("the heading was not taken off the page")
	}
	own := map[string]any{"eventId": "act0000000001", "title": "Shifts", "allowAdding": "", "showOnMain": off}
	madeBy(t, testkit.Call(t, mux, chair, "POST", "/api/activity-categories", own))
	if cache.Count(activitiesAppName, activityCategoriesTab, store.Row{"Title": "Shifts", "Show On Main Page": ""}) != 1 {
		t.Fatalf("an event category carried the page flag")
	}
}

func TestPrettyIDs(t *testing.T) {
	cache, mux := activitiesServer(t)
	m := cache.Model().Activities
	if m.ByPretty("International-Night") != m.Activity("act0000000001") || m.ByPretty("nope") != nil {
		t.Fatalf("pretty lookup")
	}
	edit := func(id, pretty string, takeOver bool) *httptest.ResponseRecorder {
		act := cache.Model().Activities.Activity(id)
		return testkit.Call(t, mux, jordan, "POST", activityAction(id, "edit"), map[string]any{
			"year": act.Year, "title": act.Title, "category": act.Category, "status": act.Status,
			"directSignUp": "Yes", "prettyId": pretty, "takeOver": takeOver,
		})
	}
	if rec := edit("act0000000002", "Bad Address!", false); rec.Code != http.StatusBadRequest {
		t.Fatalf("a pretty id with spaces went through: %d", rec.Code)
	}
	rec := edit("act0000000002", "international-night", false)
	var conflict activityPrettyConflict
	if rec.Code != http.StatusConflict || json.Unmarshal(rec.Body.Bytes(), &conflict) != nil || conflict.Prior || conflict.ID != "act0000000001" {
		t.Fatalf("same-year clash: %d %s", rec.Code, rec.Body)
	}
	if rec := edit("act0000000002", "international-night", true); rec.Code != http.StatusConflict {
		t.Fatalf("take-over of a current address went through: %d", rec.Code)
	}
	last := byTitle(cache.Model().Activities, "2025 - 2026", "International Night")
	rec = edit("act0000000002", "international-night-2025", false)
	if rec.Code != http.StatusConflict || json.Unmarshal(rec.Body.Bytes(), &conflict) != nil || !conflict.Prior || conflict.ID != last.ID || conflict.Renamed != "international-night-2025-2025" {
		t.Fatalf("prior-year clash: %d %s", rec.Code, rec.Body)
	}
	if rec := edit("act0000000002", "international-night-2025", true); rec.Code != http.StatusNoContent {
		t.Fatalf("take-over: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model().Activities
	if m.Activity("act0000000002").PrettyID != "international-night-2025" || m.Activity(last.ID).PrettyID != "international-night-2025-2025" {
		t.Fatalf("after take-over: %q %q", m.Activity("act0000000002").PrettyID, m.Activity(last.ID).PrettyID)
	}
	if n := cache.Count(activitiesAppName, activityRedirectsTab, store.Row{"Old": "/v/international-night-2025"}); n != 0 {
		t.Fatalf("the address taken over was redirected to the displaced event: %+v", m.Redirects)
	}
	if m.Resolve("intl-night") != m.Activity("act0000000001") || m.Resolve("/v/intl-night") != m.Activity("act0000000001") || m.Resolve("nope") != nil {
		t.Fatalf("sample redirect")
	}
	if rec := edit("act0000000002", "spring-party", false); rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	if rec := edit("act0000000002", "", false); rec.Code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model().Activities
	for _, old := range []string{"/v/spring-celebration", "/v/international-night-2025", "/v/spring-party", "/activities/act0000000002"} {
		if m.Resolve(old) != m.Activity("act0000000002") {
			t.Fatalf("%s did not reach the event: %+v", old, m.Redirects)
		}
	}
	if n := cache.Count(activitiesAppName, activityRedirectsTab, store.Row{"Type": RedirectActivity, "Old": "/v/spring-party", "New": "/activities/act0000000002"}); n != 1 {
		t.Fatalf("a removed address should redirect to the row: %+v", m.Redirects)
	}
	if err := cache.Commit(context.Background(), activityViewerOf(jordan, true), activitiesAppName, store.Update(activitiesTab, store.Row{"Event ID": "act0000000002"}, store.Row{"Pretty ID": "spring-fling"})); err != nil {
		t.Fatal(err)
	}
	if n := cache.Count(activitiesAppName, activityRedirectsTab, store.Row{"Type": RedirectActivity, "Old": "/activities/act0000000002", "New": "/v/spring-fling"}); n != 1 {
		t.Fatalf("a write outside the save handler moved the event without a redirect: %+v", cache.Model().Activities.Redirects)
	}
	if rec := edit("act0000000002", "international-night-2025", false); rec.Code != http.StatusNoContent {
		t.Fatalf("restore: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model().Activities
	norway, india := byTitle(m, "2026 - 2027", "Norway"), byTitle(m, "2026 - 2027", "India")
	if m.PathOf(norway) != "/v/international-night/"+norway.ID {
		t.Fatalf("child path %q", m.PathOf(norway))
	}
	child := func(node *Activity, pretty string) *httptest.ResponseRecorder {
		return testkit.Call(t, mux, jordan, "POST", activityAction(node.ID, "edit"), map[string]any{
			"year": node.Year, "title": node.Title, "parent": node.Parent, "category": node.Category, "status": node.Status,
			"directSignUp": "Yes", "prettyId": pretty,
		})
	}
	if rec := child(norway, "norway"); rec.Code != http.StatusNoContent {
		t.Fatalf("child pretty: %d %s", rec.Code, rec.Body)
	}
	if rec := child(india, "norway"); rec.Code != http.StatusConflict {
		t.Fatalf("two siblings took one address: %d", rec.Code)
	}
	m = cache.Model().Activities
	if m.Resolve("/v/international-night/norway") != m.Activity(norway.ID) || m.Resolve("/v/international-night/"+norway.ID) != m.Activity(norway.ID) || m.Resolve("/v/intl-night/norway") != m.Activity(norway.ID) {
		t.Fatalf("child by path: %q", m.PathOf(m.Activity(norway.ID)))
	}
	if rec := edit("act0000000001", "inight", false); rec.Code != http.StatusNoContent {
		t.Fatalf("rename event: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model().Activities
	if m.PathOf(m.Activity(norway.ID)) != "/v/inight/norway" || m.Resolve("/v/international-night/norway") != m.Activity(norway.ID) || m.Resolve("/v/intl-night/norway") != m.Activity(norway.ID) {
		t.Fatalf("event rename did not carry the booth: %q", m.PathOf(m.Activity(norway.ID)))
	}
	if rec := edit("act0000000002", "International-Night-2025", false); rec.Code != http.StatusNoContent {
		t.Fatalf("re-saving own address: %d %s", rec.Code, rec.Body)
	}
	next := activitiesTables(t)
	for _, row := range next[activitiesTab] {
		if row["Event ID"] == last.ID {
			row["Pretty ID"] = "inight"
		}
	}
	dup, err := BuildActivities(context.Background(), next, activitiesBundled)
	if err != nil {
		t.Fatal(err)
	}
	if dup.ByPretty("inight") != dup.Activity("act0000000001") || dup.Activity(last.ID).PrettyID != "" || dup.Skipped.PrettyIDs != 1 {
		t.Fatalf("duplicate in the sheet: %+v", dup.Skipped)
	}
}

func TestActivitiesSharePreview(t *testing.T) {
	cache, mux := activitiesServer(t)
	previews := []testkit.Preview{{
		URL: "https://team.heliosian.com/v/intl-night/",
		Want: []string{`og:title" content="International Night"`, `og:url" content="https://team.heliosian.com/v/international-night"`,
			`og:image" content="https://team.heliosian.com/open/share/act0000000001.png"`, `Thursday, September 24 · 4:00 – 6:00 PM — We invite you`},
	}, {
		URL:  "https://team.heliosian.com/v/international-night/act0000000020",
		Want: []string{`og:title" content="India · International Night"`, `Thursday, September 24 · 4:00 – 6:00 PM`},
	}}
	for _, path := range []string{"/activities/act0000000006", "/my", "/"} {
		previews = append(previews, testkit.Preview{
			URL:   "https://team.heliosian.com" + path,
			Want:  []string{`og:title" content="HCA-Team"`, `og:image" content="https://team.heliosian.com/open/share/upcoming.png"`, "Volunteers needed: "},
			Never: []string{"act0000000006"},
		})
	}
	testkit.Previews(t, ActivitiesPreviewHead(cache, activitiesTestStyle), previews...)
	list := needs(cache.Model().Activities, now())
	if len(list) == 0 {
		t.Fatalf("nothing needs hands in the sample")
	}
	last := ""
	for _, a := range list {
		if a.Status != StatusOpen || a.VolunteersComplete || (a.Spots > 0 && len(a.Volunteers) >= a.Spots) || a.Parent != "" {
			t.Fatalf("%s (%s) is not a need", a.Title, a.ID)
		}
		if a.Start != "" && last != "" && a.Start < last {
			t.Fatalf("%s comes after %s", a.Title, last)
		}
		if a.Start != "" && a.Start[:10] < "2026-09-09" {
			t.Fatalf("%s is past", a.Title)
		}
		last = a.Start
	}
	if note := needNote(&Activity{Start: "2026-09-24", Spots: 3, Volunteers: []Volunteer{{}}}); note != "Thursday, September 24 · 2 spots left" {
		t.Fatalf("note: %q", note)
	}
	testkit.Cards(t, mux, []string{"/open/share/act0000000001.png", "/open/share/upcoming.png"}, "/open/share/act0000000006.png")
	for _, status := range []string{StatusHidden, StatusPending} {
		next := activitiesTables(t)
		for _, row := range next[activitiesTab] {
			if row["Event ID"] == "act0000000001" {
				row["Status"], row["Added By"] = status, ""
			}
		}
		parked, err := BuildActivities(context.Background(), next, activitiesBundled)
		if err != nil {
			t.Fatal(err)
		}
		if child := parked.Activity("act0000000020"); child.Status != StatusOpen || activityPreviewable(parked, child) {
			t.Fatalf("%s parent: %s (%s) previews", status, child.Title, child.Status)
		}
		for _, a := range needs(parked, now()) {
			if a.ID == "act0000000001" || a.Parent == "act0000000001" {
				t.Fatalf("%s parent: %s is a need", status, a.Title)
			}
		}
	}
}

func TestWhenSpansDays(t *testing.T) {
	for _, tc := range []struct {
		start, end, line, day, hours string
	}{
		{"2026-09-24", "", "Thursday, September 24", "Thursday, September 24", ""},
		{"2026-09-24 16:00", "2026-09-24 18:00", "Thursday, September 24 · 4:00 – 6:00 PM", "Thursday, September 24", "4:00 – 6:00 PM"},
		{"2026-09-24 11:00", "2026-09-24 13:00", "Thursday, September 24 · 11:00 AM – 1:00 PM", "Thursday, September 24", "11:00 AM – 1:00 PM"},
		{"2026-10-02", "2026-10-04", "Friday, October 2 – Sunday, October 4", "Friday, October 2 – Sunday, October 4", ""},
		{"2026-10-02 16:00", "2026-10-04 12:00", "Friday, October 2 – Sunday, October 4 · Fri 4:00 PM – Sun 12:00 PM", "Friday, October 2 – Sunday, October 4", "Fri 4:00 PM – Sun 12:00 PM"},
		{"2026-10-30 09:00", "2026-11-01", "Friday, October 30 – Sunday, November 1 · Fri 9:00 AM", "Friday, October 30 – Sunday, November 1", "Fri 9:00 AM"},
	} {
		a := &Activity{Start: tc.start, End: tc.end}
		if got := whenText(a); got != tc.line {
			t.Errorf("whenText(%q, %q) = %q, want %q", tc.start, tc.end, got, tc.line)
		}
		if day, hours := activityWhenLines(a); day != tc.day || hours != tc.hours {
			t.Errorf("whenLines(%q, %q) = %q, %q, want %q, %q", tc.start, tc.end, day, hours, tc.day, tc.hours)
		}
	}
}

func TestRedirects(t *testing.T) {
	cache, mux := activitiesServer(t)
	for cell, want := range map[string]string{
		"https://hca.heliosian.com/dl/signup/s/768d91/r/nsSPomxFcPrSfoRCAgzI": "/dl/signup/s/768d91/r/nsSPomxFcPrSfoRCAgzI",
		" /dl/signup/s/768d91/ ": "/dl/signup/s/768d91", "intl-night": "/v/intl-night", "https://hca.heliosian.com": "/",
	} {
		if got := activityRedirectPath(cell); got != want {
			t.Errorf("redirectPath(%q) = %q, want %q", cell, got, want)
		}
	}
	if got := redirectTo("https://celebrate.heliosian.com/parties/abc "); got != "https://celebrate.heliosian.com/parties/abc" {
		t.Errorf("redirectTo kept %q", got)
	}
	if got := redirectTo("spring-celebration"); got != "/v/spring-celebration" {
		t.Errorf("redirectTo path %q", got)
	}
	oldLink := "https://hca.heliosian.com/dl/signup/s/768d91/r/nsSPomxFcPrSfoRCAgzI"
	if rec := testkit.Call(t, mux, chair, "POST", "/api/activity-redirects", map[string]string{"old": oldLink, "new": "/v/international-night"}); rec.Code != http.StatusForbidden {
		t.Fatalf("chair: %d %s", rec.Code, rec.Body)
	}
	oldPath := "/dl/signup/s/768d91/r/nsSPomxFcPrSfoRCAgzI"
	if made := madeBy(t, testkit.Call(t, mux, jordan, "POST", "/api/activity-redirects", map[string]string{"old": oldLink, "new": "/v/international-night"})); made != redirectKeyOf(oldPath) {
		t.Fatalf("add answered %s, want %s", made, redirectKeyOf(oldPath))
	}
	if rec := testkit.Call(t, mux, chair, "GET", "/api/activity-redirects/"+redirectKeyOf(oldPath), nil); rec.Code != http.StatusNotFound {
		t.Fatalf("a chair read a redirect: %d %s", rec.Code, rec.Body)
	}
	m := cache.Model().Activities
	if n := cache.Count(activitiesAppName, activityRedirectsTab, store.Row{"Type": RedirectAdmin, "Old": oldPath, "New": "/v/international-night"}); n != 1 {
		t.Fatalf("row not written: %+v", m.Redirects)
	}
	if m.Resolve(oldPath) != m.Activity("act0000000001") || m.Destination(oldPath) != "/v/international-night" {
		t.Fatalf("old link: %v %q", m.Resolve(oldPath), m.Destination(oldPath))
	}
	if got := m.Destination("/v/intl-night"); got != "/v/international-night" {
		t.Fatalf("rename redirect: %q", got)
	}
	if got := m.Destination("/v/intl-night/" + byTitle(m, "2026 - 2027", "India").ID); got != "/v/international-night/"+byTitle(m, "2026 - 2027", "India").ID {
		t.Fatalf("under a renamed event: %q", got)
	}
	for _, live := range []string{"/v/international-night", "/", "/calendar", "/my", "/nowhere"} {
		if got := m.Destination(live); got != "" {
			t.Fatalf("%s should be served, not sent to %q", live, got)
		}
	}
	for _, bad := range []map[string]string{
		{"old": "/calendar", "new": "/v/international-night"}, {"old": "https://hca.heliosian.com/", "new": "/v/international-night"},
		{"old": "/api/team/model", "new": "/v/international-night"}, {"old": "/v/international-night", "new": "/v/intl-night"},
		{"old": "somewhere", "new": "/v/Somewhere"}, {"old": "/a", "new": "/b"}, {"old": oldPath, "new": "/elsewhere"}, {"old": "", "new": "/x"}, {"old": "/x", "new": ""},
	} {
		if bad["old"] == "/a" {
			madeBy(t, testkit.Call(t, mux, jordan, "POST", "/api/activity-redirects", map[string]string{"old": "/b", "new": "/a"}))
		}
		if rec := testkit.Call(t, mux, jordan, "POST", "/api/activity-redirects", bad); rec.Code < 400 {
			t.Errorf("%v was accepted", bad)
		}
	}
	elsewhere := "https://celebrate.heliosian.com/parties/abc"
	if rec := testkit.Call(t, mux, chair, "POST", "/api/activity-redirects/"+redirectKeyOf(oldPath)+"/edit", map[string]string{"old": "/dl/signup/s/768d91", "new": elsewhere}); rec.Code != http.StatusNotFound {
		t.Fatalf("a chair edited a redirect: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/activity-redirects/"+redirectKeyOf(oldPath)+"/edit", map[string]string{"old": "/dl/signup/s/768d91", "new": elsewhere}); rec.Code != http.StatusNoContent {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model().Activities
	if n := cache.Count(activitiesAppName, activityRedirectsTab, store.Row{"Old": oldPath}); n != 0 {
		t.Fatalf("old row still there")
	}
	if got := m.Destination(oldPath); got != elsewhere+"/r/nsSPomxFcPrSfoRCAgzI" {
		t.Fatalf("under the edited prefix: %q", got)
	}
	if m.Resolve(oldPath) != nil {
		t.Fatalf("a chain that leaves the site names no activity")
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/activity-redirects/"+redirectKeyOf(activityRedirectPath("intl-night"))+"/edit", map[string]string{"old": "/v/intl-nite", "new": "/v/international-night"}); rec.Code != http.StatusNoContent {
		t.Fatalf("edit the sample row: %d %s", rec.Code, rec.Body)
	}
	if n := cache.Count(activitiesAppName, activityRedirectsTab, store.Row{"Type": "pretty", "Old": "/v/intl-nite"}); n != 1 {
		t.Fatalf("the sample row's kind was not kept: %+v", cache.Model().Activities.Redirects)
	}
	madeBy(t, testkit.Call(t, mux, jordan, "POST", "/api/activity-redirects", map[string]string{"old": "/v/fair", "new": "/"}))
	handler := ActivitiesRedirected(cache, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	for path, want := range map[string]string{
		"/dl/signup/s/768d91/r/nsSPomxFcPrSfoRCAgzI?x=1": elsewhere + "/r/nsSPomxFcPrSfoRCAgzI?x=1",
		"/v/intl-nite": "https://example.com/v/international-night", "/v/international-night": "", "/calendar": "", "/nowhere": "",
		"/v/fair": "https://example.com/", "/v/fair/calendar": "https://example.com/calendar", "/v/fair//elsewhere.example": "", "/v/fair/\\elsewhere.example": "",
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if want == "" {
			if rec.Code != http.StatusTeapot {
				t.Errorf("%s: %d, want served", path, rec.Code)
			}
		} else if rec.Code != http.StatusFound || rec.Header().Get("Location") != want {
			t.Errorf("%s: %d %q, want %q", path, rec.Code, rec.Header().Get("Location"), want)
		}
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("POST", "/v/intl-nite", nil))
	if rec.Code != http.StatusTeapot {
		t.Errorf("a POST is never redirected: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, jordan, "DELETE", "/api/activity-redirects/"+redirectKeyOf("/dl/signup/s/768d91"), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Activities.Destination(oldPath); got != "" {
		t.Fatalf("deleted redirect still sends to %q", got)
	}
	if rec := testkit.Call(t, mux, jordan, "DELETE", "/api/activity-redirects/"+redirectKeyOf("/dl/signup/s/768d91"), nil); rec.Code != http.StatusNotFound {
		t.Fatalf("delete again: %d %s", rec.Code, rec.Body)
	}
}
