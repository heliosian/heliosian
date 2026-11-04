package db

import (
	"context"
	"fmt"
	"html"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/data"
	"heliosian/internal/testkit/mailtest"
)

func queuedInvitations(t *testing.T, people ...string) (*Invitations, *data.Dir, *mailtest.Recorder, string) {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	s, err := NewStore(dir, dir, NewSearchIndex(), testOrigin)
	if err != nil {
		t.Fatal(err)
	}
	ids := apply(t, s, []Edit{{Insert: "GROUP", Row: map[string]any{"kind": "event", "name": "Movie Night", "status": "open", "start": time.Now().AddDate(0, 0, 30).Format("2006-01-02") + " 18:00", "visible_to": "grp00000000004"}}})
	group := ids[0]
	apply(t, s, invitationEdits(group, staff, "invitation", people, time.Now()))
	recorder := mailtest.NewRecorder(mailtest.From)
	iv := &Invitations{s: s, pics: newPictures(s), config: InvitationConfig{Sender: recorder.Mailgun, ReplyTo: whenReply, Origin: testOrigin}}
	return iv, dir, recorder, group
}

func TestInvitationWording(t *testing.T) {
	for _, hosted := range []bool{false, true} {
		iv, _, _, group := queuedInvitations(t, parent)
		if hosted {
			apply(t, iv.s, []Edit{
				{Insert: "GROUP", As: "hosts", Row: map[string]any{"kind": "group", "name": "Movie Night Hosts", "status": "open"}},
				{Insert: "MEMBER", Row: map[string]any{"group": "@hosts", "person": staff, "member": "yes"}},
				{Set: group, Cells: map[string]any{"managed_by": "@hosts"}},
			})
		}
		m := iv.s.Model()
		g, _ := m.Table("GROUP").Get(group)
		for _, kind := range []string{"invitation", "update"} {
			t.Run(fmt.Sprintf("%s/hosted=%t", kind, hosted), func(t *testing.T) {
				msg := iv.invitation(m, g, kind, parent, "https://when.test/event", true)
				intro := "You're invited to"
				if kind == "update" {
					intro = "The details have been updated for"
				}
				for format, body := range map[string]string{"text": msg.Text, "html": html.UnescapeString(msg.HTML)} {
					if !strings.Contains(body, intro) || strings.Contains(body, m.nameOf(staff)+" sent ") || strings.Contains(body, m.nameOf(staff)+" has updated ") {
						t.Errorf("%s intro: %s", format, body)
					}
					if strings.Contains(body, "Hosted by ") != hosted {
						t.Errorf("%s hosted=%t: %s", format, hosted, body)
					}
					if hosted && !strings.Contains(body, "Hosted by "+m.nameOf(staff)) {
						t.Errorf("%s missing host name: %s", format, body)
					}
				}
			})
		}
	}
}

func TestQueuedInvitationsPersistProgressInBatches(t *testing.T) {
	iv, dir, recorder, group := queuedInvitations(t)
	people := []string{}
	for _, person := range iv.s.Model().Table("PERSON").All() {
		if iv.s.Model().addressOf(person["id"]) != "" {
			people = append(people, person["id"])
		}
		if len(people) == inviteProgressBatch+1 {
			break
		}
	}
	if len(people) != inviteProgressBatch+1 {
		t.Fatal("not enough addressed people in the sample")
	}
	apply(t, iv.s, invitationEdits(group, staff, "invitation", people, time.Now()))
	copies := iv.s.Model().copiesOf(group)
	calls := 0
	recorder.BeforeSend = func(raw mailtest.Raw) error {
		calls++
		if calls != 2 && calls != inviteProgressBatch+1 {
			return nil
		}
		_, rows, err := dir.Table("datamail", "RECIPIENT")
		if err != nil {
			return err
		}
		persisted := 0
		for _, row := range rows {
			if copy, ok := copies[row["person"]]; ok && row["id"] == copy["id"] && row["sent"] != "" {
				persisted++
			}
		}
		want := 0
		if calls > inviteProgressBatch {
			want = inviteProgressBatch
		}
		if persisted != want {
			t.Errorf("before send %d, persisted %d copies; want %d", calls, persisted, want)
		}
		return nil
	}
	iv.sendQueued(context.Background())
	iv.sendQueued(context.Background())
	if len(recorder.Raws()) != inviteProgressBatch+1 {
		t.Fatalf("sent %d copies", len(recorder.Raws()))
	}
}

func TestQueuedInvitationsStopWhenTheEventCloses(t *testing.T) {
	for _, status := range []string{"closed", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			iv, _, recorder, group := queuedInvitations(t, parent, staff)
			recorder.BeforeSend = func(raw mailtest.Raw) error {
				apply(t, iv.s, []Edit{{Set: group, Cells: map[string]any{"status": status}}})
				return nil
			}
			iv.sendQueued(context.Background())
			if len(recorder.Raws()) != 1 {
				t.Errorf("sent %d copies after the first send closed the event", len(recorder.Raws()))
			}
			for _, copy := range iv.s.Model().copiesOf(group) {
				if copy["sent"] == "" && copy["failed"] == "" {
					t.Errorf("an unsent invitation remains queued after the event %s", status)
				}
			}
		})
	}
}

func TestQueuedInvitationsRetryOnlyTheRefusedCopy(t *testing.T) {
	iv, _, recorder, _ := queuedInvitations(t, parent, staff)
	refused := ""
	recorder.BeforeSend = func(raw mailtest.Raw) error {
		if refused == "" {
			refused = raw.Recipient
		}
		if raw.Recipient == refused {
			return fmt.Errorf("temporarily unavailable")
		}
		return nil
	}
	iv.sendQueued(context.Background())
	recorder.BeforeSend = nil
	iv.sendQueued(context.Background())
	iv.sendQueued(context.Background())
	if len(recorder.Raws()) != 2 {
		t.Errorf("sent %d copies across refusal and retries", len(recorder.Raws()))
	}
}

func TestQueuedGuestMailStopsAfterRemoval(t *testing.T) {
	for _, kind := range []string{"invitation", "reminder", "update", "post"} {
		for _, volunteer := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/volunteer=%t", kind, volunteer), func(t *testing.T) {
				iv, _, recorder, group := queuedInvitations(t, parent)
				original := iv.s.Model().copiesOf(group)[parent]
				member := ""
				if volunteer {
					member = "yes"
				}
				ids := apply(t, iv.s, []Edit{
					{Set: group, Cells: map[string]any{"kind": "activity"}},
					{Insert: "MEMBER", Row: map[string]any{"group": group, "person": parent, "member": member, "invited": "yes"}},
				})
				if kind != "invitation" {
					iv.sendQueued(t.Context())
					apply(t, iv.s, invitationEdits(group, staff, kind, []string{parent}, time.Now()))
				}
				before := len(recorder.Raws())
				apply(t, iv.s, []Edit{{Set: ids[1], Cells: map[string]any{"invited": "excluded"}}})
				iv.sendQueued(t.Context())
				want := before
				if volunteer {
					want++
				}
				if got := len(recorder.Raws()); got != want {
					t.Fatalf("sent %d messages, want %d after removal", got, want)
				}
				if kind != "invitation" || volunteer {
					return
				}
				suppressed, _ := iv.s.Model().Table("RECIPIENT").Get(original["id"])
				if suppressed["sent"] != "" || suppressed["failed"] == "" || suppressed["detail"] != "the person was taken off the guest list before it went" {
					t.Fatalf("the unsent invitation was not suppressed: %v", suppressed)
				}
				if copy := iv.s.Model().copiesOf(group)[parent]; copy != nil {
					t.Fatalf("the suppressed invitation still counts as a copy: %v", copy)
				}
				if count := iv.s.Model().invitationsFrom(group, staff); count != 0 {
					t.Fatalf("the suppressed invitation counts %d toward the quota", count)
				}
				if _, err := iv.addGuests(t.Context(), iv.s.Model(), Env{Viewer: staff, Now: testNow}, access.Actor{Email: maya}, addGuestsAsk{Group: group, People: []guestAsk{{Person: parent}}, Send: true}); err != nil {
					t.Fatalf("restore and invite the unsent guest: %v", err)
				}
				iv.sendQueued(t.Context())
				iv.sendQueued(t.Context())
				if got := len(recorder.Raws()); got != 1 || iv.s.Model().copiesOf(group)[parent]["sent"] == "" {
					t.Fatalf("restoration delivered %d invitations, want one", got)
				}
				if count := iv.s.Model().invitationsFrom(group, staff); count != 1 {
					t.Fatalf("the restored invitation counts %d toward the quota, want one", count)
				}
			})
		}
	}
}

func TestDeliveredInvitationStillCountsAfterDeliveryFailure(t *testing.T) {
	iv, _, _, group := queuedInvitations(t, parent)
	iv.sendQueued(t.Context())
	original := iv.s.Model().copiesOf(group)[parent]
	apply(t, iv.s, []Edit{{Set: original["id"], Cells: map[string]any{"failed": stamp(time.Now()), "detail": "permanent delivery failure"}}})
	if copy := iv.s.Model().copiesOf(group)[parent]; copy["id"] != original["id"] || iv.s.Model().invitationsFrom(group, staff) != 1 {
		t.Fatalf("a sent invitation stopped counting after a delivery failure: %v", copy)
	}
}

func TestQueuedCancellationPreservesDeliveredInvitationsAfterRemoval(t *testing.T) {
	for _, sent := range []bool{false, true} {
		t.Run(fmt.Sprint(sent), func(t *testing.T) {
			iv, _, recorder, group := queuedInvitations(t, parent)
			if sent {
				iv.sendQueued(t.Context())
			}
			apply(t, iv.s, append([]Edit{
				{Set: group, Cells: map[string]any{"kind": "activity", "status": "cancelled"}},
				{Insert: "MEMBER", Row: map[string]any{"group": group, "person": parent, "invited": "excluded"}},
			}, invitationEdits(group, staff, "update", []string{parent}, time.Now())...))
			iv.sendQueued(t.Context())
			want := 0
			if sent {
				want = 2
			}
			if got := len(recorder.Raws()); got != want {
				t.Fatalf("delivered %d messages, want %d with original sent=%t", got, want, sent)
			}
		})
	}
}

func TestGuestMailKeepsTheCalendarOrganizer(t *testing.T) {
	iv, _, recorder, group := queuedInvitations(t, parent)
	iv.sendQueued(context.Background())
	original := recorder.Messages()[0].Attachments[0].Content
	apply(t, iv.s, []Edit{{Insert: "MESSAGE", As: "update", Row: map[string]any{"direction": "out", "kind": "update", "group": group, "created": stamp(time.Now())}}, {Insert: "RECIPIENT", Row: map[string]any{"message": "@update", "person": parent, "created": stamp(time.Now())}}})
	iv.sendQueued(context.Background())
	apply(t, iv.s, []Edit{{Set: group, Cells: map[string]any{"status": "cancelled"}}, {Insert: "MESSAGE", As: "cancel", Row: map[string]any{"direction": "out", "kind": "update", "group": group, "created": stamp(time.Now())}}, {Insert: "RECIPIENT", Row: map[string]any{"message": "@cancel", "person": parent, "created": stamp(time.Now())}}})
	iv.sendQueued(context.Background())
	organizer := func(raw []byte) string {
		for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n ", ""), "\r\n") {
			if strings.HasPrefix(line, "ORGANIZER") {
				return line
			}
		}
		return ""
	}
	for _, msg := range recorder.Messages()[1:] {
		if got := organizer(msg.Attachments[0].Content); got != organizer(original) {
			t.Errorf("%s organizer changed from %q to %q", msg.Subject, organizer(original), got)
		}
	}
}

func TestStudentInvitationsCopyParentsWithoutTheirOwnCopy(t *testing.T) {
	for _, includeParent := range []bool{false, true} {
		t.Run(fmt.Sprint(includeParent), func(t *testing.T) {
			people := []string{student}
			if includeParent {
				people = append(people, parent, "per00000020018")
			}
			iv, _, recorder, group := queuedInvitations(t, people...)
			apply(t, iv.s, []Edit{{Insert: "PERSON_EMAIL", Row: map[string]any{"person": student, "address": "juni@example.org", "primary": true, "source": "manual"}}})
			iv.sendQueued(context.Background())
			copiesToParent := 0
			for _, raw := range recorder.Raws() {
				for _, address := range raw.To {
					if address == rowan {
						copiesToParent++
					}
				}
			}
			if copiesToParent != 1 {
				t.Errorf("parent received %d copies; own copy included = %t", copiesToParent, includeParent)
			}
			g, _ := iv.s.Model().Table("GROUP").Get(group)
			if got := iv.s.Model().invitedWithCalendar(g, student); got != includeParent {
				t.Errorf("original calendar attachment = %t; parents have their own copy = %t", got, includeParent)
			}
			iv.calendarInvite(context.Background(), iv.s.Model(), g, student)
			messages := recorder.Messages()
			last := messages[len(messages)-1]
			if !strings.Contains(string(last.Attachments[0].Content), ":mailto:when+") {
				t.Error("calendar invite on yes lost the original organizer")
			}
		})
	}
}
