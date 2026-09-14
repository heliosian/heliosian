package birthday

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/data"
	"heliosian/internal/mail"
)

// sentMail keeps what the app sends, and waits for it, since sending
// happens off the request.
type sentMail struct {
	mu       sync.Mutex
	messages []mail.Message
}

func (s *sentMail) Send(_ context.Context, m mail.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, m)
	return nil
}

func (s *sentMail) wait(t *testing.T, n int) []mail.Message {
	t.Helper()
	for i := 0; i < 100; i++ {
		s.mu.Lock()
		if len(s.messages) >= n {
			out := append([]mail.Message{}, s.messages...)
			s.mu.Unlock()
			return out
		}
		s.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("waited for %d messages", n)
	return nil
}

// sent is the server's outbox, fresh per newServer so one test's late sends
// never land in another's.
var sent = &sentMail{}

// joined is who the app put on its home list, per newServer.
var joined []string

const (
	parent = "robin.whitfield@heliosschool.org"
	admin  = "jordan.whitfield@heliosschool.org"
)

type syncQueue struct{}

func (syncQueue) Add(f func()) { f() }

type fakeDirectory struct{}

var staff = []Person{
	{Email: "dana.hawkins@heliosschool.org", Name: "Dana Hawkins", JobTitle: "Art Teacher", Department: "Co-Curriculars and Specialists"},
	{Email: "grace.kim@heliosschool.org", Name: "Grace Kim", JobTitle: "Head of School", Department: "Admin and Office Staff"},
	{Email: "bill.ryder@heliosschool.org", Name: "Bill Ryder", JobTitle: "Office Manager", Department: "Admin and Office Staff"},
	{Email: "ruth.amari@heliosschool.org", Name: "Ruth Amari", JobTitle: "Kindergarten Teacher", Department: "Classroom Teachers"},
	{Email: "miguel.santos@heliosschool.org", Name: "Miguel Santos", JobTitle: "1st Grade Teacher", Department: "Classroom Teachers"},
	{Email: "alice.fontaine@heliosschool.org", Name: "Alice Fontaine", JobTitle: "2nd Grade Teacher", Department: "Classroom Teachers"},
	{Email: "peter.okafor@heliosschool.org", Name: "Peter Okafor", JobTitle: "3rd Grade Teacher", Department: "Classroom Teachers"},
	{Email: "susan.byrne@heliosschool.org", Name: "Susan Byrne", JobTitle: "4th Grade Teacher", Department: "Classroom Teachers"},
	{Email: "hana.ito@heliosschool.org", Name: "Hana Ito", JobTitle: "5th/6th Grade Teacher", Department: "Classroom Teachers"},
	{Email: "marcus.bell@heliosschool.org", Name: "Marcus Bell", JobTitle: "5th/6th Grade Teacher", Department: "Classroom Teachers"},
	{Email: "lena.vogel@heliosschool.org", Name: "Lena Vogel", JobTitle: "5th/6th Grade Teacher", Department: "Classroom Teachers"},
	{Email: "tom.grady@heliosschool.org", Name: "Tom Grady", JobTitle: "5th/6th Grade Teacher", Department: "Classroom Teachers"},
	{Email: "ivy.chen@heliosschool.org", Name: "Ivy Chen", JobTitle: "Humanities Teacher", Department: "Classroom Teachers"},
	{Email: "raj.malhotra@heliosschool.org", Name: "Raj Malhotra", JobTitle: "Science Teacher", Department: "Classroom Teachers"},
	{Email: "kate.doyle@heliosschool.org", Name: "Kate Doyle", JobTitle: "Humanities Teacher", Department: "Classroom Teachers"},
	{Email: "omar.farouk@heliosschool.org", Name: "Omar Farouk", JobTitle: "Science Teacher", Department: "Classroom Teachers"},
	{Email: "hank.morrow@heliosschool.org", Name: "Hank Morrow", JobTitle: "Facilities Manager", Department: "Facilities Staff"},
	{Email: "sasha.pike@heliosschool.org", Name: "Sasha Pike", JobTitle: "Chess Club", Department: "Co-Curriculars and Specialists"},
}

func (fakeDirectory) Resolve(email string) string { return email }

func (fakeDirectory) Person(email string) (Person, bool) {
	for _, p := range staff {
		if p.Email == email {
			return p, true
		}
	}
	if email == admin {
		return Person{Email: email, Name: "Jordan Whitfield"}, true
	}
	return Person{}, false
}

func (fakeDirectory) Staff() []Person { return staff }

func (fakeDirectory) People() []Person {
	return append([]Person{{Email: admin, Name: "Jordan Whitfield"}}, staff...)
}

func (fakeDirectory) Alerts(string) (int, bool) { return 0, false }

func (fakeDirectory) Departments() []string {
	return []string{"Admin and Office Staff", "Co-Curriculars and Specialists", "Classroom Teachers", "Facilities Staff"}
}

func newServer(t *testing.T) (*Cache, *http.ServeMux) {
	t.Helper()
	t.Chdir("../..")
	now = func() time.Time { return mustTime("2026-09-09") }
	dir := &data.Dir{Root: "sampledata"}
	cache, err := NewCache(dir, func(e string) bool { return e == admin }, syncQueue{})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	sent = &sentMail{}
	joined = nil
	Register(mux, cache, dir, syncQueue{}, nil, fakeDirectory{}, func() []string { return []string{admin} }, nil, sent, "Helios Staff Birthdays <birthday@example.org>", "https://birthday.example.org", func(email string) error {
		joined = append(joined, email)
		return nil
	})
	return cache, mux
}

func call(t *testing.T, mux *http.ServeMux, as, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	auth.Fixed(as, mux).ServeHTTP(rec, req)
	return rec
}

func view(t *testing.T, cache *Cache, as string) View {
	t.Helper()
	return Render(cache.Model(), fakeDirectory{}, as, as == admin, now())
}

func find(list []StaffView, email string) *StaffView {
	for i := range list {
		if list[i].Email == email {
			return &list[i]
		}
	}
	return nil
}

func TestSampleLoads(t *testing.T) {
	cache, _ := newServer(t)
	m := cache.Model()
	if len(m.Birthdays) != 18 || len(m.Charities) != 7 || len(m.NewsletterDates) != 57 {
		t.Fatalf("got %d birthdays, %d charities, %d newsletter dates", len(m.Birthdays), len(m.Charities), len(m.NewsletterDates))
	}
	if !m.Skipped("hank.morrow@heliosschool.org") || m.InPipeline("hank.morrow@heliosschool.org") || m.Skipped("omar.farouk@heliosschool.org") || !m.InPipeline("omar.farouk@heliosschool.org") {
		t.Fatal("participation levels did not load")
	}
	if _, err := BuildModel(&Tables{Birthdays: []map[string]string{{"Email": "x@heliosschool.org", "Participation": LevelNoNewsletter}}, Charities: m.charityRows(), Settings: m.settingRows()}); err == nil {
		t.Fatal("a blank birthday without a skip loaded")
	}
}

func TestYears(t *testing.T) {
	y := YearContaining(mustTime("2026-09-09"), time.August, 14)
	if y.Label != "2026 - 2027" || y.Start != mustTime("2026-08-14") || y.End != mustTime("2027-08-14") {
		t.Fatalf("year: %+v", y)
	}
	if got := YearContaining(mustTime("2026-08-13"), time.August, 14).Label; got != "2025 - 2026" {
		t.Errorf("the day before the turnover: %s", got)
	}
	if got := y.Occurrence(mustTime("1978-06-20")); got != mustTime("2027-06-20") {
		t.Errorf("summer birthday: %s", got)
	}
	if got := y.Occurrence(mustTime("1988-02-29")); got != mustTime("2027-02-28") {
		t.Errorf("leap day: %s", got)
	}
	if got := y.Occurrence(mustTime("1982-08-13")); got != mustTime("2027-08-13") {
		t.Errorf("last day of the year: %s", got)
	}
	dates := []string{"2026-08-21", "2026-09-04", "2026-09-18", "2027-06-04", "2027-09-03"}
	if d, ok := y.Newsletter(mustTime("2026-09-10"), dates); !ok || d != mustTime("2026-09-04") {
		t.Errorf("the last newsletter before: %s %v", d, ok)
	}
	if d, ok := y.Newsletter(mustTime("2026-09-04"), dates); !ok || d != mustTime("2026-08-21") {
		t.Errorf("a birthday on an issue's day goes out the issue before: %s %v", d, ok)
	}
	if d, ok := y.Newsletter(mustTime("2026-08-20"), dates); !ok || d != mustTime("2026-08-21") {
		t.Errorf("a birthday before the first issue lands in it: %s %v", d, ok)
	}
	if d, ok := y.Newsletter(mustTime("2027-06-20"), dates); !ok || d != mustTime("2027-06-04") {
		t.Errorf("last newsletter before a summer birthday: %s %v", d, ok)
	}
	if _, ok := y.Newsletter(mustTime("2026-09-01"), []string{"2025-09-05"}); ok {
		t.Error("a date outside the year was picked")
	}
	if got := RequestBy(mustTime("2026-08-21"), DefaultRequestLeadDays); got != mustTime("2026-08-13") {
		t.Errorf("request by: %s", got)
	}
	if err := CheckYear("2026 - 2028"); err == nil {
		t.Error("a two-year span passed")
	}
}

func TestRenderStages(t *testing.T) {
	cache, _ := newServer(t)
	v := view(t, cache, parent)
	if v.Year.Current != "2026 - 2027" || v.Year.Last != "2025 - 2026" || v.Year.Start != "2026-08-14" || v.Year.End != "2027-08-13" {
		t.Fatalf("year view: %+v", v.Year)
	}
	want := map[string]string{
		"dana.hawkins@heliosschool.org":   StageComplete,
		"grace.kim@heliosschool.org":      StageNewsletter,
		"bill.ryder@heliosschool.org":     StageResponse,
		"ruth.amari@heliosschool.org":     StageOutreach,
		"miguel.santos@heliosschool.org":  StageOutreach,
		"alice.fontaine@heliosschool.org": StageWait,
		"omar.farouk@heliosschool.org":    StageComplete,
		"tom.grady@heliosschool.org":      StageWait,
	}
	for email, stage := range want {
		sv := find(v.Staff, email)
		if sv == nil || sv.Stage != stage {
			t.Errorf("%s: want %s, got %+v", email, stage, sv)
		}
	}
	dana := find(v.Staff, "dana.hawkins@heliosschool.org")
	if dana.BirthdayThisYear != "2026-08-20" || dana.NewsletterDate != "2026-08-21" || dana.RequestBy != "2026-08-13" || dana.AssignedToName != "Jordan Whitfield" {
		t.Errorf("dana: %+v", dana)
	}
	if dana.LastDonation == nil || dana.LastDonation.Charity != "Birthfund" || dana.Donation == nil || dana.Donation.UsedOn == "" {
		t.Errorf("dana's donations: %+v %+v", dana.Donation, dana.LastDonation)
	}
	if miguel := find(v.Staff, "miguel.santos@heliosschool.org"); miguel.AssignedTo != "" || miguel.Department != "Classroom Teachers" {
		t.Errorf("miguel: %+v", miguel)
	}
	if kate := find(v.Staff, "kate.doyle@heliosschool.org"); kate.NewsletterDate != "2026-10-23" || kate.RequestBy != "2026-10-15" {
		t.Errorf("override: %+v", kate)
	}
	if hana := find(v.Staff, "hana.ito@heliosschool.org"); hana.BirthdayThisYear != "2027-02-28" || hana.NewsletterDate != "2027-02-26" {
		t.Errorf("leap day: %+v", hana)
	}
	if tom := find(v.Staff, "tom.grady@heliosschool.org"); tom.NewsletterDate != "2027-06-04" || tom.RequestBy != "2027-05-27" {
		t.Errorf("summer: %+v", tom)
	}
	if omar := find(v.Staff, "omar.farouk@heliosschool.org"); omar.Level != LevelNoNewsletter || omar.Donation == nil || omar.Donation.UsedOn != "" {
		t.Errorf("no newsletter: %+v", omar)
	}
	if find(v.Staff, "former.teacher@heliosschool.org") != nil {
		t.Error("someone the directory does not list reached the pipeline")
	}
	if bill := find(v.Staff, "bill.ryder@heliosschool.org"); len(bill.Notes) != 1 {
		t.Errorf("notes: %+v", bill.Notes)
	}
	if v.Staff[0].Email != "dana.hawkins@heliosschool.org" || v.Staff[len(v.Staff)-1].Email != "raj.malhotra@heliosschool.org" {
		t.Errorf("order: %s … %s", v.Staff[0].Email, v.Staff[len(v.Staff)-1].Email)
	}
	if len(v.Skipped) != 1 || v.Skipped[0].Email != "hank.morrow@heliosschool.org" || v.Skipped[0].Name != "Hank Morrow" {
		t.Errorf("skipped: %+v", v.Skipped)
	}
	if len(v.Missing) != 1 || v.Missing[0].Email != "sasha.pike@heliosschool.org" {
		t.Errorf("missing: %+v", v.Missing)
	}
	if v.User.Name != "Robin Whitfield" || v.User.IsAdmin || !view(t, cache, admin).User.IsAdmin {
		t.Errorf("user: %+v", v.User)
	}
}

func TestPipeline(t *testing.T) {
	cache, mux := newServer(t)
	miguel := map[string]any{"email": "Miguel.Santos@heliosschool.org"}
	if rec := call(t, mux, parent, "POST", "/api/birthday/assign", miguel); rec.Code != http.StatusNoContent {
		t.Fatalf("assign: %d %s", rec.Code, rec.Body)
	}
	sv := find(view(t, cache, parent).Staff, "miguel.santos@heliosschool.org")
	if sv.AssignedTo != parent || sv.AssignedOn != "2026-09-09" || sv.Stage != StageOutreach {
		t.Fatalf("after assign: %+v", sv)
	}
	// The assignee gets the day to ask by as an invite, with the way to the page.
	invites := sent.wait(t, 1)
	m := invites[len(invites)-1]
	if len(m.To) != 1 || m.To[0] != parent || m.Subject != "Ask Miguel Santos about their birthday charity" || !strings.Contains(m.HTML, "/staff/miguel.santos\"") || m.Headers["Message-ID"] == "" {
		t.Fatalf("invite mail: %+v", m)
	}
	if len(m.Attachments) != 1 || m.Attachments[0].Name != "invite.ics" {
		t.Fatalf("invite attachment: %+v", m.Attachments)
	}
	// Unfolded, since the file wraps long lines at 75 octets.
	ics := strings.ReplaceAll(string(m.Attachments[0].Content), "\r\n ", "")
	for _, want := range []string{"METHOD:REQUEST", "DTSTART;VALUE=DATE:20260903", "DTEND;VALUE=DATE:20260904", "SUMMARY:Ask Miguel Santos about their birthday charity", "ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED;RSVP=FALSE:mailto:" + parent, "UID:birthday-miguel.santos@heliosschool.org-2026-2027@heliosian.com", "/staff/miguel.santos"} {
		if !strings.Contains(ics, want) {
			t.Fatalf("invite lacks %q:\n%s", want, ics)
		}
	}
	if rec := call(t, mux, parent, "POST", "/api/birthday/outreach", map[string]any{"email": miguel["email"], "contacted": true}); rec.Code != http.StatusNoContent {
		t.Fatalf("outreach: %d %s", rec.Code, rec.Body)
	}
	if sv = find(view(t, cache, parent).Staff, "miguel.santos@heliosschool.org"); sv.Stage != StageResponse || sv.ContactedBy != parent {
		t.Fatalf("after outreach: %+v", sv)
	}
	if rec := call(t, mux, parent, "POST", "/api/birthday/donation", map[string]any{"email": miguel["email"], "charity": "Sierra Club"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a prohibited charity was accepted: %d", rec.Code)
	}
	if rec := call(t, mux, parent, "POST", "/api/birthday/used", map[string]any{"email": miguel["email"], "used": true}); rec.Code != http.StatusBadRequest {
		t.Fatalf("used before any donation: %d", rec.Code)
	}
	if rec := call(t, mux, parent, "POST", "/api/birthday/donation", map[string]any{"email": miguel["email"], "charity": "Rocket Dog Rescue", "note": "For the dogs"}); rec.Code != http.StatusNoContent {
		t.Fatalf("donation: %d %s", rec.Code, rec.Body)
	}
	if sv = find(view(t, cache, parent).Staff, "miguel.santos@heliosschool.org"); sv.Stage != StageNewsletter || sv.Donation.Note != "For the dogs" {
		t.Fatalf("after donation: %+v", sv)
	}
	if rec := call(t, mux, parent, "POST", "/api/birthday/used", map[string]any{"email": miguel["email"], "used": true}); rec.Code != http.StatusNoContent {
		t.Fatalf("used: %d %s", rec.Code, rec.Body)
	}
	if sv = find(view(t, cache, parent).Staff, "miguel.santos@heliosschool.org"); sv.Stage != StageComplete || sv.Donation.UsedBy != parent {
		t.Fatalf("after used: %+v", sv)
	}
	if rec := call(t, mux, parent, "POST", "/api/birthday/used", map[string]any{"email": miguel["email"], "used": false}); rec.Code != http.StatusNoContent {
		t.Fatalf("unused: %d %s", rec.Code, rec.Body)
	}
	if sv = find(view(t, cache, parent).Staff, "miguel.santos@heliosschool.org"); sv.Stage != StageNewsletter || sv.Donation.UsedOn != "" {
		t.Fatalf("after unused: %+v", sv)
	}
	if rec := call(t, mux, parent, "DELETE", "/api/birthday/donation", miguel); rec.Code != http.StatusNoContent {
		t.Fatalf("remove donation: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, parent, "POST", "/api/birthday/outreach", map[string]any{"email": miguel["email"], "contacted": false}); rec.Code != http.StatusNoContent {
		t.Fatalf("undo outreach: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, parent, "DELETE", "/api/birthday/assign", miguel); rec.Code != http.StatusNoContent {
		t.Fatalf("unassign: %d %s", rec.Code, rec.Body)
	}
	if sv = find(view(t, cache, parent).Staff, "miguel.santos@heliosschool.org"); sv.Stage != StageOutreach || sv.AssignedTo != "" || sv.Donation != nil {
		t.Fatalf("back to the start: %+v", sv)
	}
	if rec := call(t, mux, parent, "POST", "/api/birthday/assign", map[string]any{"email": "hank.morrow@heliosschool.org"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("assigned someone who opted out: %d", rec.Code)
	}
	if rec := call(t, mux, parent, "POST", "/api/birthday/assign", map[string]any{"email": "sasha.pike@heliosschool.org"}); rec.Code != http.StatusNotFound {
		t.Fatalf("assigned someone with no birthday: %d", rec.Code)
	}
	if rec := call(t, mux, parent, "POST", "/api/birthday/participation", map[string]any{"email": miguel["email"], "level": LevelSkip, "note": "Asked in person"}); rec.Code != http.StatusNoContent {
		t.Fatalf("skip: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, parent, "POST", "/api/birthday/assign", miguel); rec.Code != http.StatusBadRequest {
		t.Fatalf("assigned someone who opted out: %d", rec.Code)
	}
	v := view(t, cache, parent)
	if find(v.Staff, "miguel.santos@heliosschool.org") != nil || find(v.Skipped, "miguel.santos@heliosschool.org") == nil {
		t.Fatal("a skipped birthday stayed in the pipeline")
	}
	if rec := call(t, mux, parent, "DELETE", "/api/birthday/participation", miguel); rec.Code != http.StatusNoContent {
		t.Fatalf("unskip: %d %s", rec.Code, rec.Body)
	}
	if b := cache.Model().Birthday("miguel.santos@heliosschool.org"); b == nil || b.Level != "" || b.Birthday != "1985-09-14" {
		t.Fatalf("after unskipping: %+v", b)
	}
	sasha := map[string]any{"email": "sasha.pike@heliosschool.org", "level": LevelNoNewsletter, "note": "Asked by email"}
	if rec := call(t, mux, parent, "POST", "/api/birthday/participation", sasha); rec.Code != http.StatusBadRequest {
		t.Fatalf("no-newsletter without a birthday was accepted: %d", rec.Code)
	}
	sasha["level"] = LevelSkip
	if rec := call(t, mux, parent, "POST", "/api/birthday/participation", sasha); rec.Code != http.StatusNoContent {
		t.Fatalf("skip without a birthday: %d %s", rec.Code, rec.Body)
	}
	v = view(t, cache, parent)
	if find(v.Skipped, "sasha.pike@heliosschool.org") == nil || len(v.Missing) != 0 {
		t.Fatalf("skipping someone with no birthday: skipped %v, missing %v", v.Skipped, v.Missing)
	}
	if rec := call(t, mux, parent, "DELETE", "/api/birthday/participation", sasha); rec.Code != http.StatusNoContent {
		t.Fatalf("clear skip: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Birthday("sasha.pike@heliosschool.org") != nil || len(view(t, cache, parent).Missing) != 1 {
		t.Fatal("a preference-only row survived clearing the preference")
	}
	if rec := call(t, mux, parent, "POST", "/api/birthday/note", map[string]any{"email": miguel["email"], "note": "Out until Monday"}); rec.Code != http.StatusNoContent {
		t.Fatalf("note: %d %s", rec.Code, rec.Body)
	}
	note := find(view(t, cache, parent).Staff, "miguel.santos@heliosschool.org").Notes[0]
	if rec := call(t, mux, "someone.else@heliosschool.org", "DELETE", "/api/birthday/note", note); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger removed a note: %d", rec.Code)
	}
	if rec := call(t, mux, parent, "DELETE", "/api/birthday/note", note); rec.Code != http.StatusNoContent {
		t.Fatalf("remove note: %d %s", rec.Code, rec.Body)
	}
}

func TestBirthdays(t *testing.T) {
	cache, mux := newServer(t)
	sasha := map[string]any{"email": "sasha.pike@heliosschool.org", "birthday": "1995-09-12", "override": ""}
	if rec := call(t, mux, parent, "POST", "/api/birthday/birthday", sasha); rec.Code != http.StatusNoContent {
		t.Fatalf("add birthday: %d %s", rec.Code, rec.Body)
	}
	v := view(t, cache, parent)
	if sv := find(v.Staff, "sasha.pike@heliosschool.org"); sv == nil || sv.NewsletterDate != "2026-09-11" || len(v.Missing) != 0 {
		t.Fatalf("after adding a birthday: %+v, missing %v", sv, v.Missing)
	}
	if rec := call(t, mux, parent, "POST", "/api/birthday/birthday", map[string]any{"email": "sasha.pike@heliosschool.org", "birthday": "September 12"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a bad date was accepted: %d", rec.Code)
	}
	if rec := call(t, mux, parent, "DELETE", "/api/birthday/birthday", sasha); rec.Code != http.StatusForbidden {
		t.Fatalf("a non-admin removed a birthday: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/birthday/birthday", map[string]any{"email": "dana.hawkins@heliosschool.org"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("removed a birthday with records: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/birthday/birthday", sasha); rec.Code != http.StatusNoContent {
		t.Fatalf("remove birthday: %d %s", rec.Code, rec.Body)
	}
	if len(view(t, cache, parent).Missing) != 1 {
		t.Fatal("the removed birthday did not return to missing")
	}
}

func TestCharities(t *testing.T) {
	cache, mux := newServer(t)
	added := map[string]any{"name": "Oceana", "donationLink": "https://oceana.org/", "about": "Oceans", "allowed": false, "whyNotAllowed": "ignored"}
	if rec := call(t, mux, parent, "POST", "/api/birthday/charity", added); rec.Code != http.StatusNoContent {
		t.Fatalf("add charity: %d %s", rec.Code, rec.Body)
	}
	if c := cache.Model().Charity("Oceana"); c == nil || !c.Allowed || c.WhyNotAllowed != "" || c.AddedOn != "2026-09-09" {
		t.Fatalf("a non-admin's addition: %+v", c)
	}
	prohibit := map[string]any{"original": "Oceana", "name": "Oceana", "donationLink": "https://oceana.org/", "allowed": false, "whyNotAllowed": "Politics"}
	if rec := call(t, mux, parent, "POST", "/api/birthday/charity", prohibit); rec.Code != http.StatusNoContent {
		t.Fatalf("edit charity: %d %s", rec.Code, rec.Body)
	}
	if !cache.Model().Charity("Oceana").Allowed {
		t.Fatal("a non-admin prohibited a charity")
	}
	if rec := call(t, mux, admin, "POST", "/api/birthday/charity", prohibit); rec.Code != http.StatusNoContent {
		t.Fatalf("prohibit: %d %s", rec.Code, rec.Body)
	}
	if c := cache.Model().Charity("Oceana"); c.Allowed || c.WhyNotAllowed != "Politics" {
		t.Fatalf("after prohibiting: %+v", c)
	}
	rename := map[string]any{"original": "Rocket Dog Rescue", "name": "Rocket Dog Rescue, Inc.", "donationLink": "https://www.rocketdogrescue.org/", "allowed": true}
	if rec := call(t, mux, admin, "POST", "/api/birthday/charity", rename); rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	if d, _ := cache.Model().Donation("dana.hawkins@heliosschool.org", "2026 - 2027"); d.Charity != "Rocket Dog Rescue, Inc." {
		t.Fatalf("the donation did not follow the rename: %+v", d)
	}
	if rec := call(t, mux, admin, "POST", "/api/birthday/charity", map[string]any{"original": "Birthfund", "name": "Oceana", "donationLink": "https://x.org/", "allowed": true}); rec.Code != http.StatusBadRequest {
		t.Fatalf("renamed onto an existing name: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/birthday/charity", map[string]any{"name": "Second Harvest of Silicon Valley"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("deleted the default charity: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/birthday/charity", map[string]any{"name": "Birthfund"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("deleted a charity with donations: %d", rec.Code)
	}
	if rec := call(t, mux, parent, "DELETE", "/api/birthday/charity", map[string]any{"name": "Oceana"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a non-admin deleted: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/birthday/charity", map[string]any{"name": "Oceana"}); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, admin, "POST", "/api/birthday/settings", map[string]any{"defaultCharity": "Rocket Dog Rescue, Inc.", "yearStart": "08-14", "emailSubject": "Hi", "emailBody": "Body", "noNewsletterNote": "Note", "requestLeadDays": 12}); rec.Code != http.StatusNoContent {
		t.Fatalf("settings: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, admin, "POST", "/api/birthday/settings", map[string]any{"defaultCharity": "Sierra Club", "yearStart": "08-14", "emailSubject": "Hi", "emailBody": "Body", "noNewsletterNote": "Note"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a prohibited default charity was accepted: %d", rec.Code)
	}
}

func TestTeam(t *testing.T) {
	cache, mux := newServer(t)
	if rec := call(t, mux, parent, "POST", "/api/admin/team", map[string]any{"email": "robin.whitfield@heliosschool.org", "role": RoleVolunteer}); rec.Code != http.StatusForbidden {
		t.Fatalf("a non-admin added a team member: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/admin/team", map[string]any{"email": "robin.whitfield@heliosschool.org", "role": "Boss"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("an unknown role was accepted: %d", rec.Code)
	}
	for _, role := range []string{RoleVolunteer, RoleComms, RoleVolunteer} {
		if rec := call(t, mux, admin, "POST", "/api/admin/team", map[string]any{"email": "Robin.Whitfield@heliosschool.org ", "role": role}); rec.Code != http.StatusNoContent {
			t.Fatalf("add %s: %d %s", role, rec.Code, rec.Body)
		}
	}
	roles := func(email string) []string {
		out := []string{}
		for _, m := range cache.Model().Team {
			if m.Email == email {
				out = append(out, m.Role)
			}
		}
		return out
	}
	// The sample already has Robin as a volunteer, so the repeat adds nothing and the comms role is one row.
	if got := roles("robin.whitfield@heliosschool.org"); len(got) != 2 || got[0] != RoleVolunteer || got[1] != RoleComms {
		t.Fatalf("roles after adding: %v", got)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/admin/team", map[string]any{"email": "robin.whitfield@heliosschool.org", "role": RoleVolunteer}); rec.Code != http.StatusNoContent {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body)
	}
	if got := roles("robin.whitfield@heliosschool.org"); len(got) != 1 || got[0] != RoleComms {
		t.Fatalf("roles after removing: %v", got)
	}
	if rec := call(t, mux, admin, "POST", "/api/admin/team", map[string]any{"email": "someone.new@gmail.com", "role": RoleVolunteer}); rec.Code != http.StatusNoContent {
		t.Fatalf("add by address: %d %s", rec.Code, rec.Body)
	}
	v := view(t, cache, parent)
	byEmail := map[string]TeamView{}
	for _, m := range v.Team {
		byEmail[m.Email+m.Role] = m
	}
	if m := byEmail["someone.new@gmail.comVolunteer"]; m.Name != "Someone New" {
		t.Fatalf("someone outside the directory should be named from their address: %+v", v.Team)
	}
}

func TestMovedNewsletterRefreshesInvite(t *testing.T) {
	cache, mux := newServer(t)
	if rec := call(t, mux, parent, "POST", "/api/birthday/assign", map[string]any{"email": "miguel.santos@heliosschool.org"}); rec.Code != http.StatusNoContent {
		t.Fatalf("assign: %d %s", rec.Code, rec.Body)
	}
	sent.wait(t, 1)
	// Miguel's issue moves a day, so his day to ask by does too - and Bill's
	// and Ruth's, assigned in the same issue: three updates, one each.
	if rec := call(t, mux, admin, "PUT", "/api/birthday/newsletter-date", map[string]any{"original": "2026-09-11", "date": "2026-09-12"}); rec.Code != http.StatusNoContent {
		t.Fatalf("move: %d %s", rec.Code, rec.Body)
	}
	if sv := find(view(t, cache, parent).Staff, "miguel.santos@heliosschool.org"); sv.RequestBy != "2026-09-04" {
		t.Fatalf("request by after the move: %+v", sv)
	}
	msgs := sent.wait(t, 4)
	var m *mail.Message
	for i := range msgs[1:] {
		if strings.Contains(msgs[i+1].Subject, "Miguel Santos") {
			m = &msgs[i+1]
		}
	}
	if m == nil || m.To[0] != parent || m.Subject != "Re: Ask Miguel Santos about their birthday charity" || m.Headers["In-Reply-To"] == "" || !strings.Contains(m.Text, "moved from September 3, 2026 to September 4, 2026") {
		t.Fatalf("updated invite: %+v", msgs)
	}
	if ics := string(m.Attachments[0].Content); !strings.Contains(ics, "DTSTART;VALUE=DATE:20260904") {
		t.Fatalf("updated invite's day:\n%s", ics)
	}
	// A write that moves nothing sends nothing.
	if rec := call(t, mux, parent, "POST", "/api/birthday/note", map[string]any{"email": "miguel.santos@heliosschool.org", "note": "Loves the Giants"}); rec.Code != http.StatusNoContent {
		t.Fatalf("note: %d %s", rec.Code, rec.Body)
	}
	time.Sleep(50 * time.Millisecond)
	if got := len(sent.wait(t, 4)); got != 4 {
		t.Fatalf("a note sent mail: %d messages", got)
	}
}

func TestReminders(t *testing.T) {
	cache, mux := newServer(t)
	// Bill and Ruth are assigned in the September 11 issue, so asked by the 3rd;
	// Bill was contacted on the 8th, Ruth not; Miguel is unassigned.
	app := app{cache: cache, writer: &data.Dir{Root: "sampledata"}, queue: syncQueue{}, directory: fakeDirectory{}, mailer: sent, from: "Helios Staff Birthdays <birthday@example.org>", base: "https://birthday.example.org"}
	kinds := func(day string) []string {
		out := []string{}
		for _, r := range app.dueReminders(cache.Model(), mustTime(day)) {
			out = append(out, r.sv.Name+":"+r.kind)
		}
		return out
	}
	if got := kinds("2026-09-02"); len(got) != 0 {
		t.Fatalf("the day before, due: %v", got)
	}
	// On the day, Ruth is due to be asked; Bill's outreach is recorded, so not.
	if got := kinds("2026-09-03"); len(got) != 1 || got[0] != "Ruth Amari:ask" {
		t.Fatalf("on the day, due: %v", got)
	}
	// Sending records it, so the next look finds nothing new; two days on it is late.
	if n := app.sendDueReminders(context.Background(), mustTime("2026-09-03")); n != 1 {
		t.Fatalf("sent %d", n)
	}
	ask := sent.wait(t, 1)[0]
	if ask.To[0] != "mina.park@heliosschool.org" || ask.Subject != "Re: Ask Ruth Amari about their birthday charity" || ask.Headers["In-Reply-To"] == "" {
		t.Fatalf("ask reminder: %+v", ask)
	}
	for _, want := range []string{"mailto:ruth.amari@heliosschool.org?", "cc=hca%40heliosschool.org", "Hi Ruth,", "Mina Park", "/staff/ruth.amari"} {
		if !strings.Contains(ask.Text, want) {
			t.Fatalf("ask reminder lacks %q:\n%s", want, ask.Text)
		}
	}
	if got := kinds("2026-09-03"); len(got) != 0 {
		t.Fatalf("after sending, due: %v", got)
	}
	if got := kinds("2026-09-04"); len(got) != 0 {
		t.Fatalf("the day after, due: %v", got)
	}
	if got := kinds("2026-09-05"); len(got) != 1 || got[0] != "Ruth Amari:late" {
		t.Fatalf("two days on, due: %v", got)
	}
	app.sendDueReminders(context.Background(), mustTime("2026-09-05"))
	late := sent.wait(t, 2)[1]
	if !strings.Contains(late.Text, "not marked done") || !strings.Contains(late.Text, "mailto:ruth.amari") {
		t.Fatalf("late reminder: %s", late.Text)
	}
	// Two days before the newsletter, Bill (asked by then, no donation) and
	// Ruth are nudged to record what came; Dana, complete, is not.
	got := kinds("2026-09-09")
	if len(got) != 2 || got[0] != "Bill Ryder:donation" || got[1] != "Ruth Amari:donation" {
		t.Fatalf("before the newsletter, due: %v", got)
	}
	app.sendDueReminders(context.Background(), mustTime("2026-09-09"))
	donation := sent.wait(t, 4)[2]
	if !strings.Contains(donation.Text, "no need to ask again") || !strings.Contains(donation.Text, "Second Harvest of Silicon Valley") {
		t.Fatalf("donation reminder: %s", donation.Text)
	}
	if got := kinds("2026-09-10"); len(got) != 0 {
		t.Fatalf("the next day, due again: %v", got)
	}
	if rows := cache.Tables().Reminders; len(rows) != 4 {
		t.Fatalf("reminder rows: %v", rows)
	}
	_ = mux
}

func TestCreateNewsletterDates(t *testing.T) {
	cache, mux := newServer(t)
	if rec := call(t, mux, parent, "POST", "/api/birthday/newsletter-dates/create", map[string]any{"weekday": 4, "from": "2027-08-14", "to": "2027-09-30"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a non-admin created dates: %d", rec.Code)
	}
	// Thursdays from mid-August 2027: the 19th, 26th, and September 2, 9, 16, 23, 30 - seven.
	rec := call(t, mux, admin, "POST", "/api/birthday/newsletter-dates/create", map[string]any{"weekday": 4, "from": "2027-08-14", "to": "2027-09-30"})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"added":7`) {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	dates := cache.Model().NewsletterDates
	for _, want := range []string{"2027-08-19", "2027-08-26", "2027-09-30"} {
		if !slices.Contains(dates, want) {
			t.Fatalf("missing %s in %v", want, dates)
		}
	}
	if rec := call(t, mux, admin, "POST", "/api/birthday/newsletter-dates/create", map[string]any{"weekday": 4, "from": "2027-08-14", "to": "2027-09-30"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("the same run again: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, admin, "POST", "/api/birthday/newsletter-dates/create", map[string]any{"weekday": 4, "from": "2027-09-30", "to": "2027-08-14"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a backwards run: %d", rec.Code)
	}
}

func TestResendInvites(t *testing.T) {
	_, mux := newServer(t)
	if rec := call(t, mux, parent, "POST", "/api/admin/resend-invites", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("a non-admin resent invites: %d", rec.Code)
	}
	// The sample holds six assignments this year; Dana's is complete, Grace is not in the directory, so four go.
	rec := call(t, mux, admin, "POST", "/api/admin/resend-invites", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"sent":4`) {
		t.Fatalf("resend: %d %s", rec.Code, rec.Body)
	}
	msgs := sent.wait(t, 4)
	for _, m := range msgs {
		if len(m.Attachments) != 1 || m.Headers["Message-ID"] == "" {
			t.Fatalf("resent invite: %+v", m)
		}
	}
}

func TestClearFutureNewsletterDates(t *testing.T) {
	cache, mux := newServer(t)
	if rec := call(t, mux, parent, "POST", "/api/birthday/newsletter-dates/clear-future", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("a non-admin cleared the dates: %d", rec.Code)
	}
	before := len(cache.Model().NewsletterDates)
	if rec := call(t, mux, admin, "POST", "/api/birthday/newsletter-dates/clear-future", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body)
	}
	// Today is September 9: everything from then on is gone, the past stays.
	left := cache.Model().NewsletterDates
	if len(left) >= before || len(left) == 0 || left[len(left)-1] >= "2026-09-09" {
		t.Fatalf("after clearing: %v", left)
	}
	if rec := call(t, mux, admin, "POST", "/api/birthday/newsletter-dates/clear-future", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("clearing nothing: %d %s", rec.Code, rec.Body)
	}
}

func TestJoinTeam(t *testing.T) {
	cache, mux := newServer(t)
	// Robin is already a volunteer; joining again adds nothing but still opens the app to them.
	if rec := call(t, mux, parent, "POST", "/api/birthday/team/join", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("join: %d %s", rec.Code, rec.Body)
	}
	n := 0
	for _, m := range cache.Model().Team {
		if m.Email == parent && m.Role == RoleVolunteer {
			n++
		}
	}
	if n != 1 || len(joined) != 1 || joined[0] != parent {
		t.Fatalf("after robin joined: %d rows, home %v", n, joined)
	}
	// Jordan is not on the team yet.
	if rec := call(t, mux, admin, "POST", "/api/birthday/team/join", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("join: %d %s", rec.Code, rec.Body)
	}
	found := false
	for _, m := range cache.Model().Team {
		if m.Email == admin && m.Role == RoleVolunteer {
			found = true
		}
	}
	if !found || len(joined) != 2 {
		t.Fatalf("after jordan joined: found %v, home %v", found, joined)
	}
}

func TestNewsletterDates(t *testing.T) {
	cache, mux := newServer(t)
	if rec := call(t, mux, parent, "POST", "/api/birthday/newsletter-date", map[string]any{"date": "2027-06-11"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a non-admin added a date: %d", rec.Code)
	}
	if rec := call(t, mux, parent, "PUT", "/api/birthday/newsletter-date", map[string]any{"original": "2027-06-04", "date": "2027-06-05"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a non-admin moved a date: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/birthday/newsletter-date", map[string]any{"date": "2027-06-04"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a duplicate date was accepted: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/birthday/newsletter-date", map[string]any{"date": "2027-06-11"}); rec.Code != http.StatusNoContent {
		t.Fatalf("add date: %d %s", rec.Code, rec.Body)
	}
	if tom := find(view(t, cache, parent).Staff, "tom.grady@heliosschool.org"); tom.NewsletterDate != "2027-06-11" {
		t.Fatalf("the summer newsletter did not move: %+v", tom)
	}
	if rec := call(t, mux, admin, "PUT", "/api/birthday/newsletter-date", map[string]any{"original": "2027-06-11", "date": "2027-06-04"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a date was moved onto another: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "PUT", "/api/birthday/newsletter-date", map[string]any{"original": "2027-07-01", "date": "2027-07-02"}); rec.Code != http.StatusNotFound {
		t.Fatalf("a date that is not on the list was moved: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "PUT", "/api/birthday/newsletter-date", map[string]any{"original": "2027-06-11", "date": "2027-06-18"}); rec.Code != http.StatusNoContent {
		t.Fatalf("move date: %d %s", rec.Code, rec.Body)
	}
	if tom := find(view(t, cache, parent).Staff, "tom.grady@heliosschool.org"); tom.NewsletterDate != "2027-06-18" {
		t.Fatalf("the summer newsletter did not follow the move: %+v", tom)
	}
	if rec := call(t, mux, admin, "PUT", "/api/birthday/newsletter-date", map[string]any{"original": "2026-10-23", "date": "2026-10-22"}); rec.Code != http.StatusNoContent {
		t.Fatalf("move the overridden date: %d %s", rec.Code, rec.Body)
	}
	if kate := find(view(t, cache, parent).Staff, "kate.doyle@heliosschool.org"); kate.Override != "2026-10-22" || kate.NewsletterDate != "2026-10-22" {
		t.Fatalf("the override did not move with its date: %+v", kate)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/birthday/newsletter-date", map[string]any{"date": "2027-06-18"}); rec.Code != http.StatusNoContent {
		t.Fatalf("remove date: %d %s", rec.Code, rec.Body)
	}
	if len(cache.Model().NewsletterDates) != 57 {
		t.Fatal("the date survived removal")
	}
}
