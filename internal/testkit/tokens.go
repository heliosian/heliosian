package testkit

import (
	"time"

	"heliosian/internal/auth"
)

type neverSignedOut struct{}

func (neverSignedOut) SignedOut(string) (time.Time, bool) { return time.Time{}, false }

func Tokens(signedIn func(email string) bool) auth.Tokens {
	return auth.Tokens{Key: []byte("test"), Sessions: neverSignedOut{}, SignedIn: signedIn, Now: time.Now}
}
