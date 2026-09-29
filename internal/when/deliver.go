package when

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/config"
	"heliosian/internal/store"
)

const (
	KindMessage   = "message"
	KindCancelled = "cancelled"
	mailActor     = "when mail"
)

var messageKinds = []string{KindMessage, inviteReminder, inviteUpdate, KindCancelled}

type Message struct {
	ID         string
	EventID    string
	Kind       string
	Subject    string
	Text       string
	Recipients []string
	Attach     bool
	SentBy     string
	Created    string
	SentTo     []string
}

func (m Message) pending() []string {
	out := []string{}
	for _, r := range m.Recipients {
		if !slices.Contains(m.SentTo, r) {
			out = append(out, r)
		}
	}
	return out
}

func (b *builder) messages(rows []store.Row) error {
	for _, row := range rows {
		key := strings.TrimSpace(row["Message ID"])
		if key == "" {
			continue
		}
		kind := strings.TrimSpace(row["Kind"])
		if !slices.Contains(messageKinds, kind) {
			return fmt.Errorf("message %s: kind %q is not %s", key, kind, strings.Join(messageKinds, ", "))
		}
		attach, err := cells.YesNo(row["Attach"], false)
		if err != nil {
			return fmt.Errorf("message %s: attach %w", key, err)
		}
		b.model.Messages = append(b.model.Messages, Message{
			ID: key, EventID: strings.TrimSpace(row["Event ID"]), Kind: kind, Subject: strings.TrimSpace(row["Subject"]), Text: strings.TrimSpace(row["Text"]),
			Recipients: splitEmails(row["Recipients"]), Attach: attach, SentBy: config.NormalizeEmail(row["Sent By"]), Created: strings.TrimSpace(row["Created"]),
			SentTo: splitEmails(row["Sent To"]),
		})
		b.model.eventIDs[key] = true
	}
	return nil
}

func owedBy(cell string) (string, bool) {
	return strings.CutPrefix(cell, owed+":")
}

func (a app) deliverLoop(kick <-chan struct{}) {
	for range kick {
		a.deliver(context.Background())
	}
}

func (a app) deliver(ctx context.Context) {
	a.fillGroups(ctx)
	a.mailInvitations(ctx)
	a.mailMessages(ctx)
	a.mailAnswers(ctx)
	a.mailCohosts(ctx)
	a.mailAdmins(ctx)
}

func (a app) stamp(ctx context.Context, what string, ops []store.Op) {
	if len(ops) == 0 {
		return
	}
	if err := a.cache.Commit(ctx, access.System(mailActor), ops...); err != nil {
		slog.ErrorContext(ctx, "calendar: record mail", "what", what, "error", err)
	}
}

func (a app) fillGroups(ctx context.Context) {
	actor := access.System(sweepActor)
	_, err := a.queue.Transact(ctx, actor, func(tx *store.Tx) error {
		ops := []store.Op{}
		for id, groups := range a.model().Groups {
			if len(groups) == 0 {
				continue
			}
			adder := groups[0].AddedBy
			e := a.sweptEvent(a.directory().ActorOf(adder, a.cache.Held(adder)), id)
			if e == nil || e.end.Before(now()) {
				continue
			}
			for _, g := range groups {
				filled, emails := a.fillOps(actor, e, g)
				if len(filled) > 0 {
					slog.InfoContext(ctx, "calendar: group filled", "event", e.ID, "group", g.ID, "added", len(emails))
				}
				ops = append(ops, filled...)
			}
		}
		return a.cache.Stage(tx, ops...)
	})
	if err != nil {
		slog.ErrorContext(ctx, "calendar: fill groups", "error", err)
	}
}

func (a app) mailInvitations(ctx context.Context) {
	model := a.model()
	for id, rows := range model.Invites {
		byHost := map[string][]string{}
		hosts := []string{}
		for _, inv := range rows {
			if inv.Requested == "" || inv.Sent != "" {
				continue
			}
			if _, seen := byHost[inv.RequestedBy]; !seen {
				hosts = append(hosts, inv.RequestedBy)
			}
			byHost[inv.RequestedBy] = append(byHost[inv.RequestedBy], inv.Email)
		}
		if len(hosts) == 0 {
			continue
		}
		e := a.anyEvent("", id)
		if e == nil {
			continue
		}
		ops := []store.Op{}
		for _, host := range hosts {
			done := a.sendInvitations(ctx, host, e, byHost[host], "")
			for _, email := range done {
				ops = append(ops, inviteSentOp(e.ID, email, now().Format(DateTimeFormat)))
			}
		}
		a.stamp(ctx, "invitations", ops)
	}
}

func (a app) sendInvitations(ctx context.Context, host string, e *Event, emails []string, kind string) []string {
	model := a.model()
	e = model.invitedEvent(e)
	inv := model.Invitations[e.ID]
	order, cc := a.recipients(e, emails)
	done := []string{}
	for _, email := range emails {
		if !slices.Contains(order, email) {
			done = append(done, email)
		}
	}
	hostName := a.fullName(host)
	message := ""
	if inv != nil {
		message = inv.Message
	}
	for _, to := range order {
		link := a.mail.Base + EventPath(e)
		if row := model.InviteOf(e.ID, to); row != nil && row.Token != "" {
			link = a.mail.Base + extPath(row.Token)
		}
		if err := a.sendInvitation(ctx, to, cc[to], a.namesFor(e, to), hostName, message, e, link, a.replyTo(e, to), kind); err != nil {
			slog.ErrorContext(ctx, "calendar: send invitation", "to", to, "event", e.ID, "error", err)
			continue
		}
		slog.InfoContext(ctx, "calendar: invitation sent", "to", to, "event", e.ID, "kind", kind)
		done = append(done, to)
	}
	return done
}

func (a app) namesFor(e *Event, to string) []string {
	model := a.model()
	household := a.householdOn(e, to)
	names := []string{}
	for _, row := range model.Invites[e.ID] {
		if row.Email != to && !slices.Contains(household, row.Email) {
			continue
		}
		name := row.Name
		if p := a.directory().Person(row.Email); p != nil && p.FullName != "" {
			name = p.FullName
		}
		if row.Email == to {
			names = append([]string{FirstWord(name)}, names...)
		} else {
			names = append(names, FirstWord(name))
		}
	}
	if len(names) == 0 {
		names = []string{FirstWord(cells.DisplayName(to))}
	}
	return names
}

func (a app) mailMessages(ctx context.Context) {
	model := a.model()
	for _, m := range model.Messages {
		pending := m.pending()
		if len(pending) == 0 {
			continue
		}
		e := a.anyEvent("", m.EventID)
		sent := slices.Clone(m.SentTo)
		ops := []store.Op{}
		if e == nil {
			slog.WarnContext(ctx, "calendar: message for an event that is gone", "message", m.ID, "event", m.EventID)
			sent = append(sent, pending...)
		} else {
			for _, to := range a.deliverMessage(ctx, m, e, pending) {
				sent = append(sent, to)
				if m.Kind == inviteReminder || m.Kind == inviteUpdate {
					if inv := model.InviteOf(e.ID, to); inv != nil && inv.Sent == "" {
						ops = append(ops, inviteSentOp(e.ID, to, now().Format(DateTimeFormat)))
					}
				}
			}
		}
		if len(sent) == len(m.SentTo) {
			continue
		}
		ops = append(ops, messageSentOp(m, sent))
		a.stamp(ctx, "message", ops)
	}
}

func (a app) deliverMessage(ctx context.Context, m Message, e *Event, pending []string) []string {
	if m.Kind == inviteReminder || m.Kind == inviteUpdate {
		return a.sendInvitations(ctx, m.SentBy, e, pending, m.Kind)
	}
	targets, cc := a.recipients(e, pending)
	done := []string{}
	for _, email := range pending {
		if !slices.Contains(targets, email) {
			done = append(done, email)
		}
	}
	hostName, replyTo := a.senderAndReplyTo(access.Actor{Email: m.SentBy}, e)
	for _, to := range targets {
		var err error
		switch m.Kind {
		case KindCancelled:
			err = a.sendCancellation(ctx, to, cc[to], replyTo, hostName, m.Text, a.model().invitedEvent(e))
		default:
			err = a.sendMessage(ctx, to, cc[to], replyTo, hostName, m.Subject, m.Text, e, m.Attach)
		}
		if err != nil {
			slog.ErrorContext(ctx, "calendar: send "+m.Kind, "to", to, "event", e.ID, "error", err)
			continue
		}
		slog.InfoContext(ctx, "calendar: "+m.Kind+" sent", "to", to, "event", e.ID)
		done = append(done, to)
	}
	return done
}

func (a app) mailAnswers(ctx context.Context) {
	model := a.model()
	for email, answers := range model.Answered {
		for id, ans := range answers {
			if ans.inviteMail != owed && ans.hostsTold != owed {
				continue
			}
			e := a.anyEvent(email, id)
			if e == nil {
				continue
			}
			set := store.Row{}
			if ans.inviteMail == owed {
				set["Invite Mail"] = ""
				if ans.Answer == AnswerYes {
					if err := a.sendInvite(ctx, email, e); err != nil {
						slog.ErrorContext(ctx, "calendar: send invite", "to", email, "event", e.ID, "error", err)
						delete(set, "Invite Mail")
					} else {
						slog.InfoContext(ctx, "calendar: invite sent", "to", email, "event", e.ID)
						set["Invite Mail"] = now().Format(DateTimeFormat)
					}
				}
			}
			if ans.hostsTold == owed {
				set["Hosts Told"] = now().Format(DateTimeFormat)
				if inv := model.Invitations[id]; inv != nil && ans.Answer != "" && ans.Answer != AnswerHidden {
					for _, h := range inv.Notify {
						if h == ans.By {
							continue
						}
						if err := a.sendAnswerNote(ctx, h, ans.By, email, ans.Answer, model.invitedEvent(e)); err != nil {
							slog.ErrorContext(ctx, "calendar: send answer note", "to", h, "event", e.ID, "error", err)
							delete(set, "Hosts Told")
						}
					}
				}
			}
			if len(set) > 0 {
				a.stamp(ctx, "answer", []store.Op{answerToldOp(id, email, set)})
			}
		}
	}
}

func hostToTell(entry string) (host, by string) {
	host, by, _ = strings.Cut(entry, ":")
	return host, by
}

func (a app) mailCohosts(ctx context.Context) {
	model := a.model()
	for id, inv := range model.Invitations {
		if len(inv.HostsToTell) == 0 {
			continue
		}
		e := a.anyEvent("", id)
		if e == nil {
			continue
		}
		left := []string{}
		for _, entry := range inv.HostsToTell {
			host, by := hostToTell(entry)
			if err := a.sendCohostNote(ctx, host, by, e); err != nil {
				slog.ErrorContext(ctx, "calendar: send co-host note", "to", host, "event", e.ID, "error", err)
				left = append(left, entry)
				continue
			}
			slog.InfoContext(ctx, "calendar: co-host told", "to", host, "event", e.ID)
		}
		a.stamp(ctx, "co-hosts", []store.Op{hostsToTellOp(id, left)})
	}
}

func (a app) mailAdmins(ctx context.Context) {
	model := a.model()
	for _, e := range slices.Concat(model.Events, model.Pending) {
		by, ok := owedBy(e.adminsTold)
		if e.Source != SourceSheet || !ok {
			continue
		}
		if err := a.tellAdmins(ctx, by, e); err != nil {
			slog.ErrorContext(ctx, "calendar: tell admins", "event", e.ID, "error", err)
			continue
		}
		a.stamp(ctx, "admins", []store.Op{adminsToldOp(e.ID)})
	}
}
