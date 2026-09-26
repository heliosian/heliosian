package calendar

import (
	"context"
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
		// Everywhere moves the address on every Celebrate party at once,
		// tickets and guest lists alike, and remembers where it went: one of
		// Celebrate's admins, on a party's list.
		Everywhere bool `json:"everywhere"`
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
	case model.InviteOf(e.ID, to) != nil && !body.Everywhere:
		http.Error(w, "that address is on the list already", http.StatusBadRequest)
		return
	}
	if _, known := a.directory.Person(email); known {
		http.Error(w, "their address is the directory's to change", http.StatusBadRequest)
		return
	}
	if body.Everywhere {
		if e.Source != SourceCelebrate || a.celebrate.MoveAddress == nil || a.celebrate.IsAdmin == nil || !a.celebrate.IsAdmin(actor) {
			http.Error(w, "only Celebrate's admins move an address on every party", http.StatusForbidden)
			return
		}
		if err := a.celebrate.MoveAddress(r.Context(), actor, email, to, inv.Name); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.commit(w, r, actor, store.Update(InvitesTab, store.Row{"Event ID": e.ID, "Email": email}, store.Row{"Email": to})) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: invite address changed", "actor", actor, "event", e.ID, "from", email, "to", to)
	w.WriteHeader(http.StatusNoContent)
}

// moveAddress follows an address Celebrate's admins moved (celebrate's
// MoveAddress) onto the guest list of every Celebrate party: the row takes
// the new address, its answer with it (carryInvite), a name for someone the
// directory does not hold, and an outside person's own link; a family or a
// guest brought under the old address follows too. Where the new address is
// on a list already, the old row simply goes. Anyone who had been sent the
// invitation at the old address - which reached nobody - is sent it again.
func (a app) moveAddress(ctx context.Context, actor, old, to, name string) {
	old, to = normalizeEmail(old), normalizeEmail(to)
	model := a.cache.Model()
	person, known := a.directory.Person(to)
	ops := []store.Op{}
	resend := []string{}
	for id := range model.Invites {
		if !strings.HasPrefix(id, SourceCelebrate+"/") {
			continue
		}
		row := model.InviteOf(id, old)
		if row == nil {
			continue
		}
		match := store.Row{"Event ID": id, "Email": old}
		if model.InviteOf(id, to) != nil {
			ops = append(ops, store.Delete(InvitesTab, match))
		} else {
			cells := store.Row{"Email": to, "Token": "", "Household": ""}
			switch {
			case known:
				cells["Name"] = person.Name
			default:
				cells["Token"], cells["Household"] = row.Token, row.Household
				if cells["Token"] == "" {
					cells["Token"] = NewToken()
				}
				if name != "" {
					cells["Name"] = name
				}
			}
			ops = append(ops, store.Update(InvitesTab, match, cells))
			if row.Sent != "" {
				if inv := model.Invitations[id]; inv != nil && inv.Sent != "" {
					resend = append(resend, id)
				}
			}
		}
		ops = append(ops,
			store.Update(InvitesTab, store.Row{"Event ID": id, "Household": old}, store.Row{"Household": to}),
			store.Update(InvitesTab, store.Row{"Event ID": id, "Guest Of": old}, store.Row{"Guest Of": to}),
		)
	}
	if len(ops) == 0 {
		return
	}
	if err := a.cache.Commit(ctx, actor, ops...); err != nil {
		slog.ErrorContext(ctx, "[ERROR] calendar: move address", "from", old, "to", to, "error", err)
		return
	}
	for _, id := range resend {
		e := a.eventFor(actor, true, id)
		if e == nil || e.end.Before(now()) {
			continue
		}
		host := actor
		if hosts := a.hostsOf(e); len(hosts) > 0 {
			host = hosts[0]
		}
		a.send(ctx, actor, host, e, []string{to}, "")
	}
	slog.InfoContext(ctx, "calendar: address moved", "actor", actor, "from", old, "to", to, "resent", len(resend))
}
