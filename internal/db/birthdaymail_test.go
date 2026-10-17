package db

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/testkit/mailtest"
)

type sharedRows struct {
	rows []map[string]string
}

func (s *sharedRows) Insert(app, table string, rows []map[string]string) error {
	if app != birthdaySheet || table != BirthdaySharedTab {
		panic("the weekly copy wrote to " + app + "/" + table)
	}
	for _, row := range rows {
		for column := range row {
			if !slices.Contains(BirthdaySharedColumns, column) {
				panic("the weekly copy wrote a column the shared tab does not have: " + column)
			}
		}
	}
	s.rows = append(s.rows, rows...)
	return nil
}

func (s *sharedRows) Upsert(string, string, map[string]string, map[string]string) error {
	panic("not the weekly copy's write")
}

func (s *sharedRows) Update(string, string, map[string]string, map[string]string) error {
	panic("not the weekly copy's write")
}

func (s *sharedRows) SetMany(string, string, string, map[string]map[string]string) error {
	panic("not the weekly copy's write")
}

func (s *sharedRows) Delete(string, string, map[string]string) error {
	panic("not the weekly copy's write")
}

func (s *sharedRows) DeleteMany(string, string, string, []string) error {
	panic("not the weekly copy's write")
}

func birthdayMailer(t *testing.T) (*Store, *BirthdayMailer, *mailtest.Recorder, *sharedRows) {
	t.Helper()
	s, queue := sampleWithQueue(t)
	sender := mailtest.NewRecorder(mailtest.From)
	shared := &sharedRows{}
	bm := NewBirthdayMailer(s, queue, BirthdayMailConfig{Sender: sender.Mailgun, Origin: func(app string) string { return "https://" + app + ".test" }, Shared: shared})
	return s, bm, sender, shared
}

func at(t *testing.T, moment string) time.Time {
	t.Helper()
	when, err := time.ParseInLocation("2006-01-02 15:04", moment, School)
	if err != nil {
		t.Fatal(err)
	}
	return when
}

func TestAnAssigneeGetsTheDayToAskOnTheirCalendar(t *testing.T) {
	s, bm, sender, _ := birthdayMailer(t)
	bm.pass(context.Background(), at(t, "2026-10-17 10:00"))
	got := sender.Wait(t, 4)
	want := []string{"Ask Hollis Grant about their birthday charity", "Ask Maya Lindqvist about their birthday charity", "Ask Nalini Rao about their birthday charity", "Ask Rafael Duarte about their birthday charity"}
	if subjects := subjects(got); !slices.Equal(subjects, want) {
		t.Fatalf("invites %v, want %v", subjects, want)
	}
	for _, m := range got {
		if len(m.Attachments) != 1 || !strings.Contains(string(m.Attachments[0].Content), "BEGIN:VEVENT") {
			t.Errorf("%q carries no calendar invite", m.Subject)
		}
	}
	if y, _ := s.Model().Table("BIRTHDAY_YEAR").Get("bdy00000000005"); y["invite_day"] != "2026-10-15" {
		t.Errorf("Hollis's year names the ask day %q", y["invite_day"])
	}
	bm.pass(context.Background(), at(t, "2026-10-17 11:00"))
	if n := len(sender.Messages()); n != 4 {
		t.Errorf("a second pass sent %d messages in all, want the 4 invites", n)
	}
}

func TestAnAssigneeIsRemindedOnTheDayEachStepFallsDue(t *testing.T) {
	_, bm, sender, _ := birthdayMailer(t)
	bm.pass(context.Background(), at(t, "2026-10-12 08:00"))
	late := ""
	for _, m := range sender.Wait(t, 5) {
		if strings.HasPrefix(m.Subject, "Re: Ask Nalini Rao") {
			late = m.Text
		}
	}
	if !strings.Contains(late, "was due to be asked") || !strings.Contains(late, "Hi Nalini") {
		t.Fatalf("Nalini's late reminder: %q", late)
	}
	bm.pass(context.Background(), at(t, "2026-10-21 08:00"))
	donation := ""
	for _, m := range sender.Wait(t, 6) {
		if strings.HasPrefix(m.Subject, "Re: Ask Hollis Grant") {
			donation = m.Text
		}
	}
	if !strings.Contains(donation, "charity is not recorded yet") {
		t.Fatalf("Hollis's donation reminder: %q", donation)
	}
	bm.pass(context.Background(), at(t, "2026-10-21 15:00"))
	if n := len(sender.Messages()); n != 6 {
		t.Errorf("a second pass that day sent %d messages in all, want 6", n)
	}
	bm.pass(context.Background(), at(t, "2026-10-21 22:00"))
	if n := len(sender.Messages()); n != 6 {
		t.Errorf("a pass at night sent %d messages in all, want 6", n)
	}
}

func TestTheWeeksIssueIsCopiedForTheNewsletterOnce(t *testing.T) {
	s, bm, _, shared := birthdayMailer(t)
	bm.pass(context.Background(), at(t, "2026-10-23 00:30"))
	if len(shared.rows) != 1 || shared.rows[0]["Staff Name"] != "Hollis Grant" || shared.rows[0]["Target Newsletter Date"] != "2026-10-23" || shared.rows[0]["Charity Name"] != "Second Harvest" {
		t.Fatalf("the copy for the 2026-10-23 issue: %v", shared.rows)
	}
	if y, _ := s.Model().Table("BIRTHDAY_YEAR").Get("bdy00000000005"); y["published"] == "" || y["charity"] != "chr00000000001" {
		t.Errorf("Hollis's year after the copy: %v", y)
	}
	bm.pass(context.Background(), at(t, "2026-10-23 01:30"))
	if len(shared.rows) != 1 {
		t.Errorf("a second pass copied again: %v", shared.rows)
	}
}
