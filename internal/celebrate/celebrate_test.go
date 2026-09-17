package celebrate

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/data"
	"heliosian/internal/mail"
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

func (fakeDirectory) Alerts(string) (int, bool) { return 0, false }

type bundled struct{}

func (bundled) Has(key string) (bool, error) { return strings.HasPrefix(key, "sample/"), nil }

func (bundled) Prefetch([]string) error { return nil }

// lastDir is the store behind the most recent newServer, for a test that
// wants to read what the server wrote.
var lastDir *data.Dir

func newServer(t *testing.T) (*Cache, *http.ServeMux) {
	t.Helper()
	t.Chdir("../..")
	now = testNow
	dir := &data.Dir{Root: "sampledata"}
	lastDir = dir
	// Nobody is a super admin here; Dana is on the sample Admins tab.
	cache, err := NewCache(dir, bundled{}, func(string) bool { return false }, syncQueue{})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, cache, dir, syncQueue{}, nil, fakeDirectory{}, func() []string { return nil }, ImageSearch{}, nil, testFrom)
	return cache, mux
}

// recorder is a mail.Sender that hands each message to the test.
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
	// Three families wait on the bagels, one of them for two tickets.
	if m.Party("P006").Waiting() != 4 || m.Party("P006").Full() != true {
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
	// A student takes no tickets, joins no waitlist and passes none on;
	// their parent does.
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
	// A parent takes tickets for the household; a student is refused where
	// only adults may come.
	if rec := buy(parent, "P002", "", map[string]string{"email": kid}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "for adults") {
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
	// A student's ticket is taken by a parent, and billed to one.
	if rec := buy(partner, "P003", parent, map[string]string{"email": kid}); rec.Code != http.StatusOK {
		t.Fatalf("a parent could not take a child's ticket: %d %s", rec.Code, rec.Body)
	}
	// The last ticket goes; the next person waits.
	if rec := buy(parent, "P002", "", map[string]string{"name": "Aunt May"}); rec.Code != http.StatusOK {
		t.Fatalf("a guest could not take the last ticket: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Party("P002").Availability(testNow()); got != Waitlist {
		t.Fatalf("availability after filling: %q", got)
	}
	// A full party sells nothing more: the family joins the waitlist as a
	// request for so many tickets, billed to an adult of its own; asking
	// again changes the request rather than joining the line twice.
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
	// Sold out with no waitlist refuses; closed refuses; a host is never
	// refused for room.
	if rec := buy(parent, "P008", "", map[string]string{"email": parent}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "sold out") {
		t.Fatalf("sold out: %d %s", rec.Code, rec.Body)
	}
	if rec := buy(other, "P011", "", map[string]string{"email": other}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "closed") {
		t.Fatalf("closed: %d %s", rec.Code, rec.Body)
	}
	// A host adds anyone, but bills nobody outside their own family: the
	// person added pays their own way, a student through a parent.
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

// A host gives a free ticket: minted at $0, so the invoice list never sees
// it; nobody else can.
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
	// The gift can grow the cap by one, so it takes no paid place.
	was := p.Capacity
	body["attendees"] = []map[string]string{{"name": "Annabeth Chase"}}
	body["raiseCapacity"] = true
	if rec := call(t, mux, other, "POST", "/api/celebrate/tickets", body); rec.Code != http.StatusOK {
		t.Fatalf("free ticket with room: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Party("P002").Capacity; got != was+1 {
		t.Fatalf("capacity %d, want %d", got, was+1)
	}
	// A gift can name whose guest the holder is: an adult in the directory
	// holds it for them - a student cannot.
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
	// The accounting ledger never sees a free ticket - and sees every paid
	// one as ADD, one at its cost, with the invoice columns left blank.
	dir := lastDir
	_, ledger, _ := dir.Table(appName, "INVOICING")
	before := len(ledger)
	for _, l := range ledger {
		if l["Guest Name"] == "Percy Jackson" || l["Guest Name"] == "Annabeth Chase" || l["Guest Name"] == "Grover Underwood" {
			t.Fatalf("a free ticket in the ledger: %v", l)
		}
	}
	if rec := call(t, mux, other, "POST", "/api/celebrate/tickets", map[string]any{"partyId": "P002", "attendees": []map[string]string{{"name": "Grover Underwood"}}}); rec.Code != http.StatusOK {
		t.Fatalf("paid ticket: %d %s", rec.Code, rec.Body)
	}
	_, ledger, _ = dir.Table(appName, "INVOICING")
	last := ledger[len(ledger)-1]
	if len(ledger) != before+1 || last["Action"] != "ADD" || last["Quantity"] != "1" || last["Cost"] != "75" || last["Purchaser Email"] != other || last["Guest Name"] != "Grover Underwood" || last["Event Code"] != "SC-2026" || last["Invoice"] != "" {
		t.Fatalf("ledger: %v", last)
	}
	// The model reads the ledger too, for Admin Tools, and only an admin
	// is shown it.
	if n := len(cache.Model().Invoicing); n != before+1 {
		t.Fatalf("model ledger %d, want %d", n, before+1)
	}
	if v := Render(cache.Model(), fakeDirectory{}, other, false, testNow()); len(v.Invoicing) != 0 {
		t.Fatalf("a parent was shown the ledger")
	}
	if v := Render(cache.Model(), fakeDirectory{}, admin, true, testNow()); len(v.Invoicing) != before+1 {
		t.Fatalf("the admin was not shown the ledger")
	}
}

// Hosting Open off: a parent's new party is refused, an admin's is not,
// and a host still edits what they have.
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
	if rec := call(t, mux, other, "POST", "/api/celebrate/waitlist/offer", map[string]any{"ticketId": waiting}); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger offered tickets: %d", rec.Code)
	}
	// The host offers one of the two asked for: Jordan holds it, the request
	// shrinks to one; then the other, as a guest to be named, and the request
	// is gone.
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
	// Once sold, a ticket is the fundraiser's: the family cannot give it back,
	// though it may still leave a waitlist.
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
	// A host's category is ignored: that is an admin's to file.
	if p == nil || p.Status != StatusPending || !p.Hosted(other) || p.Celebration != "SC-2026" || p.Price != 40 || p.AddedBy != other || p.Category != "" {
		t.Fatalf("posted party: %+v", p)
	}
	// The host edits it but cannot approve it; a stranger cannot touch it.
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
	// The callout's dress stays small.
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
	// The switches: a host closes sales and the party stops selling.
	flags := map[string]any{"id": p.ID, "ticketsOpen": false, "waitlist": true, "adults": true, "students": true}
	if rec := call(t, mux, other, "POST", "/api/celebrate/party/flags", flags); rec.Code != http.StatusNoContent {
		t.Fatalf("flags: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Party(p.ID).Availability(testNow()); got != Closed {
		t.Fatalf("after closing sales: %q", got)
	}
	// Nobody may be let in at all: refused.
	flags["adults"], flags["students"] = false, false
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
	// The button is a link, or "calendar" for a Save the Date - which needs
	// a date to save.
	if rec := call(t, mux, admin, "POST", "/api/celebrate/celebration", map[string]any{"original": "SC-2027", "code": "SC-2027", "title": "Helios Spring Celebration 2027", "buttonText": "Save the Date", "buttonUrl": "calendar", "current": true}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a calendar button with no date: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, admin, "POST", "/api/celebrate/celebration", map[string]any{"original": "SC-2027", "code": "SC-2027", "title": "Helios Spring Celebration 2027", "start": "2027-03-06 17:30", "buttonText": "Save the Date", "buttonUrl": "calendar", "current": true}); rec.Code != http.StatusNoContent {
		t.Fatalf("a calendar button: %d %s", rec.Code, rec.Body)
	}
	if m = cache.Model(); m.Celebration("SC-2027").ButtonURL != ButtonCalendar {
		t.Fatalf("button: %q", m.Celebration("SC-2027").ButtonURL)
	}
	// The banner is its own flag: none marked, it follows the current one;
	// marking one unmarks the rest and leaves Current alone.
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
	body := map[string]any{"id": "P003", "title": "K-Pop for a Cause!", "price": 50, "capacity": 20, "adults": true, "students": true, "ticketsOpen": true, "waitlist": true, "prettyId": "Fondue", "hostEmails": []string{"deepa.natarajan@heliosschool.org"}}
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
		`og:image" content="https://celebrate.heliosian.com/open/share/P001.png"`, `Saturday, September 19 · 5:00 – 9:00 PM — The Parks&#39; House in Los Altos — A cozy evening`} {
		if !strings.Contains(got, want) {
			t.Errorf("preview head lacks %s:\n%s", want, got)
		}
	}
	// The street stays out of the tags. A pending party and any other page
	// preview the site itself: the next party with tickets and the three
	// after it - never one that is full (P006, today), sold out later
	// (P007), closed to sales (P011), pending or hidden.
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

// A purchase mails whoever is billed with the hosts copied; a full party's
// note says so; and a host's waitlist offer gets its own note.
func TestMail(t *testing.T) {
	t.Chdir("../..")
	now = testNow
	dir := &data.Dir{Root: "sampledata"}
	cache, err := NewCache(dir, bundled{}, func(string) bool { return false }, syncQueue{})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	rec := recorder{got: make(chan mail.Message, 8)}
	Register(mux, cache, dir, syncQueue{}, nil, fakeDirectory{}, func() []string { return nil }, ImageSearch{}, rec, testFrom)
	buy := func(as, party, purchaser string, attendees ...map[string]string) *httptest.ResponseRecorder {
		return call(t, mux, as, "POST", "/api/celebrate/tickets", map[string]any{"partyId": party, "purchaser": purchaser, "note": "We\u2019ll be a little late", "attendees": attendees})
	}
	// Robin takes two tickets to the Wurst party, billed to Jordan.
	if r := buy(partner, "P004", parent, map[string]string{"email": partner}, map[string]string{"name": "Aunt May"}); r.Code != http.StatusOK {
		t.Fatalf("buy: %d %s", r.Code, r.Body)
	}
	// Two notes: the confirmation, to the family with the hosts - and
	// Robin, who took the tickets - copied; then the invite, to the family
	// alone. Both reply to the hosts.
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
	// Long lines are folded on the wire; unfold them to read.
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

	// One ticket for a child is the child's note: it speaks to them by name
	// and goes to both parents, the hosts copied.
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
	// Both parents get the invite, and nobody else.
	if ics := string(invite.Attachments[0].Content); !slices.Equal(invite.To, []string{parent, partner}) || !strings.Contains(ics, "mailto:"+parent) || !strings.Contains(ics, "mailto:"+partner) {
		t.Errorf("a child's invite: to %v\n%s", invite.To, ics)
	}
	// A free ticket's note says nothing about invoicing, and still offers
	// the calendar.
	if r := call(t, mux, "sofia.marchetti@heliosschool.org", "POST", "/api/celebrate/tickets", map[string]any{"partyId": "P004", "free": true, "attendees": []map[string]string{{"name": "Peter Parker"}}}); r.Code != http.StatusOK {
		t.Fatalf("free: %d %s", r.Code, r.Body)
	}
	// Sofia hosts and gave it, so the note is hers with the other host,
	// Paolo, copied; the invite is hers alone.
	m, invite = pair()
	if !strings.Contains(m.HTML, "no charge") || strings.Contains(m.HTML, "invoiced") || !strings.Contains(m.HTML, "Add to Calendar") {
		t.Fatalf("free ticket note: %s", m.HTML)
	}
	if !slices.Equal(m.CC, []string{"paolo.marchetti@heliosschool.org"}) || !slices.Equal(invite.To, []string{"sofia.marchetti@heliosschool.org"}) || len(invite.CC) != 0 {
		t.Fatalf("free ticket note cc %v / invite to %v cc %v", m.CC, invite.To, invite.CC)
	}
	// A full party: joining the waitlist gets a note that says so, and that
	// nothing is billed yet.
	if r := call(t, mux, teacher, "POST", "/api/celebrate/waitlist", map[string]any{"partyId": "P006", "quantity": 2, "note": "either day works"}); r.Code != http.StatusOK {
		t.Fatalf("waitlist: %d %s", r.Code, r.Body)
	}
	// Two notes go out, in whatever order: the family's, with the hosts to
	// reply to, and the hosts' own, with the family to reply to.
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
	// The host offers Jordan the two bagels places: Jordan hears.
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

// A sold ticket moves to someone else - a sibling, a guest, another family
// - and its billing always stays put.
func TestReassign(t *testing.T) {
	cache, mux := newServer(t)
	var mine string
	for _, tk := range cache.Model().Party("P001").Tickets {
		if tk.Email == teen {
			mine = tk.ID
		}
	}
	// A stranger cannot; the family can, to a guest; the ticket keeps its
	// price and its purchaser, and Ella is off the list.
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
	// A host reassigns too - to someone in the directory - and the billing
	// still stays put: a resale is the families' own business.
	if rec := call(t, mux, "mina.park@heliosschool.org", "POST", "/api/celebrate/ticket/reassign", map[string]any{"ticketId": mine, "email": teacher}); rec.Code != http.StatusNoContent {
		t.Fatalf("a host could not reassign: %d %s", rec.Code, rec.Body)
	}
	tk, _ = cache.Model().TicketByID(mine)
	if tk.Email != teacher || tk.Name != "" || tk.Purchaser != parent {
		t.Fatalf("after the host's reassign: %+v", tk)
	}
	// Nobody who already holds a ticket, and nobody the party keeps out.
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
