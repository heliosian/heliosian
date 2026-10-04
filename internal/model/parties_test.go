package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

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
	partner      = "robin.whitfield@heliosschool.org"
	partiesKid   = "sam.whitfield@heliosschool.org"
	teen         = "ella.whitfield@heliosschool.org"
	partiesAdmin = "dana.hawkins@heliosschool.org"
	teacher      = "grace.kim@heliosschool.org"
)

func partiesTestNow() time.Time {
	t, _ := time.Parse(DateTimeFormat, "2026-09-12 12:00")
	return t
}

var partiesSampleDirectory *Directory

func partyViewerOf(email string, admin bool) access.Actor {
	var held []access.Allowance
	if admin {
		held = PartiesAdminAllowances
	}
	return partiesSampleDirectory.ActorOf(email, held)
}

var partiesBundled = testkit.Images(func(key string) bool { return strings.HasPrefix(key, "sample/") })

func partiesServeWith(t *testing.T, mailer *mail.Mailgun) (*Store, *http.ServeMux) {
	t.Helper()
	sampleSheet(t)
	was := now
	now = partiesTestNow
	t.Cleanup(func() { now = was })
	outsideSuperAdmin(t, sheet)
	listRows(t, nil)
	deps := sampleDeps(sampleKey)
	deps.Parties = partiesBundled
	s := sampleStore(t, sheet, queue, deps)
	partiesSampleDirectory = s.Model().Directory
	mux := http.NewServeMux()
	calendar := calendarOver(s)
	parties := RegisterParties(mux, PartiesDeps{
		Store:    s,
		Images:   blob.NewImages(blob.New(blob.NewMemoryBucket()), "celebrate"),
		Calendar: calendar,
		Mailer:   mailer,
		Style:    partiesTestStyle,
	})
	typedRegistry(s, queue, DirectoryResources(s), calendar.Resources(), parties.Resources()).Register(mux)
	return s, mux
}

var partiesTestStyle = PartiesCardStyle(func() string { return "Helios Celebrate" }, func() string { return "Fun(d)raiser Parties" })

func partiesServer(t *testing.T) (*Store, *http.ServeMux) {
	t.Helper()
	return partiesServeWith(t, mailtest.Discard())
}

func partiesTables(t *testing.T) store.Tables {
	t.Helper()
	return testkit.Tables(t, sheet, queue, partiesAppName, celebrationsTab, partyCategoriesTab, partiesTab, hostsTab, ticketsTab, partySettingsTab, AdminsTab.Name, partyRedirectsTab, invoicingTab, id.AliasesTab)
}

func categoryTitles(m *Parties) []string {
	out := []string{}
	for _, c := range m.Categories {
		out = append(out, c.Title)
	}
	return out
}

func partiesChangeLog(t *testing.T) []store.Row {
	t.Helper()
	return testkit.ChangeLog(t, sheet, queue, partiesAppName)
}

func partyKey(partyID string) string {
	return id.Of(sampleKey, kindParty, partyID)
}

func celebrateSettingsKey() string {
	return id.Of(sampleKey, kindPartiesSettings, "")
}

func celebratePath(action string) string {
	return "/api/celebrate-settings/" + celebrateSettingsKey() + "/" + action
}

type partyRead struct {
	partyResource
	Can map[string]bool `json:"can"`
}

func partiesAs(t *testing.T, mux http.Handler, as string) []partyRead {
	t.Helper()
	out := decoded[envelope](t, testkit.Call(t, mux, as, "GET", "/api/parties", nil))
	var keys []string
	if err := json.Unmarshal(out.Result, &keys); err != nil {
		t.Fatal(err)
	}
	parties := []partyRead{}
	for _, key := range keys {
		var p partyRead
		if err := json.Unmarshal(out.Resources["parties"][key], &p); err != nil {
			t.Fatal(err)
		}
		parties = append(parties, p)
	}
	return parties
}

func partyAs(t *testing.T, mux http.Handler, as, partyID string) partyRead {
	t.Helper()
	out := decoded[envelope](t, testkit.Call(t, mux, as, "GET", "/api/parties/"+partyID, nil))
	var p partyRead
	if err := json.Unmarshal(out.Resources["parties"][partyKey(partyID)], &p); err != nil {
		t.Fatalf("no party %s: %v", partyID, err)
	}
	return p
}

func celebrateSettingsAs(t *testing.T, mux http.Handler, as string) partiesSettingsResource {
	t.Helper()
	out := decoded[envelope](t, testkit.Call(t, mux, as, "GET", "/api/celebrate-settings/"+celebrateSettingsKey(), nil))
	var s partiesSettingsResource
	if err := json.Unmarshal(out.Resources["celebrate-settings"][celebrateSettingsKey()], &s); err != nil {
		t.Fatalf("no settings: %v", err)
	}
	return s
}

func hostParty(t *testing.T, cache *Store, mux http.Handler, as string, body map[string]any) (*httptest.ResponseRecorder, string) {
	t.Helper()
	before := map[string]bool{}
	for _, p := range cache.Model().Parties.Parties {
		before[p.ID] = true
	}
	rec := testkit.Call(t, mux, as, "POST", celebratePath("host"), body)
	for _, p := range cache.Model().Parties.Parties {
		if !before[p.ID] {
			return rec, p.ID
		}
	}
	return rec, ""
}

func TestPartiesSampleLoads(t *testing.T) {
	cache, _ := partiesServer(t)
	m := cache.Model().Parties
	if len(m.Celebrations) != 2 || m.Current().Code != "SC-2026" || len(m.Parties) != 16 || len(m.Categories) != 5 {
		t.Fatalf("got %d celebrations (current %v), %d parties, %d categories", len(m.Celebrations), m.Current(), len(m.Parties), len(m.Categories))
	}
	fondue := m.Party("pty0000000001")
	if fondue == nil || fondue.Sold() != 16 || fondue.Remaining() != 54 || !fondue.Hosted(jordan) || fondue.Availability(partiesTestNow()) != Available {
		t.Fatalf("fondue did not load as expected: %+v", fondue)
	}
	cases := map[string]string{"pty0000000006": Waitlist, "pty0000000007": Waitlist, "pty0000000008": SoldOut, "pty0000000010": Past, "pty0000000011": Closed, "pty0000000002": Available}
	for id, want := range cases {
		if got := m.Party(id).Availability(partiesTestNow()); got != want {
			t.Errorf("%s availability %q, want %q", id, got, want)
		}
	}
	if m.Party("pty0000000006").Waiting() != 4 || m.Party("pty0000000006").Full() != true {
		t.Errorf("bagels waitlist %d full %v", m.Party("pty0000000006").Waiting(), m.Party("pty0000000006").Full())
	}
	if admins := cache.Model().AdminList("celebrate"); !admins.IsAdmin(partiesAdmin) || admins.IsAdmin(elena) {
		t.Errorf("admins: %v", admins.Admins())
	}
}

func TestInvoicingNamesAPartyOrAnItem(t *testing.T) {
	partiesServer(t)
	rows := partiesTables(t)
	m, err := BuildParties(context.Background(), rows, partiesBundled)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(m.Invoicing, func(l InvoiceLine) bool { return l.PartyID == "" })
	if i < 0 || m.Invoicing[i].Party != "Head of School for a Day" || m.Invoicing[i].Code != "SC-2026" {
		t.Fatalf("the item line: %+v", m.Invoicing)
	}
	rows[invoicingTab] = append(rows[invoicingTab], store.Row{"Date": "2026-09-10", "Party ID": "pty0000000001", "Party Title": "Fondue", "Celebration": "cbn0000002026", "Purchaser Email": "dana.hawkins@heliosschool.org"})
	if _, err := BuildParties(context.Background(), rows, partiesBundled); err == nil || !strings.Contains(err.Error(), "names both") {
		t.Errorf("a line naming a party and an item loaded: %v", err)
	}
}

func TestRowOrderIsNotTabOrder(t *testing.T) {
	partiesServer(t)
	rows := partiesTables(t)
	slices.Reverse(rows[celebrationsTab])
	slices.Reverse(rows[partyCategoriesTab])
	slices.Reverse(rows[ticketsTab])
	m, err := BuildParties(context.Background(), rows, partiesBundled)
	if err != nil {
		t.Fatal(err)
	}
	if m.Celebrations[0].Code != "SC-2026" || m.Celebrations[1].Code != "SC-2025" {
		t.Errorf("celebrations: %s, %s", m.Celebrations[0].Code, m.Celebrations[1].Code)
	}
	if !slices.Equal(categoryTitles(m), []string{"Family Social", "Adult Social", "Children Social", "Athletic", "Educational"}) {
		t.Errorf("categories: %v", m.Categories)
	}
	tickets := m.Party("pty0000000001").Tickets
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

func TestPartiesBrokenSheetRefusesToLoad(t *testing.T) {
	dir := &data.Dir{Root: "../../sampledata"}
	if err := dir.Delete(partiesAppName, "Categories", map[string]string{}); err != nil {
		t.Fatal(err)
	}
	if err := dir.Insert(partiesAppName, "Categories", []map[string]string{{"Category ID": "pcg0000000001", "Title": "Family Social", "Order": "10"}}); err != nil {
		t.Fatal(err)
	}
	deps := sampleDeps(sampleKey)
	deps.Parties = partiesBundled
	if _, err := NewStore(dir, dir, store.NewQueue(), deps); err == nil || !strings.Contains(err.Error(), "ends in 0") {
		t.Fatalf("a broken sheet loaded: %v", err)
	}
}

func TestOldIDsReachTheirPartyAndTicket(t *testing.T) {
	cache, mux := partiesServer(t)
	m := cache.Model().Parties
	if p := m.Party("P001"); p == nil || p.ID != "pty0000000001" {
		t.Fatalf("P001 reached %+v", p)
	}
	if tk, p := m.TicketByID("t005"); tk == nil || tk.ID != "tkt0000000005" || p == nil || p.ID != "pty0000000001" {
		t.Fatalf("t005 reached %+v on %+v", tk, p)
	}
	if rec := testkit.Call(t, mux, partner, "POST", "/api/tickets/T005/edit", map[string]any{"note": "from an old link"}); rec.Code != http.StatusNoContent {
		t.Fatalf("edit a ticket by its old id: %d %s", rec.Code, rec.Body)
	}
	if tk, _ := cache.Model().Parties.TicketByID("tkt0000000005"); tk.Note != "from an old link" {
		t.Fatalf("after the edit: %+v", tk)
	}
	flags := map[string]any{"ticketsOpen": false, "waitlist": true, "adults": true}
	if rec := testkit.Call(t, mux, elena, "POST", "/api/parties/P002/edit", flags); rec.Code != http.StatusNoContent {
		t.Fatalf("set flags by an old party id: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Parties.Party("pty0000000002").TicketsOpen {
		t.Fatal("the old party id changed nothing")
	}
	keys := map[string]bool{}
	for _, row := range partiesChangeLog(t) {
		keys[row["Key"]] = true
	}
	if !keys["Ticket ID=tkt0000000005"] || !keys["Party ID=pty0000000002"] || len(keys) != 2 {
		t.Fatalf("the writes matched %v", keys)
	}
	rec := testkit.Call(t, mux, jordan, "GET", "/parties/P001", nil)
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/parties/pty0000000001" {
		t.Fatalf("an old party address: %d %v", rec.Code, rec.Header())
	}
	if rec := testkit.Call(t, mux, jordan, "GET", "/parties/pty0000000001", nil); rec.Code != http.StatusOK {
		t.Fatalf("a party address: %d", rec.Code)
	}
}

func TestPartiesRenderHidesWhatItShould(t *testing.T) {
	_, mux := partiesServer(t)
	for _, p := range partiesAs(t, mux, elena) {
		if p.Status != StatusOpen {
			t.Errorf("%s reached a parent as %s", p.Title, p.Status)
		}
		if p.Can["edit"] && p.PartyID != "pty0000000002" && p.PartyID != "pty0000000021" {
			t.Errorf("%s is editable by someone who does not host it", p.Title)
		}
	}
	found := false
	for _, p := range partiesAs(t, mux, "layla.haddad@heliosschool.org") {
		if p.PartyID == "pty0000000013" {
			found = p.Can["edit"] && p.Hosting
		}
	}
	if !found {
		t.Error("a host cannot see or edit their own pending party")
	}
	adminParties := partiesAs(t, mux, partiesAdmin)
	me := decoded[struct {
		Allowances []string `json:"allowances"`
	}](t, testkit.Call(t, mux, partiesAdmin, "GET", "/api/me", nil))
	if len(adminParties) != 16 || !slices.Contains(me.Allowances, SeeAllParties.Name) {
		t.Errorf("admin view: %d parties, allowances %v", len(adminParties), me.Allowances)
	}
	for _, p := range adminParties {
		if !p.Can["edit"] {
			t.Errorf("an admin cannot edit %s", p.Title)
		}
	}
	if got := celebrateSettingsAs(t, mux, jordan).User; got.Name != "Jordan Whitfield" || got.PhotoURL != partiesSampleDirectory.HeroPhoto(jordan) || len(got.Children) != 2 {
		t.Errorf("user %+v", got)
	}
}

func TestAttendeeLines(t *testing.T) {
	_, mux := partiesServer(t)
	fondue := partyAs(t, mux, elena, "pty0000000001")
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
	for _, a := range partyAs(t, mux, partner, "pty0000000001").Attendees {
		if a.Name == "Zander Whitfield (cousin, age 8)" && (!a.Mine || a.Note == "") {
			t.Errorf("the household did not get its own ticket back: %+v", a)
		}
	}
	for _, a := range partyAs(t, mux, partiesKid, "pty0000000001").Attendees {
		if a.Name == "Zander Whitfield (cousin, age 8)" && (a.Mine || a.Note != "" || a.Purchaser != "" || a.Line != "Guest of Jordan Whitfield") {
			t.Errorf("a student saw their family's ticket as theirs: %+v", a)
		}
		if a.Email == partiesKid && !a.Mine {
			t.Errorf("a student did not see their own ticket as theirs: %+v", a)
		}
	}
}

func TestStudentsActForThemselves(t *testing.T) {
	cache, mux := partiesServer(t)
	var guest, own string
	for _, tk := range cache.Model().Parties.Party("pty0000000001").Tickets {
		if tk.Purchaser == jordan && tk.Email == "" {
			guest = tk.ID
		}
		if tk.Email == partiesKid {
			own = tk.ID
		}
	}
	if guest == "" || own == "" {
		t.Fatal("the sample lacks a parent's guest ticket or the student's own ticket on pty0000000001")
	}
	note := func(as, id string) int {
		return testkit.Call(t, mux, as, "POST", "/api/tickets/"+id+"/edit", map[string]any{"note": "see you there"}).Code
	}
	if got := note(partiesKid, guest); got != http.StatusForbidden {
		t.Errorf("a student edited their parent's ticket: %d", got)
	}
	if got := note(partner, guest); got != http.StatusNoContent {
		t.Errorf("a parent could not edit their household's ticket: %d", got)
	}
	if got := note(partiesKid, own); got != http.StatusNoContent {
		t.Errorf("a student could not edit their own ticket: %d", got)
	}
}

func TestBuyTicketsRules(t *testing.T) {
	cache, mux := partiesServer(t)
	buy := func(as, party string, purchaser string, attendees ...map[string]string) *httptest.ResponseRecorder {
		return testkit.Call(t, mux, as, "POST", "/api/parties/"+party+"/buy", map[string]any{"purchaser": purchaser, "attendees": attendees})
	}
	if rec := buy(partiesKid, "pty0000000001", "", map[string]string{"email": partiesKid}); rec.Code != http.StatusForbidden {
		t.Fatalf("a student bought: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, teen, "POST", "/api/parties/pty0000000006/join-waitlist", map[string]any{"quantity": 1}); rec.Code != http.StatusForbidden {
		t.Fatalf("a student joined the waitlist: %d %s", rec.Code, rec.Body)
	}
	var own string
	for _, tk := range cache.Model().Parties.Party("pty0000000001").Tickets {
		if tk.Email == teen {
			own = tk.ID
		}
	}
	if rec := testkit.Call(t, mux, teen, "POST", "/api/tickets/"+own+"/reassign", map[string]any{"email": partiesKid}); rec.Code != http.StatusForbidden {
		t.Fatalf("a student reassigned: %d %s", rec.Code, rec.Body)
	}
	if rec := buy(jordan, "pty0000000002", "", map[string]string{"email": partiesKid}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "for adults") {
		t.Fatalf("a student got an adult ticket: %d %s", rec.Code, rec.Body)
	}
	if rec := buy(jordan, "pty0000000002", "", map[string]string{"email": partner}); rec.Code != http.StatusNoContent {
		t.Fatalf("a parent could not buy for their partner: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Parties.Party("pty0000000002").Sold(); got != 15 {
		t.Fatalf("dink & clink sold %d", got)
	}
	if rec := buy(jordan, "pty0000000002", "", map[string]string{"email": partner}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a second ticket for the same person: %d %s", rec.Code, rec.Body)
	}
	if rec := buy(jordan, "pty0000000002", "", map[string]string{"email": elena}); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent bought for a stranger: %d %s", rec.Code, rec.Body)
	}
	if rec := buy(jordan, "pty0000000002", elena, map[string]string{"email": jordan}); rec.Code != http.StatusForbidden {
		t.Fatalf("billed a stranger: %d %s", rec.Code, rec.Body)
	}
	if rec := buy(partner, "pty0000000003", jordan, map[string]string{"email": partiesKid}); rec.Code != http.StatusNoContent {
		t.Fatalf("a parent could not take a child's ticket: %d %s", rec.Code, rec.Body)
	}
	if rec := buy(jordan, "pty0000000002", "", map[string]string{"name": "Aunt May"}); rec.Code != http.StatusNoContent {
		t.Fatalf("a guest could not take the last ticket: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Parties.Party("pty0000000002").Availability(partiesTestNow()); got != Waitlist {
		t.Fatalf("availability after filling: %q", got)
	}
	if rec := buy(teacher, "pty0000000002", "", map[string]string{"email": teacher}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a full party sold a ticket: %d %s", rec.Code, rec.Body)
	}
	join := func(as, party, purchaser string, quantity int) *httptest.ResponseRecorder {
		return testkit.Call(t, mux, as, "POST", "/api/parties/"+party+"/join-waitlist", map[string]any{"purchaser": purchaser, "quantity": quantity, "note": "any two"})
	}
	if rec := join(teacher, "pty0000000002", elena, 2); rec.Code != http.StatusForbidden {
		t.Fatalf("billed a stranger from the waitlist: %d", rec.Code)
	}
	if rec := join(teacher, "pty0000000002", "", 2); rec.Code != http.StatusNoContent {
		t.Fatalf("join the waitlist: %d %s", rec.Code, rec.Body)
	}
	if rec := join(teacher, "pty0000000002", "", 3); rec.Code != http.StatusNoContent {
		t.Fatalf("change the request: %d %s", rec.Code, rec.Body)
	}
	requests := 0
	for _, tk := range cache.Model().Parties.Party("pty0000000002").Tickets {
		if tk.Status == TicketWaitlist {
			requests++
			if tk.Purchaser != teacher || tk.Quantity != 3 || tk.Note != "any two" {
				t.Fatalf("request: %+v", tk)
			}
		}
	}
	if requests != 1 || cache.Model().Parties.Party("pty0000000002").Waiting() != 3 {
		t.Fatalf("%d requests, waiting %d", requests, cache.Model().Parties.Party("pty0000000002").Waiting())
	}
	if rec := buy(jordan, "pty0000000008", "", map[string]string{"email": jordan}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "sold out") {
		t.Fatalf("sold out: %d %s", rec.Code, rec.Body)
	}
	if rec := buy(elena, "pty0000000011", "", map[string]string{"email": elena}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "closed") {
		t.Fatalf("closed: %d %s", rec.Code, rec.Body)
	}
	if rec := buy("abena.osei@heliosschool.org", "pty0000000008", teacher, map[string]string{"email": teacher}); rec.Code != http.StatusForbidden {
		t.Fatalf("a host billed a stranger: %d %s", rec.Code, rec.Body)
	}
	if rec := buy("abena.osei@heliosschool.org", "pty0000000008", "", map[string]string{"email": teacher}); rec.Code != http.StatusNoContent {
		t.Fatalf("a host could not add to a sold-out party: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Parties.Party("pty0000000008").Sold(); got != 15 {
		t.Fatalf("fruity drinks sold %d after the host added one", got)
	}
	for _, tk := range cache.Model().Parties.Party("pty0000000008").Tickets {
		if tk.Email == teacher && tk.Purchaser != teacher {
			t.Fatalf("the teacher's ticket is billed to %s", tk.Purchaser)
		}
	}
	if rec := buy("mina.park@heliosschool.org", "pty0000000012", "", map[string]string{"email": teen}); rec.Code != http.StatusNoContent {
		t.Fatalf("a host could not add a student: %d %s", rec.Code, rec.Body)
	}
	for _, tk := range cache.Model().Parties.Party("pty0000000012").Tickets {
		if tk.Email == teen && tk.Purchaser != jordan {
			t.Fatalf("the student's ticket is billed to %s, not a parent", tk.Purchaser)
		}
	}
}

func TestFreeTicket(t *testing.T) {
	cache, mux := partiesServer(t)
	const buy = "/api/parties/pty0000000002/buy"
	body := map[string]any{"free": true, "attendees": []map[string]string{{"name": "Percy Jackson"}}}
	if rec := testkit.Call(t, mux, teacher, "POST", buy, body); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger gave a free ticket: %d %s", rec.Code, rec.Body)
	}
	raised := cache.Model().Parties.Party("pty0000000002").Raised()
	if rec := testkit.Call(t, mux, elena, "POST", buy, body); rec.Code != http.StatusNoContent {
		t.Fatalf("the host could not give a free ticket: %d %s", rec.Code, rec.Body)
	}
	p := cache.Model().Parties.Party("pty0000000002")
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
	if rec := testkit.Call(t, mux, elena, "POST", buy, body); rec.Code != http.StatusNoContent {
		t.Fatalf("free ticket with room: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Parties.Party("pty0000000002").Capacity; got != was+1 {
		t.Fatalf("capacity %d, want %d", got, was+1)
	}
	body = map[string]any{"free": true, "purchaser": teen, "attendees": []map[string]string{{"name": "Grover Underwood"}}}
	if rec := testkit.Call(t, mux, elena, "POST", buy, body); rec.Code != http.StatusBadRequest {
		t.Fatalf("a student hosts a guest: %d %s", rec.Code, rec.Body)
	}
	body["purchaser"] = jordan
	if rec := testkit.Call(t, mux, elena, "POST", buy, body); rec.Code != http.StatusNoContent {
		t.Fatalf("free ticket as someone's guest: %d %s", rec.Code, rec.Body)
	}
	for _, tk := range cache.Model().Parties.Party("pty0000000002").Tickets {
		if tk.Name == "Grover Underwood" && (tk.Purchaser != jordan || tk.Price != 0) {
			t.Fatalf("the guest's ticket: %+v", tk)
		}
	}
	ledger := partiesTables(t)[invoicingTab]
	before := len(ledger)
	for _, l := range ledger {
		if l["Guest Name"] == "Percy Jackson" || l["Guest Name"] == "Annabeth Chase" || l["Guest Name"] == "Grover Underwood" {
			t.Fatalf("a free ticket in the ledger: %v", l)
		}
	}
	if rec := testkit.Call(t, mux, elena, "POST", buy, map[string]any{"attendees": []map[string]string{{"name": "Grover Underwood"}}}); rec.Code != http.StatusNoContent {
		t.Fatalf("paid ticket: %d %s", rec.Code, rec.Body)
	}
	ledger = partiesTables(t)[invoicingTab]
	last := ledger[len(ledger)-1]
	if len(ledger) != before+1 || last["Action"] != "ADD" || last["Quantity"] != "1" || last["Cost"] != "75" || last["Purchaser Email"] != elena || last["Guest Name"] != "Grover Underwood" || last["Party ID"] != "pty0000000002" || last["Celebration"] != "cbn0000002026" || last["Invoice"] != "" {
		t.Fatalf("ledger: %v", last)
	}
	if n := len(cache.Model().Parties.Invoicing); n != before+1 {
		t.Fatalf("model ledger %d, want %d", n, before+1)
	}
	if v := celebrateSettingsAs(t, mux, elena); len(v.Invoicing) != 0 {
		t.Fatalf("a parent was shown the ledger")
	}
	if v := celebrateSettingsAs(t, mux, partiesAdmin); len(v.Invoicing) != before+1 {
		t.Fatalf("the admin was not shown the ledger")
	}
	logged := 0
	for _, row := range partiesChangeLog(t) {
		if row["Tab"] == invoicingTab && row["Action"] == "insert" && row["Column"] == "" && row["Previous"] == "" && strings.Contains(row["Key"], "Guest Name=Grover Underwood") {
			logged++
		}
	}
	if logged != 1 {
		t.Fatalf("the ledger row was logged %d times: %v", logged, partiesChangeLog(t))
	}
}

func TestHostingClosed(t *testing.T) {
	cache, mux := partiesServer(t)
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", celebratePath("settings"), map[string]any{"partiesIntro": "x", "ticketNote": "y", "hostingOpen": false}); rec.Code != http.StatusNoContent {
		t.Fatalf("settings: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Parties.Settings.HostingOpen {
		t.Fatal("hosting still open")
	}
	body := map[string]any{"title": "Late Party", "price": 10, "adults": true, "ticketsOpen": true, "waitlist": true, "start": "2026-11-21 18:00"}
	if rec := testkit.Call(t, mux, elena, "POST", celebratePath("host"), body); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent posted while closed: %d %s", rec.Code, rec.Body)
	}
	if rec, made := hostParty(t, cache, mux, partiesAdmin, body); rec.Code != http.StatusNoContent || made == "" {
		t.Fatalf("an admin could not post while closed: %d %s", rec.Code, rec.Body)
	}
	edit := map[string]any{"title": "Dink & Clink!", "price": 75, "adults": true, "ticketsOpen": true, "waitlist": true, "start": "2026-09-26 18:00", "hostEmails": []string{elena, "marco.torres@heliosschool.org"}}
	if rec := testkit.Call(t, mux, elena, "POST", "/api/parties/pty0000000002/edit", edit); rec.Code != http.StatusNoContent {
		t.Fatalf("a host could not edit while closed: %d %s", rec.Code, rec.Body)
	}
}

func TestRemoveAndPromote(t *testing.T) {
	cache, mux := partiesServer(t)
	var waiting string
	for _, tk := range cache.Model().Parties.Party("pty0000000006").Tickets {
		if tk.Email == jordan {
			waiting = tk.ID
		}
	}
	if rec := testkit.Call(t, mux, elena, "DELETE", "/api/tickets/"+waiting, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger removed a ticket: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, elena, "POST", "/api/tickets/"+waiting+"/offer", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger offered tickets: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, "freja.lindqvist@heliosschool.org", "POST", "/api/tickets/"+waiting+"/offer", map[string]any{"quantity": 1}); rec.Code != http.StatusNoContent {
		t.Fatalf("a host could not offer: %d %s", rec.Code, rec.Body)
	}
	if tk, _ := cache.Model().Parties.TicketByID(waiting); tk == nil || tk.Quantity != 1 {
		t.Fatalf("request after one offer: %+v", tk)
	}
	if rec := testkit.Call(t, mux, "freja.lindqvist@heliosschool.org", "POST", "/api/tickets/"+waiting+"/offer", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("a host could not offer the rest: %d %s", rec.Code, rec.Body)
	}
	if tk, _ := cache.Model().Parties.TicketByID(waiting); tk != nil {
		t.Fatal("the request is still there")
	}
	deleted := false
	for _, row := range partiesChangeLog(t) {
		if row["Tab"] == ticketsTab && row["Action"] == "delete" && row["Key"] == "Ticket ID="+waiting && row["Column"] == "Quantity" && row["Previous"] == "1" {
			deleted = true
		}
	}
	if !deleted {
		t.Fatalf("the request's delete was not logged with its last quantity: %v", partiesChangeLog(t))
	}
	offerTo := func(purchaser string) *Ticket {
		for _, tk := range cache.Model().Parties.Party("pty0000000006").Tickets {
			if tk.Status == TicketOffered && tk.Purchaser == purchaser {
				return &tk
			}
		}
		return nil
	}
	if o := offerTo(jordan); o == nil || o.Quantity != 2 || cache.Model().Parties.Party("pty0000000006").Sold() != 8 {
		t.Fatalf("after the offers: offer %+v, sold %d", o, cache.Model().Parties.Party("pty0000000006").Sold())
	}
	if rec := testkit.Call(t, mux, partner, "DELETE", "/api/tickets/"+offerTo(jordan).ID, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("the family withdrew its own offer: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, elena, "POST", "/api/parties/pty0000000006/buy", map[string]any{"attendees": []map[string]any{{"email": elena}}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a family with no offer bought into a full party: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/parties/pty0000000006/buy", map[string]any{"attendees": []map[string]any{{"email": jordan}, {"email": partner}}}); rec.Code != http.StatusNoContent {
		t.Fatalf("the family could not buy what it was offered: %d %s", rec.Code, rec.Body)
	}
	if o := offerTo(jordan); o != nil || cache.Model().Parties.Party("pty0000000006").Sold() != 10 {
		t.Fatalf("after buying the offer: offer %+v, sold %d", o, cache.Model().Parties.Party("pty0000000006").Sold())
	}
	for _, tk := range cache.Model().Parties.Party("pty0000000006").Tickets {
		if tk.Status == TicketSold && tk.Email == jordan {
			waiting = tk.ID
		}
	}
	if rec := testkit.Call(t, mux, "freja.lindqvist@heliosschool.org", "POST", "/api/tickets/tkt0000000058/offer", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("a host could not offer: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, "freja.lindqvist@heliosschool.org", "DELETE", "/api/tickets/"+offerTo("asha.chandra@heliosschool.org").ID, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("a host could not withdraw an offer: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, "asha.chandra@heliosschool.org", "POST", "/api/parties/pty0000000006/buy", map[string]any{"attendees": []map[string]any{{"email": "asha.chandra@heliosschool.org"}}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a withdrawn offer still let the family buy: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, partner, "DELETE", "/api/tickets/"+waiting, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("the family gave a sold ticket back: %d %s", rec.Code, rec.Body)
	}
	var stillWaiting string
	for _, tk := range cache.Model().Parties.Party("pty0000000007").Tickets {
		if tk.Status == TicketWaitlist && tk.Purchaser == jordan {
			stillWaiting = tk.ID
		}
	}
	if rec := testkit.Call(t, mux, jordan, "DELETE", "/api/tickets/"+stillWaiting, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("the family could not leave a waitlist: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, "freja.lindqvist@heliosschool.org", "DELETE", "/api/tickets/"+waiting, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("a host could not remove a ticket: %d %s", rec.Code, rec.Body)
	}
	if tk, _ := cache.Model().Parties.TicketByID(waiting); tk != nil {
		t.Fatal("the ticket is still there")
	}
}

func TestPostAndEditParty(t *testing.T) {
	cache, mux := partiesServer(t)
	body := map[string]any{
		"title": "Board Game Night", "summary": "Games and snacks", "price": 40, "capacity": 12, "start": "2026-11-21 18:00", "end": "2026-11-21 21:00",
		"adults": true, "students": true, "ticketsOpen": true, "waitlist": true, "category": "pcg0000000001", "hosts": "The Torres Family",
	}
	rec, made := hostParty(t, cache, mux, elena, body)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("post a party: %d %s", rec.Code, rec.Body)
	}
	p := cache.Model().Parties.Party(made)
	if p == nil || p.Status != StatusPending || !p.Hosted(elena) || p.Celebration != "cbn0000002026" || p.Price != 40 || p.AddedBy != elena || p.Category != "" {
		t.Fatalf("posted party: %+v", p)
	}
	if got, ok := id.Parse(p.ID); !ok || got != p.ID {
		t.Fatalf("a posted party's id %q is not a minted id", p.ID)
	}
	edit := "/api/parties/" + p.ID + "/edit"
	body["status"] = StatusOpen
	body["title"] = "Board Game Night!"
	body["hostEmails"] = []string{elena, "marco.torres@heliosschool.org"}
	body["needToKnow"], body["noteEmoji"], body["noteTitle"] = "Bring a game", "🎲", "House rules"
	if rec := testkit.Call(t, mux, elena, "POST", edit, body); rec.Code != http.StatusForbidden {
		t.Fatalf("a host approved their own party by editing it: %d %s", rec.Code, rec.Body)
	}
	delete(body, "status")
	delete(body, "category")
	if rec := testkit.Call(t, mux, elena, "POST", edit, body); rec.Code != http.StatusNoContent {
		t.Fatalf("a host could not edit: %d %s", rec.Code, rec.Body)
	}
	p = cache.Model().Parties.Party(p.ID)
	if p.Status != StatusPending || p.Title != "Board Game Night!" || len(p.HostEmails) != 2 || p.NoteEmoji != "🎲" || p.NoteTitle != "House rules" {
		t.Fatalf("after the host's edit: %+v", p)
	}
	body["noteTitle"] = strings.Repeat("x", 61)
	if rec := testkit.Call(t, mux, elena, "POST", edit, body); rec.Code != http.StatusBadRequest {
		t.Fatalf("a long note title was taken: %d", rec.Code)
	}
	body["noteTitle"] = "House rules"
	if rec := testkit.Call(t, mux, "abena.osei@heliosschool.org", "POST", edit, body); rec.Code != http.StatusNotFound {
		t.Fatalf("a stranger edited: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, elena, "POST", "/api/parties/"+p.ID+"/status", map[string]string{"status": StatusOpen}); rec.Code != http.StatusForbidden {
		t.Fatalf("a host approved their own party: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", "/api/parties/"+p.ID+"/status", map[string]string{"status": StatusOpen}); rec.Code != http.StatusNoContent {
		t.Fatalf("an admin could not approve: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, "abena.osei@heliosschool.org", "POST", edit, body); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger edited an open party: %d", rec.Code)
	}
	flags := map[string]any{"ticketsOpen": false, "waitlist": true, "adults": true, "students": true}
	if rec := testkit.Call(t, mux, elena, "POST", edit, flags); rec.Code != http.StatusNoContent {
		t.Fatalf("flags: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Parties.Party(p.ID).Availability(partiesTestNow()); got != Closed {
		t.Fatalf("after closing sales: %q", got)
	}
	flags["adults"], flags["students"] = false, false
	if rec := testkit.Call(t, mux, elena, "POST", edit, flags); rec.Code != http.StatusBadRequest {
		t.Fatalf("a party for nobody: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "DELETE", "/api/parties/pty0000000001", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("deleted a party with tickets: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "DELETE", "/api/parties/"+p.ID, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Parties.Party(p.ID) != nil {
		t.Fatal("the party is still there")
	}
}

func TestPriceIsFixedOnceBought(t *testing.T) {
	cache, mux := partiesServer(t)
	body := map[string]any{
		"title": "Pie Night", "price": 40, "capacity": 12, "start": "2026-11-21 18:00", "end": "2026-11-21 21:00",
		"adults": true, "ticketsOpen": true, "status": StatusOpen,
	}
	rec, made := hostParty(t, cache, mux, partiesAdmin, body)
	if rec.Code != http.StatusNoContent || made == "" {
		t.Fatalf("post a party: %d %s", rec.Code, rec.Body)
	}
	edit := "/api/parties/" + made + "/edit"
	body["price"] = 45
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", edit, body); rec.Code != http.StatusNoContent {
		t.Fatalf("the price could not change before a sale: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/parties/"+made+"/buy", map[string]any{"attendees": []map[string]string{{"email": jordan}}}); rec.Code != http.StatusNoContent {
		t.Fatalf("buy: %d %s", rec.Code, rec.Body)
	}
	body["price"] = 50
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", edit, body); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "price") {
		t.Fatalf("the price changed after a sale: %d %s", rec.Code, rec.Body)
	}
	body["price"] = 45
	body["title"] = "Pie Night!"
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", edit, body); rec.Code != http.StatusNoContent {
		t.Fatalf("an edit that keeps the price was refused: %d %s", rec.Code, rec.Body)
	}
	if p := cache.Model().Parties.Party(made); p.Price != 45 || p.Title != "Pie Night!" {
		t.Fatalf("after the edits: %+v", p)
	}
}

func TestDeletingAPartyTakesItsHosts(t *testing.T) {
	cache, mux := partiesServer(t)
	body := map[string]any{"title": "Trivia Night", "price": 20, "adults": true, "ticketsOpen": true, "hostEmails": []string{elena, teacher}}
	rec, made := hostParty(t, cache, mux, partiesAdmin, body)
	if rec.Code != http.StatusNoContent || made == "" {
		t.Fatalf("post: %d %s", rec.Code, rec.Body)
	}
	if n := cache.Count(partiesAppName, hostsTab, store.Row{"Party ID": made}); n != 2 {
		t.Fatalf("%d hosts after posting", n)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "DELETE", "/api/parties/"+made, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if n := cache.Count(partiesAppName, hostsTab, store.Row{"Party ID": made}); n != 0 {
		t.Fatalf("%d hosts outlived their party in memory", n)
	}
	for _, row := range partiesTables(t)[hostsTab] {
		if row["Party ID"] == made {
			t.Fatalf("the sheet kept %v", row)
		}
	}
	gone := map[string]bool{}
	for _, row := range partiesChangeLog(t) {
		if row["Tab"] == hostsTab && row["Action"] == "delete" && row["Column"] == "Email" {
			gone[row["Previous"]] = true
		}
	}
	if !gone[elena] || !gone[teacher] {
		t.Fatalf("the hosts' delete was not logged: %v", partiesChangeLog(t))
	}
}

func TestCategoriesAndCelebrations(t *testing.T) {
	cache, mux := partiesServer(t)
	const adultSocial = "pcg0000000002"
	filed := cache.Count(partiesAppName, partiesTab, store.Row{"Category": adultSocial})
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", "/api/party-categories/"+adultSocial+"/edit", map[string]string{"title": "Grown-Ups"}); rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	m := cache.Model().Parties
	if c := m.Category(adultSocial); c == nil || c.Title != "Grown-Ups" || m.Party("pty0000000002").Category != adultSocial {
		t.Fatalf("after the rename: %+v, the party files under %q", c, m.Party("pty0000000002").Category)
	}
	if filed == 0 || cache.Count(partiesAppName, partiesTab, store.Row{"Category": adultSocial}) != filed {
		t.Fatalf("%d parties were filed under the category, %d after its rename", filed, cache.Count(partiesAppName, partiesTab, store.Row{"Category": adultSocial}))
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", "/api/party-categories/pcg0000000001/edit", map[string]string{"title": "Grown-Ups"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("two categories took one title: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "DELETE", "/api/party-categories/"+adultSocial, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("deleted a category in use: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", "/api/celebrations", map[string]any{"code": "SC-2026", "title": "Twice", "start": "2027-03-06 17:30"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("added a celebration with a code another has: %d %s", rec.Code, rec.Body)
	}
	rec := testkit.Call(t, mux, partiesAdmin, "POST", "/api/celebrations", map[string]any{"code": "SC-2027", "title": "Helios Spring Celebration 2027", "start": "2027-03-06 17:30", "current": true})
	if rec.Code != http.StatusOK {
		t.Fatalf("add celebration: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model().Parties
	next := m.Celebration("SC-2027")
	if next == nil || m.Current() != next || m.Celebration("SC-2026").Current || created(t, rec) != next.ID {
		t.Fatalf("current after adding: %s", m.Current().Code)
	}
	if got, ok := id.Parse(next.ID); !ok || got != next.ID || m.CelebrationByID(next.ID) != next {
		t.Fatalf("a new celebration's id %q is not a minted id", next.ID)
	}
	celebration := func(key string) string { return "/api/celebrations/" + key + "/edit" }
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", celebration(next.ID), map[string]any{"code": "SC-2027", "title": "Helios Spring Celebration 2027", "start": "", "buttonText": "Save the Date", "buttonUrl": "calendar", "current": true}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a calendar button with no date: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", celebration(next.ID), map[string]any{"code": "SC-2027", "title": "Helios Spring Celebration 2027", "start": "2027-03-06 17:30", "buttonText": "Save the Date", "buttonUrl": "calendar", "current": true}); rec.Code != http.StatusNoContent {
		t.Fatalf("a calendar button: %d %s", rec.Code, rec.Body)
	}
	if m = cache.Model().Parties; m.CelebrationByID(next.ID).ButtonURL != ButtonCalendar {
		t.Fatalf("button: %q", m.CelebrationByID(next.ID).ButtonURL)
	}
	if m.Banner().ID != next.ID {
		t.Fatalf("banner follows current: %s", m.Banner().Code)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", celebration("cbn0000002026"), map[string]any{"code": "SC-2026", "title": "Helios Spring Celebration 2026", "start": "2026-03-07 17:30", "banner": true}); rec.Code != http.StatusNoContent {
		t.Fatalf("mark banner: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model().Parties
	if m.Banner().Code != "SC-2026" || m.Current().Code != "SC-2027" {
		t.Fatalf("banner %s current %s", m.Banner().Code, m.Current().Code)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", celebration(next.ID), map[string]any{"code": "SC-2027", "title": "Helios Spring Celebration 2027", "start": "2027-03-06 17:30", "current": true, "banner": true}); rec.Code != http.StatusNoContent {
		t.Fatalf("move banner: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model().Parties
	if m.Banner().Code != "SC-2027" || m.Celebration("SC-2026").Banner {
		t.Fatalf("banner did not move: %s", m.Banner().Code)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "DELETE", "/api/celebrations/cbn0000002026", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("deleted a celebration with parties: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", celebration("cbn0000002025"), map[string]any{"code": "SC-2026", "title": "Helios Spring Celebration 2025", "start": "2025-03-01 17:30"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("took another celebration's code: %d %s", rec.Code, rec.Body)
	}
	parties := len(m.SortedParties("cbn0000002025"))
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", celebration("cbn0000002025"), map[string]any{"code": "SC-2025B", "title": "Helios Spring Celebration 2025", "start": "2025-03-01 17:30"}); rec.Code != http.StatusNoContent {
		t.Fatalf("change a celebration's code: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model().Parties
	if c := m.Celebration("SC-2025B"); parties == 0 || c == nil || c.ID != "cbn0000002025" || m.Celebration("SC-2025") != nil || len(m.SortedParties("cbn0000002025")) != parties {
		t.Fatalf("after changing the code: %+v, %d of %d parties still filed", c, len(m.SortedParties("cbn0000002025")), parties)
	}
	for _, l := range m.Invoicing {
		if l.Celebration == "cbn0000002025" && (l.Code != "SC-2025B" || l.Party != "Wines of the Southern Hemisphere!" && l.Party != "Splash into the New Year") {
			t.Fatalf("a ledger line after the code changed: %+v", l)
		}
	}
	for _, row := range partiesChangeLog(t) {
		if row["Tab"] == partiesTab || row["Tab"] == invoicingTab {
			t.Fatalf("a rename or code change rewrote %v", row)
		}
	}
	rec = testkit.Call(t, mux, partiesAdmin, "GET", "/api/celebrate/invoices.csv?celebration=cbn0000002025", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Wines of the Southern Hemisphere!,SC-2025B,") || strings.Contains(rec.Body.String(), "Fondue") {
		t.Fatalf("invoices: %d %s", rec.Code, rec.Body)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "DELETE", "/api/celebrations/"+next.ID, nil); rec.Code != http.StatusNoContent || cache.Model().Parties.CelebrationByID(next.ID) != nil {
		t.Fatalf("delete an empty celebration: %d %s", rec.Code, rec.Body)
	}
	for _, code := range []string{"SC-2025B", `x"; filename*=UTF-8''invoice.html`} {
		if rec := testkit.Call(t, mux, partiesAdmin, "GET", "/api/celebrate/invoices.csv?celebration="+url.QueryEscape(code), nil); rec.Code != http.StatusNotFound {
			t.Fatalf("invoices for %q: %d", code, rec.Code)
		}
	}
	if rec := testkit.Call(t, mux, elena, "GET", "/api/celebrate/invoices.csv", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent read the invoices: %d", rec.Code)
	}
}

func TestReorderCategoriesWritesOneKey(t *testing.T) {
	cache, mux := partiesServer(t)
	order := []string{"pcg0000000005", "pcg0000000001", "pcg0000000002", "pcg0000000003", "pcg0000000004"}
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", celebratePath("order-categories"), map[string]any{"ids": order}); rec.Code != http.StatusNoContent {
		t.Fatalf("reorder: %d %s", rec.Code, rec.Body)
	}
	if got := categoryTitles(cache.Model().Parties); !slices.Equal(got, []string{"Educational", "Family Social", "Adult Social", "Children Social", "Athletic"}) {
		t.Fatalf("categories: %v", got)
	}
	moved := []store.Row{}
	for _, row := range partiesChangeLog(t) {
		if row["Tab"] == partyCategoriesTab {
			moved = append(moved, row)
		}
	}
	if len(moved) != 1 || moved[0]["Key"] != "Category ID=pcg0000000005" || moved[0]["Column"] != store.OrderColumn || moved[0]["Previous"] != "9" || moved[0]["Action"] != "set" {
		t.Fatalf("logged %v", moved)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", celebratePath("order-categories"), map[string]any{"ids": order[1:]}); rec.Code != http.StatusBadRequest {
		t.Fatalf("an order missing a category: %d", rec.Code)
	}
	rec := testkit.Call(t, mux, partiesAdmin, "POST", "/api/party-categories", map[string]string{"title": "Musical"})
	if rec.Code != http.StatusOK {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	got := cache.Model().Parties.Categories
	musical := got[len(got)-1]
	if key, ok := id.Parse(musical.ID); musical.Title != "Musical" || !ok || key != musical.ID || created(t, rec) != musical.ID {
		t.Fatalf("a new category did not land last with a minted id: %v", got)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "DELETE", "/api/party-categories/"+musical.ID, nil); rec.Code != http.StatusNoContent || cache.Model().Parties.Category(musical.ID) != nil {
		t.Fatalf("delete an empty category: %d %s", rec.Code, rec.Body)
	}
}

func TestPastAndPrices(t *testing.T) {
	p := &Party{Start: "2026-09-11", Status: StatusOpen, TicketsOpen: true}
	if !p.Past(partiesTestNow()) {
		t.Error("yesterday's all-day party is not past")
	}
	p.Start = "2026-09-12"
	if p.Past(partiesTestNow()) {
		t.Error("today's all-day party is already past")
	}
	p.Start, p.End = "2026-09-12 09:00", "2026-09-12 11:00"
	if !p.Past(partiesTestNow()) {
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
	cache, mux := partiesServer(t)
	m := cache.Model().Parties
	if m.PathOf(m.Party("pty0000000001")) != "/p/fondue" || m.PathOf(m.Party("pty0000000005")) != "/parties/pty0000000005" {
		t.Fatalf("paths: %s %s", m.PathOf(m.Party("pty0000000001")), m.PathOf(m.Party("pty0000000005")))
	}
	for path, want := range map[string]string{"/p/fondue": "pty0000000001", "/p/Fondue/": "pty0000000001", "/parties/pty0000000001": "pty0000000001", "/parties/P001": "pty0000000001", "/p/fondue-night": "pty0000000001", "pickleball": "pty0000000002", "/p/nothing": ""} {
		got := ""
		if p := m.Resolve(path); p != nil {
			got = p.ID
		}
		if got != want {
			t.Errorf("Resolve(%q) = %q, want %q", path, got, want)
		}
	}
	const edit = "/api/parties/pty0000000003/edit"
	body := map[string]any{"title": "K-Pop for a Cause!", "price": 50, "capacity": 20, "adults": true, "students": true, "ticketsOpen": true, "waitlist": true, "prettyId": "Fondue", "hostEmails": []string{"deepa.natarajan@heliosschool.org"}}
	rec := testkit.Call(t, mux, "deepa.natarajan@heliosschool.org", "POST", edit, body)
	var conflict partyPrettyConflict
	if rec.Code != http.StatusConflict || json.Unmarshal(rec.Body.Bytes(), &conflict) != nil || conflict.ID != "pty0000000001" || !strings.Contains(conflict.Message, "is already the address of") {
		t.Fatalf("took another party's address: %d %s", rec.Code, rec.Body)
	}
	body["prettyId"] = "k pop!"
	if rec := testkit.Call(t, mux, "deepa.natarajan@heliosschool.org", "POST", edit, body); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "friendly address") {
		t.Fatalf("took a malformed address: %d %s", rec.Code, rec.Body)
	}
	body["prettyId"] = "k-pop"
	if rec := testkit.Call(t, mux, "deepa.natarajan@heliosschool.org", "POST", edit, body); rec.Code != http.StatusNoContent {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body)
	}
	m = cache.Model().Parties
	if got := m.Resolve("/p/kpop"); got == nil || got.ID != "pty0000000003" {
		t.Fatalf("the old address no longer resolves: %v", got)
	}
	if m.PathOf(m.Party("pty0000000003")) != "/p/k-pop" {
		t.Fatalf("path after rename: %s", m.PathOf(m.Party("pty0000000003")))
	}
	written := false
	for _, row := range partiesTables(t)[partyRedirectsTab] {
		if row["Old"] == "/p/kpop" && row["New"] == "/p/k-pop" && row["Type"] == RedirectParty && row["Date"] == "2026-09-12" {
			written = true
		}
	}
	if !written {
		t.Fatalf("the redirect is not in the sheet: %v", partiesTables(t)[partyRedirectsTab])
	}
	logged := false
	for _, row := range partiesChangeLog(t) {
		if row["Tab"] == partyRedirectsTab && row["Action"] == "insert" && row["Key"] == "Old=/p/kpop" && row["Actor"] == "deepa.natarajan@heliosschool.org" {
			logged = true
		}
	}
	if !logged {
		t.Fatalf("the redirect was not logged: %v", partiesChangeLog(t))
	}
	body["title"] = "K-Pop for a Good Cause!"
	before := len(partiesTables(t)[partyRedirectsTab])
	if rec := testkit.Call(t, mux, "deepa.natarajan@heliosschool.org", "POST", edit, body); rec.Code != http.StatusNoContent {
		t.Fatalf("retitle: %d %s", rec.Code, rec.Body)
	}
	if after := len(partiesTables(t)[partyRedirectsTab]); after != before {
		t.Fatalf("a retitle left a redirect: %d then %d", before, after)
	}
}

func TestPartiesSharePreview(t *testing.T) {
	cache, mux := partiesServer(t)
	previews := []testkit.Preview{{
		URL: "https://celebrate.heliosian.com/p/fondue",
		Want: []string{`og:title" content="Fondue &amp; Fort Night"`, `og:url" content="https://celebrate.heliosian.com/p/fondue"`,
			`og:image" content="https://celebrate.heliosian.com/open/share/pty0000000001.png"`, `Saturday, September 19 · 5:00 – 9:00 PM — The Parks&#39; House in Los Altos — A cozy evening`},
		Never: []string{"Alder"},
	}}
	for _, path := range []string{"/", "/parties/pty0000000013", "/my", "/p/nothing"} {
		previews = append(previews, testkit.Preview{
			URL: "https://celebrate.heliosian.com" + path,
			Want: []string{`og:title" content="Upcoming Parties"`, `og:url" content="https://celebrate.heliosian.com/"`,
				`og:image" content="https://celebrate.heliosian.com/open/share/upcoming.png"`,
				`Next up: Fondue &amp; Fort Night — Saturday, September 19 · 5:00 – 9:00 PM — The Parks&#39; House in Los Altos. Also coming: Dink &amp; Clink (Sep 26), K-Pop for a Cause! (Sep 27), Wurst Helios Party (Oct 3).`},
			Never: []string{"Alder", "Baegels", "Backyard", "Crochet"},
		})
	}
	testkit.Previews(t, PartiesPreviewHead(cache, partiesTestStyle), previews...)
	ids := []string{}
	for _, p := range upcoming(cache.Model().Parties) {
		ids = append(ids, p.ID)
	}
	if strings.Join(ids, " ") != "pty0000000001 pty0000000002 pty0000000003 pty0000000004 pty0000000012 pty0000000005 pty0000000009" {
		t.Errorf("upcoming: %v", ids)
	}
	testkit.Cards(t, mux, []string{"/open/share/pty0000000001.png", "/open/share/upcoming.png"}, "/open/share/pty0000000013.png")
}

func TestPartiesMail(t *testing.T) {
	rec := mailtest.NewRecorder(mailtest.From)
	cache, mux := partiesServeWith(t, rec.Mailgun)
	buy := func(as, party, purchaser string, attendees ...map[string]string) *httptest.ResponseRecorder {
		return testkit.Call(t, mux, as, "POST", "/api/parties/"+party+"/buy", map[string]any{"purchaser": purchaser, "note": "We\u2019ll be a little late", "attendees": attendees})
	}
	if r := buy(partner, "pty0000000004", jordan, map[string]string{"email": partner}, map[string]string{"name": "Aunt May"}); r.Code != http.StatusNoContent {
		t.Fatalf("buy: %d %s", r.Code, r.Body)
	}
	pair := func() (note, invite mail.Message) {
		for range 2 {
			m := rec.Next(t)
			if len(m.Attachments) > 0 {
				invite = m
			} else {
				note = m
			}
		}
		return note, invite
	}
	m, invite := pair()
	if m.Subject != "Your tickets to Wurst Helios Party" || !slices.Equal(m.To, []string{jordan}) || !slices.Equal(m.CC, []string{partner, "sofia.marchetti@heliosschool.org", "paolo.marchetti@heliosschool.org"}) || len(m.Attachments) != 0 {
		t.Fatalf("confirmation: %+v", m)
	}
	for _, want := range []string{"Hi Jordan", "Robin Whitfield, Aunt May", "$150 (2 × $75)", "Robin Whitfield took them", "/open/share/pty0000000004.png", "88 Castro Street", "invoiced by Helios", "calendar.google.com/calendar/render?action=TEMPLATE", "Add to Calendar"} {
		if !strings.Contains(m.HTML, want) {
			t.Errorf("confirmation lacks %q", want)
		}
	}
	if !slices.Equal(m.ReplyTo, []string{"sofia.marchetti@heliosschool.org", "paolo.marchetti@heliosschool.org"}) {
		t.Errorf("reply-to: %v", m.ReplyTo)
	}
	if invite.Subject != "Calendar invite: Wurst Helios Party" || !slices.Equal(invite.To, []string{jordan}) || len(invite.CC) != 0 || !slices.Equal(invite.ReplyTo, m.ReplyTo) || !strings.Contains(invite.HTML, "Add it to your calendar") {
		t.Fatalf("invite note: %+v", invite)
	}
	ics := strings.ReplaceAll(string(invite.Attachments[0].Content), "\r\n ", "")
	if invite.Attachments[0].Name != "invite.ics" || !strings.HasPrefix(invite.Attachments[0].ContentType, "text/calendar; method=REQUEST") {
		t.Fatalf("invite: %+v", invite.Attachments[0])
	}
	for _, want := range []string{"METHOD:REQUEST", "UID:celebrate-pty0000000004-" + jordan + "@heliosian.com", "SUMMARY:Wurst Helios Party", "DTSTART:20261003T", "ORGANIZER;CN=Helios Celebrate:mailto:" + mail.AddressOf(mailtest.From), "ATTENDEE;CN=Jordan Whitfield;ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED;RSVP=FALSE:mailto:" + jordan, "LOCATION:88 Castro Street"} {
		if !strings.Contains(ics, want) {
			t.Errorf("invite lacks %q:\n%s", want, ics)
		}
	}
	if strings.Contains(ics, "mailto:sofia.marchetti") {
		t.Errorf("a host is on the family's invite:\n%s", ics)
	}
	if r := buy(jordan, "pty0000000003", "", map[string]string{"email": partiesKid}); r.Code != http.StatusNoContent {
		t.Fatalf("buy for a child: %d %s", r.Code, r.Body)
	}
	m, invite = pair()
	if m.Subject != "Sam Whitfield's ticket to K-Pop for a Cause!" || !slices.Equal(m.To, []string{jordan, partner}) || !slices.Contains(m.CC, "deepa.natarajan@heliosschool.org") {
		t.Fatalf("a child's note: %+v", m)
	}
	for _, want := range []string{"Sam, you&#39;re going!", "Hi Sam - you have a ticket"} {
		if !strings.Contains(m.HTML, want) {
			t.Errorf("a child's note lacks %q", want)
		}
	}
	if ics := string(invite.Attachments[0].Content); !slices.Equal(invite.To, []string{jordan, partner}) || !strings.Contains(ics, "mailto:"+jordan) || !strings.Contains(ics, "mailto:"+partner) {
		t.Errorf("a child's invite: to %v\n%s", invite.To, ics)
	}
	if r := testkit.Call(t, mux, "sofia.marchetti@heliosschool.org", "POST", "/api/parties/pty0000000004/buy", map[string]any{"free": true, "attendees": []map[string]string{{"name": "Peter Parker"}}}); r.Code != http.StatusNoContent {
		t.Fatalf("free: %d %s", r.Code, r.Body)
	}
	m, invite = pair()
	if !strings.Contains(m.HTML, "no charge") || strings.Contains(m.HTML, "invoiced") || !strings.Contains(m.HTML, "Add to Calendar") {
		t.Fatalf("free ticket note: %s", m.HTML)
	}
	if !slices.Equal(m.CC, []string{"paolo.marchetti@heliosschool.org"}) || !slices.Equal(invite.To, []string{"sofia.marchetti@heliosschool.org"}) || len(invite.CC) != 0 {
		t.Fatalf("free ticket note cc %v / invite to %v cc %v", m.CC, invite.To, invite.CC)
	}
	if r := testkit.Call(t, mux, "sofia.marchetti@heliosschool.org", "POST", "/api/parties/pty0000000004/buy", map[string]any{"free": true, "attendees": []map[string]string{{"name": "Michael Bolin", "email": "mbolin@example.com"}}}); r.Code != http.StatusNoContent {
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
	if r := testkit.Call(t, mux, "sofia.marchetti@heliosschool.org", "POST", "/api/parties/pty0000000004/buy", map[string]any{"invoice": jordan, "attendees": []map[string]string{{"email": elena}}}); r.Code != http.StatusNoContent {
		t.Fatalf("a host's paid ticket: %d %s", r.Code, r.Body)
	}
	m, _ = pair()
	if !slices.Equal(m.To, []string{jordan, elena}) || !slices.Contains(m.CC, "sofia.marchetti@heliosschool.org") {
		t.Fatalf("a host's paid ticket goes to %v cc %v, want the invoiced adult and the holder, the host copied", m.To, m.CC)
	}
	for _, want := range []string{"Billed to", "Jordan Whitfield (" + jordan + ")", "$75 (1 × $75)"} {
		if !strings.Contains(m.HTML, want) {
			t.Errorf("a host's paid ticket note lacks %q", want)
		}
	}
	if !slices.ContainsFunc(cache.Model().Parties.Invoicing, func(l InvoiceLine) bool {
		return l.PartyID == "pty0000000004" && l.Purchaser == jordan && l.Guest == "Elena Torres" && l.Cost == 75 && l.Action == "ADD"
	}) {
		t.Errorf("a host's paid ticket left no invoicing line billed to the chosen adult: %+v", cache.Model().Parties.Invoicing)
	}
	if r := testkit.Call(t, mux, teacher, "POST", "/api/parties/pty0000000006/join-waitlist", map[string]any{"quantity": 2, "note": "either day works"}); r.Code != http.StatusNoContent {
		t.Fatalf("waitlist: %d %s", r.Code, r.Body)
	}
	byTo := map[string]mail.Message{}
	for range 2 {
		m = rec.Next(t)
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
	for _, tk := range cache.Model().Parties.Party("pty0000000006").Tickets {
		if tk.Status == TicketWaitlist && tk.Purchaser == jordan {
			waiting = tk.ID
		}
	}
	if r := testkit.Call(t, mux, "freja.lindqvist@heliosschool.org", "POST", "/api/tickets/"+waiting+"/offer", nil); r.Code != http.StatusNoContent {
		t.Fatalf("offer: %d %s", r.Code, r.Body)
	}
	m = rec.Next(t)
	if m.Subject != "A place opened up: 2 tickets to Baegels and Meimosas" || !slices.Equal(m.To, []string{jordan}) || !slices.Equal(m.CC, []string{"freja.lindqvist@heliosschool.org", "anders.lindqvist@heliosschool.org"}) || !strings.Contains(m.HTML, "Freja Lindqvist has offered") || !strings.Contains(m.HTML, "nothing is billed until you do") || len(m.Attachments) != 0 {
		t.Fatalf("offer note: %+v", m)
	}
}

func TestOutsidePurchaserGoesByTheirTicketsName(t *testing.T) {
	cache, _ := partiesServer(t)
	d := cache.Model().Directory
	outside := "ahappyvillage@gmail.com"
	p := &Party{Tickets: []Ticket{
		{ID: "tkt0000000801", Email: outside, Name: "Elaine Tse", Purchaser: outside, Status: TicketSold},
		{ID: "tkt0000000802", Name: "Pat Guest", Purchaser: outside, Status: TicketSold},
	}}
	if got := p.PurchaserName(d, outside); got != "Elaine Tse" {
		t.Fatalf("outside purchaser named %q", got)
	}
	if got := p.PurchaserName(d, jordan); got != nameOf(d, jordan) {
		t.Fatalf("directory purchaser named %q", got)
	}
	if a := (partyViewer{directory: d}).attendee(p, p.Tickets[1], true); a.PurchaserName != "Elaine Tse" || a.Line != "Guest of Elaine Tse" {
		t.Fatalf("guest of an outside purchaser: %+v", a)
	}
}

func TestHolderGroupsWithTheirOwnFamily(t *testing.T) {
	cache, _ := partiesServer(t)
	d := cache.Model().Directory
	mine, ok := d.FamilyOf(partner)
	theirs, ok2 := d.FamilyOf(elena)
	if !ok || !ok2 || mine.Key == theirs.Key {
		t.Fatalf("sample families: %v %v", mine.Key, theirs.Key)
	}
	p := &Party{Tickets: []Ticket{
		{ID: "tkt0000000811", Email: partner, Purchaser: elena, Status: TicketSold},
		{ID: "tkt0000000812", Name: "Pat Guest", Purchaser: elena, Status: TicketSold},
	}}
	v := partyViewer{directory: d}
	if a := v.attendee(p, p.Tickets[0], true); a.FamilyKey != mine.Key || a.Household != mine.Key {
		t.Fatalf("a holder given a ticket by another family grouped as %q, want %q", a.FamilyKey, mine.Key)
	}
	if a := v.attendee(p, p.Tickets[1], true); a.FamilyKey != theirs.Key {
		t.Fatalf("a guest grouped as %q, want the buyer's %q", a.FamilyKey, theirs.Key)
	}
}

func TestReassign(t *testing.T) {
	cache, mux := partiesServer(t)
	var mine string
	for _, tk := range cache.Model().Parties.Party("pty0000000001").Tickets {
		if tk.Email == teen {
			mine = tk.ID
		}
	}
	reassign := "/api/tickets/" + mine + "/reassign"
	if rec := testkit.Call(t, mux, elena, "POST", reassign, map[string]any{"name": "Percy Jackson"}); rec.Code != http.StatusForbidden {
		t.Fatalf("a stranger reassigned: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, partner, "POST", reassign, map[string]any{"name": "Percy Jackson", "email": "percy.jackson@gmail.com"}); rec.Code != http.StatusNoContent {
		t.Fatalf("reassign to a guest: %d %s", rec.Code, rec.Body)
	}
	tk, _ := cache.Model().Parties.TicketByID(mine)
	if tk.Name != "Percy Jackson" || tk.Email != "percy.jackson@gmail.com" || tk.Purchaser != jordan || tk.Price != 65 || tk.Status != TicketSold {
		t.Fatalf("after reassign: %+v", tk)
	}
	for _, other := range cache.Model().Parties.Party("pty0000000001").Tickets {
		if other.Email == teen {
			t.Fatal("Ella is still on the list")
		}
	}
	previous := map[string]string{}
	for _, row := range partiesChangeLog(t) {
		if row["Key"] == "Ticket ID="+mine {
			previous[row["Column"]] = row["Previous"]
		}
	}
	if len(previous) != 2 || previous["Email"] != teen || previous["Name"] != "" {
		t.Fatalf("reassign logged %v", previous)
	}
	if rec := testkit.Call(t, mux, "mina.park@heliosschool.org", "POST", reassign, map[string]any{"email": teacher}); rec.Code != http.StatusNoContent {
		t.Fatalf("a host could not reassign: %d %s", rec.Code, rec.Body)
	}
	tk, _ = cache.Model().Parties.TicketByID(mine)
	if tk.Email != teacher || tk.Name != "" || tk.Purchaser != jordan {
		t.Fatalf("after the host's reassign: %+v", tk)
	}
	if rec := testkit.Call(t, mux, "mina.park@heliosschool.org", "POST", reassign, map[string]any{"email": jordan}); rec.Code != http.StatusBadRequest {
		t.Fatalf("reassigned to someone with a ticket: %d", rec.Code)
	}
	var adults string
	for _, tk := range cache.Model().Parties.Party("pty0000000002").Tickets {
		if tk.Email == jordan {
			adults = tk.ID
		}
	}
	if rec := testkit.Call(t, mux, jordan, "POST", "/api/tickets/"+adults+"/reassign", map[string]any{"email": partiesKid}); rec.Code != http.StatusBadRequest {
		t.Fatalf("reassigned an adults-only ticket to a child: %d", rec.Code)
	}
}

func TestReassignMail(t *testing.T) {
	rec := mailtest.NewRecorder(mailtest.From)
	cache, mux := partiesServeWith(t, rec.Mailgun)
	p := cache.Model().Parties.Party("pty0000000001")
	var mine string
	for _, tk := range p.Tickets {
		if tk.Email == teen {
			mine = tk.ID
		}
	}
	if r := testkit.Call(t, mux, partner, "POST", "/api/tickets/"+mine+"/reassign", map[string]any{"name": "Percy Jackson", "email": "percy.jackson@gmail.com"}); r.Code != http.StatusNoContent {
		t.Fatalf("reassign: %d %s", r.Code, r.Body)
	}
	m := rec.Next(t)
	if m.Subject != "Percy Jackson's ticket to "+p.Title || !slices.Equal(m.To, []string{"percy.jackson@gmail.com"}) || !slices.Equal(m.CC, append([]string{partner}, p.HostEmails...)) || !slices.Equal(m.ReplyTo, p.HostEmails) {
		t.Fatalf("reassign note: %s to %v cc %v reply-to %v", m.Subject, m.To, m.CC, m.ReplyTo)
	}
	for _, want := range []string{"Percy, you", "Hi Percy", "Robin Whitfield has passed you a ticket"} {
		if !strings.Contains(m.HTML, want) {
			t.Errorf("reassign note lacks %q", want)
		}
	}
}

func TestMoveAddress(t *testing.T) {
	cache, mux := partiesServer(t)
	const (
		school = "ella.graduated@heliosschool.org"
		home   = "ella.w@gmail.com"
		later  = "ella.whitfield@college.edu"
	)
	if err := cache.Commit(context.Background(), access.System("test"), CalendarApp, store.Insert(InvitesTab, store.Row{"Event ID": "pty0000000001", "Email": school, "Name": "Ella Graduated", "Token": "schooltoken"})); err != nil {
		t.Fatal(err)
	}
	var held string
	for _, tk := range cache.Model().Parties.Party("pty0000000001").Tickets {
		if tk.Email == teen {
			held = tk.ID
		}
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", "/api/tickets/"+held+"/reassign", map[string]any{"email": school}); rec.Code != http.StatusNoContent {
		t.Fatalf("set up the alum's ticket: %d %s", rec.Code, rec.Body)
	}
	move := celebratePath("move-address")
	body := map[string]any{"old": school, "to": home, "name": "Ella Whitfield"}
	if rec := testkit.Call(t, mux, jordan, "POST", move, body); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent moved an address: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", move, map[string]any{"old": jordan, "to": home}); rec.Code != http.StatusBadRequest {
		t.Fatalf("moved an address the directory holds: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", move, body); rec.Code != http.StatusNoContent {
		t.Fatalf("move: %d %s", rec.Code, rec.Body)
	}
	tk, _ := cache.Model().Parties.TicketByID(held)
	if tk.Email != home || tk.Name != "Ella Whitfield" || tk.Purchaser != jordan {
		t.Fatalf("after the move: %+v", tk)
	}
	if moved := cache.Model().Calendar.InviteOf("pty0000000001", home); moved == nil || cache.Model().Calendar.InviteOf("pty0000000001", school) != nil || moved.Token != "schooltoken" || moved.Name != "Ella Whitfield" {
		t.Fatalf("When's invite after the move: %+v, old %+v", moved, cache.Model().Calendar.InviteOf("pty0000000001", school))
	}
	moves := 0
	for _, row := range testkit.ChangeLog(t, sheet, queue, CalendarApp) {
		if row["Tab"] == InvitesTab && row["Column"] == "Email" && row["Previous"] == school {
			moves++
			if row["Actor"] != partiesAdmin {
				t.Fatalf("When's invite was moved by %q", row["Actor"])
			}
		}
	}
	if moves != 1 {
		t.Fatalf("When moved the invite %d times", moves)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", move, body); rec.Code != http.StatusBadRequest {
		t.Fatalf("moved the same address twice: %d", rec.Code)
	}

	if err := cache.Commit(context.Background(), access.System(partiesAdmin), partiesAppName, store.Insert(ticketsTab, store.Row{"Ticket ID": "TOLD", "Party ID": "pty0000000002", "Email": school, "Purchaser": school, "Status": TicketSold, "Price": "0"})); err != nil {
		t.Fatal(err)
	}
	if tk, _ := cache.Model().Parties.TicketByID("TOLD"); tk.Email != home || tk.Purchaser != home || tk.Name != "Ella Whitfield" {
		t.Fatalf("a row naming the old address: %+v", tk)
	}

	if rec := testkit.Call(t, mux, partiesAdmin, "POST", move, map[string]any{"old": home, "to": later}); rec.Code != http.StatusNoContent {
		t.Fatalf("second move: %d %s", rec.Code, rec.Body)
	}
	if got := cache.Model().Parties.CurrentAddress(school); got != later {
		t.Fatalf("the first address now goes to %s", got)
	}
	if tk, _ := cache.Model().Parties.TicketByID(held); tk.Email != later || tk.Name != "Ella Whitfield" {
		t.Fatalf("after the second move: %+v", tk)
	}
	queue.Flush()
	_, rows, err := sheet.Table(partiesAppName, formerTab)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["New"] != later || rows[1]["Old"] != home || rows[1]["New"] != later || rows[0]["Changed"] != todayLocal() {
		t.Fatalf("former addresses: %v", rows)
	}
}

func TestFormerAddressesRefuseALoop(t *testing.T) {
	_, _ = partiesServer(t)
	tabs := partiesTables(t)
	tabs[formerTab] = []store.Row{{"Old": "a@x.org", "New": "b@x.org"}, {"Old": "b@x.org", "New": "c@x.org"}}
	if _, err := BuildParties(context.Background(), tabs, partiesBundled); err == nil || !strings.Contains(err.Error(), "has itself moved") {
		t.Fatalf("a chain loaded: %v", err)
	}
	tabs[formerTab] = []store.Row{{"Old": "a@x.org", "New": "a@x.org"}}
	if _, err := BuildParties(context.Background(), tabs, partiesBundled); err == nil {
		t.Fatal("an address moving to itself loaded")
	}
}

func TestProblemAddresses(t *testing.T) {
	cache, mux := partiesServer(t)
	const (
		alum    = "maya.lin@heliosschool.org"
		outside = "percy.jackson@gmail.com"
	)
	var tickets []string
	for _, tk := range cache.Model().Parties.Party("pty0000000001").Tickets {
		if tk.Email == teen || tk.Email == partiesKid {
			tickets = append(tickets, tk.ID)
		}
	}
	testkit.Call(t, mux, partiesAdmin, "POST", "/api/tickets/"+tickets[0]+"/reassign", map[string]any{"email": alum})
	testkit.Call(t, mux, partiesAdmin, "POST", "/api/tickets/"+tickets[1]+"/reassign", map[string]any{"email": outside, "name": "Percy Jackson"})
	key := id.Of(sampleKey, kindPartiesAddresses, "")
	path := "/api/celebrate-addresses/" + key
	if rec := testkit.Call(t, mux, jordan, "GET", path, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("a parent read the addresses: %d", rec.Code)
	}
	addresses := func() addressesResource {
		out := decoded[envelope](t, testkit.Call(t, mux, partiesAdmin, "GET", path, nil))
		var v addressesResource
		if err := json.Unmarshal(out.Resources["celebrate-addresses"][key], &v); err != nil {
			t.Fatalf("addresses: %v", err)
		}
		return v
	}
	view := addresses()
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
	if find(outside) != nil || find(jordan) != nil || find(teen) != nil {
		t.Fatalf("listed an outside or directory address")
	}
	testkit.Call(t, mux, partiesAdmin, "POST", celebratePath("move-address"), map[string]any{"old": alum, "to": "maya@college.edu", "name": "Maya Lin"})
	view = addresses()
	if find(alum) != nil || len(view.Moved) != 1 || view.Moved[0].Old != alum || view.Moved[0].New != "maya@college.edu" {
		t.Fatalf("after the change: %+v %+v", view.Problems, view.Moved)
	}
}

func TestPendingPartiesFollowCan(t *testing.T) {
	cache, mux := partiesServer(t)
	const pending, host = "pty0000000013", "layla.haddad@heliosschool.org"
	if p := cache.Model().Parties.Party(pending); p == nil || p.Status != StatusPending {
		t.Fatalf("the sample's pending party: %+v", p)
	}
	ids := func(as, path string) []string {
		out := decoded[envelope](t, testkit.Call(t, mux, as, "GET", path, nil))
		keys := []string{}
		if err := json.Unmarshal(out.Result, &keys); err != nil {
			t.Fatal(err)
		}
		return keys
	}
	if got := ids(partiesAdmin, "/api/parties?status=pending&can=status"); !slices.Equal(got, []string{partyKey(pending)}) {
		t.Errorf("the admin's approvals = %v", got)
	}
	if got := ids(partiesAdmin, "/api/parties?status=pending"); !slices.Equal(got, []string{partyKey(pending)}) {
		t.Errorf("the pending parties = %v", got)
	}
	if got := ids(host, "/api/parties?status=pending"); !slices.Contains(got, partyKey(pending)) {
		t.Errorf("the host's pending parties = %v", got)
	}
	if got := ids(host, "/api/parties?status=pending&can=status"); len(got) != 0 {
		t.Errorf("the host may approve %v", got)
	}
	if got := ids(jordan, "/api/parties?status=pending"); len(got) != 0 {
		t.Errorf("someone else's pending parties = %v", got)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "GET", "/api/parties?status=open", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("another status: %d", rec.Code)
	}
	if rec := testkit.Call(t, mux, partiesAdmin, "POST", "/api/parties/"+partyKey(pending)+"/status", map[string]string{"status": StatusOpen}); rec.Code != http.StatusNoContent {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body)
	}
	if got := ids(partiesAdmin, "/api/parties?status=pending&can=status"); len(got) != 0 {
		t.Errorf("an approved party is still to approve: %v", got)
	}
}
