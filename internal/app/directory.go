package app

import (
	"heliosian/internal/auth"
	"heliosian/internal/db"
)

func spoofing(s *db.Store) *auth.Spoof {
	return &auth.Spoof{
		Allowed: func(email string) bool { return s.Model().SuperAdmin(email) },
		Person: func(email string) (auth.Person, bool) {
			address, name, ok := s.Model().SignedInAs(email)
			return auth.Person{Email: address, Name: name}, ok
		},
	}
}
