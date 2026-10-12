package db

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/store"
	"heliosian/internal/testkit/mailtest"
)

func TestASignUpMailsTheVolunteerTheirChairsAndWhoeverIsNotified(t *testing.T) {
	const year, night, decor, superAdmins = "grp00000000510", "grp00000000512", "grp00000000900", "grp00000000005"
	s, queue := sampleWithQueue(t)
	if err := commit(s, GroupsSheet,
		store.Update("GROUP", store.Row{"id": year}, store.Row{"notify": superAdmins}),
		store.Insert("GROUP", store.Row{"id": decor, "parent": night, "kind": "activity", "status": "open", "name": "Decor", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004", "posting": "members", "replying": "members", "join": "others"}),
	); err != nil {
		t.Fatal(err)
	}
	sender := mailtest.NewRecorder(mailtest.From)
	NewMembershipMailer(s, queue, MembershipMailConfig{Sender: sender.Mailgun, Origin: func(app string) string { return "https://" + app + ".test" }})
	write := func(env Env, edits ...Edit) []string {
		t.Helper()
		ids, err := Write(context.Background(), s, queue, access.Actor{Email: "juni.ashdown@example.org"}, env, Batch{Batch: edits})
		if err != nil {
			t.Fatal(err)
		}
		return ids
	}

	imported := write(Env{Viewer: staff, Mode: "import", Now: testNow}, Edit{Insert: "MEMBER", Row: map[string]any{"group": decor, "person": staff, "member": "yes"}})
	joined := write(Env{Viewer: student, Now: testNow}, Edit{Insert: "MEMBER", Row: map[string]any{"group": decor, "person": student, "member": "yes", "added_by": student, "note": "I can bring tape"}})

	messages := sender.Wait(t, 3)
	subjects := []string{}
	for _, m := range messages {
		subjects = append(subjects, m.Subject)
	}
	slices.Sort(subjects)
	if want := []string{"Calendar invite: Decor", "New sign-up: Juni Ashdown for Decor", "Thanks for volunteering for Decor"}; !slices.Equal(subjects, want) {
		t.Fatalf("a sign-up sent %q, want %q", subjects, want)
	}
	for _, m := range messages {
		switch m.Subject {
		case "Thanks for volunteering for Decor":
			if !slices.Contains(m.To, "rowan.ashdown@example.org") || !strings.Contains(m.Text, "I can bring tape") || !strings.Contains(m.Text, "Event Chairs: Rowan Ashdown") {
				t.Errorf("the thank-you goes to %q and says:\n%s", m.To, m.Text)
			}
		case "Calendar invite: Decor":
			if len(m.Attachments) != 1 || !strings.Contains(string(m.Attachments[0].Content), "METHOD:REQUEST") {
				t.Errorf("the calendar invite carries %d attachments", len(m.Attachments))
			}
		case "New sign-up: Juni Ashdown for Decor":
			if len(m.To) != 1 || m.To[0] != "maya.lindqvist@example.org" {
				t.Errorf("the notice goes to %q, want the year's notify group", m.To)
			}
		}
	}

	write(Env{Viewer: student, Now: testNow}, Edit{Delete: joined[0]})
	removed := sender.Wait(t, 4)[3]
	if removed.Subject != "Removed: Decor" || len(removed.Attachments) != 1 || !strings.Contains(string(removed.Attachments[0].Content), "METHOD:CANCEL") {
		t.Errorf("removing a sign-up sent %q with %d attachments", removed.Subject, len(removed.Attachments))
	}
	if got := len(sender.Messages()); got != 4 {
		t.Errorf("sent %d messages, want 4: the import's sign-up sends none", got)
	}

	write(Env{Viewer: parent, Now: testNow}, Edit{Set: imported[0], Cells: map[string]any{"lead": true}})
	cochair := sender.Wait(t, 5)[4]
	if cochair.Subject != "You're a co-chair of Decor" || len(cochair.To) != 1 || cochair.To[0] != "maya.lindqvist@example.org" || !slices.Contains(cochair.CC, "rowan.ashdown@example.org") {
		t.Errorf("making someone a co-chair sent %q to %q, copying %q", cochair.Subject, cochair.To, cochair.CC)
	}

	recorded := func() int {
		n := 0
		for _, m := range s.Model().Table("MESSAGE").All() {
			if m["group"] == decor && m["direction"] == "out" {
				n++
			}
		}
		return n
	}
	deadline := time.Now().Add(5 * time.Second)
	for recorded() < 5 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := recorded(); got != 5 {
		t.Errorf("recorded %d messages, want 5", got)
	}
}

func TestAnOfferToCoChairNotifiesAndThanksNobody(t *testing.T) {
	const year, night, decor, superAdmins = "grp00000000510", "grp00000000512", "grp00000000900", "grp00000000005"
	s, queue := sampleWithQueue(t)
	if err := commit(s, GroupsSheet,
		store.Update("GROUP", store.Row{"id": year}, store.Row{"notify": superAdmins}),
		store.Insert("GROUP", store.Row{"id": decor, "parent": night, "kind": "activity", "status": "open", "name": "Decor", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004", "posting": "members", "replying": "members", "join": "others"}),
		store.Insert("MEMBER", store.Row{"id": "mem00000000900", "group": decor, "person": student, "member": "yes"}),
	); err != nil {
		t.Fatal(err)
	}
	sender := mailtest.NewRecorder(mailtest.From)
	NewMembershipMailer(s, queue, MembershipMailConfig{Sender: sender.Mailgun, Origin: func(app string) string { return "https://" + app + ".test" }})
	write := func(viewer string, edits ...Edit) []string {
		t.Helper()
		ids, err := Write(context.Background(), s, queue, access.Actor{Email: "juni.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
		if err != nil {
			t.Fatal(err)
		}
		return ids
	}
	opened := write(parent,
		Edit{Insert: "GROUP", As: "o", Row: map[string]any{"parent": decor, "kind": "group", "status": "open", "name": "Decor Co-Chair Offers", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004", "posting": "none", "replying": "none", "join": "direct", "eligible": decor, "added_by": parent}},
		Edit{Set: decor, Cells: map[string]any{"chair_offers": "@o"}},
	)
	write(student, Edit{Insert: "MEMBER", Row: map[string]any{"group": opened[0], "person": student, "member": "yes", "added_by": student}})
	notice := sender.Wait(t, 1)[0]
	if notice.Subject != "Offer to co-chair: Juni Ashdown for Decor" || len(notice.To) != 1 || notice.To[0] != "maya.lindqvist@example.org" {
		t.Errorf("an offer sent %q to %q", notice.Subject, notice.To)
	}
	time.Sleep(100 * time.Millisecond)
	if got := len(sender.Messages()); got != 1 {
		t.Errorf("an offer sent %d messages, want the notice alone", got)
	}
}
