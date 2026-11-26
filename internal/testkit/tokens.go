package testkit

import (
	"time"

	"heliosian/internal/auth"
)

type neverSignedOut struct{}

func (neverSignedOut) SignedOut(string) (time.Time, bool) { return time.Time{}, false }

func Tokens(person func(email string) string, now func() time.Time) auth.Tokens {
	return auth.Tokens{Key: []byte("test"), Sessions: neverSignedOut{}, Person: person, Now: now}
}
