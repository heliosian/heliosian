package access

import (
	"fmt"
	"net/http"
)

type Allowance struct {
	Name string
}

func Named(name string) Allowance {
	return Allowance{Name: name}
}

func Grant(held []Allowance) map[Allowance]bool {
	out := map[Allowance]bool{}
	for _, a := range held {
		out[a] = true
	}
	return out
}

type Actor struct {
	Email      string
	Household  map[string]bool
	Allowances map[Allowance]bool
}

type Actors func(r *http.Request, held func(email string) []Allowance) Actor

func System(name string) Actor {
	return Actor{Email: name}
}

func (a Actor) May(allowance Allowance) bool {
	return a.Allowances[allowance]
}

func (a Actor) Mine(email string) bool {
	return email != "" && (email == a.Email || a.Household[email])
}

type Refusal struct {
	Status  int
	Message string
	Body    any
}

func (r *Refusal) Error() string {
	return r.Message
}

func Refuse(status int, format string, args ...any) error {
	return &Refusal{Status: status, Message: fmt.Sprintf(format, args...)}
}

func Forbidden(format string, args ...any) error {
	return Refuse(http.StatusForbidden, format, args...)
}

func Missing(format string, args ...any) error {
	return Refuse(http.StatusNotFound, format, args...)
}

func Invalid(format string, args ...any) error {
	return Refuse(http.StatusBadRequest, format, args...)
}
