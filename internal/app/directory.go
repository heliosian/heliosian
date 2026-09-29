package app

import (
	"heliosian/internal/auth"
	"heliosian/internal/mail"
	"heliosian/internal/model"
)

func spoofPerson(s *model.Store) func(email string) (auth.Person, bool) {
	return func(email string) (auth.Person, bool) {
		directory := s.Model().Directory
		p := directory.Person(directory.Resolve(mail.Normalize(email)))
		if p == nil {
			return auth.Person{}, false
		}
		return auth.Person{Email: p.Email, FullName: p.FullName, Words: p.Words()}, true
	}
}
