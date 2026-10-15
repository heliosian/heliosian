package db

import (
	"context"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

const (
	celebration = "grp00000000520"
	tacoNight   = "grp00000000521"
)

func celebrateSample(t *testing.T) (*Store, *store.Queue, func(viewer string, edits ...Edit) ([]string, error)) {
	t.Helper()
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{
		{Insert: "GROUP", As: "admins", Row: map[string]any{"kind": "admins", "name": "Celebrate Admins", "status": "open", "managed_by": "grp00000000005"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@admins", "person": student, "member": "yes"}},
		{Insert: "APP", Row: map[string]any{"key": "celebrate", "name": "Helios Celebrate", "admins": "@admins", "visible_to": "grp00000000004"}},
	})
	return s, queue, func(viewer string, edits ...Edit) ([]string, error) {
		return Write(context.Background(), s, queue, access.Actor{Email: "rowan.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
	}
}

func TestPartiesAreHostedWhileTheCelebrationTakesThem(t *testing.T) {
	s, _, write := celebrateSample(t)
	hosted := func(status string) []Edit {
		return []Edit{
			{Insert: "GROUP", As: "p", Row: map[string]any{"kind": "party", "parent": celebration, "name": "Dumpling Night", "status": status, "added_by": parent, "listed": true, "visible_to": "grp00000000004", "members_visible_to": "grp00000000004"}},
			{Insert: "GROUP", As: "m", Row: map[string]any{"kind": "group", "name": "Dumpling Night Managers", "status": "open", "added_by": parent}},
			{Set: "@m", Cells: map[string]any{"managed_by": "@m"}},
			{Insert: "MEMBER", Row: map[string]any{"group": "@m", "person": parent, "member": "yes"}},
			{Set: "@p", Cells: map[string]any{"managed_by": "@m"}},
		}
	}

	if _, err := write(parent, hosted("pending")...); err == nil {
		t.Error("a parent hosted a party while the celebration takes none")
	}
	if _, err := write(student, Edit{Set: celebration, Cells: map[string]any{"adding": "approval"}}); err != nil {
		t.Fatalf("a Celebrate admin can't open hosting: %v", err)
	}
	if _, err := write(parent, hosted("open")...); err == nil {
		t.Error("a parent's party went live without approval")
	}
	made, err := write(parent, hosted("pending")...)
	if err != nil {
		t.Fatalf("a parent can't host a party: %v", err)
	}
	if _, err := write(parent, Edit{Set: made[0], Cells: map[string]any{"subtitle": "Fold, cook, eat", "price": "40", "unit": "person", "minimum": "6", "drop_off_allowed": true, "parent_ticket_required": true}}); err != nil {
		t.Errorf("a host can't set up their pending party: %v", err)
	}
	if _, err := write(parent, Edit{Set: made[0], Cells: map[string]any{"status": "open"}}); err == nil {
		t.Error("a host approved their own party")
	}
	if _, err := write(student, Edit{Set: made[0], Cells: map[string]any{"status": "open", "name": "Dumpling Time"}}); err != nil {
		t.Errorf("a Celebrate admin can't approve and edit a party: %v", err)
	}
	if _, err := write(guest, Edit{Set: made[0], Cells: map[string]any{"price": "10"}}); err == nil {
		t.Error("someone who doesn't host a party set its price")
	}
	if g, _ := s.Model().Table("GROUP").Get(made[0]); g["status"] != "open" || g["managed_by"] != made[1] || g["price"] != "40" {
		t.Fatalf("the hosted party reads %v", g)
	}
}

func TestAHostMovesSomeoneFromTheWaitlistOntoTheParty(t *testing.T) {
	s, queue, write := celebrateSample(t)
	apply(t, s, queue, []Edit{{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000522", "person": parent, "member": "yes"}}})
	made, err := write(parent,
		Edit{Insert: "GROUP", As: "w", Row: map[string]any{"kind": "group", "parent": tacoNight, "name": "Taco Night Waitlist", "status": "open", "listed": true, "added_by": parent, "visible_to": "grp00000000004", "members_visible_to": "grp00000000004"}},
		Edit{Set: tacoNight, Cells: map[string]any{"waitlist": "@w"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": "@w", "person": guest, "member": "yes", "added_by": parent}},
	)
	if err != nil {
		t.Fatalf("a host can't give a party a waitlist: %v", err)
	}
	if _, err := write(parent,
		Edit{Delete: made[2]},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": tacoNight, "person": guest, "member": "yes", "added_by": parent}},
	); err != nil {
		t.Fatalf("a host can't move someone off the waitlist onto the party: %v", err)
	}
	if _, ok := s.Model().Table("MEMBER").Find(made[0], guest); ok {
		t.Error("the moved person is still on the waitlist")
	}
	if _, ok := s.Model().Table("MEMBER").Find(tacoNight, guest); !ok {
		t.Error("the moved person holds no ticket")
	}
}

func TestPaymentsAreTheirPayersHoldersHostsAndAdmins(t *testing.T) {
	s, queue, write := celebrateSample(t)
	payment := map[string]any{"person": parent, "group": tacoNight, "amount": "80", "added": "2026-10-11 18:13"}
	if _, err := write(student, Edit{Insert: "PAYMENT", Row: payment}); err == nil {
		t.Error("a Celebrate admin wrote a payment, which only the server does")
	}
	made := apply(t, s, queue, []Edit{
		{Insert: "PAYMENT", Row: payment},
		{Insert: "PAYMENT", Row: map[string]any{"person": parent, "for": guest, "group": tacoNight, "amount": "40", "added": "2026-10-11 18:13", "stripe_invoice": "in_1UPW1SEdXpaCNGo9yXCCTItm"}},
	})
	read := func(id string) string { return `(from PAYMENT (where (= id "` + id + `")))` }
	if len(as(t, s, guest, read(made[0]))) != 0 {
		t.Error("someone neither paying, holding nor hosting read a payment")
	}
	if len(as(t, s, guest, read(made[1]))) != 1 {
		t.Error("a ticket's holder can't read what it cost")
	}
	apply(t, s, queue, []Edit{{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000522", "person": guest, "member": "yes"}}})
	for _, viewer := range []string{parent, guest, student} {
		if len(as(t, s, viewer, read(made[0]))) != 1 {
			t.Errorf("%s can't read the payment", viewer)
		}
	}
	if _, err := write(parent, Edit{Set: made[0], Cells: map[string]any{"status": "paid"}}); err == nil {
		t.Error("the payer marked their own payment paid")
	}
	if _, err := write(guest, Edit{Set: made[0], Cells: map[string]any{"status": "paid"}}); err == nil {
		t.Error("a host marked a payment paid")
	}
	if _, err := write(student, Edit{Set: made[0], Cells: map[string]any{"status": "paid", "person": staff}}); err != nil {
		t.Errorf("a Celebrate admin can't say where a payment stands and who pays: %v", err)
	}
}
