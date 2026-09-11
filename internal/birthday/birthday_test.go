package birthday

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/data"
)

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
	Register(mux, cache, dir, syncQueue{}, fakeDirectory{}, func() []string { return []string{admin} })
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
	dates := []string{"2026-08-21", "2026-09-04", "2027-06-04", "2027-09-03"}
	if d, ok := y.Newsletter(mustTime("2026-08-20"), dates); !ok || d != mustTime("2026-08-21") {
		t.Errorf("first newsletter after: %s %v", d, ok)
	}
	if d, ok := y.Newsletter(mustTime("2027-06-20"), dates); !ok || d != mustTime("2027-06-04") {
		t.Errorf("last newsletter before a summer birthday: %s %v", d, ok)
	}
	if _, ok := y.Newsletter(mustTime("2026-09-01"), []string{"2025-09-05"}); ok {
		t.Error("a date outside the year was picked")
	}
	if got := RequestBy(mustTime("2026-08-21")); got != mustTime("2026-08-11") {
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
	if dana.BirthdayThisYear != "2026-08-20" || dana.NewsletterDate != "2026-08-21" || dana.RequestBy != "2026-08-11" || dana.AssignedToName != "Jordan Whitfield" {
		t.Errorf("dana: %+v", dana)
	}
	if dana.LastDonation == nil || dana.LastDonation.Charity != "Birthfund" || dana.Donation == nil || dana.Donation.UsedOn == "" {
		t.Errorf("dana's donations: %+v %+v", dana.Donation, dana.LastDonation)
	}
	if miguel := find(v.Staff, "miguel.santos@heliosschool.org"); miguel.AssignedTo != "" || miguel.Department != "Classroom Teachers" {
		t.Errorf("miguel: %+v", miguel)
	}
	if kate := find(v.Staff, "kate.doyle@heliosschool.org"); kate.NewsletterDate != "2026-10-23" || kate.RequestBy != "2026-10-13" {
		t.Errorf("override: %+v", kate)
	}
	if hana := find(v.Staff, "hana.ito@heliosschool.org"); hana.BirthdayThisYear != "2027-02-28" || hana.NewsletterDate != "2027-03-05" {
		t.Errorf("leap day: %+v", hana)
	}
	if tom := find(v.Staff, "tom.grady@heliosschool.org"); tom.NewsletterDate != "2027-06-04" || tom.RequestBy != "2027-05-25" {
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
	if sv := find(v.Staff, "sasha.pike@heliosschool.org"); sv == nil || sv.NewsletterDate != "2026-09-18" || len(v.Missing) != 0 {
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
	if rec := call(t, mux, admin, "POST", "/api/birthday/settings", map[string]any{"defaultCharity": "Rocket Dog Rescue, Inc.", "yearStart": "08-14", "emailSubject": "Hi", "emailBody": "Body", "noNewsletterNote": "Note"}); rec.Code != http.StatusNoContent {
		t.Fatalf("settings: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, admin, "POST", "/api/birthday/settings", map[string]any{"defaultCharity": "Sierra Club", "yearStart": "08-14", "emailSubject": "Hi", "emailBody": "Body", "noNewsletterNote": "Note"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a prohibited default charity was accepted: %d", rec.Code)
	}
}

func TestNewsletterDates(t *testing.T) {
	cache, mux := newServer(t)
	if rec := call(t, mux, parent, "POST", "/api/birthday/newsletter-date", map[string]any{"date": "2027-06-11"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a non-admin added a date: %d", rec.Code)
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
	if rec := call(t, mux, admin, "DELETE", "/api/birthday/newsletter-date", map[string]any{"date": "2027-06-11"}); rec.Code != http.StatusNoContent {
		t.Fatalf("remove date: %d %s", rec.Code, rec.Body)
	}
	if len(cache.Model().NewsletterDates) != 57 {
		t.Fatal("the date survived removal")
	}
}
