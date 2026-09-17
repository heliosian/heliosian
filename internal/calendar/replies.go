package calendar

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/http"
	netmail "net/mail"
	"slices"
	"strings"

	"heliosian/internal/mail"
)

// An invite names the calendar's reply address as its organizer, so the
// Accept or Decline a person taps in their own calendar app comes back
// here as a reply email with an iCalendar REPLY in it. Resend takes those
// in and calls /api/calendar/replies; the reply's attendee and standing
// become the person's answer, the same as a Yes or No on the site - with
// no invite sent back, since they are answering the one they have.

// Inbox fetches a received message back from the mail provider by its id.
type Inbox interface {
	Received(ctx context.Context, id string) (mail.Received, error)
}

// Mail is the calendar's mail: the sender its invites go out through (nil
// sends none), the address they come from, the address replies go to - the
// invites' organizer, which the inbox receives for - and the secret the
// provider signs its webhook calls with.
type Mail struct {
	Sender  mail.Sender
	From    string
	ReplyTo string
	Inbox   Inbox
	Secret  string
}

// Reply is what an iCalendar REPLY says: which event, who, and their
// standing on it.
type Reply struct {
	UID, Email, Standing string
}

// replies is the webhook the mail provider calls for each message the
// reply address receives. It answers 204 to everything it can read and
// 400 to a call it cannot trust, so the provider retries only what might
// be a real fault on this side.
func (a app) replies(w http.ResponseWriter, r *http.Request) {
	if a.mail.Secret == "" || a.mail.Inbox == nil {
		http.Error(w, "replies are not set up", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := mail.VerifyWebhook(a.mail.Secret, r.Header, body, now()); err != nil {
		slog.WarnContext(r.Context(), "calendar: reply webhook refused", "error", err)
		http.Error(w, "signature", http.StatusUnauthorized)
		return
	}
	var event struct {
		Type string `json:"type"`
		Data struct {
			EmailID     string   `json:"email_id"`
			From        string   `json:"from"`
			To          []string `json:"to"`
			CC          []string `json:"cc"`
			BCC         []string `json:"bcc"`
			ReceivedFor []string `json:"received_for"`
			Subject     string   `json:"subject"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil || event.Type != "email.received" || event.Data.EmailID == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// Every receiving domain's mail reaches every webhook: only what was
	// addressed to the reply address is fetched.
	if !addressed(a.mail.ReplyTo, slices.Concat(event.Data.To, event.Data.CC, event.Data.BCC, event.Data.ReceivedFor)) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	received, err := a.mail.Inbox.Received(r.Context(), event.Data.EmailID)
	if err != nil {
		slog.ErrorContext(r.Context(), "calendar: fetch reply", "id", event.Data.EmailID, "error", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	reply, err := ParseReply(received.Raw)
	if err != nil {
		slog.InfoContext(r.Context(), "calendar: mail received is not a reply", "id", received.ID, "from", received.From, "subject", received.Subject, "error", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := a.takeReply(r.Context(), reply, received.From); err != nil {
		slog.WarnContext(r.Context(), "calendar: reply not taken", "id", received.ID, "from", received.From, "uid", reply.UID, "attendee", reply.Email, "standing", reply.Standing, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func addressed(replyTo string, recipients []string) bool {
	want := normalizeEmail(mailAddress(replyTo))
	for _, r := range recipients {
		if normalizeEmail(mailAddress(r)) == want {
			return true
		}
	}
	return false
}

// takeReply records a reply as the attendee's answer: accepted is a yes,
// declined a no, and a tentative or anything else leaves their word as it
// stands. The attendee must be someone the directory knows, the message
// must come from them - a calendar app replies from its owner's address -
// and the event must be on the calendar.
func (a app) takeReply(ctx context.Context, reply Reply, from string) error {
	answer := ""
	switch reply.Standing {
	case "ACCEPTED":
		answer = AnswerYes
	case "DECLINED":
		answer = AnswerNo
	default:
		return fmt.Errorf("standing %q changes nothing", reply.Standing)
	}
	email := normalizeEmail(reply.Email)
	if _, known := a.directory.Person(email); !known {
		return fmt.Errorf("attendee is not in the directory")
	}
	if sender := normalizeEmail(mailAddress(from)); sender != email {
		return fmt.Errorf("sent by %s, not the attendee", sender)
	}
	id := idOfUID(reply.UID)
	if err := a.record(ctx, email, id, answer, false); err != nil {
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
