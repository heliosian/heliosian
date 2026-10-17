package access

import (
	"fmt"
	"net/http"
)

type Actor struct {
	Email string
}

func System(name string) Actor {
	return Actor{Email: name}
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
