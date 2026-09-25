package loop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"heliosian/internal/mail"
	"heliosian/internal/store"
)

type Archive interface {
	Put(ctx context.Context, name, mimeType string, content []byte) error
	Get(ctx context.Context, name string) ([]byte, error)
}

type Documents interface {
	Post(ctx context.Context, actor, group string, raw []byte) error
	Remove(ctx context.Context, actor, group string) error
}

type Mail struct {
	Sender     mail.RawSender
	SigningKey string
	Key        []byte
	Base       string
	Archive    Archive
	Documents  Documents
}

func (m Mail) ready() bool {
	return m.SigningKey != "" && m.Sender != nil
}

type DirArchive struct {
	Dir string
}

func (d DirArchive) Put(_ context.Context, name, _ string, content []byte) error {
	path := filepath.Join(d.Dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func (d DirArchive) Get(_ context.Context, name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(d.Dir, filepath.FromSlash(name)))
}

const (
	stateReceived    = "received"
	stateSent        = "sent"
	stateDropped     = "dropped"
	stateFailed      = "failed"
	maxEventBody     = 1 << 20
	mailType         = "message/rfc822"
	unsubscribeLocal = "unsubscribe"
	bounceFrom       = "HCA-Team <team@" + Domain + ">"
)

type job struct {
	id, group, object string
}

func idOf(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

func (j job) key() string {
	return j.id + "|" + j.group
}

var deliveryBatch = 5 * time.Second

const mailerActor = "loop mailer"

type mailer struct {
	cache     *Cache
	directory Directory
	mail      Mail
	mu        sync.Mutex
	busy      map[string]bool
	done      map[string]bool
	work      chan job
	pending   []store.Row
	flushing  bool
}

func newMailer(cache *Cache, directory Directory, mailbox Mail) *mailer {
	m := &mailer{cache: cache, directory: directory, mail: mailbox, busy: map[string]bool{}, done: map[string]bool{}, work: make(chan job, 256)}
	go m.run()
	return m
}

func (m *mailer) run() {
	for j := range m.work {
		state := m.forward(context.Background(), j)
		m.mu.Lock()
		delete(m.busy, j.key())
		if state == stateSent || state == stateDropped {
			m.done[j.key()] = true
		}
		m.mu.Unlock()
	}
}

func (m *mailer) take(j job) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busy[j.key()] || m.done[j.key()] {
		return false
	}
	for _, msg := range m.cache.Model().Messages {
		if msg.ID == j.id && msg.Group == j.group && (msg.State == stateSent || msg.State == stateDropped) {
			return false
		}
	}
	m.busy[j.key()] = true
	return true
}

func (m *mailer) release(j job) {
	m.mu.Lock()
	delete(m.busy, j.key())
	m.mu.Unlock()
}

func (m *mailer) recover() {
	for _, msg := range m.cache.Model().Messages {
		if msg.State == stateReceived {
			j := job{id: msg.ID, group: msg.Group, object: msg.Object}
			if m.take(j) {
				slog.Info("groups: resuming a message", "message", j.id, "group", j.group)
				m.work <- j
			}
		}
	}
}

func (m *mailer) mark(j job, state string, cells store.Row) {
	cells["State"] = state
	if err := m.cache.Commit(context.Background(), mailerActor, store.Update(messagesTab, store.Row{"ID": j.id, "Group": j.group}, cells)); err != nil {
		slog.Error("[ERROR] groups: mail record", "message", j.id, "group", j.group, "error", err)
	}
}

func (m *mailer) recordSent(ctx context.Context, j job, raw []byte, cells map[string]string) {
	m.mark(j, stateSent, cells)
	if err := m.mail.Documents.Post(ctx, mailerActor, j.group, raw); err != nil {
		slog.Error("[ERROR] groups: not filed for ask", "message", j.id, "group", j.group, "error", err)
	}
}

func localsIn(addresses []string) []string {
	out := []string{}
	for _, address := range addresses {
		local, domain, _ := strings.Cut(strings.ToLower(mail.AddressOf(address)), "@")
		if domain == Domain && !slices.Contains(out, local) {
			out = append(out, local)
		}
	}
	return out
}

func (m *mailer) groupsIn(addresses []string) []string {
	model := m.cache.Model()
	out := []string{}
	for _, local := range localsIn(addresses) {
		g := model.Resolve(local)
		if g == nil {
			slog.Warn("groups: mail for no group", "local", local)
			continue
		}
		if !slices.Contains(out, g.Name) {
			out = append(out, g.Name)
		}
	}
	return out
}

func (m *mailer) received(ctx context.Context, raw []byte, from, subject string, addresses []string) error {
	id := idOf(raw)
	lines, _ := mail.SplitMessage(raw)
	messageID := messageID(lines)
	for _, name := range m.groupsIn(addresses) {
		j := job{id: id, group: name}
		if !m.take(j) {
			continue
		}
		j.object = fmt.Sprintf("loop/%s/%s-%s.eml", name, time.Now().UTC().Format("20060102T150405Z"), id)
		if err := m.mail.Archive.Put(ctx, j.object, mailType, raw); err != nil {
			m.release(j)
			return fmt.Errorf("archive %s for %s: %w", id, name, err)
		}
		cells := store.Row{"Received": time.Now().Format(time.RFC3339), "From": from, "Subject": subject, "State": stateReceived, "Recipients": "", "Object": j.object, "Detail": "", "Message ID": messageID}
		if err := m.cache.CommitAndWait(ctx, mailerActor, store.Set(messagesTab, store.Row{"ID": id, "Group": name}, cells)); err != nil {
			m.release(j)
			return fmt.Errorf("record %s for %s: %w", id, name, err)
		}
		slog.Info("groups: mail received", "message", id, "group", name, "from", from, "subject", subject)
		m.work <- j
	}
	return nil
}

func (m *mailer) record(row store.Row) {
	m.mu.Lock()
	m.pending = append(m.pending, row)
	start := !m.flushing
	m.flushing = true
	m.mu.Unlock()
	if start {
		time.AfterFunc(deliveryBatch, m.flush)
	}
}

func (m *mailer) flush() {
	m.mu.Lock()
	rows := m.pending
	m.pending = nil
	m.flushing = false
	m.mu.Unlock()
	ops := make([]store.Op, 0, len(rows))
	for _, row := range rows {
		ops = append(ops, store.Insert(deliveriesTab, row))
	}
	if err := m.cache.Commit(context.Background(), mailerActor, ops...); err != nil {
		slog.Error("[ERROR] groups: delivery record", "rows", len(rows), "error", err)
	}
}

func (m *mailer) repliesTo(group string, lines []mail.HeaderLine) bool {
	sent := map[string]bool{}
	for _, msg := range m.cache.Model().Messages {
		if msg.State == stateSent && msg.Group == group && msg.MessageID != "" {
			sent[messageKey(msg.MessageID)] = true
		}
	}
	for _, id := range referenced(lines) {
		if sent[id] {
			return true
		}
	}
	return false
}

func (m *mailer) sentAs(group, messageID string) bool {
	key := messageKey(messageID)
	if key == "" {
		return false
	}
	for _, msg := range m.cache.Model().Messages {
		if msg.Group == group && messageKey(msg.MessageID) == key {
			return true
		}
	}
	return false
}

func (m *mailer) delivery(event, from, messageID, detail string, when time.Time, address string) {
	names := m.groupsIn([]string{from})
	if len(names) == 0 || !m.sentAs(names[0], messageID) {
		return
	}
	email := strings.ToLower(mail.AddressOf(address))
	if event != eventDelivered {
		slog.Warn("groups: delivery trouble", "event", event, "group", names[0], "email", email, "detail", detail)
	}
	m.record(map[string]string{"Timestamp": when.Format(time.RFC3339), "Group": names[0], "Email": email, "Event": event, "Message": messageID, "Detail": detail})
}

func (m *mailer) forward(ctx context.Context, j job) string {
	log := slog.With("message", j.id, "group", j.group)
	fail := func(what string, err error) string {
		log.Error("groups: forward failed", "step", what, "error", err)
		m.mark(j, stateFailed, map[string]string{"Detail": what + ": " + err.Error()})
		return stateFailed
	}
	g := m.cache.Model().Group(j.group)
	if g == nil {
		return fail("group", fmt.Errorf("the group is gone"))
	}
	if !m.mail.ready() {
		return fail("mail", fmt.Errorf("mail is not set up"))
	}
	if j.object == "" {
		return fail("archive", fmt.Errorf("the message names no archive object"))
	}
	raw, err := m.mail.Archive.Get(ctx, j.object)
	if err != nil {
		return fail("archive", err)
	}
	lines, body := mail.SplitMessage(raw)
	if reason := held(lines); reason != "" {
		log.Info("groups: message held", "reason", reason)
		m.mark(j, stateDropped, map[string]string{"Detail": reason})
		return stateDropped
	}
	if reason := mail.Authenticated(lines); reason != "" {
		log.Info("groups: message not authenticated", "reason", reason, "from", mail.Header(lines, "from"))
		m.mark(j, stateDropped, map[string]string{"Detail": reason})
		return stateDropped
	}
	sender := m.directory.Resolve(strings.ToLower(mail.AddressOf(mail.Header(lines, "from"))))
	reply := m.repliesTo(g.Name, lines)
	if !g.PostableBy(sender, reply, SourcesOf(m.directory)) {
		audience, verb := g.Posting, "post"
		if reply {
			audience, verb = g.Replying, "reply"
		}
		log.Info("groups: post refused", "from", sender, "reply", reply, "audience", audience)
		if err := m.bounce(ctx, *g, lines, audience, verb); err != nil {
			log.Error("[ERROR] groups: bounce failed", "to", sender, "error", err)
		}
		m.mark(j, stateDropped, map[string]string{"Detail": "only the group's " + audience + " may " + verb})
		return stateDropped
	}
	id := messageID(lines)
	head, err := rewrite(lines, *g)
	if err != nil {
		return fail("rewrite", err)
	}
	members := Members(*g, SourcesOf(m.directory))
	sent, failures := 0, []string{}
	for _, rcpt := range members {
		tok := token(m.mail.Key, g.Name, rcpt)
		unsubscribe := "List-Unsubscribe: <mailto:" + unsubscribeLocal + "@" + Domain + "?subject=" + tok + ">, <" + m.mail.Base + "/open/unsubscribe/" + tok + ">"
		msg := render(head, []string{unsubscribe, "List-Unsubscribe-Post: List-Unsubscribe=One-Click"}, body)
		if err := m.mail.Sender.SendRaw(ctx, g.Address(), []string{rcpt}, msg); err != nil {
			log.Error("groups: send failed", "to", rcpt, "error", err)
			failures = append(failures, rcpt+": "+err.Error())
			continue
		}
		m.record(map[string]string{"Timestamp": time.Now().Format(time.RFC3339), "Group": g.Name, "Email": rcpt, "Event": eventSent, "Message": id})
		sent++
	}
	state := stateSent
	if sent == 0 && len(members) > 0 {
		state = stateFailed
	}
	log.Info("groups: forwarded", "members", len(members), "sent", sent, "failed", len(failures))
	cells := map[string]string{"Recipients": strconv.Itoa(sent), "Detail": strings.Join(failures, "; ")}
	if state == stateSent {
		m.recordSent(ctx, j, raw, cells)
		return state
	}
	m.mark(j, state, cells)
	return state
}

var posters = map[string]string{PostingMembers: "the people on it and its managers", PostingManagers: "its managers"}

func (m *mailer) bounce(ctx context.Context, g Group, lines []mail.HeaderLine, audience, verb string) error {
	to := mail.AddressOf(mail.Header(lines, "from"))
	subject := decodeHeader(mail.Header(lines, "subject"))
	text := fmt.Sprintf("Your message to %s, “%s”, was not sent to the group: only %s can %s to it.", g.Address(), subject, posters[audience], verb)
	headers := map[string]string{"Auto-Submitted": "auto-replied", loopHeader: g.Name}
	if id := mail.Header(lines, "message-id"); id != "" {
		headers["In-Reply-To"] = id
		headers["References"] = id
	}
	raw := mail.Compose(bounceFrom, mail.Message{To: []string{to}, Subject: "Not delivered: " + subject, Text: text, HTML: "<p>" + html.EscapeString(text) + "</p>", Headers: headers})
	return m.mail.Sender.SendRaw(ctx, bounceFrom, []string{to}, []byte(raw))
}

func (a app) inbound(w http.ResponseWriter, r *http.Request) {
	if !a.mail.ready() {
		http.Error(w, "mail is not set up", http.StatusNotFound)
		return
	}
	fields, err := mail.Notification(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := mail.VerifyNotification(a.mail.SigningKey, fields, time.Now()); err != nil {
		slog.WarnContext(r.Context(), "groups: inbound call refused", "error", err)
		http.Error(w, "signature", http.StatusNotAcceptable)
		return
	}
	posted := []string{}
	for _, recipient := range strings.Split(fields["recipient"], ",") {
		local, domain, _ := strings.Cut(strings.ToLower(mail.AddressOf(recipient)), "@")
		if domain == Domain && local == unsubscribeLocal {
			a.unsubscribeByMail(r.Context(), fields["subject"], fields["sender"])
			continue
		}
		posted = append(posted, recipient)
	}
	raw := []byte(fields["body-mime"])
	if len(raw) == 0 {
		slog.WarnContext(r.Context(), "groups: inbound call carries no body-mime", "recipient", fields["recipient"])
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := a.mailer.received(r.Context(), raw, fields["from"], fields["subject"], posted); err != nil {
		slog.ErrorContext(r.Context(), "[ERROR] groups: mail not recorded", "recipient", fields["recipient"], "error", err)
		http.Error(w, "not recorded", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (a app) events(w http.ResponseWriter, r *http.Request) {
	if !a.mail.ready() {
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
			Event     string  `json:"event"`
			Timestamp float64 `json:"timestamp"`
			Severity  string  `json:"severity"`
			Recipient string  `json:"recipient"`
			Reason    string  `json:"reason"`
			Message   struct {
				Headers struct {
					MessageID string `json:"message-id"`
					From      string `json:"from"`
				} `json:"headers"`
			} `json:"message"`
			Status struct {
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
		slog.WarnContext(r.Context(), "groups: event call refused", "error", err)
		http.Error(w, "signature", http.StatusNotAcceptable)
		return
	}
	d := event.Data
	kind := ""
	switch d.Event {
	case "failed":
		kind = eventDelayed
		if d.Severity == "permanent" {
			kind = eventBounced
		}
	case "complained":
		kind = eventComplaint
	case "delivered":
		kind = eventDelivered
	}
	if kind != "" {
		detail := d.Status.Description
		if detail == "" {
			detail = d.Status.Message
		}
		if detail == "" {
			detail = d.Reason
		}
		if kind == eventDelivered {
			detail = ""
		}
		when := time.UnixMilli(int64(d.Timestamp * 1000))
		a.mailer.delivery(kind, d.Message.Headers.From, d.Message.Headers.MessageID, detail, when, d.Recipient)
	}
	w.WriteHeader(http.StatusOK)
}
