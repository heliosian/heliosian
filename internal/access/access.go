package access

import (
	"fmt"
	"net/http"
)

type Actor struct {
	Email     string
	Admin     bool
	Household map[string]bool
}

func System(name string) Actor {
	return Actor{Email: name}
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
