package groups

import (
	"context"
	"encoding/json"
	"fmt"
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

	"heliosian/internal/data"
	"heliosian/internal/mail"
)

type Inbox interface {
	Received(ctx context.Context, id string) (mail.Received, error)
}

type Archive interface {
	Put(ctx context.Context, name, mimeType string, content []byte) error
}

type Mail struct {
	Sender  mail.RawSender
	Inbox   Inbox
	Secret  string
	Key     []byte
	Base    string
	Archive Archive
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
	stateReceived  = "received"
	stateStored    = "stored"
	stateSent      = "sent"
	stateDropped   = "dropped"
	stateFailed    = "failed"
	maxWebhookBody = 1 << 20
	mailType       = "message/rfc822"
)

var sendInterval = 500 * time.Millisecond

var troubleEvents = []string{"email.bounced", "email.complained", "email.delivery_delayed", "email.failed", "email.suppressed"}

type job struct {
	id, group string
}

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
}

func newMailer(cache *Cache, writer data.Writer, queue Enqueuer, directory Directory, mailbox Mail) *mailer {
	m := &mailer{cache: cache, writer: writer, queue: queue, directory: directory, mail: mailbox, busy: map[string]bool{}, done: map[string]bool{}, work: make(chan job, 256)}
	go m.run()
	return m
}

func (j job) key() string {
	return j.id + "|" + j.group
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
			j := job{id: row["ID"], group: strings.ToLower(row["Group"])}
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
		return m.writer.Set(appName, messagesTab, map[string]string{"ID": j.id, "Group": j.group}, cells)
	})
}

func groupsIn(addresses []string) []string {
	out := []string{}
	for _, address := range addresses {
		local, domain, _ := strings.Cut(strings.ToLower(addressOf(address)), "@")
		if domain == Domain && !slices.Contains(out, local) {
			out = append(out, local)
		}
	}
	return out
}

func (m *mailer) received(id, from, subject string, addresses []string) {
	for _, name := range groupsIn(addresses) {
		if m.cache.Model().Group(name) == nil {
			slog.Warn("groups: mail for no group", "message", id, "group", name, "from", from)
			continue
		}
		j := job{id: id, group: name}
		if !m.take(j) {
			continue
		}
		slog.Info("groups: mail received", "message", id, "group", name, "from", from, "subject", subject)
		m.queue.Add(func() {
			cells := map[string]string{"Received": time.Now().Format(time.RFC3339), "From": from, "Subject": subject, "State": stateReceived, "Recipients": "", "Object": "", "Detail": ""}
			if err := m.writer.Set(appName, messagesTab, map[string]string{"ID": id, "Group": name}, cells); err != nil {
				slog.Error("groups: mail record", "error", err)
			}
		})
		m.work <- j
	}
}

func (m *mailer) trouble(event, from, messageID, detail string, addresses []string) {
	names := groupsIn([]string{from})
	if len(names) == 0 {
		return
	}
	when := time.Now().Format(time.RFC3339)
	for _, address := range addresses {
		email := strings.ToLower(addressOf(address))
		slog.Warn("groups: delivery trouble", "event", event, "group", names[0], "email", email, "detail", detail)
		m.queue.Add(func() {
			if err := m.writer.Append(appName, deliveriesTab, []string{when, names[0], email, event, messageID, detail}); err != nil {
				slog.Error("groups: delivery record", "error", err)
			}
		})
	}
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
	if m.mail.Inbox == nil || m.mail.Sender == nil {
		return fail("mail", fmt.Errorf("mail is not set up"))
	}
	received, err := m.mail.Inbox.Received(ctx, j.id)
	if err != nil {
		return fail("fetch", err)
	}
	lines, body := splitMessage(received.Raw)
	if reason := held(lines); reason != "" {
		log.Info("groups: message held", "reason", reason)
		m.mark(j, stateDropped, map[string]string{"Detail": reason})
		return stateDropped
	}
	object := fmt.Sprintf("loop/%s/%s-%s.eml", g.Name, time.Now().UTC().Format("20060102T150405Z"), j.id)
	if err := m.mail.Archive.Put(ctx, object, mailType, received.Raw); err != nil {
		return fail("archive", err)
	}
	m.mark(j, stateStored, map[string]string{"Object": object})
	head, err := rewrite(lines, *g)
	if err != nil {
		return fail("rewrite", err)
	}
	members := Members(*g, SourcesOf(m.directory))
	sent, failures := 0, []string{}
	for i, rcpt := range members {
		if i > 0 {
			time.Sleep(sendInterval)
		}
		link := m.mail.Base + "/unsubscribe/" + token(m.mail.Key, g.Name, rcpt)
		msg := render(head, []string{"List-Unsubscribe: <" + link + ">", "List-Unsubscribe-Post: List-Unsubscribe=One-Click"}, body)
		if err := m.mail.Sender.SendRaw(ctx, g.Address(), []string{rcpt}, msg); err != nil {
			log.Error("groups: send failed", "to", rcpt, "error", err)
			failures = append(failures, rcpt+": "+err.Error())
			continue
		}
		sent++
	}
	state := stateSent
	if sent == 0 && len(members) > 0 {
		state = stateFailed
	}
	log.Info("groups: forwarded", "members", len(members), "sent", sent, "failed", len(failures))
	m.mark(j, state, map[string]string{"Recipients": strconv.Itoa(sent), "Detail": strings.Join(failures, "; ")})
	return state
}

func (a app) webhook(w http.ResponseWriter, r *http.Request) {
	if a.mail.Secret == "" || a.mail.Inbox == nil {
		http.Error(w, "mail is not set up", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := mail.VerifyWebhook(a.mail.Secret, r.Header, body, time.Now()); err != nil {
		slog.WarnContext(r.Context(), "groups: webhook refused", "error", err)
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
			Bounce      struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"bounce"`
			Failed struct {
				Reason string `json:"reason"`
			} `json:"failed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil || event.Data.EmailID == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	d := event.Data
	switch {
	case event.Type == "email.received":
		a.mailer.received(d.EmailID, d.From, d.Subject, slices.Concat(d.To, d.CC, d.BCC, d.ReceivedFor))
	case slices.Contains(troubleEvents, event.Type):
		detail := d.Bounce.Message
		if detail == "" {
			detail = d.Failed.Reason
		}
		if detail == "" {
			detail = d.Bounce.Type
		}
		a.mailer.trouble(strings.TrimPrefix(event.Type, "email."), d.From, d.EmailID, detail, d.To)
	}
	w.WriteHeader(http.StatusNoContent)
}
