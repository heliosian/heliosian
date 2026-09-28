package who

import (
	"net/http"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/config"
)

var (
	EditAnyone = access.Acting("who.edit-anyone")
	Administer = access.Standing("who.administer")
)

var AdminAllowances = []access.Allowance{EditAnyone, Administer, config.Configure}

func (m *Model) ActorOf(email string, held []access.Allowance, hat bool) access.Actor {
	return access.Actor{Email: email, Household: m.Family(email), Allowances: access.Grant(held, hat)}
}

func (m *Model) Actor(r *http.Request, held func(email string) []access.Allowance) access.Actor {
	email := m.Resolve(strings.ToLower(auth.Email(r)))
	return m.ActorOf(email, held(email), auth.Hat(r))
}
