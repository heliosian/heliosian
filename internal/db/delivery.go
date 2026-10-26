package db

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"sync"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/mail"
)

type Deliveries struct {
	s          *Store
	signingKey string
	mu         sync.Mutex
	pending    []Edit
	flushing   bool
}

func NewDeliveries(s *Store, signingKey string) *Deliveries {
	return &Deliveries{s: s, signingKey: signingKey}
}

func (d *Deliveries) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /hooks/events", d.events)
}

func (m *Model) sendShared(ctx context.Context, s *Store, sender *mail.Mailgun, system, group, kind string, people []string, msg mail.Message) error {
	created := stamp(time.Now())
	edits := []Edit{{Insert: "MESSAGE", As: "m", Row: map[string]any{"direction": "out", "kind": kind, "group": group, "subject": msg.Subject, "created": created}}}
	seen := map[string]bool{}
	for _, p := range people {
		if seen[p] || m.addressOf(p) == "" {
			continue
		}
		seen[p] = true
		edits = append(edits, Edit{Insert: "RECIPIENT", Row: map[string]any{"message": "@m", "person": p, "created": created}})
	}
	env := Env{System: system, Now: time.Now()}
	ids, err := Write(ctx, s, access.System(system), env, Batch{Batch: edits})
	if err != nil {
		return fmt.Errorf("record mail: %w", err)
	}
	headers := map[string]string{"Message-Id": mail.MessageID(ids[0], sender.From())}
	maps.Copy(headers, msg.Headers)
	msg.Headers = headers
	raw := []byte(mail.Compose(sender.From(), msg))
	sent := []Edit{}
	for _, c := range s.Model().Table("RECIPIENT").Referencing("message", ids[0]) {
		id := c["id"]
		if err := sender.SendCopy(ctx, sender.From(), []string{m.addressOf(c["person"])}, raw, id); err != nil {
			slog.ErrorContext(ctx, "mail: copy not sent", "recipient", id, "error", err)
			sent = append(sent, Edit{Set: id, Cells: map[string]any{"failed": stamp(time.Now()), "detail": err.Error()}})
			continue
		}
		sent = append(sent, Edit{Set: id, Cells: map[string]any{"sent": stamp(time.Now())}})
	}
	if _, err := Write(ctx, s, access.System(system), env, Batch{Batch: sent}); err != nil {
		return fmt.Errorf("record sends: %w", err)
	}
	return nil
}

func (d *Deliveries) events(w http.ResponseWriter, r *http.Request) {
	if d.signingKey == "" {
		http.Error(w, "mail is not set up", http.StatusNotFound)
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
			Event     string            `json:"event"`
			Timestamp float64           `json:"timestamp"`
			Severity  string            `json:"severity"`
			Recipient string            `json:"recipient"`
			Reason    string            `json:"reason"`
			Variables map[string]string `json:"user-variables"`
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
	if err := mail.VerifyMailgun(d.signingKey, event.Signature.Timestamp, event.Signature.Token, event.Signature.Signature, time.Now()); err != nil {
		slog.WarnContext(r.Context(), "deliveries: event call refused", "error", err)
		http.Error(w, "signature", http.StatusNotAcceptable)
		return
	}
	e := event.Data
	detail := e.Status.Description
	if detail == "" {
		detail = e.Status.Message
	}
	if detail == "" {
		detail = e.Reason
	}
	id := e.Variables[mail.CopyVariable]
	if _, ok := d.s.Model().Table("RECIPIENT").Get(id); !ok {
		w.WriteHeader(http.StatusOK)
		return
	}
	when := stamp(time.UnixMilli(int64(e.Timestamp * 1000)))
	switch {
	case e.Event == "delivered":
		d.record(Edit{Set: id, Cells: map[string]any{"delivered": when}})
	case e.Event == "failed" && e.Severity == "permanent":
		d.record(Edit{Set: id, Cells: map[string]any{"failed": when, "detail": detail}})
	default:
		slog.WarnContext(r.Context(), "deliveries: delivery trouble", "event", e.Event, "recipient", id, "to", e.Recipient, "detail", detail)
	}
	w.WriteHeader(http.StatusOK)
}

func (d *Deliveries) record(e Edit) {
	d.mu.Lock()
	d.pending = append(d.pending, e)
	start := !d.flushing
	d.flushing = true
	d.mu.Unlock()
	if start {
		go d.flush()
	}
}

func (d *Deliveries) flush() {
	for {
		d.mu.Lock()
		edits := d.pending
		d.pending = nil
		if len(edits) == 0 {
			d.flushing = false
			d.mu.Unlock()
			return
		}
		d.mu.Unlock()
		if _, err := Write(context.Background(), d.s, access.System(mailerSystem), Env{System: mailerSystem, Now: time.Now()}, Batch{Batch: edits}); err != nil {
			slog.Error("deliveries: not recorded", "edits", len(edits), "error", err)
		}
	}
}
