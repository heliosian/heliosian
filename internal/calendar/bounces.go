package calendar

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

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
		email := normalizeEmail(row["Email"])
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
	email := normalizeEmail(mailAddress(d.Recipient))
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
	row := store.Row{"Email": email, "When": now().Format(DateTimeFormat), "Reason": reason}
	if err := a.cache.CommitAndWait(r.Context(), deliveryActor, store.Insert(BouncesTab, row)); err != nil {
		slog.ErrorContext(r.Context(), "[ERROR] calendar: note bounce", "email", email, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slog.InfoContext(r.Context(), "calendar: bounce noted", "email", email, "reason", reason)
	w.WriteHeader(http.StatusOK)
}

func (a app) changeInviteEmail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		To    string `json:"to"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor, e, ok := a.hostedEvent(w, r, body.ID)
	if !ok {
		return
	}
	model := a.cache.Model()
	email := normalizeEmail(body.Email)
	to := normalizeEmail(body.To)
	inv := model.InviteOf(e.ID, email)
	switch {
	case inv == nil:
		http.Error(w, "that person is not on the list", http.StatusNotFound)
		return
	case isGuestKey(email):
		http.Error(w, "a guest named without an address has none to change", http.StatusBadRequest)
		return
	case !emailForm.MatchString(to):
		http.Error(w, "that is not an email address", http.StatusBadRequest)
		return
	case to == email:
		w.WriteHeader(http.StatusNoContent)
		return
	case model.InviteOf(e.ID, to) != nil:
		http.Error(w, "that address is on the list already", http.StatusBadRequest)
		return
	}
	if _, known := a.directory.Person(email); known {
		http.Error(w, "their address is the directory's to change", http.StatusBadRequest)
		return
	}
	if !a.commit(w, r, actor, store.Update(InvitesTab, store.Row{"Event ID": e.ID, "Email": email}, store.Row{"Email": to})) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: invite address changed", "actor", actor, "event", e.ID, "from", email, "to", to)
	w.WriteHeader(http.StatusNoContent)
}
