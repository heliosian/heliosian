package when

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/config"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

type Bounce struct {
	When   string `json:"when"`
	Reason string `json:"reason,omitempty"`
}

const (
	maxEventBody  = 1 << 20
	deliveryActor = "mail events"
)

func (b *builder) bounces(rows []store.Row) {
	for _, row := range rows {
		email := config.NormalizeEmail(row["Email"])
		if email == "" {
			continue
		}
		b.model.Bounced[email] = Bounce{When: strings.TrimSpace(row["When"]), Reason: strings.TrimSpace(row["Reason"])}
	}
}

func (a app) deliveryEvents(w http.ResponseWriter, r *http.Request) {
	if a.mail.SigningKey == "" {
		http.Error(w, "events are not set up", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxEventBody))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var event struct {
		Signature struct {
			Timestamp string `json:"timestamp"`
			Token     string `json:"token"`
			Signature string `json:"signature"`
		} `json:"signature"`
		Data struct {
			Event     string `json:"event"`
			Severity  string `json:"severity"`
			Recipient string `json:"recipient"`
			Reason    string `json:"reason"`
			Status    struct {
				Message     string `json:"message"`
				Description string `json:"description"`
			} `json:"delivery-status"`
		} `json:"event-data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	if err := mail.VerifyMailgun(a.mail.SigningKey, event.Signature.Timestamp, event.Signature.Token, event.Signature.Signature, time.Now()); err != nil {
		slog.WarnContext(r.Context(), "calendar: event call refused", "error", err)
		http.Error(w, "signature", http.StatusNotAcceptable)
		return
	}
	d := event.Data
	if d.Event != "failed" || d.Severity != "permanent" {
		w.WriteHeader(http.StatusOK)
		return
	}
	email := config.NormalizeEmail(mail.AddressOf(d.Recipient))
	if email == "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	reason := strings.TrimSpace(d.Status.Description)
	if reason == "" {
		reason = strings.TrimSpace(d.Status.Message)
	}
	if reason == "" {
		reason = d.Reason
	}
	actor := access.System(deliveryActor)
	if err := a.cache.CommitAndWait(r.Context(), actor, bounceOps(actor, email, reason)...); err != nil {
		slog.ErrorContext(r.Context(), "calendar: note bounce", "email", email, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slog.InfoContext(r.Context(), "calendar: bounce noted", "email", email, "reason", reason)
	w.WriteHeader(http.StatusOK)
}

type addressBody struct {
	ID         string `json:"id"`
	Email      string `json:"email"`
	To         string `json:"to"`
	Everywhere bool   `json:"everywhere"`
}

func (a app) moveAddress(ctx context.Context, actor access.Actor, old, to, name string) {
	old, to = config.NormalizeEmail(old), config.NormalizeEmail(to)
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
	if err := a.cache.Commit(ctx, actor, ops...); err != nil {
		slog.ErrorContext(ctx, "calendar: move address", "from", old, "to", to, "error", err)
		return
	}
	slog.InfoContext(ctx, "calendar: address moved", "actor", actor.Email, "from", old, "to", to, "resent", resent)
}
