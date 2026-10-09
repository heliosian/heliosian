package model

import (
	"net/http"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/mail"
)

var (
	EditAnyone = access.Named("who.edit-anyone")
	Administer = access.Named("who.administer")
)

var WhoAdminAllowances = []access.Allowance{EditAnyone, Administer}

func (m *Directory) ActorOf(email string, held []access.Allowance) access.Actor {
	return access.Actor{Email: email, Household: m.Family(email), Allowances: access.Grant(held)}
}

func (m *Directory) Actor(r *http.Request, held func(email string) []access.Allowance) access.Actor {
	email := m.Resolve(mail.Normalize(auth.Email(r)))
	return m.ActorOf(email, held(email))
}
