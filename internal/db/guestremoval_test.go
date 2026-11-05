package db

import (
	"net/http"
	"strings"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

func TestGuestRemovalUsesOrdinaryGuestPermissions(t *testing.T) {
	for _, kind := range guestListKinds {
		for _, relation := range []string{"self", "household", "brought"} {
			t.Run(kind+"/"+relation, func(t *testing.T) {
				h := newInviteHarness(t)
				h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": kind, "eligible": "", "guests_invited_by": "grp00000000004"}}})
				person := parent
				if relation == "household" {
					person = student
				}
				if relation == "brought" {
					person = guest
				}
				row := map[string]any{"group": h.event, "person": person, "member": "yes", "opened": "2026-11-03 12:00"}
				if kind == "activity" {
					row["member"], row["invited"] = "", "yes"
				}
				if relation == "brought" {
					row["guest_of"] = parent
				}
				h.setup([]Edit{{Insert: "MEMBER", Row: row}})
				h.setup(invitationEdits(h.event, staff, "skip", []string{person}, testNow))
				h.setup(h.s.Model().answerEdits(h.group(), person, "yes", parent, testNow))
				if h.s.Model().superAdmin(parent) || h.s.Model().runsGroup(h.event, parent) {
					t.Fatal("the caller must be an ordinary guest")
				}
				if relation == "brought" {
					q := mustParse(t, `(from RECIPIENT (where (= person "`+person+`")) (columns id))`)
					if result := h.s.Model().Run(t.Context(), q, Env{Viewer: parent, Now: testNow}); result.Err != nil || len(result.IDs) != 0 {
						t.Fatalf("the bringer can read another sender's copy: %+v", result)
					}
				}
				rec := h.do(rowan, http.MethodPost, "/api/do/remove-guest", removeGuestAsk{Group: h.event, Person: person})
				if rec.Code != http.StatusOK {
					t.Fatalf("remove guest: %d %s", rec.Code, rec.Body)
				}
				left, ok := h.s.Model().Table("MEMBER").Find(h.event, person)
				if !ok || left["opened"] != row["opened"] || h.s.Model().onList(h.group(), person) || h.s.Model().answerOf(h.group(), person) != "" {
					t.Fatalf("guest not removed with their open stamp preserved: %v", left)
				}
			})
		}
	}
}

func TestGuestRemovalRejectsOtherPeopleAndPaidPlaces(t *testing.T) {
	for _, kind := range guestListKinds {
		for _, state := range []string{"none", "opened", "member", "paid"} {
			t.Run(kind+"/"+state, func(t *testing.T) {
				h := newInviteHarness(t)
				h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": kind, "eligible": "", "guests_invited_by": "grp00000000004"}}})
				person := guest
				if state == "paid" {
					person = parent
					h.setup([]Edit{{Set: h.event, Cells: map[string]any{"price": "5"}}})
				}
				if state != "none" {
					row := map[string]any{"group": h.event, "person": person, "opened": "2026-11-03 12:00"}
					if state != "opened" {
						row["member"] = "yes"
					}
					h.setup([]Edit{{Insert: "MEMBER", Row: row}})
				}
				rec := h.do(rowan, http.MethodPost, "/api/do/remove-guest", removeGuestAsk{Group: h.event, Person: person})
				if rec.Code != http.StatusForbidden && rec.Code != http.StatusConflict {
					t.Fatalf("unavailable removal accepted: %d %s", rec.Code, rec.Body)
				}
			})
		}
	}
}

func TestGuestRemovalAndInvitationOpensShareTheTransaction(t *testing.T) {
	for _, kind := range guestListKinds {
		for _, first := range []string{"open", "remove"} {
			t.Run(kind+"/"+first, func(t *testing.T) {
				h := newInviteHarness(t)
				h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": kind, "eligible": "", "guests_invited_by": "grp00000000004"}}})
				h.setup([]Edit{{Insert: "RULE", Row: map[string]any{"group": h.event, "person": parent, "order": "a"}}})
				open := Edit{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": parent, "opened": "2026-11-03 12:00"}}
				entered, release := make(chan struct{}), make(chan struct{})
				firstDone, secondDone := make(chan error, 1), make(chan error, 1)
				go func() {
					_, err := h.s.Transact(t.Context(), access.Actor{Email: maya}, func(tx *store.Tx[Model]) error {
						var err error
						if first == "open" {
							_, err = stageEdits(h.s, tx, Env{Viewer: parent, Now: testNow}, []Edit{open}, "open", false)
						} else {
							_, err = stageGuestRemoval(h.s, tx, Env{Viewer: staff, Now: testNow}, h.group(), parent)
						}
						close(entered)
						<-release
						return err
					})
					firstDone <- err
				}()
				<-entered
				go func() {
					if first == "open" {
						_, err := (&Invitations{s: h.s}).removeGuest(t.Context(), h.s.Model(), Env{Viewer: staff, Now: testNow}, access.Actor{Email: maya}, removeGuestAsk{Group: h.event, Person: parent})
						secondDone <- err
						return
					}
					_, err := Write(t.Context(), h.s, access.Actor{Email: rowan}, Env{Viewer: parent, Now: testNow}, Batch{Batch: []Edit{open}})
					secondDone <- err
				}()
				close(release)
				for _, done := range []<-chan error{firstDone, secondDone} {
					if err := <-done; err != nil {
						t.Fatal(err)
					}
				}
				rows := h.s.Model().Table("MEMBER").Referencing("group", h.event)
				if len(rows) != 1 || rows[0]["opened"] != "2026-11-03 12:00" || h.s.Model().onList(h.group(), parent) {
					t.Fatalf("concurrent open/removal lost state: %v", rows)
				}
			})
		}
	}
}

func TestAnOpenedInsertPreservesMembershipAndChecksItsRequest(t *testing.T) {
	h := newInviteHarness(t)
	id := h.setup([]Edit{{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": parent, "member": "excluded", "opened": "2026-11-03 12:00"}}})[0]
	write := func(row map[string]any) ([]string, error) {
		row["group"], row["person"], row["opened"] = h.event, parent, "2026-11-04 12:00"
		return Write(t.Context(), h.s, access.Actor{Email: rowan}, Env{Viewer: parent, Now: testNow}, Batch{Batch: []Edit{{Insert: "MEMBER", Row: row}}})
	}
	if ids, err := write(map[string]any{}); err != nil || len(ids) != 1 || ids[0] != id {
		t.Fatalf("duplicate open failed: %v %v", ids, err)
	}
	for _, row := range []map[string]any{{"guest_of": parent}, {"note": "forged"}, {"added_by": staff}} {
		if _, err := write(row); err == nil {
			t.Fatalf("forged open accepted: %v", row)
		}
	}
	row, _ := h.s.Model().Table("MEMBER").Get(id)
	if row["member"] != "excluded" || row["opened"] != "2026-11-03 12:00" {
		t.Fatalf("opening changed an exclusion or first-open time: %v", row)
	}
}

func TestGuestRemovalClearsAnswersBeforeLeavingAVisibleGroup(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{
		{Set: h.event, Cells: map[string]any{"visible_to": h.event}},
		{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": parent, "member": "yes"}},
	})
	h.setup(h.s.Model().answerEdits(h.group(), parent, "yes", parent, testNow))
	rec := h.do(rowan, http.MethodPost, "/api/do/remove-guest", removeGuestAsk{Group: h.event, Person: parent})
	if rec.Code != http.StatusOK || h.s.Model().onList(h.group(), parent) || h.s.Model().answerOf(h.group(), parent) != "" {
		t.Fatalf("leaving retained the answer or failed: %d %s", rec.Code, rec.Body)
	}
}

func TestGuestRemovalExcludesRuleMembersWithoutAMembershipRow(t *testing.T) {
	for _, kind := range guestListKinds {
		for _, person := range []string{parent, student} {
			t.Run(kind+"/"+person, func(t *testing.T) {
				h := newInviteHarness(t)
				h.setup([]Edit{
					{Set: h.event, Cells: map[string]any{"kind": kind, "eligible": "", "guests_invited_by": "grp00000000004"}},
					{Insert: "RULE", Row: map[string]any{"group": h.event, "person": person, "order": "a"}},
				})
				h.setup(invitationEdits(h.event, staff, "skip", []string{person}, testNow))
				rec := h.do(rowan, http.MethodPost, "/api/do/remove-guest", removeGuestAsk{Group: h.event, Person: person})
				if rec.Code != http.StatusOK || h.s.Model().onList(h.group(), person) {
					t.Fatalf("rule member could not leave: %d %s", rec.Code, rec.Body)
				}
			})
		}
	}
}

func TestOutsideGuestRemovalClearsTheirGuestsAnswers(t *testing.T) {
	for _, kind := range []string{"event", "party"} {
		for _, answer := range []string{"yes", "no"} {
			t.Run(kind+"/"+answer, func(t *testing.T) {
				h := newInviteHarness(t)
				h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": kind, "eligible": "", "capacity": "20", "price": ""}}})
				if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", addGuestsAsk{Group: h.event, Send: true, People: []guestAsk{{Name: "Coach Pat", Email: "coach@example.net"}}}); rec.Code != http.StatusOK {
					t.Fatalf("invite coach: %d %s", rec.Code, rec.Body)
				}
				coach := h.s.Model().personAt("coach@example.net")
				path := "/open/ext/" + h.copyOf(coach)["token"]
				if rec := h.do("", http.MethodPost, path+"/guest", map[string]string{"name": "Friend", "email": "friend@example.net"}); rec.Code != http.StatusNoContent {
					t.Fatalf("bring friend: %d %s", rec.Code, rec.Body)
				}
				friend := h.s.Model().personAt("friend@example.net")
				if rec := h.do("", http.MethodPost, path, map[string]string{"key": friend, "answer": answer}); rec.Code != http.StatusNoContent {
					t.Fatalf("answer for friend: %d %s", rec.Code, rec.Body)
				}
				if rec := h.do("", http.MethodDelete, path+"/guest", map[string]string{"key": friend}); rec.Code != http.StatusNoContent {
					t.Fatalf("remove answered friend: %d %s", rec.Code, rec.Body)
				}
				if h.s.Model().onList(h.group(), friend) || h.s.Model().answerOf(h.group(), friend) != "" {
					t.Fatal("removal retained guest or RSVP")
				}
				if rec := h.do("", http.MethodPost, path+"/guest", map[string]string{"name": "Friend", "email": "friend@example.net"}); rec.Code != http.StatusNoContent || !h.s.Model().onList(h.group(), friend) {
					t.Fatalf("restore friend: %d %s", rec.Code, rec.Body)
				}
			})
		}
	}
}

func TestBroughtGuestsCanReturnOnlyAfterTheirOwnRemoval(t *testing.T) {
	for _, kind := range []string{"event", "party"} {
		for _, entry := range []string{"outside", "signed in"} {
			t.Run(kind+"/"+entry, func(t *testing.T) {
				h := newInviteHarness(t)
				h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": kind, "eligible": "", "capacity": "20", "price": ""}}})
				bringer, path := parent, ""
				if entry == "outside" {
					if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", addGuestsAsk{Group: h.event, Send: true, People: []guestAsk{{Name: "Coach Pat", Email: "coach@example.net"}}}); rec.Code != http.StatusOK {
						t.Fatalf("invite coach: %d %s", rec.Code, rec.Body)
					}
					bringer = h.s.Model().personAt("coach@example.net")
					path = "/open/ext/" + h.copyOf(bringer)["token"] + "/guest"
				}
				add := func() int {
					if entry == "outside" {
						return h.do("", http.MethodPost, path, map[string]string{"name": "Friend", "email": "friend@example.net"}).Code
					}
					return h.do(rowan, http.MethodPost, "/api/do/add-guests", addGuestsAsk{Group: h.event, GuestOf: bringer, People: []guestAsk{{Name: "Friend", Email: "friend@example.net"}}}).Code
				}
				if code := add(); code != http.StatusOK && code != http.StatusNoContent {
					t.Fatalf("bring friend: %d", code)
				}
				friend := h.s.Model().personAt("friend@example.net")
				original, _ := h.s.Model().Table("MEMBER").Find(h.event, friend)
				code := 0
				if entry == "outside" {
					code = h.do("", http.MethodDelete, path, map[string]string{"key": friend}).Code
				} else {
					code = h.do(rowan, http.MethodPost, "/api/do/remove-guest", removeGuestAsk{Group: h.event, Person: friend}).Code
				}
				removed, _ := h.s.Model().Table("MEMBER").Find(h.event, friend)
				if code != http.StatusOK && code != http.StatusNoContent || removed["member"] != "cancelled" || h.s.Model().onList(h.group(), friend) {
					t.Fatalf("own removal failed: %d %v", code, removed)
				}
				if code := add(); code != http.StatusOK && code != http.StatusNoContent {
					t.Fatalf("restore friend: %d", code)
				}
				restored, _ := h.s.Model().Table("MEMBER").Find(h.event, friend)
				if restored["id"] != original["id"] || !h.s.Model().onList(h.group(), friend) {
					t.Fatalf("restoring did not reuse the membership: %v", restored)
				}
				if rec := h.do(maya, http.MethodPost, "/api/do/remove-guest", removeGuestAsk{Group: h.event, Person: friend}); rec.Code != http.StatusOK {
					t.Fatalf("manager removal: %d %s", rec.Code, rec.Body)
				}
				add()
				blocked, _ := h.s.Model().Table("MEMBER").Find(h.event, friend)
				if blocked["member"] != "excluded" || h.s.Model().onList(h.group(), friend) {
					t.Fatalf("a bringer undid a manager exclusion: %v", blocked)
				}
				for _, edit := range []Edit{{Set: blocked["id"], Cells: map[string]any{"member": "cancelled"}}, {Set: blocked["id"], Cells: map[string]any{"member": "yes"}}, {Delete: blocked["id"]}} {
					if _, err := Write(t.Context(), h.s, access.Actor{Email: "bringer@example.net"}, Env{Viewer: bringer, Now: testNow}, Batch{Batch: []Edit{edit}}); err == nil {
						t.Fatalf("direct write undid a manager exclusion: %+v", edit)
					}
				}
			})
		}
	}
}

func TestGuestRestorationUsesCurrentJoiningRules(t *testing.T) {
	for _, kind := range []string{"event", "party"} {
		for _, blocked := range []string{"price", "eligibility", "capacity"} {
			t.Run(kind+"/"+blocked, func(t *testing.T) {
				h := newInviteHarness(t)
				h.setup([]Edit{
					{Set: h.event, Cells: map[string]any{"kind": kind, "eligible": "", "capacity": "1", "price": ""}},
					{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": guest, "member": "cancelled", "guest_of": parent}},
				})
				people := []guestAsk{{Person: guest}}
				switch blocked {
				case "price":
					h.setup([]Edit{{Set: h.event, Cells: map[string]any{"price": "5"}}})
				case "eligibility":
					h.setup([]Edit{{Set: h.event, Cells: map[string]any{"eligible": "grp00000000003"}}})
				case "capacity":
					ids := h.setup([]Edit{
						{Insert: "PERSON", Row: map[string]any{"source": "guest", "name_long_override": "Another friend"}, As: "friend"},
						{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": "@friend", "member": "cancelled", "guest_of": parent}},
					})
					people = append(people, guestAsk{Person: ids[0]})
				}
				rec := h.do(rowan, http.MethodPost, "/api/do/add-guests", addGuestsAsk{Group: h.event, People: people})
				if rec.Code == http.StatusOK {
					t.Fatalf("restoration bypassed %s: %s", blocked, rec.Body)
				}
				for _, p := range people {
					row, _ := h.s.Model().Table("MEMBER").Find(h.event, p.Person)
					if row["member"] != "cancelled" || h.s.Model().onList(h.group(), p.Person) {
						t.Fatalf("a refused restoration changed membership: %v", row)
					}
				}
			})
		}
	}
}

func TestCancelledGuestMembershipIsConsistentWithEffectiveMembershipAndDrafts(t *testing.T) {
	for _, kind := range guestListKinds {
		t.Run(kind, func(t *testing.T) {
			h := newInviteHarness(t)
			h.setup([]Edit{
				{Set: h.event, Cells: map[string]any{"kind": kind}},
				{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": parent, "member": "cancelled"}},
				{Insert: "RULE", Row: map[string]any{"group": h.event, "person": parent, "order": "a"}},
			})
			draft, err := h.s.Model().draftMembers(t.Context(), Env{Viewer: staff, Now: testNow}, groupDraft{List: h.event, Rules: []draftRule{{Person: parent}}, Members: []draftMember{{Person: parent, Member: "cancelled"}}})
			want := kind == "activity"
			if err != nil || h.s.Model().effectivelyIn(h.event, parent) != want || h.s.Model().onList(h.group(), parent) != want || (len(draft.Members) == 1) != want {
				t.Fatalf("cancelled membership is inconsistent: kind=%s draft=%+v err=%v", kind, draft, err)
			}
		})
	}
}

func TestTicketInsertsRestoreRetainedPartyMemberships(t *testing.T) {
	for _, removal := range []string{"guest", "host"} {
		for _, viewer := range []string{parent, staff} {
			for _, price := range []string{"", "40"} {
				t.Run(removal+"/"+viewer+"/"+price, func(t *testing.T) {
					h := newInviteHarness(t)
					h.setup([]Edit{
						{Delete: "mem00000000007"},
						{Set: h.event, Cells: map[string]any{"kind": "party", "eligible": "", "capacity": "20"}},
						{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": parent, "member": "yes", "opened": "2026-11-03 12:00"}},
					})
					original, _ := h.s.Model().Table("MEMBER").Find(h.event, parent)
					remover := rowan
					if removal == "host" {
						remover = maya
					}
					if rec := h.do(remover, http.MethodPost, "/api/do/remove-guest", removeGuestAsk{Group: h.event, Person: parent}); rec.Code != http.StatusOK {
						t.Fatalf("remove ticket: %d %s", rec.Code, rec.Body)
					}
					h.setup([]Edit{{Set: h.event, Cells: map[string]any{"price": price}}})
					edits := []Edit{}
					if price != "" {
						edits = append(edits, Edit{Insert: "PAYMENT", Row: map[string]any{"group": h.event, "person": parent, "for": parent, "amount": price, "added": stamp(testNow)}})
					}
					edits = append(edits, Edit{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": parent, "member": "yes", "added_by": viewer, "added": stamp(testNow)}})
					_, err := Write(t.Context(), h.s, access.Actor{Email: rowan}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
					blocked := removal == "host" && viewer == parent
					if blocked {
						if err == nil || strings.Contains(err.Error(), "two rows have the same") {
							t.Fatalf("manager exclusion needs a permission refusal: %v", err)
						}
						if len(h.s.Model().Table("PAYMENT").Referencing("group", h.event)) != 0 {
							t.Fatal("refused ticket left a payment")
						}
					} else if err != nil {
						t.Fatalf("restore ticket: %v", err)
					}
					row, _ := h.s.Model().Table("MEMBER").Find(h.event, parent)
					if row["id"] != original["id"] || row["opened"] != original["opened"] || h.s.Model().onList(h.group(), parent) == blocked {
						t.Fatalf("restoration lost identity or state: %v", row)
					}
				})
			}
		}
	}
}

func TestCancelledGuestMembershipCannotBeDeletedByGuests(t *testing.T) {
	for _, kind := range []string{"event", "party"} {
		for _, person := range []string{parent, student, guest} {
			t.Run(kind+"/"+person, func(t *testing.T) {
				h := newInviteHarness(t)
				h.setup([]Edit{
					{Delete: "mem00000000007"},
					{Set: h.event, Cells: map[string]any{"kind": kind, "eligible": ""}},
					{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": person, "member": "yes", "guest_of": parent}},
				})
				h.setup(invitationEdits(h.event, staff, "skip", []string{person}, testNow))
				if rec := h.do(rowan, http.MethodPost, "/api/do/remove-guest", removeGuestAsk{Group: h.event, Person: person}); rec.Code != http.StatusOK {
					t.Fatalf("remove guest: %d %s", rec.Code, rec.Body)
				}
				row, _ := h.s.Model().Table("MEMBER").Find(h.event, person)
				if row["member"] != "cancelled" {
					t.Fatalf("expected voluntary departure: %v", row)
				}
				batch := Batch{Batch: []Edit{{Delete: row["id"]}}}
				if _, err := Write(t.Context(), h.s, access.Actor{Email: rowan}, Env{Viewer: parent, Now: testNow}, batch); err == nil {
					t.Fatal("a guest deleted a retained departure")
				}
				if h.s.Model().onList(h.group(), person) {
					t.Fatal("deletion put the guest back through their invitation")
				}
				if _, err := Write(t.Context(), h.s, access.Actor{Email: maya}, Env{Viewer: staff, Now: testNow}, batch); err != nil || !h.s.Model().onList(h.group(), person) {
					t.Fatalf("a host cannot clear the departure: %v", err)
				}
			})
		}
	}
}

func TestMembershipInsertsRecheckJoiningForCancelledGuests(t *testing.T) {
	for _, kind := range []string{"event", "party"} {
		for _, blocked := range []string{"price", "eligibility", "capacity"} {
			t.Run(kind+"/"+blocked, func(t *testing.T) {
				h := newInviteHarness(t)
				h.setup([]Edit{
					{Set: h.event, Cells: map[string]any{"kind": kind, "join": "direct", "eligible": "", "capacity": "1"}},
					{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": parent, "member": "cancelled"}},
					{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": student, "member": "cancelled"}},
				})
				switch blocked {
				case "price":
					h.setup([]Edit{{Set: h.event, Cells: map[string]any{"price": "40"}}})
				case "eligibility":
					h.setup([]Edit{{Set: h.event, Cells: map[string]any{"eligible": "grp00000000003"}}})
				}
				edits := []Edit{{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": parent, "member": "yes", "added_by": parent}}}
				if blocked == "capacity" {
					edits = append(edits, Edit{Insert: "MEMBER", Row: map[string]any{"group": h.event, "person": student, "member": "yes", "added_by": parent}})
				}
				_, err := Write(t.Context(), h.s, access.Actor{Email: rowan}, Env{Viewer: parent, Now: testNow}, Batch{Batch: edits})
				if err == nil || strings.Contains(err.Error(), "two rows have the same") {
					t.Fatalf("restoring by insert needs a permission refusal for %s: %v", blocked, err)
				}
				for _, person := range []string{parent, student} {
					row, _ := h.s.Model().Table("MEMBER").Find(h.event, person)
					if row["member"] != "cancelled" || h.s.Model().onList(h.group(), person) {
						t.Fatalf("refused inserts changed a departure: %v", row)
					}
				}
			})
		}
	}
}
