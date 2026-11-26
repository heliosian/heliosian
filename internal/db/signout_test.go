package db

import (
	"context"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
)

func TestSigningOutRefusesEarlierSessions(t *testing.T) {
	s := sample(t)
	sessions := NewSessions(s)
	if _, ok := sessions.SignedOut("rowan.ashdown@example.org"); ok {
		t.Fatal("rowan has a sign-out before ever signing out")
	}
	before := time.Now().Truncate(time.Second)
	if err := sessions.SignOut(context.Background(), " Rowan.Ashdown@example.org "); err != nil {
		t.Fatal(err)
	}
	at, ok := sessions.SignedOut("rowan.ashdown@example.org")
	if !ok || at.Before(before) || at.After(time.Now()) {
		t.Fatalf("signed out at %v, %v; want a moment from %v on", at, ok, before)
	}
	if row, _ := s.Model().Table("PERSON").Get(parent); row["signed_out"] == "" {
		t.Error("the sign-out is not on rowan's PERSON row")
	}
	if _, ok := sessions.SignedOut("maya.lindqvist@example.org"); ok {
		t.Error("rowan's sign-out reached maya")
	}
	if err := sessions.SignOut(context.Background(), "nobody@example.org"); err != nil {
		t.Errorf("signing out an address on no person: %v", err)
	}
}

func TestASignOutInTheRepeatedHourCutsOffAtItsLaterInstant(t *testing.T) {
	s := sample(t)
	sessions := NewSessions(s)
	pst := time.Date(2026, 11, 1, 9, 15, 0, 0, time.UTC).In(School)
	_, err := Write(context.Background(), s, access.Actor{Email: "rowan.ashdown@example.org"}, Env{System: signOutActor, Now: pst}, Batch{Batch: []Edit{{Set: parent, Cells: map[string]any{"signed_out": pst.Format(cells.StampFormat)}}}})
	if err != nil {
		t.Fatal(err)
	}
	at, ok := sessions.SignedOut("rowan.ashdown@example.org")
	if !ok || !at.Equal(pst) {
		t.Fatalf("a sign-out at %v reads as %v", pst, at)
	}
}
