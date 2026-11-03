package db

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

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
