package db

import (
	"context"
	"testing"

	"heliosian/internal/access"
)

func TestAHostOffersMaybeAndAGuestAnswersIt(t *testing.T) {
	const theo = "grp00000009001"
	s := sample(t)
	write := func(viewer string, edits ...Edit) error {
		_, err := Write(context.Background(), s, access.Actor{Email: "test@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
		return err
	}
	g, _ := s.Model().Table("GROUP").Get(theo)
	if answerColumn(g, "maybe") != "" || g["rsvp_maybe"] != "" {
		t.Fatalf("maybe is offered before the hosts turn it on")
	}
	if err := write(parent, Edit{Set: theo, Cells: map[string]any{"maybe": true}}); err == nil {
		t.Error("a guest turned on maybe")
	}
	if err := write(staff, Edit{Set: theo, Cells: map[string]any{"maybe": true}}); err != nil {
		t.Fatalf("the host can't turn on maybe: %v", err)
	}
	g, _ = s.Model().Table("GROUP").Get(theo)
	maybe, ok := s.Model().Table("GROUP").Get(g["rsvp_maybe"])
	if !ok || maybe["name"] != "Theo Turns Eight Maybe" || maybe["parent"] != theo {
		t.Fatalf("turning on maybe made %+v", maybe)
	}
	answer := func(group string) error {
		return write(parent, Edit{Insert: "MEMBER", Row: map[string]any{"group": group, "person": parent, "member": "yes", "added_by": parent}})
	}
	if err := answer(g["rsvp_maybe"]); err != nil {
		t.Fatalf("a guest can't answer maybe: %v", err)
	}
	if got := s.Model().answerOf(g, parent); got != "maybe" {
		t.Errorf("the guest's answer is %q, want maybe", got)
	}
	if err := answer(g["rsvp_yes"]); err != nil {
		t.Fatalf("a guest can't change maybe to yes: %v", err)
	}
	m := s.Model()
	if _, still := m.Table("MEMBER").Find(g["rsvp_maybe"], parent); still || m.answerOf(g, parent) != "yes" {
		t.Errorf("answering yes left the maybe; answer is %q", m.answerOf(g, parent))
	}
	if err := write(staff, Edit{Set: theo, Cells: map[string]any{"maybe": false}}); err != nil {
		t.Fatalf("the host can't turn maybe off: %v", err)
	}
	g, _ = s.Model().Table("GROUP").Get(theo)
	if answerColumn(g, "maybe") != "" || g["rsvp_maybe"] == "" {
		t.Errorf("maybe turned off is still offered, or lost its group")
	}
}
