package celebrate

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/testkit"
)

type attendee = struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

func TestTakeTicketsDirectly(t *testing.T) {
	cache, _ := newServer(t)
	m := cache.Model()
	const abena = "abena.osei@heliosschool.org"
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
		{"a student cannot buy", viewerOf(kid, false), order("P001", "", false, false, attendee{Email: kid}), http.StatusForbidden, 0, 0, 0},
		{"only a host gives a free ticket", viewerOf(teacher, false), order("P002", "", true, false, attendee{Name: "Percy"}), http.StatusForbidden, 0, 0, 0},
		{"nobody named", viewerOf(parent, false), order("P002", "", false, false), http.StatusBadRequest, 0, 0, 0},
		{"no such party", viewerOf(parent, false), order("P999", "", false, false, attendee{Email: parent}), http.StatusNotFound, 0, 0, 0},
		{"sold out", viewerOf(parent, false), order("P008", "", false, false, attendee{Email: parent}), http.StatusBadRequest, 0, 0, 0},
		{"closed", viewerOf(other, false), order("P011", "", false, false, attendee{Email: other}), http.StatusBadRequest, 0, 0, 0},
		{"full with a waitlist", viewerOf(parent, false), order("P006", "", false, false, attendee{Email: parent}), http.StatusBadRequest, 0, 0, 0},
		{"a student at an adult party", viewerOf(parent, false), order("P002", "", false, false, attendee{Email: kid}), http.StatusBadRequest, 0, 0, 0},
		{"a stranger", viewerOf(parent, false), order("P002", "", false, false, attendee{Email: other}), http.StatusForbidden, 0, 0, 0},
		{"billed to a stranger", viewerOf(parent, false), order("P002", other, false, false, attendee{Email: parent}), http.StatusForbidden, 0, 0, 0},
		{"a student hosts no guest", viewerOf(other, false), order("P002", teen, true, false, attendee{Name: "Grover"}), http.StatusBadRequest, 0, 0, 0},
		{"a ticket and its invoice", viewerOf(parent, false), order("P002", "", false, false, attendee{Email: partner}), http.StatusOK, 1, 0, 2},
		{"past the last ticket goes on the waitlist", viewerOf(parent, false), order("P002", "", false, false, attendee{Email: partner}, attendee{Name: "Aunt May"}, attendee{Name: "Uncle Ben"}), http.StatusOK, 2, 1, 5},
		{"a host adds past capacity", viewerOf(abena, false), order("P008", "", false, false, attendee{Email: teacher}), http.StatusOK, 1, 0, 2},
		{"a free ticket raises capacity and skips the invoice", viewerOf(other, false), order("P002", "", true, true, attendee{Name: "Percy"}), http.StatusOK, 1, 0, 2},
	}
	for _, c := range cases {
		got, err := m.takeTickets(c.actor, sampleDirectory, c.order)
		if status := testkit.Status(t, err); status != c.status {
			t.Errorf("%s: status %d, want %d (%v)", c.name, status, c.status, err)
			continue
		}
		if err == nil && (got.sold != c.sold || got.waitlisted != c.waitlisted || len(got.ops) != c.ops) {
			t.Errorf("%s: sold %d, waitlisted %d, %d ops; want %d, %d, %d", c.name, got.sold, got.waitlisted, len(got.ops), c.sold, c.waitlisted, c.ops)
		}
	}
	got, err := m.takeTickets(viewerOf("mina.park@heliosschool.org", false), sampleDirectory, order("P012", "", false, false, attendee{Email: teen}))
	if err != nil || len(got.added) != 1 || got.added[0]["Purchaser"] != parent {
		t.Fatalf("a host's ticket for a student is billed to %v (%v)", got.added, err)
	}
}

func TestSavePartyDirectly(t *testing.T) {
	cache, _ := newServer(t)
	m := cache.Model()
	fresh := partyBody{Title: "Board Game Night", Adults: true, Status: StatusOpen, Category: "Family Social", Celebration: "SC-2025"}
	edit := partyBody{ID: "P002", Title: "Dink & Clink", Adults: true, Status: StatusHidden, HostEmails: []string{other, "marco.torres@heliosschool.org"}}
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
		{"a parent's party waits for approval", viewerOf(other, false), fresh, http.StatusOK, StatusPending, 2},
		{"an admin's party keeps its status", viewerOf(admin, true), with(fresh, func(b *partyBody) { b.Status = StatusHidden }), http.StatusOK, StatusHidden, 1},
		{"an admin's party opens by default", viewerOf(admin, true), with(fresh, func(b *partyBody) { b.Status = "" }), http.StatusOK, StatusOpen, 1},
		{"a party for nobody", viewerOf(other, false), with(fresh, func(b *partyBody) { b.Adults = false }), http.StatusBadRequest, "", 0},
		{"a negative price", viewerOf(other, false), with(fresh, func(b *partyBody) { b.Price = -1 }), http.StatusBadRequest, "", 0},
		{"an unknown status", viewerOf(admin, true), with(fresh, func(b *partyBody) { b.Status = "Maybe" }), http.StatusBadRequest, "", 0},
		{"a stranger cannot edit", viewerOf(teacher, false), edit, http.StatusForbidden, "", 0},
		{"a host keeps the status", viewerOf(other, false), edit, http.StatusOK, StatusOpen, 1},
		{"a host cannot leave", viewerOf(other, false), with(edit, func(b *partyBody) { b.HostEmails = []string{"marco.torres@heliosschool.org"} }), http.StatusBadRequest, "", 0},
		{"a host adds a host", viewerOf(other, false), with(edit, func(b *partyBody) { b.HostEmails = append(b.HostEmails, teacher) }), http.StatusOK, StatusOpen, 2},
		{"an admin drops a host", viewerOf(admin, true), with(edit, func(b *partyBody) { b.HostEmails = []string{other} }), http.StatusOK, StatusHidden, 2},
		{"a malformed address", viewerOf(other, false), with(edit, func(b *partyBody) { b.PrettyID = "k pop!" }), http.StatusBadRequest, "", 0},
	}
	for _, c := range cases {
		saved, err := m.saveParty(c.actor, c.body)
		if status := testkit.Status(t, err); status != c.status {
			t.Errorf("%s: status %d, want %d (%v)", c.name, status, c.status, err)
			continue
		}
		if err == nil && (saved.status != c.want || len(saved.ops) != c.ops) {
			t.Errorf("%s: saved as %q with %d ops, want %q with %d", c.name, saved.status, len(saved.ops), c.want, c.ops)
		}
	}
	saved, err := m.saveParty(viewerOf(other, false), fresh)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Commit(context.Background(), viewerOf(other, false), saved.ops...); err != nil {
		t.Fatal(err)
	}
	p := cache.Model().Party(saved.id)
	if p == nil || p.Status != StatusPending || p.Category != "" || p.Celebration != "SC-2026" || !p.Hosted(other) || p.AddedBy != other {
		t.Fatalf("a parent's party landed as %+v", p)
	}
	closed := *cache.Model()
	closed.Settings.HostingOpen = false
	if _, err := closed.saveParty(viewerOf(other, false), fresh); testkit.Status(t, err) != http.StatusForbidden {
		t.Fatalf("a parent posted while hosting is closed: %v", err)
	}
	if _, err := closed.saveParty(viewerOf(admin, true), fresh); err != nil {
		t.Fatalf("an admin could not post while hosting is closed: %v", err)
	}
}

func TestSavePartyAddressConflict(t *testing.T) {
	cache, _ := newServer(t)
	body := partyBody{ID: "P003", Title: "K-Pop for a Cause!", Adults: true, Students: true, PrettyID: "Fondue", HostEmails: []string{"deepa.natarajan@heliosschool.org"}}
	_, err := cache.Model().saveParty(viewerOf("deepa.natarajan@heliosschool.org", false), body)
	var refusal *access.Refusal
	if !errors.As(err, &refusal) || refusal.Status != http.StatusConflict {
		t.Fatalf("took another party's address: %v", err)
	}
	if c, ok := refusal.Body.(*prettyConflict); !ok || c.ID != "P001" || c.Title != "Fondue & Fort Night" || c.Message != refusal.Message {
		t.Fatalf("conflict body: %+v", refusal.Body)
	}
}
