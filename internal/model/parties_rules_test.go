package model

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/id"
	"heliosian/internal/testkit"
)

type attendee = struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

func TestTakeTicketsDirectly(t *testing.T) {
	cache, _ := partiesServer(t)
	m := cache.Model().Parties
	order := func(party, purchaser string, free, raise bool, people ...attendee) ticketOrder {
		return ticketOrder{PartyID: party, Purchaser: purchaser, Free: free, RaiseCapacity: raise, Attendees: people}
	}
	cases := []struct {
		name       string
		actor      access.Actor
		order      ticketOrder
		status     int
		sold       int
		waitlisted int
		ops        int
	}{
		{"a student cannot buy", partyViewerOf(partiesKid, false), order("pty0000000001", "", false, false, attendee{Email: partiesKid}), http.StatusForbidden, 0, 0, 0},
		{"only a host gives a free ticket", partyViewerOf(teacher, false), order("pty0000000002", "", true, false, attendee{Name: "Percy"}), http.StatusForbidden, 0, 0, 0},
		{"nobody named", partyViewerOf(jordan, false), order("pty0000000002", "", false, false), http.StatusBadRequest, 0, 0, 0},
		{"no such party", partyViewerOf(jordan, false), order("pty0000000999", "", false, false, attendee{Email: jordan}), http.StatusNotFound, 0, 0, 0},
		{"sold out", partyViewerOf(jordan, false), order("pty0000000008", "", false, false, attendee{Email: jordan}), http.StatusBadRequest, 0, 0, 0},
		{"closed", partyViewerOf(elena, false), order("pty0000000011", "", false, false, attendee{Email: elena}), http.StatusBadRequest, 0, 0, 0},
		{"full with a waitlist", partyViewerOf(jordan, false), order("pty0000000006", "", false, false, attendee{Email: jordan}), http.StatusBadRequest, 0, 0, 0},
		{"a student at an adult party", partyViewerOf(jordan, false), order("pty0000000002", "", false, false, attendee{Email: partiesKid}), http.StatusBadRequest, 0, 0, 0},
		{"a stranger", partyViewerOf(jordan, false), order("pty0000000002", "", false, false, attendee{Email: elena}), http.StatusForbidden, 0, 0, 0},
		{"billed to a stranger", partyViewerOf(jordan, false), order("pty0000000002", elena, false, false, attendee{Email: jordan}), http.StatusForbidden, 0, 0, 0},
		{"a student hosts no guest", partyViewerOf(elena, false), order("pty0000000002", teen, true, false, attendee{Name: "Grover"}), http.StatusBadRequest, 0, 0, 0},
		{"a ticket and its invoice", partyViewerOf(jordan, false), order("pty0000000002", "", false, false, attendee{Email: partner}), http.StatusOK, 1, 0, 2},
		{"past the last ticket goes on the waitlist", partyViewerOf(jordan, false), order("pty0000000002", "", false, false, attendee{Email: partner}, attendee{Name: "Aunt May"}, attendee{Name: "Uncle Ben"}), http.StatusOK, 2, 1, 5},
		{"a host adds past capacity", partyViewerOf(abena, false), order("pty0000000008", "", false, false, attendee{Email: teacher}), http.StatusOK, 1, 0, 2},
		{"a free ticket raises capacity and skips the invoice", partyViewerOf(elena, false), order("pty0000000002", "", true, true, attendee{Name: "Percy"}), http.StatusOK, 1, 0, 2},
		{"an old party id", partyViewerOf(jordan, false), order("P002", "", false, false, attendee{Email: partner}), http.StatusOK, 1, 0, 2},
	}
	for _, c := range cases {
		got, err := m.takeTickets(c.actor, partiesSampleDirectory, c.order)
		if status := testkit.Status(t, err); status != c.status {
			t.Errorf("%s: status %d, want %d (%v)", c.name, status, c.status, err)
			continue
		}
		if err == nil && (got.sold != c.sold || got.waitlisted != c.waitlisted || len(got.ops) != c.ops) {
			t.Errorf("%s: sold %d, waitlisted %d, %d ops; want %d, %d, %d", c.name, got.sold, got.waitlisted, len(got.ops), c.sold, c.waitlisted, c.ops)
		}
	}
	got, err := m.takeTickets(partyViewerOf("mina.park@heliosschool.org", false), partiesSampleDirectory, order("pty0000000012", "", false, false, attendee{Email: teen}))
	if err != nil || len(got.added) != 1 || got.added[0]["Purchaser"] != jordan {
		t.Fatalf("a host's ticket for a student is billed to %v (%v)", got.added, err)
	}
	got, err = m.takeTickets(partyViewerOf(jordan, false), partiesSampleDirectory, order("P002", "", false, false, attendee{Email: partner}, attendee{Name: "Aunt May"}, attendee{Name: "Uncle Ben"}))
	if err != nil {
		t.Fatal(err)
	}
	minted := map[string]bool{}
	for _, row := range got.added {
		key, ok := id.Parse(row["Ticket ID"])
		if !ok || minted[key] || m.taken(key) || row["Party ID"] != "pty0000000002" {
			t.Fatalf("minted %v", got.added)
		}
		minted[key] = true
	}
	if len(minted) != 3 {
		t.Fatalf("minted %d ids for two tickets and a waitlist request", len(minted))
	}
}

func TestSavePartyDirectly(t *testing.T) {
	cache, _ := partiesServer(t)
	m := cache.Model().Parties
	fresh := partyBody{Title: "Board Game Night", Adults: true, Status: StatusOpen, Category: "pcg0000000001", Celebration: "cbn0000002025"}
	dink := m.Party("pty0000000002")
	edit := partyBody{ID: dink.ID, Title: "Dink & Clink", Price: dink.Price, Adults: true, Status: StatusOpen, Celebration: dink.Celebration, Category: dink.Category, HostEmails: []string{elena, "marco.torres@heliosschool.org"}}
	whole := map[string]bool{"hostEmails": true}
	for field := range partyColumns {
		whole[field] = true
	}
	with := func(b partyBody, change func(*partyBody)) partyBody {
		change(&b)
		return b
	}
	cases := []struct {
		name   string
		actor  access.Actor
		body   partyBody
		status int
		want   string
		ops    int
	}{
		{"a parent's party waits for approval", partyViewerOf(elena, false), fresh, http.StatusOK, StatusPending, 2},
		{"an admin's party keeps its status", partyViewerOf(partiesAdmin, true), with(fresh, func(b *partyBody) { b.Status = StatusHidden }), http.StatusOK, StatusHidden, 1},
		{"an admin's party opens by default", partyViewerOf(partiesAdmin, true), with(fresh, func(b *partyBody) { b.Status = "" }), http.StatusOK, StatusOpen, 1},
		{"a party for nobody", partyViewerOf(elena, false), with(fresh, func(b *partyBody) { b.Adults = false }), http.StatusBadRequest, "", 0},
		{"a negative price", partyViewerOf(elena, false), with(fresh, func(b *partyBody) { b.Price = -1 }), http.StatusBadRequest, "", 0},
		{"an unknown status", partyViewerOf(partiesAdmin, true), with(fresh, func(b *partyBody) { b.Status = "Maybe" }), http.StatusBadRequest, "", 0},
		{"a stranger cannot edit", partyViewerOf(teacher, false), edit, http.StatusForbidden, "", 0},
		{"a host saves", partyViewerOf(elena, false), edit, http.StatusOK, StatusOpen, 1},
		{"a host cannot change the status", partyViewerOf(elena, false), with(edit, func(b *partyBody) { b.Status = StatusHidden }), http.StatusForbidden, "", 0},
		{"a host cannot leave", partyViewerOf(elena, false), with(edit, func(b *partyBody) { b.HostEmails = []string{"marco.torres@heliosschool.org"} }), http.StatusBadRequest, "", 0},
		{"a host adds a host", partyViewerOf(elena, false), with(edit, func(b *partyBody) { b.HostEmails = append(b.HostEmails, teacher) }), http.StatusOK, StatusOpen, 2},
		{"an admin drops a host", partyViewerOf(partiesAdmin, true), with(edit, func(b *partyBody) { b.HostEmails, b.Status = []string{elena}, StatusHidden }), http.StatusOK, StatusHidden, 2},
		{"a malformed address", partyViewerOf(elena, false), with(edit, func(b *partyBody) { b.PrettyID = "k pop!" }), http.StatusBadRequest, "", 0},
	}
	for _, c := range cases {
		saved, err := m.saveParty(c.actor, c.body, whole)
		if status := testkit.Status(t, err); status != c.status {
			t.Errorf("%s: status %d, want %d (%v)", c.name, status, c.status, err)
			continue
		}
		if err == nil && (saved.status != c.want || len(saved.ops) != c.ops) {
			t.Errorf("%s: saved as %q with %d ops, want %q with %d", c.name, saved.status, len(saved.ops), c.want, c.ops)
		}
	}
	saved, err := m.saveParty(partyViewerOf(elena, false), fresh, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Commit(context.Background(), partyViewerOf(elena, false), partiesAppName, saved.ops...); err != nil {
		t.Fatal(err)
	}
	p := cache.Model().Parties.Party(saved.id)
	if p == nil || p.Status != StatusPending || p.Category != "" || p.Celebration != "cbn0000002026" || !p.Hosted(elena) || p.AddedBy != elena {
		t.Fatalf("a parent's party landed as %+v", p)
	}
	if _, ok := id.Parse(p.ID); !ok || p.ID != saved.id {
		t.Fatalf("a parent's party landed as %+v", p)
	}
	closed := *cache.Model().Parties
	closed.Settings.HostingOpen = false
	if _, err := closed.saveParty(partyViewerOf(elena, false), fresh, nil); testkit.Status(t, err) != http.StatusForbidden {
		t.Fatalf("a parent posted while hosting is closed: %v", err)
	}
	if _, err := closed.saveParty(partyViewerOf(partiesAdmin, true), fresh, nil); err != nil {
		t.Fatalf("an admin could not post while hosting is closed: %v", err)
	}
}

func TestSavePartyAddressConflict(t *testing.T) {
	cache, _ := partiesServer(t)
	body := partyBody{ID: "pty0000000003", Title: "K-Pop for a Cause!", Price: cache.Model().Parties.Party("pty0000000003").Price, Adults: true, Students: true, PrettyID: "Fondue", HostEmails: []string{"deepa.natarajan@heliosschool.org"}}
	_, err := cache.Model().Parties.saveParty(partyViewerOf("deepa.natarajan@heliosschool.org", false), body, map[string]bool{"prettyId": true})
	var refusal *access.Refusal
	if !errors.As(err, &refusal) || refusal.Status != http.StatusConflict {
		t.Fatalf("took another party's address: %v", err)
	}
	if c, ok := refusal.Body.(*partyPrettyConflict); !ok || c.ID != "pty0000000001" || c.Title != "Fondue & Fort Night" || c.Message != refusal.Message {
		t.Fatalf("conflict body: %+v", refusal.Body)
	}
}
