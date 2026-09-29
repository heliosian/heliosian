package when

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	netmail "net/mail"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/mail"
)

type Mail struct {
	Sender     *mail.Mailgun
	ReplyTo    string
	Base       string
	SigningKey string
	Key        []byte
}

type Reply struct {
	UID, Email, Standing string
}

func (a app) replies(w http.ResponseWriter, r *http.Request) {
	if a.mail.SigningKey == "" || len(a.mail.Key) == 0 {
		http.Error(w, "replies are not set up", http.StatusNotFound)
		return
	}
	fields, err := mail.Notification(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := mail.VerifyNotification(a.mail.SigningKey, fields, now()); err != nil {
		slog.WarnContext(r.Context(), "calendar: reply notification refused", "error", err)
		http.Error(w, "signature", http.StatusNotAcceptable)
		return
	}
	tag, ok := addressed(a.mail.ReplyTo, strings.Split(fields["recipient"], ","))
	if !ok {
		w.WriteHeader(http.StatusOK)
		return
	}
	raw := []byte(fields["body-mime"])
	if len(raw) == 0 {
		slog.WarnContext(r.Context(), "calendar: reply notification carries no body-mime", "from", fields["from"], "subject", fields["subject"])
		w.WriteHeader(http.StatusOK)
		return
	}
	reply, err := ParseReply(raw)
	if err != nil {
		slog.InfoContext(r.Context(), "calendar: mail received is not a reply", "from", fields["from"], "subject", fields["subject"], "error", err)
		w.WriteHeader(http.StatusOK)
		return
	}
	lines, _ := mail.SplitMessage(raw)
	from := mail.AddressOf(mail.Header(lines, "from"))
	if reason := mail.Authenticated(lines); reason != "" {
		slog.WarnContext(r.Context(), "calendar: reply not authenticated", "from", from, "uid", reply.UID, "attendee", reply.Email, "reason", reason)
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := a.takeReply(r.Context(), reply, from, tag); err != nil {
		if errors.Is(err, errNotRecorded) {
			slog.ErrorContext(r.Context(), "calendar: reply not recorded", "from", from, "uid", reply.UID, "attendee", reply.Email, "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		slog.WarnContext(r.Context(), "calendar: reply not taken", "from", from, "uid", reply.UID, "attendee", reply.Email, "standing", reply.Standing, "error", err)
	}
	w.WriteHeader(http.StatusOK)
}

func addressed(replyTo string, recipients []string) (string, bool) {
	want := mail.Normalize(mail.AddressOf(replyTo))
	for _, r := range recipients {
		local, domain, _ := strings.Cut(mail.Normalize(mail.AddressOf(r)), "@")
		base, tag, _ := strings.Cut(local, "+")
		if base+"@"+domain == want {
			return tag, true
		}
	}
	return "", false
}

func (a app) replyToken(id, email string) string {
	mac := hmac.New(sha256.New, a.mail.Key)
	mac.Write([]byte(id + "|" + mail.Normalize(email)))
	return hex.EncodeToString(mac.Sum(nil)[:16])
}

func (a app) takeReply(ctx context.Context, reply Reply, from, tag string) error {
	answer := ""
	switch reply.Standing {
	case "ACCEPTED":
		answer = AnswerYes
	case "DECLINED":
		answer = AnswerNo
	case "TENTATIVE":
		answer = AnswerMaybe
	default:
		return fmt.Errorf("standing %q changes nothing", reply.Standing)
	}
	email := mail.Normalize(reply.Email)
	uid := idOfUID(reply.UID)
	id := a.canonical(uid)
	if a.directory().Person(email) == nil && a.model().InviteOf(id, email) == nil {
		return fmt.Errorf("attendee is not in the directory")
	}
	if sender := mail.Normalize(mail.AddressOf(from)); sender != email {
		return fmt.Errorf("sent by %s, not the attendee", sender)
	}
	if !hmac.Equal([]byte(tag), []byte(a.replyToken(uid, email))) {
		return fmt.Errorf("sent to an address that is not this attendee's for this event")
	}
	if err := a.recordBy(ctx, access.System(email), email, id, answer, ViaCalendar, false, true); err != nil {
		return err
	}
	slog.InfoContext(ctx, "calendar: answered by reply", "actor", email, "event", id, "answer", answer)
	return nil
}

func idOfUID(uid string) string {
	return strings.TrimSuffix(strings.TrimSpace(uid), "@when.heliosian.com")
}

func ParseReply(raw []byte) (Reply, error) {
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return Reply{}, fmt.Errorf("read message: %w", err)
	}
	var ics []byte
	err = mail.Parts(msg, func(p mail.Part) error {
		if ics != nil || (p.MediaType != "text/calendar" && p.MediaType != "application/ics" && !strings.HasSuffix(strings.ToLower(p.Name), ".ics")) {
			return nil
		}
		out, err := io.ReadAll(io.LimitReader(p.Body, 1<<20))
		if err != nil {
			return err
		}
		if len(out) > 0 {
			ics = out
		}
		return nil
	})
	if err != nil {
		return Reply{}, err
	}
	if ics == nil {
		return Reply{}, fmt.Errorf("no calendar part")
	}
	return readReply(ics)
}

func readReply(ics []byte) (Reply, error) {
	text := strings.ReplaceAll(string(ics), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\n ", "")
	text = strings.ReplaceAll(text, "\n\t", "")
	var reply Reply
	method := ""
	for _, line := range strings.Split(text, "\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		field, params, _ := strings.Cut(name, ";")
		switch strings.ToUpper(field) {
		case "METHOD":
			method = strings.ToUpper(strings.TrimSpace(value))
		case "UID":
			if reply.UID == "" {
				reply.UID = strings.TrimSpace(value)
			}
		case "ATTENDEE":
			if reply.Email != "" {
				continue
			}
			reply.Email = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "mailto:")
			for _, p := range strings.Split(params, ";") {
				if k, v, ok := strings.Cut(p, "="); ok && strings.EqualFold(k, "PARTSTAT") {
					reply.Standing = strings.ToUpper(strings.Trim(v, `"`))
				}
			}
		}
	}
	if method != "REPLY" {
		return Reply{}, fmt.Errorf("calendar method is %q, not REPLY", method)
	}
	if reply.UID == "" || reply.Email == "" {
		return Reply{}, fmt.Errorf("reply names no event or attendee")
	}
	return reply, nil
}
