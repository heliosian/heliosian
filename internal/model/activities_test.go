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

func activitiesServeWith(t *testing.T, mailer *mail.Mailgun) (*ActivitiesCache, *http.ServeMux) {
	t.Helper()
	t.Chdir("../..")
	sheet = &data.Dir{Root: "sampledata"}
	queue = store.NewQueue()
	var err error
	if directory, err = LoadDirectory(sheet, nil, testkit.None, []byte("test")); err != nil {
		t.Fatal(err)
	}
	cache, err := NewActivitiesCache(sheet, sheet, activitiesBundled, func() []string { return []string{jordan} }, queue)
	if err != nil {
		t.Fatal(err)
	}
	parties, err := NewPartiesCache(sheet, sheet, testkit.All, func() []string { return nil }, queue)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterActivities(mux, ActivitiesDeps{
		Cache:     cache,
		Images:    blob.NewImages(blob.New(blob.NewMemoryBucket()), "team"),
		Directory: func() *Directory { return directory },
		Settings:  func() *Config { return settings },
		Calendar:  calendarOver(t, parties, cache, func() *Directory { return directory }),
		Mailer:    mailer,
		Lists:     noEmailList,
		Style:     activitiesTestStyle,
	})
	return cache, mux
}

func noEmailList(string) string {
	return ""
}

var activitiesTestStyle = ActivitiesCardStyle(func() string { return "HCA-Team" }, func() string { return "HCA Volunteer Portal" })

func activitiesServer(t *testing.T) (*ActivitiesCache, *http.ServeMux) {
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
	for _, status := range []string{StatusOpen, StatusHidden} {
		t.Run(status, func(t *testing.T) {
			cache, mux := activitiesServer(t)
			approve := map[string]any{"id": "act0000000012", "status": status}
			if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/activity", approve); rec.Code != http.StatusForbidden {
				t.Fatalf("a parent changed an activity's status: %d", rec.Code)
			}
			before := len(activitiesChangeLog(t))
			if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/activity", approve); rec.Code != http.StatusOK {
				t.Fatalf("making act0000000012 %s: %d %s", status, rec.Code, rec.Body)
			}
			log := activitiesChangeLog(t)[before:]
			if got := cache.Model().Activity("act0000000012").Status; got != status || len(log) != 1 || log[0]["Column"] != "Status" {
				t.Fatalf("act0000000012 after making it %s: status %s, change log %v", status, got, log)
			}
		})
	}
}

func TestASaveWritesOnlyWhatItNames(t *testing.T) {
	cache, mux := activitiesServer(t)
	was := *cache.Model().Activity("act0000000002")
	before := len(activitiesChangeLog(t))
	if rec := testkit.Call(t, mux, chair, "POST", "/api/team/activity", map[string]any{"id": "act0000000002", "title": "Renamed"}); rec.Code != http.StatusOK {
		t.Fatalf("the co-chair's rename: %d %s", rec.Code, rec.Body)
	}
	log := activitiesChangeLog(t)[before:]
	got := cache.Model().Activity("act0000000002")
	if len(log) != 1 || log[0]["Column"] != "Title" || got.Title != "Renamed" || got.Description != was.Description || got.Status != was.Status || got.Category != was.Category {
		t.Fatalf("after a rename: %+v, change log %v", got, log)
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
	m := cache.Model()
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
	cache, _ := activitiesServer(t)
	m := cache.Model()
	all := []*Activity{}
	for _, root := range m.Activities {
		all = append(append(all, root), root.Descendants()...)
	}
	for _, c := range []struct {
		email string
		admin bool
	}{{activitiesParent, false}, {"elena.torres@heliosschool.org", false}, {m.Activity("act0000000001").CoChairs()[0], false}, {activitiesParent, true}} {
		shown := map[string]bool{}
		var walk func([]*ActivityView)
		walk = func(list []*ActivityView) {
			for _, a := range list {
				shown[a.ID] = true
				walk(a.Children)
			}
		}
		as := activityViewerOf(c.email, c.admin)
		for _, a := range RenderActivities(m, directory, settings, noRSVPs, noEmailList, as, now()).Activities {
			shown[a.ID] = true
			walk(a.Children)
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
	cache, _ := activitiesServer(t)
	view := RenderActivities(cache.Model(), directory, settings, noRSVPs, noEmailList, activityViewerOf(activitiesParent, false), now())
	for _, a := range view.Activities {
		if a.Status == StatusHidden || a.Status == StatusPending {
			t.Errorf("%s reached a parent as %s", a.Title, a.Status)
		}
		if a.Title == "Room Parents" {
			for _, v := range a.Volunteers {
				if v.Position != PositionCoChair {
					t.Errorf("room parents list leaked %s", v.Email)
				}
			}
			if a.Taken != 1 {
				t.Errorf("room parents taken %d", a.Taken)
			}
		}
	}
	if view.User.Name != "Robin Whitfield" || len(view.User.Spouses) != 1 || view.User.Spouses[0].Email != jordan || len(view.User.Children) != 2 || view.User.Children[0] != (Child{Email: student, Name: "Sam Whitfield", Grade: "Grade 3"}) || view.People != nil {
		t.Errorf("user %+v, people %v", view.User, view.People)
	}
	suggester := RenderActivities(cache.Model(), directory, settings, noRSVPs, noEmailList, activityViewerOf("elena.torres@heliosschool.org", false), now())
	found := false
	for _, a := range suggester.Activities {
		if a.Title == "Family Escape Room Night" {
			found = true
		}
	}
	if !found {
		t.Error("a suggester cannot see their own pending suggestion")
	}
	if got := RenderActivities(cache.Model(), directory, settings, noRSVPs, noEmailList, activityViewerOf("someone.new@heliosschool.org", false), now()).User.Name; got != "Someone New" {
		t.Errorf("display name %q", got)
	}
}

func TestActivityForPrivateList(t *testing.T) {
	cache, _ := activitiesServer(t)
	m := cache.Model()
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
	body := map[string]any{"id": "act0000000017", "position": PositionOpen, "note": "happy to help"}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/volunteer", body); rec.Code != http.StatusNoContent {
		t.Fatalf("sign up: %d %s", rec.Code, rec.Body)
	}
	role := cache.Model().Activity("act0000000017")
	if len(role.Volunteers) != 1 || role.Volunteers[0].Email != activitiesParent || role.Volunteers[0].AddedBy != activitiesParent {
		t.Fatalf("volunteers after sign up: %+v", role.Volunteers)
	}
	body["position"] = PositionCoChair
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/volunteer", body); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent named themselves co-chair: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, chair, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000017", "email": activitiesParent, "position": PositionCoChair}); rec.Code != http.StatusNoContent {
		t.Fatalf("a co-chair could not promote: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000017", "position": PositionCoChair, "note": "here to lead"}); rec.Code != http.StatusNoContent {
		t.Fatalf("a co-chair could not edit their own note: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000017", "position": PositionVolunteer}); rec.Code != http.StatusNoContent {
		t.Fatalf("a co-chair could not step down: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000017", "position": PositionCoChair}); rec.Code != http.StatusForbidden {
		t.Fatalf("a volunteer named themselves co-chair: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000017", "email": activitiesParent, "position": PositionCoChair}); rec.Code != http.StatusNoContent {
		t.Fatalf("an admin could not promote: %d %s", rec.Code, rec.Body)
	}
	full := map[string]any{"id": "act0000000026", "position": PositionVolunteer}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/volunteer", full); rec.Code != http.StatusBadRequest {
		t.Fatalf("a full role took a sign-up: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000001", "email": "Facilities@heliosschool.org", "position": PositionVolunteer}); rec.Code != http.StatusNoContent {
		t.Fatalf("an admin could not sign up an alias: %d %s", rec.Code, rec.Body)
	}
	if v := cache.Model().Activity("act0000000001").volunteer("hank.morrow@heliosschool.org"); v == nil {
		t.Fatalf("a sign-up by alias was not stored as the directory's address: %+v", cache.Model().Activity("act0000000001").Volunteers)
	}
	for _, as := range []string{activitiesParent, jordan} {
		if rec := testkit.Call(t, mux, as, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000017", "email": "x@elsewhere.example", "position": PositionVolunteer}); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s signed up an address outside the directory: %d", as, rec.Code)
		}
	}
	direct := map[string]any{"id": "act0000000001", "position": PositionVolunteer}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/volunteer", direct); rec.Code != http.StatusBadRequest {
		t.Fatalf("an activity without direct sign-up took one: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, "someone.else@heliosschool.org", "DELETE", "/api/team/volunteer", map[string]any{"id": "act0000000017", "email": activitiesParent}); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger removed someone: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000017", "email": activitiesKid, "position": PositionVolunteer}); rec.Code != http.StatusNoContent {
		t.Fatalf("a parent could not sign their child up: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000017", "email": activitiesKid, "position": PositionVolunteer, "note": "after school only"}); rec.Code != http.StatusNoContent {
		t.Fatalf("a parent could not edit their child's sign-up: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, activitiesKid, "DELETE", "/api/team/volunteer", map[string]any{"id": "act0000000017", "email": activitiesParent}); rec.Code != http.StatusForbidden {
		t.Fatalf("a child removed their parent: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "DELETE", "/api/team/volunteer", map[string]any{"id": "act0000000017", "email": activitiesKid}); rec.Code != http.StatusNoContent {
		t.Fatalf("a parent could not remove their child: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "DELETE", "/api/team/volunteer", map[string]any{"id": "act0000000017", "email": activitiesParent}); rec.Code != http.StatusNoContent {
		t.Fatalf("remove self: %d %s", rec.Code, rec.Body)
	}
	if n := len(cache.Model().Activity("act0000000017").Volunteers); n != 0 {
		t.Fatalf("%d volunteers left after removal", n)
	}
}

func TestActivitiesMail(t *testing.T) {
	rec := mailtest.NewRecorder(mailtest.From)
	_, mux := activitiesServeWith(t, rec.Mailgun)
	if r := testkit.Call(t, mux, jordan, "POST", "/api/team/notify", map[string]any{"kinds": []string{"signups", "offers"}}); r.Code != http.StatusNoContent {
		t.Fatalf("notify prefs: %d %s", r.Code, r.Body)
	}
	if r := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000017", "position": PositionOpen, "note": "happy to help"}); r.Code != http.StatusNoContent {
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
	if r := testkit.Call(t, mux, chair, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000017", "email": activitiesParent, "position": PositionCoChair}); r.Code != http.StatusNoContent {
		t.Fatalf("promote: %d %s", r.Code, r.Body)
	}
	m := rec.Next(t)
	if m.Subject != "You're a co-chair of Clean Up Crew" || !slices.Equal(m.To, []string{activitiesParent}) || !slices.Contains(m.CC, chair) {
		t.Fatalf("co-chair note: %+v", m)
	}
	other := student
	if r := testkit.Call(t, mux, other, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000017", "position": PositionVolunteer}); r.Code != http.StatusNoContent {
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
	if r := testkit.Call(t, mux, chair, "DELETE", "/api/team/volunteer", map[string]any{"id": "act0000000017", "email": other}); r.Code != http.StatusNoContent {
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
	if r := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000017", "email": activitiesKid, "position": PositionOpen, "note": "bring snacks"}); r.Code != http.StatusNoContent {
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
		for _, v := range cache.Model().Activity(id).Volunteers {
			if v.Email == activitiesParent {
				return true
			}
		}
		return false
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000017", "position": PositionVolunteer, "note": "evenings"}); rec.Code != http.StatusNoContent {
		t.Fatalf("sign up: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, "someone.else@heliosschool.org", "POST", "/api/team/volunteer", map[string]any{"id": "act0000000018", "email": activitiesParent, "position": PositionVolunteer, "from": "act0000000017"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger moved someone: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000018", "position": PositionVolunteer, "note": "evenings", "from": "act0000000017"}); rec.Code != http.StatusNoContent {
		t.Fatalf("move: %d %s", rec.Code, rec.Body)
	}
	if on("act0000000017") || !on("act0000000018") {
		t.Fatalf("after the move: on act0000000017 %v, on act0000000018 %v", on("act0000000017"), on("act0000000018"))
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000019", "position": PositionVolunteer, "from": "act0000000017"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("moved from a thing not signed up for: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000016", "position": PositionOpen}); rec.Code != http.StatusBadRequest {
		t.Fatalf("offered to co-chair where none is wanted: %d", rec.Code)
	}
}

func TestMoveNeedsBothEnds(t *testing.T) {
	cache, mux := activitiesServer(t)
	move := func(who, from, to string) int {
		return testkit.Call(t, mux, who, "POST", "/api/team/volunteer", map[string]any{"id": to, "email": marco, "position": PositionVolunteer, "from": from}).Code
	}
	if code := move(chair, "act0000000016", "act0000000003"); code != http.StatusForbidden || cache.Model().Activity("act0000000016").volunteer(marco) == nil {
		t.Fatalf("a co-chair moved someone into a thing they do not run: %d", code)
	}
	if code := move(chair, "act0000000016", "act0000000017"); code != http.StatusNoContent || cache.Model().Activity("act0000000017").volunteer(marco) == nil {
		t.Fatalf("a co-chair could not move someone between two things they run: %d", code)
	}
	if code := move(jordan, "act0000000017", "act0000000003"); code != http.StatusNoContent || cache.Model().Activity("act0000000003").volunteer(marco) == nil {
		t.Fatalf("an admin could not move someone: %d", code)
	}
}

func TestReorderChildren(t *testing.T) {
	cache, mux := activitiesServer(t)
	titles := func() []string {
		out := []string{}
		for _, c := range cache.Model().Activity("act0000000002").Children {
			out = append(out, c.Title)
		}
		return out
	}
	if got := titles(); !slices.Equal(got, []string{"Decor", "Childcare", "Marketing"}) {
		t.Fatalf("row order to start: %v", got)
	}
	body := map[string]any{"parent": "act0000000002", "ids": []string{"act0000000025", "act0000000023", "act0000000024"}}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/order", body); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent reordered: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, chair, "POST", "/api/team/order", body); rec.Code != http.StatusNoContent {
		t.Fatalf("the chair could not reorder: %d %s", rec.Code, rec.Body)
	}
	if got := titles(); !slices.Equal(got, []string{"Marketing", "Decor", "Childcare"}) {
		t.Fatalf("order after: %v", got)
	}
	before := len(activitiesChangeLog(t))
	if rec := testkit.Call(t, mux, chair, "POST", "/api/team/order", map[string]any{"parent": "act0000000002", "ids": []string{"act0000000023", "act0000000025", "act0000000024"}}); rec.Code != http.StatusNoContent {
		t.Fatalf("second reorder: %d %s", rec.Code, rec.Body)
	}
	if got := titles(); !slices.Equal(got, []string{"Decor", "Marketing", "Childcare"}) || len(activitiesChangeLog(t))-before != 1 {
		t.Fatalf("order after one move: %v, %d log rows", got, len(activitiesChangeLog(t))-before)
	}
	if rec := testkit.Call(t, mux, chair, "POST", "/api/team/order", map[string]any{"parent": "act0000000002", "ids": []string{"act0000000025", "act0000000001"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a stranger's id was taken: %d", rec.Code)
	}
}

func TestSuggestApproveRenameDelete(t *testing.T) {
	cache, mux := activitiesServer(t)
	suggestion := map[string]any{"year": "2026 - 2027", "title": "Kite Day", "category": "tcg0000000006", "status": StatusOpen, "description": "Fly kites", "signUp": PositionOpen, "directSignUp": "Yes"}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/activity", suggestion); rec.Code != http.StatusOK {
		t.Fatalf("suggest: %d %s", rec.Code, rec.Body)
	}
	kite := byTitle(cache.Model(), "2026 - 2027", "Kite Day")
	if kite == nil || kite.Status != StatusPending || kite.AddedBy != activitiesParent || len(kite.Volunteers) != 1 || kite.Volunteers[0].Position != PositionOpen {
		t.Fatalf("suggestion landed as %+v", kite)
	}
	edit := map[string]any{"id": kite.ID, "year": "2026 - 2027", "title": "Kite Festival", "category": "tcg0000000002", "status": StatusOpen, "directSignUp": "Yes"}
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/activity", edit); rec.Code != http.StatusForbidden {
		t.Fatalf("a non-chair edited an activity: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/activity", edit); rec.Code != http.StatusOK {
		t.Fatalf("approve and rename: %d %s", rec.Code, rec.Body)
	}
	if byTitle(cache.Model(), "2026 - 2027", "Kite Day") != nil {
		t.Fatal("the old title survived the rename")
	}
	festival := cache.Model().Activity(kite.ID)
	if festival == nil || festival.Status != StatusOpen || len(festival.Volunteers) != 1 {
		t.Fatalf("renamed activity: %+v", festival)
	}
	if rec := testkit.Call(t, mux, jordan, "DELETE", "/api/team/activity", map[string]string{"id": kite.ID}); rec.Code != http.StatusBadRequest {
		t.Fatalf("delete with a volunteer on it: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, jordan, "DELETE", "/api/team/volunteer", map[string]any{"id": kite.ID, "email": activitiesParent}); rec.Code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, jordan, "DELETE", "/api/team/activity", map[string]string{"id": kite.ID}); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Activity(kite.ID) != nil {
		t.Fatal("the activity survived deletion")
	}
}

func TestYearMoveCarriesTheTreeAndDeleteTakesTheLinks(t *testing.T) {
	cache, mux := activitiesServer(t)
	spring := cache.Model().Activity("act0000000002")
	edit := map[string]any{"id": "act0000000002", "year": "2027 - 2028", "title": spring.Title, "category": spring.Category, "status": spring.Status, "directSignUp": "Yes", "prettyId": spring.PrettyID}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/activity", edit); rec.Code != http.StatusOK {
		t.Fatalf("move year: %d %s", rec.Code, rec.Body)
	}
	for _, c := range cache.Model().Activity("act0000000002").Children {
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
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/activity", map[string]any{"year": "2026 - 2027", "title": "Bake Sale", "category": "tcg0000000002", "status": StatusOpen, "directSignUp": "Yes"}); rec.Code != http.StatusOK {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	sale := byTitle(cache.Model(), "2026 - 2027", "Bake Sale")
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/link", map[string]any{"id": sale.ID, "title": "Menu", "url": "https://example.org/menu"}); rec.Code != http.StatusNoContent {
		t.Fatalf("link: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, jordan, "DELETE", "/api/team/activity", map[string]string{"id": sale.ID}); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if cache.Count(linksTab, store.Row{"Event ID": sale.ID}) != 0 {
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
		"id": "act0000000020", "year": "2026 - 2027", "title": "India Booth", "parent": "act0000000001",
		"category": "tcg0000000008", "status": StatusOpen, "coLeaderNeeded": true, "directSignUp": "Yes",
	}
	if rec := testkit.Call(t, mux, chair, "POST", "/api/team/activity", edit); rec.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	booth := cache.Model().Activity("act0000000020")
	if booth == nil || booth.Title != "India Booth" || len(booth.Volunteers) != 1 || len(booth.Links) != 1 || len(booth.Children) != 1 || booth.Children[0].Parent != "act0000000020" {
		t.Fatalf("renamed: %+v", booth)
	}
	if rec := testkit.Call(t, mux, jordan, "DELETE", "/api/team/activity", map[string]string{"id": "act0000000020"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("deleted something with children and volunteers: %d", rec.Code)
	}
	loop := map[string]any{"id": "act0000000001", "year": "2026 - 2027", "title": "International Night", "parent": "act0000000020", "category": "", "status": StatusOpen}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/activity", loop); rec.Code != http.StatusBadRequest {
		t.Fatalf("a parent loop was accepted: %d", rec.Code)
	}
}

func TestCoChairApproves(t *testing.T) {
	cache, mux := activitiesServer(t)
	save := func(who, id, parentID, category, status string) int {
		act := cache.Model().Activity(id)
		return testkit.Call(t, mux, who, "POST", "/api/team/activity", map[string]any{
			"id": id, "year": act.Year, "title": act.Title, "parent": parentID, "category": category, "status": status, "directSignUp": act.DirectSignUpOwn,
		}).Code
	}
	status := func(id string) string { return cache.Model().Activity(id).Status }
	if code := save(chair, "act0000000022", "act0000000001", "tcg0000000008", StatusHidden); code != http.StatusBadRequest {
		t.Fatalf("a co-chair hid a suggestion: %d", code)
	}
	if code := save(chair, "act0000000022", "act0000000001", "tcg0000000008", StatusPending); code != http.StatusOK || status("act0000000022") != StatusPending {
		t.Fatalf("a co-chair's save that leaves it pending: %d, %s", code, status("act0000000022"))
	}
	if code := save(chair, "act0000000022", "act0000000001", "tcg0000000008", StatusOpen); code != http.StatusOK || status("act0000000022") != StatusOpen {
		t.Fatalf("the event's co-chair could not approve a suggestion: %d, %s", code, status("act0000000022"))
	}
	if code := testkit.Call(t, mux, jordan, "POST", "/api/team/volunteer", map[string]any{"id": "act0000000012", "email": chair, "position": PositionCoChair}).Code; code != http.StatusNoContent {
		t.Fatalf("make a co-chair: %d", code)
	}
	if code := save(chair, "act0000000012", "", "tcg0000000001", StatusOpen); code != http.StatusOK || status("act0000000012") != StatusPending {
		t.Fatalf("a co-chair approved their own pending event: %d, %s", code, status("act0000000012"))
	}
}

func TestCoChairMovesOnlyUnderTheirOwn(t *testing.T) {
	cache, mux := activitiesServer(t)
	const india = "deepa.natarajan@heliosschool.org"
	move := func(who, id, parentID string) int {
		act := cache.Model().Activity(id)
		return testkit.Call(t, mux, who, "POST", "/api/team/activity", map[string]any{
			"id": id, "year": act.Year, "title": act.Title, "parent": parentID, "category": "", "status": act.Status, "directSignUp": act.DirectSignUpOwn,
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
	if code := move(chair, "act0000000019", "act0000000002"); code != http.StatusOK || cache.Model().Activity("act0000000019").Parent != "act0000000002" {
		t.Fatalf("a co-chair could not move a booth between their own events: %d", code)
	}
	if code := move(jordan, "act0000000016", "act0000000003"); code != http.StatusOK {
		t.Fatalf("an admin could not move a crew: %d", code)
	}
}

func TestCopyToNextYear(t *testing.T) {
	cache, mux := activitiesServer(t)
	if rec := testkit.Call(t, mux, chair, "POST", "/api/team/copy", map[string]string{"id": "act0000000001"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a co-chair copied: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/copy", map[string]string{"id": "act0000000001"}); rec.Code != http.StatusNoContent {
		t.Fatalf("copy: %d %s", rec.Code, rec.Body)
	}
	next := byTitle(cache.Model(), "2027 - 2028", "International Night")
	if next == nil || next.ID == "act0000000001" || next.Status != StatusOpen || next.Start != "" || len(next.Volunteers) != 0 || len(next.Descendants()) != 6 || byTitle(cache.Model(), "2027 - 2028", "Cybertron") != nil || len(next.Links) != 2 {
		t.Fatalf("copied activity: %+v", next)
	}
	for _, c := range next.Descendants() {
		if p := cache.Model().Activity(c.Parent); p == nil || p.Year != "2027 - 2028" {
			t.Fatalf("copied child %q points at parent %q in the wrong year", c.Title, c.Parent)
		}
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/copy", map[string]string{"id": "act0000000001"}); rec.Code != http.StatusBadRequest {
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
	if got := ShiftActivityYear("2026 - 2027", -1); got != "2025 - 2026" {
		t.Errorf("shift: %s", got)
	}
	if err := CheckActivityYear("2026 - 2028"); err == nil {
		t.Error("a two-year span passed")
	}
}

func TestEventCategories(t *testing.T) {
	cache, mux := activitiesServer(t)
	m := cache.Model()
	night := m.Activity("act0000000001")
	if len(night.Categories) != 2 || night.Categories[1].ID != "tcg0000000008" || night.Categories[1].Adding != AddingYes {
		t.Fatalf("international night's categories: %+v", night.Categories)
	}
	if len(m.Categories) != 6 {
		t.Fatalf("the page should see only the six headings, got %d", len(m.Categories))
	}
	propose := func(who, category string) int {
		return testkit.Call(t, mux, who, "POST", "/api/team/activity", map[string]any{
			"year": "2026 - 2027", "title": "Sweden", "parent": "act0000000001", "category": category, "status": StatusOpen, "directSignUp": "Yes",
		}).Code
	}
	if code := propose(activitiesParent, "tcg0000000008"); code != http.StatusOK {
		t.Fatalf("a booth under an open category was refused: %d", code)
	}
	if sweden := byTitle(cache.Model(), "2026 - 2027", "Sweden"); sweden == nil || sweden.Status != StatusOpen {
		t.Fatalf("a booth added under a Yes category should be open: %+v", sweden)
	}
	if code := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/activity", map[string]any{
		"year": "2026 - 2027", "title": "Loose Booth", "parent": "act0000000001", "category": "", "status": StatusOpen, "directSignUp": "Yes",
	}).Code; code != http.StatusOK {
		t.Fatalf("an uncategorised proposal under an Approval Needed event was refused: %d", code)
	}
	if loose := byTitle(cache.Model(), "2026 - 2027", "Loose Booth"); loose == nil || loose.Status != StatusPending {
		t.Fatalf("an uncategorised proposal should wait for approval: %+v", loose)
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
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/category", own); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent made an event category: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, chair, "POST", "/api/team/category", own); rec.Code != http.StatusNoContent {
		t.Fatalf("the co-chair could not add an event category: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, chair, "POST", "/api/team/category", map[string]any{"title": "New Heading"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a co-chair made a page heading: %d", rec.Code)
	}
	if n := len(cache.Model().Activity("act0000000001").Categories); n != 3 {
		t.Fatalf("event categories after adding: %d", n)
	}
	ids := []string{}
	for _, c := range cache.Model().Activity("act0000000001").Categories {
		ids = append(ids, c.ID)
	}
	ids[0], ids[1] = ids[1], ids[0]
	if rec := testkit.Call(t, mux, chair, "POST", "/api/team/categories/order", map[string]any{"eventId": "act0000000001", "ids": ids}); rec.Code != http.StatusNoContent {
		t.Fatalf("reorder: %d %s", rec.Code, rec.Body)
	}
	after := cache.Model()
	got := []string{}
	for _, c := range after.Activity("act0000000001").Categories {
		got = append(got, c.ID)
	}
	if !slices.Equal(got, ids) || after.Categories[0].ID != "tcg0000000001" || after.Activity("act0000000013").Categories[0].ID != "tcg0000000009" {
		t.Fatalf("reorder: %v, or it leaked out of its scope", got)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/categories/order", map[string]any{"eventId": "act0000000001", "ids": ids[1:]}); rec.Code != http.StatusBadRequest {
		t.Fatalf("an order missing one was taken: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/copy", map[string]string{"id": "act0000000001"}); rec.Code != http.StatusNoContent {
		t.Fatalf("copy: %d %s", rec.Code, rec.Body)
	}
	next := byTitle(cache.Model(), "2027 - 2028", "International Night")
	if next == nil || len(next.Categories) != 3 || next.Categories[0].ID == ids[0] || next.Categories[0].Title != after.Activity("act0000000001").Categories[0].Title {
		t.Fatalf("copied categories: %+v", next.Categories)
	}
	norway := byTitle(cache.Model(), "2027 - 2028", "Norway")
	if norway == nil || cache.Model().Category(norway.Category).EventID != next.ID {
		t.Fatalf("copied child still names the old event's category: %+v", norway)
	}
}

func TestUncategorizedFallback(t *testing.T) {
	cache, mux := activitiesServer(t)
	if len(cache.Model().Categories) != 6 {
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
	if rec := testkit.Call(t, mux, activitiesParent, "POST", "/api/team/activity", add); rec.Code != http.StatusBadRequest {
		t.Fatalf("a proposal without a category went through: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/activity", add); rec.Code != http.StatusOK {
		t.Fatalf("admin add: %d %s", rec.Code, rec.Body)
	}
	loose := byTitle(cache.Model(), "2026 - 2027", "Loose End")
	if loose == nil || loose.Category != UncategorizedID || cache.Count(activitiesTab, store.Row{"Title": "Loose End", "Category": ""}) != 1 {
		t.Fatalf("loose end: %+v", loose)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/category", map[string]any{"id": UncategorizedID, "title": "Misc"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("edited the built-in heading: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, jordan, "DELETE", "/api/team/category", map[string]any{"id": UncategorizedID}); rec.Code != http.StatusBadRequest {
		t.Fatalf("deleted the built-in heading: %d", rec.Code)
	}
}

func TestActivitiesBrokenSheetRefusesToLoad(t *testing.T) {
	t.Chdir("../..")
	broken := t.TempDir()
	if err := os.MkdirAll(filepath.Join(broken, "events"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Categories", "Activities", "Links", "Settings", "Notifications", "Admins", "Redirects", "Aliases", "Change Log"} {
		raw, err := os.ReadFile(filepath.Join("sampledata", "events", name+".csv"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(broken, "events", name+".csv"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(broken, "events", "Volunteers.csv"), []byte("Year,Activity,Role,Email,Position,Note,Added By,Added\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := &data.Dir{Root: broken}
	if _, err := NewActivitiesCache(dir, dir, activitiesBundled, func() []string { return nil }, store.NewQueue()); err == nil || !strings.Contains(err.Error(), `missing column "Event ID"`) {
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
	s, err := cache.Model().saveActivity(activityViewerOf(chair, false), patch(AddingApproval))
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Commit(context.Background(), activityViewerOf(chair, false), s.ops...); err != nil {
		t.Fatal(err)
	}
	if got := cache.Model().Activity("act0000000001").AllowAdding; got != AddingApproval {
		t.Fatalf("the event's chair set allow adding to %q, want %q", got, AddingApproval)
	}
	if _, err := cache.Model().saveActivity(activityViewerOf(activitiesParent, false), patch(AddingYes)); testkit.Status(t, err) != http.StatusForbidden {
		t.Fatalf("a parent changed allow adding: %v", err)
	}
}

func TestCategorySettings(t *testing.T) {
	cache, _ := activitiesServer(t)
	m := cache.Model()
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
	if err := cache.Commit(context.Background(), activityViewerOf(chair, false), op); err != nil {
		t.Fatal(err)
	}
	if c := cache.Model().Category("tcg0000000008"); c.DirectSignUpOwn != "No" || c.DirectSignUp || c.Adding != AddingApproval {
		t.Fatalf("after saving: %+v", c)
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
	if n := len(m.Activity("act0000000001").Volunteers); n != len(cache.Model().Activity("act0000000001").Volunteers) {
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
	if c := cache.Model().Category("tcg0000000001"); !c.ShowOnMain {
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
	edit := map[string]any{"id": "tcg0000000002", "title": "Activities", "allowAdding": AddingNo, "showOnMain": off}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/category", edit); rec.Code != http.StatusNoContent {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Category("tcg0000000002").ShowOnMain || cache.Count(activityCategoriesTab, store.Row{"Category ID": "tcg0000000002", "Show On Main Page": "No"}) != 1 {
		t.Fatalf("the heading was not taken off the page")
	}
	own := map[string]any{"eventId": "act0000000001", "title": "Shifts", "allowAdding": "", "showOnMain": off}
	if rec := testkit.Call(t, mux, chair, "POST", "/api/team/category", own); rec.Code != http.StatusNoContent {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	if cache.Count(activityCategoriesTab, store.Row{"Title": "Shifts", "Show On Main Page": ""}) != 1 {
		t.Fatalf("an event category carried the page flag")
	}
}

func TestPrettyIDs(t *testing.T) {
	cache, mux := activitiesServer(t)
	m := cache.Model()
	if m.ByPretty("International-Night") != m.Activity("act0000000001") || m.ByPretty("nope") != nil {
		t.Fatalf("pretty lookup")
	}
	edit := func(id, pretty string, takeOver bool) *httptest.ResponseRecorder {
		act := cache.Model().Activity(id)
		return testkit.Call(t, mux, jordan, "POST", "/api/team/activity", map[string]any{
			"id": id, "year": act.Year, "title": act.Title, "category": act.Category, "status": act.Status,
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
	last := byTitle(cache.Model(), "2025 - 2026", "International Night")
	rec = edit("act0000000002", "international-night-2025", false)
	if rec.Code != http.StatusConflict || json.Unmarshal(rec.Body.Bytes(), &conflict) != nil || !conflict.Prior || conflict.ID != last.ID || conflict.Renamed != "international-night-2025-2025" {
		t.Fatalf("prior-year clash: %d %s", rec.Code, rec.Body)
	}
	if rec := edit("act0000000002", "international-night-2025", true); rec.Code != http.StatusOK {
		t.Fatalf("take-over: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model()
	if m.Activity("act0000000002").PrettyID != "international-night-2025" || m.Activity(last.ID).PrettyID != "international-night-2025-2025" {
		t.Fatalf("after take-over: %q %q", m.Activity("act0000000002").PrettyID, m.Activity(last.ID).PrettyID)
	}
	if n := cache.Count(activityRedirectsTab, store.Row{"Old": "/v/international-night-2025"}); n != 0 {
		t.Fatalf("the address taken over was redirected to the displaced event: %+v", m.Redirects)
	}
	if m.Resolve("intl-night") != m.Activity("act0000000001") || m.Resolve("/v/intl-night") != m.Activity("act0000000001") || m.Resolve("nope") != nil {
		t.Fatalf("sample redirect")
	}
	if rec := edit("act0000000002", "spring-party", false); rec.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	if rec := edit("act0000000002", "", false); rec.Code != http.StatusOK {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model()
	for _, old := range []string{"/v/spring-celebration", "/v/international-night-2025", "/v/spring-party", "/activities/act0000000002"} {
		if m.Resolve(old) != m.Activity("act0000000002") {
			t.Fatalf("%s did not reach the event: %+v", old, m.Redirects)
		}
	}
	if n := cache.Count(activityRedirectsTab, store.Row{"Type": RedirectActivity, "Old": "/v/spring-party", "New": "/activities/act0000000002"}); n != 1 {
		t.Fatalf("a removed address should redirect to the row: %+v", m.Redirects)
	}
	if err := cache.Commit(context.Background(), activityViewerOf(jordan, true), store.Update(activitiesTab, store.Row{"Event ID": "act0000000002"}, store.Row{"Pretty ID": "spring-fling"})); err != nil {
		t.Fatal(err)
	}
	if n := cache.Count(activityRedirectsTab, store.Row{"Type": RedirectActivity, "Old": "/activities/act0000000002", "New": "/v/spring-fling"}); n != 1 {
		t.Fatalf("a write outside the save handler moved the event without a redirect: %+v", cache.Model().Redirects)
	}
	if rec := edit("act0000000002", "international-night-2025", false); rec.Code != http.StatusOK {
		t.Fatalf("restore: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model()
	norway, india := byTitle(m, "2026 - 2027", "Norway"), byTitle(m, "2026 - 2027", "India")
	if m.PathOf(norway) != "/v/international-night/"+norway.ID {
		t.Fatalf("child path %q", m.PathOf(norway))
	}
	child := func(node *Activity, pretty string) *httptest.ResponseRecorder {
		return testkit.Call(t, mux, jordan, "POST", "/api/team/activity", map[string]any{
			"id": node.ID, "year": node.Year, "title": node.Title, "parent": node.Parent, "category": node.Category, "status": node.Status,
			"directSignUp": "Yes", "prettyId": pretty,
		})
	}
	if rec := child(norway, "norway"); rec.Code != http.StatusOK {
		t.Fatalf("child pretty: %d %s", rec.Code, rec.Body)
	}
	if rec := child(india, "norway"); rec.Code != http.StatusConflict {
		t.Fatalf("two siblings took one address: %d", rec.Code)
	}
	m = cache.Model()
	if m.Resolve("/v/international-night/norway") != m.Activity(norway.ID) || m.Resolve("/v/international-night/"+norway.ID) != m.Activity(norway.ID) || m.Resolve("/v/intl-night/norway") != m.Activity(norway.ID) {
		t.Fatalf("child by path: %q", m.PathOf(m.Activity(norway.ID)))
	}
	if rec := edit("act0000000001", "inight", false); rec.Code != http.StatusOK {
		t.Fatalf("rename event: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model()
	if m.PathOf(m.Activity(norway.ID)) != "/v/inight/norway" || m.Resolve("/v/international-night/norway") != m.Activity(norway.ID) || m.Resolve("/v/intl-night/norway") != m.Activity(norway.ID) {
		t.Fatalf("event rename did not carry the booth: %q", m.PathOf(m.Activity(norway.ID)))
	}
	if rec := edit("act0000000002", "International-Night-2025", false); rec.Code != http.StatusOK {
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
	list := needs(cache.Model(), now())
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
	if rec := testkit.Call(t, mux, chair, "POST", "/api/team/redirect", map[string]string{"old": oldLink, "new": "/v/international-night"}); rec.Code != http.StatusForbidden {
		t.Fatalf("chair: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/redirect", map[string]string{"old": oldLink, "new": "/v/international-night"}); rec.Code != http.StatusNoContent {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	m := cache.Model()
	oldPath := "/dl/signup/s/768d91/r/nsSPomxFcPrSfoRCAgzI"
	if n := cache.Count(activityRedirectsTab, store.Row{"Type": RedirectAdmin, "Old": oldPath, "New": "/v/international-night"}); n != 1 {
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
			if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/redirect", map[string]string{"old": "/b", "new": "/a"}); rec.Code != http.StatusNoContent {
				t.Fatalf("b: %d %s", rec.Code, rec.Body)
			}
		}
		if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/redirect", bad); rec.Code < 400 {
			t.Errorf("%v was accepted", bad)
		}
	}
	elsewhere := "https://celebrate.heliosian.com/parties/abc"
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/redirect", map[string]string{"original": oldLink, "old": "/dl/signup/s/768d91", "new": elsewhere}); rec.Code != http.StatusNoContent {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model()
	if n := cache.Count(activityRedirectsTab, store.Row{"Old": oldPath}); n != 0 {
		t.Fatalf("old row still there")
	}
	if got := m.Destination(oldPath); got != elsewhere+"/r/nsSPomxFcPrSfoRCAgzI" {
		t.Fatalf("under the edited prefix: %q", got)
	}
	if m.Resolve(oldPath) != nil {
		t.Fatalf("a chain that leaves the site names no activity")
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/redirect", map[string]string{"original": "intl-night", "old": "/v/intl-nite", "new": "/v/international-night"}); rec.Code != http.StatusNoContent {
		t.Fatalf("edit the sample row: %d %s", rec.Code, rec.Body)
	}
	if n := cache.Count(activityRedirectsTab, store.Row{"Type": "pretty", "Old": "/v/intl-nite"}); n != 1 {
		t.Fatalf("the sample row's kind was not kept: %+v", cache.Model().Redirects)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/team/redirect", map[string]string{"old": "/v/fair", "new": "/"}); rec.Code != http.StatusNoContent {
		t.Fatalf("to the front page: %d %s", rec.Code, rec.Body)
	}
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
	if rec := testkit.Call(t, mux, jordan, "DELETE", "/api/team/redirect", map[string]string{"old": "/dl/signup/s/768d91"}); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Destination(oldPath); got != "" {
		t.Fatalf("deleted redirect still sends to %q", got)
	}
	if rec := testkit.Call(t, mux, jordan, "DELETE", "/api/team/redirect", map[string]string{"old": "/dl/signup/s/768d91"}); rec.Code != http.StatusNotFound {
		t.Fatalf("delete again: %d %s", rec.Code, rec.Body)
	}
}
