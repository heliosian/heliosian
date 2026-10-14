package model

import (
	"context"
	"log/slog"
	"strings"

	"heliosian/internal/access"
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
	ID         string `json:"id"`
	Email      string `json:"email"`
	To         string `json:"to"`
	Everywhere bool   `json:"everywhere"`
}

func (a calendarApp) moveAddress(ctx context.Context, actor access.Actor, old, to, name string) {
	old, to = mail.Normalize(old), mail.Normalize(to)
	ops, resend := a.moveAddressOps(actor, old, to, name)
	if len(ops) == 0 {
		return
	}
	resent := 0
	for _, id := range resend {
		e := a.anyEvent(actor.Email, id)
		if e == nil || e.end.Before(now()) {
			continue
		}
		host := actor.Email
		if hosts := a.hostsOf(e); len(hosts) > 0 {
			host = hosts[0]
		}
		ops = append(ops, resendOp(e, to, host))
		resent++
	}
	if err := a.commit(ctx, actor, ops...); err != nil {
		slog.ErrorContext(ctx, "calendar: move address", "from", old, "to", to, "error", err)
		return
	}
	slog.InfoContext(ctx, "calendar: address moved", "actor", actor.Email, "from", old, "to", to, "resent", resent)
}
