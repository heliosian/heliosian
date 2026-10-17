package db

import (
	"context"
	"testing"
	"time"
)

func TestSigningOutRefusesEarlierSessions(t *testing.T) {
	s, queue := sampleWithQueue(t)
	sessions := NewSessions(s, queue)
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
