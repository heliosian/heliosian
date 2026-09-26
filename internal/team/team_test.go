package team

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/data"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

const (
	parent = "robin.whitfield@heliosschool.org"
	admin  = "jordan.whitfield@heliosschool.org"
	chair  = "mina.park@heliosschool.org"

	testFrom = "HCA-Team <hca@example.org>"
	spouse   = "sam.whitfield@heliosschool.org"
	kid      = "kit.whitfield@heliosschool.org"
)

var (
	sheet *data.Dir
	queue *store.Queue
)

type fakeDirectory struct{}

// parentAlias is another address of the parent's, as Email Aliases lists it.
const parentAlias = "robin@heliosschool.org"

func (fakeDirectory) Resolve(email string) string {
	if email == parentAlias {
		return parent
	}
	return email
}

func (fakeDirectory) People() []DirectoryPerson { return nil }

func (fakeDirectory) Household(email string) (adults, kids []Child) {
	if email == parent {
		return []Child{{Email: spouse, Name: "Sam Whitfield"}}, []Child{{Email: kid, Name: "Kit Whitfield", Grade: "3"}}
	}
	return nil, nil
}

func (fakeDirectory) Grade(string) string { return "" }

func (fakeDirectory) Alerts(string) ([]string, []string) { return nil, nil }

func (fakeDirectory) GradeColors() map[string]string { return nil }

func (fakeDirectory) Parents(string) []string { return nil }

func (fakeDirectory) Person(email string) (string, string, bool) {
	if email == parent {
		return "Robin Whitfield", "/photos/robin.jpg", true
	}
	if strings.HasSuffix(email, "@heliosschool.org") {
		return displayName(email), "", true
	}
	return "", "", false
}

type bundled struct{}

func (bundled) Has(key string) (bool, error) { return strings.HasPrefix(key, "brand/"), nil }

func (bundled) Prefetch(context.Context, []string) error { return nil }

func serveWith(t *testing.T, mailer mail.Sender) (*Cache, *http.ServeMux) {
	t.Helper()
	t.Chdir("../..")
	sheet = &data.Dir{Root: "sampledata"}
	queue = store.NewQueue()
	cache, err := NewCache(sheet, sheet, bundled{}, func(e string) bool { return e == admin }, queue)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, cache, nil, fakeDirectory{}, func() []string { return []string{admin} }, ImageSearch{}, mailer, testFrom, nil, nil)
	return cache, mux
}

func newServer(t *testing.T) (*Cache, *http.ServeMux) {
	t.Helper()
	return serveWith(t, nil)
}

func tables(t *testing.T) store.Tables {
	t.Helper()
	names := []string{categoriesTab, activitiesTab, volunteersTab, linksTab, settingsTab, adminsTab, redirectsTab}
	queue.Flush()
	tabs, err := sheet.Tabs(context.Background(), appName, names, nil)
	if err != nil {
		t.Fatal(err)
	}
	out := store.Tables{}
	for _, name := range names {
		out[name] = tabs[name].Rows
	}
	return out
}

func changeLog(t *testing.T) []store.Row {
	t.Helper()
	queue.Flush()
	_, rows, err := sheet.Table(appName, store.ChangeLogTab)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func call(t *testing.T, mux *http.ServeMux, as, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	auth.Fixed(as, mux).ServeHTTP(rec, req)
	return rec
}

func byTitle(m *Model, year, title string) *Activity {
	for _, root := range m.Activities {
		for _, node := range append([]*Activity{root}, root.Descendants()...) {
			if node.Year == year && node.Title == title {
				return node
			}
		}
	}
	return nil
}

func TestSampleLoads(t *testing.T) {
	cache, _ := newServer(t)
	m := cache.Model()
	if len(m.Categories) != 6 || len(m.Activities) != 15 {
		t.Fatalf("got %d categories, %d root activities", len(m.Categories), len(m.Activities))
	}
	night := m.Activity("E001")
	perf := byTitle(m, "2026 - 2027", "India Performance")
	if night == nil || len(night.Children) != 6 || perf == nil || perf.Parent != "E020" {
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
	cache, _ := newServer(t)
	m := cache.Model()
	all := []*Activity{}
	for _, root := range m.Activities {
		all = append(append(all, root), root.Descendants()...)
	}
	for _, c := range []struct {
		email string
		admin bool
	}{{parent, false}, {"elena.torres@heliosschool.org", false}, {m.Activity("E001").CoChairs()[0], false}, {parent, true}} {
		shown := map[string]bool{}
		var walk func([]*ActivityView)
		walk = func(list []*ActivityView) {
			for _, a := range list {
				shown[a.ID] = true
				walk(a.Children)
			}
		}
		for _, a := range Render(m, fakeDirectory{}, c.email, c.admin, now()).Activities {
			shown[a.ID] = true
			walk(a.Children)
		}
		hidden := 0
		for _, a := range all {
			if m.VisibleTo(a, c.email, c.admin) != shown[a.ID] {
				t.Errorf("%s (admin %v): %q (%s) VisibleTo %v, rendered %v", c.email, c.admin, a.Title, a.Status, m.VisibleTo(a, c.email, c.admin), shown[a.ID])
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

func TestRenderHidesWhatItShould(t *testing.T) {
	cache, _ := newServer(t)
	view := Render(cache.Model(), fakeDirectory{}, parent, false, now())
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
	if view.User.Name != "Robin Whitfield" || view.User.PhotoURL != "/photos/robin.jpg" || view.People != nil {
		t.Errorf("user %+v, people %v", view.User, view.People)
	}
	suggester := Render(cache.Model(), fakeDirectory{}, "elena.torres@heliosschool.org", false, now())
	found := false
	for _, a := range suggester.Activities {
		if a.Title == "Family Escape Room Night" {
			found = true
		}
	}
	if !found {
		t.Error("a suggester cannot see their own pending suggestion")
	}
	if got := Render(cache.Model(), fakeDirectory{}, "someone.new@heliosschool.org", false, now()).User.Name; got != "Someone New" {
		t.Errorf("display name %q", got)
	}
}

func TestSignUpAndRemove(t *testing.T) {
	cache, mux := newServer(t)
	body := map[string]any{"id": "E017", "position": PositionOpen, "note": "happy to help"}
	if rec := call(t, mux, parent, "POST", "/api/team/volunteer", body); rec.Code != http.StatusNoContent {
		t.Fatalf("sign up: %d %s", rec.Code, rec.Body)
	}
	role := cache.Model().Activity("E017")
	if len(role.Volunteers) != 1 || role.Volunteers[0].Email != parent || role.Volunteers[0].AddedBy != parent {
		t.Fatalf("volunteers after sign up: %+v", role.Volunteers)
	}
	body["position"] = PositionCoChair
	if rec := call(t, mux, parent, "POST", "/api/team/volunteer", body); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent named themselves co-chair: %d", rec.Code)
	}
	if rec := call(t, mux, chair, "POST", "/api/team/volunteer", map[string]any{"id": "E017", "email": parent, "position": PositionCoChair}); rec.Code != http.StatusNoContent {
		t.Fatalf("a co-chair could not promote: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, parent, "POST", "/api/team/volunteer", map[string]any{"id": "E017", "position": PositionCoChair, "note": "here to lead"}); rec.Code != http.StatusNoContent {
		t.Fatalf("a co-chair could not edit their own note: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, parent, "POST", "/api/team/volunteer", map[string]any{"id": "E017", "position": PositionVolunteer}); rec.Code != http.StatusNoContent {
		t.Fatalf("a co-chair could not step down: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, parent, "POST", "/api/team/volunteer", map[string]any{"id": "E017", "position": PositionCoChair}); rec.Code != http.StatusForbidden {
		t.Fatalf("a volunteer named themselves co-chair: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/team/volunteer", map[string]any{"id": "E017", "email": parent, "position": PositionCoChair}); rec.Code != http.StatusNoContent {
		t.Fatalf("an admin could not promote: %d %s", rec.Code, rec.Body)
	}
	full := map[string]any{"id": "E026", "position": PositionVolunteer}
	if rec := call(t, mux, parent, "POST", "/api/team/volunteer", full); rec.Code != http.StatusBadRequest {
		t.Fatalf("a full role took a sign-up: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/team/volunteer", map[string]any{"id": "E001", "email": "Robin@heliosschool.org", "position": PositionVolunteer}); rec.Code != http.StatusNoContent {
		t.Fatalf("an admin could not sign up an alias: %d %s", rec.Code, rec.Body)
	}
	if v := cache.Model().Activity("E001").volunteer(parent); v == nil {
		t.Fatalf("a sign-up by alias was not stored as the directory's address: %+v", cache.Model().Activity("E001").Volunteers)
	}
	for _, as := range []string{parent, admin} {
		if rec := call(t, mux, as, "POST", "/api/team/volunteer", map[string]any{"id": "E017", "email": "x@elsewhere.example", "position": PositionVolunteer}); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s signed up an address outside the directory: %d", as, rec.Code)
		}
	}
	direct := map[string]any{"id": "E001", "position": PositionVolunteer}
	if rec := call(t, mux, parent, "POST", "/api/team/volunteer", direct); rec.Code != http.StatusBadRequest {
		t.Fatalf("an activity without direct sign-up took one: %d", rec.Code)
	}
	if rec := call(t, mux, "someone.else@heliosschool.org", "DELETE", "/api/team/volunteer", map[string]any{"id": "E017", "email": parent}); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger removed someone: %d", rec.Code)
	}
	if rec := call(t, mux, parent, "POST", "/api/team/volunteer", map[string]any{"id": "E017", "email": kid, "position": PositionVolunteer}); rec.Code != http.StatusNoContent {
		t.Fatalf("a parent could not sign their child up: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, parent, "POST", "/api/team/volunteer", map[string]any{"id": "E017", "email": kid, "position": PositionVolunteer, "note": "after school only"}); rec.Code != http.StatusNoContent {
		t.Fatalf("a parent could not edit their child's sign-up: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, kid, "DELETE", "/api/team/volunteer", map[string]any{"id": "E017", "email": parent}); rec.Code != http.StatusForbidden {
		t.Fatalf("a child removed their parent: %d", rec.Code)
	}
	if rec := call(t, mux, parent, "DELETE", "/api/team/volunteer", map[string]any{"id": "E017", "email": kid}); rec.Code != http.StatusNoContent {
		t.Fatalf("a parent could not remove their child: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, parent, "DELETE", "/api/team/volunteer", map[string]any{"id": "E017", "email": parent}); rec.Code != http.StatusNoContent {
		t.Fatalf("remove self: %d %s", rec.Code, rec.Body)
	}
	if n := len(cache.Model().Activity("E017").Volunteers); n != 0 {
		t.Fatalf("%d volunteers left after removal", n)
	}
}

type recorder struct{ got chan mail.Message }

func (r recorder) Send(_ context.Context, m mail.Message) error {
	r.got <- m
	return nil
}

func (r recorder) next(t *testing.T) mail.Message {
	t.Helper()
	select {
	case m := <-r.got:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("no mail arrived")
		return mail.Message{}
	}
}

func TestMail(t *testing.T) {
	rec := recorder{got: make(chan mail.Message, 8)}
	_, mux := serveWith(t, rec)
	if r := call(t, mux, admin, "POST", "/api/team/notify", map[string]any{"kinds": []string{"signups", "offers"}}); r.Code != http.StatusNoContent {
		t.Fatalf("notify prefs: %d %s", r.Code, r.Body)
	}
	if r := call(t, mux, parent, "POST", "/api/team/volunteer", map[string]any{"id": "E017", "position": PositionOpen, "note": "happy to help"}); r.Code != http.StatusNoContent {
		t.Fatalf("sign up: %d %s", r.Code, r.Body)
	}
	bySubject := map[string]mail.Message{}
	for range 4 {
		m := rec.next(t)
		bySubject[m.Subject] = m
	}
	thanks := bySubject["Thanks for volunteering for Clean Up Crew"]
	if !slices.Equal(thanks.To, []string{parent}) || !slices.Equal(thanks.CC, []string{admin, chair}) || len(thanks.Attachments) != 0 || !strings.Contains(thanks.HTML, "Hi Robin") || !strings.Contains(thanks.HTML, "/open/share/E017.png") || !strings.Contains(thanks.HTML, "Add to Calendar") {
		t.Fatalf("thank-you: %+v (subjects %v)", thanks, keys(bySubject))
	}
	if !slices.Equal(thanks.ReplyTo, []string{admin, chair}) {
		t.Errorf("thank-you reply-to: %v", thanks.ReplyTo)
	}
	invite := bySubject["Calendar invite: Clean Up Crew"]
	if !slices.Equal(invite.To, []string{parent}) || len(invite.CC) != 0 || !slices.Equal(invite.ReplyTo, []string{admin, chair}) || len(invite.Attachments) != 1 || !strings.Contains(invite.HTML, "Add it to your calendar") {
		t.Fatalf("invite note: %+v (subjects %v)", invite, keys(bySubject))
	}
	ics := strings.ReplaceAll(string(invite.Attachments[0].Content), "\r\n ", "")
	for _, want := range []string{"METHOD:REQUEST", "UID:team-E017-" + parent + "@heliosian.com", "SUMMARY:Clean Up Crew (International Night)", "DTSTART:20260924T230000Z", "DTEND:20260925T010000Z", "ORGANIZER;CN=HCA-Team:mailto:hca@example.org", "ATTENDEE;CN=Robin Whitfield;ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED;RSVP=FALSE:mailto:" + parent, "STATUS:CONFIRMED"} {
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
	if n, ok := bySubject["New sign-up: Robin Whitfield for Clean Up Crew"]; !ok || !slices.Equal(n.To, []string{admin}) {
		t.Fatalf("sign-up notice: %+v", n)
	}
	if n, ok := bySubject["Co-chair offer: Robin Whitfield for Clean Up Crew"]; !ok || !slices.Equal(n.To, []string{admin}) {
		t.Fatalf("offer notice: %+v", n)
	}
	if r := call(t, mux, chair, "POST", "/api/team/volunteer", map[string]any{"id": "E017", "email": parent, "position": PositionCoChair}); r.Code != http.StatusNoContent {
		t.Fatalf("promote: %d %s", r.Code, r.Body)
	}
	m := rec.next(t)
	if m.Subject != "You're a co-chair of Clean Up Crew" || !slices.Equal(m.To, []string{parent}) || !slices.Contains(m.CC, chair) {
		t.Fatalf("co-chair note: %+v", m)
	}
	other := "sam.whitfield@heliosschool.org"
	if r := call(t, mux, other, "POST", "/api/team/volunteer", map[string]any{"id": "E017", "position": PositionVolunteer}); r.Code != http.StatusNoContent {
		t.Fatalf("second sign up: %d %s", r.Code, r.Body)
	}
	for range 3 {
		m := rec.next(t)
		switch {
		case strings.HasPrefix(m.Subject, "Thanks for volunteering"):
			if !strings.Contains(m.Text, "Clean Up Crew Leads: Robin Whitfield\n") || !strings.Contains(m.Text, "Event Chairs: ") || !slices.Equal(m.CC, []string{parent, admin, chair}) {
				t.Fatalf("thank-you under a lead: %q cc %v", m.Text, m.CC)
			}
		case strings.HasPrefix(m.Subject, "Calendar invite"):
			if !slices.Equal(m.To, []string{other}) || len(m.CC) != 0 || !strings.Contains(string(m.Attachments[0].Content), "mailto:"+other) {
				t.Fatalf("a student's invite: to %v cc %v", m.To, m.CC)
			}
		}
	}
	if r := call(t, mux, chair, "DELETE", "/api/team/volunteer", map[string]any{"id": "E017", "email": other}); r.Code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", r.Code, r.Body)
	}
	m = rec.next(t)
	cancel := strings.ReplaceAll(string(m.Attachments[0].Content), "\r\n ", "")
	if m.Subject != "Removed: Clean Up Crew" || !slices.Equal(m.To, []string{other}) || !strings.HasPrefix(m.Attachments[0].ContentType, "text/calendar; method=CANCEL") || !strings.Contains(cancel, "METHOD:CANCEL") || !strings.Contains(cancel, "STATUS:CANCELLED") || !strings.Contains(cancel, "UID:team-E017-"+other+"@heliosian.com") {
		t.Fatalf("cancellation: %+v\n%s", m, cancel)
	}
	if !strings.Contains(m.Text, "Mina Park removed your sign-up for Clean Up Crew") {
		t.Errorf("cancellation does not name who removed it: %q", m.Text)
	}
	if r := call(t, mux, parent, "POST", "/api/team/volunteer", map[string]any{"id": "E017", "email": kid, "position": PositionOpen, "note": "bring snacks"}); r.Code != http.StatusNoContent {
		t.Fatalf("sign up a child: %d %s", r.Code, r.Body)
	}
	bySubject = map[string]mail.Message{}
	for range 4 {
		m := rec.next(t)
		bySubject[m.Subject] = m
	}
	if thanks := bySubject["Thanks for volunteering for Clean Up Crew"]; !strings.Contains(thanks.Text, "Note from Robin Whitfield: bring snacks") {
		t.Errorf("a note someone else wrote is not theirs: %q", thanks.Text)
	}
	if offer := bySubject["Co-chair offer: Kit Whitfield for Clean Up Crew"]; !strings.Contains(offer.Text, "Robin Whitfield added Kit Whitfield as co-chair of Clean Up Crew") || !strings.Contains(offer.Text, "Added by: Robin Whitfield") {
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
	cache, mux := newServer(t)
	on := func(id string) bool {
		for _, v := range cache.Model().Activity(id).Volunteers {
			if v.Email == parent {
				return true
			}
		}
		return false
	}
	if rec := call(t, mux, parent, "POST", "/api/team/volunteer", map[string]any{"id": "E017", "position": PositionVolunteer, "note": "evenings"}); rec.Code != http.StatusNoContent {
		t.Fatalf("sign up: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, "someone.else@heliosschool.org", "POST", "/api/team/volunteer", map[string]any{"id": "E018", "email": parent, "position": PositionVolunteer, "from": "E017"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger moved someone: %d", rec.Code)
	}
	if rec := call(t, mux, parent, "POST", "/api/team/volunteer", map[string]any{"id": "E018", "position": PositionVolunteer, "note": "evenings", "from": "E017"}); rec.Code != http.StatusNoContent {
		t.Fatalf("move: %d %s", rec.Code, rec.Body)
	}
	if on("E017") || !on("E018") {
		t.Fatalf("after the move: on E017 %v, on E018 %v", on("E017"), on("E018"))
	}
	if rec := call(t, mux, parent, "POST", "/api/team/volunteer", map[string]any{"id": "E019", "position": PositionVolunteer, "from": "E017"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("moved from a thing not signed up for: %d", rec.Code)
	}
	if rec := call(t, mux, parent, "POST", "/api/team/volunteer", map[string]any{"id": "E016", "position": PositionOpen}); rec.Code != http.StatusBadRequest {
		t.Fatalf("offered to co-chair where none is wanted: %d", rec.Code)
	}
}

func TestReorderChildren(t *testing.T) {
	cache, mux := newServer(t)
	titles := func() []string {
		out := []string{}
		for _, c := range cache.Model().Activity("E002").Children {
			out = append(out, c.Title)
		}
		return out
	}
	if got := titles(); !slices.Equal(got, []string{"Decor", "Childcare", "Marketing"}) {
		t.Fatalf("row order to start: %v", got)
	}
	body := map[string]any{"parent": "E002", "ids": []string{"E025", "E023", "E024"}}
	if rec := call(t, mux, parent, "POST", "/api/team/order", body); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent reordered: %d", rec.Code)
	}
	if rec := call(t, mux, chair, "POST", "/api/team/order", body); rec.Code != http.StatusNoContent {
		t.Fatalf("the chair could not reorder: %d %s", rec.Code, rec.Body)
	}
	if got := titles(); !slices.Equal(got, []string{"Marketing", "Decor", "Childcare"}) {
		t.Fatalf("order after: %v", got)
	}
	before := len(changeLog(t))
	if rec := call(t, mux, chair, "POST", "/api/team/order", map[string]any{"parent": "E002", "ids": []string{"E023", "E025", "E024"}}); rec.Code != http.StatusNoContent {
		t.Fatalf("second reorder: %d %s", rec.Code, rec.Body)
	}
	if got := titles(); !slices.Equal(got, []string{"Decor", "Marketing", "Childcare"}) || len(changeLog(t))-before != 1 {
		t.Fatalf("order after one move: %v, %d log rows", got, len(changeLog(t))-before)
	}
	if rec := call(t, mux, chair, "POST", "/api/team/order", map[string]any{"parent": "E002", "ids": []string{"E025", "E001"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a stranger's id was taken: %d", rec.Code)
	}
}

func TestSuggestApproveRenameDelete(t *testing.T) {
	cache, mux := newServer(t)
	suggestion := map[string]any{"year": "2026 - 2027", "title": "Kite Day", "category": "C06", "status": StatusOpen, "description": "Fly kites", "signUp": PositionOpen, "directSignUp": true}
	if rec := call(t, mux, parent, "POST", "/api/team/activity", suggestion); rec.Code != http.StatusNoContent {
		t.Fatalf("suggest: %d %s", rec.Code, rec.Body)
	}
	kite := byTitle(cache.Model(), "2026 - 2027", "Kite Day")
	if kite == nil || kite.Status != StatusPending || kite.AddedBy != parent || len(kite.Volunteers) != 1 || kite.Volunteers[0].Position != PositionOpen {
		t.Fatalf("suggestion landed as %+v", kite)
	}
	edit := map[string]any{"id": kite.ID, "year": "2026 - 2027", "title": "Kite Festival", "category": "C02", "status": StatusOpen, "directSignUp": true}
	if rec := call(t, mux, parent, "POST", "/api/team/activity", edit); rec.Code != http.StatusForbidden {
		t.Fatalf("a non-chair edited an activity: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/team/activity", edit); rec.Code != http.StatusNoContent {
		t.Fatalf("approve and rename: %d %s", rec.Code, rec.Body)
	}
	if byTitle(cache.Model(), "2026 - 2027", "Kite Day") != nil {
		t.Fatal("the old title survived the rename")
	}
	festival := cache.Model().Activity(kite.ID)
	if festival == nil || festival.Status != StatusOpen || len(festival.Volunteers) != 1 {
		t.Fatalf("renamed activity: %+v", festival)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/team/activity", map[string]string{"id": kite.ID}); rec.Code != http.StatusBadRequest {
		t.Fatalf("delete with a volunteer on it: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/team/volunteer", map[string]any{"id": kite.ID, "email": parent}); rec.Code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/team/activity", map[string]string{"id": kite.ID}); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Activity(kite.ID) != nil {
		t.Fatal("the activity survived deletion")
	}
}

func TestYearMoveCarriesTheTreeAndDeleteTakesTheLinks(t *testing.T) {
	cache, mux := newServer(t)
	spring := cache.Model().Activity("E002")
	edit := map[string]any{"id": "E002", "year": "2027 - 2028", "title": spring.Title, "category": spring.Category, "status": spring.Status, "directSignUp": true, "prettyId": spring.PrettyID}
	if rec := call(t, mux, admin, "POST", "/api/team/activity", edit); rec.Code != http.StatusNoContent {
		t.Fatalf("move year: %d %s", rec.Code, rec.Body)
	}
	for _, c := range cache.Model().Activity("E002").Children {
		if c.Year != "2027 - 2028" {
			t.Fatalf("%s stayed in %s", c.Title, c.Year)
		}
	}
	moved := 0
	for _, row := range tables(t)[activitiesTab] {
		if row["Parent"] == "E002" && row["Year"] == "2027 - 2028" {
			moved++
		}
	}
	years := map[string]int{}
	for _, row := range changeLog(t) {
		if row["Column"] == "Year" {
			years[row["Key"]+" "+row["Previous"]]++
			if row["Actor"] != admin || row["Action"] != "set" {
				t.Errorf("log row %v", row)
			}
		}
	}
	if moved != 3 || len(years) != 4 || years["Event ID=E023 2026 - 2027"] != 1 {
		t.Fatalf("sheet moved %d children, logged %v", moved, years)
	}
	if rec := call(t, mux, admin, "POST", "/api/team/activity", map[string]any{"year": "2026 - 2027", "title": "Bake Sale", "category": "C02", "status": StatusOpen, "directSignUp": true}); rec.Code != http.StatusNoContent {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	sale := byTitle(cache.Model(), "2026 - 2027", "Bake Sale")
	if rec := call(t, mux, admin, "POST", "/api/team/link", map[string]any{"id": sale.ID, "title": "Menu", "url": "https://example.org/menu"}); rec.Code != http.StatusNoContent {
		t.Fatalf("link: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/team/activity", map[string]string{"id": sale.ID}); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if cache.Count(linksTab, store.Row{"Event ID": sale.ID}) != 0 {
		t.Fatal("the link outlived its activity in memory")
	}
	for _, row := range tables(t)[linksTab] {
		if row["Event ID"] == sale.ID {
			t.Fatalf("the sheet kept %v", row)
		}
	}
	gone := 0
	for _, row := range changeLog(t) {
		if row["Action"] == "delete" && row["Tab"] == linksTab && row["Column"] == "URL" && row["Previous"] == "https://example.org/menu" {
			gone++
		}
	}
	if gone != 1 {
		t.Fatalf("the link's delete was not logged: %v", changeLog(t))
	}
}

func TestRenameKeepsTheTree(t *testing.T) {
	cache, mux := newServer(t)
	edit := map[string]any{
		"id": "E020", "year": "2026 - 2027", "title": "India Booth", "parent": "E001",
		"category": "C08", "status": StatusOpen, "coLeaderNeeded": true, "directSignUp": true,
	}
	if rec := call(t, mux, chair, "POST", "/api/team/activity", edit); rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	booth := cache.Model().Activity("E020")
	if booth == nil || booth.Title != "India Booth" || len(booth.Volunteers) != 1 || len(booth.Links) != 1 || len(booth.Children) != 1 || booth.Children[0].Parent != "E020" {
		t.Fatalf("renamed: %+v", booth)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/team/activity", map[string]string{"id": "E020"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("deleted something with children and volunteers: %d", rec.Code)
	}
	loop := map[string]any{"id": "E001", "year": "2026 - 2027", "title": "International Night", "parent": "E020", "category": "", "status": StatusOpen}
	if rec := call(t, mux, admin, "POST", "/api/team/activity", loop); rec.Code != http.StatusBadRequest {
		t.Fatalf("a parent loop was accepted: %d", rec.Code)
	}
}

// A co-chair approves what was suggested under their event by opening or
// finishing it; hiding stays an admin's, and a pending event of its own
// waits for an admin whoever chairs it.
func TestCoChairApproves(t *testing.T) {
	cache, mux := newServer(t)
	save := func(who, id, parentID, category, status string) int {
		act := cache.Model().Activity(id)
		return call(t, mux, who, "POST", "/api/team/activity", map[string]any{
			"id": id, "year": act.Year, "title": act.Title, "parent": parentID, "category": category, "status": status, "directSignUp": act.DirectSignUp,
		}).Code
	}
	status := func(id string) string { return cache.Model().Activity(id).Status }
	if code := save(chair, "E022", "E001", "C08", StatusHidden); code != http.StatusBadRequest {
		t.Fatalf("a co-chair hid a suggestion: %d", code)
	}
	if code := save(chair, "E022", "E001", "C08", StatusPending); code != http.StatusNoContent || status("E022") != StatusPending {
		t.Fatalf("a co-chair's save that leaves it pending: %d, %s", code, status("E022"))
	}
	if code := save(chair, "E022", "E001", "C08", StatusOpen); code != http.StatusNoContent || status("E022") != StatusOpen {
		t.Fatalf("the event's co-chair could not approve a suggestion: %d, %s", code, status("E022"))
	}
	if code := call(t, mux, admin, "POST", "/api/team/volunteer", map[string]any{"id": "E012", "email": chair, "position": PositionCoChair}).Code; code != http.StatusNoContent {
		t.Fatalf("make a co-chair: %d", code)
	}
	if code := save(chair, "E012", "", "C01", StatusOpen); code != http.StatusNoContent || status("E012") != StatusPending {
		t.Fatalf("a co-chair approved their own pending event: %d, %s", code, status("E012"))
	}
}

// A co-chair moves a thing only under something else they run.
func TestCoChairMovesOnlyUnderTheirOwn(t *testing.T) {
	cache, mux := newServer(t)
	const india = "deepa.natarajan@heliosschool.org"
	move := func(who, id, parentID string) int {
		act := cache.Model().Activity(id)
		return call(t, mux, who, "POST", "/api/team/activity", map[string]any{
			"id": id, "year": act.Year, "title": act.Title, "parent": parentID, "category": "", "status": act.Status, "directSignUp": act.DirectSignUp,
		}).Code
	}
	if code := move(india, "E020", "E002"); code != http.StatusForbidden {
		t.Fatalf("a co-chair moved their booth under an event they do not run: %d", code)
	}
	if code := move(india, "E020", ""); code != http.StatusForbidden {
		t.Fatalf("a co-chair made their booth an event of its own: %d", code)
	}
	if code := move(chair, "E016", "E003"); code != http.StatusForbidden {
		t.Fatalf("a co-chair moved a crew under an event they do not run: %d", code)
	}
	if code := move(chair, "E019", "E002"); code != http.StatusNoContent || cache.Model().Activity("E019").Parent != "E002" {
		t.Fatalf("a co-chair could not move a booth between their own events: %d", code)
	}
	if code := move(admin, "E016", "E003"); code != http.StatusNoContent {
		t.Fatalf("an admin could not move a crew: %d", code)
	}
}

func TestCopyToNextYear(t *testing.T) {
	cache, mux := newServer(t)
	if rec := call(t, mux, chair, "POST", "/api/team/copy", map[string]string{"id": "E001"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a co-chair copied: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/team/copy", map[string]string{"id": "E001"}); rec.Code != http.StatusNoContent {
		t.Fatalf("copy: %d %s", rec.Code, rec.Body)
	}
	next := byTitle(cache.Model(), "2027 - 2028", "International Night")
	if next == nil || next.ID == "E001" || next.Status != StatusOpen || next.Start != "" || len(next.Volunteers) != 0 || len(next.Descendants()) != 6 || byTitle(cache.Model(), "2027 - 2028", "Cybertron") != nil || len(next.Links) != 2 {
		t.Fatalf("copied activity: %+v", next)
	}
	for _, c := range next.Descendants() {
		if p := cache.Model().Activity(c.Parent); p == nil || p.Year != "2027 - 2028" {
			t.Fatalf("copied child %q points at parent %q in the wrong year", c.Title, c.Parent)
		}
	}
	if rec := call(t, mux, admin, "POST", "/api/team/copy", map[string]string{"id": "E001"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("copied twice: %d", rec.Code)
	}
}

func TestYears(t *testing.T) {
	if got := SchoolYear(mustTime("2026-09-09")); got != "2026 - 2027" {
		t.Errorf("september: %s", got)
	}
	if got := SchoolYear(mustTime("2027-03-01")); got != "2026 - 2027" {
		t.Errorf("march: %s", got)
	}
	if got := ShiftYear("2026 - 2027", -1); got != "2025 - 2026" {
		t.Errorf("shift: %s", got)
	}
	if err := CheckYear("2026 - 2028"); err == nil {
		t.Error("a two-year span passed")
	}
}

func TestEventCategories(t *testing.T) {
	cache, mux := newServer(t)
	m := cache.Model()
	night := m.Activity("E001")
	if len(night.Categories) != 2 || night.Categories[1].ID != "C08" || night.Categories[1].Adding != AddingYes {
		t.Fatalf("international night's categories: %+v", night.Categories)
	}
	if len(m.Categories) != 6 {
		t.Fatalf("the page should see only the six headings, got %d", len(m.Categories))
	}
	propose := func(who, category string) int {
		return call(t, mux, who, "POST", "/api/team/activity", map[string]any{
			"year": "2026 - 2027", "title": "Sweden", "parent": "E001", "category": category, "status": StatusOpen, "directSignUp": true,
		}).Code
	}
	if code := propose(parent, "C08"); code != http.StatusNoContent {
		t.Fatalf("a booth under an open category was refused: %d", code)
	}
	if sweden := byTitle(cache.Model(), "2026 - 2027", "Sweden"); sweden == nil || sweden.Status != StatusOpen {
		t.Fatalf("a booth added under a Yes category should be open: %+v", sweden)
	}
	if code := call(t, mux, parent, "POST", "/api/team/activity", map[string]any{
		"year": "2026 - 2027", "title": "Loose Booth", "parent": "E001", "category": "", "status": StatusOpen, "directSignUp": true,
	}).Code; code != http.StatusNoContent {
		t.Fatalf("an uncategorised proposal under an Approval Needed event was refused: %d", code)
	}
	if loose := byTitle(cache.Model(), "2026 - 2027", "Loose Booth"); loose == nil || loose.Status != StatusPending {
		t.Fatalf("an uncategorised proposal should wait for approval: %+v", loose)
	}
	if code := propose(parent, "C07"); code != http.StatusBadRequest {
		t.Fatalf("a proposal into a closed category went through: %d", code)
	}
	if code := propose(chair, "C07"); code != http.StatusNoContent {
		t.Fatalf("the co-chair could not add into a closed category: %d", code)
	}
	if code := propose(chair, "C01"); code != http.StatusBadRequest {
		t.Fatalf("a child took a page heading as its category: %d", code)
	}
	own := map[string]any{"eventId": "E001", "title": "Performances", "allowAdding": AddingYes}
	if rec := call(t, mux, parent, "POST", "/api/team/category", own); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent made an event category: %d", rec.Code)
	}
	if rec := call(t, mux, chair, "POST", "/api/team/category", own); rec.Code != http.StatusNoContent {
		t.Fatalf("the co-chair could not add an event category: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, chair, "POST", "/api/team/category", map[string]any{"title": "New Heading"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a co-chair made a page heading: %d", rec.Code)
	}
	if n := len(cache.Model().Activity("E001").Categories); n != 3 {
		t.Fatalf("event categories after adding: %d", n)
	}
	ids := []string{}
	for _, c := range cache.Model().Activity("E001").Categories {
		ids = append(ids, c.ID)
	}
	ids[0], ids[1] = ids[1], ids[0]
	if rec := call(t, mux, chair, "POST", "/api/team/categories/order", map[string]any{"eventId": "E001", "ids": ids}); rec.Code != http.StatusNoContent {
		t.Fatalf("reorder: %d %s", rec.Code, rec.Body)
	}
	after := cache.Model()
	got := []string{}
	for _, c := range after.Activity("E001").Categories {
		got = append(got, c.ID)
	}
	if !slices.Equal(got, ids) || after.Categories[0].ID != "C01" || after.Activity("E013").Categories[0].ID != "C09" {
		t.Fatalf("reorder: %v, or it leaked out of its scope", got)
	}
	if rec := call(t, mux, admin, "POST", "/api/team/categories/order", map[string]any{"eventId": "E001", "ids": ids[1:]}); rec.Code != http.StatusBadRequest {
		t.Fatalf("an order missing one was taken: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/team/copy", map[string]string{"id": "E001"}); rec.Code != http.StatusNoContent {
		t.Fatalf("copy: %d %s", rec.Code, rec.Body)
	}
	next := byTitle(cache.Model(), "2027 - 2028", "International Night")
	if next == nil || len(next.Categories) != 3 || next.Categories[0].ID == ids[0] || next.Categories[0].Title != after.Activity("E001").Categories[0].Title {
		t.Fatalf("copied categories: %+v", next.Categories)
	}
	norway := byTitle(cache.Model(), "2027 - 2028", "Norway")
	if norway == nil || cache.Model().Category(norway.Category).EventID != next.ID {
		t.Fatalf("copied child still names the old event's category: %+v", norway)
	}
}

func TestUncategorizedFallback(t *testing.T) {
	cache, mux := newServer(t)
	if len(cache.Model().Categories) != 6 {
		t.Fatalf("the heading appeared without anything in it")
	}
	next := tables(t)
	next[activitiesTab] = append(next[activitiesTab],
		store.Row{"Event ID": "E900", "Year": "2026 - 2027", "Title": "Blank", "Status": StatusOpen},
		store.Row{"Event ID": "E901", "Year": "2026 - 2027", "Title": "Unknown", "Category": "nope", "Status": StatusOpen},
		store.Row{"Event ID": "E902", "Year": "2026 - 2027", "Title": "Borrowed", "Category": "C07", "Status": StatusOpen},
		store.Row{"Event ID": "E903", "Year": "2026 - 2027", "Title": "Child", "Parent": "E001", "Category": "C09", "Status": StatusOpen},
	)
	m, err := BuildModel(context.Background(), next, bundled{})
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
	if rec := call(t, mux, parent, "POST", "/api/team/activity", add); rec.Code != http.StatusBadRequest {
		t.Fatalf("a proposal without a category went through: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/team/activity", add); rec.Code != http.StatusNoContent {
		t.Fatalf("admin add: %d %s", rec.Code, rec.Body)
	}
	loose := byTitle(cache.Model(), "2026 - 2027", "Loose End")
	if loose == nil || loose.Category != UncategorizedID || cache.Count(activitiesTab, store.Row{"Title": "Loose End", "Category": ""}) != 1 {
		t.Fatalf("loose end: %+v", loose)
	}
	if rec := call(t, mux, admin, "POST", "/api/team/category", map[string]any{"id": UncategorizedID, "title": "Misc"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("edited the built-in heading: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/team/category", map[string]any{"id": UncategorizedID}); rec.Code != http.StatusBadRequest {
		t.Fatalf("deleted the built-in heading: %d", rec.Code)
	}
}

func TestBrokenSheetRefusesToLoad(t *testing.T) {
	t.Chdir("../..")
	broken := t.TempDir()
	if err := os.MkdirAll(filepath.Join(broken, "events"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Categories", "Activities", "Links", "Settings", "Admins", "Redirects", "Change Log"} {
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
	if _, err := NewCache(dir, dir, bundled{}, func(string) bool { return false }, store.NewQueue()); err == nil || !strings.Contains(err.Error(), `missing column "Event ID"`) {
		t.Fatalf("a broken sheet loaded: %v", err)
	}
}

func TestHandWrittenRows(t *testing.T) {
	cache, _ := newServer(t)
	next := tables(t)
	next[activitiesTab] = append(next[activitiesTab],
		store.Row{"Event ID": "E900", "Title": "Bare Child", "Parent": "E020", "Status": StatusOpen},
		store.Row{"Event ID": "", "Title": "Gone", "Year": "nonsense", "Status": "Active", "Parent": "nope"},
	)
	next[volunteersTab] = append(next[volunteersTab], store.Row{"Event ID": "", "Email": "not an email", "Position": "Boss"})
	next[linksTab] = append(next[linksTab], store.Row{"Event ID": "", "Title": "Old", "URL": "https://example.com"})
	m, err := BuildModel(context.Background(), next, bundled{})
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
		store.Row{"Event ID": "E910", "Title": "Orphan", "Parent": "gone-id", "Status": StatusOpen},
		store.Row{"Event ID": "E911", "Title": "Orphan's Child", "Parent": "E910", "Status": StatusOpen},
	)
	next[volunteersTab] = append(next[volunteersTab],
		store.Row{"Event ID": "gone-id", "Email": parent, "Position": PositionVolunteer},
		store.Row{"Event ID": "E001", "Email": chair, "Position": PositionVolunteer},
	)
	next[linksTab] = append(next[linksTab], store.Row{"Event ID": "E910", "Title": "Lost", "URL": "https://example.com"})
	m, err = BuildModel(context.Background(), next, bundled{})
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
	if n := len(m.Activity("E001").Volunteers); n != len(cache.Model().Activity("E001").Volunteers) {
		t.Fatalf("the duplicate sign-up was added: %d volunteers", n)
	}
	next[activitiesTab] = append(next[activitiesTab], store.Row{"Event ID": "E901", "Title": "Lost", "Year": "2025 - 2026", "Parent": "E020", "Status": StatusOpen})
	if _, err := BuildModel(context.Background(), next, bundled{}); err == nil || !strings.Contains(err.Error(), "has its parent") {
		t.Fatalf("a child in another year loaded: %v", err)
	}
	next = tables(t)
	next[activitiesTab][20][store.OrderColumn] = "10"
	if _, err := BuildModel(context.Background(), next, bundled{}); err == nil || !strings.Contains(err.Error(), "ends in 0") {
		t.Fatalf("an order ending in 0 loaded: %v", err)
	}
}

func TestShowOnMainPage(t *testing.T) {
	cache, mux := newServer(t)
	if c := cache.Model().Category("C01"); !c.ShowOnMain {
		t.Fatalf("a heading defaults to being shown: %+v", c)
	}
	next := tables(t)
	next[categoriesTab][0]["Show On Main Page"] = ""
	next[categoriesTab][6]["Show On Main Page"] = "No"
	m, err := BuildModel(context.Background(), next, bundled{})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Category("C01").ShowOnMain || !m.Category("C07").ShowOnMain {
		t.Fatalf("blank or event-scoped categories should be shown")
	}
	off := false
	edit := map[string]any{"id": "C02", "title": "Activities", "allowAdding": AddingNo, "showOnMain": off}
	if rec := call(t, mux, admin, "POST", "/api/team/category", edit); rec.Code != http.StatusNoContent {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Category("C02").ShowOnMain || cache.Count(categoriesTab, store.Row{"Category ID": "C02", "Show On Main Page": "No"}) != 1 {
		t.Fatalf("the heading was not taken off the page")
	}
	own := map[string]any{"eventId": "E001", "title": "Shifts", "allowAdding": "", "showOnMain": off}
	if rec := call(t, mux, chair, "POST", "/api/team/category", own); rec.Code != http.StatusNoContent {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	if cache.Count(categoriesTab, store.Row{"Title": "Shifts", "Show On Main Page": ""}) != 1 {
		t.Fatalf("an event category carried the page flag")
	}
}

func TestPrettyIDs(t *testing.T) {
	cache, mux := newServer(t)
	m := cache.Model()
	if m.ByPretty("International-Night") != m.Activity("E001") || m.ByPretty("nope") != nil {
		t.Fatalf("pretty lookup")
	}
	edit := func(id, pretty string, takeOver bool) *httptest.ResponseRecorder {
		act := cache.Model().Activity(id)
		return call(t, mux, admin, "POST", "/api/team/activity", map[string]any{
			"id": id, "year": act.Year, "title": act.Title, "category": act.Category, "status": act.Status,
			"directSignUp": true, "prettyId": pretty, "takeOver": takeOver,
		})
	}
	if rec := edit("E002", "Bad Address!", false); rec.Code != http.StatusBadRequest {
		t.Fatalf("a pretty id with spaces went through: %d", rec.Code)
	}
	rec := edit("E002", "international-night", false)
	var conflict prettyConflict
	if rec.Code != http.StatusConflict || json.Unmarshal(rec.Body.Bytes(), &conflict) != nil || conflict.Prior || conflict.ID != "E001" {
		t.Fatalf("same-year clash: %d %s", rec.Code, rec.Body)
	}
	if rec := edit("E002", "international-night", true); rec.Code != http.StatusConflict {
		t.Fatalf("take-over of a current address went through: %d", rec.Code)
	}
	last := byTitle(cache.Model(), "2025 - 2026", "International Night")
	rec = edit("E002", "international-night-2025", false)
	if rec.Code != http.StatusConflict || json.Unmarshal(rec.Body.Bytes(), &conflict) != nil || !conflict.Prior || conflict.ID != last.ID || conflict.Renamed != "international-night-2025-2025" {
		t.Fatalf("prior-year clash: %d %s", rec.Code, rec.Body)
	}
	if rec := edit("E002", "international-night-2025", true); rec.Code != http.StatusNoContent {
		t.Fatalf("take-over: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model()
	if m.Activity("E002").PrettyID != "international-night-2025" || m.Activity(last.ID).PrettyID != "international-night-2025-2025" {
		t.Fatalf("after take-over: %q %q", m.Activity("E002").PrettyID, m.Activity(last.ID).PrettyID)
	}
	if m.Resolve("intl-night") != m.Activity("E001") || m.Resolve("/v/intl-night") != m.Activity("E001") || m.Resolve("nope") != nil {
		t.Fatalf("sample redirect")
	}
	if rec := edit("E002", "spring-party", false); rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	if rec := edit("E002", "", false); rec.Code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model()
	for _, old := range []string{"/v/spring-celebration", "/v/international-night-2025", "/v/spring-party", "/activities/E002"} {
		if m.Resolve(old) != m.Activity("E002") {
			t.Fatalf("%s did not reach the event: %+v", old, m.Redirects)
		}
	}
	if n := cache.Count(redirectsTab, store.Row{"Type": RedirectActivity, "Old": "/v/spring-party", "New": "/activities/E002"}); n != 1 {
		t.Fatalf("a removed address should redirect to the row: %+v", m.Redirects)
	}
	if rec := edit("E002", "international-night-2025", false); rec.Code != http.StatusNoContent {
		t.Fatalf("restore: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model()
	norway, india := byTitle(m, "2026 - 2027", "Norway"), byTitle(m, "2026 - 2027", "India")
	if m.PathOf(norway) != "/v/international-night/"+norway.ID {
		t.Fatalf("child path %q", m.PathOf(norway))
	}
	child := func(node *Activity, pretty string) *httptest.ResponseRecorder {
		return call(t, mux, admin, "POST", "/api/team/activity", map[string]any{
			"id": node.ID, "year": node.Year, "title": node.Title, "parent": node.Parent, "category": node.Category, "status": node.Status,
			"directSignUp": true, "prettyId": pretty,
		})
	}
	if rec := child(norway, "norway"); rec.Code != http.StatusNoContent {
		t.Fatalf("child pretty: %d %s", rec.Code, rec.Body)
	}
	if rec := child(india, "norway"); rec.Code != http.StatusConflict {
		t.Fatalf("two siblings took one address: %d", rec.Code)
	}
	m = cache.Model()
	if m.Resolve("/v/international-night/norway") != m.Activity(norway.ID) || m.Resolve("/v/international-night/"+norway.ID) != m.Activity(norway.ID) || m.Resolve("/v/intl-night/norway") != m.Activity(norway.ID) {
		t.Fatalf("child by path: %q", m.PathOf(m.Activity(norway.ID)))
	}
	if rec := edit("E001", "inight", false); rec.Code != http.StatusNoContent {
		t.Fatalf("rename event: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model()
	if m.PathOf(m.Activity(norway.ID)) != "/v/inight/norway" || m.Resolve("/v/international-night/norway") != m.Activity(norway.ID) || m.Resolve("/v/intl-night/norway") != m.Activity(norway.ID) {
		t.Fatalf("event rename did not carry the booth: %q", m.PathOf(m.Activity(norway.ID)))
	}
	if rec := edit("E002", "International-Night-2025", false); rec.Code != http.StatusNoContent {
		t.Fatalf("re-saving own address: %d %s", rec.Code, rec.Body)
	}
	next := tables(t)
	for _, row := range next[activitiesTab] {
		if row["Event ID"] == last.ID {
			row["Pretty ID"] = "inight"
		}
	}
	dup, err := BuildModel(context.Background(), next, bundled{})
	if err != nil {
		t.Fatal(err)
	}
	if dup.ByPretty("inight") != dup.Activity("E001") || dup.Activity(last.ID).PrettyID != "" || dup.Skipped.PrettyIDs != 1 {
		t.Fatalf("duplicate in the sheet: %+v", dup.Skipped)
	}
}

func TestSharePreview(t *testing.T) {
	cache, mux := newServer(t)
	head := PreviewHead(cache)
	tags := head(httptest.NewRequest("GET", "https://team.heliosian.com/v/intl-night/", nil))
	for _, want := range []string{`og:title" content="International Night"`, `og:url" content="https://team.heliosian.com/v/international-night"`,
		`og:image" content="https://team.heliosian.com/open/share/E001.png"`, `Thursday, September 24 · 4:00–6:00 PM — We invite you`} {
		if !strings.Contains(tags, want) {
			t.Fatalf("preview lacks %s:\n%s", want, tags)
		}
	}
	tags = head(httptest.NewRequest("GET", "https://team.heliosian.com/v/international-night/E020", nil))
	for _, want := range []string{`og:title" content="India · International Night"`, `Thursday, September 24 · 4:00–6:00 PM`} {
		if !strings.Contains(tags, want) {
			t.Fatalf("child preview lacks %s:\n%s", want, tags)
		}
	}
	for _, path := range []string{"/activities/E006", "/my", "/"} {
		tags = head(httptest.NewRequest("GET", "https://team.heliosian.com"+path, nil))
		for _, want := range []string{`og:title" content="HCA-Team"`, `og:image" content="https://team.heliosian.com/open/share/upcoming.png"`, "Volunteers needed: "} {
			if !strings.Contains(tags, want) {
				t.Fatalf("%s preview lacks %s:\n%s", path, want, tags)
			}
		}
		if strings.Contains(tags, "E006") {
			t.Fatalf("%s preview names a hidden thing:\n%s", path, tags)
		}
	}
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
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/open/share/E001.png", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || rec.Body.Len() < 10000 {
		t.Fatalf("card: %d %s %d bytes", rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/open/share/E006.png", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("a hidden thing has a card: %d", rec.Code)
	}
	for _, status := range []string{StatusHidden, StatusPending} {
		next := tables(t)
		for _, row := range next[activitiesTab] {
			if row["Event ID"] == "E001" {
				row["Status"], row["Added By"] = status, ""
			}
		}
		parked, err := BuildModel(context.Background(), next, bundled{})
		if err != nil {
			t.Fatal(err)
		}
		if child := parked.Activity("E020"); child.Status != StatusOpen || previewable(parked, child) {
			t.Fatalf("%s parent: %s (%s) previews", status, child.Title, child.Status)
		}
		for _, a := range needs(parked, now()) {
			if a.ID == "E001" || a.Parent == "E001" {
				t.Fatalf("%s parent: %s is a need", status, a.Title)
			}
		}
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/open/share/upcoming.png", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || rec.Body.Len() < 10000 {
		t.Fatalf("portal card: %d %s %d bytes", rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
	}
}

func TestWhenSpansDays(t *testing.T) {
	for _, tc := range []struct {
		start, end, line, day, hours string
	}{
		{"2026-09-24", "", "Thursday, September 24", "Thursday, September 24", ""},
		{"2026-09-24 16:00", "2026-09-24 18:00", "Thursday, September 24 · 4:00–6:00 PM", "Thursday, September 24", "4:00 – 6:00 PM"},
		{"2026-10-02", "2026-10-04", "Friday, October 2 – Sunday, October 4", "Friday, October 2 – Sunday, October 4", ""},
		{"2026-10-02 16:00", "2026-10-04 12:00", "Friday, October 2 – Sunday, October 4 · Fri 4:00 PM – Sun 12:00 PM", "Friday, October 2 – Sunday, October 4", "Fri 4:00 PM – Sun 12:00 PM"},
		{"2026-10-30 09:00", "2026-11-01", "Friday, October 30 – Sunday, November 1 · Fri 9:00 AM", "Friday, October 30 – Sunday, November 1", "Fri 9:00 AM"},
	} {
		a := &Activity{Start: tc.start, End: tc.end}
		if got := when(a); got != tc.line {
			t.Errorf("when(%q, %q) = %q, want %q", tc.start, tc.end, got, tc.line)
		}
		if day, hours := whenLines(a); day != tc.day || hours != tc.hours {
			t.Errorf("whenLines(%q, %q) = %q, %q, want %q, %q", tc.start, tc.end, day, hours, tc.day, tc.hours)
		}
	}
}

func TestRedirects(t *testing.T) {
	cache, mux := newServer(t)
	for cell, want := range map[string]string{
		"https://hca.heliosian.com/dl/signup/s/768d91/r/nsSPomxFcPrSfoRCAgzI": "/dl/signup/s/768d91/r/nsSPomxFcPrSfoRCAgzI",
		" /dl/signup/s/768d91/ ": "/dl/signup/s/768d91", "intl-night": "/v/intl-night", "https://hca.heliosian.com": "/",
	} {
		if got := redirectPath(cell); got != want {
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
	if rec := call(t, mux, chair, "POST", "/api/team/redirect", map[string]string{"old": oldLink, "new": "/v/international-night"}); rec.Code != http.StatusForbidden {
		t.Fatalf("chair: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, admin, "POST", "/api/team/redirect", map[string]string{"old": oldLink, "new": "/v/international-night"}); rec.Code != http.StatusNoContent {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	m := cache.Model()
	oldPath := "/dl/signup/s/768d91/r/nsSPomxFcPrSfoRCAgzI"
	if n := cache.Count(redirectsTab, store.Row{"Type": RedirectAdmin, "Old": oldPath, "New": "/v/international-night"}); n != 1 {
		t.Fatalf("row not written: %+v", m.Redirects)
	}
	if m.Resolve(oldPath) != m.Activity("E001") || m.Destination(oldPath) != "/v/international-night" {
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
			if rec := call(t, mux, admin, "POST", "/api/team/redirect", map[string]string{"old": "/b", "new": "/a"}); rec.Code != http.StatusNoContent {
				t.Fatalf("b: %d %s", rec.Code, rec.Body)
			}
		}
		if rec := call(t, mux, admin, "POST", "/api/team/redirect", bad); rec.Code < 400 {
			t.Errorf("%v was accepted", bad)
		}
	}
	elsewhere := "https://celebrate.heliosian.com/parties/abc"
	if rec := call(t, mux, admin, "POST", "/api/team/redirect", map[string]string{"original": oldLink, "old": "/dl/signup/s/768d91", "new": elsewhere}); rec.Code != http.StatusNoContent {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model()
	if n := cache.Count(redirectsTab, store.Row{"Old": oldPath}); n != 0 {
		t.Fatalf("old row still there")
	}
	if got := m.Destination(oldPath); got != elsewhere+"/r/nsSPomxFcPrSfoRCAgzI" {
		t.Fatalf("under the edited prefix: %q", got)
	}
	if m.Resolve(oldPath) != nil {
		t.Fatalf("a chain that leaves the site names no activity")
	}
	if rec := call(t, mux, admin, "POST", "/api/team/redirect", map[string]string{"original": "intl-night", "old": "/v/intl-nite", "new": "/v/international-night"}); rec.Code != http.StatusNoContent {
		t.Fatalf("edit the sample row: %d %s", rec.Code, rec.Body)
	}
	if n := cache.Count(redirectsTab, store.Row{"Type": "pretty", "Old": "/v/intl-nite"}); n != 1 {
		t.Fatalf("the sample row's kind was not kept: %+v", cache.Model().Redirects)
	}
	if rec := call(t, mux, admin, "POST", "/api/team/redirect", map[string]string{"old": "/v/fair", "new": "/"}); rec.Code != http.StatusNoContent {
		t.Fatalf("to the front page: %d %s", rec.Code, rec.Body)
	}
	handler := Redirected(cache, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	for path, want := range map[string]string{
		"/dl/signup/s/768d91/r/nsSPomxFcPrSfoRCAgzI?x=1": elsewhere + "/r/nsSPomxFcPrSfoRCAgzI?x=1",
		"/v/intl-nite": "/v/international-night", "/v/international-night": "", "/calendar": "", "/nowhere": "",
		"/v/fair": "/", "/v/fair/calendar": "/calendar", "/v/fair//elsewhere.example": "", "/v/fair/\\elsewhere.example": "",
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
	if rec := call(t, mux, admin, "DELETE", "/api/team/redirect", map[string]string{"old": "/dl/signup/s/768d91"}); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Destination(oldPath); got != "" {
		t.Fatalf("deleted redirect still sends to %q", got)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/team/redirect", map[string]string{"old": "/dl/signup/s/768d91"}); rec.Code != http.StatusNotFound {
		t.Fatalf("delete again: %d %s", rec.Code, rec.Body)
	}
}
