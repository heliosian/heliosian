package model

import (
	"strings"

	"heliosian/internal/mail"
	"heliosian/internal/store"
)

type Bounce struct {
	When   string `json:"when"`
	Reason string `json:"reason,omitempty"`
}

func (b *builder) bounces(rows []store.Row) {
	for _, row := range rows {
		email := mail.Normalize(row["Email"])
		if email == "" {
			continue
		}
		b.model.Bounced[email] = Bounce{When: strings.TrimSpace(row["When"]), Reason: strings.TrimSpace(row["Reason"])}
	}
}

type addressBody struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	To    string `json:"to"`
}
