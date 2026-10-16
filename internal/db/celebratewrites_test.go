package db

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/mail"
	"heliosian/internal/store"
	"heliosian/internal/testkit/mailtest"
)

const avery = "per00000020018"

func partyMailer(s *Store, queue *store.Queue) *mailtest.Recorder {
	sender := mailtest.NewRecorder(mailtest.From)
	NewMembershipMailer(s, queue, MembershipMailConfig{Sender: mailtest.Discard(), Celebrate: sender.Mailgun, Origin: func(app string) string { return "https://" + app + ".test" }})
	return sender
}

func subjects(messages []mail.Message) []string {
	out := []string{}
	for _, m := range messages {
		out = append(out, m.Subject)
	}
	slices.Sort(out)
	return out
}

func TestBuyingTicketsIsJoiningTheParty(t *testing.T) {
	s, queue, write := celebrateSample(t)
	sender := partyMailer(s, queue)
	paid := func(person string) Edit {
		return Edit{Insert: "PAYMENT", Row: map[string]any{"person": parent, "for": person, "group": tacoNight, "amount": "40", "added": "2026-10-15 09:00"}}
	}
	ticket := func(person string) Edit {
		return Edit{Insert: "MEMBER", Row: map[string]any{"group": tacoNight, "person": person, "member": "yes", "added_by": parent, "guest_of": parent}}
	}
	tobin := "per00000020019"
	if _, err := write(tobin,
		Edit{Insert: "PAYMENT", Row: map[string]any{"person": tobin, "for": tobin, "group": tacoNight, "amount": "40", "added": "2026-10-15 09:00"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": tacoNight, "person": tobin, "member": "yes", "added_by": tobin}},
	); err == nil {
		t.Error("a student bought a ticket")
	}
	if _, err := write(parent, ticket(avery)); err == nil {
		t.Error("someone took a paid place without paying for it")
	}
	if _, err := write(parent, Edit{Insert: "PAYMENT", Row: map[string]any{"person": parent, "for": avery, "group": tacoNight, "amount": "1", "added": "2026-10-15 09:00"}}); err == nil {
		t.Error("someone paid less than the party's price")
	}
	if _, err := write(parent,
		Edit{Insert: "PERSON", As: "g", Row: map[string]any{"source": "guest", "name_long_override": "Robin Guest"}},
		paid(student), ticket(student), paid("@g"), ticket("@g"),
	); err != nil {
		t.Fatalf("someone can't buy tickets for their child and a guest: %v", err)
	}
	messages := sender.Wait(t, 2)
	time.Sleep(100 * time.Millisecond)
	if got := subjects(sender.Messages()); !slices.Equal(got, []string{"Calendar invite: Taco Night", "Your tickets to Taco Night"}) {
		t.Fatalf("buying two tickets in one write sent %q", got)
	}
	for _, m := range messages {
		if m.Subject != "Your tickets to Taco Night" {
			continue
		}
		if !slices.Contains(m.To, "rowan.ashdown@example.org") || !slices.Contains(m.CC, "maya.lindqvist@example.org") {
			t.Errorf("the tickets note goes to %q copying %q", m.To, m.CC)
		}
		for _, want := range []string{"Juni Ashdown and Robin Guest", "$80 (2 × $40)", "Billed to: Rowan Ashdown"} {
			if !strings.Contains(m.Text, want) {
				t.Errorf("the tickets note lacks %q:\n%s", want, m.Text)
			}
		}
	}
}

func TestTheWaitlistAndPassingATicketOnMailWhoTheyShould(t *testing.T) {
	s, queue, write := celebrateSample(t)
	made := apply(t, s, queue, []Edit{
		{Insert: "GROUP", As: "w", Row: map[string]any{"kind": "group", "parent": tacoNight, "name": "Taco Night Waitlist", "status": "open", "listed": true, "join": "direct", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004"}},
		{Set: tacoNight, Cells: map[string]any{"waitlist": "@w"}},
	})
	sender := partyMailer(s, queue)
	if _, err := write(parent,
		Edit{Insert: "MEMBER", Row: map[string]any{"group": made[0], "person": avery, "member": "yes", "added_by": parent, "note": "Two if we can"}},
		Edit{Insert: "PERSON", As: "g", Row: map[string]any{"source": "guest"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": made[0], "person": "@g", "member": "yes", "added_by": parent, "guest_of": avery}},
	); err != nil {
		t.Fatalf("someone can't ask for two places on a waitlist: %v", err)
	}
	waited := sender.Wait(t, 2)
	if got := subjects(waited); !slices.Equal(got, []string{"Waitlist for Taco Night: Avery Ashdown wants 2 tickets", "You're on the waitlist for Taco Night"}) {
		t.Fatalf("a waitlist request sent %q", got)
	}
	for _, m := range waited {
		if strings.HasPrefix(m.Subject, "Waitlist for") && (len(m.To) != 1 || m.To[0] != "maya.lindqvist@example.org" || !strings.Contains(m.Text, "Two if we can")) {
			t.Errorf("the hosts' note goes to %q and says:\n%s", m.To, m.Text)
		}
	}

	if _, err := write(guest, Edit{Set: "mem00000000025", Cells: map[string]any{"person": avery}}); err == nil {
		t.Error("someone passed on a ticket that isn't theirs")
	}
}

func TestPassingATicketOnMailsItsNewHolder(t *testing.T) {
	s, queue, write := celebrateSample(t)
	sender := partyMailer(s, queue)
	if _, err := write(parent, Edit{Set: "mem00000000025", Cells: map[string]any{"person": avery}}); err != nil {
		t.Fatalf("someone can't pass their ticket to someone in their family: %v", err)
	}
	passed := sender.Wait(t, 1)[0]
	if passed.Subject != "Avery Ashdown's ticket to Taco Night" || !slices.Contains(passed.To, "avery.ashdown@example.org") || !slices.Contains(passed.CC, "rowan.ashdown@example.org") {
		t.Errorf("passing a ticket on sent %q to %q copying %q", passed.Subject, passed.To, passed.CC)
	}
}

const (
	celebration = "grp00000000520"
	tacoNight   = "grp00000000521"
)

func celebrateSample(t *testing.T) (*Store, *store.Queue, func(viewer string, edits ...Edit) ([]string, error)) {
	t.Helper()
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000009", "person": student, "member": "yes"}}})
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

func TestHostingAPartyWithItsWaitlistInOneWrite(t *testing.T) {
	s, _, write := celebrateSample(t)
	if _, err := write(student, Edit{Set: celebration, Cells: map[string]any{"adding": "approval"}}); err != nil {
		t.Fatalf("a Celebrate admin can't open hosting: %v", err)
	}
	made, err := write(parent,
		Edit{Insert: "GROUP", As: "p", Row: map[string]any{
			"kind": "party", "parent": celebration, "name": "Dumpling Night", "description": "> [!chip] Kids\n\nFold, cook, eat.", "slug": "dumplings",
			"location": "Montara", "address_override": "1 Main St, Montara", "start": "2027-04-10 17:00", "end": "2027-04-10 19:00",
			"price": "40", "unit": "child", "capacity": "8", "join": "direct", "eligible": "grp00000000001", "drop_off_allowed": true,
			"status": "pending", "listed": true, "added_by": parent, "added": "2026-10-15", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004",
		}},
		Edit{Insert: "GROUP", As: "m", Row: map[string]any{"kind": "group", "name": "Dumpling Night Managers", "status": "open", "added_by": parent}},
		Edit{Set: "@m", Cells: map[string]any{"managed_by": "@m"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": "@m", "person": parent, "member": "yes"}},
		Edit{Set: "@p", Cells: map[string]any{"managed_by": "@m"}},
		Edit{Insert: "GROUP", As: "wl", Row: map[string]any{"kind": "group", "parent": "@p", "name": "Dumpling Night Waitlist", "status": "open", "listed": true, "added_by": parent, "visible_to": "grp00000000004", "members_visible_to": "grp00000000004"}},
		Edit{Set: "@wl", Cells: map[string]any{"join": "direct", "eligible": "grp00000000001"}},
		Edit{Set: "@p", Cells: map[string]any{"waitlist": "@wl"}},
	)
	if err != nil {
		t.Fatalf("a parent can't host a party with its waitlist: %v", err)
	}
	party, waitlist := made[0], made[5]
	if g, _ := s.Model().Table("GROUP").Get(party); g["waitlist"] != waitlist || g["status"] != "pending" || g["address_override"] != "1 Main St, Montara" {
		t.Fatalf("the hosted party reads %v", g)
	}
	if _, err := write(parent,
		Edit{Insert: "MEMBER", Row: map[string]any{"group": made[1], "person": staff, "member": "yes"}},
		Edit{Set: party, Cells: map[string]any{"name": "Dumpling Time", "capacity": "10", "join": "none", "minimum": "4", "address_override": "2 Main St, Montara"}},
	); err != nil {
		t.Fatalf("a host can't add a co-host and edit their party: %v", err)
	}
	if _, err := write(guest, Edit{Set: party, Cells: map[string]any{"address_override": "3 Main St, Montara"}}); err == nil {
		t.Error("someone who doesn't host a party gave its address")
	}
	if _, err := write(student, Edit{Set: celebration, Cells: map[string]any{"address_override": "4249 El Camino Real, Palo Alto"}}); err != nil {
		t.Errorf("a Celebrate admin can't give a celebration's address: %v", err)
	}
	if _, err := write(student,
		Edit{Set: party, Cells: map[string]any{"status": "open"}},
		Edit{Set: party, Cells: map[string]any{"visible_to": ""}},
		Edit{Set: waitlist, Cells: map[string]any{"visible_to": "", "members_visible_to": ""}},
	); err != nil {
		t.Fatalf("a Celebrate admin can't approve and hide a party: %v", err)
	}
	if _, err := write(parent, Edit{Set: party, Cells: map[string]any{"waitlist": ""}}, Edit{Delete: waitlist}); err != nil {
		t.Fatalf("a host can't turn off an empty waitlist: %v", err)
	}
}

func TestCelebrateAdminsKeepACelebrationsCategories(t *testing.T) {
	_, _, write := celebrateSample(t)
	made, err := write(student, Edit{Insert: "GROUP", Row: map[string]any{"kind": "category", "parent": celebration, "name": "Adult Social", "status": "open", "order": "m", "added_by": student, "visible_to": "grp00000000004", "members_visible_to": "grp00000000004"}})
	if err != nil {
		t.Fatalf("a Celebrate admin can't add a category to a celebration: %v", err)
	}
	if _, err := write(student, Edit{Set: made[0], Cells: map[string]any{"name": "Grown-Up Social", "order": "n"}}); err != nil {
		t.Errorf("a Celebrate admin can't rename and reorder a category: %v", err)
	}
	if _, err := write(student, Edit{Delete: made[0]}); err != nil {
		t.Errorf("a Celebrate admin can't delete an empty category: %v", err)
	}
	if _, err := write(student, Edit{Insert: "GROUP", Row: map[string]any{"kind": "celebration", "parent": "grp00000000013", "slug": "sc-2028", "name": "Helios Spring Celebration 2028", "status": "open", "added_by": student, "visible_to": "grp00000000004", "members_visible_to": "grp00000000004"}}); err != nil {
		t.Errorf("a Celebrate admin can't add a celebration: %v", err)
	}
	if _, err := write(parent, Edit{Insert: "GROUP", Row: map[string]any{"kind": "celebration", "parent": "grp00000000013", "slug": "sc-2029", "name": "Helios Spring Celebration 2029", "status": "open", "added_by": parent, "visible_to": "grp00000000004", "members_visible_to": "grp00000000004"}}); err == nil {
		t.Error("someone who isn't a Celebrate admin added a celebration")
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
		Edit{Insert: "PAYMENT", Row: map[string]any{"person": guest, "for": guest, "group": tacoNight, "amount": "40", "added": "2026-10-15 09:00"}},
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

func tacoWaitlist(t *testing.T, s *Store, queue *store.Queue, capacity string) string {
	t.Helper()
	return apply(t, s, queue, []Edit{
		{Insert: "GROUP", As: "w", Row: map[string]any{"kind": "group", "parent": tacoNight, "name": "Taco Night Waitlist", "status": "open", "listed": true, "join": "direct", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004"}},
		{Set: tacoNight, Cells: map[string]any{"waitlist": "@w", "capacity": capacity}},
	})[0]
}

func TestAHostAddsAPaidGuestAndAFreeTicket(t *testing.T) {
	s, _, write := celebrateSample(t)
	if _, err := write(staff,
		Edit{Insert: "PERSON", As: "g", Row: map[string]any{"source": "guest", "name_long_override": "Robin Guest"}},
		Edit{Insert: "PERSON_EMAIL", Row: map[string]any{"person": "@g", "address": "robin.guest@example.com", "primary": true, "source": "guest"}},
		Edit{Insert: "PAYMENT", Row: map[string]any{"person": parent, "for": "@g", "group": tacoNight, "amount": "40", "added": "2026-10-15 09:00:00"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": tacoNight, "person": "@g", "member": "yes", "added_by": staff, "guest_of": staff, "added": "2026-10-15 09:00:00"}},
	); err != nil {
		t.Fatalf("a host can't add a guest's ticket billed to someone else: %v", err)
	}
	if _, err := write(staff,
		Edit{Insert: "MEMBER", Row: map[string]any{"group": tacoNight, "person": avery, "member": "yes", "added_by": staff, "note": "Free ticket from the hosts", "added": "2026-10-15 09:01:00"}},
		Edit{Set: tacoNight, Cells: map[string]any{"capacity": "21"}},
	); err != nil {
		t.Fatalf("a host can't give a free ticket and raise the capacity: %v", err)
	}
	if g, _ := s.Model().Table("GROUP").Get(tacoNight); g["capacity"] != "21" {
		t.Errorf("the capacity reads %q", g["capacity"])
	}
	if _, ok := s.Model().Table("MEMBER").Find(tacoNight, avery); !ok {
		t.Error("the free ticket isn't there")
	}
}

func TestPlacesPastCapacityGoOnTheWaitlistInTheSameWrite(t *testing.T) {
	s, queue, write := celebrateSample(t)
	waitlist := tacoWaitlist(t, s, queue, "2")
	if _, err := write(parent,
		Edit{Insert: "PAYMENT", Row: map[string]any{"person": parent, "for": avery, "group": tacoNight, "amount": "40", "added": "2026-10-15 09:00:00"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": tacoNight, "person": avery, "member": "yes", "added_by": parent, "added": "2026-10-15 09:00:00"}},
		Edit{Insert: "PERSON", As: "g", Row: map[string]any{"source": "guest", "name_long_override": "Robin Guest"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": waitlist, "person": "@g", "member": "yes", "added_by": parent, "guest_of": parent, "added": "2026-10-15 09:00:00"}},
	); err != nil {
		t.Fatalf("someone can't take the last place and wait for another: %v", err)
	}
	if _, ok := s.Model().Table("MEMBER").Find(tacoNight, avery); !ok {
		t.Error("the place taken isn't there")
	}
	if _, err := write(parent,
		Edit{Insert: "PERSON", As: "h", Row: map[string]any{"source": "guest", "name_long_override": "Sam Guest"}},
		Edit{Insert: "PAYMENT", Row: map[string]any{"person": parent, "for": "@h", "group": tacoNight, "amount": "40", "added": "2026-10-15 09:00:00"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": tacoNight, "person": "@h", "member": "yes", "added_by": parent, "guest_of": parent}},
	); err == nil {
		t.Error("someone took a place at a full party")
	}
}

func TestAFamilyLeavesTheWaitlistWithItsExtraPlaces(t *testing.T) {
	s, queue, write := celebrateSample(t)
	waitlist := tacoWaitlist(t, s, queue, "1")
	made, err := write(parent,
		Edit{Insert: "MEMBER", Row: map[string]any{"group": waitlist, "person": parent, "member": "yes", "added_by": parent, "note": "Two if we can", "added": "2026-10-15 09:00:00"}},
		Edit{Insert: "PERSON", As: "w0", Row: map[string]any{"source": "guest"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": waitlist, "person": "@w0", "member": "yes", "added_by": parent, "guest_of": parent, "added": "2026-10-15 09:00:00"}},
	)
	if err != nil {
		t.Fatalf("someone can't ask for two places: %v", err)
	}
	if _, err := write(parent, Edit{Delete: made[0]}, Edit{Delete: made[2]}); err != nil {
		t.Fatalf("someone can't leave the waitlist with their extra place: %v", err)
	}
}

func TestATicketPassesToANewGuestWithAnAddress(t *testing.T) {
	s, _, write := celebrateSample(t)
	if _, err := write(parent,
		Edit{Insert: "PERSON", As: "g", Row: map[string]any{"source": "guest", "name_long_override": "Robin Guest"}},
		Edit{Insert: "PERSON_EMAIL", Row: map[string]any{"person": "@g", "address": "robin.guest@example.com", "primary": true, "source": "guest"}},
		Edit{Set: "mem00000000025", Cells: map[string]any{"person": "@g"}},
	); err != nil {
		t.Fatalf("someone can't pass their ticket to a guest: %v", err)
	}
	if m, _ := s.Model().Table("MEMBER").Get("mem00000000025"); m["person"] == parent {
		t.Error("the ticket still reads its old holder")
	}
}

func TestAnyGuestMayBeBoughtAPlace(t *testing.T) {
	s, queue, write := celebrateSample(t)
	made := apply(t, s, queue, []Edit{
		{Insert: "GROUP", As: "elsewhere", Row: map[string]any{"kind": "group", "name": "Somewhere Hidden", "status": "open"}},
		{Insert: "PERSON", As: "g", Row: map[string]any{"source": "guest", "name_long_override": "Isaac Guest"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@elsewhere", "person": "@g", "member": "yes", "guest_of": staff}},
	})
	isaac := made[1]
	if len(as(t, s, parent, `(from PERSON (where (= id "`+isaac+`")))`)) != 0 {
		t.Fatal("the parent sees a guest they shouldn't")
	}
	if _, err := write(parent,
		Edit{Insert: "PAYMENT", Row: map[string]any{"person": parent, "for": isaac, "group": tacoNight, "amount": "40", "added": "2026-10-15 09:00:00"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": tacoNight, "person": isaac, "member": "yes", "added_by": parent, "guest_of": parent, "added": "2026-10-15 09:00:00"}},
	); err != nil {
		t.Fatalf("someone can't buy a place for a guest another family brought: %v", err)
	}
}

func TestDeletingARowTakesItsAliases(t *testing.T) {
	s, queue, write := celebrateSample(t)
	waitlist := tacoWaitlist(t, s, queue, "1")
	made := apply(t, s, queue, []Edit{
		{Insert: "MEMBER", As: "w", Row: map[string]any{"group": waitlist, "person": guest, "member": "yes", "added": "2026-03-08 09:53"}},
		{Insert: "ALIAS", Row: map[string]any{"alias": "t9mqt6y0rw8ac", "target": "@w", "hidden": true}},
	})
	if _, err := write(staff,
		Edit{Delete: made[0]},
		Edit{Insert: "PAYMENT", Row: map[string]any{"person": guest, "for": guest, "group": tacoNight, "amount": "40", "added": "2026-10-16 09:00:00"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": tacoNight, "person": guest, "member": "yes", "added_by": staff}},
	); err != nil {
		t.Fatalf("a host can't give a place to someone waiting under an old ID: %v", err)
	}
	if _, ok := s.Model().Table("ALIAS").Get(made[1]); ok {
		t.Error("the deleted place's old ID still names it")
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
