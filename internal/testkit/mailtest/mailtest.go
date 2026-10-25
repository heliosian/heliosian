package mailtest

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	netmail "net/mail"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"heliosian/internal/intercept"
	"heliosian/internal/mail"
)

const From = "Helios Sample <sample@example.org>"

type Raw struct {
	Domain    string
	To        []string
	Recipient string
	Message   []byte
}

type Recorder struct {
	Mailgun  *mail.Mailgun
	mu       sync.Mutex
	messages []mail.Message
	raws     []Raw
}

var (
	installed sync.Once
	mu        sync.Mutex
	recorders = map[string]*Recorder{}
)

func install() {
	installed.Do(func() {
		intercept.Install(mail.Host, http.HandlerFunc(serve))
	})
}

func SignMailgun(key, token string, at time.Time) (timestamp, signature string) {
	timestamp = strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(timestamp + token))
	return timestamp, hex.EncodeToString(mac.Sum(nil))
}

func Discard() *mail.Mailgun {
	install()
	return mail.NewMailgun("discard", From)
}

func NewRecorder(from string) *Recorder {
	install()
	mu.Lock()
	defer mu.Unlock()
	key := fmt.Sprintf("recorder-%d", len(recorders))
	r := &Recorder{Mailgun: mail.NewMailgun(key, from)}
	recorders[key] = r
	return r
}

func serve(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, key, _ := r.BasicAuth()
	mu.Lock()
	rec := recorders[key]
	mu.Unlock()
	if rec != nil {
		parts := strings.Split(r.URL.Path, "/")
		domain, endpoint := parts[2], parts[3]
		var err error
		switch endpoint {
		case "messages":
			err = rec.keep(r)
		case "messages.mime":
			err = rec.keepRaw(r, domain)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	w.Write([]byte(`{"id":"<intercepted@example.org>","message":"Queued. Thank you."}`))
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (rec *Recorder) keep(r *http.Request) error {
	form := r.MultipartForm.Value
	m := mail.Message{To: form["to"], CC: form["cc"], Subject: first(form["subject"]), HTML: first(form["html"]), Text: first(form["text"])}
	if line := first(form["from"]); line != rec.Mailgun.From() {
		m.FromName = line[:strings.LastIndex(line, " <")]
	}
	if replyTo := first(form["h:Reply-To"]); replyTo != "" {
		m.ReplyTo = strings.Split(replyTo, ", ")
	}
	for k, v := range form {
		if name, ok := strings.CutPrefix(k, "h:"); ok && name != "Reply-To" {
			if m.Headers == nil {
				m.Headers = map[string]string{}
			}
			m.Headers[name] = v[0]
		}
	}
	for _, header := range r.MultipartForm.File["attachment"] {
		f, err := header.Open()
		if err != nil {
			return err
		}
		content, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return err
		}
		m.Attachments = append(m.Attachments, mail.Attachment{Name: header.Filename, ContentType: header.Header.Get("Content-Type"), Content: content})
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.messages = append(rec.messages, m)
	return nil
}

func (rec *Recorder) keepRaw(r *http.Request, domain string) error {
	f, _, err := r.FormFile("message")
	if err != nil {
		return err
	}
	defer f.Close()
	content, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	raw := Raw{Domain: domain, To: r.MultipartForm.Value["to"], Recipient: first(r.MultipartForm.Value["v:"+mail.CopyVariable]), Message: content}
	var decoded *mail.Message
	if raw.Recipient != "" {
		if m, err := rec.decode(content); err == nil {
			decoded = &m
		}
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.raws = append(rec.raws, raw)
	if decoded != nil && !slices.ContainsFunc(rec.messages, func(m mail.Message) bool { return m.Headers["Message-Id"] == decoded.Headers["Message-Id"] }) {
		rec.messages = append(rec.messages, *decoded)
	}
	return nil
}

var plainHeaders = []string{"From", "To", "Cc", "Reply-To", "Subject", "Date", "Mime-Version", "Content-Type"}

func (rec *Recorder) decode(content []byte) (mail.Message, error) {
	msg, err := netmail.ReadMessage(bytes.NewReader(content))
	if err != nil {
		return mail.Message{}, err
	}
	list := func(name string) []string {
		out := []string{}
		for _, a := range strings.Split(msg.Header.Get(name), ", ") {
			if a != "" {
				out = append(out, a)
			}
		}
		return out
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil {
		return mail.Message{}, err
	}
	m := mail.Message{To: list("To"), CC: list("Cc"), Subject: subject, Headers: map[string]string{}}
	if replyTo := list("Reply-To"); len(replyTo) > 0 {
		m.ReplyTo = replyTo
	}
	if line := msg.Header.Get("From"); line != rec.Mailgun.From() {
		m.FromName = line[:strings.LastIndex(line, " <")]
	}
	for k, v := range msg.Header {
		if !slices.Contains(plainHeaders, k) {
			m.Headers[k] = v[0]
		}
	}
	if id := msg.Header.Get("Message-Id"); id != "" {
		m.Headers["Message-Id"] = id
	}
	return m, decodePart(msg.Header.Get("Content-Type"), msg.Body, &m)
}

func decodePart(contentType string, body io.Reader, m *mail.Message) error {
	kind, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(kind, "multipart/") {
		content, err := io.ReadAll(body)
		if err != nil {
			return err
		}
		text := strings.TrimSuffix(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
		switch kind {
		case "text/plain":
			m.Text = text
		case "text/html":
			m.HTML = text
		}
		return nil
	}
	parts := multipart.NewReader(body, params["boundary"])
	for {
		p, err := parts.NextPart()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if p.FileName() == "" {
			if err := decodePart(p.Header.Get("Content-Type"), p, m); err != nil {
				return err
			}
			continue
		}
		content, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, p))
		if err != nil {
			return err
		}
		partType, _, _ := strings.Cut(p.Header.Get("Content-Type"), ";")
		m.Attachments = append(m.Attachments, mail.Attachment{Name: p.FileName(), ContentType: partType, Content: content})
	}
}

func (rec *Recorder) Messages() []mail.Message {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return slices.Clone(rec.messages)
}

func (rec *Recorder) Raws() []Raw {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return slices.Clone(rec.raws)
}

func (rec *Recorder) Wait(t *testing.T, n int) []mail.Message {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if messages := rec.Messages(); len(messages) >= n {
			return messages
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("waited for %d messages", n)
	return nil
}
