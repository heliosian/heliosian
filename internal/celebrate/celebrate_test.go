package celebrate

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/data"
)

const (
	parent  = "jordan.whitfield@heliosschool.org"
	partner = "robin.whitfield@heliosschool.org"
	kid     = "sam.whitfield@heliosschool.org"
	teen    = "ella.whitfield@heliosschool.org"
	admin   = "dana.hawkins@heliosschool.org"
	other   = "elena.torres@heliosschool.org"
	teacher = "grace.kim@heliosschool.org"
)

// The tests run on a Saturday in September 2026, between the parties that
// have happened and the ones still to come in the sample data.
func testNow() time.Time {
	t, _ := time.Parse(DateTimeFormat, "2026-09-12 12:00")
	return t
}

type syncQueue struct{}

func (syncQueue) Add(f func()) { f() }

// fakeDirectory is the sample community's Whitfield family and a few others:
// enough for the household, audience, and billing rules.
type fakeDirectory struct{}

var people = map[string]Person{
	parent:  {Email: parent, Name: "Jordan Whitfield", IsParent: true, PhotoURL: "/photos/jordan.jpg"},
	partner: {Email: partner, Name: "Robin Whitfield", IsParent: true},
	kid:     {Email: kid, Name: "Sam Whitfield", IsStudent: true, Grade: "Grade 3"},
	teen:    {Email: teen, Name: "Ella Whitfield", IsStudent: true, Grade: "Grade 6"},
	admin:   {Email: admin, Name: "Dana Hawkins", IsStaff: true, IsParent: true, JobTitle: "Art Teacher"},
	other:   {Email: other, Name: "Elena Torres", IsParent: true},
	teacher: {Email: teacher, Name: "Grace Kim", IsStaff: true, JobTitle: "Head of School"},
}

func (fakeDirectory) Resolve(email string) string { return email }

func (fakeDirectory) Person(email string) (Person, bool) {
	p, ok := people[email]
	return p, ok
}

func (fakeDirectory) Household(email string) (adults, kids []Person) {
	whitfields := map[string]bool{parent: true, partner: true, kid: true, teen: true}
	if !whitfields[email] {
		return nil, nil
	}
	for _, e := range []string{parent, partner} {
		if e != email {
			adults = append(adults, people[e])
		}
	}
	for _, e := range []string{kid, teen} {
		if e != email {
			kids = append(kids, people[e])
		}
	}
	return adults, kids
}

func (fakeDirectory) People() []Person { return nil }

func (fakeDirectory) Alerts(string) (int, bool) { return 0, false }

type bundled struct{}

func (bundled) Has(key string) (bool, error) { return strings.HasPrefix(key, "sample/"), nil }

func (bundled) Prefetch([]string) error { return nil }

func newServer(t *testing.T) (*Cache, *http.ServeMux) {
	t.Helper()
	t.Chdir("../..")
	now = testNow
	dir := &data.Dir{Root: "sampledata"}
	// Nobody is a super admin here; Dana is on the sample Admins tab.
	cache, err := NewCache(dir, bundled{}, func(string) bool { return false }, syncQueue{})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, cache, dir, syncQueue{}, nil, fakeDirectory{}, func() []string { return nil }, ImageSearch{})
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

func TestSampleLoads(t *testing.T) {
	cache, _ := newServer(t)
	m := cache.Model()
	if len(m.Celebrations) != 2 || m.Current().Code != "SC-2026" || len(m.Parties) != 16 || len(m.Categories) != 5 {
		t.Fatalf("got %d celebrations (current %v), %d parties, %d categories", len(m.Celebrations), m.Current(), len(m.Parties), len(m.Categories))
	}
	fondue := m.Party("P001")
	if fondue == nil || fondue.Sold() != 16 || fondue.Remaining() != 54 || !fondue.Hosted(parent) || fondue.Availability(testNow()) != Available {
		t.Fatalf("fondue did not load as expected: %+v", fondue)
	}
	cases := map[string]string{"P006": Waitlist, "P007": Waitlist, "P008": SoldOut, "P010": Past, "P011": Closed, "P002": Available}
	for id, want := range cases {
		if got := m.Party(id).Availability(testNow()); got != want {
			t.Errorf("%s availability %q, want %q", id, got, want)
		}
	}
	if m.Party("P006").Waiting() != 3 || m.Party("P006").Full() != true {
		t.Errorf("bagels waitlist %d full %v", m.Party("P006").Waiting(), m.Party("P006").Full())
	}
}

func TestRenderHidesWhatItShould(t *testing.T) {
	cache, _ := newServer(t)
	view := Render(cache.Model(), fakeDirectory{}, other, false, testNow())
	for _, p := range view.Parties {
		if p.Status != StatusOpen {
			t.Errorf("%s reached a parent as %s", p.Title, p.Status)
		}
		if p.CanEdit && p.ID != "P002" && p.ID != "P021" {
			t.Errorf("%s is editable by someone who does not host it", p.Title)
		}
	}
	// A host sees their own pending party; an admin sees everything.
	host := Render(cache.Model(), fakeDirectory{}, "layla.haddad@heliosschool.org", false, testNow())
	found := false
	for _, p := range host.Parties {
		if p.ID == "P013" {
			found = p.CanEdit && p.Hosting
		}
	}
	if !found {
		t.Error("a host cannot see or edit their own pending party")
	}
	if got := Render(cache.Model(), fakeDirectory{}, admin, true, testNow()); len(got.Parties) != 16 {
		t.Errorf("admin view: %d parties", len(got.Parties))
	}
	if got := Render(cache.Model(), fakeDirectory{}, parent, false, testNow()); got.User.PhotoURL != "/photos/jordan.jpg" || len(got.User.Children) != 2 {
		t.Errorf("admin view: %d parties, user %+v", len(got.Parties), got.User)
	}
}

func TestAttendeeLines(t *testing.T) {
	cache, _ := newServer(t)
	view := Render(cache.Model(), fakeDirectory{}, other, false, testNow())
	var fondue PartyView
	for _, p := range view.Parties {
		if p.ID == "P001" {
			fondue = p
		}
	}
	lines := map[string]string{}
	notes := map[string]string{}
	for _, a := range fondue.Attendees {
		lines[a.Name] = a.Line
		notes[a.Name] = a.Note
	}
	want := map[string]string{
		"Jordan Whitfield":                 "Parent to Sam Whitfield (Grade 3), Ella Whitfield (Grade 6)",
		"Sam Whitfield":                    "Grade 3",
		"Dana Hawkins":                     "Art Teacher",
		"Zander Whitfield (cousin, age 8)": "Guest of Jordan Whitfield",
	}
	for name, line := range want {
		if lines[name] != line {
			t.Errorf("%s: line %q, want %q", name, lines[name], line)
		}
	}
	// A purchaser's note is theirs and the hosts'; another parent sees none.
	if notes["Zander Whitfield (cousin, age 8)"] != "" {
		t.Errorf("a stranger saw a note: %q", notes["Zander Whitfield (cousin, age 8)"])
	}
	mine := Render(cache.Model(), fakeDirectory{}, partner, false, testNow())
	for _, p := range mine.Parties {
		if p.ID != "P001" {
			continue
		}
		for _, a := range p.Attendees {
			if a.Name == "Zander Whitfield (cousin, age 8)" && (!a.Mine || a.Note == "") {
				t.Errorf("the household did not get its own ticket back: %+v", a)
			}
		}
	}
}

func TestBuyTicketsRules(t *testing.T) {
	cache, mux := newServer(t)
	buy := func(as, party string, purchaser string, attendees ...map[string]string) *httptest.ResponseRecorder {
		return call(t, mux, as, "POST", "/api/celebrate/tickets", map[string]any{"partyId": party, "purchaser": purchaser, "attendees": attendees})
	}
	// A parent takes tickets for the household; a student is refused where
	// only adults may come.
	if rec := buy(parent, "P002", "", map[string]string{"email": kid}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "parents and staff") {
		t.Fatalf("a student got an adult ticket: %d %s", rec.Code, rec.Body)
	}
	if rec := buy(parent, "P002", "", map[string]string{"email": partner}); rec.Code != http.StatusOK {
		t.Fatalf("a parent could not buy for their partner: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Party("P002").Sold(); got != 15 {
		t.Fatalf("dink & clink sold %d", got)
	}
	// The same person cannot be sold twice.
	if rec := buy(parent, "P002", "", map[string]string{"email": partner}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a second ticket for the same person: %d %s", rec.Code, rec.Body)
	}
	// Someone outside the household is a guest by name, not by address.
	if rec := buy(parent, "P002", "", map[string]string{"email": other}); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent bought for a stranger: %d %s", rec.Code, rec.Body)
	}
	// Tickets are billed to an adult in the family, nobody else.
	if rec := buy(parent, "P002", other, map[string]string{"email": parent}); rec.Code != http.StatusForbidden {
		t.Fatalf("billed a stranger: %d %s", rec.Code, rec.Body)
	}
	// A student's tickets go to a parent.
	if rec := buy(kid, "P003", "", map[string]string{"email": kid}); rec.Code != http.StatusForbidden {
		t.Fatalf("a student billed themselves: %d %s", rec.Code, rec.Body)
	}
	if rec := buy(kid, "P003", parent, map[string]string{"email": kid}); rec.Code != http.StatusOK {
		t.Fatalf("a student could not bill a parent: %d %s", rec.Code, rec.Body)
	}
	// The last ticket goes; the next person waits.
	if rec := buy(parent, "P002", "", map[string]string{"name": "Aunt May"}); rec.Code != http.StatusOK {
		t.Fatalf("a guest could not take the last ticket: %d %s", rec.Code, rec.Body)
	}
	rec := buy(teacher, "P002", "", map[string]string{"email": teacher})
	var result map[string]int
	json.Unmarshal(rec.Body.Bytes(), &result)
	if rec.Code != http.StatusOK || result["waitlisted"] != 1 || result["sold"] != 0 {
		t.Fatalf("a full party did not waitlist: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Party("P002").Availability(testNow()); got != Waitlist {
		t.Fatalf("availability after filling: %q", got)
	}
	// Sold out with no waitlist refuses; closed refuses; a host is never
	// refused for room.
	if rec := buy(parent, "P008", "", map[string]string{"email": parent}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "sold out") {
		t.Fatalf("sold out: %d %s", rec.Code, rec.Body)
	}
	if rec := buy(other, "P011", "", map[string]string{"email": other}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "closed") {
		t.Fatalf("closed: %d %s", rec.Code, rec.Body)
	}
	if rec := buy("abena.osei@heliosschool.org", "P008", teacher, map[string]string{"email": teacher}); rec.Code != http.StatusOK {
		t.Fatalf("a host could not add to a sold-out party: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Party("P008").Sold(); got != 15 {
		t.Fatalf("fruity drinks sold %d after the host added one", got)
	}
}

func TestRemoveAndPromote(t *testing.T) {
	cache, mux := newServer(t)
	// Jordan waits on the bagels; a stranger cannot take that place away, the
	// family can, and a host can offer it.
	var waiting string
	for _, tk := range cache.Model().Party("P006").Tickets {
		if tk.Email == parent {
			waiting = tk.ID
		}
	}
	if rec := call(t, mux, other, "DELETE", "/api/celebrate/ticket", map[string]string{"ticketId": waiting}); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger removed a ticket: %d", rec.Code)
	}
	if rec := call(t, mux, other, "POST", "/api/celebrate/ticket", map[string]string{"ticketId": waiting, "status": TicketSold}); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger promoted a ticket: %d", rec.Code)
	}
	if rec := call(t, mux, "freja.lindqvist@heliosschool.org", "POST", "/api/celebrate/ticket", map[string]string{"ticketId": waiting, "status": TicketSold}); rec.Code != http.StatusNoContent {
		t.Fatalf("a host could not promote: %d %s", rec.Code, rec.Body)
	}
	if tk, _ := cache.Model().TicketByID(waiting); tk.Status != TicketSold {
		t.Fatalf("ticket after promotion: %+v", tk)
	}
	// Once sold, a ticket is the fundraiser's: the family cannot give it back,
	// though it may still leave a waitlist.
	if rec := call(t, mux, partner, "DELETE", "/api/celebrate/ticket", map[string]string{"ticketId": waiting}); rec.Code != http.StatusForbidden {
		t.Fatalf("the family gave a sold ticket back: %d %s", rec.Code, rec.Body)
	}
	var stillWaiting string
	for _, tk := range cache.Model().Party("P007").Tickets {
		if tk.Email == partner {
			stillWaiting = tk.ID
		}
	}
	if rec := call(t, mux, parent, "DELETE", "/api/celebrate/ticket", map[string]string{"ticketId": stillWaiting}); rec.Code != http.StatusNoContent {
		t.Fatalf("the family could not leave a waitlist: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, "freja.lindqvist@heliosschool.org", "DELETE", "/api/celebrate/ticket", map[string]string{"ticketId": waiting}); rec.Code != http.StatusNoContent {
		t.Fatalf("a host could not remove a ticket: %d %s", rec.Code, rec.Body)
	}
	if tk, _ := cache.Model().TicketByID(waiting); tk != nil {
		t.Fatal("the ticket is still there")
	}
	// Only an admin records invoicing.
	var sold string
	for _, tk := range cache.Model().Party("P010").Tickets {
		if tk.Email == parent {
			sold = tk.ID
		}
	}
	if rec := call(t, mux, "colin.quinn@heliosschool.org", "POST", "/api/celebrate/ticket", map[string]string{"ticketId": sold, "invoice": InvoicePaid}); rec.Code != http.StatusForbidden {
		t.Fatalf("a host recorded an invoice: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/celebrate/ticket", map[string]string{"ticketId": sold, "invoice": InvoicePaid}); rec.Code != http.StatusNoContent {
		t.Fatalf("an admin could not record an invoice: %d %s", rec.Code, rec.Body)
	}
}

func TestPostAndEditParty(t *testing.T) {
	cache, mux := newServer(t)
	body := map[string]any{
		"title": "Board Game Night", "summary": "Games and snacks", "price": 40, "capacity": 12, "start": "2026-11-21 18:00", "end": "2026-11-21 21:00",
		"parents": true, "students": true, "staff": false, "ticketsOpen": true, "waitlist": true, "category": "Family Social", "hosts": "The Torres Family",
	}
	rec := call(t, mux, other, "POST", "/api/celebrate/party", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("post a party: %d %s", rec.Code, rec.Body)
	}
	var made map[string]string
	json.Unmarshal(rec.Body.Bytes(), &made)
	p := cache.Model().Party(made["id"])
	if p == nil || p.Status != StatusPending || !p.Hosted(other) || p.Celebration != "SC-2026" || p.Price != 40 || p.AddedBy != other {
		t.Fatalf("posted party: %+v", p)
	}
	// The host edits it but cannot approve it; a stranger cannot touch it.
	body["id"] = p.ID
	body["status"] = StatusOpen
	body["title"] = "Board Game Night!"
	body["hostEmails"] = []string{other, "marco.torres@heliosschool.org"}
	if rec := call(t, mux, other, "POST", "/api/celebrate/party", body); rec.Code != http.StatusOK {
		t.Fatalf("a host could not edit: %d %s", rec.Code, rec.Body)
	}
	p = cache.Model().Party(p.ID)
	if p.Status != StatusPending || p.Title != "Board Game Night!" || len(p.HostEmails) != 2 {
		t.Fatalf("after the host's edit: %+v", p)
	}
	if rec := call(t, mux, "abena.osei@heliosschool.org", "POST", "/api/celebrate/party", body); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger edited: %d", rec.Code)
	}
	if rec := call(t, mux, other, "POST", "/api/celebrate/party/status", map[string]string{"id": p.ID, "status": StatusOpen}); rec.Code != http.StatusForbidden {
		t.Fatalf("a host approved their own party: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/celebrate/party/status", map[string]string{"id": p.ID, "status": StatusOpen}); rec.Code != http.StatusNoContent {
		t.Fatalf("an admin could not approve: %d %s", rec.Code, rec.Body)
	}
	// The switches: a host closes sales and the party stops selling.
	flags := map[string]any{"id": p.ID, "ticketsOpen": false, "waitlist": true, "parents": true, "students": true, "staff": false}
	if rec := call(t, mux, other, "POST", "/api/celebrate/party/flags", flags); rec.Code != http.StatusNoContent {
		t.Fatalf("flags: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Party(p.ID).Availability(testNow()); got != Closed {
		t.Fatalf("after closing sales: %q", got)
	}
	// Nobody may be let in at all: refused.
	flags["parents"], flags["students"] = false, false
	if rec := call(t, mux, other, "POST", "/api/celebrate/party/flags", flags); rec.Code != http.StatusBadRequest {
		t.Fatalf("a party for nobody: %d", rec.Code)
	}
	// Deleting needs an admin and an empty party.
	if rec := call(t, mux, admin, "DELETE", "/api/celebrate/party", map[string]string{"id": "P001"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("deleted a party with tickets: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/celebrate/party", map[string]string{"id": p.ID}); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Party(p.ID) != nil {
		t.Fatal("the party is still there")
	}
}

func TestCategoriesAndCelebrations(t *testing.T) {
	cache, mux := newServer(t)
	if rec := call(t, mux, admin, "POST", "/api/celebrate/category", map[string]string{"original": "Adult Social", "title": "Grown-Ups"}); rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Party("P002").Category != "Grown-Ups" {
		t.Fatal("a rename did not carry the parties along")
	}
	if rec := call(t, mux, admin, "DELETE", "/api/celebrate/category", map[string]string{"title": "Grown-Ups"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("deleted a category in use: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/celebrate/celebration", map[string]any{"code": "SC-2027", "title": "Helios Spring Celebration 2027", "start": "2027-03-06 17:30", "current": true}); rec.Code != http.StatusNoContent {
		t.Fatalf("add celebration: %d %s", rec.Code, rec.Body)
	}
	m := cache.Model()
	if m.Current().Code != "SC-2027" || m.Celebration("SC-2026").Current {
		t.Fatalf("current after adding: %s", m.Current().Code)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/celebrate/celebration", map[string]string{"code": "SC-2026"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("deleted a celebration with parties: %d", rec.Code)
	}
	rec := call(t, mux, admin, "GET", "/api/celebrate/invoices.csv?celebration=SC-2025", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Wines of the Southern Hemisphere!") || strings.Contains(rec.Body.String(), "Fondue") {
		t.Fatalf("invoices: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, other, "GET", "/api/celebrate/invoices.csv", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent read the invoices: %d", rec.Code)
	}
}

func TestPastAndPrices(t *testing.T) {
	p := &Party{Start: "2026-09-11", Status: StatusOpen, TicketsOpen: true}
	if !p.Past(testNow()) {
		t.Error("yesterday's all-day party is not past")
	}
	p.Start = "2026-09-12"
	if p.Past(testNow()) {
		t.Error("today's all-day party is already past")
	}
	p.Start, p.End = "2026-09-12 09:00", "2026-09-12 11:00"
	if !p.Past(testNow()) {
		t.Error("this morning's party is not past")
	}
	for cell, want := range map[string]float64{"65": 65, "65.0": 65, "$12.50": 12.5, "": 0} {
		if got, err := ParsePrice(cell); err != nil || got != want {
			t.Errorf("ParsePrice(%q) = %v, %v", cell, got, err)
		}
	}
	if _, err := ParsePrice("free"); err == nil {
		t.Error("a word parsed as a price")
	}
	if PriceCell(65) != "65" || PriceCell(12.5) != "12.50" {
		t.Errorf("PriceCell: %s %s", PriceCell(65), PriceCell(12.5))
	}
}

func TestFriendlyAddresses(t *testing.T) {
	cache, mux := newServer(t)
	m := cache.Model()
	if m.PathOf(m.Party("P001")) != "/p/fondue" || m.PathOf(m.Party("P005")) != "/parties/P005" {
		t.Fatalf("paths: %s %s", m.PathOf(m.Party("P001")), m.PathOf(m.Party("P005")))
	}
	// A live address, an old one through the Redirects tab, a bare word.
	for path, want := range map[string]string{"/p/fondue": "P001", "/p/Fondue/": "P001", "/parties/P001": "P001", "/p/fondue-night": "P001", "pickleball": "P002", "/p/nothing": ""} {
		got := ""
		if p := m.Resolve(path); p != nil {
			got = p.ID
		}
		if got != want {
			t.Errorf("Resolve(%q) = %q, want %q", path, got, want)
		}
	}
	// Renaming an address leaves a redirect behind; a taken one is refused.
	body := map[string]any{"id": "P003", "title": "K-Pop for a Cause!", "price": 50, "capacity": 20, "parents": true, "students": true, "ticketsOpen": true, "waitlist": true, "prettyId": "Fondue", "hostEmails": []string{"deepa.natarajan@heliosschool.org"}}
	if rec := call(t, mux, "deepa.natarajan@heliosschool.org", "POST", "/api/celebrate/party", body); rec.Code != http.StatusBadRequest {
		t.Fatalf("took another party's address: %d %s", rec.Code, rec.Body)
	}
	body["prettyId"] = "k-pop"
	if rec := call(t, mux, "deepa.natarajan@heliosschool.org", "POST", "/api/celebrate/party", body); rec.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model()
	if got := m.Resolve("/p/kpop"); got == nil || got.ID != "P003" {
		t.Fatalf("the old address no longer resolves: %v", got)
	}
	if m.PathOf(m.Party("P003")) != "/p/k-pop" {
		t.Fatalf("path after rename: %s", m.PathOf(m.Party("P003")))
	}
}

func TestSharePreview(t *testing.T) {
	cache, mux := newServer(t)
	head := PreviewHead(cache)
	req := httptest.NewRequest("GET", "/p/fondue", nil)
	req.Host = "celebrate.heliosian.com"
	got := head(req)
	for _, want := range []string{`og:title" content="Fondue &amp; Fort Night"`, `og:url" content="https://celebrate.heliosian.com/p/fondue"`,
		`og:image" content="https://celebrate.heliosian.com/share/P001.png"`, `Saturday, September 19 · 5:00 – 9:00 PM — The Parks&#39; House in Los Altos — A cozy evening`} {
		if !strings.Contains(got, want) {
			t.Errorf("preview head lacks %s:\n%s", want, got)
		}
	}
	// The street stays out of the tags; a pending party and any other page
	// get none.
	if strings.Contains(got, "Alder") {
		t.Error("the street address leaked into the preview")
	}
	for _, path := range []string{"/parties/P013", "/my", "/p/nothing"} {
		req := httptest.NewRequest("GET", path, nil)
		if head(req) != "" {
			t.Errorf("%s previewed", path)
		}
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/share/P001.png", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || rec.Body.Len() < 10000 {
		t.Fatalf("share card: %d %s %d bytes", rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/share/P013.png", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("a pending party's card: %d", rec.Code)
	}
}
