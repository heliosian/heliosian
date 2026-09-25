package artifacts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/http"
	netmail "net/mail"
	"strings"
	"time"

	"golang.org/x/net/html/charset"

	"heliosian/internal/mail"
	"heliosian/internal/store"
)

type Bucket interface {
	Put(folder, name, mimeType string, content []byte) error
}

type Inbox struct {
	SigningKey string
	Bucket     Bucket
}

func (i Inbox) ready() bool {
	return i.SigningKey != "" && i.Bucket != nil
}

const mailActor = "ask mail"

type Holder interface {
	Hold()
	Release()
}

type Filer struct {
	Inbox
	cache    *Cache
	embedder Embedder
	holder   Holder
}

func Register(mux *http.ServeMux, cache *Cache, embedder Embedder, holder Holder, mailbox Inbox) *Filer {
	in := &Filer{Inbox: mailbox, cache: cache, embedder: embedder, holder: holder}
	mux.HandleFunc("POST /hooks/mail/mime", in.hook)
	return in
}

func (in *Filer) hook(w http.ResponseWriter, r *http.Request) {
	if !in.ready() {
		http.Error(w, "mail is not set up", http.StatusNotFound)
		return
	}
	fields, err := mail.Notification(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := mail.VerifyNotification(in.SigningKey, fields, time.Now()); err != nil {
		slog.WarnContext(r.Context(), "artifacts: inbound call refused", "error", err)
		http.Error(w, "signature", http.StatusNotAcceptable)
		return
	}
	raw := []byte(fields["body-mime"])
	if len(raw) == 0 {
		slog.WarnContext(r.Context(), "artifacts: inbound call carries no body-mime", "from", fields["from"], "subject", fields["subject"])
		w.WriteHeader(http.StatusOK)
		return
	}
	m, err := ParseMail(raw)
	if err != nil {
		slog.WarnContext(r.Context(), "artifacts: mail not readable", "from", fields["from"], "subject", fields["subject"], "error", err)
		w.WriteHeader(http.StatusOK)
		return
	}
	logHeaders(raw, m)
	in.holder.Hold()
	defer in.holder.Release()
	if err := in.file(r.Context(), mailActor, m); err != nil {
		slog.ErrorContext(r.Context(), "[ERROR] artifacts: mail not imported", "id", m.MessageID, "subject", m.Subject, "error", err)
		http.Error(w, "not imported", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// Temporary: gathers what real forwarded mail carries, to settle the fix for
// security-audit/findings/ask-mail-hook-trusts-sender-headers.md. Remove with it.
func logHeaders(raw []byte, m Message) {
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return
	}
	h := msg.Header
	channel, kind, ok := m.Broadcast()
	slog.Info("artifacts: mail headers",
		"id", m.MessageID, "subject", m.Subject, "from", m.From, "to", m.To, "cc", m.CC,
		"channel", channel, "kind", kind, "broadcast", ok,
		"list-id", h["List-Id"], "sender", h["Sender"], "return-path", h["Return-Path"],
		"x-forwarded-for", h["X-Forwarded-For"], "x-forwarded-to", h["X-Forwarded-To"],
		"authentication-results", h["Authentication-Results"],
		"arc-authentication-results", h["Arc-Authentication-Results"],
		"arc-seal", h["Arc-Seal"])
}

func (in *Filer) Post(ctx context.Context, actor, group string, raw []byte) error {
	in.holder.Hold()
	defer in.holder.Release()
	m, err := ParseMail(raw)
	if err != nil {
		return err
	}
	m.Channel, m.Kind = group, KindGroup
	return in.file(ctx, actor, m)
}

func (in *Filer) Remove(ctx context.Context, actor, group string) error {
	if err := in.cache.Commit(ctx, actor, store.Delete(documentsTab, store.Row{"Kind": KindGroup, "Channel": group})); err != nil {
		return err
	}
	slog.Info("artifacts: a group's mail removed", "group", group)
	return nil
}

func (in *Filer) FileSaved(ctx context.Context, actor, path string) error {
	saved, err := ReadSaved(path)
	if err != nil {
		return err
	}
	doc, err := saved.Build(NewResolver(), in.embedder.Model())
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return in.record(ctx, actor, doc)
}

func (in *Filer) file(ctx context.Context, actor string, m Message) error {
	doc, err := Build(m, NewResolver(), in.embedder.Model())
	if errors.Is(err, ErrNotBroadcast) || errors.Is(err, ErrNoWords) {
		slog.Info("artifacts: mail left out", "from", m.From, "subject", m.Subject, "reason", err)
		return nil
	}
	if err != nil {
		return err
	}
	return in.record(ctx, actor, doc)
}

func (in *Filer) record(ctx context.Context, actor string, doc *Document) error {
	if in.known(doc) {
		slog.Info("artifacts: already on file", "key", doc.Key, "subject", doc.Title, "date", doc.Date)
		return nil
	}
	if err := doc.Embed(ctx, in.embedder); err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	body, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	if err := in.Bucket.Put(Folder, doc.ObjectFile(), "application/json", body); err != nil {
		return err
	}
	if err := in.cache.Hold(doc); err != nil {
		return err
	}
	if err := in.cache.CommitAndWait(ctx, actor, store.Insert(documentsTab, doc.Row())); err != nil {
		return fmt.Errorf("record: %w", err)
	}
	slog.Info("artifacts: filed", "key", doc.Key, "subject", doc.Title, "date", doc.Date, "channel", doc.Channel, "chunks", len(doc.Chunks))
	return nil
}

func issue(doc *Document) string {
	if doc.Kind != KindNewsletter {
		return ""
	}
	return doc.Title + "|" + doc.Date
}

func (in *Filer) known(doc *Document) bool {
	for _, d := range in.cache.Model().Documents {
		if d.Key == doc.Key || (issue(doc) != "" && issue(d) == issue(doc)) {
			return true
		}
	}
	return false
}

var headerDecoder = &mime.WordDecoder{CharsetReader: charset.NewReaderLabel}

func ParseMail(raw []byte) (Message, error) {
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return Message{}, fmt.Errorf("read message: %w", err)
	}
	h := msg.Header
	id := strings.Trim(strings.TrimSpace(h.Get("Message-Id")), "<>")
	if id == "" {
		return Message{}, fmt.Errorf("the message has no id")
	}
	received, err := receivedAt(h)
	if err != nil {
		return Message{}, fmt.Errorf("%s: %w", id, err)
	}
	m := Message{MessageID: id, Date: received.Format(time.RFC3339), ListID: h.Get("List-Id")}
	if m.Subject, err = headerDecoder.DecodeHeader(h.Get("Subject")); err != nil {
		return Message{}, fmt.Errorf("%s: Subject: %w", id, err)
	}
	if m.From, err = headerDecoder.DecodeHeader(h.Get("From")); err != nil {
		return Message{}, fmt.Errorf("%s: From: %w", id, err)
	}
	if m.To, err = addresses(h.Get("To")); err != nil {
		return Message{}, fmt.Errorf("%s: To: %w", id, err)
	}
	if m.CC, err = addresses(h.Get("Cc")); err != nil {
		return Message{}, fmt.Errorf("%s: Cc: %w", id, err)
	}
	if err := m.read(h.Get("Content-Type"), h.Get("Content-Transfer-Encoding"), h.Get("Content-Disposition"), msg.Body); err != nil {
		return Message{}, fmt.Errorf("%s: %w", id, err)
	}
	return m, nil
}

func receivedAt(h netmail.Header) (time.Time, error) {
	hops := h["Received"]
	if len(hops) == 0 {
		return time.Time{}, fmt.Errorf("the message has no Received header")
	}
	i := strings.LastIndex(hops[0], ";")
	if i < 0 {
		return time.Time{}, fmt.Errorf("the last Received header has no date: %q", hops[0])
	}
	return netmail.ParseDate(strings.TrimSpace(hops[0][i+1:]))
}

func addresses(header string) ([]string, error) {
	if strings.TrimSpace(header) == "" {
		return nil, nil
	}
	list, err := (&netmail.AddressParser{WordDecoder: headerDecoder}).ParseList(header)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, address := range list {
		out = append(out, address.Address)
	}
	return out, nil
}

func (m *Message) read(contentType, encoding, disposition string, body io.Reader) error {
	if contentType == "" {
		contentType = "text/plain"
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("content type %q: %w", contentType, err)
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		reader := multipart.NewReader(body, params["boundary"])
		for {
			part, err := reader.NextRawPart()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			if err := m.read(part.Header.Get("Content-Type"), part.Header.Get("Content-Transfer-Encoding"), part.Header.Get("Content-Disposition"), part); err != nil {
				return err
			}
		}
	}
	if kind, _, _ := mime.ParseMediaType(disposition); kind == "attachment" {
		return nil
	}
	into := &m.Text
	if mediaType == "text/html" {
		into = &m.HTML
	}
	if mediaType != "text/plain" && mediaType != "text/html" {
		return nil
	}
	content := body
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		content = base64.NewDecoder(base64.StdEncoding, content)
	case "quoted-printable":
		content = quotedprintable.NewReader(content)
	}
	if label := params["charset"]; label != "" {
		if content, err = charset.NewReaderLabel(label, content); err != nil {
			return fmt.Errorf("charset %q: %w", label, err)
		}
	}
	text, err := io.ReadAll(content)
	if err != nil {
		return err
	}
	*into += string(text)
	return nil
}
