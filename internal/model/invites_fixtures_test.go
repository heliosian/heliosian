package model

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/store"
	"heliosian/internal/testkit/mailtest"
)

const (
	host   = "jordan.whitfield@heliosschool.org"
	robin  = "robin.whitfield@heliosschool.org"
	sam    = "sam.whitfield@heliosschool.org"
	ella   = "ella.whitfield@heliosschool.org"
	mia    = "mia.torres@heliosschool.org"
	coach  = "coach@example.org"
	partyA = "pty0000000001"
)

var testStore *Store

func directoryOf() *Directory { return testStore.Model().Directory }

const bookFair = "act0000000101"

func magicKeysOf(t *testing.T, h http.Handler) []string {
	t.Helper()
	out := decoded[envelope](t, call(t, h, "GET", "/api/magic-tags", ""))
	var ids []string
	if err := json.Unmarshal(out.Result, &ids); err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for _, key := range ids {
		var tag magicTagResource
		if err := json.Unmarshal(out.Resources["magic-tags"][key], &tag); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, tag.Key)
	}
	return keys
}

func tagKeysOf(t *testing.T, h http.Handler) []string {
	t.Helper()
	out := decoded[envelope](t, call(t, h, "GET", "/api/tags", ""))
	var ids []string
	if err := json.Unmarshal(out.Result, &ids); err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for _, key := range ids {
		keys = append(keys, TagKey(key))
	}
	return keys
}

var testHooks CalendarHooks

func fondueTicket(key, email, name, status, price, added string) store.Row {
	return store.Row{"Ticket ID": key, "Party ID": partyA, "Email": email, "Name": name, "Purchaser": host, "Status": status, "Price": price, "Added": added}
}

func invitesLinked() (store.Tables, store.Tables) {
	celebrate := store.Tables{
		partiesTab: {{"Party ID": partyA, "Celebration": "cbn0000002026", "Title": "Fondue Night", "Price": "30", "Start": "2026-11-14 18:00", "End": "2026-11-14 21:00", "Status": StatusOpen}},
		hostsTab:   {{"Party ID": partyA, "Email": mia}},
		ticketsTab: {
			fondueTicket("tkt0000000901", robin, "Robin Whitfield", TicketSold, "30", "2026-09-01 10:00"),
			fondueTicket("tkt0000000902", sam, "Sam Whitfield", TicketSold, "30", "2026-09-01 10:01"),
			fondueTicket("tkt0000000903", "", "A cousin", TicketWaitlist, "", "2026-09-01 10:02"),
		},
		id.AliasesTab: {{"Alias": "P001", "ID": partyA}},
	}
	team := store.Tables{
		activitiesTab: {{"Event ID": bookFair, CalendarEventColumn: "tev0000000101", "Year": "2026 - 2027", "Title": "Book Fair", "Category": "tcg0000000002", "Status": StatusOpen, "Start": "2026-11-20 08:00", "End": "2026-11-20 15:00"}},
		volunteersTab: {{"Event ID": bookFair, "Email": mia, "Position": PositionCoChair}},
		id.AliasesTab: {{"Alias": "E001", "ID": bookFair}},
	}
	return celebrate, team
}

func calendarInvitesApp(t *testing.T) (http.Handler, *Store, *mailtest.Recorder) {
	h, c, k, _ := invitesAppWith(t)
	return h, c, k
}

func invitesAppWith(t *testing.T) (http.Handler, *Store, *mailtest.Recorder, *sampleSources) {
	t.Helper()
	celebrate, team := invitesLinked()
	sampleSheet(t)
	outsideSuperAdmin(t, sheet)
	linkRows(t, celebrate, team)
	listRows(t, nil)
	_, aliases, err := sheet.Table(CalendarApp, id.AliasesTab)
	if err != nil {
		t.Fatal(err)
	}
	replaceRows(t, CalendarApp, id.AliasesTab, append(aliases, store.Row{"Alias": "celebrate/P001", "ID": partyA}, store.Row{"Alias": "team/E001", "ID": "tev0000000101"}))
	cache := sampleStore(t, sheet, queue, sampleDeps(sampleKey))
	testStore = cache
	sources := &sampleSources{directory: cache}
	kept := keptMail()
	mux := http.NewServeMux()
	testDeps = calendarDeps(cache, CalendarMail{Sender: kept.Mailgun, Base: "https://when.heliosian.com", ReplyTo: "Helios When <rsvp@reply.heliosian.com>", Key: []byte("session-key")})
	testHooks = RegisterCalendar(mux, testDeps)
	return served(mux, cache, testHooks), cache, kept, sources
}

var testDeps CalendarDeps

func fillNow(t *testing.T) {
	t.Helper()
	newCalendarApp(testDeps).fillGroups(context.Background())
}

func idOf(t *testing.T, cache *Store, address string) string {
	t.Helper()
	e := cache.Model().Calendar.Event(address)
	if e == nil {
		t.Fatalf("no event at %s", address)
	}
	if _, ok := id.Parse(e.ID); !ok {
		t.Fatalf("the event at %s was not minted an ID: %q", address, e.ID)
	}
	return e.ID
}

type sampleSources struct {
	directory *Store
}

func (s *sampleSources) tag(t *testing.T, key string, people ...string) {
	t.Helper()
	d := s.directory.Model().Directory
	tag, ok := d.Tag(key)
	if !ok {
		t.Fatalf("no tag %s", key)
	}
	owner := d.ActorOf(tag.Owner, nil)
	for _, person := range people {
		if slices.Contains(tag.People, person) {
			continue
		}
		if err := s.directory.Commit(context.Background(), owner, DirectoryApp, store.Insert(tagsTable, store.Row{tagID: key, tagPerson: person})); err != nil {
			t.Fatalf("tag %s: %v", person, err)
		}
	}
}

func (s *sampleSources) newTag(t *testing.T, owner, name, person string) string {
	t.Helper()
	d := s.directory.Model().Directory
	key := id.New(func(k string) bool { return d.tagByKey(k) != nil })
	err := s.directory.Commit(context.Background(), d.ActorOf(owner, nil), DirectoryApp,
		store.Insert(tagListTable, store.Row{tagID: key, tagOwner: owner, tagName: name}),
		store.Insert(tagsTable, store.Row{tagID: key, tagPerson: person}),
	)
	if err != nil {
		t.Fatalf("tag %s: %v", name, err)
	}
	return key
}

func eventually(t *testing.T, what string, done func() bool) {
	t.Helper()
	for range 500 {
		if done() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("never happened: %s", what)
}

func rowOf(v InviteView, key string) *GuestRow {
	for i := range v.List {
		if v.List[i].Key == key {
			return &v.List[i]
		}
	}
	return nil
}

func guestKeyOf(t *testing.T, cache *Store, eventID, of, name string) string {
	t.Helper()
	for _, inv := range cache.Model().Calendar.Invites[eventID] {
		if inv.GuestOf == of && inv.Name == name {
			return inv.Email
		}
	}
	t.Fatalf("no guest %s of %s on %s: %+v", name, of, eventID, cache.Model().Calendar.Invites[eventID])
	return ""
}

func viaGroup(cache *Store, eventID, group string) int {
	n := 0
	for _, inv := range cache.Model().Calendar.Invites[eventID] {
		if inv.Via == ViaGroup+group {
			n++
		}
	}
	return n
}

func mailTo(kept *mailtest.Recorder, to string) []mail.Message {
	out := []mail.Message{}
	for _, m := range kept.Messages() {
		if m.To[0] == to {
			out = append(out, m)
		}
	}
	return out
}

func waitFor(kept *mailtest.Recorder, n int) []mail.Message {
	for i := 0; i < 100 && len(kept.Messages()) < n; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	return kept.Messages()
}
