package calendar

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/mail"
)

// A bounce is the mail provider's word that an address could not be
// reached. The provider calls /hooks/events for each delivery event on the
// calendar's sending domain; a permanent failure is kept on the Bounces
// tab, and from then on the address wears a warning on every guest list
// it is on, until a host changes it. A host may still send to it.

// Bounce is the latest word on one address.
type Bounce struct {
	When   string `json:"when"`
	Reason string `json:"reason,omitempty"`
}

const maxEventBody = 1 << 20

func (b *builder) bounces(rows []map[string]string) {
	for _, row := range rows {
		email := normalizeEmail(row["Email"])
		if email == "" {
			continue
		}
		b.model.Bounced[email] = Bounce{When: strings.TrimSpace(row["When"]), Reason: strings.TrimSpace(row["Reason"])}
	}
}

// deliveryEvents is POST /hooks/events: the provider's delivery events, as Mailgun
// sends them, signed. A permanent failure is a bounce; anything else is
// read and let go.
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
	row := map[string]string{"Email": email, "When": now().Format(DateTimeFormat), "Reason": reason}
	tables := a.cache.Tables().WithBounce(row)
	built, err := BuildModel(tables, a.cache.roster())
	if err != nil {
		slog.ErrorContext(r.Context(), "calendar: note bounce", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.cache.set(tables, built)
	a.queue.Add(func() {
		if err := a.writer.AppendCells(appName, BouncesTab, row); err != nil {
			slog.ErrorContext(r.Context(), "calendar write", "error", err)
		}
	})
	slog.InfoContext(r.Context(), "calendar: bounce noted", "email", email, "reason", reason)
	w.WriteHeader(http.StatusOK)
}

// changeInviteEmail is POST /api/calendar/invites/email: a host giving
// someone on the list a different address - one the directory does not
// hold, since a directory person's address is theirs there. The row
// keeps everything else, their answer and their outside page's token
// with it.
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
	if !a.commit(r.Context(), w, a.cache.Tables().WithInviteEmail(e.ID, email, to), func() error {
		if err := a.writer.Set(appName, InvitesTab, map[string]string{"Event ID": e.ID, "Email": inv.Email}, map[string]string{"Email": to}); err != nil {
			return err
		}
		if _, answered := model.Answered[email][e.ID]; answered {
			return a.writer.Set(appName, RSVPsTab, map[string]string{"Event ID": e.ID, "Email": email}, map[string]string{"Email": to})
		}
		return nil
	}) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: invite address changed", "actor", actor, "event", e.ID, "from", email, "to", to)
	w.WriteHeader(http.StatusNoContent)
}
