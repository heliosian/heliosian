package celebrate

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	parent  = "jordan.whitfield@heliosschool.org"
	partner = "robin.whitfield@heliosschool.org"
	kid     = "sam.whitfield@heliosschool.org"
	teen    = "ella.whitfield@heliosschool.org"
	admin   = "dana.hawkins@heliosschool.org"
	other   = "elena.torres@heliosschool.org"
	teacher = "grace.kim@heliosschool.org"

	testFrom = "Helios Celebrate <celebrate@example.org>"
)

func testNow() time.Time {
	t, _ := time.Parse(DateTimeFormat, "2026-09-12 12:00")
	return t
}

type fakeDirectory struct{}

var people = map[string]Person{
	parent:  {Email: parent, Name: "Jordan Whitfield", IsParent: true, PhotoURL: "/photos/jordan.jpg"},
	partner: {Email: partner, Name: "Robin Whitfield", IsParent: true},
	kid:     {Email: kid, Name: "Sam Whitfield", IsStudent: true, Grade: "Grade 3", ParentEmails: []string{parent, partner}},
	teen:    {Email: teen, Name: "Ella Whitfield", IsStudent: true, Grade: "Grade 6", ParentEmails: []string{parent, partner}},
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

func (fakeDirectory) Alerts(string) ([]string, []string) { return nil, nil }

type bundled struct{}

func (bundled) Has(key string) (bool, error) { return strings.HasPrefix(key, "sample/"), nil }

func (bundled) Prefetch(context.Context, []string) error { return nil }

var (
	sheet *data.Dir
	queue *store.Queue
)

func serveWith(t *testing.T, mailer mail.Sender) (*Cache, *http.ServeMux) {
	t.Helper()
	t.Chdir("../..")
	now = testNow
	sheet = &data.Dir{Root: "sampledata"}
	queue = store.NewQueue()
	cache, err := NewCache(sheet, sheet, bundled{}, func(string) bool { return false }, queue)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, cache, nil, fakeDirectory{}, func() []string { return nil }, ImageSearch{}, mailer, testFrom, nil, nil)
	return cache, mux
}

func newServer(t *testing.T) (*Cache, *http.ServeMux) {
	t.Helper()
	return serveWith(t, nil)
}

func tables(t *testing.T) store.Tables {
	t.Helper()
	names := []string{celebrationsTab, categoriesTab, partiesTab, hostsTab, ticketsTab, settingsTab, adminsTab, redirectsTab, invoicingTab}
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
	if m.Party("P006").Waiting() != 4 || m.Party("P006").Full() != true {
		t.Errorf("bagels waitlist %d full %v", m.Party("P006").Waiting(), m.Party("P006").Full())
	}
	if !cache.IsAdmin(admin) || cache.IsAdmin(other) {
		t.Errorf("admins: %v", cache.Admins(nil))
	}
}

func TestRowOrderIsNotTabOrder(t *testing.T) {
	newServer(t)
	rows := tables(t)
	slices.Reverse(rows[celebrationsTab])
	slices.Reverse(rows[categoriesTab])
	slices.Reverse(rows[ticketsTab])
	m, err := BuildModel(context.Background(), rows, bundled{})
	if err != nil {
		t.Fatal(err)
	}
	if m.Celebrations[0].Code != "SC-2026" || m.Celebrations[1].Code != "SC-2025" {
		t.Errorf("celebrations: %s, %s", m.Celebrations[0].Code, m.Celebrations[1].Code)
	}
	if !slices.Equal(m.Categories, []string{"Family Social", "Adult Social", "Children Social", "Athletic", "Educational"}) {
		t.Errorf("categories: %v", m.Categories)
	}
	tickets := m.Party("P001").Tickets
	for i := 1; i < len(tickets); i++ {
		if tickets[i].Added < tickets[i-1].Added {
			t.Fatalf("tickets out of order: %s before %s", tickets[i-1].Added, tickets[i].Added)
		}
	}
	for _, tk := range tickets {
		if got, _ := m.TicketByID(tk.ID); got == nil || got.ID != tk.ID {
			t.Fatalf("ticket %s indexes %+v", tk.ID, got)
		}
	}
}

func TestBrokenSheetRefusesToLoad(t *testing.T) {
	t.Chdir("../..")
	broken := t.TempDir()
	if err := os.MkdirAll(filepath.Join(broken, appName), 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join("sampledata", appName))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join("sampledata", appName, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(broken, appName, e.Name()), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(broken, appName, "Categories.csv"), []byte("Title,Order\nFamily Social,10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := &data.Dir{Root: broken}
	if _, err := NewCache(dir, dir, bundled{}, func(string) bool { return false }, store.NewQueue()); err == nil || !strings.Contains(err.Error(), "ends in 0") {
		t.Fatalf("a broken sheet loaded: %v", err)
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
	if rec := buy(kid, "P001", "", map[string]string{"email": kid}); rec.Code != http.StatusForbidden {
		t.Fatalf("a student bought: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, teen, "POST", "/api/celebrate/waitlist", map[string]any{"partyId": "P006", "quantity": 1}); rec.Code != http.StatusForbidden {
		t.Fatalf("a student joined the waitlist: %d %s", rec.Code, rec.Body)
	}
	var own string
	for _, tk := range cache.Model().Party("P001").Tickets {
		if tk.Email == teen {
			own = tk.ID
		}
	}
	if rec := call(t, mux, teen, "POST", "/api/celebrate/ticket/reassign", map[string]any{"ticketId": own, "email": kid}); rec.Code != http.StatusForbidden {
		t.Fatalf("a student reassigned: %d %s", rec.Code, rec.Body)
	}
	if rec := buy(parent, "P002", "", map[string]string{"email": kid}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "for adults") {
		t.Fatalf("a student got an adult ticket: %d %s", rec.Code, rec.Body)
	}
	if rec := buy(parent, "P002", "", map[string]string{"email": partner}); rec.Code != http.StatusOK {
		t.Fatalf("a parent could not buy for their partner: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Party("P002").Sold(); got != 15 {
		t.Fatalf("dink & clink sold %d", got)
	}
	if rec := buy(parent, "P002", "", map[string]string{"email": partner}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a second ticket for the same person: %d %s", rec.Code, rec.Body)
	}
	if rec := buy(parent, "P002", "", map[string]string{"email": other}); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent bought for a stranger: %d %s", rec.Code, rec.Body)
	}
	if rec := buy(parent, "P002", other, map[string]string{"email": parent}); rec.Code != http.StatusForbidden {
		t.Fatalf("billed a stranger: %d %s", rec.Code, rec.Body)
	}
	if rec := buy(partner, "P003", parent, map[string]string{"email": kid}); rec.Code != http.StatusOK {
		t.Fatalf("a parent could not take a child's ticket: %d %s", rec.Code, rec.Body)
	}
	if rec := buy(parent, "P002", "", map[string]string{"name": "Aunt May"}); rec.Code != http.StatusOK {
		t.Fatalf("a guest could not take the last ticket: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Party("P002").Availability(testNow()); got != Waitlist {
		t.Fatalf("availability after filling: %q", got)
	}
	if rec := buy(teacher, "P002", "", map[string]string{"email": teacher}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a full party sold a ticket: %d %s", rec.Code, rec.Body)
	}
	join := func(as, party, purchaser string, quantity int) *httptest.ResponseRecorder {
		return call(t, mux, as, "POST", "/api/celebrate/waitlist", map[string]any{"partyId": party, "purchaser": purchaser, "quantity": quantity, "note": "any two"})
	}
	if rec := join(teacher, "P002", other, 2); rec.Code != http.StatusForbidden {
		t.Fatalf("billed a stranger from the waitlist: %d", rec.Code)
	}
	if rec := join(teacher, "P002", "", 2); rec.Code != http.StatusOK {
		t.Fatalf("join the waitlist: %d %s", rec.Code, rec.Body)
	}
	if rec := join(teacher, "P002", "", 3); rec.Code != http.StatusOK {
		t.Fatalf("change the request: %d %s", rec.Code, rec.Body)
	}
	requests := 0
	for _, tk := range cache.Model().Party("P002").Tickets {
		if tk.Status == TicketWaitlist {
			requests++
			if tk.Purchaser != teacher || tk.Quantity != 3 || tk.Note != "any two" {
				t.Fatalf("request: %+v", tk)
			}
		}
	}
	if requests != 1 || cache.Model().Party("P002").Waiting() != 3 {
		t.Fatalf("%d requests, waiting %d", requests, cache.Model().Party("P002").Waiting())
	}
	if rec := buy(parent, "P008", "", map[string]string{"email": parent}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "sold out") {
		t.Fatalf("sold out: %d %s", rec.Code, rec.Body)
	}
	if rec := buy(other, "P011", "", map[string]string{"email": other}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "closed") {
		t.Fatalf("closed: %d %s", rec.Code, rec.Body)
	}
	if rec := buy("abena.osei@heliosschool.org", "P008", teacher, map[string]string{"email": teacher}); rec.Code != http.StatusForbidden {
		t.Fatalf("a host billed a stranger: %d %s", rec.Code, rec.Body)
	}
	if rec := buy("abena.osei@heliosschool.org", "P008", "", map[string]string{"email": teacher}); rec.Code != http.StatusOK {
		t.Fatalf("a host could not add to a sold-out party: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Party("P008").Sold(); got != 15 {
		t.Fatalf("fruity drinks sold %d after the host added one", got)
	}
	for _, tk := range cache.Model().Party("P008").Tickets {
		if tk.Email == teacher && tk.Purchaser != teacher {
			t.Fatalf("the teacher's ticket is billed to %s", tk.Purchaser)
		}
	}
	if rec := buy("mina.park@heliosschool.org", "P012", "", map[string]string{"email": teen}); rec.Code != http.StatusOK {
		t.Fatalf("a host could not add a student: %d %s", rec.Code, rec.Body)
	}
	for _, tk := range cache.Model().Party("P012").Tickets {
		if tk.Email == teen && tk.Purchaser != parent {
			t.Fatalf("the student's ticket is billed to %s, not a parent", tk.Purchaser)
		}
	}
}

func TestFreeTicket(t *testing.T) {
	cache, mux := newServer(t)
	body := map[string]any{"partyId": "P002", "free": true, "attendees": []map[string]string{{"name": "Percy Jackson"}}}
	if rec := call(t, mux, teacher, "POST", "/api/celebrate/tickets", body); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger gave a free ticket: %d %s", rec.Code, rec.Body)
	}
	raised := cache.Model().Party("P002").Raised()
	if rec := call(t, mux, other, "POST", "/api/celebrate/tickets", body); rec.Code != http.StatusOK {
		t.Fatalf("the host could not give a free ticket: %d %s", rec.Code, rec.Body)
	}
	p := cache.Model().Party("P002")
	var free *Ticket
	for i := range p.Tickets {
		if p.Tickets[i].Name == "Percy Jackson" {
			free = &p.Tickets[i]
		}
	}
	if free == nil || free.Price != 0 || free.Status != TicketSold {
		t.Fatalf("free ticket: %+v", free)
	}
	if p.Raised() != raised {
		t.Fatalf("a free ticket raised money: %v then %v", raised, p.Raised())
	}
	was := p.Capacity
	body["attendees"] = []map[string]string{{"name": "Annabeth Chase"}}
	body["raiseCapacity"] = true
	if rec := call(t, mux, other, "POST", "/api/celebrate/tickets", body); rec.Code != http.StatusOK {
		t.Fatalf("free ticket with room: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Party("P002").Capacity; got != was+1 {
		t.Fatalf("capacity %d, want %d", got, was+1)
	}
	body = map[string]any{"partyId": "P002", "free": true, "purchaser": teen, "attendees": []map[string]string{{"name": "Grover Underwood"}}}
	if rec := call(t, mux, other, "POST", "/api/celebrate/tickets", body); rec.Code != http.StatusBadRequest {
		t.Fatalf("a student hosts a guest: %d %s", rec.Code, rec.Body)
	}
	body["purchaser"] = parent
	if rec := call(t, mux, other, "POST", "/api/celebrate/tickets", body); rec.Code != http.StatusOK {
		t.Fatalf("free ticket as someone's guest: %d %s", rec.Code, rec.Body)
	}
	for _, tk := range cache.Model().Party("P002").Tickets {
		if tk.Name == "Grover Underwood" && (tk.Purchaser != parent || tk.Price != 0) {
			t.Fatalf("the guest's ticket: %+v", tk)
		}
	}
	ledger := tables(t)[invoicingTab]
	before := len(ledger)
	for _, l := range ledger {
		if l["Guest Name"] == "Percy Jackson" || l["Guest Name"] == "Annabeth Chase" || l["Guest Name"] == "Grover Underwood" {
			t.Fatalf("a free ticket in the ledger: %v", l)
		}
	}
	if rec := call(t, mux, other, "POST", "/api/celebrate/tickets", map[string]any{"partyId": "P002", "attendees": []map[string]string{{"name": "Grover Underwood"}}}); rec.Code != http.StatusOK {
		t.Fatalf("paid ticket: %d %s", rec.Code, rec.Body)
	}
	ledger = tables(t)[invoicingTab]
	last := ledger[len(ledger)-1]
	if len(ledger) != before+1 || last["Action"] != "ADD" || last["Quantity"] != "1" || last["Cost"] != "75" || last["Purchaser Email"] != other || last["Guest Name"] != "Grover Underwood" || last["Event Code"] != "SC-2026" || last["Invoice"] != "" {
		t.Fatalf("ledger: %v", last)
	}
	if n := len(cache.Model().Invoicing); n != before+1 {
		t.Fatalf("model ledger %d, want %d", n, before+1)
	}
	if v := Render(cache.Model(), fakeDirectory{}, other, false, testNow()); len(v.Invoicing) != 0 {
		t.Fatalf("a parent was shown the ledger")
	}
	if v := Render(cache.Model(), fakeDirectory{}, admin, true, testNow()); len(v.Invoicing) != before+1 {
		t.Fatalf("the admin was not shown the ledger")
	}
	logged := 0
	for _, row := range changeLog(t) {
		if row["Tab"] == invoicingTab && row["Action"] == "insert" && row["Column"] == "" && row["Previous"] == "" && strings.Contains(row["Key"], "Guest Name=Grover Underwood") {
			logged++
		}
	}
	if logged != 1 {
		t.Fatalf("the ledger row was logged %d times: %v", logged, changeLog(t))
	}
}

func TestHostingClosed(t *testing.T) {
	cache, mux := newServer(t)
	if rec := call(t, mux, admin, "POST", "/api/celebrate/settings", map[string]any{"partiesIntro": "x", "ticketNote": "y", "hostingOpen": false}); rec.Code != http.StatusNoContent {
		t.Fatalf("settings: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Settings.HostingOpen {
		t.Fatal("hosting still open")
	}
	body := map[string]any{"title": "Late Party", "price": 10, "adults": true, "ticketsOpen": true, "waitlist": true, "start": "2026-11-21 18:00"}
	if rec := call(t, mux, other, "POST", "/api/celebrate/party", body); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent posted while closed: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, admin, "POST", "/api/celebrate/party", body); rec.Code != http.StatusOK {
		t.Fatalf("an admin could not post while closed: %d %s", rec.Code, rec.Body)
	}
	edit := map[string]any{"id": "P002", "title": "Dink & Clink!", "price": 75, "adults": true, "ticketsOpen": true, "waitlist": true, "start": "2026-09-26 18:00", "hostEmails": []string{other, "marco.torres@heliosschool.org"}}
	if rec := call(t, mux, other, "POST", "/api/celebrate/party", edit); rec.Code != http.StatusOK {
		t.Fatalf("a host could not edit while closed: %d %s", rec.Code, rec.Body)
	}
}

func TestRemoveAndPromote(t *testing.T) {
	cache, mux := newServer(t)
	var waiting string
	for _, tk := range cache.Model().Party("P006").Tickets {
		if tk.Email == parent {
			waiting = tk.ID
		}
	}
	if rec := call(t, mux, other, "DELETE", "/api/celebrate/ticket", map[string]string{"ticketId": waiting}); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger removed a ticket: %d", rec.Code)
	}
	if rec := call(t, mux, other, "POST", "/api/celebrate/waitlist/offer", map[string]any{"ticketId": waiting}); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger offered tickets: %d", rec.Code)
	}
	if rec := call(t, mux, "freja.lindqvist@heliosschool.org", "POST", "/api/celebrate/waitlist/offer", map[string]any{"ticketId": waiting, "quantity": 1}); rec.Code != http.StatusNoContent {
		t.Fatalf("a host could not offer: %d %s", rec.Code, rec.Body)
	}
	if tk, _ := cache.Model().TicketByID(waiting); tk == nil || tk.Quantity != 1 {
		t.Fatalf("request after one offer: %+v", tk)
	}
	if rec := call(t, mux, "freja.lindqvist@heliosschool.org", "POST", "/api/celebrate/waitlist/offer", map[string]any{"ticketId": waiting}); rec.Code != http.StatusNoContent {
		t.Fatalf("a host could not offer the rest: %d %s", rec.Code, rec.Body)
	}
	if tk, _ := cache.Model().TicketByID(waiting); tk != nil {
		t.Fatal("the request is still there")
	}
	deleted := false
	for _, row := range changeLog(t) {
		if row["Tab"] == ticketsTab && row["Action"] == "delete" && row["Key"] == "Ticket ID="+waiting && row["Column"] == "Quantity" && row["Previous"] == "1" {
			deleted = true
		}
	}
	if !deleted {
		t.Fatalf("the request's delete was not logged with its last quantity: %v", changeLog(t))
	}
	held, guests := 0, 0
	for _, tk := range cache.Model().Party("P006").Tickets {
		if tk.Status == TicketSold && tk.Purchaser == parent {
			if tk.Email == parent {
				held++
				waiting = tk.ID
			} else if strings.Contains(tk.Name, "to be named") {
				guests++
			}
		}
	}
	if held != 1 || guests != 1 || cache.Model().Party("P006").Sold() != 10 {
		t.Fatalf("after the offers: held %d, guests %d, sold %d", held, guests, cache.Model().Party("P006").Sold())
	}
	if rec := call(t, mux, partner, "DELETE", "/api/celebrate/ticket", map[string]string{"ticketId": waiting}); rec.Code != http.StatusForbidden {
		t.Fatalf("the family gave a sold ticket back: %d %s", rec.Code, rec.Body)
	}
	var stillWaiting string
	for _, tk := range cache.Model().Party("P007").Tickets {
		if tk.Status == TicketWaitlist && tk.Purchaser == parent {
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
}

func TestPostAndEditParty(t *testing.T) {
	cache, mux := newServer(t)
	body := map[string]any{
		"title": "Board Game Night", "summary": "Games and snacks", "price": 40, "capacity": 12, "start": "2026-11-21 18:00", "end": "2026-11-21 21:00",
		"adults": true, "students": true, "ticketsOpen": true, "waitlist": true, "category": "Family Social", "hosts": "The Torres Family",
	}
	rec := call(t, mux, other, "POST", "/api/celebrate/party", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("post a party: %d %s", rec.Code, rec.Body)
	}
	var made map[string]string
	json.Unmarshal(rec.Body.Bytes(), &made)
	p := cache.Model().Party(made["id"])
	if p == nil || p.Status != StatusPending || !p.Hosted(other) || p.Celebration != "SC-2026" || p.Price != 40 || p.AddedBy != other || p.Category != "" {
		t.Fatalf("posted party: %+v", p)
	}
	body["id"] = p.ID
	body["status"] = StatusOpen
	body["title"] = "Board Game Night!"
	body["hostEmails"] = []string{other, "marco.torres@heliosschool.org"}
	body["needToKnow"], body["noteEmoji"], body["noteTitle"] = "Bring a game", "🎲", "House rules"
	if rec := call(t, mux, other, "POST", "/api/celebrate/party", body); rec.Code != http.StatusOK {
		t.Fatalf("a host could not edit: %d %s", rec.Code, rec.Body)
	}
	p = cache.Model().Party(p.ID)
	if p.Status != StatusPending || p.Title != "Board Game Night!" || len(p.HostEmails) != 2 || p.NoteEmoji != "🎲" || p.NoteTitle != "House rules" {
		t.Fatalf("after the host's edit: %+v", p)
	}
	body["noteTitle"] = strings.Repeat("x", 61)
	if rec := call(t, mux, other, "POST", "/api/celebrate/party", body); rec.Code != http.StatusBadRequest {
		t.Fatalf("a long note title was taken: %d", rec.Code)
	}
	body["noteTitle"] = "House rules"
	if rec := call(t, mux, "abena.osei@heliosschool.org", "POST", "/api/celebrate/party", body); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger edited: %d", rec.Code)
	}
	if rec := call(t, mux, other, "POST", "/api/celebrate/party/status", map[string]string{"id": p.ID, "status": StatusOpen}); rec.Code != http.StatusForbidden {
		t.Fatalf("a host approved their own party: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/celebrate/party/status", map[string]string{"id": p.ID, "status": StatusOpen}); rec.Code != http.StatusNoContent {
		t.Fatalf("an admin could not approve: %d %s", rec.Code, rec.Body)
	}
	flags := map[string]any{"id": p.ID, "ticketsOpen": false, "waitlist": true, "adults": true, "students": true}
	if rec := call(t, mux, other, "POST", "/api/celebrate/party/flags", flags); rec.Code != http.StatusNoContent {
		t.Fatalf("flags: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Party(p.ID).Availability(testNow()); got != Closed {
		t.Fatalf("after closing sales: %q", got)
	}
	flags["adults"], flags["students"] = false, false
	if rec := call(t, mux, other, "POST", "/api/celebrate/party/flags", flags); rec.Code != http.StatusBadRequest {
		t.Fatalf("a party for nobody: %d", rec.Code)
	}
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

func TestDeletingAPartyTakesItsHosts(t *testing.T) {
	cache, mux := newServer(t)
	body := map[string]any{"title": "Trivia Night", "price": 20, "adults": true, "ticketsOpen": true, "hostEmails": []string{other, teacher}}
	rec := call(t, mux, admin, "POST", "/api/celebrate/party", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("post: %d %s", rec.Code, rec.Body)
	}
	var made map[string]string
	json.Unmarshal(rec.Body.Bytes(), &made)
	if n := cache.Count(hostsTab, store.Row{"Party ID": made["id"]}); n != 2 {
		t.Fatalf("%d hosts after posting", n)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/celebrate/party", map[string]string{"id": made["id"]}); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if n := cache.Count(hostsTab, store.Row{"Party ID": made["id"]}); n != 0 {
		t.Fatalf("%d hosts outlived their party in memory", n)
	}
	for _, row := range tables(t)[hostsTab] {
		if row["Party ID"] == made["id"] {
			t.Fatalf("the sheet kept %v", row)
		}
	}
	gone := map[string]bool{}
	for _, row := range changeLog(t) {
		if row["Tab"] == hostsTab && row["Action"] == "delete" && row["Column"] == "Email" {
			gone[row["Previous"]] = true
		}
	}
	if !gone[other] || !gone[teacher] {
		t.Fatalf("the hosts' delete was not logged: %v", changeLog(t))
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
	for _, row := range tables(t)[partiesTab] {
		if row["Category"] == "Adult Social" {
			t.Fatalf("the sheet kept the old category on %s", row["Party ID"])
		}
	}
	carried := 0
	for _, row := range changeLog(t) {
		if row["Tab"] == partiesTab && row["Column"] == "Category" && row["Previous"] == "Adult Social" && row["Actor"] == admin {
			carried++
		}
	}
	if carried == 0 || carried != cache.Count(partiesTab, store.Row{"Category": "Grown-Ups"}) {
		t.Fatalf("logged %d carried parties", carried)
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
	if rec := call(t, mux, admin, "POST", "/api/celebrate/celebration", map[string]any{"original": "SC-2027", "code": "SC-2027", "title": "Helios Spring Celebration 2027", "buttonText": "Save the Date", "buttonUrl": "calendar", "current": true}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a calendar button with no date: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, admin, "POST", "/api/celebrate/celebration", map[string]any{"original": "SC-2027", "code": "SC-2027", "title": "Helios Spring Celebration 2027", "start": "2027-03-06 17:30", "buttonText": "Save the Date", "buttonUrl": "calendar", "current": true}); rec.Code != http.StatusNoContent {
		t.Fatalf("a calendar button: %d %s", rec.Code, rec.Body)
	}
	if m = cache.Model(); m.Celebration("SC-2027").ButtonURL != ButtonCalendar {
		t.Fatalf("button: %q", m.Celebration("SC-2027").ButtonURL)
	}
	if m.Banner().Code != "SC-2027" {
		t.Fatalf("banner follows current: %s", m.Banner().Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/celebrate/celebration", map[string]any{"original": "SC-2026", "code": "SC-2026", "title": "Helios Spring Celebration 2026", "start": "2026-03-07 17:30", "banner": true}); rec.Code != http.StatusNoContent {
		t.Fatalf("mark banner: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model()
	if m.Banner().Code != "SC-2026" || m.Current().Code != "SC-2027" {
		t.Fatalf("banner %s current %s", m.Banner().Code, m.Current().Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/celebrate/celebration", map[string]any{"original": "SC-2027", "code": "SC-2027", "title": "Helios Spring Celebration 2027", "start": "2027-03-06 17:30", "current": true, "banner": true}); rec.Code != http.StatusNoContent {
		t.Fatalf("move banner: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model()
	if m.Banner().Code != "SC-2027" || m.Celebration("SC-2026").Banner {
		t.Fatalf("banner did not move: %s", m.Banner().Code)
	}
	if rec := call(t, mux, admin, "DELETE", "/api/celebrate/celebration", map[string]string{"code": "SC-2026"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("deleted a celebration with parties: %d", rec.Code)
	}
	parties := len(m.SortedParties("SC-2025"))
	if rec := call(t, mux, admin, "POST", "/api/celebrate/celebration", map[string]any{"original": "SC-2025", "code": "SC-2025B", "title": "Helios Spring Celebration 2025", "start": "2025-03-01 17:30"}); rec.Code != http.StatusNoContent {
		t.Fatalf("rename celebration: %d %s", rec.Code, rec.Body)
	}
	if m = cache.Model(); parties == 0 || len(m.SortedParties("SC-2025B")) != parties || cache.Count(partiesTab, store.Row{"Celebration": "SC-2025"}) != 0 {
		t.Fatalf("a celebration rename left parties behind: %d of %d moved", len(m.SortedParties("SC-2025B")), parties)
	}
	if cache.Count(invoicingTab, store.Row{"Event Code": "SC-2025"}) != 0 {
		t.Fatal("a celebration rename left ledger rows behind")
	}
	rec := call(t, mux, admin, "GET", "/api/celebrate/invoices.csv?celebration=SC-2025B", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Wines of the Southern Hemisphere!") || strings.Contains(rec.Body.String(), "Fondue") {
		t.Fatalf("invoices: %d %s", rec.Code, rec.Body)
	}
	for _, code := range []string{"SC-2025", `x"; filename*=UTF-8''invoice.html`} {
		if rec := call(t, mux, admin, "GET", "/api/celebrate/invoices.csv?celebration="+url.QueryEscape(code), nil); rec.Code != http.StatusNotFound {
			t.Fatalf("invoices for %q: %d", code, rec.Code)
		}
	}
	if rec := call(t, mux, other, "GET", "/api/celebrate/invoices.csv", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent read the invoices: %d", rec.Code)
	}
}

func TestReorderCategoriesWritesOneKey(t *testing.T) {
	cache, mux := newServer(t)
	order := []string{"Educational", "Family Social", "Adult Social", "Children Social", "Athletic"}
	if rec := call(t, mux, admin, "POST", "/api/celebrate/categories/order", map[string]any{"titles": order}); rec.Code != http.StatusNoContent {
		t.Fatalf("reorder: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Categories; !slices.Equal(got, order) {
		t.Fatalf("categories: %v", got)
	}
	moved := []store.Row{}
	for _, row := range changeLog(t) {
		if row["Tab"] == categoriesTab {
			moved = append(moved, row)
		}
	}
	if len(moved) != 1 || moved[0]["Key"] != "Title=Educational" || moved[0]["Column"] != store.OrderColumn || moved[0]["Previous"] != "9" || moved[0]["Action"] != "set" {
		t.Fatalf("logged %v", moved)
	}
	if rec := call(t, mux, admin, "POST", "/api/celebrate/categories/order", map[string]any{"titles": order[1:]}); rec.Code != http.StatusBadRequest {
		t.Fatalf("an order missing a category: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/celebrate/category", map[string]string{"title": "Musical"}); rec.Code != http.StatusNoContent {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Categories; got[len(got)-1] != "Musical" {
		t.Fatalf("a new category did not land last: %v", got)
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

func TestCSVCell(t *testing.T) {
	for cell, want := range map[string]string{
		"":                "",
		"Ann Lee":         "Ann Lee",
		"-40":             "-40",
		"+12.50":          "+12.50",
		`=HYPERLINK("x")`: `'=HYPERLINK("x")`,
		"@SUM(A1)":        "'@SUM(A1)",
		"-2+3":            "'-2+3",
		"\t=1+1":          "'\t=1+1",
		"a=b":             "a=b",
	} {
		if got := csvCell(cell); got != want {
			t.Errorf("csvCell(%q) = %q, want %q", cell, got, want)
		}
	}
}

func TestFriendlyAddresses(t *testing.T) {
	cache, mux := newServer(t)
	m := cache.Model()
	if m.PathOf(m.Party("P001")) != "/p/fondue" || m.PathOf(m.Party("P005")) != "/parties/P005" {
		t.Fatalf("paths: %s %s", m.PathOf(m.Party("P001")), m.PathOf(m.Party("P005")))
	}
	for path, want := range map[string]string{"/p/fondue": "P001", "/p/Fondue/": "P001", "/parties/P001": "P001", "/p/fondue-night": "P001", "pickleball": "P002", "/p/nothing": ""} {
		got := ""
		if p := m.Resolve(path); p != nil {
			got = p.ID
		}
		if got != want {
			t.Errorf("Resolve(%q) = %q, want %q", path, got, want)
		}
	}
	body := map[string]any{"id": "P003", "title": "K-Pop for a Cause!", "price": 50, "capacity": 20, "adults": true, "students": true, "ticketsOpen": true, "waitlist": true, "prettyId": "Fondue", "hostEmails": []string{"deepa.natarajan@heliosschool.org"}}
	if rec := call(t, mux, "deepa.natarajan@heliosschool.org", "POST", "/api/celebrate/party", body); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "is already the address of") {
		t.Fatalf("took another party's address: %d %s", rec.Code, rec.Body)
	}
	body["prettyId"] = "k pop!"
	if rec := call(t, mux, "deepa.natarajan@heliosschool.org", "POST", "/api/celebrate/party", body); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "friendly address") {
		t.Fatalf("took a malformed address: %d %s", rec.Code, rec.Body)
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
	written := false
	for _, row := range tables(t)[redirectsTab] {
		if row["Old"] == "/p/kpop" && row["New"] == "/p/k-pop" && row["Type"] == RedirectParty && row["Date"] == "2026-09-12" {
			written = true
		}
	}
	if !written {
		t.Fatalf("the redirect is not in the sheet: %v", tables(t)[redirectsTab])
	}
	logged := false
	for _, row := range changeLog(t) {
		if row["Tab"] == redirectsTab && row["Action"] == "insert" && row["Key"] == "Old=/p/kpop" && row["Actor"] == "deepa.natarajan@heliosschool.org" {
			logged = true
		}
	}
	if !logged {
		t.Fatalf("the redirect was not logged: %v", changeLog(t))
	}
	body["title"] = "K-Pop for a Good Cause!"
	before := len(tables(t)[redirectsTab])
	if rec := call(t, mux, "deepa.natarajan@heliosschool.org", "POST", "/api/celebrate/party", body); rec.Code != http.StatusOK {
		t.Fatalf("retitle: %d %s", rec.Code, rec.Body)
	}
	if after := len(tables(t)[redirectsTab]); after != before {
		t.Fatalf("a retitle left a redirect: %d then %d", before, after)
	}
}

func TestSharePreview(t *testing.T) {
	cache, mux := newServer(t)
	head := PreviewHead(cache)
	req := httptest.NewRequest("GET", "/p/fondue", nil)
	req.Host = "celebrate.heliosian.com"
	got := head(req)
	for _, want := range []string{`og:title" content="Fondue &amp; Fort Night"`, `og:url" content="https://celebrate.heliosian.com/p/fondue"`,
		`og:image" content="https://celebrate.heliosian.com/open/share/P001.png"`, `Saturday, September 19 · 5:00 – 9:00 PM — The Parks&#39; House in Los Altos — A cozy evening`} {
		if !strings.Contains(got, want) {
			t.Errorf("preview head lacks %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Alder") {
		t.Error("the street address leaked into the preview")
	}
	for _, path := range []string{"/", "/parties/P013", "/my", "/p/nothing"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Host = "celebrate.heliosian.com"
		got := head(req)
		for _, want := range []string{`og:title" content="Upcoming Parties"`, `og:url" content="https://celebrate.heliosian.com/"`,
			`og:image" content="https://celebrate.heliosian.com/open/share/upcoming.png"`,
			`Next up: Fondue &amp; Fort Night — Saturday, September 19 · 5:00 – 9:00 PM — The Parks&#39; House in Los Altos. Also coming: Dink &amp; Clink (Sep 26), K-Pop for a Cause! (Sep 27), Wurst Helios Party (Oct 3).`} {
			if !strings.Contains(got, want) {
				t.Errorf("%s: preview head lacks %s:\n%s", path, want, got)
			}
		}
		for _, leak := range []string{"Alder", "Baegels", "Backyard", "Crochet"} {
			if strings.Contains(got, leak) {
				t.Errorf("%s: %s is in the preview", path, leak)
			}
		}
	}
	ids := []string{}
	for _, p := range upcoming(cache.Model()) {
		ids = append(ids, p.ID)
	}
	if strings.Join(ids, " ") != "P001 P002 P003 P004 P012 P005 P009" {
		t.Errorf("upcoming: %v", ids)
	}
	for _, path := range []string{"/open/share/P001.png", "/open/share/upcoming.png"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || rec.Body.Len() < 10000 {
			t.Fatalf("%s: %d %s %d bytes", path, rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
		}
		etag := rec.Header().Get("ETag")
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("If-None-Match", etag)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotModified {
			t.Errorf("%s again with its ETag: %d", path, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/open/share/P013.png", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("a pending party's card: %d", rec.Code)
	}
}

func TestMail(t *testing.T) {
	rec := recorder{got: make(chan mail.Message, 8)}
	cache, mux := serveWith(t, rec)
	buy := func(as, party, purchaser string, attendees ...map[string]string) *httptest.ResponseRecorder {
		return call(t, mux, as, "POST", "/api/celebrate/tickets", map[string]any{"partyId": party, "purchaser": purchaser, "note": "We\u2019ll be a little late", "attendees": attendees})
	}
	if r := buy(partner, "P004", parent, map[string]string{"email": partner}, map[string]string{"name": "Aunt May"}); r.Code != http.StatusOK {
		t.Fatalf("buy: %d %s", r.Code, r.Body)
	}
	pair := func() (note, invite mail.Message) {
		for range 2 {
			m := rec.next(t)
			if len(m.Attachments) > 0 {
				invite = m
			} else {
				note = m
			}
		}
		return note, invite
	}
	m, invite := pair()
	if m.Subject != "Your tickets to Wurst Helios Party" || !slices.Equal(m.To, []string{parent}) || !slices.Equal(m.CC, []string{partner, "sofia.marchetti@heliosschool.org", "paolo.marchetti@heliosschool.org"}) || len(m.Attachments) != 0 {
		t.Fatalf("confirmation: %+v", m)
	}
	for _, want := range []string{"Hi Jordan", "Robin Whitfield, Aunt May", "$150 (2 × $75)", "Robin Whitfield took them", "/open/share/P004.png", "88 Castro Street", "invoiced by Helios", "calendar.google.com/calendar/render?action=TEMPLATE", "Add to Calendar"} {
		if !strings.Contains(m.HTML, want) {
			t.Errorf("confirmation lacks %q", want)
		}
	}
	if !slices.Equal(m.ReplyTo, []string{"sofia.marchetti@heliosschool.org", "paolo.marchetti@heliosschool.org"}) {
		t.Errorf("reply-to: %v", m.ReplyTo)
	}
	if invite.Subject != "Calendar invite: Wurst Helios Party" || !slices.Equal(invite.To, []string{parent}) || len(invite.CC) != 0 || !slices.Equal(invite.ReplyTo, m.ReplyTo) || !strings.Contains(invite.HTML, "Add it to your calendar") {
		t.Fatalf("invite note: %+v", invite)
	}
	ics := strings.ReplaceAll(string(invite.Attachments[0].Content), "\r\n ", "")
	if invite.Attachments[0].Name != "invite.ics" || !strings.HasPrefix(invite.Attachments[0].ContentType, "text/calendar; method=REQUEST") {
		t.Fatalf("invite: %+v", invite.Attachments[0])
	}
	for _, want := range []string{"METHOD:REQUEST", "UID:celebrate-P004-" + parent + "@heliosian.com", "SUMMARY:Wurst Helios Party", "DTSTART:20261003T", "ORGANIZER;CN=Helios Celebrate:mailto:celebrate@example.org", "ATTENDEE;CN=Jordan Whitfield;ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED;RSVP=FALSE:mailto:" + parent, "LOCATION:88 Castro Street"} {
		if !strings.Contains(ics, want) {
			t.Errorf("invite lacks %q:\n%s", want, ics)
		}
	}
	if strings.Contains(ics, "mailto:sofia.marchetti") {
		t.Errorf("a host is on the family's invite:\n%s", ics)
	}
	if r := buy(parent, "P003", "", map[string]string{"email": kid}); r.Code != http.StatusOK {
		t.Fatalf("buy for a child: %d %s", r.Code, r.Body)
	}
	m, invite = pair()
	if m.Subject != "Sam Whitfield's ticket to K-Pop for a Cause!" || !slices.Equal(m.To, []string{parent, partner}) || !slices.Contains(m.CC, "deepa.natarajan@heliosschool.org") {
		t.Fatalf("a child's note: %+v", m)
	}
	for _, want := range []string{"Sam, you&#39;re going!", "Hi Sam - you have a ticket"} {
		if !strings.Contains(m.HTML, want) {
			t.Errorf("a child's note lacks %q", want)
		}
	}
	if ics := string(invite.Attachments[0].Content); !slices.Equal(invite.To, []string{parent, partner}) || !strings.Contains(ics, "mailto:"+parent) || !strings.Contains(ics, "mailto:"+partner) {
		t.Errorf("a child's invite: to %v\n%s", invite.To, ics)
	}
	if r := call(t, mux, "sofia.marchetti@heliosschool.org", "POST", "/api/celebrate/tickets", map[string]any{"partyId": "P004", "free": true, "attendees": []map[string]string{{"name": "Peter Parker"}}}); r.Code != http.StatusOK {
		t.Fatalf("free: %d %s", r.Code, r.Body)
	}
	m, invite = pair()
	if !strings.Contains(m.HTML, "no charge") || strings.Contains(m.HTML, "invoiced") || !strings.Contains(m.HTML, "Add to Calendar") {
		t.Fatalf("free ticket note: %s", m.HTML)
	}
	if !slices.Equal(m.CC, []string{"paolo.marchetti@heliosschool.org"}) || !slices.Equal(invite.To, []string{"sofia.marchetti@heliosschool.org"}) || len(invite.CC) != 0 {
		t.Fatalf("free ticket note cc %v / invite to %v cc %v", m.CC, invite.To, invite.CC)
	}
	if r := call(t, mux, "sofia.marchetti@heliosschool.org", "POST", "/api/celebrate/tickets", map[string]any{"partyId": "P004", "free": true, "attendees": []map[string]string{{"name": "Michael Bolin", "email": "mbolin@example.com"}}}); r.Code != http.StatusOK {
		t.Fatalf("outside guest: %d %s", r.Code, r.Body)
	}
	m, invite = pair()
	if m.Subject != "Michael Bolin's ticket to Wurst Helios Party" || !slices.Contains(m.To, "mbolin@example.com") {
		t.Fatalf("outside guest's note: %+v", m)
	}
	for _, want := range []string{"Michael, you&#39;re going!", "Hi Michael - you have a ticket"} {
		if !strings.Contains(m.HTML, want) {
			t.Errorf("outside guest's note lacks %q", want)
		}
	}
	if strings.Contains(m.HTML, "Mbolin") {
		t.Errorf("outside guest's note goes by their address:\n%s", m.HTML)
	}
	if ics := strings.ReplaceAll(string(invite.Attachments[0].Content), "\r\n ", ""); !strings.Contains(ics, "ATTENDEE;CN=Michael Bolin;") {
		t.Errorf("outside guest's invite goes by their address:\n%s", ics)
	}
	if r := call(t, mux, teacher, "POST", "/api/celebrate/waitlist", map[string]any{"partyId": "P006", "quantity": 2, "note": "either day works"}); r.Code != http.StatusOK {
		t.Fatalf("waitlist: %d %s", r.Code, r.Body)
	}
	byTo := map[string]mail.Message{}
	for range 2 {
		m = rec.next(t)
		byTo[m.To[0]] = m
	}
	m = byTo[teacher]
	if m.Subject != "You're on the waitlist for Baegels and Meimosas" || !strings.Contains(m.HTML, "is full") || !strings.Contains(m.HTML, "waitlist for 2 tickets") || !strings.Contains(m.HTML, "Nothing is billed") || len(m.CC) != 0 {
		t.Fatalf("waitlist note: %s cc %v\n%s", m.Subject, m.CC, m.HTML)
	}
	if !slices.Equal(m.ReplyTo, []string{"freja.lindqvist@heliosschool.org", "anders.lindqvist@heliosschool.org"}) {
		t.Errorf("waitlist reply-to: %v", m.ReplyTo)
	}
	m = byTo["freja.lindqvist@heliosschool.org"]
	if m.Subject != "Waitlist for Baegels and Meimosas: Grace Kim wants 2 tickets" || !slices.Equal(m.To, []string{"freja.lindqvist@heliosschool.org", "anders.lindqvist@heliosschool.org"}) || !slices.Equal(m.ReplyTo, []string{teacher}) || !strings.Contains(m.HTML, "either day works") {
		t.Fatalf("hosts' waitlist note: %+v", m)
	}
	var waiting string
	for _, tk := range cache.Model().Party("P006").Tickets {
		if tk.Status == TicketWaitlist && tk.Purchaser == parent {
			waiting = tk.ID
		}
	}
	if r := call(t, mux, "freja.lindqvist@heliosschool.org", "POST", "/api/celebrate/waitlist/offer", map[string]any{"ticketId": waiting}); r.Code != http.StatusNoContent {
		t.Fatalf("offer: %d %s", r.Code, r.Body)
	}
	m, invite = pair()
	if m.Subject != "You're in: 2 tickets to Baegels and Meimosas" || !slices.Equal(m.To, []string{parent}) || !slices.Equal(m.CC, []string{"freja.lindqvist@heliosschool.org", "anders.lindqvist@heliosschool.org"}) || !strings.Contains(m.HTML, "Freja Lindqvist has offered") || !strings.Contains(m.HTML, "to be named") {
		t.Fatalf("offer note: %+v", m)
	}
	if !slices.Equal(invite.To, []string{parent}) || !strings.Contains(string(invite.Attachments[0].Content), "UID:celebrate-P006-"+parent+"@heliosian.com") {
		t.Fatalf("offer invite: %+v", invite)
	}
}

func TestReassign(t *testing.T) {
	cache, mux := newServer(t)
	var mine string
	for _, tk := range cache.Model().Party("P001").Tickets {
		if tk.Email == teen {
			mine = tk.ID
		}
	}
	if rec := call(t, mux, other, "POST", "/api/celebrate/ticket/reassign", map[string]any{"ticketId": mine, "name": "Percy Jackson"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger reassigned: %d", rec.Code)
	}
	if rec := call(t, mux, partner, "POST", "/api/celebrate/ticket/reassign", map[string]any{"ticketId": mine, "name": "Percy Jackson", "email": "percy.jackson@gmail.com"}); rec.Code != http.StatusNoContent {
		t.Fatalf("reassign to a guest: %d %s", rec.Code, rec.Body)
	}
	tk, _ := cache.Model().TicketByID(mine)
	if tk.Name != "Percy Jackson" || tk.Email != "percy.jackson@gmail.com" || tk.Purchaser != parent || tk.Price != 65 || tk.Status != TicketSold {
		t.Fatalf("after reassign: %+v", tk)
	}
	for _, other := range cache.Model().Party("P001").Tickets {
		if other.Email == teen {
			t.Fatal("Ella is still on the list")
		}
	}
	previous := map[string]string{}
	for _, row := range changeLog(t) {
		if row["Key"] == "Ticket ID="+mine {
			previous[row["Column"]] = row["Previous"]
		}
	}
	if len(previous) != 2 || previous["Email"] != teen || previous["Name"] != "" {
		t.Fatalf("reassign logged %v", previous)
	}
	if rec := call(t, mux, "mina.park@heliosschool.org", "POST", "/api/celebrate/ticket/reassign", map[string]any{"ticketId": mine, "email": teacher}); rec.Code != http.StatusNoContent {
		t.Fatalf("a host could not reassign: %d %s", rec.Code, rec.Body)
	}
	tk, _ = cache.Model().TicketByID(mine)
	if tk.Email != teacher || tk.Name != "" || tk.Purchaser != parent {
		t.Fatalf("after the host's reassign: %+v", tk)
	}
	if rec := call(t, mux, "mina.park@heliosschool.org", "POST", "/api/celebrate/ticket/reassign", map[string]any{"ticketId": mine, "email": parent}); rec.Code != http.StatusBadRequest {
		t.Fatalf("reassigned to someone with a ticket: %d", rec.Code)
	}
	var adults string
	for _, tk := range cache.Model().Party("P002").Tickets {
		if tk.Email == parent {
			adults = tk.ID
		}
	}
	if rec := call(t, mux, parent, "POST", "/api/celebrate/ticket/reassign", map[string]any{"ticketId": adults, "email": kid}); rec.Code != http.StatusBadRequest {
		t.Fatalf("reassigned an adults-only ticket to a child: %d", rec.Code)
	}
}

func TestMoveAddress(t *testing.T) {
	cache, _ := newServer(t)
	type move struct{ actor, old, to, name string }
	told := []move{}
	mux := http.NewServeMux()
	Register(mux, cache, nil, fakeDirectory{}, func() []string { return nil }, ImageSearch{}, nil, testFrom, nil, func(_ context.Context, actor, old, to, name string) {
		told = append(told, move{actor, old, to, name})
	})
	const (
		school = "ella.graduated@heliosschool.org"
		home   = "ella.w@gmail.com"
		later  = "ella.whitfield@college.edu"
	)
	var held string
	for _, tk := range cache.Model().Party("P001").Tickets {
		if tk.Email == teen {
			held = tk.ID
		}
	}
	// A ticket that took its name from the directory while its holder was
	// still in it: an address and no name.
	if rec := call(t, mux, admin, "POST", "/api/celebrate/ticket/reassign", map[string]any{"ticketId": held, "email": school}); rec.Code != http.StatusNoContent {
		t.Fatalf("set up the alum's ticket: %d %s", rec.Code, rec.Body)
	}
	body := map[string]any{"old": school, "to": home, "name": "Ella Whitfield"}
	if rec := call(t, mux, parent, "POST", "/api/celebrate/address", body); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent moved an address: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/celebrate/address", map[string]any{"old": parent, "to": home}); rec.Code != http.StatusBadRequest {
		t.Fatalf("moved an address the directory holds: %d", rec.Code)
	}
	if rec := call(t, mux, admin, "POST", "/api/celebrate/address", body); rec.Code != http.StatusNoContent {
		t.Fatalf("move: %d %s", rec.Code, rec.Body)
	}
	tk, _ := cache.Model().TicketByID(held)
	if tk.Email != home || tk.Name != "Ella Whitfield" || tk.Purchaser != parent {
		t.Fatalf("after the move: %+v", tk)
	}
	if len(told) != 1 || told[0] != (move{admin, school, home, "Ella Whitfield"}) {
		t.Fatalf("When was told %+v", told)
	}
	if rec := call(t, mux, admin, "POST", "/api/celebrate/address", body); rec.Code != http.StatusBadRequest {
		t.Fatalf("moved the same address twice: %d", rec.Code)
	}

	// A ticket row still naming the old address - pasted in by hand - reads
	// as the new one.
	if err := cache.Commit(context.Background(), admin, store.Insert(ticketsTab, store.Row{"Ticket ID": "TOLD", "Party ID": "P002", "Email": school, "Purchaser": school, "Status": TicketSold, "Price": "0"})); err != nil {
		t.Fatal(err)
	}
	if tk, _ := cache.Model().TicketByID("TOLD"); tk.Email != home || tk.Purchaser != home || tk.Name != "Ella Whitfield" {
		t.Fatalf("a row naming the old address: %+v", tk)
	}

	// Moving again points the first move at the last address too, so no
	// address is ever a step away from where it went.
	if rec := call(t, mux, admin, "POST", "/api/celebrate/address", map[string]any{"old": home, "to": later}); rec.Code != http.StatusNoContent {
		t.Fatalf("second move: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().CurrentAddress(school); got != later {
		t.Fatalf("the first address now goes to %s", got)
	}
	if tk, _ := cache.Model().TicketByID(held); tk.Email != later || tk.Name != "Ella Whitfield" {
		t.Fatalf("after the second move: %+v", tk)
	}
	queue.Flush()
	_, rows, err := sheet.Table(appName, formerTab)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["New"] != later || rows[1]["Old"] != home || rows[1]["New"] != later || rows[0]["Changed"] != today() {
		t.Fatalf("former addresses: %v", rows)
	}
}

func TestFormerAddressesRefuseALoop(t *testing.T) {
	_, _ = newServer(t)
	tabs := tables(t)
	tabs[formerTab] = []store.Row{{"Old": "a@x.org", "New": "b@x.org"}, {"Old": "b@x.org", "New": "c@x.org"}}
	if _, err := BuildModel(context.Background(), tabs, bundled{}); err == nil || !strings.Contains(err.Error(), "has itself moved") {
		t.Fatalf("a chain loaded: %v", err)
	}
	tabs[formerTab] = []store.Row{{"Old": "a@x.org", "New": "a@x.org"}}
	if _, err := BuildModel(context.Background(), tabs, bundled{}); err == nil {
		t.Fatal("an address moving to itself loaded")
	}
}

func TestProblemAddresses(t *testing.T) {
	cache, mux := newServer(t)
	const (
		alum    = "maya.lin@heliosschool.org"
		outside = "percy.jackson@gmail.com"
	)
	var tickets []string
	for _, tk := range cache.Model().Party("P001").Tickets {
		if tk.Email == teen || tk.Email == kid {
			tickets = append(tickets, tk.ID)
		}
	}
	call(t, mux, admin, "POST", "/api/celebrate/ticket/reassign", map[string]any{"ticketId": tickets[0], "email": alum})
	call(t, mux, admin, "POST", "/api/celebrate/ticket/reassign", map[string]any{"ticketId": tickets[1], "email": outside, "name": "Percy Jackson"})
	if rec := call(t, mux, parent, "GET", "/api/celebrate/addresses", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent read the addresses: %d", rec.Code)
	}
	rec := call(t, mux, admin, "GET", "/api/celebrate/addresses", nil)
	var view struct {
		Problems []Problem
		Moved    []Moved
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil || rec.Code != 200 {
		t.Fatalf("addresses: %d %s", rec.Code, rec.Body)
	}
	// The test directory holds a handful of people, so the sample's other
	// school addresses are listed too; what matters is who is and is not.
	find := func(email string) *Problem {
		for i := range view.Problems {
			if view.Problems[i].Email == email {
				return &view.Problems[i]
			}
		}
		return nil
	}
	if pr := find(alum); pr == nil || pr.Name != "Maya Lin" || len(pr.Uses) != 1 || pr.Uses[0].Role != "Ticket" || pr.Uses[0].Path != "/p/fondue" || !pr.Upcoming {
		t.Fatalf("the alum = %+v", pr)
	}
	if find(outside) != nil || find(parent) != nil || find(teen) != nil {
		t.Fatalf("listed an outside or directory address")
	}
	call(t, mux, admin, "POST", "/api/celebrate/address", map[string]any{"old": alum, "to": "maya@college.edu", "name": "Maya Lin"})
	rec = call(t, mux, admin, "GET", "/api/celebrate/addresses", nil)
	view.Problems, view.Moved = nil, nil
	json.Unmarshal(rec.Body.Bytes(), &view)
	if find(alum) != nil || len(view.Moved) != 1 || view.Moved[0].Old != alum || view.Moved[0].New != "maya@college.edu" {
		t.Fatalf("after the change: %+v %+v", view.Problems, view.Moved)
	}
}
