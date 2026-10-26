package db

import (
	"context"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/store"
)

const signOutActor = "signout"

type Sessions struct {
	s *Store
}

func NewSessions(s *Store) *Sessions {
	return &Sessions{s: s}
}

func (ss *Sessions) person(email string) (store.Row, bool) {
	m := ss.s.Model()
	return m.Table("PERSON").Get(m.PersonOf(email))
}

func (ss *Sessions) SignedOut(email string) (time.Time, bool) {
	p, ok := ss.person(email)
	if !ok || p["signed_out"] == "" {
		return time.Time{}, false
	}
	at, err := cells.When(p["signed_out"])
	if err != nil {
		return time.Time{}, false
	}
	return time.Date(at.Year(), at.Month(), at.Day(), at.Hour(), at.Minute(), at.Second(), 0, School), true
}

func (ss *Sessions) SignOut(ctx context.Context, email string) error {
	p, ok := ss.person(email)
	if !ok {
		return nil
	}
	now := time.Now().In(School)
	_, err := Write(ctx, ss.s, access.Actor{Email: strings.ToLower(strings.TrimSpace(email))}, Env{System: signOutActor, Now: now}, Batch{Batch: []Edit{{Set: p["id"], Cells: map[string]any{"signed_out": now.Format(cells.StampFormat)}}}})
	return err
}
