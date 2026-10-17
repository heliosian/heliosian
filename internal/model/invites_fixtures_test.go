package model

import (
	"net/http"
	"testing"

	"heliosian/internal/id"
	"heliosian/internal/store"
	"heliosian/internal/testkit/mailtest"
)

const (
	host  = "jordan.whitfield@heliosschool.org"
	robin = "robin.whitfield@heliosschool.org"
	sam   = "sam.whitfield@heliosschool.org"
	ella  = "ella.whitfield@heliosschool.org"
	mia   = "mia.torres@heliosschool.org"
	coach = "coach@example.org"
)

const bookFair = "act0000000101"

var testHooks CalendarHooks

func invitesLinked() store.Tables {
	return store.Tables{
		activitiesTab: {{"Event ID": bookFair, CalendarEventColumn: "tev0000000101", "Year": "2026 - 2027", "Title": "Book Fair", "Category": "tcg0000000002", "Status": StatusOpen, "Start": "2026-11-20 08:00", "End": "2026-11-20 15:00"}},
		volunteersTab: {{"Event ID": bookFair, "Email": mia, "Position": PositionCoChair}},
		id.AliasesTab: {{"Alias": "E001", "ID": bookFair}},
	}
}

func calendarInvitesApp(t *testing.T) (http.Handler, *Store, *mailtest.Recorder) {
	h, c, k, _ := invitesAppWith(t)
	return h, c, k
}

func invitesAppWith(t *testing.T) (http.Handler, *Store, *mailtest.Recorder, *sampleSources) {
	t.Helper()
	team := invitesLinked()
	sampleSheet(t)
	outsideSuperAdmin(t, sheet)
	linkRows(t, team)
	listRows(t, nil)
	_, aliases, err := sheet.Table(CalendarApp, id.AliasesTab)
	if err != nil {
		t.Fatal(err)
	}
	replaceRows(t, CalendarApp, id.AliasesTab, append(aliases, store.Row{"Alias": "team/E001", "ID": "tev0000000101"}))
	cache := sampleStore(t, sheet, queue, sampleDeps(sampleKey))
	sources := &sampleSources{directory: cache}
	kept := keptMail()
	mux := http.NewServeMux()
	testDeps = calendarDeps(cache, CalendarMail{Sender: kept.Mailgun, Base: "https://when.heliosian.com", ReplyTo: "Helios When <rsvp@reply.heliosian.com>", Key: []byte("session-key")})
	testHooks = RegisterCalendar(mux, testDeps)
	return served(mux, cache, testHooks), cache, kept, sources
}

var testDeps CalendarDeps

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
