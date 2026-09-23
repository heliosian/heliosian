package calendar

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/http"
	netmail "net/mail"
	"strings"

	"heliosian/internal/mail"
)

type Mail struct {
	Sender     mail.Sender
	From       string
	ReplyTo    string
	SigningKey string
	Key        []byte
}

// Reply is what an iCalendar REPLY says: which event, who, and their
// standing on it.
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
		slog.WarnContext(r.Context(), "calendar: reply not taken", "from", from, "uid", reply.UID, "attendee", reply.Email, "standing", reply.Standing, "error", err)
	}
	w.WriteHeader(http.StatusOK)
}

func addressed(replyTo string, recipients []string) (string, bool) {
	want := normalizeEmail(mailAddress(replyTo))
	for _, r := range recipients {
		local, domain, _ := strings.Cut(normalizeEmail(mailAddress(r)), "@")
		base, tag, _ := strings.Cut(local, "+")
		if base+"@"+domain == want {
			return tag, true
		}
	}
	return "", false
}

func (a app) replyToken(id, email string) string {
	mac := hmac.New(sha256.New, a.mail.Key)
	mac.Write([]byte(id + "|" + normalizeEmail(email)))
	return hex.EncodeToString(mac.Sum(nil)[:16])
}

// takeReply records a reply as the attendee's answer: accepted is a yes,
// declined a no, tentative a maybe, and anything else leaves their word
// as it stands. The attendee must be someone the directory knows - or on
// the event's guest list, for a guest from outside - the message must come
// from them - a calendar app replies from its owner's address, which the
// caller has authenticated - to the reply address their own invite for this
// event named, and the event must be on the calendar.
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
	email := normalizeEmail(reply.Email)
	id := idOfUID(reply.UID)
	if _, known := a.directory.Person(email); !known && a.cache.Model().InviteOf(id, email) == nil {
		return fmt.Errorf("attendee is not in the directory")
	}
	if sender := normalizeEmail(mailAddress(from)); sender != email {
		return fmt.Errorf("sent by %s, not the attendee", sender)
	}
	if !hmac.Equal([]byte(tag), []byte(a.replyToken(id, email))) {
		return fmt.Errorf("sent to an address that is not this attendee's for this event")
	}
	if err := a.recordBy(ctx, email, email, id, answer, ViaCalendar, false); err != nil {
		return err
	}
	slog.InfoContext(ctx, "calendar: answered by reply", "actor", email, "event", id, "answer", answer)
	return nil
}

// idOfUID is the event id an invite's UID was made from (uidOf).
func idOfUID(uid string) string {
	return strings.TrimSuffix(strings.TrimSpace(uid), "@calendar.heliosian.com")
}

// ParseReply finds the iCalendar REPLY in a raw email - a text/calendar
// part, or a .ics attachment - and reads the event's UID and the attendee
// with their PARTSTAT out of it. It is an error when there is none.
func ParseReply(raw []byte) (Reply, error) {
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return Reply{}, fmt.Errorf("read message: %w", err)
	}
	ics := findCalendar(msg.Header.Get("Content-Type"), msg.Header.Get("Content-Transfer-Encoding"), msg.Header.Get("Content-Disposition"), msg.Body)
	if ics == nil {
		return Reply{}, fmt.Errorf("no calendar part")
	}
	return readReply(ics)
}

// findCalendar walks a message's parts for the calendar: a text/calendar
// part, or an application/ics one, or a part whose file is a .ics, with
// its transfer encoding undone.
func findCalendar(contentType, encoding, disposition string, body io.Reader) []byte {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = "text/plain"
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		reader := multipart.NewReader(body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err != nil {
				return nil
			}
			if found := findCalendar(part.Header.Get("Content-Type"), part.Header.Get("Content-Transfer-Encoding"), part.Header.Get("Content-Disposition"), part); found != nil {
				return found
			}
		}
	}
	_, dispositionParams, _ := mime.ParseMediaType(disposition)
	name := strings.ToLower(params["name"] + dispositionParams["filename"])
	if mediaType != "text/calendar" && mediaType != "application/ics" && !strings.HasSuffix(name, ".ics") {
		return nil
	}
	var content io.Reader = body
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		content = base64.NewDecoder(base64.StdEncoding, body)
	case "quoted-printable":
		content = quotedprintable.NewReader(body)
	}
	out, err := io.ReadAll(io.LimitReader(content, 1<<20))
	if err != nil || len(out) == 0 {
		return nil
	}
	return out
}

// readReply reads the METHOD, UID and ATTENDEE lines out of a calendar,
// its folded lines joined first: a reply names one attendee, with their
// standing in its PARTSTAT parameter and their address after the colon.
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
