package model

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/artifacts"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/id"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
)

func TestSentTo(t *testing.T) {
	for _, c := range []struct {
		email SchoolEmail
		want  string
	}{
		{SchoolEmail{Kind: DocumentKindNewsletter}, "All families"},
		{SchoolEmail{Kind: DocumentKindNewsletter, Audience: "Everyone"}, "All families"},
		{SchoolEmail{Kind: DocumentKindAnnouncement, Audience: "Jays"}, "Jays"},
		{SchoolEmail{Kind: DocumentKindNewsletter, Audience: "Jays, Ravens"}, "Jays & Ravens"},
		{SchoolEmail{Kind: DocumentKindNewsletter, Audience: "Hawks, Falcons, Jays"}, "Hawks, Falcons & Jays"},
		{SchoolEmail{Kind: DocumentKindNewsletter, Audience: "Grade 2, Grade 4, Grade 6, Grade 8"}, "Grades 2, 4, 6 & 8"},
		{SchoolEmail{Kind: DocumentKindNewsletter, Audience: "Grade 5"}, "Grade 5"},
		{SchoolEmail{Kind: DocumentKindNewsletter, Audience: "Kindergarten"}, "Kindergarten"},
		{SchoolEmail{Kind: DocumentKindNewsletter, Audience: "Kindergarten, Grade 1"}, "Grades K & 1"},
		{SchoolEmail{Kind: DocumentKindNewsletter, Audience: "Jays, Grade 3"}, "Jays · Grade 3"},
		{SchoolEmail{Kind: DocumentKindNewsletter, Audience: "Hummingbirds, Hawks, Kindergarten, Grade 1"}, "Hummingbirds & Hawks · Grades K & 1"},
		{SchoolEmail{Kind: DocumentKindNewsletter, Audience: " Jays , , Ravens "}, "Jays & Ravens"},
		{SchoolEmail{Kind: DocumentKindList, Channel: "newsletter"}, "Newsletter"},
		{SchoolEmail{Kind: DocumentKindList, Channel: "parentsandstaff"}, "Parents & staff"},
		{SchoolEmail{Kind: DocumentKindList, Channel: "parentsonly"}, "Parents"},
		{SchoolEmail{Kind: DocumentKindList, Channel: "parentsandstudents"}, "Parents & students"},
		{SchoolEmail{Kind: DocumentKindList, Channel: "community"}, "Community"},
		{SchoolEmail{Kind: DocumentKindList, Channel: "parents"}, "All parents"},
		{SchoolEmail{Kind: DocumentKindList, Channel: "newstudentfamilies"}, "New families"},
		{SchoolEmail{Kind: DocumentKindList, Channel: "new.parents"}, "New families"},
		{SchoolEmail{Kind: DocumentKindList, Channel: "jays.parents"}, "Jays parents"},
		{SchoolEmail{Kind: DocumentKindList, Channel: "hummingbirds.staff"}, "Hummingbirds staff"},
		{SchoolEmail{Kind: DocumentKindList, Channel: "jaysandravens"}, "Jays & Ravens"},
		{SchoolEmail{Kind: DocumentKindList, Channel: "hawksandfalcons"}, "Hawks & Falcons"},
		{SchoolEmail{Kind: DocumentKindList, Channel: "herons"}, "Herons"},
	} {
		if got := sentTo(c.email); got != c.want {
			t.Errorf("%+v is sent to %q, want %q", c.email, got, c.want)
		}
	}
}

func schoolServer(t *testing.T) (*Store, homeSheet, *http.ServeMux) {
	t.Helper()
	objects := blob.NewMemoryBucket()
	dir := &data.Dir{Root: "../../sampledata"}
	queue := store.NewQueue()
	outsideSuperAdmin(t, dir)
	embedder := vertex(t)
	c := sampleStore(t, dir, queue, inboxDeps(objects, embedder))
	in := &DocumentFiler{Inbox: artifacts.Inbox{SigningKey: "key", Bucket: objects}, store: c, embedder: embedder, holder: queue}
	files, err := filepath.Glob(filepath.Join(documentSamples, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if err := in.FileSaved(context.Background(), access.System("test"), file); err != nil {
			t.Fatal(err)
		}
	}
	s := homeSheet{dir: dir, queue: queue}
	return c, s, homeOver(t, c, s)
}

func TestSchoolEmails(t *testing.T) {
	c, _, mux := schoolServer(t)
	audience := map[string]string{"2026-09-11": "Everyone", "2026-09-25": "Jays, Grade 3", "2026-09-04": "Everyone"}
	ops := []store.Op{}
	for _, d := range c.Model().Documents.Documents {
		if a, ok := audience[d.Date]; ok && d.Kind == DocumentKindNewsletter {
			ops = append(ops, store.Update(documentsTab, store.Row{"Key": d.Key}, store.Row{DocumentAudienceColumn: a}))
		}
	}
	if len(ops) != 3 {
		t.Fatalf("the sample has %d newsletters to address", len(ops))
	}
	if err := c.Commit(context.Background(), access.System("test"), DocumentsApp, ops...); err != nil {
		t.Fatal(err)
	}
	inbox := func(as string) map[string]string {
		r := read(t, mux, as, "/api/school-emails")
		ids := []string{}
		json.Unmarshal(r.Result, &ids)
		out := map[string]string{}
		for _, key := range ids {
			var e schoolEmailResource
			raw, _ := json.Marshal(r.one(t, "school-emails", key))
			if err := json.Unmarshal(raw, &e); err != nil {
				t.Fatal(err)
			}
			if key != id.Of(sampleKey, kindSchoolEmail, e.Key) {
				t.Errorf("%s is not the ID derived from its key %s", key, e.Key)
			}
			if e.To != sentTo(e.SchoolEmail) {
				t.Errorf("%s is sent to %q, want %q", e.Title, e.To, sentTo(e.SchoolEmail))
			}
			if e.Date < "2026-09-03" {
				t.Errorf("%s from %s is older than two weeks", e.Title, e.Date)
			}
			out[e.Date+" "+e.Channel] = e.To
		}
		return out
	}
	parent := inbox("jordan.whitfield@heliosschool.org")
	for key, want := range map[string]string{
		"2026-09-11 newsletter":      "All families",
		"2026-09-25 newsletter":      "Jays · Grade 3",
		"2026-09-21 jays.parents":    "Jays parents",
		"2026-09-21 jaysandravens":   "Jays & Ravens",
		"2026-09-22 parentsonly":     "Parents",
		"2026-09-23 parentsandstaff": "Parents & staff",
	} {
		if parent[key] != want {
			t.Errorf("the parent's %s reads %q, want %q (%v)", key, parent[key], want, parent)
		}
	}
	if parent["2026-09-04 newsletter"] != "All families" {
		t.Errorf("a newsletter from within the two weeks is not in the inbox: %v", parent)
	}
	if _, chat := parent["2026-08-30 chat"]; chat {
		t.Errorf("a chat is in the inbox")
	}
	staff := inbox("ruth.amari@heliosschool.org")
	if _, jays := staff["2026-09-21 jays.parents"]; jays {
		t.Errorf("a staff member without a Jays seat reads the Jays parents' mail: %v", staff)
	}
	if _, jays := staff["2026-09-25 newsletter"]; jays {
		t.Errorf("a staff member without a Jays seat reads the Jays newsletter: %v", staff)
	}
	if staff["2026-09-11 newsletter"] != "All families" {
		t.Errorf("the staff member's inbox lacks the newsletter to everyone: %v", staff)
	}
	clockAt(t, testNow.AddDate(0, 0, 3))
	later := inbox("jordan.whitfield@heliosschool.org")
	if _, old := later["2026-09-04 newsletter"]; old || later["2026-09-11 newsletter"] != "All families" {
		t.Errorf("three days on, the inbox is %v, want the Sep 4 newsletter gone and Sep 11's kept", later)
	}
}

func TestAlertsAsAResource(t *testing.T) {
	c, _, mux := homeServer(t)
	for _, email := range []string{"jordan.whitfield@heliosschool.org", "robin.whitfield@heliosschool.org", "ruth.amari@heliosschool.org", "sam.whitfield@heliosschool.org", outsider} {
		r := read(t, mux, email, "/api/alerts")
		ids := []string{}
		json.Unmarshal(r.Result, &ids)
		if len(ids) != 1 || ids[0] != id.Of(sampleKey, kindAlerts, "") {
			t.Fatalf("%s's alerts: %v", email, ids)
		}
		var got Alerts
		raw, _ := json.Marshal(r.one(t, "alerts", ids[0]))
		json.Unmarshal(raw, &got)
		m := c.Model()
		want := m.Directory.Alerts(m.Directory.Resolve(email), m.Config.StaleYears, now())
		if want.Stale == nil {
			want.Stale = []string{}
		}
		if want.Privacy == nil {
			want.Privacy = []string{}
		}
		if got.Stale == nil || got.Privacy == nil || !slices.Equal(got.Stale, want.Stale) || !slices.Equal(got.Privacy, want.Privacy) {
			t.Errorf("%s's alerts = %+v, want %+v", email, got, want)
		}
	}
	if got := c.Model().Directory.Alerts("jordan.whitfield@heliosschool.org", c.Model().Config.StaleYears, now()); len(got.Stale)+len(got.Privacy) == 0 {
		t.Errorf("the sample parent has nothing to be told, so the test proves little")
	}
}

func TestHomeSettings(t *testing.T) {
	c, _, mux := homeServer(t)
	key := id.Of(sampleKey, kindHomeSettings, "")
	r := read(t, mux, "robin.whitfield@heliosschool.org", "/api/home-settings?include=viewer")
	got := r.one(t, "home-settings", key)
	robin := c.Model().Directory.Person("robin.whitfield@heliosschool.org")
	if got["viewer"] != robin.ID || r.one(t, "people", robin.ID) == nil || got["imageSearch"] != false {
		t.Errorf("home settings = %v", got)
	}
	if roles, _ := got["roles"].([]any); len(roles) != len(AudienceRoles) {
		t.Errorf("roles = %v", got["roles"])
	}
}

func TestAdminListsAsResources(t *testing.T) {
	c, dir, mux := homeServer(t)
	lists := func(as string) []string {
		out := []string{}
		for _, l := range listOf(t, mux, as, "/api/admin-lists") {
			out = append(out, l["app"].(string))
		}
		return out
	}
	mine := []string{}
	for _, a := range adminApps {
		if c.Model().AdminList(a.key).IsAdmin(homeAdmin) {
			mine = append(mine, a.key)
		}
	}
	if got := lists(homeAdmin); !slices.Equal(got, mine) || !slices.Contains(got, "home") {
		t.Errorf("the home admin's lists = %v, want %v", got, mine)
	}
	if got := lists(outsider); len(got) != len(adminApps) {
		t.Errorf("a super admin's lists = %v", got)
	}
	if got := lists("robin.whitfield@heliosschool.org"); len(got) != 0 {
		t.Errorf("a member's lists = %v", got)
	}
	home := read(t, mux, homeAdmin, "/api/admin-lists/home").one(t, "admin-lists", id.Of(sampleKey, kindAdminList, "home"))
	if admins, _ := home["admins"].([]any); !slices.Contains(admins, any(homeAdmin)) || !slices.Contains(admins, any(outsider)) || !can(home, "edit") {
		t.Errorf("the home list = %v", home)
	}
	if rec := testkit.Call(t, mux, "robin.whitfield@heliosschool.org", "GET", "/api/admin-lists/home", nil); rec.Code != http.StatusNotFound {
		t.Errorf("a member reading the home list: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, "robin.whitfield@heliosschool.org", "POST", "/api/admin-lists/home/edit", map[string]any{"admins": []string{"robin.whitfield@heliosschool.org"}}); rec.Code != http.StatusNotFound {
		t.Errorf("a member writing the home list: %d", rec.Code)
	}
	if slices.Contains(mine, "celebrate") {
		t.Fatalf("the home admin runs celebrate in the sample, so a list they cannot see needs another app")
	}
	if rec := testkit.Call(t, mux, homeAdmin, "POST", "/api/admin-lists/celebrate/edit", map[string]any{"admins": []string{homeAdmin}}); rec.Code != http.StatusNotFound {
		t.Errorf("the home admin writing celebrate's list: %d", rec.Code)
	}
	before := len(homeChangeLog(t, dir))
	write(t, mux, homeAdmin, "POST", "/api/admin-lists/home/edit", map[string]any{"admins": []string{homeAdmin, " Abena.Osei@heliosschool.org "}})
	if !c.Model().AdminList("home").IsAdmin(abena) {
		t.Errorf("the new admin did not take")
	}
	if log := homeLog(t, dir, before); !slices.Equal(log, []string{homeAdmin + "|insert|" + AdminsTab.Name + "|Email=" + abena + "|"}) {
		t.Errorf("the edit wrote %v", log)
	}
	if !slices.Contains(lists(abena), "home") {
		t.Errorf("the new admin does not see the list they are on: %v", lists(abena))
	}
}
