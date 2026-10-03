package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	netmail "net/mail"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html/charset"

	"heliosian/internal/access"
	"heliosian/internal/artifacts"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

const maxMailMarkdown = 200_000

var documentsMailActor = access.System("ask mail")

type DocumentFiler struct {
	artifacts.Inbox
	store    *Store
	embedder *artifacts.Vertex
	holder   *store.Queue
	filing   sync.Mutex
}

func RegisterDocuments(mux *http.ServeMux, s *Store, embedder *artifacts.Vertex, holder *store.Queue, mailbox artifacts.Inbox) *DocumentFiler {
	in := &DocumentFiler{Inbox: mailbox, store: s, embedder: embedder, holder: holder}
	mux.HandleFunc("POST /hooks/mail/mime", in.hook)
	return in
}

func (in *DocumentFiler) hook(w http.ResponseWriter, r *http.Request) {
	if in.SigningKey == "" || in.Bucket == nil {
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
	if strings.Contains(strings.ToLower(m.From), "<forwarding-noreply@google.com>") {
		slog.InfoContext(r.Context(), "artifacts: forwarding confirmation", "from", m.From, "subject", m.Subject, "text", m.Text)
	}
	lines, _ := mail.SplitMessage(raw)
	if reason := vouch(lines, m); reason != "" {
		slog.WarnContext(r.Context(), "artifacts: mail not vouched for", "id", m.MessageID, "from", m.From, "subject", m.Subject, "reason", reason)
		w.WriteHeader(http.StatusOK)
		return
	}
	in.holder.Hold()
	defer in.holder.Release()
	if err := in.file(r.Context(), documentsMailActor, m); err != nil {
		slog.ErrorContext(r.Context(), "artifacts: mail not imported", "id", m.MessageID, "subject", m.Subject, "error", err)
		http.Error(w, "not imported", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (in *DocumentFiler) Post(ctx context.Context, actor access.Actor, group string, raw []byte) error {
	in.holder.Hold()
	defer in.holder.Release()
	m, err := ParseMail(raw)
	if err != nil {
		return err
	}
	m.Channel, m.Kind = group, DocumentKindGroup
	return in.file(ctx, actor, m)
}

func (in *DocumentFiler) Remove(ctx context.Context, actor access.Actor, group string) error {
	if err := in.store.Commit(ctx, actor, DocumentsApp, in.store.Model().Documents.removeGroup(actor, group)...); err != nil {
		return err
	}
	slog.Info("artifacts: a group's mail removed", "group", group)
	return nil
}

func (in *DocumentFiler) FileSaved(ctx context.Context, actor access.Actor, path string) error {
	saved, err := ReadSaved(path)
	if err != nil {
		return err
	}
	doc, err := saved.Build(&LinkResolver{}, in.embedder.Model())
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return in.record(ctx, actor, doc)
}

func (in *DocumentFiler) file(ctx context.Context, actor access.Actor, m DocumentMessage) error {
	doc, err := m.Build(&LinkResolver{}, in.embedder.Model())
	if errors.Is(err, ErrNotBroadcast) || errors.Is(err, ErrNoWords) {
		slog.Info("artifacts: mail left out", "from", m.From, "subject", m.Subject, "reason", err)
		return nil
	}
	if err != nil {
		return err
	}
	if len(doc.Markdown) > maxMailMarkdown {
		slog.Info("artifacts: mail left out", "from", m.From, "subject", m.Subject, "reason", fmt.Sprintf("%d characters of markdown, over %d", len(doc.Markdown), maxMailMarkdown))
		return nil
	}
	return in.record(ctx, actor, doc)
}

func (in *DocumentFiler) record(ctx context.Context, actor access.Actor, doc *Document) error {
	in.filing.Lock()
	defer in.filing.Unlock()
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
	if err := in.Bucket.Put(ctx, doc.Object(), "application/json", body); err != nil {
		return err
	}
	if err := in.store.Hold(doc); err != nil {
		return err
	}
	if err := in.store.CommitAndWait(ctx, actor, DocumentsApp, in.store.Model().Documents.Record(actor, doc)...); err != nil {
		return fmt.Errorf("record: %w", err)
	}
	slog.Info("artifacts: filed", "key", doc.Key, "subject", doc.Title, "date", doc.Date, "channel", doc.Channel, "chunks", len(doc.Chunks))
	return nil
}

func newsletterIssue(doc *Document) string {
	if doc.Kind != DocumentKindNewsletter {
		return ""
	}
	return doc.Title + "|" + doc.Date
}

func (in *DocumentFiler) known(doc *Document) bool {
	for _, d := range in.store.Model().Documents.Documents {
		if d.Key == doc.Key || (newsletterIssue(doc) != "" && newsletterIssue(d) == newsletterIssue(doc)) {
			return true
		}
	}
	return false
}

var headerDecoder = &mime.WordDecoder{CharsetReader: charset.NewReaderLabel}

func ParseMail(raw []byte) (DocumentMessage, error) {
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return DocumentMessage{}, fmt.Errorf("read message: %w", err)
	}
	h := msg.Header
	id := strings.Trim(strings.TrimSpace(h.Get("Message-Id")), "<>")
	if id == "" {
		return DocumentMessage{}, fmt.Errorf("the message has no id")
	}
	received, err := receivedAt(h)
	if err != nil {
		return DocumentMessage{}, fmt.Errorf("%s: %w", id, err)
	}
	m := DocumentMessage{MessageID: id, Date: received.Format(time.RFC3339), ListID: h.Get("List-Id")}
	if m.Subject, err = headerDecoder.DecodeHeader(h.Get("Subject")); err != nil {
		return DocumentMessage{}, fmt.Errorf("%s: Subject: %w", id, err)
	}
	if m.From, err = headerDecoder.DecodeHeader(h.Get("From")); err != nil {
		return DocumentMessage{}, fmt.Errorf("%s: From: %w", id, err)
	}
	if m.To, err = headerAddresses(h.Get("To")); err != nil {
		return DocumentMessage{}, fmt.Errorf("%s: To: %w", id, err)
	}
	if m.CC, err = headerAddresses(h.Get("Cc")); err != nil {
		return DocumentMessage{}, fmt.Errorf("%s: Cc: %w", id, err)
	}
	if err := mail.Parts(msg, m.read); err != nil {
		return DocumentMessage{}, fmt.Errorf("%s: %w", id, err)
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

func headerAddresses(header string) ([]string, error) {
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

func (m *DocumentMessage) read(p mail.Part) error {
	if p.Disposition == "attachment" {
		return nil
	}
	into := &m.Text
	switch p.MediaType {
	case "text/plain":
	case "text/html":
		into = &m.HTML
	default:
		return nil
	}
	text, err := io.ReadAll(p.Body)
	if err != nil {
		return err
	}
	*into += string(text)
	return nil
}
