package groups

import (
	"context"
	"encoding/json"
	"fmt"
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

type Mail struct {
	Sender     mail.RawSender
	Store      Fetcher
	SigningKey string
	Key        []byte
	Base       string
	Archive    Archive
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
	maxNotifyBody    = 40 << 20
	maxEventBody     = 1 << 20
	mailType         = "message/rfc822"
	unsubscribeLocal = "unsubscribe"
)

type job struct {
	id, group, source string
}

func (j job) key() string {
	return j.id + "|" + j.group
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

func (m *mailer) received(id, source, from, subject string, addresses []string) {
	for _, name := range groupsIn(addresses) {
		if m.cache.Model().Group(name) == nil {
			slog.Warn("groups: mail for no group", "message", id, "group", name, "from", from)
			continue
		}
		j := job{id: id, group: name, source: source}
		if !m.take(j) {
			continue
		}
		slog.Info("groups: mail received", "message", id, "group", name, "from", from, "subject", subject)
		m.queue.Add(func() {
			cells := map[string]string{"Received": time.Now().Format(time.RFC3339), "From": from, "Subject": subject, "State": stateReceived, "Recipients": "", "Object": "", "Detail": "", "Source": source}
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
	object := fmt.Sprintf("loop/%s/%s-%s.eml", g.Name, time.Now().UTC().Format("20060102T150405Z"), j.id)
	if err := m.mail.Archive.Put(ctx, object, mailType, raw); err != nil {
		return fail("archive", err)
	}
	m.mark(j, stateStored, map[string]string{"Object": object})
	head, err := rewrite(lines, *g)
	if err != nil {
		return fail("rewrite", err)
	}
	members := Members(*g, SourcesOf(m.directory))
	sent, failures := 0, []string{}
	for _, rcpt := range members {
		tok := token(m.mail.Key, g.Name, rcpt)
		unsubscribe := "List-Unsubscribe: <mailto:" + unsubscribeLocal + "@" + Domain + "?subject=" + tok + ">, <" + m.mail.Base + "/unsubscribe/" + tok + ">"
		msg := render(head, []string{unsubscribe, "List-Unsubscribe-Post: List-Unsubscribe=One-Click"}, body)
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

func notifyFields(r *http.Request) (map[string]string, error) {
	fields := map[string]string{}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var raw map[string]any
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			return nil, err
		}
		for k, v := range raw {
			if s, ok := v.(string); ok {
				fields[strings.ToLower(k)] = s
			}
		}
		return fields, nil
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(maxNotifyBody); err != nil {
			return nil, err
		}
	} else if err := r.ParseForm(); err != nil {
		return nil, err
	}
	for k, v := range r.Form {
		if len(v) > 0 {
			fields[strings.ToLower(k)] = v[0]
		}
	}
	return fields, nil
}

func (a app) inbound(w http.ResponseWriter, r *http.Request) {
	if !a.mail.ready() {
		http.Error(w, "mail is not set up", http.StatusNotFound)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxNotifyBody)
	fields, err := notifyFields(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := mail.VerifyMailgun(a.mail.SigningKey, fields["timestamp"], fields["token"], fields["signature"], time.Now()); err != nil {
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
			Event     string `json:"event"`
			Severity  string `json:"severity"`
			Recipient string `json:"recipient"`
			Reason    string `json:"reason"`
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
		kind = "delivery_delayed"
		if d.Severity == "permanent" {
			kind = "bounced"
		}
	case "complained":
		kind = "complained"
	}
	if kind != "" {
		detail := d.Status.Description
		if detail == "" {
			detail = d.Status.Message
		}
		if detail == "" {
			detail = d.Reason
		}
		a.mailer.trouble(kind, d.Message.Headers.From, d.Message.Headers.MessageID, detail, []string{d.Recipient})
	}
	w.WriteHeader(http.StatusOK)
}
