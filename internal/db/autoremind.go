package db

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

func daysSetting(g store.Row, column string) (int, bool) {
	n, err := strconv.Atoi(g[column])
	return n, err == nil && n > 0
}

func (m *Model) autoSentTo(g store.Row, kind string) map[string]bool {
	out := map[string]bool{}
	for _, msg := range m.Table("MESSAGE").Referencing("group", g["id"]) {
		if msg["direction"] != "out" || msg["kind"] != kind || msg["from_person"] != "" {
			continue
		}
		for _, r := range m.Table("RECIPIENT").Referencing("message", msg["id"]) {
			out[r["person"]] = true
		}
	}
	return out
}

func (m *Model) openedBy(g store.Row, person string) bool {
	row, ok := m.Table("MEMBER").Find(g["id"], person)
	return ok && row["opened"] != ""
}

func (m *Model) waitingOnAnswer(g store.Row) []string {
	out := []string{}
	copies := m.copiesOf(g["id"])
	for _, p := range slices.Sorted(maps.Keys(copies)) {
		r := copies[p]
		if r["sent"] == "" || r["failed"] != "" || m.keptOff(g, p) || m.answerOf(g, p) != "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (m *Model) sentDaysAgo(g store.Row, person string, days int, now time.Time) bool {
	sent, err := time.ParseInLocation(cells.StampFormat, m.copiesOf(g["id"])[person]["sent"], School)
	return err == nil && !now.Before(sent.AddDate(0, 0, days))
}

func (m *Model) remindersDue(g store.Row, now time.Time) []string {
	before, beforeSet := daysSetting(g, "remind_days")
	after, afterSet := daysSetting(g, "remind_after_days")
	if !beforeSet && !afterSet {
		return nil
	}
	if beforeSet {
		from, _, _, spans := mail.Span(g["start"], g["end"], School)
		if !spans || now.Before(from.AddDate(0, 0, -before)) {
			return nil
		}
	}
	reminded := m.autoSentTo(g, "reminder")
	out := []string{}
	for _, p := range m.waitingOnAnswer(g) {
		if reminded[p] || (afterSet && !m.sentDaysAgo(g, p, after, now)) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (m *Model) resendsDue(g store.Row, now time.Time) []string {
	days, ok := daysSetting(g, "resend_days")
	if !ok {
		return nil
	}
	resent := m.autoSentTo(g, "resend")
	out := []string{}
	for _, p := range m.waitingOnAnswer(g) {
		if resent[p] || m.openedBy(g, p) || !m.sentDaysAgo(g, p, days, now) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (iv *Invitations) autoRemind(ctx context.Context) {
	m := iv.s.Model()
	now := time.Now()
	for _, g := range m.Table("GROUP").All() {
		if !slices.Contains(guestListKinds, g["kind"]) || g["status"] != "open" || m.ended(g, now) {
			continue
		}
		if people := m.remindersDue(g, now); len(people) > 0 {
			iv.queueAutoReminder(ctx, m, g, people, now)
		}
		if people := m.resendsDue(g, now); len(people) > 0 {
			if _, err := iv.write(ctx, access.System(invitationsJob), invitationEdits(g["id"], "", "resend", people, now)); err != nil {
				slog.ErrorContext(ctx, "invitations: resend not queued", "group", g["id"], "error", err)
				continue
			}
			slog.InfoContext(ctx, "invitations: resent to those who have not opened", "group", g["id"], "people", len(people))
		}
	}
}

func (iv *Invitations) queueAutoReminder(ctx context.Context, m *Model, g store.Row, people []string, now time.Time) {
	day, hours := dayAndHours(g)
	when := day
	if hours != "" {
		when += ", " + hours
	}
	text := fmt.Sprintf("We haven't heard from you yet about %s on %s. Open the invitation to let the hosts know whether you can come.", g["name"], when)
	ref, stored, err := iv.storeText(ctx, m, text)
	if err != nil {
		slog.ErrorContext(ctx, "invitations: reminder words not stored", "group", g["id"], "error", err)
		return
	}
	created := stamp(now)
	edits := append(stored, Edit{Insert: "MESSAGE", As: "mail", Row: map[string]any{
		"direction": "out", "kind": "reminder", "group": g["id"], "subject": "Reminder: please RSVP", "content": ref, "created": created,
	}})
	for _, p := range people {
		edits = append(edits, Edit{Insert: "RECIPIENT", Row: map[string]any{"message": "@mail", "person": p, "created": created}})
	}
	if _, err := iv.write(ctx, access.System(invitationsJob), edits); err != nil {
		slog.ErrorContext(ctx, "invitations: reminder not queued", "group", g["id"], "error", err)
		return
	}
	slog.InfoContext(ctx, "invitations: reminded those who have not answered", "group", g["id"], "people", len(people))
}
