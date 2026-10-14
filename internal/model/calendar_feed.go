package model

import (
	"strings"

	"heliosian/internal/mail"
)

var brand = mail.Brand{Name: "Helios When", Color: "#0e4d54", Tagline: "the school calendar"}

func (e *Event) uidKey() string {
	if e.googleUID != "" {
		return e.googleUID
	}
	return e.ID
}

func uidOf(id string) string {
	if strings.Contains(id, "@") {
		return id
	}
	return id + "@when.heliosian.com"
}
