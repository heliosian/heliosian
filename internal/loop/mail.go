package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/mail"
)

type Fetcher interface {
	Stored(ctx context.Context, url string) ([]byte, error)
}

type Archive interface {
	Put(ctx context.Context, name, mimeType string, content []byte) error
}

type Documents interface {
	Post(ctx context.Context, group string, raw []byte) error
}

type Mail struct {
	Sender     mail.RawSender
	Store      Fetcher
	SigningKey string
	Key        []byte
	Base       string
	Archive    Archive
	Documents  Documents
}

func (m Mail) ready() bool {
	return m.SigningKey != "" && m.Store != nil && m.Sender != nil
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

const (
	stateReceived    = "received"
	stateStored      = "stored"
	stateSent        = "sent"
	stateDropped     = "dropped"
	stateFailed      = "failed"
	maxEventBody     = 1 << 20
	mailType         = "message/rfc822"
	unsubscribeLocal = "unsubscribe"
	bounceFrom       = "HCA-Team <team@" + Domain + ">"
)

type job struct {
	id, group, source string
}

func (j job) key() string {
	return j.id + "|" + j.group
}

var deliveryBatch = 5 * time.Second

type mailer struct {
	cache     *Cache
	writer    data.Writer
	queue     Enqueuer
	directory Directory
	mail      Mail
	mu        sync.Mutex
	busy      map[string]bool
	done      map[string]bool
	work      chan job
	pending   []map[string]string
	flushing  bool
}

func newMailer(cache *Cache, writer data.Writer, queue Enqueuer, directory Directory, mailbox Mail) *mailer {
	m := &mailer{cache: cache, writer: writer, queue: queue, directory: directory, mail: mailbox, busy: map[string]bool{}, done: map[string]bool{}, work: make(chan job, 256)}
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
	for _, row := range m.cache.Tables().Messages {
		if row["ID"] == j.id && strings.EqualFold(row["Group"], j.group) && (row["State"] == stateSent || row["State"] == stateDropped) {
			return false
		}
	}
	m.busy[j.key()] = true
	return true
}

func (m *mailer) recover() {
	for _, row := range m.cache.Tables().Messages {
		if state := row["State"]; state == stateReceived || state == stateStored {
			j := job{id: row["ID"], group: strings.ToLower(row["Group"]), source: row["Source"]}
			if m.take(j) {
				slog.Info("groups: resuming a message", "message", j.id, "group", j.group, "state", state)
				m.work <- j
			}
		}
	}
}

func (m *mailer) write(fn func() error) {
	done := make(chan struct{})
	m.queue.Add(func() {
		defer close(done)
		if err := fn(); err != nil {
			slog.Error("groups: mail record", "error", err)
		}
	})
	<-done
}

func (m *mailer) mark(j job, state string, cells map[string]string) {
	cells["State"] = state
	m.write(func() error {
		m.cache.edit(func(t *Tables) *Tables { return t.withMessage(j.id, j.group, cells) })
		return m.writer.Set(appName, messagesTab, map[string]string{"ID": j.id, "Group": j.group}, cells)
	})
}

func (m *mailer) recordSent(ctx context.Context, j job, raw []byte, cells map[string]string) {
	m.mark(j, stateSent, cells)
	if err := m.mail.Documents.Post(ctx, j.group, raw); err != nil {
		slog.Error("[ERROR] groups: not filed for ask", "message", j.id, "group", j.group, "error", err)
	}
}

func localsIn(addresses []string) []string {
	out := []string{}
	for _, address := range addresses {
		local, domain, _ := strings.Cut(strings.ToLower(addressOf(address)), "@")
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

func (m *mailer) received(id, source, from, subject string, addresses []string) {
	for _, name := range m.groupsIn(addresses) {
		j := job{id: id, group: name, source: source}
		if !m.take(j) {
			continue
		}
		slog.Info("groups: mail received", "message", id, "group", name, "from", from, "subject", subject)
		m.queue.Add(func() {
			cells := map[string]string{"Received": time.Now().Format(time.RFC3339), "From": from, "Subject": subject, "State": stateReceived, "Recipients": "", "Object": "", "Detail": "", "Source": source}
			m.cache.edit(func(t *Tables) *Tables { return t.withMessage(id, name, cells) })
			if err := m.writer.Set(appName, messagesTab, map[string]string{"ID": id, "Group": name}, cells); err != nil {
				slog.Error("groups: mail record", "error", err)
			}
		})
		m.work <- j
	}
}

func (m *mailer) record(row map[string]string) {
	m.mu.Lock()
	m.pending = append(m.pending, row)
	start := !m.flushing
	m.flushing = true
	m.mu.Unlock()
	if start {
		time.AfterFunc(deliveryBatch, func() { m.queue.Add(m.flush) })
	}
}

func (m *mailer) flush() {
	m.mu.Lock()
	rows := m.pending
	m.pending = nil
	m.flushing = false
	m.mu.Unlock()
	m.cache.edit(func(t *Tables) *Tables { return t.withDeliveries(rows) })
	cells := make([][]string, 0, len(rows))
	for _, row := range rows {
		line := make([]string, 0, len(DeliveryColumns))
		for _, column := range DeliveryColumns {
			line = append(line, row[column])
		}
		cells = append(cells, line)
	}
	if err := m.writer.AppendAll(appName, deliveriesTab, cells); err != nil {
		slog.Error("[ERROR] groups: delivery record", "rows", len(rows), "error", err)
	}
}

func (m *mailer) repliesTo(group string, lines []headerLine) bool {
	sent := map[string]bool{}
	for _, row := range m.cache.Tables().Messages {
		if row["State"] == stateSent && strings.EqualFold(row["Group"], group) && row["Message ID"] != "" {
			sent[messageKey(row["Message ID"])] = true
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
	for _, row := range m.cache.Tables().Messages {
		if strings.EqualFold(row["Group"], group) && messageKey(row["Message ID"]) == key {
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
	email := strings.ToLower(addressOf(address))
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
	if j.source == "" {
		return fail("fetch", fmt.Errorf("the message has no source to fetch"))
	}
	raw, err := m.mail.Store.Stored(ctx, j.source)
	if err != nil {
		return fail("fetch", err)
	}
	lines, body := splitMessage(raw)
	if reason := held(lines); reason != "" {
		log.Info("groups: message held", "reason", reason)
		m.mark(j, stateDropped, map[string]string{"Detail": reason})
		return stateDropped
	}
	if reason := authenticated(lines); reason != "" {
		log.Info("groups: message not authenticated", "reason", reason, "from", header(lines, "from"))
		m.mark(j, stateDropped, map[string]string{"Detail": reason})
		return stateDropped
	}
	sender := m.directory.Resolve(strings.ToLower(addressOf(header(lines, "from"))))
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
	object := fmt.Sprintf("loop/%s/%s-%s.eml", g.Name, time.Now().UTC().Format("20060102T150405Z"), j.id)
	if err := m.mail.Archive.Put(ctx, object, mailType, raw); err != nil {
		return fail("archive", err)
	}
	id := messageID(lines)
	m.mark(j, stateStored, map[string]string{"Object": object, "Message ID": id})
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

func (m *mailer) bounce(ctx context.Context, g Group, lines []headerLine, audience, verb string) error {
	to := addressOf(header(lines, "from"))
	subject := decodeHeader(header(lines, "subject"))
	text := fmt.Sprintf("Your message to %s, “%s”, was not sent to the group: only %s can %s to it.", g.Address(), subject, posters[audience], verb)
	headers := map[string]string{"Auto-Submitted": "auto-replied", loopHeader: g.Name}
	if id := header(lines, "message-id"); id != "" {
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
	recipients := strings.Split(fields["recipient"], ",")
	for _, recipient := range recipients {
		local, domain, _ := strings.Cut(strings.ToLower(addressOf(recipient)), "@")
		if domain == Domain && local == unsubscribeLocal {
			a.unsubscribeByMail(r.Context(), fields["subject"], fields["sender"])
		}
	}
	source := fields["message-url"]
	if source == "" {
		slog.WarnContext(r.Context(), "groups: inbound call carries no message-url", "recipient", fields["recipient"])
		w.WriteHeader(http.StatusOK)
		return
	}
	a.mailer.received(path.Base(source), source, fields["from"], fields["subject"], recipients)
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
